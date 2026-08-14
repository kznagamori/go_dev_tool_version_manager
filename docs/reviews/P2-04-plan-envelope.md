# P2-04 決定記録（3/3）: Plan dataとCLI JSON envelope

対象task: `docs/13-progress.md` P2-04（3分割の3本目、本PRでP2-04完了）
規範仕様: [04-storage-and-data.md](../04-storage-and-data.md) §16・§16.1・§16.2・§17・§17.1・§17.2、[02-architecture.md](../02-architecture.md) §14、[10-security.md](../10-security.md) §8・§9.2

## 1. 実装した範囲

| file | 対象 |
|---|---|
| `plan.go` | §16 Planのtyped表現、§16.1 `PlanWarningCode` 8値、enum群 |
| `planbuild.go` | §16 summary・inputs・SetupPlan・warningsの検査 |
| `planarray.go` | §16 downloads・extracts・probes・writes・storageの検査 |
| `planfile.go` | §16のexact key構造と書出し用整列 |
| `envelope.go` | §17 CLI JSON envelope、§16.2 `ResultWarningCode` 5値、§17.1のenum群 |
| `envelopefile.go` | §17のexact key構造とcommandごとの`data` |
| `pathvalue.go` | §17.2の`PathValue`とpath制約 |

これでP2-04（§8〜§18の12形式）が完了する。

## 2. 着手時に確認した境界

前回の停止記録で「§16の`PathValue`と§17.2のrole 22値を`domain.PathValue`でそのまま表現できるか」を確認事項としていた。**そのまま使える**。`domain.PathValue`（P1-02）は§17.2のexact key（`role`/`path`）と22 roleをすでに持ち、`PathRoleCount = 22`で件数も固定している。Plan/envelope固有のpath型は作らず、JSON構造だけを`pathValueJSON`として足した。

## 3. 判断

### 3.1 path制約を「絶対path必須」を既定とする組合せで表す

§17.2は3種類の制約を課す。

| 制約 | 対象 |
|---|---|
| OS nativeの正規absolute path | Plan・CLI JSONの`path`一般 |
| 空を許す | `SetupPlan`のprevious root・integration target・backup path、§17のoptional field |
| registry locator `HKCU\Environment\Path`を許す | `SetupPlan.integration_target`と`writes[]`の`registry-value`（**唯一の例外**） |

当初は択一のenumで表したが、`integration_target`は**空とlocatorの両方**を取るため表せなかった（testが検出した）。`allowEmpty`/`allowLocator`の2 boolを持つ`pathMode`にして解決した。

