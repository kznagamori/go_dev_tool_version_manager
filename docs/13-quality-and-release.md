# 品質・テスト・リリース仕様

## 1. 開発基準

- 実装言語はGo。
- 開発開始時のminimum toolchainはGo 1.26系とし、CI/releaseは最新security patchへ固定する。本仕様作成時点の公式最新はGo 1.26.5（2026-07-07公開）。
- client CLIはWindows/LinuxともCGOなしでbuildする。
- format、vet、static analysis、race test（対応host）、vulnerability scan、license scanをCI必須にする。
- production pathでpanicを通常error処理に使わない。最上位で予期しないpanicをoperation ID付きerrorに変換し、secretを含まないcrash情報をlogする。

Go release policy参照: `https://go.dev/doc/devel/release`

### 1.1 ソースコメント

実装するGoソースには、コードを読むだけでは把握しにくい契約と処理意図を残すため、次のコメントを必須とする。

- 各packageには、そのpackageの責務、扱う範囲、依存してよい層を説明するpackage documentation commentを1件置く。複数fileに重複して書かない。
- exportされるtype、function、method、variable、constantにはGoのdocumentation conventionに従うドキュメントコメントを書く。コメントは原則として宣言名から始め、用途、重要な入力制約、戻り値、状態変更、並行安全性、error条件のうち利用者が判断に必要な内容を記載する。
- exportされない宣言でも、Application Service境界、domain invariant、security検査、永続化schema、transaction、platform固有処理、複雑なalgorithmを担うものには、その責務または不変条件を説明するコメントを書く。
- 複数段階の処理には、段階を分ける理由、順序を変えてはならない理由、rollback境界、外部processを実行する理由などを、処理の直前へ適度に記載する。
- Windows junctionの非原子的置換、Linux symlink安全性、path containment、署名・digest検証、lock順序、Plan fingerprint、secret maskなど、安全性やplatform差に関わる判断は「何をしているか」だけでなく「なぜ必要か」を記載する。
- 設定定義や仕様から導出される処理では、tool名を前提とした説明を避け、schema fieldまたはdomain contractとの関係を説明する。標準tool固有の動作をGoコメントで補完してはならず、TOML定義または仕様へ記載する。
- 自明な代入、loop、条件分岐を日本語へ置き換えただけの行単位コメント、コードと同じ内容の反復、コメントアウトした旧コードは禁止する。
- コメントと実装が一致しない状態を禁止する。挙動変更時は関連コメントも同じ変更で更新し、reviewではコードとコメントを一つの契約として確認する。
- `TODO`, `FIXME`を残す場合は追跡先issue ID、未完了理由、完了条件を併記する。追跡先のない一時コメントをmain/release branchへ残さない。

コメントは日本語または英語で記載できるが、同一package内では原則として使用言語を統一する。識別子、error code、schema key、CLI名は正確な表記を保ち、翻訳によって別名を作らない。コメントだけを仕様の唯一の記載場所にせず、外部から観測できる契約は必ず本仕様、schema、testにも反映する。

## 2. build target

| GOOS | GOARCH | CGO | artifact |
|---|---|---:|---|
| windows | amd64 | 0 | `gdtvm_<version>_windows_amd64.zip` |
| windows | arm64 | 0 | `gdtvm_<version>_windows_arm64.zip` |
| linux | amd64 | 0 | `gdtvm_<version>_linux_amd64.tar.gz` |
| linux | arm64 | 0 | `gdtvm_<version>_linux_arm64.tar.gz` |

Windows archiveは `gdtvm.exe`、license、最小readmeだけを含めてよい。Linux archiveは`gdtvm`、license、最小readme。標準tool definitionsや別shim executableをclient archiveへ同梱しないという「exe主体配布」を守り、初回にregistryから取得する。

gdtvm本体は起動basenameでCLI/shimを分岐するmulti-call binaryとする。初回setup時、同一volumeではhardlink/symlinkを使う。Windowsでそれが不可能な場合だけ、同releaseからbuildしてclient resourceへ内蔵した最小fallback shimを管理shim領域へ展開し、version/digestを検証する。fallback shimはarchiveの独立fileでもnetwork取得物でもない。外部`tools/symexe.exe`は含めない。

