# 内部API仕様

## 1. 適用範囲

内部APIは同一Goモジュール内のCLIと将来のWails v3 GUIが利用する。第三者互換を約束する公開APIではないが、UIとコアの分離契約として安定させる。

APIは翻訳済み文章、ANSI、CLI option、Wails型を扱わない。すべての要求にcontext、すべての状態変更操作にoperation IDを使用する。

## 2. Application Service生成

Service生成時に次を明示注入する。

- Paths/Mode
- FileSystem、LinkManager、LockManager
- HTTPClient、ProcessRunner、ArchiveExtractor
- Config/State/Registry stores
- SignatureVerifier、HashCalculator
- Clock、ID generator
- EventSink、Prompt/Approval provider
- Logger、Locale-neutral message catalog IDs

暗黙のpackage global mutable stateを禁止する。shim用の最小Serviceはnetwork、Prompt、installerを含めず、read-only resolver/runtimeだけを組み立てる。

## 3. 共通要求

すべてのoperation requestは概念上次を持つ。

| field | 内容 |
|---|---|
| operation_id | 呼出側指定またはService生成 |
| home_override/mode | bootstrapで解決済みでも明示可能 |
| working_directory | project探索基点。絶対pathへ正規化 |
| project_file | 任意の明示file |
| ignore_project | bool |
| offline | bool |
| non_interactive | bool |
| assume_yes | bool |
| language | UI側利用。ドメイン判断には不使用 |

`home_override`, `working_directory`, `project_file`は存在しない末尾を除きabsolute/canonical化したpath valueで受ける。空文字を「未指定」として使わずoptional fieldで表す。`mode`は`portable|user`、languageは`auto|ja|en`、booleanは三値にせず必ずtrue/falseとする。`project_file`と`ignore_project`、home_overrideとmode=userはService境界でも排他検査する。CLIで検査済みという前提を置かない。

## 4. 読取り操作

### 4.1 Initialize

**入力**: 共通要求、create_if_missing。

**出力**: mode、全論理path、初回判定、global config、active registry、必要setup action、warning。

create_if_missing=falseなら一切書かない。shimとdoctorの読取り経路で使用する。

### 4.2 ListTools

**入力**: installed_only、supported_only、origin filter。

**出力要素**: tool ID、name、aliases、description message ID、homepage、license、origin、platform support/reason、installed count、user/project/effective selection。

### 4.3 ListAvailable

**入力**: tool ID、channel、refresh policy。

**出力要素**: exact version、channel、published time、platform availability、artifact kind、verification capability、catalog fetched/expiry、metadata。

refresh policyは `never`, `if-missing`, `if-stale`, `force`。

### 4.4 ListInstalled

**入力**: 任意tool ID、deep_verify。

**出力要素**: install key、receipt概要、health、size、dependencies、selection flags、payload path、repair hint。

### 4.5 ResolveCurrent

**入力**: 任意tool ID、explain。

**出力**: toolごとのEffectiveSelection。explain時は探索candidate、採用/不採用理由、project boundary、receipt/link状態を含む。

### 4.6 Diagnose

**入力**: 任意tool ID、deep、network_allowed。

**出力**: Diagnostic配列。各要素はID、severity、scope、summary message ID、structured args、evidence、repairable、repair action ID。

### 4.7 GetBuildInfo

**入力**: なし。

**出力**: client calendar version `YYYY.mm.DD.XX`、commit、build time、Go version、OS/arch、state schema、tool definition schema、対応registry schema、active registry version nullable。

build時埋込み値とruntime stateをServiceで型検査して返す。CLIの`--version`/`version`と将来GUIのAbout画面はこの結果を使い、CLI packageがstate fileを直接読まない。

## 5. 計画・変更操作

状態変更操作は、UIが事前表示できるよう `PlanX` と `ExecutePlan` の二段APIを内部的に提供する。CLIの簡易methodは両者を順に呼ぶだけでよい。

Plan共通field:

| field | 内容 |
|---|---|
| plan_id | 一意、短いTTL |
| operation_id | 関連ID |
| created_at/expires_at | 時刻 |
| input_fingerprint | config/definition/catalog/state revision hash |
| actions | 順序/DAG、kind、対象、download、process、write |
| warnings | severityと承認要否 |
| approvals | approval ID、policy、表示data |
| estimated_download/disk | 不明値を表現可能 |

