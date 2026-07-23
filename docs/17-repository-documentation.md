# GitHub公開文書仕様

## 1. 目的と成果物

GitHub repository `https://github.com/kznagamori/go_dev_tool_version_manager` の利用者・開発者向け文書として、repository rootへ次の2ファイルを作成する。

| path | 対象 | 役割 |
|---|---|---|
| `/README.md` | 初めて訪れた利用者、導入者、開発参加者 | 製品概要、特徴、対応環境、最短導入、基本操作、ビルド、署名鍵運用への入口 |
| `/USER_GUIDE.md` | 日常利用者、上級利用者、maintainer | 全操作、設定、platform差、troubleshooting、外部program、registry署名鍵の詳細 |

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
- screenshotはCLI理解に実質的な効果がある場合だけ使用し、出力にuser名、home path、token、秘密鍵を含めない。画像なしでも手順が完結する文章を必須とする。
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

badgeの代替textを保持し、1行が長すぎる場合は2行へ分ける。release badgeはclient tag `vYYYY.mm.DD.XX`を対象とする。registry tagを最新client releaseとして表示しないことをGitHub Release運用と表示確認で保証する。

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
10. `registry署名鍵`
11. `セキュリティ`
12. `バージョン`
13. `ライセンス`
14. `開発者`

章を追加する場合も、導入前にmaintainer専用情報を長く置かない。

### 3.4 概要・主な機能

次を簡潔に説明する。

- 任意folderへ展開するportable modeが既定。
- OS user dataを使うuser modeも選択可能。
- tool versionの取得、digest検証、展開、完全version選択、削除。
- projectごとの`.gdtvm.toml`とuser既定選択。
- Windowsのjunction、Linuxのsymlink、native shimによる大容量tool tree非copy切替。
- 署名済みonline registryからtool定義を取得し、TOMLだけでtool追加・変更可能。
- Windows cmd、Windows PowerShell 5.1、PowerShell 7、Linux bash/zsh/fish。
- 一般ユーザー・非rootで動作し、system package manager、UAC、sudoを自動実行しない。
- 外部helperを取得する場合は事前表示し、SHA-256検証後に管理領域だけで使用する。

### 3.5 対応環境・対応ツール

platform表はWindows amd64/arm64、Linux amd64/arm64を示し、release時の評価状態を`対応`, `条件付き`, `非対応`で表す。Linux clientはCGOなしでbuildし、musl環境でclient自体を起動できることと、個々のtool artifactのlibc要件は別に説明する。

対応tool表には07章2.1節の17正規ID、表示名、主要用途、platform概要を掲載する。aliasは必要なものだけ併記する。全version一覧や変化しやすい最新版をREADMEへ固定しない。

### 3.6 インストール方法

WindowsとLinuxを分け、次を必須記載する。

#### Windows

1. GitHub ReleasesからOS/architecture一致のZIPを取得する。
2. SHA-256 checksumを照合する。
3. 書込み可能な任意folderへ展開する。
4. PowerShellまたはcmdから`gdtvm.exe setup`を実行する。
5. 表示されたshell変更内容を確認する。
6. shellを再起動し、`gdtvm version`と`gdtvm doctor`で確認する。

管理者として実行する必要がないこと、SmartScreen等の警告が出る可能性、download元とchecksumを確認することを記載する。Windows Package Managerやinstallerが提供されるまでは存在するように書かない。

#### Linux

1. GitHub ReleasesからOS/architecture一致のtar.gzを取得する。
2. SHA-256 checksumを照合する。
3. userが書き込める任意directoryへ展開する。
4. `gdtvm setup`を実行する。
5. shellを再起動し、`gdtvm version`と`gdtvm doctor`で確認する。

root/sudo不要を明記し、system-wide directoryへ配置する例を既定にしない。

portable/user modeの違いは1段落で説明し、詳細をUSER_GUIDEへlinkする。削除前に`gdtvm setup --remove`を実行する注意も記載する。

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
- checksum、SBOM、署名/provenanceは正式release工程で生成する。
- VS Code taskを提供した場合だけ、そのtask名と同等commandを併記する。

READMEのcommandをcopyすればclean checkoutからbuildできることをWindowsとLinux CIで確認する。存在しないscript、Make target、taskを先に文書化しない。

