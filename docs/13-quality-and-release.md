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
- Windows junctionの非原子的置換、Linux symlink安全性、path containment、tool artifactの署名・digest検証、client release archiveのdigest検証、lock順序、Plan fingerprint、secret maskなど、安全性やplatform差に関わる判断は「何をしているか」だけでなく「なぜ必要か」を記載する。
- 設定定義や仕様から導出される処理では、tool名を前提とした説明を避け、schema fieldまたはdomain contractとの関係を説明する。標準tool固有の動作をGoコメントで補完してはならず、TOML定義または仕様へ記載する。
- 自明な代入、loop、条件分岐を日本語へ置き換えただけの行単位コメント、コードと同じ内容の反復、コメントアウトした旧コードは禁止する。
- コメントと実装が一致しない状態を禁止する。挙動変更時は関連コメントも同じ変更で更新し、reviewではコードとコメントを一つの契約として確認する。
- `TODO`, `FIXME`を残す場合は追跡先issue ID、未完了理由、完了条件を併記する。追跡先のない一時コメントをmain/release branchへ残さない。

コメントは日本語または英語で記載できるが、同一package内では原則として使用言語を統一する。識別子、error code、schema key、CLI名は正確な表記を保ち、翻訳によって別名を作らない。コメントだけを仕様の唯一の記載場所にせず、外部から観測できる契約は必ず本仕様、schema、testにも反映する。

### 1.2 クライアントバージョン

`go_dev_tool_version_manager` クライアントの正規versionは `YYYY.mm.DD.XX` とする。

- `YYYY`は4桁の西暦年、`mm`と`DD`は2桁zero paddingした月・日、`XX`は2桁zero paddingした日次通番である。
- 日付はGit tag作成・release公開を行う時点の日本時間（IANA timezone `Asia/Tokyo`）の暦日を使う。build machineのlocal timezoneやUTC日付へ依存しない。
- その日の通常初版は`00`とする。同じ日のbug fix、差替えを伴わない追加release、または特別なreleaseは`01`, `02`の順に1ずつ増やす。
- 正規表現は `^[0-9]{4}[.](0[1-9]|1[0-2])[.](0[1-9]|[12][0-9]|3[01])[.][0-9]{2}$` とし、正規表現一致後に実在するGregorian calendarの日付であることも検査する。
- `XX`は`00`～`99`とする。同日中に次の値が100になる場合は形式を拡張せずreleaseを停止し、仕様改訂を行う。翌日の日付を先取りしてはならない。
- 比較は`YYYY`, `mm`, `DD`, `XX`を10進整数へ変換した4要素tupleの辞書順とする。文字列の部分一致、zero padding省略、SemVerへの変換を行わない。
- prerelease/build metadataを付加しない。正式公開前の成果物はversionを変形せず、release対象外のcommit SHAとCI run IDで識別する。
- client release tagは `v<client-version>`、例として `v2026.07.23.00` とする。標準定義だけのtagやreleaseは作成しない。
- versionの正本はrepository rootの`/VERSION`だけとする。UTF-8 BOMなしASCII、内容は正規`YYYY.mm.DD.XX`の1行、末尾LFちょうど1つとし、comment、空行、前後空白を禁止する。
- build/release処理は`/VERSION`をstrict検証してbuild metadataへ注入する。release binaryが実行時に外部`VERSION` fileを読む構成にはせず、埋込み値を`gdtvm --version`、`gdtvm version --short`、状態の`client_version`へ使用する。
- release tag、archive名、GitHub Release versionは`/VERSION`と完全一致させる。通常development buildだけは埋込み値`devel`を許すが、release artifactとstate migration fixtureでは拒否する。

## 2. build target

| GOOS | GOARCH | CGO | artifact |
|---|---|---:|---|
| windows | amd64 | 0 | `gdtvm_<version>_windows_amd64.zip` |
| windows | arm64 | 0 | `gdtvm_<version>_windows_arm64.zip` |
| linux | amd64 | 0 | `gdtvm_<version>_linux_amd64.tar.gz` |
| linux | arm64 | 0 | `gdtvm_<version>_linux_arm64.tar.gz` |

各archiveは展開root直下に、対象platformの`gdtvm.exe`または`gdtvm`、既定値を明記した`gdtvm.toml`、`registry/`全体、`README.md`、`USER_GUIDE.md`、`LICENSE`を必ず含める。余分な親directoryを挟まない。`registry/`はtag対象commitのrepository rootにある同名treeから生成し、17 tool、2 helper、schema、message、license、upstream key、失効情報を欠落させない。独立したregistry manifestは生成しない。

