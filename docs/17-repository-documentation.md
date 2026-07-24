# GitHub公開文書仕様

## 1. 目的と成果物

GitHub repository `https://github.com/kznagamori/go_dev_tool_version_manager` の利用者・開発者向け文書として、repository rootへ次の2ファイルを作成する。

| path | 対象 | 役割 |
|---|---|---|
| `/README.md` | 初めて訪れた利用者、導入者、開発参加者 | 製品概要、特徴、対応環境、最短導入、基本操作、ビルド、release検証 |
| `/USER_GUIDE.md` | 日常利用者、上級利用者、maintainer | 全操作、設定、platform差、troubleshooting、外部program、release検証と保証範囲 |

本章は上記公開文書の作成仕様である。`/docs/README.md`は実装仕様索引であり、公開用`/README.md`とは別物とする。公開文書から内部仕様へ誘導してよいが、通常利用者に内部仕様の理解を要求してはならない。

公開文書は日本語で記載する。command、option、path、環境変数、schema key、error code、製品名は仕様上のASCII表記を保つ。専門語を初出時に短く説明し、初心者向け手順ではregistry、receipt、shim等の内部語を説明なしで使用しない。

参考にする構成・読みやすさは、`https://github.com/kznagamori/AIChatHelper` の`README.md`にある「概要、主な機能、インストール、使い方、設定、ビルド、ライセンス、開発者」の流れとする。ただし内容、badge、command、platform、version、securityは本製品仕様を正とし、AIChatHelper固有のWPF、C#、画面、設定を転記しない。

## 2. 共通執筆規則

- GitHub Flavored Markdownとしてrenderできること。
- 見出しは1ファイルにつきH1を1件とし、階層を飛ばさない。
- relative linkはrepository rootを基準にし、GitHubのbranch名をURLへ埋め込まず相対pathを優先する。
- 外部linkはHTTPSとし、製品repository、GitHub Releases、Go公式、各上流公式だけを通常手順へ使用する。
- command例は実際の04章CLI構文に一致させ、完全versionを使用する。`latest`をproject設定例へ保存しない。
- Windows例はPowerShellまたはcmdのどちらかを明示し、Linux例はPOSIX shellを明示する。同じcode block内でshell構文を混在させない。
- placeholderは`<version>`, `<path>`, `<tool>`等で明示し、利用者がそのまま秘密値をcommitする例を示さない。
- 実装済みでない機能を現在利用可能と記載しない。release時に実装・評価済みのplatform/toolだけを対応表へ表示する。
- 変更される最新version番号を本文へ固定しない。badge、Releases link、`gdtvm --version`で確認させる。
- screenshotはCLI理解に実質的な効果がある場合だけ使用し、出力にuser名、home path、token、secretを含めない。画像なしでも手順が完結する文章を必須とする。
- READMEとUSER_GUIDEで同じ長い説明を重複させず、READMEから詳細節へrelative linkする。
- 文書内commandをcontract testまたはdocumentation testで検証し、廃止optionや誤った引数順を検出する。

## 3. `/README.md`仕様

### 3.1 冒頭

H1は次とする。

```text
# go_dev_tool_version_manager
```

直後に1～2文で、Windows/Linux対応、Go製、開発ツールの完全versionを導入・切替するCLI、短縮command名`gdtvm`であることを説明する。

### 3.2 badge

badgeはH1直後または概要直前へ、次の順で掲載する。repository名をAIChatHelperのまま残してはならない。

```markdown
[![Latest Release](https://img.shields.io/github/v/release/kznagamori/go_dev_tool_version_manager?label=release)](https://github.com/kznagamori/go_dev_tool_version_manager/releases)
![Platform](https://img.shields.io/badge/platform-Windows%20%7C%20Linux-0078D4)
![UI](https://img.shields.io/badge/UI-CLI-4EAA25)
![Language](https://img.shields.io/badge/language-Go-00ADD8)
![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)
```

badgeの代替textを保持し、1行が長すぎる場合は2行へ分ける。release badgeはclient tag `vYYYY.mm.DD.XX`を対象とする。

### 3.3 必須章

READMEは次の順序を基本とする。

1. `概要`
2. `主な機能`
3. `対応環境`
4. `対応ツール`
5. `インストール方法`
6. `クイックスタート`
7. `基本的な使い方`
8. `詳細ドキュメント`
9. `ビルド方法`
10. `リリース検証`
11. `セキュリティ`
12. `バージョン`
13. `ライセンス`
14. `開発者`