## 3. build metadata

`gdtvm version` が次を返せるよう、release buildへ値を固定する。

- client SemVer
- Git commit SHA（dirty禁止）
- UTC build timeまたはreproducible buildではSOURCE_DATE_EPOCH
- Go toolchain version
- tool definition schema max
- state schema max
- embedded registry public key IDs

版はsourceの一か所を正とし、CLI framework、archive名、release tag間の不一致をCIで拒否する。

## 4. 再現可能性とsupply chain

- dependencyは`go.mod`/`go.sum`で固定。
- `-trimpath`相当でbuild machineのpathを除く。
- `SOURCE_DATE_EPOCH`をrelease commit timeへ固定。
- targetごとのSHA-256 checksum fileを発行。
- release artifact/checksumへmaintainer signatureまたはprovenance attestationを付ける。
- SPDXまたはCycloneDX SBOMを発行。
- GitHub Actions等のworkflowは最小permission、pinned action commit、protected release environmentを使う。
- registry signing keyとclient release signing credentialを分離する。

## 5. unit test

最低限の対象:

### 5.1 domain/config

- tool ID/alias正規化、衝突
- exact version拒否例（部分、range、latest、空白、Unicode記号）
- semver/numeric/JEP223/date/regex比較
- global/env/CLI優先順位
- project探索（Git directory/file、境界越え設定、root、symlink loop）
- TOML unknown key、型、line/column error
- project disabled/fallback

### 5.2 definition

- 全fieldとunknown key
- template context escaping/root逸脱
- conditionの型/left/nest/limit
- source transform/channel/deduplicate、JSON `items_path` flatten、asset mapping
- platform priority ambiguity
- artifact roleごとの主/補助artifact一意選択、availability HEAD/Range
- step output DAG、dependency cycle、`for_each`順序/cardinality
- command/alias衝突
- shell/hook/checksum policy
- rustup backendのexact selector、managed home、他selectorを削除しない契約
- [15-reference-definition.md](15-reference-definition.md)のTOML parse/schema validationとfake upstream contract

### 5.3 registry/security

- valid/invalid Ed25519
- raw bytes改行/BOM変更で署名失敗
- key ID/rotation
- manifest path/size/hash mismatch
- annotated/lightweight tag解決
- archive traversal、symlink、case collision、extra/missing file
- previous snapshot rollback維持
- local definition fingerprint/reapproval
- secret mask

### 5.4 install/store

- plan stale fingerprint
- interrupted download/Range resume
- retry対象/非対象
- digest mismatch
- archive bomb limit
- staging failure/commit/cleanup warning
- same install同時実行
- receipt/state atomic backup recovery
- selectionのversion/variant/install ID一致とdefinition更新後の非読み替え
- dependency plan/conflict/optional/system hint
- uninstall selection/dependency guard

### 5.5 runtime/shim

- invoked basename、`.exe`除去
- project/user/disabled優先順位
- exact receipt targetとloop防止
- 同名command候補が1 toolだけ有効なら起動し、複数有効なら`E_COMMAND_AMBIGUOUS`となり列挙順で選ばない
- env merge、Windows key case、PATH重複
- args/stdio/exit code/signal
- codepage復元
- stale shim index read-only fallback

## 6. integration test

### 6.1 Windows

管理者でないtest user/tokenで実行する。

- writable NTFS portable root
- directory junction作成・置換・targetを消さない除去
- shim hardlinkとsmall copy fallback、argv[0]によるhardlink呼出名判定
- native/cmd-script launcherの引数、exit code、特殊文字
- cmd.exeからnative shim argv/exit code
- Windows PowerShell 5.1、PowerShell 7から同じ結果
- HKCU AutoRun未設定/単純script/複合inline値の分岐、既存値保持、marker idempotency、undo、第三者変更競合
- PowerShell profile encoding/backup
- UAC promptが出ないこと
- pathにspace、日本語、長pathを含むcase
- VS Code相当の親子process inheritance test

### 6.2 Linux

非root user、最小container/VMで実行する。