### 3.10 registry署名鍵のREADME記載範囲

READMEにはmaintainer向けであることを明示し、次の概要だけを記載してUSER_GUIDEの詳細節へlinkする。

1. registryはEd25519鍵pairで署名する。
2. 秘密鍵はrepository外に保存し、commitしない。
3. 公開鍵と`key_id`だけをclient trust storeへ登録してcommitする。
4. code commit後にorphan `registry` branchへswitchする。
5. manifestを生成し、repository外の秘密鍵で署名する。

秘密鍵の実値、秘密鍵をrepository内へcopyする例、CI logへ秘密鍵path/contentを表示する例をREADMEへ記載しない。

### 3.11 セキュリティ・version・license・開発者

セキュリティ節には、署名済みregistry、artifact digest、third-party警告、外部program事前表示、一般ユーザー動作、脆弱性の公開issue投稿を避ける連絡方法を記載する。security policy fileを設ける場合は`SECURITY.md`へlinkする。

version節はclientの`YYYY.mm.DD.XX`、日本時間、`00`開始、同日修正版incrementを説明し、registryのSemVerとは別であることを明記する。

licenseはMITとし、`LICENSE`へrelative linkする。開発者は`kznagamori`とGitHub profile `https://github.com/kznagamori`を掲載する。

## 4. `/USER_GUIDE.md`仕様

### 4.1 目的と導線

H1は「go_dev_tool_version_manager 詳細操作ガイド」とする。冒頭に対象client versionまたは「最新release」、対応platform、本ガイドの更新日を記載し、READMEへ戻るlinkを置く。

目次を設け、GitHubの見出しanchorで移動できるようにする。通常操作、設定、保守、maintainer向け署名鍵を明確に分離する。

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
10. registryとcatalog
11. portable root移動・backup・削除
12. offline・proxy・CI利用
13. 外部program・third-party artifact
14. VS Code連携
15. troubleshootingと`doctor`/`repair`
16. security model
17. maintainer向けregistry署名鍵
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
| `registry update` | latest/完全registry版、署名、rollback警告 |
| `doctor` | tool、deep、read-only |
| `repair` | tool、dry-run、安全な修復範囲 |
| `exec` | 明示tool、一式環境、`--`、VS Code例 |
| `completion` | 対応shell、offline候補 |
| `version` | short、client版とregistry版 |

各command節には構文、目的、引数、option、既定値、排他条件、状態変更、対話条件、主要終了code、text例、JSON対応可否、少なくとも1つの成功例と失敗例を記載する。READMEと異なる別名や旧`anyvm_win`構文を受理するように記載しない。

### 4.4 設定とplatform

05章のbootstrap/global/project全field、既定値、優先順位、環境変数、未知key、relative path解決を表で説明する。security緩和設定には警告を併記する。

Windowsではjunction、hardlink/small shim fallback、cmd AutoRun、PowerShell profile、execution policy、標準ユーザー、VS Codeを説明する。Linuxではsymlink、bash/zsh/fish、XDG、glibc/musl判定、system prerequisite、非rootを説明する。

### 4.5 外部program

外部programを次の分類で説明する。

- managed helper：7-Zip、WiX
- supplemental artifact：Windows NuGet CLI
- backend：rustup
- OS component：`msiexec.exe`, `cmd.exe`, Windows PowerShell
- system prerequisite/interpreter：`sh`, `bash`等
- client内蔵：native shim、旧symexe相当

download前のPlanに名称、完全版、URL、license、digest、size、実行理由、argv要約、書込み先が表示されること、managed helperはSHA-256検証後に管理領域だけで実行することを説明する。外部program導入前に注意・警告が表示されることと、third-party/unverified時の承認差を例示する。

### 4.6 registry署名鍵の詳細

この節はmaintainer専用と明記し、鍵を「証明書」と曖昧に呼ばずEd25519署名鍵pairと表記する。X.509は使用しない。

#### 鍵形式

- 秘密鍵の保存形式はPKCS#8 PEM。
- 公開鍵の交換・保管形式はSubjectPublicKeyInfo PEM。
- client埋込みtrust storeでは、Ed25519 raw public key 32 bytesをstandard base64で表した値とASCII `key_id`を使用する。
- `key_id`は例として`registry-root-YYYY-NN`形式を使用できるが、repository内で一意かつ不変とする。