章を追加する場合も、導入前にmaintainer専用情報を長く置かない。

### 3.4 概要・主な機能

次を簡潔に説明する。

- 任意folderへ展開するportable modeが既定。
- OS user dataを使うuser modeも選択可能。
- 管理者所有のread-only共有distributionとuser別data rootを使うmulti-user modeも選択可能。
- tool versionの取得、digest検証、展開、完全version選択、削除。
- projectごとの`.gdtvm.toml`とuser既定選択。
- Windowsのjunction、Linuxのsymlink、native shimによる大容量tool tree非copy切替。
- client releaseに検証済み標準registryを同梱し、TOMLだけでtool追加・変更可能。
- Windows cmd、Windows PowerShell 5.1、PowerShell 7、Linux bash/zsh/fish。
- 一般ユーザー・非rootで動作し、system package manager、UAC、sudoを自動実行しない。
- 外部helperを取得する場合は事前表示し、SHA-256検証後に管理領域だけで使用する。

### 3.5 対応環境・対応ツール

platform表はWindows amd64/arm64、Linux amd64/arm64を示し、release時の評価状態を`対応`, `条件付き`, `非対応`で表す。Linux clientはCGOなしでbuildし、musl環境でclient自体を起動できることと、個々のtool artifactのlibc要件は別に説明する。

対応tool表には07章3節の17正規ID、表示名、主要用途、platform概要を掲載する。aliasは必要なものだけ併記する。全version一覧や変化しやすい最新版をREADMEへ固定しない。

### 3.6 インストール方法

WindowsとLinuxを分け、次を必須記載する。

#### Windows

1. GitHub Releasesから`gdtvm_<version>_windows_<arch>.zip`を取得する。
2. SHA-256 checksumを照合する。
3. 書込み可能な空の任意folderへ、余分な親folderを作らず展開する。
4. PowerShellまたはcmdから`gdtvm.exe setup`を実行する。
5. 表示されたshell変更内容を確認する。
6. shellを再起動し、`gdtvm version`と`gdtvm doctor`で確認する。

管理者として実行する必要がないこと、SmartScreen等の警告が出る可能性、download元とchecksumを確認することを記載する。Windows Package Managerやinstallerが提供されるまでは存在するように書かない。

#### Linux

1. GitHub Releasesから`gdtvm_<version>_linux_<arch>.tar.gz`を取得する。
2. SHA-256 checksumを照合する。
3. userが書き込める空の任意directoryへ、余分な親directoryを作らず展開する。
4. `gdtvm setup`を実行する。
5. shellを再起動し、`gdtvm version`と`gdtvm doctor`で確認する。

root/sudo不要を明記し、system-wide directoryへ配置する例を既定にしない。

portable/user/multi-user modeの違いは1段落で説明し、詳細をUSER_GUIDEへlinkする。既存配置へ手動上書きすると`gdtvm.toml`を失う可能性があるため、更新には`gdtvm self-update`を使用すること、削除前に`gdtvm setup --remove`を実行することを記載する。

### 3.7 クイックスタート・基本操作

初心者が最初に覚える4操作を、実在する完全versionの例で示す。

```text
gdtvm setup
gdtvm install node@<exact-version>
gdtvm use node@<exact-version>
gdtvm current
```

追加で`gdtvm tools`, `gdtvm available node`, `gdtvm installed node`, `gdtvm uninstall node@<exact-version>`を短く紹介する。`--latest`は解決完全版を確認する便利機能として説明できるが、`.gdtvm.toml`には完全版だけを保存することを明記する。

project固定例は`.gdtvm.toml`の最小TOML例と`gdtvm use <tool>@<exact-version> --project`を示し、user選択を変更しないことを説明する。

### 3.8 詳細文書へのlink

最低限、次を掲載する。

- 公開READMEでは「詳細な操作ガイド」というlink textで`USER_GUIDE.md`へ相対linkする。
- `[ライセンス](LICENSE)`
- GitHub Releases
- issue tracker

仕様書を公開repositoryへ含める場合だけ、開発者向けとして`docs/README.md`へlinkする。

### 3.9 ビルド方法

次を正確に記載する。

- 必要なGo toolchainは13章で固定したversion以上。同じrelease branchのCI固定versionを推奨。
- Git cloneとrepository移動。
- dependency取得・検証。
- format、unit test、race test、vet/static analysis。
- 開発build。
- Windows amd64/arm64、Linux amd64/arm64のrelease build。
- `CGO_ENABLED=0`、build metadata、`-trimpath`、version埋込み、artifact出力先。
- checksum、SBOM、provenance、artifact attestationは正式release工程で生成する。
- VS Code taskを提供した場合だけ、そのtask名と同等commandを併記する。