- glibc distributionとmusl distributionでclient起動
- amd64/arm64 targetはnative CIまたは信頼できるrunner
- link方式のcurrent relative symlink/atomic replace/shim direct fallbackと、backend方式のlink非作成
- native/sh-script launcherの引数、exit code、特殊文字
- bash/zsh/fish marker/setup/undo
- XDG各変数の設定/未設定
- permission、umask、executable bit、symlink archive safety

## 7. fake upstream test server

internet状態に依存しない統合testのため、local HTTP test serverで次を模倣する。

- GitHub tag/release pagination、ETag、rate limit
- redirect chain、TLS test CA、404/429/5xx、Retry-After
- Content-Length不一致、Range resume、接続切断
- HTML/JSON layout正常/変更/空一覧
- checksum file、signature、artifact corruption
- zip/tar/7z fixtureと悪性archive

productionでHTTPを拒否しても、test portへ注入したHTTPClientはin-memory transportで試験できるようにする。

## 8. standard tool contract test

registryの各definitionに共通contract suiteを適用する。

- ID/file/name/schema整合
- 現platformの一致候補で最大priorityが一意であり、Resolverの選択結果が1件以下
- sourceから得たversionが完全版でsort可能
- stable latestがprereleaseでない
- artifact templateに未解決変数なし
- checksum requiredまたは明示unverified policy
- install planがroot外へ書かない
- required command targetがlink方式ではpayload内、backend方式ではtool shared内
- validation expected version一致
- uninstallでreceipt外treeを削除しない

上流へ接続するnightly testと、固定fixtureだけのPR testを分ける。上流一時障害でclientコードPRを不安定にしないが、registry発行はlive smoke成功を必須にする。

## 9. 旧機能回帰受入

Windows amd64で17toolすべてについて次を確認する。

1. catalog更新（Android/Rustのbackend固有方式を含む）。
2. available list/latest/exact install。
3. installed/current/use/disable/uninstall。
4. link方式はcurrent junctionまたは規定fallback、Rust backend方式はshared inventory/完全selector。
5. 旧activateで設定していたPATH/環境変数の新runtime反映。
6. download progressと成功後archive削除。
7. 2版間切替と選択mark。

固有回帰:

- Ninja: UTF-8 codepage、固定target、symexe不要。
- Python: WiX＋MSI administrative extraction、ensurepip。
- LLVM/MinGW/WinLibs: 検証済み7-Zip helper、7z展開。
- dotnet: NuGet実体と分離cache/environment。
- Rust: rustup/CARGO_HOME/RUSTUP_HOME、`RUSTUP_TOOLCHAIN`の完全selector、Windows GNU legacy variant、任意sccache、deprecatedな`RUSTUP_DIST_ROOT`を設定しないこと、版別payload copyがないこと。
- Android: cmdline-tools/latest layout、SDK root環境。
- Go/Dart/Flutter/Gradle/JDK/LLVM/WinLibs: 固有home/cache/library環境。
- 全体refresh、全体disable、全体currentが旧wrapper機能を代替。
- repairが旧rehashの必要性を代替。

## 10. end-to-end受入scenario

### Scenario A: 初心者portable

1. 空の書込み可能folderへclientだけを置く。
2. `gdtvm setup`。
3. signed registryが取得される。
4. `gdtvm install node@<fixture-exact>`。
5. `gdtvm use node@<same>`。
6. 新shellで`node --version`が完全一致。
7. folderを移動、`repair`、再度一致。

### Scenario B: project固定

1. user node Aを選ぶ。
2. Git projectへ `.gdtvm.toml` でnode Bを完全指定。
3. project内のnode shimはB、外ではA。
4. Git root外の親設定は既定で無視。
5. global search settingをtrueにすると境界を越える。

### Scenario C: 未導入use

- 対話: install提案→yes→導入→選択。
- 非対話 `--yes`なし: 変更なしで失敗。
- 非対話 `--yes`: verified official、またはsigned standard definitionのverified third-partyなら成功。third-party警告は残る。

### Scenario D: third-party Python Linux