#### OpenSSLによる生成例

OpenSSL 3系を使用する例として、次の意味を持つcommandを掲載する。

1. repository外のowner-only directoryを作成する。
2. `openssl genpkey -algorithm ED25519`でPKCS#8秘密鍵を生成する。
3. `openssl pkey -pubout`でSPKI公開鍵を生成する。
4. 公開鍵fingerprintを表示し、別経路で照合する。
5. WindowsではACL、Linuxではdirectory `0700`・private key `0600`相当を設定する。

実際のREADME/USER_GUIDE作成時には、Windows PowerShell用とLinux shell用のcommand blockを分ける。command実行前に`<repository外の安全なpath>`を置換する説明を入れる。秘密鍵を現在directoryへ生成する例を禁止する。

OpenSSL version差でraw public key抽出が不安定になるため、client trust store用base64はmaintainer utility `gdtvm-registry key inspect`または`gdtvm-registry key add`でSPKI公開鍵から変換する。公開文書には07章4.3節の正式構文だけを掲載し、独自scriptやOpenSSL byte切出しpipelineを代替手順にしない。

#### 公開鍵の付与

詳細手順は次の順序とする。

1. client source branchで鍵pairをrepository外に生成する。
2. private key、seed、一時fileがGit worktree内にないことを確認する。
3. `gdtvm-registry key add --public-key <absolute-spki-pem> --key-id <id>`で公開鍵を`internal/security/trust/registry-keys.toml`へ登録する。
4. 正しい署名を受理し、別鍵・改変manifestを拒否するtestを実行する。
5. 公開鍵、`key_id`、testだけをclient source branchへcommitする。
6. 作業treeがcleanで秘密鍵が未追跡fileにもないことを確認する。
7. 既存registryでは`git switch registry`、初回だけ07章のorphan branch作成手順を実行する。
8. registry fileを配置してmanifestを生成する。
9. 07章4.3.3節の`gdtvm-registry manifest build`へrepository外のpublic/private key path、`key_id`、registry版、作成日時、最小client版を明示して`manifest.toml`と`manifest.sig`を生成する。
10. embedded public keyで署名、manifest hash、全file hashを独立検証してからcommit/tag/pushする。

初回の手順7では、作業treeがcleanであることを確認した後、次を順番どおり掲載する。`<空ブランチ名>`は標準構成では`registry`である。この2コマンドは既存registry更新時には実行しない。

```text
git checkout --orphan <空ブランチ名>
git rm -rf .
```

秘密鍵pathをbuild flag、source、TOML、shell history、CI artifact、logへ保存しない。CI secretを使用する場合はprotected environment、最小権限、mask、fork PR非公開を記載する。鍵紛失、漏えい、rotationは07章と11章の規則を説明し、公開済みtagを差し替えない。

#### utility掲載条件

公開文書へ07章4.3節の鍵登録・manifest署名commandを掲載する前に、次を満たす。

- `gdtvm-registry`のcommand名、全option、入力形式、出力path、終了codeが07章4.3節と`--help`で一致する。
- dry-runまたはverify-only経路がある。
- private key内容をstdout/stderrへ出さない。
- repository内private key pathを拒否または強い警告で停止する。
- Windows標準ユーザーとLinux非rootで評価済み。
- documentation testがcommand例を実行または構文検証する。

### 4.7 troubleshooting

最低限、次を症状、確認command、原因、安全な修復、禁止事項の順で説明する。

- `gdtvm`がPATHで見つからない。
- shell変更が反映されない。
- junction/symlink/hardlinkを作成できない。
- registry bootstrap/update失敗、offline、proxy、rate limit。
- digest/signature不一致。
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
- registry branch/tag、署名algorithm、key ID。

CLI、設定schema、platform対応、tool一覧、鍵utilityの変更では、同じpull requestでREADME、USER_GUIDE、本仕様、関連testを更新する。文書だけが先行して未実装機能を案内しない。

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
9. 鍵手順でprivate keyがrepository、Git index、log、artifactへ入らない。
10. 公開鍵登録後のpositive/negative signature testとmanifest verifyが成功する。
11. AIChatHelper固有のrepository名、WPF、C#、画面説明が残っていない。
12. secret、個人home path、token、private key、内部限定URLが文書・画像に含まれない。
