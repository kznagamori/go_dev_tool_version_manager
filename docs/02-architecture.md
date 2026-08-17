# アーキテクチャ・内部API仕様

## 1. 基本方針

実装はヘキサゴナル構成に準じ、ドメインとユースケースをOS、CLI、HTTP、TOMLライブラリから分離する。v0.1のUIはCLIだけである。

依存方向は次の一方向とする。

```text
CLI adapter
      ↓
Application Service（ユースケース）
      ├──→ Domain（値、規則、計画、結果）
      └──→ 抽象ポート ←── Infrastructure adapter
                         （FS、HTTP、Process、Windows/Linux、TOML）
```

DomainとApplication ServiceからCLI、具体的OS API、具体的HTTP clientを参照することを禁止する。抽象ポートはcore側が所有し、Infrastructureはそれを実装する。

## 2. Goモジュール内の論理構成

実装時の正確なディレクトリ名は次を標準とする。すべて同一Goモジュール内とし、`internal`境界を使う。

| 論理領域 | 責務 |
|---|---|
| `cmd/gdtvm` | 引数解析、表示形式、終了コード、サービス呼出し |
| `internal/app` | ユースケースの公開窓口、要求検証、トランザクション境界 |
| `internal/domain` | ToolID、Version、Platform、Plan、Selection、Receipt、Error |
| `internal/config` | 実行file隣接設定、project設定、許可された環境変数の読込みと統合 |
| `internal/definition` | ツール定義の解析、スキーマ検証、テンプレート評価 |
| `internal/registry` | 同梱registryの読込みとschema検証 |
| `internal/catalog` | 配布元照会、版正規化、channel/lifecycle判定、カタログcache |
| `internal/install` | ダウンロード、検証、安全展開、probe、receipt、transaction |
| `internal/selection` | user/project選択、currentリンク、優先順位 |
| `internal/runtime` | 実行環境生成、コマンド解決、子プロセス起動 |
| `internal/shim` | shim metadata生成、呼出名解決、実体委譲 |
| `internal/shell` | setup、profile marker、undo |
| `internal/store` | state、catalog、receipt、atomic write、structured logの出力とrotation |
| `internal/platform` | Windows/Linux固有のリンク、プロセス、権限、パス、HTTP client |
| `internal/security` | upstream SHA-256/SHA-512、内部SHA-256、path検査、mask |
| `internal/doctor` | 診断規則とreport生成 |
| `internal/progress` | 型付き進捗、warning、cancel境界 |
| `internal/message` | メッセージIDとカタログ |

CLIフレームワークやTOMLライブラリの型を`internal/app`の境界に露出させてはならない。

## 3. 主要ドメイン値

| 値 | 必須条件 |
|---|---|
| ToolID | 正規化済みkebab-case、aliasではない |
| Version | 空でない完全版。元文字列と比較用キーを保持 |
| Platform | OS、arch、必要時libc、実行形式suffix |
| Scope | `user` または `project` |
| Mode | `portable` または `user` |
| Channel | `stable` または `prerelease` |
| Lifecycle | `supported`, `eol`, `unknown` |
| Digest | algorithmと小文字hex値。gdtvm自身が計算する値はSHA-256固定、upstream由来の値は`sha256\|sha512` |
| InstallKey | ToolID＋Version＋Platform |
| EffectiveSelection | 選択値、由来、設定ファイル、導入状態 |

Versionの比較規則はtool definitionが指定する`semver|go|python`だけを組み込みで提供する。入力一致はcomparison keyではなく、catalogに保存された正規完全versionのbyte完全一致とする。

## 4. Service生成とport

概念上のconstructorは次とする。

```go
type Services struct {
    App     ApplicationService
    Runtime RuntimeResolver
}

func NewServices(build BuildInfo, ports Ports) (Services, error)
```

`Ports`は最低限、Filesystem、Link、Registry、HTTP、Archive、Process、Environment、UserLookup、Clock、Lock、Random、Loggerを持つ。digest計算はportにしない。外部作用を持たない純計算で、同じ入力が常に同じ結果を返すため差し替える意味がなく、§2が「upstream SHA-256/SHA-512、内部SHA-256」を`internal/security`の責務としている。production adapterとfakeを同じinterfaceへ注入する。progress/cancelはrequestごとに渡す。Prompt/terminalはadapter責務でありPortsへ入れない。package global mutable state、暗黙working directory、暗黙時刻/networkを使わない。

constructorは依存の存在とbuild metadata形式だけを検査し、filesystem/network変更を行わない。初期化は`Initialize`で行う。

### 4.1 抽象ポート

名称は概念名であり、Goの具体的な宣言そのものではない。

