# 導入・選択・実行仕様

## 1. 全体状態機械

導入operationは次の状態だけを取る。

```text
created → resolved → planned → approved → downloading → verifying
       → staging → validating → committing → committed → cleaning → succeeded
                                         ↘ failed
任意の実行状態 → cancelling → cancelled
```

`committed` より前は完成versionとして列挙しない。`committed` 後のcleanup失敗は `succeeded_with_warning` 相当の結果とし、payloadを巻き戻さない。

## 2. version解決

### 2.1 exact指定

1. tool ID/aliasを正規化する。
2. 現platformのdefinitionを選ぶ。
3. 検証済みcatalogを読む。期限切れでもexact entryのdigest/source metadataが揃えば利用可能だが、オンライン時はrefreshを提案する。
4. 入力文字列をtrimせず、catalogの正規versionとbyte単位の完全一致で探す。
5. 0件はnot found、2件以上はregistry/catalog corruption。

版比較上等価でも文字列が異なるものを同一入力として受けない。例として `22.18` は `22.18.0` に補完しない。

### 2.2 `--latest`

1. stable channelだけを対象にする。
2. 現platformで実際にartifactを選択できる版だけを残す。
3. version schemeの降順先頭を選ぶ。
4. 完全版、artifact kind、source、検証状態をplanへ固定する。
5. 確認後にcatalogが更新されてもoperation中は変更しない。

## 3. 依存計画

tool versionをnodeとする有向グラフを作り、managed-tool/helper/systemを解決する。

- managed-tool/helperは完全版とdigestまでplanへ固定する。
- 既に正常導入済みならverify actionだけにする。
- topological順で導入し、同階層のdownloadは並列可。
- version conflictは自動で片方を選ばず、両要求元を示して失敗する。
- optional dependencyは既定でskipできるが、機能差をplanに表示する。
- system-command/libraryはprobeし、不足時にOS別install hintを示す。gdtvmがpackage managerやUAC/sudoを起動してはならない。

## 4. plan表示

最低限、次を実行前に確定する。

- tool、完全version、platform、variant
- definition origin/version/hash
- artifactのofficial/third-party、URL host、license、予想size
- Resolve段階で値まで確定したchecksum/signatureと検証方法
- managed/helper/system依存
- 外部process/hookの実行ファイル、引数の秘密値除外版、cwd、書込みroot
- 最終payload、shared、cacheの論理path
- 選択変更の有無
- warningと必要な確認

通常のofficial＋SHA-256＋組込みstepだけのexact installは簡潔表示し、対話確認を省略してもよい。`--latest`、third-party、unverified、hook、system不足は確認必須。

catalogのchecksumが`pending`ならPlan生成前にchecksum sourceだけを取得・検証してcatalog/cacheへ確定し、その結果をfingerprintへ含める。このmetadata取得はartifact本体downloadではないがnetwork eventとして表示する。Plan表示後またはapproval後に別digestへ再解決しない。offlineで値を確定できなければplanを作らない。

## 5. download

- HTTPSのみを既定許可し、TLS検証を無効化するoptionを提供しない。
- redirectごとにschemeとhost policyを再検査する。
- response status、Content-Length上限、Content-Typeは妥当性検査する。
- `.part`へstreamしながらSHA-256を計算する。
- Range再開はETag/Last-Modified/expected sizeが前回と一致する場合だけ行う。
- serverがRangeを無視したら0から安全に再開する。
- `Content-Disposition` のfile名をpathとして使わずdefinitionのfile名を使う。
- retryはtimeout、connection reset、429、5xxに限定し、`Retry-After`を上限内で尊重する。
- 404、digest mismatch、signature mismatchは自動retryで無限に繰り返さない。

## 6. artifact検証

SHA-256以上のdigest一致を検証済みの最低条件とする。upstream signatureがあれば追加検証する。

checksumが取得不能またはdefinitionでnoneの場合:

- source、license、完全版、platform、最終URL、未検証理由を表示する。
- 対話時だけ明示確認を許す。
- `--yes` を指定しても警告・auditを省略しない。
- 非対話時は拒否する。
- receiptのverificationを `unverified-approved` とし、doctor warningを継続する。

digest mismatchのfileは通常cacheとして再利用せず隔離名へ移すか削除し、expected/actual digestを記録する。

## 7. 安全な展開

archiveを展開前に全entryを列挙し、次を拒否する。