absolute判定は`filepath.IsAbs`を使わない。Plan JSONは生成したOSと別のOSで読むこと（両OS runnerでの検査を含む）があり、`runtime.GOOS`に依存すると片方のOSのpathを常に相対と誤判定する。POSIX（`/`）、UNC（`\\`）、drive（`C:\`）を明示的に判定する。`C:x`のようなdrive相対pathは、process ごとのcurrent directoryに依存するため拒否する。

roleもfieldごとに固定する（例: `receipt_path.role`は`receipt`、`report_path.role`は`report`）。§17.2の目的は「`doctor --report`がhome配下のpathをrole単位で置換できること」と「CIの書込み範囲検査がroleで封じ込めを判定できること」であり、roleが自由だとどちらも効かない。

### 3.2 warning codeの承認要否を表で固定する

§16.1が「`requires_explicit_approval=true`のcode集合がApprovalの単位」と定める。codeごとの真偽を**表で持ち**、Plan作成側の値と一致しなければ拒否する。作成側の判断に任せると、同じcodeが場面によって承認を要したり要さなかったりする。

同じcodeを2度出すことも拒否する。承認単位はcode集合なので、件数と集合サイズが食い違うと「何件承認したか」が決まらない。

`W_RESTART_REQUIRED`だけが承認不要であり、8件中7件が承認対象であることを`PlanApprovalCodeCount`とtestで固定した。

### 3.3 §16の同値制約を3つとも検査する

§16はPlanの内部整合を3か所で定める。どれも「表示と実態がずれない」ための規定である。

| 規定 | 検査 |
|---|---|
| `warning_count`は`warnings`の件数と一致 | 要約だけを見た利用者が警告を見落とさない |
| `restart_required=true`と`W_RESTART_REQUIRED` exactly 1件が同値 | 再起動が要るのに警告が出ない状態を作らない |
| previous 3 fieldは同時に空/非空、非空と`W_MODE_CHANGE` exactly 1件が同値 | mode変更を承認なしで通さない |

### 3.4 platform別capability契約を検査する

§16がWindows/Linuxごとに必須capabilityとstrategyの組合せを定める。

| link strategy | 必須capability | 許すshim strategy |
|---|---|---|
| `junction` | 共通4件＋`junction` | `hardlink`、`fallback-resolver` |
| `symlink` | 共通4件＋`symlink` | `symlink`、`fallback-resolver` |

共通4件は`atomic-replace`、`directory-rename`、`file-identity`、`owner-enforcement`である。あわせて§16の「hardlinkを使う場合**だけ**capabilityへ`hardlink`を含める」を双方向に検査する。capabilityとstrategyがずれたPlanは、実行時に想定と違う方式で切替えることになる。

必須capabilityを確認できない場合に`E_PLATFORM_UNSUPPORTED`とするのはsetup engineの責務であり、codecはPlanとして矛盾していないかだけを見る。

### 3.5 provider `none`をPlanだけで許す

§17.1が「対象toolがないPlan operationだけ`none`」と定める。receipt・catalogの許可集合（`receiptProviderKinds`）とPlanの許可集合（`planProviderKinds`）を分けた。あわせてoperationとの整合も見る。

- `setup`/`setup-remove`は`none`必須。tool/version/platform/channel/lifecycle/digest/checksumはすべて空。
- `install`/`use`/`uninstall`は`none`禁止。

`install`で`none`を許すと、どのartifactを扱うか不明なPlanを承認できてしまう。

### 3.6 third-partyは取得元と理由を必ず持たせる

[10-security.md](../10-security.md) §8が「外部programはPlanで名称、完全版、取得元、digest、license、実行理由、argv要約、書込み先を表示し、検証前に起動しない」と定める。§16の「officialのadoption reasonだけ空」と合わせ、third-partyの`provider_repository`・`provider_homepage`・`license`・`adoption_reason_message_id`が空のPlanを拒否する。

probeも同じ方針で、`version`・`source`（HTTPS URL）・`artifact_digest`・`license`・`reason_message_id`のいずれかが空なら拒否する。子processを起動する前に、何を起動するかを完全に示すためである。

### 3.7 `doctor`のdiagnosticsを10件・code順で固定する

§17.1が「上表の10件をcode順にexactly 1件ずつ返す」と定める。件数が足りないdoctor結果は、**検査していない項目を「問題なし」に見せてしまう**。10件ちょうどであることとcode順であることの両方を検査する。

statusも導出規則（error≥1なら`unhealthy`、errorなしでwarn≥1なら`degraded`、それ以外`healthy`）と一致させる。statusとdiagnosticsが食い違うと、利用者が総合判定と個別項目のどちらを信じればよいか決まらない。

### 3.8 commandごとの`data`をexact keyで固定する

§17がcommandごとに`data`のexact keyを定める。5 commandぶんを1つの構造体（`envelopeData`）へまとめ、`checkDataKeys`がcommandに対応するkeyだけがあることを検査する。1構造体にするのは`DisallowUnknownFields`を効かせるためで、command別の型にするとdecode前にcommandを読む必要があり、strict decodeが2 passになる。

encode側も同じ整合を見る。`Envelope`が`Command`と違うcommandのdataを持つ場合は拒否する（`checkEnvelopeFieldsMatchCommand`）。黙って落とすと、呼出し側が渡したはずのdataが出力から消える。

### 3.9 `available`のitemはschemeを持たない

§17は「`CatalogItem`は§15 itemのexact key集合」と定めるが、CLI JSONにschemeは無い。catalog（2本目）ではschemeを必須にしたが、envelopeでは要求しない。§17は順序契約を再掲しておらず、順序はcatalogを読んだ時点で検査済みだからである。

`CatalogItem`へ`VersionText`を足し、schemeを持つ場合（catalog由来）と持たない場合（CLI JSON由来）の両方を1つの型で表す。field検査は`buildCatalogItemFields`として共通化し、片方だけが緩む余地を無くした。

### 3.10 retryableを非retryable codeへ載せない

[02-architecture.md](../02-architecture.md) §14の非retryable code（8件、P1-04で`domain.IsRetryableAllowed`として実装済み）に`retryable=true`を載せたenvelopeを拒否する。「再実行できる」と表示された失敗が、実際には何度やっても直らないためである。

### 3.11 Plan/envelopeの失敗を`E_INTERNAL`にする

Planとenvelopeはgdtvm自身が作るものであり、契約に合わないのは利用者入力ではなく内部誤りである。state（`E_STATE_CORRUPT`）、receipt（`E_RECEIPT_INVALID`）、catalog（`E_CATALOG_MISSING`）と分けた。

## 4. 検証で見つかった欠陥

本PRの検査が実装の欠陥を**3件**検出した。

### 4.1 encode経路が自分の出力を読み直せない

`jsonToScalar`が`json.Number`（decode由来）だけを受け付け、`int64`（`encodeScalarMap`由来）を「scalarでない値」として拒否していた。encode経路もparseと同じ検査を通す設計のため、**parametersにintegerを持つPlan/envelopeがencodeできなかった**。両方の型を受け取るよう修正した。

### 4.2 `file_name`がpathを受理していた

§16の`downloads[].file_name`と§15の`artifact_file`が`dist/node.tar.gz`のような区切り付きの値を通していた。download先がcache directoryやstaging directoryの外へ出る。2本目でreceiptの`artifact.file`にだけ入れていた検査を`requireFileName`として共通化し、3か所すべてへ適用した。

### 4.3 command以外のdataを黙って落としていた

`Envelope`が`Command=current`かつ`Installs`を持つ場合、`installs`が出力から静かに消えていた。§3.8の`checkEnvelopeFieldsMatchCommand`を足して拒否するようにした。

## 5. 検証

### 5.1 ローカル検証

Linux container（Go 1.26.5、Python 3.11.15）で実行した。

| command | 結果 |
|---|---|
| `gofmt -l .` | 0 file |
| `go vet ./...` | 成功 |
| `go test ./... -race -shuffle=on -covermode=atomic -coverprofile=coverage.out` | 成功。total 90.1% |
| `scripts/ci/check_policy.py` | 成功。production Go file 75件 |
| `scripts/ci/check_imports.py` | 成功。package 20件 / internal import 68件 |
| `scripts/ci/check_docs.py` | 成功。file 35件 |
| `scripts/ci/check_licenses.py` | 成功。module 14件 |
| `scripts/ci/check_pr_refs.py` | 成功（task-id=p2-04, slug=plan-envelope） |
| `git diff --check` | 差分なし |

package別coverage: `internal/progress` 100.0%、`internal/security` 99.0%、`internal/domain` 95.8%、`internal/app` 94.9%、`internal/config` 93.3%、`internal/domain/port` 92.7%、`internal/store` 88.9%、`internal/domain/port/fake` 86.1%、`cmd/gdtvm` 66.7%。

test件数: `internal/store` 569件（subtest込み）、うちnegative subtest 380件。

### 5.2 fixtureを2種類持つ

§16の構造例は**operation entryを空にしている**（同§が明記）。この例だけでは downloads/extracts/probes/writes/storage の契約がまったく検査されず、securityでもっとも重要な部分（外部programの起動と利用者可視の書込み）が素通りする。仕様例の`specPlanJSON`に加えて、全配列を埋めた`fullPlanJSON`を持つ。

§16は「semantic validatorは実行可能なPlanとして拒否しなければならない」とも定めるが、これはPlan**作成**側の責務（P6-02）である。codec層はkey形状の検査までを担うため、構造例はcodecとしては通るのが正しい。この分担をtestのコメントへ書いた。

### 5.3 主なnegative test

| 対象 | 内容 |
|---|---|
| §16 top-level | 19件。unknown key（top-level/summary）、重複key、schema 2、operation enum外、ID不正2件、client_version不正、created_at非UTC、summary欠落、inputsのroot_id/digest形式/revision負、warning code 2件、warningsとdownloadsのnull、trailing data、BOM、空 |
| §16 warning整合 | 4件。approval反転2件（W_THIRD_PARTY／W_RESTART_REQUIRED）、warning_count不一致、code重複 |
| §16 SetupPlan | 20件。setup/operation排他2件、capability 7件、previous field 2件、restart 1件、integration target 4件、toolless summary 6件 |
| §16 downloads | 11件。destination role、URL非HTTPS・userinfo、digest形式、officialのadoption reason、file_nameの区切り、size負、ID grammar、third-party 2件 |
| §16 extracts | 5件。source_download_id不在、format enum外、strip 2/負、destination role |
| §16 probes | 12件。version空、source非HTTPS、license空、reason 2件、timeout 2件、expect別contract 2件、working_directory相対、required_path kind、stream enum外 |
| §16 PlanArg | 6件。literalにpath、literalのvalue空、pathにvalue、pathのpath null、kind enum外、path相対 |
| §16 writes/storage | 7件。registry-value以外のlocator、role不一致、action enum外、scope/purge不整合、target相対、kind enum外 |
| §16 ID/順序 | 5件。種類をまたいだID重複2件、配列のID降順、writes降順 |
| §17 envelope | 14件。unknown key、重複key、schema 2、command書込み系・enum外、invocation_id、warnings欠落、error code/message_id、parameters 2件、trailing data、BOM、空 |
| §17 data/error排他 | 4件。ok=trueでerror、ok=trueでdata無し、ok=falseでdata、ok=falseでerror無し |
| §17 command別data | 6件。対応しないkey、余分なkey、必須key欠落 |
| §17 doctor | 6件。件数不足、code順違反、status/severity不一致3件 |
| §17 SelectionSummary | 7件。source別のproject_file/version/install_id/payload_path整合 |
| §17 PathValue | 6件。role不一致、role enum外、相対path、空path、drive相対path、path欠落 |
| §17 その他 | 6件。retryable不整合、Plan warning codeの誤用、approval fieldの混入、development/version不一致、installs順序2件 |

誤検出しないことも同じtestで確認している。Windows absolute path（`D:\...`）とPOSIX absolute pathの両方が通ること、`expect=path-within`が`expected_root`を持てること、`strip_components=0`が通ること、registry locatorが`registry-value`でだけ通ることを固定した。

P1-03のglobal state検査が新規package変数21件を検出したため、根拠を添えて許可表へ追加した。P2-02・P2-03・P2-04(1/3)・P2-04(2/3)に続き5回目である。

### 5.4 CI

<!-- PR作成後にworkflow runとcheck結果を追記する -->

## 6. 未実施・制約

- `go tool govulncheck ./...` はこのcontainerから`vuln.go.dev`が`Forbidden`で到達できず、ローカルでは未実行である（P0-02から継続）。本PRは新規依存を追加していない。
- Planの**semantic validation**（operationごとに必要なdownload/extract/probeが揃っているか）はPlan作成側の責務であり、[11-quality-and-ci.md](../11-quality-and-ci.md) §6と**P6-02**で検査する。本PRはkey形状・enum・整合の検査までである。
- `E_PLATFORM_UNSUPPORTED`の判定（必須capabilityを確認できない場合にPlanを作らない）はsetup engineの責務である。
- `inputs`の各値を実体から再取得して照合するのはExecuteの責務であり、`E_PLAN_STALE`の判定を含めてP6以降である。
- Approvalがcode集合を満たすかの検査（`E_APPROVAL_REQUIRED`）はExecuteの責務である。本PRは`Plan.ApprovalCodes()`で集合を返すところまでとした。
- §19 lock metadataは**P2-05**の範囲である。P2-04のtask行が列挙する対象に含まれていない。
- atomic write、revision採番、破損復旧、log rotationは**P2-05**である。
- `ToolID`の長さ上限は仕様に規定が無く未設定のままである（P1-02から継続）。
- Windowsでの実行はCI matrixでのみ確認する。