| ポート | 操作 |
|---|---|
| FileSystem | stat、read、atomic write、mkdir、rename、remove、walk、permission、realpath |
| LinkManager | junction/symlink/hardlink作成、リンク種別取得、安全な除去、能力検査 |
| Registry | Windows HKCU valueのraw/type読書き、再読、通知 |
| HTTPClient | GET、HEAD、redirect、proxy、TLS、response limit |
| ProcessRunner | argv実行、環境、cwd、stdio、signal、exit code、timeout |
| ArchiveExtractor | list、安全検査、選択展開、進捗、形式判定 |
| LockManager | process間共有/排他ロック、所有情報、timeout |
| Environment | 親環境取得、case規則、process block生成 |
| UserLookup | 実user/UID、Known Folder/OS account home、owner identity |
| Clock | 現在時刻、単調時間、待機 |
| Random | 128 bit ID生成 |
| ProgressSink | 型付きphase/current/total/rate通知 |
| Logger | 構造化level、operation ID、秘密値マスク |

テストでは全ポートをメモリまたは一時ディレクトリ実装へ差し替え可能にする。

## 5. 共通型

```go
type RequestContext struct {
    InvocationID   string
    ModeOverride   *Mode
    HomeOverride   *AbsolutePath
    Offline        bool
    NonInteractive bool
    Project        ProjectPolicy
    Cancel         CancelToken
    Progress       ProgressSink
}

type ResultMeta struct {
    InvocationID string
    Warnings     []ResultWarning
    StartedAt    time.Time
    FinishedAt   time.Time
}
```

RequestはCLI境界で型変換後immutableとして扱う。path、version、tool ID、duration、URL、digest、enumはdomain valueにparse済みで渡し、service内部でraw文字列を再解釈しない。Result/Errorはmessage IDとtyped parameterを返し、表示文はpresentation層で生成する。

## 6. 初期化

```go
Initialize(ctx RequestContext, req InitializeRequest) (InitializeResult, error)
```

有効distribution/data rootを決め、config、setup state、schema、registry、receipt indexをstrictに読んでimmutable snapshotを生成する。`ReadOnly=true`ではwriteを行わず、診断と安全に読めたsnapshotだけを返す。CLI adapterはconfig/stateを読まず、raw optionを渡す。

`Initialize`後のoperationは同じsnapshot、root identity、registry識別子を使う。別呼出しがstateを変更した場合はrevision検査でstaleを返す。

## 7. 読取りoperation

| method | request主要field | result主要field | CLI |
|---|---|---|---|
| `ListAvailable` | tool, refresh | catalog item[] | `available` |
| `ListInstalled` | optional tool | install summary[] | `installed` |
| `ResolveCurrent` | tool, project policy | effective selection/health/source | `current` |
| `Diagnose` | report flag | diagnostic[]/overall health | `doctor` |
| `GetBuildInfo` | short flag | client/build/registry info | `version` |

catalog cacheのatomic書込みはcatalog lockとcancel/progressを使うが、Plan/approval対象ではなく、tool payload/selection/storage/config/setupを変更しない。`version --short|--version`用build infoはregistry/config破損時もbinary metadataだけで返せる。

read resultのslice/mapは決定的な順序を仕様化し、internal map iteration順を露出しない。`Diagnose`だけは破損箇所を複数列挙するため、読める範囲を継続して最大件数で打ち切る。

## 8. Plan/Execute operation

変更operationは必ず2段階にする。

```go
PlanSetup(ctx, SetupRequest) (Plan, error)
ExecuteSetup(ctx, Plan, Approval) (SetupResult, error)
```

同じ形を`Install`, `Use`, `Uninstall`へ適用する。`Plan`は[04-storage-and-data.md](04-storage-and-data.md)の完全なtyped内容を持つ。Executeは次を順に検査する。

1. Plan schema/client/invocationの一致。
2. Approvalが必要な`PlanWarningCode`を含む。
3. `inputs`に固定したroot/config/registry/definition/catalog/receipt index/selection/setupのrevision/digest identity。
4. lock取得後に同じ検査を繰り返す。
5. Execute中のdownload/extract/probeがPlanの列挙と一致し、全書込みがdata root、distribution root、宣言済みintegration対象、project fileの中にあり、任意helper/backend processを起動しないこと。

Approvalは`InteractiveYes|AssumeYes`と承認categoryの集合を持つ一時値で、永続approval databaseを作らない。security failureを承認categoryにしない。

### 8.1 request/result

| operation | request主要field | result主要field |
|---|---|---|
| Setup | mode, integration, shell | root, changed entries, restart hint |
| Install | tool, exact versionまたはlatest, use scope | install identity, receipt, optional selection |
| Use | tool, managed exact, scope | old/new selection, health |
| Uninstall | tool exact, force, purge | removed receipt, retained/purged storage |