- provider/source/license/reason/digestを表示。
- noで変更なし。
- yesでreceiptにthird-party approval。
- `--yes`でもwarningがlog/outputに残る。

### Scenario E: offline

- registry/catalog/artifact cacheありでinstall成功。
- artifactだけ不足ならnetworkを試さず明確な不足error。
- active registryがあればcurrent/shimは正常。

### Scenario F: security

- registry signature不正、artifact digest不正、zip slip、変更local definitionをすべて実行前拒否。
- `--force`/`--yes`でsignature/path拒否を回避不可。

### Scenario G: VS Code

- setup済みcmd/PowerShellから`code.exe .`相当のtest childを起動。
- PATHからshimを解決しproject版を起動。
- user currentのhome環境が継承される。
- `gdtvm exec -- code.exe .`相当ではproject環境変数も親childへ入る。

## 11. 非機能受入

- client binaryは4targetすべて起動し、Linuxで`ldd`依存なし相当を検証。
- tool未導入の`current`は100 ms目標。
- cached shim median overhead 20 ms目標をbenchmarkで追跡。
- force kill後に完成versionが半端公開されない。
- logにfixture token/secretが出ない。
- 10,000 catalog versionsでもmemory/time上限内。
- 2 GiB超artifactを全量memoryへ載せずstream処理。

## 12. 国際化品質

- message IDをja/enで1対1に揃えるCIを設ける。
- error code、tool ID、version、path、URLは翻訳しない。
- 日本語端末で文字化けしない。cmdのcode page差はshim/runtimeで処理する。
- `--json` のenum/codeは英語固定、任意のlocalized message fieldはlanguageを明記。
- pluralやbyte/time表記はlocale layerで処理し、domainに文字列組立を置かない。

## 13. logging/observability

console levelとfile levelを分離できる。既定はconsole info、file info。logはUTC ISO 8601、level、operation ID、component、event、structured fields。

- rotation: 5 MiB×5既定。
- error時にstack/causeをfileへ、通常consoleには要約。
- external stdout/stderrはdebugで上限付き、secret mask後。
- progressの全tickをfileへ記録せず節目だけ。
- `doctor --deep`でlog directory/permission/rotationを検査。

## 14. client release手順

1. 全unit/integration/security/contract test。
2. 4target buildとnative smoke。
3. version/schema/public key確認。
4. SBOM、checksums、provenance/signature生成。
5. clean VMでexe-only first-run bootstrap。
6. GitHub releaseをdraft作成、artifact照合。
7. tagを作成し公開。
8. latest downloadを再取得しchecksum/起動確認。

client releaseとregistry releaseは独立する。client公開時に互換registryが最低1版存在することを必須とする。

## 15. Wails v3将来受入

GUI実装前でも次を維持する。

- コアにWails/CGO importなし。
- 全長時間操作にeventとcancel。
- 全確認にApproval API。
- Planを先に取得可能。
- path/URL/process等がstructured result。
- CLIなしでApplication Service integration testが通る。

Wails buildはCLIのCGOなし配布物と別artifact・別build pipelineにする。GUI都合でLinux CLIをmusl linkへ変更したりWebView依存を加えない。

## 16. 完了条件

初期Go移植版は、次がすべて成立した時だけ完成とする。

- 本仕様の必須コマンドと4 client platform build。
- Windows amd64で17toolの旧機能回帰。
- 対応表「必須」のplatform/tool smoke。
- signed registry bootstrap/update/offline/rollback safety。
- portable/user mode、一般ユーザー、shell setup/undo。
- exact-only version policyとproject/user selection。
- junction/symlink/shim fallback、ツールtree copyなし。
- third-party/unverified/local definition警告・承認。
- 7-Zip/WiX/NuGet/rustup等の外部実体がPlan前表示、digest検証、管理path固定を満たす。
- 15章のreference definitionと追加local toolがtool固有Goコードなしで全CRUD/runtimeを通る。
- core internal API経由でCLIが動き、CLIにbusiness logicがない。
- Wails bridge相当のPlan/Start/Subscribe/Cancel contract testがCLIなしで通る。
- unit/integration/security/E2EがCIで成功。