ZIPはZIP64対応のdeflate、tar.gzはPOSIX ustarで表現できないpathがある場合だけPAXを使用しgzip圧縮する。archive entryはrelative POSIX pathのUTF-8 byte順とし、timestampを`SOURCE_DATE_EPOCH`、owner/group IDを0、owner/group nameを空へ固定する。directory entryを先に置き、重複entry、absolute path、`..`、symlink、hardlink、device、extended attribute、Windows alternate data streamを禁止する。Linuxのmodeは07章、Windowsは展開後に通常fileとして扱う。同じsource・toolchain・targetの再buildでarchive SHA-256が一致することをrelease gateとする。

付随asset名はtargetごとに次へ固定する。

- SBOM: `gdtvm_<version>_<os>_<arch>.sbom.spdx.json`
- provenance: `gdtvm_<version>_<os>_<arch>.provenance.intoto.jsonl`

`checksums.txt`は4個のclient archiveだけを列挙する。SBOMとprovenanceはRelease asset ID/name/sizeで公開対象との対応を固定し、内容の完全性は各成果物をsubjectとするGitHub artifact attestationで検証する。

gdtvm本体は起動basenameでCLI/shimを分岐するmulti-call binaryとする。初回setup時、同一volumeではhardlink/symlinkを使う。Windowsでそれが不可能な場合だけ、同releaseからbuildしてclient resourceへ内蔵した最小fallback shimを管理shim領域へ展開し、version/digestを検証する。fallback shimはarchiveの独立fileでもnetwork取得物でもない。外部`tools/symexe.exe`は含めない。

## 3. build metadata

`gdtvm version` が次を返せるよう、release buildへ値を固定する。

- client calendar version `YYYY.mm.DD.XX`
- Git commit SHA（dirty禁止）
- UTC build timeまたはreproducible buildではSOURCE_DATE_EPOCH
- Go toolchain version
- tool definition schema max
- state schema max
- release同梱registry schemaとexpected tree SHA-256

版はsourceの一か所を正とし、CLI framework、archive名、release tag間の不一致をCIで拒否する。

## 4. 再現可能性とsupply chain

- dependencyは`go.mod`/`go.sum`で固定。
- `-trimpath`相当でbuild machineのpathを除く。
- `SOURCE_DATE_EPOCH`をrelease commit timeへ固定。
- 全archiveを列挙する`checksums.txt`を発行し、各行をarchive名のASCII byte順で`<64文字lowercase SHA-256><ASCII space 2個><file名><LF>`とする。BOM、CRLF、path、重複file名を禁止する。
- provenance attestationとtarget別SBOMも発行する。
- SPDXまたはCycloneDX SBOMを発行。
- GitHub Actions workflowは最小permission、commit SHAで固定したaction、protected release environment、CODEOWNERS reviewを使う。

## 5. unit test

最低限の対象:

### 5.1 domain/config

- tool ID/alias正規化、衝突
- exact version拒否例（部分、range、latest、空白、Unicode記号）
- semver/numeric/JEP223/date/regex比較
- 隣接`gdtvm.toml`、許可環境変数、CLIの優先順位
- project探索（Git directory/file、境界越え設定、root、symlink loop）
- TOML unknown key、型、line/column error
- project disabled/fallback、user/project各scopeの単体・`--all`対象snapshot、後日追加toolを暗黙無効化しないこと

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

- `checksums.txt`のcanonical形式、BOM/CRLF、unknown/missing entry
- official repository owner/name、release/tag/asset ID固定とredirect host
- checksumのmissing/extra/duplicate archive、file名不一致、digest mismatch
- GitHub Release tagと`/VERSION`、asset名、archive内binary versionの不一致
- archive traversal、symlink、case collision、required fileのextra/missing
- bundled `registry/registry.toml` compatibilityとtree schema
- self-update staging、config保持、rollback
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
- OS user lookup API、通常時に`HOME`/XDG環境変数をpath決定へ使わないこと、multi-userで許可した`GDTVM_USER_HOME`
- permission、umask、executable bit、symlink archive safety

## 7. fake upstream test server

internet状態に依存しない統合testのため、local HTTP test serverで次を模倣する。

- GitHub tag/release pagination、ETag、rate limit
- redirect chain、TLS test CA、404/429/5xx、Retry-After
- Content-Length不一致、Range resume、接続切断
- HTML/JSON layout正常/変更/空一覧
- tool提供元のchecksum file、上流artifact署名、artifact corruption
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