`InstallRequest`はexact versionとlatestをunionで排他にする。bool組合せで無効状態を表現しない。setup/setup-removeのPlanだけは`SetupPlan`を必須とし、他operationではnullにしてoperation固有fieldをtool summaryやwarning parameterへ埋め込まない。
resultがpathを返す場合は[04-storage-and-data.md](04-storage-and-data.md)§17.2の`PathValue`を使い、裸のstring pathを公開境界へ出さない。

## 9. Runtime Resolver

shimのhot pathは独立した最小APIを使う。

```go
ResolveInvocation(ctx RuntimeContext, req InvocationRequest) (InvocationPlan, error)
LaunchInvocation(ctx RuntimeContext, plan InvocationPlan) (ExitStatus, error)
```

`InvocationRequest`はargv0、module path、cwd、argv、parent environment、stdio/signal handleを持つ。`InvocationPlan`はselection source、managed identity、absolute target、fixed/user argv、environment map、cwdを持つ。

Resolverはnetwork、prompt、repair、definition再downloadを行わない。validated shim index、selection、receiptだけを使い、曖昧・破損時は短いtyped errorを返す。Launch直前にtarget identityとcontainmentを再確認する。

## 10. Progressとcancel

```go
type Progress struct {
    OperationID string
    Phase       ProgressPhase
    ToolID      string
    Version     string
    Current     int64
    Total       *int64
    Unit        ProgressUnit
    Rate        *float64
    MessageID   string
    Parameters  map[string]Scalar
}

type ProgressSink interface { Report(Progress) }
type CancelToken interface { Done() <-chan struct{} }
```

phaseは`resolve|plan|download|verify|extract|probe|commit|cleanup|rollback|complete`。unitは`none|bytes|items|steps`。Currentは単調非減少、TotalがあるときCurrent<=Total。message parameterはscalarだけでsecret/path raw contentを含めない。`MessageID`と`Parameters`のkeyは§14と同じく[04-storage-and-data.md](04-storage-and-data.md)§7のgrammarに従う。

ProgressSinkは遅いconsumerでoperationを無期限blockさせない。最新値coalesceまたは有界bufferをadapterで行う。progressをJSON Linesとしてstdoutへ公開しない。

cancel検出時は`E_CANCELLED`。commit境界では整合状態またはrollback完了まで待つ。process adapterはgraceful signal→組込み5秒猶予→所有するprocess tree終了を行い、無関係processをkillしない。

## 11. 処理計画と実行の分離

利用者状態を変更する`setup|install|use|uninstall`は、必ず`Resolve → Plan → Approve → Execute → Commit → Cleanup`の段階を通る。catalog refresh、log rotation、検証済みtmp/cache cleanupはPlan/Approveを持たず、専用lockとatomic write/deleteを使う。

1. **Resolve**: alias、platform、version、definition、artifact/storage/runtimeを確定する。
2. **Plan**: ダウンロード、検証、外部コマンド、ディスク変更、警告を列挙する。
3. **Approve**: 危険度と対話ポリシーに従って承認を得る。
4. **Execute**: staging領域だけを変更する。
5. **Commit**: 完成receiptを記録してから導入先へ原子的に公開する。
6. **Cleanup**: 一時物を除去し、保持ポリシーに従いdownload cacheを整理する。

CLIはPlanの重要要約を冒頭と確認直前に表示し、その間に全詳細を表示する。Planに含まれない外部processや管理root外書込みを実行してはならない。

## 12. 並行処理とロック

ロック順序を固定し、デッドロックを防ぐ。

1. state lock
2. catalog lock（ToolID順）
3. install lock（ToolID、version、platform順）
4. storage lock（ToolID、storage ID順）
5. setup lock
6. shim lock

同一InstallKeyの同時導入は後発が待機し、先発成功後に整合性検査だけを行う。同じtool/storageを変更する公式commandとgdtvm操作はstorage lockで直列化できる範囲だけ保護し、管理外processを強制停止しない。異なるInstallKeyの操作は別invocationとして並行実行できるが、1つのoperation内のdownloadは逐次とし（[15-deferred.md](15-deferred.md) D-26）、commitとshim更新は短い排他区間にする。

lock fileにはlock ID、role、PID、取得時刻、operation IDを[04-storage-and-data.md](04-storage-and-data.md)の形式で記録する。排他性の正本はOS lock/handleであり、PID不在やfile ageだけで即時破棄しない。active OS lockを強制解除しない。cancel/timeoutでも取得済みlockを必ず解放する。

