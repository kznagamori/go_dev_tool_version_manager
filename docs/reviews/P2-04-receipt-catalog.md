# P2-04 決定記録（2/3）: install receiptとcatalog JSON

対象task: `docs/13-progress.md` P2-04（3分割の2本目）
規範仕様: [04-storage-and-data.md](../04-storage-and-data.md) §7・§14・§15・§17.1・§21、[06-tool-definition.md](../06-tool-definition.md) §3・§7.2・§8・§11・§12

## 1. 実装した範囲

| file | 対象 |
|---|---|
| `receipt.go` | §14 install receiptのtyped表現と検査 |
| `receiptfile.go` | §14のexact key構造、共通field helper、書出し用の整列 |
| `catalog.go` | §15 catalog JSONのparse/encode |
| `template.go` | receipt内templateの許可rootと子path検査 |

1本目（PR #40）の共通codec層をそのまま使い、`requireHTTPSURL`と`parseUpstreamDigest`、`parseOptionalTimestamp`を本PRで復活させた。1本目では未使用だったため`CLAUDE.md` §7に従って削除してあった。

## 2. 着手時に確認した境界

前回の停止記録で「§14のtemplate（`{{payload}}`・`{{storage.<id>}}`）の展開検査をcodec層とP3 definitionのどちらが持つか」を次タスクの確認事項としていた。仕様から一意に決まった。

§14は「target/fixed args/path/setで許すtemplateは`{{payload}}`と**receiptに存在する**`{{storage.<id>}}`およびその子pathだけ」と定める。「receiptに存在する」はreceipt自身の`[[storage]]`との照合であり、**definitionを必要としない**。したがって次の分担になる。

| 対象 | 担当 |
|---|---|
| templateのgrammar、許可root、子path、storage IDの実在 | 本PR（codec層） |
| templateの評価（実pathの生成） | `internal/definition`・runtime（[02-architecture.md](../02-architecture.md) §2「テンプレート評価」） |

## 3. 判断

### 3.1 templateの許可rootをcommandとprobeで分ける

§14がreceiptのcommand系へ許すrootと、[06-tool-definition.md](../06-tool-definition.md) §11がprobeへ許すrootは異なる。

| 場所 | `{{payload}}` | `{{storage.<id>}}` | `{{probe_temp}}` |
|---|:---:|:---:|:---:|
| command target、fixed args、storage path、environment `set` | ○ | ○ | × |
| probe args、required_paths、expected_root | ○ | ○ | ○ |

§12が「`{{probe_temp}}`はvalidation probe内だけ」と定める。commandがこれを使えると、probe終了後に削除される一時directoryを恒久的な参照先にしてしまう。同じtemplateがprobe argsでは通りcommand fixed_argsでは落ちることをtestで固定した。

`{{version}}`、`{{platform.id}}`のようなdefinition側のrootはreceiptでは拒否する。§14が「metadata/version/staging/outputや再帰展開は禁止する」と定めており、receiptはrender済みの値を持つためである。

root直後のliteral連結（`{{payload}}bin/node`）も拒否する。§11が「path templateへのliteral prefix/suffix連結は拒否」と定める。子pathは`/`区切りのPOSIX relative pathだけを許し、子pathの中の`{{...}}`は再帰展開として拒否する。

### 3.2 未知変数をliteralとして通さない

templateに一致しない値はliteralとして通すが、値のどこかに`{{...}}`が残っていれば拒否する。`{{staging}}/x`のような未知変数を「templateではないただの文字列」として受理すると、render後もそのまま残ってpath componentに`{`が入る。§12の「未知変数、再帰展開、function、condition、shell evaluationを禁止する」に対応する。

### 3.3 storage scopeとpurgeの組合せを固定する

[06-tool-definition.md](../06-tool-definition.md) §8が定める組合せを検査する。

| scope | 許すpurge |
|---|---|
| `tool` | `retain`、`explicit` |
| `version` | `with-version`だけ |

検査しないと、version scopeのstorageがuninstall後も残る、あるいはtool scopeがversionと一緒に消える、といった状態を受理してしまう。

### 3.4 承認fieldの整合を双方向に見る

§14は「third-partyなら`third_party_approved=true`必須」と定める。加えて、**officialなのに`third_party_approved=true`が立っている**receiptも拒否した。承認は必要なときだけ記録する値であり、不要な承認が残ると「何を承認したのか」が読めなくなる。

`license_notice_approved`はdefinitionが当該platformへ`license_notice`を宣言しているかどうかで必須性が決まる（§14）。definitionを持たないcodec層では判定できないため、boolとして読むところまでとした。整合の検査はdefinitionが入るP3以降である。

### 3.5 probeのexpect別契約を固定する

[06-tool-definition.md](../06-tool-definition.md) §11の表と§14の「非該当string/arrayは空」を組にして検査する。