READMEのcommandをcopyすればclean checkoutからbuildできることをWindowsとLinux CIで確認する。存在しないscript、Make target、taskを先に文書化しない。

### 3.10 release検証のREADME記載範囲

READMEには利用者向けに次を記載し、詳細をUSER_GUIDEへlinkする。

1. GitHub ReleaseからOS・architecture一致のarchiveと`checksums.txt`を取得する。
2. archiveのSHA-256を`checksums.txt`と照合する。
3. archive、`checksums.txt`、SBOM、provenanceは同じGitHub Releaseのassetとして公開される。
4. checksum照合は破損・取り違えを検出するが、GitHub release権限とasset/checksumが同時に侵害された場合の真正性は保証しない。

### 3.11 セキュリティ・version・license・開発者

セキュリティ節には、official GitHub repository、release/archive checksum、保証範囲、artifact attestation、tool artifactのdigest、third-party警告、外部program事前表示、一般ユーザー動作、脆弱性の公開issue投稿を避ける連絡方法を記載する。security policy fileを設ける場合は`SECURITY.md`へlinkする。

version節はclientの`YYYY.mm.DD.XX`、日本時間、`00`開始、同日修正版incrementを説明する。標準定義も同じclient releaseに含まれ、独立versionを持たないことを明記する。

licenseはMITとし、`LICENSE`へrelative linkする。開発者は`kznagamori`とGitHub profile `https://github.com/kznagamori`を掲載する。

## 4. `/USER_GUIDE.md`仕様

### 4.1 目的と導線

H1は「go_dev_tool_version_manager 詳細操作ガイド」とする。冒頭に対象client versionまたは「最新release」、対応platform、本ガイドの更新日を記載し、READMEへ戻るlinkを置く。

目次を設け、GitHubの見出しanchorで移動できるようにする。通常操作、設定、保守、maintainer向けrelease工程を明確に分離する。

### 4.2 必須章

1. 製品の考え方と用語
2. system requirement
3. Windows導入
4. Linux導入
5. setupとshell integration
6. 最初のtool導入
7. 全command reference
8. project単位のversion固定
9. 設定ファイル
10. 標準定義とcatalog
11. portable root移動・backup・削除
12. offline・proxy・CI利用
13. 外部program・third-party artifact
14. VS Code連携
15. troubleshootingと`doctor`/`repair`
16. security model
17. release検証・供給工程
18. build・test・release
19. error code・終了code
20. FAQ

### 4.3 command reference

04章の全16commandを漏れなく記載する。

| command | 必須内容 |
|---|---|
| `setup` | portable/user、shell、再実行、remove、backup、警告 |
| `tools` | filter、alias、platform対応、origin |
| `available` | channel、refresh、検証状態、offline |
| `refresh` | 単体/全tool、force、部分成功 |
| `install` | exact、latest、use/project、Plan、警告 |
| `uninstall` | exact、force、keep-shared、参照拒否 |
| `installed` | health、variant、選択mark |
| `use` | user/project、未導入時、完全版 |
| `disable` | tool/all、user/project、fallback差 |
| `current` | explain、選択由来 |
| `self-update` | check/完全client版、対象OS・architecture、official Release identity・checksum検証、設定保持、rollback |
| `doctor` | tool、deep、read-only |
| `repair` | tool、dry-run、安全な修復範囲 |
| `exec` | 明示tool、一式環境、`--`、VS Code例 |
| `completion` | 対応shell、offline候補 |
| `version` | short、client版、同梱registry schema/tree revision |

各command節には構文、目的、引数、option、既定値、排他条件、状態変更、対話条件、主要終了code、text例、JSON対応可否、少なくとも1つの成功例と失敗例を記載する。READMEと異なる別名や旧`anyvm_win`構文を受理するように記載しない。

### 4.4 設定とplatform

05章の実行file隣接`gdtvm.toml`とproject設定の全field、既定値、優先順位、許可された環境変数、未知key、relative path解決を表で説明する。security緩和設定には警告を併記する。

Windowsではjunction、hardlink/small shim fallback、cmd AutoRun、PowerShell profile、execution policyを変更しない方針、標準ユーザー、VS Codeを説明する。Linuxではsymlink、bash/zsh/fish、OS user lookup、通常時の環境変数非依存、glibc/musl判定、system prerequisite、非rootを説明する。