- absolute path、drive prefix、UNC
- `..` でdestination外へ出るpath
- NUL/control character
- Windows予約名、case-fold衝突、末尾dot/space衝突
- 同一path重複、file/directory種別衝突
- destination外を指すsymlink/hardlink/reparse point
- device、FIFO、socket等の特殊file
- definition上限を超えるentry数、単一file size、総展開size、圧縮比

symlinkを必要とする公式Linux SDKは、targetが展開root内の相対pathである場合だけ許可する。Windowsではarchive内symlinkを既定拒否する。

`strip_components` 適用後に空になるentryはskipする。include/excludeはnormalized POSIX pathへ適用する。

## 8. stagingとcommit

1. `tmp/operations/<id>/staging` を同じvolumeに作る。
2. download/verify後、definition stepをstagingへ適用する。
3. permissionを正規化する。Linux公開実体にはowner executeを保持/付与し、setuid/setgid bitsを除去する。
4. validation probeを隔離環境で実行する。
5. receiptをstaging payloadの親へ書く。
6. version完成先がないことを再確認する。
7. staging version directoryを完成先へatomic renameする。
8. state indexを更新する。
9. `--use` またはuse連鎖ならselectionを別commitとして行う。

完成先が同時に作られた場合は両receiptを比較し、同一なら後発stagingを破棄して成功、異なれば競合エラー。

### 8.1 backend方式

`selection_strategy="backend"` は外部managerがshared storeを管理するため、上記のpayload rename transactionをそのまま適用しない。初期rustup backendはtool全体のbackend lockを取得し、次を行う。

1. 実行前のbackend inventoryとselectorをjournalへ記録する。
2. 検証済みinstaller/backend commandで完全selectorをshared storeへ導入する。
3. required probeで完全version/hostを照合する。
4. version directoryにはpayloadを複製せずbackend receiptだけをatomic commitする。
5. 失敗時、新規作成したselectorだけをbackendから除去する。操作前から存在したselector/shared fileは削除しない。

backend commandがshared storeを部分変更してrollbackにも失敗した場合、そのselectorをunhealthyとしてdoctorへ残し、receiptを導入済みとしてcommitしない。

## 9. helperと外部program

7-Zip、WiX等はhelper definitionによりdownload、digest検証、管理cache内展開を行う。OSに偶然存在する同名commandを優先してはならない。system `msiexec.exe` 等、OS componentとして定義されたものだけを明示probeして使う。

外部実体の分類と取得規則は次に固定する。

| 分類 | 例 | 取得・実行規則 |
|---|---|---|
| managed helper | 7-Zip、WiX `dark.exe` | 署名registryのhelper定義からdownload、SHA-256検証、helper receiptの絶対entrypointだけを実行 |
| supplemental tool artifact | Windows `nuget.exe` | toolの補助artifact roleとして主payloadと同じplanで取得・検証し、tool receiptへ含める |
| backend bootstrap/manager | `rustup-init`, managed `rustup` | bootstrap artifactを検証しmanaged shared homeだけへ導入。以後はmanaged絶対pathを使用 |
| OS component | `msiexec.exe`, `cmd.exe`, Windows PowerShell | downloadしない。OS API/既知system directoryから解決してprobeし、欠落時は失敗 |
| system prerequisite/interpreter | `sh`, `bash`, `pwsh`, shared library | 自動導入せず絶対path/版をprobeし、install hintを出す |
| client内蔵機能 | shim、旧symexe相当 | gdtvm本体または同release内蔵fallback。registry/tool downloadとして扱わない |

managed helper、supplemental artifact、backend bootstrapを初めて取得するinstall planでは、tool本体とは別に名称、version、source URL、license、digest、download size、実行理由を表示する。third-party/unverified/hookでなくても外部programを実行する事実とargv要約をinstall開始前に表示する。`--yes`は通常確認を省略できるが、警告・監査eventは省略しない。Planに現れなかったexternal executableをExecute中に発見して起動してはならず、新Planを要求する。

外部processは必ず:

- executableとargvを分離
- shellなしを既定
- cwdを許可rootへ固定
- 最小限の環境map
- stdin policy（閉じる/継承）を明示
- timeoutとprocess tree終了
- stdout/stderr size上限と末尾保持
- planとjournalのstep ID

を持つ。

## 10. 選択transaction

### 10.1 user