| expect | 必須 | 空でなければならない |
|---|---|---|
| `version` | `regex`、`expected_version` | `expected_root` |
| `success` | — | `expected_version`、`expected_root`、`reported_version` |
| `path-within` | `regex`、`expected_root` | `expected_version`、`reported_version` |

あわせて§14の「required=trueはpassed必須」を検査する。skippedのrequired probeがreceiptに残ると、検証していない導入を検証済みとして扱うことになる。`reported_version`が`expected_version`と食い違うreceiptも拒否する。

### 3.6 environment変数名をplatform規則で一意にする

§14が「env map keyはplatform規則で一意」と定める。Windowsは環境変数名をcase非依存に扱うため`PATH`と`Path`は同じ変数であり、両方あるprofileは1つの変数への矛盾した指定になる。Linuxはcase sensitiveのため別変数として通す。receiptが`platform_id`を持つため、codec層で判定できる。

文字集合は§14が定めていないため、POSIXとWindowsの両方で安全な`^[A-Za-z_][A-Za-z0-9_]*$`へ限った。`=`とNULはどちらのOSでも環境block自体を壊す。

### 3.7 `set`をpointerで受ける

`environment_profiles[].set`を`*map[string]string`にした。§14は「全key必須」と定めるため、**空tableとkey欠落を区別**しなければならない。当初は`map[string]string`で受けていたが、storage無しreceiptのtestが「空の`[environment_profiles.set]`」と「tableごと欠落」の両方をnilとして拒否したことで判明した。他のfieldと同じくpointerにして解決した。

### 3.8 catalogはschemeを必須にする

§15が「itemsはversion comparison降順、同値ならversion byte順」と定める。**comparisonはschemeを要する**ため、`ParseCatalog`は`CatalogRequest.Scheme`を必須にした。

state file（1本目）ではversionをtextのまま持ちschemeを要求しなかったが、catalogでは扱いを変えている。理由は§15自身にある。catalogは「definition/platform不一致時に利用しない」cacheであり、`definition_sha256`を持つ。**catalogを読む側は必ずdefinitionを持っている**ため、schemeを要求しても呼出し側が困らない。逆にschemeなしで読めるようにすると、順序契約を検査できないまま通すことになる。

byte順とcomparisonが食い違う例（`22.9.0` と `22.18.0`）を両方向でtestし、byte順に並んだcatalogが拒否されることを固定した。

### 3.9 URLをHTTPS・userinfoなしへ限る

§7が「URL | HTTPS、userinfoなし、最大8 KiB」と定める。userinfoを拒否するのは、`https://user:token@host/`のようなURLがcatalogやreceiptへ保存されるとcredentialが平文でdiskとlogへ残るためである（[10-security.md](../10-security.md) §9.2）。

`source_identity`だけはURLとdefinition記録のどちらにもなる（§15「`json-index` sourceの`source_identity`はindex文書のURL」「override/staticならdefinition記録を使う」）。schemeを持つ値だけURLとして検査し、`http://`を黙って通さないようにした。`definition:static`のような値はURLと判定しない。

### 3.10 error codeをstate・receipt・catalogで分ける

[03-cli.md](../03-cli.md) §7の終了code表に従う。

| 対象 | code | 理由 |
|---|---|---|
| state file | `E_STATE_CORRUPT` | 正本stateの破損 |
| receipt | `E_RECEIPT_INVALID` | 導入記録の不整合 |
| catalog | `E_CATALOG_MISSING` | cacheであり再取得で回復できる |

いずれも`PathRole`だけを載せ、実pathをmessage parameterへ入れない（P2-02・P2-03と同じ方針）。

## 4. 検証

### 4.1 ローカル検証

Linux container（Go 1.26.5、Python 3.11.15）で実行した。

| command | 結果 |
|---|---|
| `gofmt -l .` | 0 file |
| `go vet ./...` | 成功 |
| `go test ./... -race -shuffle=on -covermode=atomic -coverprofile=coverage.out` | 成功。total 92.8% |
| `scripts/ci/check_policy.py` | 成功。production Go file 68件 |
| `scripts/ci/check_imports.py` | 成功。package 20件 / internal import 58件 |
| `scripts/ci/check_docs.py` | 成功。file 35件 |
| `scripts/ci/check_licenses.py` | 成功。module 14件 |
| `scripts/ci/check_pr_refs.py` | 成功（task-id=p2-04, slug=receipt-catalog） |
| `git diff --check` | 差分なし |

package別coverage: `internal/progress` 100.0%、`internal/security` 99.0%、`internal/domain` 95.8%、`internal/app` 94.9%、`internal/config` 93.3%、`internal/store` 93.1%、`internal/domain/port` 92.7%、`internal/domain/port/fake` 86.1%、`cmd/gdtvm` 66.7%。

