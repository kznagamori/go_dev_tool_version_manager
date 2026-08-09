# P1-04 決定記録: typed error、message ID、exit code、mask、ID、cancel/progress、structured logger

対象task: `docs/13-progress.md` P1-04
規範仕様: [02-architecture.md](../02-architecture.md) §4.1・§5・§10・§14・§15、[03-cli.md](../03-cli.md) §7、[04-storage-and-data.md](../04-storage-and-data.md) §7・§16.2・§18・§20、[09-platform.md](../09-platform.md) §9、[10-security.md](../10-security.md) §9.2・§12

## 1. 実装した範囲

| 対象 | 実装 |
|---|---|
| error code / exit code | `internal/domain` の `ErrorCode`（34件）、`ExitCode()`、`AllErrorCodes()` |
| typed error | `internal/domain` の `Error`、`Internal()`、`CodeOf()`、`ExitCodeOf()` |
| scalar parameter | `internal/domain` の `Scalar`、`Parameters`、key grammar |
| message ID | `internal/domain` の `MessageID` |
| invocation / operation ID | `internal/domain` の `InvocationID`、`OperationID`、`NewInvocationID`／`NewOperationID` |
| progress / cancel / warning | `internal/progress` の `Progress`、`Phase`（10）、`Unit`（4）、`Sink`、`CancelToken`、`Reporter`、`ResultWarningCode`（5） |
| secret masking | `internal/security` の `IsSecretEnvName`、`IsSecretHeader`、`MaskURL`、`PathMasker` |
| structured logger | `internal/domain/port` の `LogLevel`（5）、`LogRecord`、`Logger`、`Random`。fakeは `internal/domain/port/fake` |

## 2. 仕様が一意に決まらなかった3件と利用者判断

`CLAUDE.md` §2に従い、推測で埋めず利用者へ確認した。確定内容は同じ変更で仕様書へ規範化した。

| 項目 | 仕様の記述 | 確定内容 | 反映先 |
|---|---|---|---|
| scalar parameter keyのgrammar | [04](../04-storage-and-data.md)§18「key ASCII grammar」だけで実体なし | `^[a-z][a-z0-9_]*$`、1～64文字 | §7へ行を追加、§18から参照 |
| message IDのgrammar | [04](../04-storage-and-data.md)§20「ASCII dotted key集合」だけ | `^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)*$`、segment 2件以上、1～128文字 | §7へ行を追加、§20から参照 |
| `Retryable=true`を禁じるcode | [02](../02-architecture.md)§14「checksum/path/identity/registry corruption」だけで列挙なし | exactly 8件 | §14へcode一覧を追加 |

parameter keyは、typed errorの`parameters`、progressの`parameters`、Plan/result warningの`parameters`、structured logの`fields`で共通とした。同じ値を指すkeyが箇所ごとに`tool_id`と`toolId`へ分かれると、message templateのplaceholder（§20）とCLI JSONを突き合わせられなくなる。

`Retryable=true`を禁じる8件は `E_CHECKSUM_MISMATCH`、`E_ARCHIVE_UNSAFE`、`E_PATH_UNSAFE`、`E_PATH_CONFLICT`、`E_REGISTRY_INVALID`、`E_DEFINITION_INVALID`、`E_STATE_CORRUPT`、`E_RECEIPT_INVALID`。仕様の文言（4分類）より広く取り、[10-security.md](../10-security.md)§13のfail closed方針に寄せた。

## 3. 判断

### 3.1 error codeは分類ではなく表で持つ

[03-cli.md](../03-cli.md)§7の終了code表をそのまま`map[ErrorCode]int`にした。code名から終了codeを推測する実装（`E_PATH_*`は7、など）は表の変更に追随できない。testは仕様表を独立に書き下して突き合わせるので、実装表の写し間違いを検出できる。

閉じた集合の外は`E_INTERNAL`の終了code 1へ落とす。§14の「未分類codeを公開境界へ返さない。想定外の内部失敗だけは公開code`E_INTERNAL`、終了code 1へ変換する」をfail closedで実装したものである。

[09-platform.md](../09-platform.md)§9のplatform error 7件が§7の34件に含まれることをtestで確かめた。§14が両表の和集合を閉じた集合と定めるため、platform側にだけあるcodeが生まれると終了codeへ写像できなくなる。

### 3.2 `Error`はfield組立てとValidateを分ける

`BuildInfo`（P1-03）と同じ形にした。errorを作る箇所は例外処理の途中にあり、そこで多段のconstructor errorを扱うと本来のerrorが埋もれる。公開境界（CLI adapter、JSON codec）が出力前に`Validate()`する。