1. install receiptとprobe状態を検査する。
2. 選んだreceiptのversion、variant、install IDを含む新 `selections.toml` を一時生成する。
3. link方式なら新current linkをtemporary nameで作りtarget検査する。backend方式なら完全backend selectorを検査する。
4. stateをcommitする。
5. link方式ならcurrent linkを置換する。backend方式はlinkを作らない。
6. shim index/runtime cacheとshell環境snapshotを更新する。

4と5の間に失敗した場合はstateを正とし、shim direct-targetで動作を継続し `repair` warningを出す。古currentが残ってもshimはstate/receiptを優先する。Windowsのjunctionはtemporary作成・旧link除去・renameの間に短い欠落区間があり得るため、置換自体を完全にatomicとは表現しない。

link方式でcurrent linkが利用できる場合、shell環境snapshotが保持するtool固有pathとhome変数はversion payloadの絶対pathではなく安定した`current` pathを使う。したがって、既に起動中のshellでもlink切替後は同じ環境変数値のまま新versionを参照できる。shim-only fallbackでは選択payloadの絶対path、backend方式ではshared rootとbackend selectorをsnapshotへ書くため、選択変更後に既存shell/VS Codeの再起動が必要である。snapshotは新しいshell起動用であり、実行中の親processを書き換えようとしない。

### 10.2 project

- 対象 `.gdtvm.toml` を決め、コメントと未知でない順序を可能な限り保持するTOML editorを用いる。
- `[tools].<id>` を完全版へ設定し、disabledから除く。
- fileがなければ `schema=1` と `[tools]` だけの最小fileを作る。
- global selection/current linkは変更しない。
- 同時編集は元file digest比較で検出し、上書きしない。

## 11. shim構成

### 11.1 目的

shimは旧 `symexe` の次の能力を一般化する。

- shim自身の呼出名から実体を選ぶ。
- 実体directory/PATHを設定する。
- 固定引数を前置する。
- Windows code pageを指定できる。
- 実体へ引数と終了コードを透過する。

さらにproject探索、user fallback、環境profile、複数tool、receipt検証を行う。

### 11.2 配置

`gdtvm[.exe]` はmulti-call binaryとし、起動basenameが`gdtvm`のときだけCLI、shim indexの公開command名なら最小runtime resolverへ直接分岐する。CLI frameworkやnetwork初期化より前に分岐する。配布物はgdtvm一つだが、hardlinkを使えないWindows向けの同release最小fallback shim bytesをgdtvm内へ署名対象resourceとして内蔵できる。

- portableかつ同一volumeではrootのgdtvm本体へのhardlink（Linuxではsymlinkも可）。
- Windows user modeの別volume等では、内蔵した最小fallback shimを `shims/gdtvm-shim.exe` へ1回だけ展開し、埋込みSHA-256とclient versionを検証する。このshimは同じread-only resolverを使用し、registryやnetworkから取得しない。
- Linuxは別volumeでもfile symlinkを使えるため、gdtvm本体のcopyをfallbackにしない。
- unknown basenameはCLIとして解釈せず、shim indexにない呼出名として短いerrorを返す。

公開command名ごとに次を作る。

- Windows NTFS: 実体へのhardlink `<command>.exe` を優先。
- Windows hardlink不可: 設定許可時だけ小型shim executableをコピー。ツール本体は絶対にコピーしない。
- Linux: symlinkまたはhardlink。argv[0]のbasenameを保持できる方式を選ぶ。

hardlinkではfileの実体pathからcommand名を推測できないため、shimはOSが渡した起動時のargv[0]を正とし、そのbasenameを使う。argv[0]が空、path separatorだけ、またはindexにない場合は実行しない。実行fileのcanonical pathはhome/client整合性検査にだけ使う。

shim基底はgdtvm client binaryのhardlink/symlinkまたはclient内蔵fallback shimであり、registryからdownloadしない。client更新後にshim基底のversion/digestが違えばdoctor warningとし、`setup`/`repair`が基底を更新してcommand hardlinkを張り直す。既存hardlinkが古いfile recordを保持しうるため、client fileの置換だけで更新済みとみなさない。

### 11.3 metadata

`state/shim-index.toml` はcommandから1件以上の候補tool/runtime commandへのindexだけを持つ。version targetを固定しない。起動時に各候補の有効selectionとreceiptから所有tool/targetを得る。indexはregistry snapshot hash、local definition hash、schema revisionを持ち、不一致時はactive definitionと導入receiptから安全に再生成する。