1. 空の書込み可能folderへrelease archiveを展開する。
2. `gdtvm setup`。
3. 同梱registryのschemaと互換性が検証される。
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

- 同梱registryとcatalog/artifact cacheありでinstall成功。
- artifactだけ不足ならnetworkを試さず明確な不足error。
- receiptとselection stateがあればcurrent/shimはnetworkなしで正常。

### Scenario F: security

- release archive digest不正、artifact digest不正、zip slip、変更local definitionをすべて実行前拒否。
- `--force`/`--yes`で上流artifact署名不正またはpath安全性違反の拒否を回避不可。

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

releaseはGitHub Actionsだけが公開し、triggerは`vYYYY.mm.DD.XX`形式のrelease tag pushとする。workflowはtagが指すcommitだけをcheckoutし、branch先端や別commitのregistryを混ぜない。

1. tag作成前に`/VERSION`、tag予定値、日本時間の日付・通番、作業tree、`registry/`全entryを検証し、全unit/integration/security/contract testを完了する。
2. `/VERSION`と完全一致するannotated tag `v<version>`を作成してpushする。
3. GitHub Actionsはtag形式、tag名と`/VERSION`の一致、commitの一意性を再検査する。
4. tag commitのGit objectから`registry/`を改行変換なしで一度だけexportし、strict検証後に07章のtree SHA-256を計算する。workflow内のimmutable registry bundleとそのSHA-256を作り、4 matrix jobは同じbundleを再検証して使用する。各jobはそのtree hashをbuild metadataへ固定し、Windows amd64/arm64とLinux amd64/arm64を`CGO_ENABLED=0`で生成して各archiveへ2章の必須file群を配置する。runnerごとのworking-tree改行変換後fileをpackageしない。
5. archive内のregistry strict validation、binary埋込みregistry hashとの一致、文書、設定、binary version、実行permission、余分なfileの有無を検査する。
6. target別SBOMとprovenanceを生成し、4 archiveの`checksums.txt`をcanonical生成する。
7. 別jobが`checksums.txt`から全archive SHA-256を再計算し、archive名・size・binary version・registry hashを再検査する。
8. GitHub artifact attestationを各archive、SBOM、provenanceへ生成する。attestationは監査用であり、schema 1 self-updateの必須入力にはしない。
9. protected release environmentのapproval後、GitHub Releaseを作成し、4 archive、`checksums.txt`、SBOM、provenanceをassetとして公開する。同名assetの上書きと公開済みtagの差替えを禁止する。
10. 公開assetを再downloadし、Release APIのowner/name、release ID、tag、asset ID/name/size、SHA-256をrelease記録へ保存する。Windows clean VMを先に、Linux clean環境を後に、展開、setup、registry読込み、install smoke、`self-update --check`まで確認する。

## 14.1 release securityの保証範囲

- `checksums.txt`とarchive SHA-256照合は転送破損、部分download、asset取り違えを検出する。
- binary埋込みregistry tree hashはclientとregistryの混在、展開後のregistry変更を検出する。
- reproducible build、SBOM、provenance、GitHub artifact attestationはrelease監査と第三者検証を補助する。
- branch/tag protection、CODEOWNERS、pinned action、最小workflow permission、protected environment approval、公開後再取得検査でrelease権限の侵害可能性を低減する。
- GitHub account・workflow・release権限が侵害され、archiveと`checksums.txt`が整合する形で同時改ざんされた場合、schema 1 clientはそれを暗号学的に検出できない。この制約をREADMEとUSER_GUIDEのsecurity節へ明記する。
- GitHubで侵害または誤releaseが疑われる場合、該当releaseを黙って差し替えず公開停止し、新version tagで修正版を発行してsecurity advisoryへrelease ID、asset ID、既知SHA-256、影響版を掲載する。

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
- official GitHub Release identity・archive checksum・同梱registry検証・self-update rollback safety。
- portable/user/multi-user mode、一般ユーザー、非root、shell setup/undo。
- exact-only version policyとproject/user selection。
- junction/symlink/shim fallback、ツールtree copyなし。
- third-party/unverified/local definition警告・承認。
- 7-Zip/WiX/NuGet/rustup等の外部実体がPlan前表示、digest検証、管理path固定を満たす。
- 15章のreference definitionと追加local toolがtool固有Goコードなしで全CRUD/runtimeを通る。
- core internal API経由でCLIが動き、CLIにbusiness logicがない。
- Wails bridge相当のPlan/Start/Subscribe/Cancel contract testがCLIなしで通る。
- unit/integration/security/E2EがCIで成功。