test件数: `internal/store` 389件（subtest込み）、うちnegative subtest 265件。

### 4.2 「全件必須」をkey単位で検査する

§14と§15の「全件必須」を個別のnegative testで書くと、fieldを増やしたときに検査漏れが黙って通る。仕様の例から**1行ずつkeyを落として全件が拒否されること**を確かめるtestを置いた（`TestReceiptRequiresEveryKey`、`TestCatalogRequiresEveryKey`）。合計78件のkey欠落caseを自動生成しており、receiptに56件、catalogに22件が対応する。

catalogの`expires_at`だけは欠落を許す。§15が「static sourceは`expires_at=null`を許す」と定めるためで、この1件だけ期待値を反転させている。

### 4.3 主なnegative test

| 対象 | 内容 |
|---|---|
| §14 key欠落 | 56件。仕様の例から1行ずつ落として全件拒否 |
| §14 template | 11件拒否／5件受理。`{{version}}`、`{{platform.id}}`、command側の`{{probe_temp}}`、未定義storage、未知変数、literal連結（前後）、子pathの再帰展開、子pathの相対参照・絶対path、空 |
| §14 required_paths | 5件拒否／4件受理。prefix無し、`dir:`、`FILE:`、`{{version}}`、未定義storage |
| §14 storage | 3件。tool scopeに`with-version`、version scopeに`retain`／`explicit` |
| §14 probe | 7件。expect別の必須・禁止field、reported/expectedの不一致 |
| §14 承認 | 2件。未承認third-party、officialにthird_party_approved |
| §14 環境変数 | 2件。Windowsのcase違い重複（Linuxでは通ることも確認） |
| §14 その他 | 25件。unknown key、schema 2、payload_path他値、client_commitの長さ・大文字、definition_path絶対、artifact欠落、URL非HTTPS・userinfo、digestの形式・hex長不一致、file名に区切り、size負、checksum_source enum外、commands空、未定義profile、working_directory enum外、storage ID重複、storage kind enum外、probe timeout上下限、stream enum外、command_targetsのpayload外・digest形式、環境変数名grammar、BOM、1 MiB上限 |
| §15 key欠落 | 22件（`expires_at`は許容側で検査） |
| §15 順序 | 3件。comparison昇順、byte順で並べたcomparison昇順、同一versionの重複 |
| §15 scheme | 2件。未指定、未知scheme |
| §15 installable整合 | 4件。trueでreasonあり、falseでreason空、message IDでない、segment 1件 |
| §15 その他 | 24件。unknown key（top-level/item）、重複key、schema 2、tool_id大文字、platform未対応、digest形式、source_identityの空・HTTP、channel/lifecycle/provider_kind/checksum_source enum外、evidence非HTTPS、artifact_urlのuserinfo、size負・小数、artifact_file絶対、version範囲指定・latest、expires_at逆転、trailing data、BOM、空 |
| encode経路 | 25件。receipt 15件、catalog 10件 |
| URL | 8件拒否／4件受理 |

誤検出しないことも同じtestで確認している。`lifecycle=unknown`でもevidenceとassessed_atが必須であること、`published_at`だけは空を許すこと、`expires_at`が`fetched_at`と同時刻なら通ること、item 0件のcatalogが有効であることを固定した。

P1-03のglobal state検査が新規package変数17件を検出したため、根拠を添えて許可表へ追加した。P2-02・P2-03・P2-04(1/3)に続き4回目である。

### 4.4 CI

PR #43（commit `e696067`、workflow run 31777345312）で、6 job×2 OSの **12 checkすべてがsuccess** になった。

## 5. 未実施・制約

- `go tool govulncheck ./...` はこのcontainerから`vuln.go.dev`が`Forbidden`で到達できず、ローカルでは未実行である（P0-02から継続）。本PRは新規依存を追加していない。
- §16 Planと§17 CLI JSON envelopeは**3本目**の範囲である。
- `license_notice_approved`の必須性判定はdefinitionを要するためP3以降である。本PRはboolとして読むところまでとした。
- templateの**評価**（実pathの生成、canonical containment再検査）は`internal/definition`・runtimeの責務であり、本PRは形の検査だけを行う。
- `command_targets`が「required runtime commandのtargetとfixed argsが参照するpayload内fileだけ」であること（§14）の照合は、commandのtargetをrenderしないと判定できないためP3以降である。本PRはpayload配下であることと一意・整列までを見る。
- `artifact.size`とcatalogの`artifact_size`の実値照合はdownload時の責務である（P5）。
- atomic write、revision採番、破損復旧、log rotationは**P2-05**である。
- `ToolID`の長さ上限は仕様に規定が無く未設定のままである（P1-02から継続）。
- Windowsでの実行はCI matrixでのみ確認する。