`Error()`文字列に`Cause`を含めない。§14が「`Cause`はdebug log用でJSON/public messageへ直接serializeしない」と定めるため、呼出し側が`err.Error()`をそのまま表示しても内部errorが漏れない構造にした。causeは`Unwrap()`と構造化logから辿る。testはcauseにcredential付きURLを入れ、表示文字列へ現れないことを確かめている。

### 3.3 `Progress`と`ResultWarning`は`internal/progress`

[02-architecture.md](../02-architecture.md)§2が同packageへ「型付き進捗、warning、cancel境界」を割り当てている。§4.1の抽象port表は`ProgressSink`を挙げるが、§4の`Ports` fieldからは外し「progress/cancelはrequestごとに渡す」と定めるため、`internal/domain/port`へは置かなかった。`Progress`が`internal/progress`にある以上、`ProgressSink`を`internal/domain/port`へ置くとdomain配下がdomain外をimportすることになり、§1の依存方向にも反する。

### 3.4 単調性は`Reporter`が持つ

§10の「Currentは単調非減少」はProgress 1件では検査できない。`Reporter`が直前値と突き合わせ、違反した通知を捨てる。捨てるのは、進捗表示の不整合のために本来の処理を失敗させるのが割に合わないためで、破棄件数は`Dropped()`でtestが検出できる。

単調性はphase単位で見る。phaseが変わればcurrentは新しい対象の計数へ切り替わり、download 100 byteのあとにextract 0 itemが来るのは正常である。

### 3.5 `MessageID`はdomain値

§14・§10の宣言は`MessageID string`だが、`ToolID`/`Version`/`Digest`（P1-02）と同じく検証付きconstructorを通す型にした。§2は「メッセージIDとカタログ」を`internal/message`へ割り当てるが、`Error`と`Progress`が持つ値の型を`internal/message`へ置くと`internal/domain`が外側をimportすることになり§1に反する。`internal/message`はcatalog（`ja.toml`の読込み、template展開、key/parameter集合の一致検査）を担当する分担とし、P4-02で実装する。

message IDのうち実装側で固定したのは2件だけである。`error.internal`（§14が定める内部失敗の変換路）と`error.cancelled`（§10が定めるcancelの変換路）で、いずれも仕様が変換先を明示しているためcatalog未整備でも必要になる。他のIDはcatalogを作るtaskで決める。

### 3.6 `Logger`と`Random`をportへ追加

P0-03で残していた8 portのうち2件を、最初に必要とする本taskで追加した。`Ports`は8 fieldになった。`LogRecord`は§18のkey集合と同じ項目を持ち、`Random`は§4.1の「128 bit ID生成」に対応する。

`Random`は`[16]byte`を返し、hex encodingは`domain.NewInvocationID`／`NewOperationID`が行う。encodingをdomain側へ置くことで、port実装ごとに大文字hexやUUID表記へぶれないようにした。

`Logger`に`Enabled(LogLevel) bool`を持たせた。記録されないlevelのためにfieldの組立てとmaskを行うのは無駄であり、呼出し側が先に判定できるようにするためである。

maskは型で強制していない。何がsecretかはfieldの意味に依存し、型では表せない。§15が「呼出側とsinkの両方でmask」と定めるとおり二重に行い、[10-security.md](../10-security.md)§12が要求するmask漏れのnegative testで担保する。

### 3.7 `MaskURL`はquery値を種類によらず置換する

[10-security.md](../10-security.md)§9.2は「URL userinfo、既知のtoken query key」を除去対象とするが、「既知のtoken query key」の一覧を定めていない。一覧を実装側で作ると、載っていないkey名のtokenが素通りする。そこでquery値を種類によらず置換した。要求される集合の上位集合であり、§13のfail closed方針に沿う。key名は残すので、どのparameterが付いていたかは診断できる。

解析できない文字列は丸ごと`<redacted>`にする。解析失敗をそのまま返すと、壊れたURLに埋まったcredentialが素通りする。

`PathMasker`は置換対象を呼出し側から受け取る。home、user名、hostnameの実値はOS user lookupの結果であり、`internal/security`がOS APIを直接呼ぶと§1の依存方向に反する。置換は長い文字列から先に適用する。user名がhome pathの一部である場合（`/home/alice`と`alice`）、短い方を先に適用すると`/home/<USER>`になりhome全体の置換規則が効かなくなる。

### 3.8 `check_imports.py`のALLOWED表

| 追加 | 根拠 |
|---|---|
| `internal/domain/port` → `internal/domain` | `LogRecord`と`Random`のsignatureがdomain値を使う |
| `internal/domain/port/fake` → `internal/domain` | 同signatureを実装するため |
| `internal/progress` → `internal/domain` | `Progress`/`ResultWarning`がmessage ID、scalar、ID、tool/versionを持つ |
| `internal/security` → `internal/domain` | §9.2のmaskをscalar parameterへ適用する |