Service instanceは複数read operationを並行利用できる。request/resultを呼出後に変更しない。cacheはimmutable valueまたはlock保護し、package global singletonを置かない。

## 13. 失敗と回復

- 外部処理失敗はsecret maskとsize上限を適用した標準エラー末尾、終了コード、step IDを型付きエラーへ含める。
- staging失敗は完成版へ影響させない。中断した操作は`tmp/operations/<operation-id>/`をdirectory単位で削除すれば復旧する。
- commit後に選択リンク更新が失敗した場合、導入成功・選択失敗として区別し、`use`再実行で解消できるようにする。
- state file破損時は検証済みbackupを優先し、index/cacheだけはreceiptと実directoryから再構築する。selection等の正本を推測再生成しない。
- cleanup失敗は主操作成功を覆さず`W_CLEANUP_INCOMPLETE`とdoctor項目にする。
- キャンセルは子プロセスを終了し、安全な一時物だけを除去する。

## 14. Error model

```go
type Error struct {
    Code       ErrorCode
    MessageID  string
    Parameters map[string]Scalar
    Operation  string
    ToolID     string
    Version    string
    PathRole   string
    Retryable  bool
    Cause      error
}
```

`PathRole`は[04-storage-and-data.md](04-storage-and-data.md)§17.2の閉じた集合から取り、絶対pathを公開境界へ出さずに対象を特定できるようにする。`Cause`はdebug log用でJSON/public messageへ直接serializeしない。`Code`は`E_`で始まるstable codeとし、[03-cli.md](03-cli.md)の終了code表および[09-platform.md](09-platform.md)のplatform error表の和集合をv0.1の閉じた集合とする。各stable codeは終了code exactly 1件へmapし、未分類codeを公開境界へ返さない。想定外の内部失敗だけは公開code `E_INTERNAL`、終了code 1へ変換する。

同じ失敗条件はCLI human/JSON/shimで同じcode/message IDにする。retryableは自動retry対象を意味せず、利用者が状態変更後に再実行可能かを示す。checksum/path/identity/registry corruptionをretryable=trueにしない。該当codeは次のexactly 8件とし、実装は`Retryable=true`との組合せを拒否する。

```text
E_CHECKSUM_MISMATCH  E_ARCHIVE_UNSAFE   E_PATH_UNSAFE     E_PATH_CONFLICT
E_REGISTRY_INVALID   E_DEFINITION_INVALID  E_STATE_CORRUPT  E_RECEIPT_INVALID
```

`MessageID`は[04-storage-and-data.md](04-storage-and-data.md)§7のmessage ID grammar、`Parameters`のkeyは同§7のscalar parameter key grammarに従う。`Parameters`の値はstring、boolean、integer、nullだけとする。

## 15. Logger

Loggerはlevel、timestamp、invocation/operation ID、component、message ID、typed fieldを受ける。URL credential/query secret、proxy credential、environment secret、authorization header、registry raw PATH、file contentを渡す前に呼出側とsinkの両方でmaskする。

専用audit logは持たない。install/use/uninstallの事実は通常structured log、receipt、resultから追跡する。

## 16. CLI薄層の受入条件

CLI packageに次があれば違反とする。

- TOML/JSON/state/receipt/registryの直接read/write
- path/root/current junction/symlinkの決定・変更
- HTTP、checksum、archive、process実行
- tool/version/provider/storage/security policy判断
- project探索やselection優先順位
- retry、rollback、lock

CLI packageに許すのはcommand table、help、raw入力からdomain型へのparse、service呼出し、Plan表示/prompt、Result/Error/progress表示、exit code変換だけ。fake Serviceを用いた全command mapping testと、human/JSONが同じtyped resultを表すcontract testを必須とする。

## 17. 外部依存方針

Go標準ライブラリを優先する。外部モジュールは、CLI解析、TOML、Windows API、process間lockなど、保守上の利益が明確なものに限定する。archive形式は`zip`と`tar.gz`だけを扱うため、圧縮形式のための外部moduleを追加しない。

採用時は次を記録する。

- SPDXライセンスと再配布条件
- 最終更新、既知脆弱性、maintainer状況
- transitive dependency一覧
- コアの抽象ポートで置換可能であること
- `go.sum`固定と定期更新方針

CLIフレームワークのコマンドオブジェクトからドメイン処理を直接呼び分けず、要求値へ変換してApplication Serviceを1回呼び出す。

## 18. 互換性

内部Go APIは同一module内で変更できるが、observable CLI、JSON、state、receipt、Plan、message/error codeを変える場合は該当仕様・fixture・testを同じ変更で更新する。延期機能を追加するときは[15-deferred.md](15-deferred.md)の再導入gateを先に完了する。