### 11.4 起動アルゴリズム

1. 呼出basenameから`.exe`を除きcommand indexの候補を検索する。
2. home locatorを決定する。
3. project fileを上向き探索する。
4. 候補toolごとに有効selectionを解決し、disabled/未選択を除く。残りが0件なら未選択エラー、2件以上なら候補toolを列挙した`E_COMMAND_AMBIGUOUS`とし、priorityや列挙順で選ばない。1件だけなら所有toolとする。
5. 所有toolのreceiptを読み、link方式はtargetがpayload内、backend方式はtargetがtool shared内に存在することを検査する。
6. 選択toolと定義されたmanaged dependencyのruntime environmentをtopological順にmergeし、宣言済みtool conflictも検査する。
7. command固定引数＋利用者引数を作り、launcher種別に従ってnativeまたは固定interpreterのargvへ変換する。
8. Windows codepageを必要時変更し、子終了後にshim process内で復元する。
9. 子をstdio継承で起動し、signal/console controlを転送する。
10. 子の終了コードを返す。

shimはregistry更新、download、対話を行わない。未導入、定義不整合時は短いerrorと `gdtvm use/install/repair` の案内をstderrへ出す。

### 11.5 再帰防止

PATH検索でshim自身をtargetにしない。targetはreceiptのpayload相対pathまたはbackend shared相対pathから解決し、一般PATH検索を使わない。子環境にoperation markerを置き、同commandへの意図しないshim loopを最大depth 8で拒否する。

## 12. 環境merge

親環境をcopyし、次の順で適用する。

1. managed dependency（DAG順）
2. 対象tool default profile
3. command個別profile
4. `exec` の明示tool（引数順、同tool重複禁止）

同一環境変数に異なる値を設定するtoolを同時にmergeする場合、後段profileの `override_allowed` にその変数名が列挙されていない限りconflict error。PATHは正規化して先勝ちで重複排除する。

Windows環境blockはcase-insensitive keyで生成し、元のkey casingを可能なら保持する。Linuxはcase-sensitive。

## 13. uninstall

1. exact install keyとreceiptを取得する。
2. user selection、今回のcwd/`--project-file`で解決したproject selection、dependency receipts、operation journalsを逆参照する。管理root外の全`.gdtvm.toml`を走査・索引化しない。
3. pre-uninstall hookをplan/confirm後にstaging contextで行う。
4. `--force`で既知selectionを解除する場合は、新selection/current/backend snapshotを準備してstateを先にcommitする。project file競合があれば削除へ進まない。
5. version directoryを同一rootのtrash nameへatomic renameする。
6. receipt index/shim/shell snapshotを更新する。
7. trashを再帰削除する。

手順4後にrenameが失敗した場合、版は安全に残るが解除済みとして警告し、利用者が必要なら再度`use`できる。trash削除失敗時はtrashを非導入状態としてdoctorが後で清掃する。管理root外cacheを削除しない。sharedは明示承認時だけ削除する。

backend方式では、手順5を「backend inventoryを再確認して完全selectorを除去し、その後receipt directoryをtrashへrename」に置き換える。他のselectorやshared cargo binaries/cacheを削除しない。backend除去失敗時はreceiptを残してunhealthyを記録し、導入済み状態を偽って消さない。

## 14. repair

修復可能項目:

- link方式のselectionsとcurrent linkの不一致、broken current link
- backend方式のreceipt/inventory/selector不一致（自動で新規downloadしない範囲）
- shim hardlink/symlink/index欠落
- 移動したportable rootの既知埋込みpath
- stale `.part`、staging、trash、operation journal
- state `.bak` からの復元
- receipt index再構築
- shell marker重複

修復不能/自動禁止:

- digest不一致payloadの黙認
- unapproved local hook
- registry signature不正の無視
- 管理root外へ出たlinkの追跡
- system package自動導入
- tool payloadの無断再download

## 15. VS Codeと長寿命process

選択変更は既に動いているprocessの環境を変更できない。shimを後から起動するprocessは新選択を解決するが、language serverが実体pathを保持している場合は再起動が必要である。`use` 成功時、対象toolが開発サーバー用途なら「terminal再起動またはVS Code Reload Windowが必要な場合がある」と一度だけ表示する。