1件目はP0-03から「port signatureをdomain型へ寄せるか」として持ち越していた検討事項で、本taskで寄せる判断をした。

### 3.9 `allowedGlobals`の追加

P1-03のglobal state検査が、本taskで増えた12件のpackage-level varをすべて検出した。いずれもcompile済みregexpか読取り専用の対応表であり、根拠を添えて表へ追加した。検査が空振りしていないことの実地確認にもなっている。

## 4. 検証

### 4.0 CI

PR #26（commit `9883e53`、workflow run 31332059134）で、6 job×2 OSの **12 checkすべてがsuccess** になった。

`unit`のtotal coverageは`ubuntu-latest`・`windows-latest`ともに91.8%で、両OSとも`-race`付き（`covermode=atomic`）で実行された。

### 4.1 ローカル検証

Linux container（Go 1.26.5、Python 3.11.15）で実行した。

| command | 結果 |
|---|---|
| `gofmt -l .` | 0 file |
| `go vet ./...` | 成功 |
| `go test ./... -race -shuffle=on -covermode=atomic -coverprofile=coverage.out` | 成功。total 91.8% |
| `scripts/ci/check_policy.py` | 成功。production Go file 54件 |
| `scripts/ci/check_imports.py` | 成功。package 20件 / internal import 24件 |
| `scripts/ci/check_docs.py` | 成功。file 29件 |
| `scripts/ci/check_pr_refs.py --head claude/feature-p1-04-error-and-observability --base claude/work` | 成功 |
| `scripts/ci/check_licenses.py` | 成功。module 13件 |
| `git diff --check` | 差分なし |

package別coverage: `internal/progress` 100.0%、`internal/security` 100.0%、`internal/domain` 95.8%、`internal/app` 94.9%、`internal/domain/port` 92.7%、`internal/domain/port/fake` 86.1%、`cmd/gdtvm` 66.7%。

P1-03で0.0%だった`internal/domain/port`は、本taskで`LogRecord.Validate`と`Ports.Missing`のtestを同packageへ置いたため92.7%になった。

test件数（subtest込み）: `internal/domain` 59、`internal/domain/port/fake` 49、`internal/app` 44、`internal/progress` 25、`internal/domain/port` 15、`internal/security` 11。

### 4.2 主なnegative test

| 対象 | 内容 |
|---|---|
| error code | 未知code、大文字小文字違い、前後空白、exit 12への写像が無いこと、exit 1〜11の範囲 |
| retryable | 禁止8件それぞれで`Retryable=true`がValidateで落ちること、他codeでは通ること |
| `Error()` | causeに入れたcredential付きURLが表示文字列へ現れないこと |
| parameter key | 大文字、hyphen、dot、数字始まり、underscore始まり、空白、非ASCII、長さ超過 |
| message ID | segment 1件、先頭/末尾dot、連続dot、大文字、数字始まりsegment、非ASCII、長さ超過 |
| ID | 31/33桁、hex外、大文字hex、UUID表記、空白 |
| progress | phase/unit範囲外、負のcurrent/total/rate、current>total、単調性違反 |
| result warning | §16.1のPlan warning code（`W_THIRD_PARTY`等）を受理しないこと |
| log record | UTC以外の時刻、level範囲外、field key違反、64件ちょうどと65件の境界 |
| mask | userinfo、未知のquery key、fragment、解析不能URL、`*_TOKEN`系key、元mapを書き換えないこと |

## 5. 未実施・制約

- `go tool govulncheck ./...` はこのcontainerから`vuln.go.dev`が`Forbidden`で到達できず未実行である（P0-02から継続）。検証手段はCIだけである。
- structured logのJSON Lines serializationとfile出力・rotationは実装していない。[04-storage-and-data.md](../04-storage-and-data.md)§18が形式を、[10-security.md](../10-security.md)§12がrotation/保持上限を定めるが、書き出し先packageを[02-architecture.md](../02-architecture.md)§2の18論理領域が持たず、担当taskも進捗台帳に無い。P1-04は`LogRecord`とport境界までとし、担当の確定を次のtaskへ送った。
- `ProgressSink`の「遅いconsumerでblockさせない」性質（§10）はadapter側の要件であり、本taskのfakeでは検査していない。TTY progress barを実装するP8-05で扱う。
- message catalogは作っていない。実装側で固定したIDは`error.internal`と`error.cancelled`の2件だけである。
- Windowsでの実行はCI matrixでのみ確認する。