Execute時にfingerprintが変わっていれば古いplanを実行せず `E_PLAN_STALE`。

plan TTLのbuilt-in既定は10分、上限1時間とする。Planはprocess memory内のimmutable objectとし、秘密値を含めず、GUI表示用に安全なDTOへ変換できる。plan IDだけを再起動後に永続実行する機能はschema 1で提供しない。ExecutePlanはplan ID、承認回答集合、呼出contextを受け、同じplanの二重実行を拒否する。完了後の同じrequestは保存済みoperation resultを返してもよいが、変更操作を再実行してはならない。

承認回答はplanのapproval IDすべてに対し`allow|deny`を明示する。欠落はdeny、未知IDはrequest error。plan作成後にUIが任意のactionやargvを差し替えることはできず、変更したい場合は新しいPlanを作る。

### 5.1 Setup

**要求**: mode、shell一覧、remove、skip_shell、execution policy提案。

**結果**: 作成/更新/skipしたpath・registry key、backup IDs、registry bootstrap結果、再起動が必要なshell。

### 5.2 RefreshCatalog

**要求**: 任意tool、force。

**結果**: tool別status、versions count、added/removed count、ETag、fetched time、aggregate partial flag。

### 5.3 Install

**要求**: tool ID、exact versionまたはlatest flag、select_after、scope。

**結果**: resolved exact install key、receipt、dependency results、verification、selection result、cleanup warnings。

`version` とlatestは排他。Application Service自身が完全指定制約を再検査し、CLIだけに頼らない。

schema 1ではvariantを利用者入力にしない。Resolverが現在のOS、arch、libcに一致する`platforms`候補からpriority最大の一意なvariantを選び、結果のInstallKeyとreceiptへ記録する。同順位候補は定義エラーであり、暗黙選択しない。definition更新後も既存installはreceiptのvariantで識別し、新規installだけが更新後の決定規則を使う。

### 5.4 Uninstall

**要求**: tool ID＋exact version＋current platform、または内部画面用のexact install key。force、keep_shared。

**結果**: removed receipts、selection changes、retained shared/cache、cleanup warnings。CLI形式では同じtool/version/current OS/archの全variantを解決してplanへ含め、別platformは含めない。

### 5.5 Use

**要求**: tool ID、exact version、scope、auto_install policy。

**結果**: previous/effective selection、project path、link mode、auto install result、reload hint。

Useは未導入時にInstall planを子planとして含める。UI確認は一つの統合planで行う。

### 5.6 Disable

**要求**: tool ID一覧またはall、scope。

**結果**: previous selections、new effective fallback/disabled、project path、link changes。

### 5.7 UpdateRegistry

**要求**: 任意exact registry version、force。

**結果**: previous/new version、tag/commit、key ID、manifest digest、compatibility、changed tool IDs、retained snapshots。

### 5.8 Repair

**要求**: 任意tool、dry_run。

**結果**: diagnostics before/after、actions、skipped unsafe actions、remaining issues。

### 5.9 Exec

**要求**: explicit install keys配列、command argv、cwd、stdio policy、追加env（内部信頼呼出だけ）、timeout任意。

**結果**: resolved tools、target executable、exit code、signal、duration。stdio inherit時はoutput bytesを結果へ含めない。

外部CLIから任意env追加を受けるoptionは初期版に設けない。Wailsもsecretをコアログへ渡さない。

### 5.10 BuildCompletion

**要求**: shell。

**結果**: mime/encoding、completion text、cache dependency。これは状態を変更しない。

## 6. Runtime Resolver API

shimが利用する最小API:

### ResolveInvocation

**入力**: invoked basename、cwd、argv、parent environment概要。

**出力**:

- owning tool ID
- effective selectionとorigin
- receipt/definition fingerprints
- absolute target
- fixed argsと利用者args
- child environment map
- working directory
- codepage
- signal/stdin/out policy

このAPIはnetwork、install、prompt、state writeを禁止する。shim indexがstaleなら、active definition、導入receipt、selection stateからメモリ再構成を試し、永続修復は `gdtvm repair` に委ねる。呼出commandに有効な所有toolが複数あれば候補を返さず `E_COMMAND_AMBIGUOUS` とする。

## 7. Event API

Event共通field:

| field | 内容 |
|---|---|
| schema | event schema=1 |
| id | event ID |
| operation_id | operation |
| parent_operation_id | dependency operation等 |
| timestamp | UTC |
| sequence | operation内単調増加 |
| type | 下表 |
| message_id | UI翻訳key |
| args | 非秘密structured values |
| data | type固有payload |

必須event type:

- `operation-started`, `operation-completed`, `operation-failed`
- `plan-created`, `approval-required`, `approval-resolved`
- `download-started`, `download-progress`, `download-completed`
- `verification-started`, `verification-completed`
- `step-started`, `step-output`, `step-completed`
- `install-committed`, `selection-changed`, `registry-changed`
- `warning`, `diagnostic`, `cleanup-warning`

progressはbytes current/totalまたはitems current/totalを持ち、total unknownをnullで表す。EventSinkが遅い場合、progress中間eventはcoalesceできるが、開始・終了・warning・approvalを落としてはならない。

### 7.1 GUI operation lifecycle

Wails bridgeはコアServiceの薄いadapterとして次の概念操作だけを公開する。名称はfrontend bindingで変更できるが意味を変えない。

| 操作 | 入力 | 結果 |
|---|---|---|
| Plan | operation種別と型付きrequest | Plan DTO |
| Start | plan ID、approval回答 | operation IDを即時返す |
| GetOperation | operation ID | queued/running/succeeded/failed/cancelled、開始/終了時刻、最終result/error |
| Cancel | operation ID | cancellation accepted bool。完了済みならfalse |
| Subscribe | operation IDまたはall | Event stream。sequence付き |

Start後の実処理はGUI event loopをblockしないworker contextで行う。Cancelは同じcontextへ伝播し、download、ProcessRunner、lock待機が有限時間内に応答する。commitの原子的置換を開始した後は途中で状態fileの半書込みを作らず、最小commit区間を完了してからcancelled-with-commit resultを返すことがある。その場合はresultに`committed=true`とreload/repair hintを持たせる。

Event再接続のため、operation中の非progress eventと最新progressを少なくともoperation完了後10分またはclient process終了までmemory ring bufferへ保持する。Subscribeは`after_sequence`を受け、保持範囲より古ければgap eventとGetOperation再取得を返す。Wails固有event emitter、JavaScript型、window handleを`internal/app`以下へ渡さない。

GUIを閉じる際、実行中operationごとにcancelを要求し、設定されたgrace period内に終了しなければjournalをflushしてprocess終了判断をUIへ返す。GUIが消えただけでworkerをdetached background daemonにしない。CLIはStart/Subscribeを必須経路にせず、同じExecutePlanを同期呼出ししてeventを端末へ流せる。

## 8. Approval API

ApprovalRequest:

- approval ID
- kind: normal, third-party, unverified, local-definition, shell-hook, profile-change, destructive
- title/message IDとargs
- evidence: URL、license、hash、commands、paths
- default answer（常にdeny）
- can_assume_yes
- expires_at

非対話providerの判断:

| kind | `--yes`なし | `--yes`あり |
|---|---|---|
| normal | deny | allow |
| third-party | deny | signed standard definitionか承認済みlocal definitionで、SHA-256以上を検証できる場合だけallow。警告/audit必須 |
| shell-hook | deny | policyが許せばallow |
| local-definition | deny | policy=allowだけallow |
| unverified | deny | deny |
| profile-change | deny | 通常marker追加のみallow |
| destructive | deny | forceも要求される場合だけallow |

一つのartifactが`third-party`かつ`unverified`なら、より厳しい`unverified`判定を適用して非対話では拒否する。approval kindを分割して片方だけ承認済みにしてはならない。

## 9. エラーmodel

CoreErrorは次を持つ。

- stable code (`E_*`)
- category
- message IDとargs
- operation ID
- tool/install/path/step context
- cause chain（内部用）
- retryable bool
- remediation message IDs
- safe details

最低限のstable code:

`E_USAGE`, `E_CONFIG_PARSE`, `E_CONFIG_SCHEMA`, `E_HOME_NOT_WRITABLE`, `E_TOOL_UNKNOWN`, `E_COMMAND_AMBIGUOUS`, `E_PLATFORM_UNSUPPORTED`, `E_VERSION_INVALID`, `E_VERSION_NOT_FOUND`, `E_VERSION_NOT_INSTALLED`, `E_CATALOG_MISSING`, `E_OFFLINE`, `E_NETWORK`, `E_DIGEST_MISMATCH`, `E_SIGNATURE_INVALID`, `E_POLICY_DENIED`, `E_DEFINITION_UNAPPROVED`, `E_ARCHIVE_UNSAFE`, `E_DEPENDENCY_CYCLE`, `E_DEPENDENCY_CONFLICT`, `E_PROCESS_FAILED`, `E_TIMEOUT`, `E_LOCKED`, `E_PLAN_STALE`, `E_STATE_CORRUPT`, `E_LINK_FAILED`, `E_CANCELLED`, `E_PARTIAL`。

CLI終了コードへのmappingは [04-cli.md](04-cli.md) に従う。GUIはcodeを分岐に使い、翻訳文をparseしない。

## 10. versioning

内部API型にschema整数を持つのは永続化またはWails bridgeを越える値だけとする。CLIとGUIを同じreleaseで配布するため、クライアント版とは別の公開API versionを設けない。ただしWails bridgeの破壊変更ではfrontend生成物とcontract testを同時更新する。

## 11. thread safety

- Application Serviceは複数goroutineから呼出し可能。
- request/resultは呼出後immutableとして扱う。
- StoreとLockManagerがprocess間整合性を担当する。
- Event sequenceはoperationごとに直列化する。
- Prompt providerは同時approvalをqueueまたはUI上区別する。
- ProcessRunnerとdownloadはcontext cancelに必ず応答する。

## 12. CLI薄層の判定

### 12.1 CLIとApplication Serviceの対応

CLIは次の対応で型付きrequestを1回構築する。複数の状態変更を伴う操作はApplication Service側が統合Planまたは子Planとして構成し、CLIが個別serviceを任意順に連結してはならない。

| CLI | Application Service | CLI固有処理 |
|---|---|---|
| `setup`, `setup --remove` | `PlanSetup` → `ExecutePlan` | shell名とflagをSetup requestへ変換 |
| `tools` | `ListTools` | filter flagと表示 |
| `available` | `ListAvailable` | `--refresh`をrefresh policyへ変換 |
| `refresh` | `PlanRefreshCatalog` → `ExecutePlan` | 任意toolと`--force`を変換 |
| `install` | `PlanInstall` → `ExecutePlan` | `tool@version`または`--latest`を排他的に解析 |
| `uninstall` | `PlanUninstall` → `ExecutePlan` | exact versionとflagを変換 |
| `installed` | `ListInstalled` | 任意tool filterと表示 |
| `use` | `PlanUse` → `ExecutePlan` | scopeをuser/projectへ変換。未導入時のInstall連鎖はServiceが構成 |
| `disable` | `PlanDisable` → `ExecutePlan` | tool一覧/allとscopeを変換 |
| `current` | `ResolveCurrent` | 任意toolとexplainを変換 |
| `registry update` | `PlanUpdateRegistry` → `ExecutePlan` | 任意registry完全版と`--force`を変換 |
| `doctor` | `Diagnose` | tool、deep、offlineからnetwork_allowedを決定 |
| `repair` | `PlanRepair` → `ExecutePlan` | toolとdry-runを変換 |
| `exec` | `PlanExec` → `ExecutePlan` | `--`境界までをinstall key入力、以降を未加工argvとして変換 |
| `completion` | `BuildCompletion` | shell enum検査とtext出力 |
| `version`, global `--version` | `GetBuildInfo` | 通常/short表示だけを選択 |

`PlanExec` は状態を変更しない場合でも、依存・競合・target・環境・外部processを実行前に固定するためPlanを作る。CLIは戻されたtargetやenvironmentを組み直さず、そのPlanを実行する。`doctor --deep`のnetwork可否は共通要求のofflineと矛盾してはならず、offlineなら常にfalseとする。

### 12.2 CLIへ置ける責務

CLI内に置いてよい処理:

- flag/argument構文解析
- requestへの型変換
- Event/Resultのtable、text、JSON表示
- Prompt回答の取得
- CoreError→終了コードmapping

CLI内に置いてはならない処理:

- tool/version解決
- file path決定
- TOML/state直接更新
- HTTP/download/extract
- link、process、environment生成
- security承認policy
- install/useの連鎖判断