### 4.5 外部program

外部programを次の分類で説明する。

- managed helper：7-Zip、WiX
- supplemental artifact：Windows NuGet CLI
- backend：rustup
- OS component：`msiexec.exe`, `cmd.exe`, Windows PowerShell
- system prerequisite/interpreter：`sh`, `bash`等
- client内蔵：native shim、旧symexe相当

download前のPlanに名称、完全版、URL、license、digest、size、実行理由、argv要約、書込み先が表示されること、managed helperはSHA-256検証後に管理領域だけで実行することを説明する。外部program導入前に注意・警告が表示されることと、third-party/unverified時の承認差を例示する。

### 4.6 release検証・供給工程

利用者向け説明には次を記載する。

- `checksums.txt`のcanonical形式とWindows/LinuxでのSHA-256確認方法。
- self-updateが公式repository owner/name、published release、tag、asset ID/name/sizeを確認すること。
- archive SHA-256、binary version、registry tree hash、archive構造を照合してから置換すること。
- checksumは転送破損・取り違え検出であり、GitHub権限とarchive/checksumの同時侵害を暗号学的には検出できないこと。

maintainer向け説明には次を記載する。

1. branch/tag protection、CODEOWNERS review、pinned action、最小workflow permissionを設定する。
2. protected release environmentのapproval後だけGitHub Releaseを公開する。
3. tag Git objectから同一registry bundleを生成し、4targetで同じtree hashを使う。
4. 4 archive、canonical `checksums.txt`、target別SBOM、provenance、artifact attestationを生成する。
5. 別jobで全archive SHA-256、binary version、registry hash、archive内容を再検査する。
6. 公開assetを再取得し、release ID、asset ID/name/size、SHA-256を保存する。
7. 公開後のassetを上書きせず、問題時は公開停止と新version release、security advisoryで対応する。

### 4.7 troubleshooting

最低限、次を症状、確認command、原因、安全な修復、禁止事項の順で説明する。

- `gdtvm`がPATHで見つからない。
- shell変更が反映されない。
- junction/symlink/hardlinkを作成できない。
- self-updateのrelease取得失敗、offline、proxy、rate limit。
- checksum/digest不一致。
- tool/version/platformが見つからない。
- project設定が効かない、Git境界。
- command owner競合。
- external helper/system prerequisite不足。
- VS Code/language serverが旧versionを保持する。
- stale journal、broken link、receipt不整合。
- uninstall後のshared cache。

security errorを`--force`、checksum無視、TLS無効化で回避する案内は禁止する。

## 5. READMEとUSER_GUIDEの同期

次の値は機械的またはCIで一致を検査する。

- 製品名、repository URL、CLI名。
- client version形式とtag形式。
- platformとrelease artifact名。
- 17 tool IDとalias。
- 16 command名と構文。
- global/project設定key。
- supported shell。
- license。
- build targetとminimum Go toolchain。
- release tag、archive構成、checksum形式、SBOM/provenanceのRelease asset名、artifact attestationのsubject名とdigest。

CLI、設定schema、platform対応、tool一覧、release workflowの変更では、同じpull requestでREADME、USER_GUIDE、本仕様、関連testを更新する。文書だけが先行して未実装機能を案内しない。

## 6. 受入検査

公開前に次をすべて確認する。

1. 日本語として読み通せ、初心者がREADMEだけでdownload、checksum確認、setup、最初のtool導入まで完了できる。
2. USER_GUIDEだけで全16command、設定、Windows/Linux差、offline、外部program、troubleshootingを判断できる。
3. README badgeのrepository、platform、UI、language、licenseが正しい。
4. relative link、anchor、外部linkに切れがない。
5. code blockのshellが明示され、構文検証に成功する。
6. `gdtvm --help`と全command構文・optionが一致する。
7. release artifact名、client tag、version形式が13章と一致する。
8. Windows clean VMとLinux clean environmentでinstallation/build手順を再現できる。
9. GitHub Releaseのasset集合が13章で許可したarchive、checksums、SBOM、provenanceだけである。
10. `checksums.txt`のcanonical検査、全archive SHA-256再計算、digest不一致negative testが成功する。
11. AIChatHelper固有のrepository名、WPF、C#、画面説明が残っていない。
12. secret、個人home path、token、内部限定URLが文書・画像に含まれない。
