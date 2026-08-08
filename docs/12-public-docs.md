# GitHub公開文書仕様

## 1. 成果物と正本

repository rootに次を置く。

- `/README.md`: 初見の利用者向け概要、対応状況、導入、4操作quick start。
- `/USER_GUIDE.md`: 完全な利用者guide、command/config/platform/storage/troubleshooting。

本章は公開文書の必須内容を定める。CLI/schema/security/releaseの正本は番号付き仕様。未実装・未検証の機能を「利用可能」と書かない。実装前はplanned、部分完成はexperimental/未対応platformを明記する。

## 2. 共通執筆規則

- 日本語を主文とし、CLI/tool ID/error code/schema key/versionを翻訳しない。
- copy可能なcommandはcode fence、placeholderは`<version>`等と明示する。
- 暗黙latest、部分version、rangeをquick startに使わず、実在確認済み完全version例または`<exact-version>`を使う。
- Windows/Linux差、portable/user差、対話/非対話差を混ぜない。
- linkは相対path、anchor/file存在をCI検査する。
- secret、個人home、内部URL、変化する最新version、未公開download URLを固定しない。
- screenshotだけで操作を説明せずtextを正本とする。colorだけでwarningを区別しない。
- v0.1で提供しない機能を「近日」「予定」として詳細に書かない。延期範囲は[15-deferred.md](15-deferred.md)へlinkする。

README/USER_GUIDEのcommand、option、対応tool/platform、default、release asset名は[03-cli.md](03-cli.md)/[07-registry-and-tools.md](07-registry-and-tools.md)/[11-quality-and-ci.md](11-quality-and-ci.md)から生成またはcontract testで同期する。

## 3. README

### 3.1 冒頭

title、1段落概要、現在statusを最初に置く。概要は次を短く伝える。

- Windows/Linuxで開発toolの完全versionを非admin/nonroot導入・切替するGo CLI。
- 初心者が本/サイトに書かれた完全versionを間違えず再現することが第一の使い方。
- 標準toolはGo、Node.js、Python、.NET SDKだけ。
- tool設定/package管理は上流標準commandを使い、gdtvmは保存先を隔離する。

実装中なら「仕様策定中」「利用不可」等を冒頭で明示し、install commandを載せない。

### 3.2 badge

実在するworkflow/release/license linkだけを使う。未設定CI、security score、download数、対応外platformのbadgeを置かない。badge failureを本文statusと矛盾させない。

### 3.3 必須章

実装公開時の順序:

1. Status
2. Why gdtvm / product priorities
3. Supported platforms and tools
4. Security/privilege summary
5. Install
6. Quick start
7. Exact versions and project pins
8. Tool settings and package storage
9. Troubleshooting and reporting
10. Documentation
11. Build/test
12. Limitations/roadmap
13. Contributing/adding tools
14. Release verification
15. License

### 3.4 対応表

v0.1完成時だけ次をsupportedとする。

| Platform | Go | Node.js | Python | .NET SDK |
|---|---|---|---|---|
| Windows amd64 standard user | yes | yes | yes | yes |
| Linux amd64 glibc nonroot | yes | yes | yes | yes |

Windows/Linux arm64、Linux musl、その他tool、GUIはunsupported/planned。表の近くで次の2点を明記する。

- PythonはAstral third-party buildを使う。公式CPython binaryと誤認させない。
- **.NET SDKのWindows配布物はMITではなく .NET Library License（Microsoft独自EULA）**であり、install時に明示承認を求める。Linux配布物はMIT。

CIで自動検証した範囲と、[11-quality-and-ci.md](11-quality-and-ci.md)§9の利用者確認に委ねた範囲を区別して書く。

### 3.5 install

実装・release検証済み後、順に示す。

#### Windows推奨

1. official GitHub Releaseから`install.ps1`をdownload。
2. 公開元とscript内容を確認。
3. user権限PowerShellで実行。
4. setupで推奨`user-path`または`none`を選び、新terminalを開く。

直接`irm ... | iex`だけを唯一の例にしない。ExecutionPolicyをsystem-wide変更する例、管理者PowerShellを要求する例を載せない。

#### Linux推奨

1. `install.sh`をdownload。
2. 公開元と内容を確認。
3. user権限で実行。
4. setupで現在shell profileまたは`none`を選び、新shellを開く。

`sudo`、system directory、package自動導入を例にしない。

bootstrap optionは[11-quality-and-ci.md](11-quality-and-ci.md)§12のexact interfaceだけを掲載し、version省略時のlatest stable、完全client version指定、非対話時のpath integration/shell必須条件、任意repository/URL/rootを受けないことを説明する。

client releaseが`YYYY.MM.DD.XX`のCalVerであること、`v0.1`は機能scope名で実際のversion/tagではないことを明記する。CalVer tagはSemVerではないため、`go install <module>@vYYYY.MM.DD.XX`を導入例にせず、GitHub Releasesのarchive/bootstrapだけを正式導入経路として案内する。

#### 手動portable

release archiveと`checksums.txt`をdownloadし、SHA-256、filename、archive構造を検査してuser所有directoryへ展開する。手動archiveの既定modeはportable、bootstrapはuserである差を明記する。OS別checksum commandは実機確認済みだけを掲載する。

setup後にportable rootを移動した場合は`gdtvm setup`を再実行することを書く。

#### 更新

v0.1に自己更新commandはない。更新はbootstrap scriptの再実行または新しいarchiveの展開で行うことを明記し、存在しない`self-update`を案内しない。bootstrap再実行は新releaseを検証後にactive distributionを置換し、開発版では旧distributionのbackup/自動rollbackを行わないこと、戻す場合は完全client versionを指定して開発利用者が再取得することも明記する。導入済みtool/state/storageはdistribution置換の対象外で、任意`gdtvm.toml`は新distributionへ引き継ぐと説明する。

### 3.6 quick start

初心者向け標準4操作をこの順に示す。

```text
gdtvm setup
gdtvm install node@<exact-version>
gdtvm use node@<exact-version>
gdtvm current node
```

続けて`node --version`を確認する。`install`だけではselectionが変わらないこと、`--latest`は明示したときだけstableかつlifecycleがEOLでない完全versionへ解決すること、not-found時は`gdtvm available node`で確認することを書く。

project完全versionは`.gdtvm.toml`または`use --project`で固定できることを短く示す。

### 3.7 設定・package

READMEは詳細をUSER_GUIDEへlinkし、最低限次を示す。

- Go: `go env -w`, `go install ...@version`、global binaryは全Go version共有。
- Node.js: `npm config`, `npm install -g`、global packageはNode version別。
- Python: `python -m venv .venv`, `python -m pip`、base globalは`--user`、version別。
- .NET SDK: `dotnet nuget`, `dotnet tool`、NuGet cacheは全.NET version共有、CLI home（workload/first-run）は.NET version別。**NuGet.Config と `dotnet tool install -g` の配置先はgdtvm管理外**であることを必ず書く。

gdtvm独自package subcommandがあるように書かない。

### 3.8 limitation/security

一般user/nonroot、HKCUだけ、system PATH/HKLM/UAC/sudo/system packageなしを明記する。registry/receipt/checksum/archive検査の概要と、**client releaseが署名key/attestationを持たない保証範囲**も誤解なく短く書く。

serviceやCIはuser selectionよりproject完全versionを推奨し、use/setup後の既存process再起動が必要な場合を書く。

### 3.9 troubleshooting/報告導線

問題が起きたときの最初の1手として`gdtvm doctor`、報告時は`gdtvm doctor --report <path>`を案内する。reportがsecretと個人pathを除去したMarkdownであること、貼る前に中身を確認できることを書く。報告テンプレートは[14-maintenance.md](14-maintenance.md)§5へlinkする。

### 3.10 developer導線

build前提、minimum Go、主要test command、CI matrix、進捗台帳を示す。tool追加は[docs/14-maintenance.md](14-maintenance.md)へlinkし、definitionを1 file置くだけで無条件対応できると書かない。

Contributingには[11-quality-and-ci.md](11-quality-and-ci.md)§5.2～§5.6を要約し、次を明記する。

- `claude|codex/feature-<task-id>-<slug> → claude|codex/work → develop/work → main`の統合方向と、異なるagent workをbaseにしないこと。
- featureはsquash merge後に削除し、agent work→develop→mainはmerge commitを使うこと。
- feature PRは両OSの`lint/unit/policy`、develop以降は両OSの全6 jobを必須とすること。
- protected branchへの通常のdirect/force pushと削除は禁止し、同期・release後再作成は指定maintainerの手順で行うこと。
- release中はagent work→developを凍結するが、feature→agent workは継続できること。

## 4. USER_GUIDE

### 4.1 必須章

1. Concepts: exact version、managed selection、user/project、portable/user
2. Install/setup/remove integration/更新方法
3. 4-operation tutorial
4. 全command reference
5. Project exact/search
6. Available/latest/prerelease/EOL
7. Plan/approval/progress/JSON/offline/cache
8. Tool別設定/storage/package/venv
9. Windows PATH、Linux shell/link
10. VS Code/service/CI
11. Doctor/report/uninstall
12. Security/release verification
13. Troubleshooting/error/exit code
14. Limitations and adding tools

### 4.2 command reference

[03-cli.md](03-cli.md)の9 commandすべてについてsyntax、argument/option、default、scope、network/write/prompt、human/JSON result、主要error、例を記載する。global option位置と無効組合せも含める。`--json`が読取り専用5 commandだけであること、状態変更commandは終了codeで判定することを明記する。

`setup`ではWindowsの`user-path|none`、Linuxの`shell-profile|none`を初心者向けに説明する。Planにmode、旧/新root、filesystem/link方式、shim、integration対象、backup、再起動要否が表示されることを示す。Windowsはregistryへshim path 1 entryだけ追加し`setx`を使わないこと、removeはgdtvm entryだけ除くことを書く。

`install`ではPlan上部/下部の完全version/provider/checksum/warningをまず確認させ、その間のURL/license/read/write/probe詳細を参照できるレイアウトにする。download progress barの意味、非TTY動作も説明する。

`doctor`では10診断項目、`healthy|degraded|unhealthy`の集約、unhealthy時だけexit 12になることと、`--report`の出力内容・除去される情報を説明する。

### 4.3 tool別guide

[07-registry-and-tools.md](07-registry-and-tools.md)の4 toolだけを記載する。

- providerとofficial/third-party区分、checksum algorithm
- 配布物のlicenseとplatform差、明示承認が必要な場合
- required commandとversion確認
- 管理されるconfig/cache/global storageとscope、**管理外の領域**
- upstream標準設定command
- venv/native addon/Go global binary/NuGet cache等の注意

Python sectionはなぜsource buildではなくstandalone buildなのか、provider buildがCPython versionと別identityで固定されることを説明する。

.NET SDK sectionは次を必須項目とする。

- **Windows配布物がMITではなく .NET Library License であること**と、install時に明示承認を求めること。Linux配布物はMITであること。
- **Linuxのnative package前提**（glibc / libgcc / libstdc++ / ca-certificates / openssl / tzdata / krb5、およびICU）。gdtvmはこれらを導入しないこと。不足時にinstall probeが失敗すること。ICU無しで動かす場合は利用者が`DOTNET_SYSTEM_GLOBALIZATION_INVARIANT`を設定すること。
- **NuGet.Config と `dotnet tool install -g` の配置先が管理外**であること。
- `dotnet workload install`がread-only payloadのため失敗すること。
- telemetryが既定で無効化され、利用者が解除できないこと。

### 4.4 troubleshooting

症状→確認command→原因分類→安全な対処→変更されないdataの順で書く。最低限:

- exact version not found、catalog missing/stale、offline
- checksum/provider metadata変更
- third-party/EOL/制限的license approval不足
- PATH/profile setup conflict、new terminal/VS Code reload
- Node native global packageのversion差、Python venv/pip `--user`
- **.NET on Linux: native package前提（ICU/openssl/krb5等）が不足してinstall probeが失敗する場合**
- **.NET: `dotnet workload install`が失敗する / NuGet.Configとglobal toolが管理外である**
- junction/symlink/permission、payload unhealthy
- cancel後のtmp残留
- portable rootを移動した場合の`setup`再実行

security errorへ`--force`を万能対処として案内しない。`doctor --report`共有前に中身を確認することを案内する。

### 4.5 release検証

5 asset名、checksums canonical形式、archive SHA-256/構造/binary version、公開後再検査、bootstrap script自身の保証限界を説明する。`YYYY.MM.DD.XX`の日付がannotated tagのJST tagger timestamp、`XX`が同日最大通番＋1であること、失敗tagを再利用しないことも記載する。署名/attestationがv0.1に存在しないことを明記し、存在しないverification commandを載せない。

## 5. 同期matrix

| 変更 | README | USER_GUIDE | 同期元 |
|---|---|---|---|
| command/option/default/exit | quick例に影響時 | 必須 | 02,03,04 |
| platform/mode/PATH/shell | 必須 | 必須 | 04,09 |
| standard tool/provider/storage | 対応表/概要 | tool別必須 | 06,07 |
| security/checksum/release | 概要/導入 | 詳細必須 | 10,11 |
| branch/PR/CI/version | contributor導線/release検証 | developer/release検証 | 11 |
| config/project/data | 概要link | 必須 | 04,05 |
| tool追加workflow | contributor導線 | limitation | 14 |
| 延期機能 | limitation/roadmap | limitation | 15 |

同じPRで更新し、生成可能な表はschema/registryから検証する。文書だけ先にsupportedへしない。

## 6. 受入検査

- Markdown link/anchor/file、code fence、line ending、secret/path scan。
- READMEとUSER_GUIDEのcommand/optionsがCLI help snapshotと一致。
- standard tool件数=4、platform件数=2、command件数=9、provider/storage表がregistry contractと一致。
- .NET SDKのWindows license、Linux native前提、管理外領域が記載されている。
- quick startに暗黙latest/partial/range/auto-useがない。
- install例がadmin/root/system変更を要求しない。
- 存在しないcommand（`self-update`, `repair`, `exec`, `disable`, `refresh`, `tools`, `completion`, `use --system`）を案内していない。
- checksums/archive/script名が[11-quality-and-ci.md](11-quality-and-ci.md)のrelease fixtureと一致。
- branch名、統合方向、merge方式、CI gate、CalVerと`v0.1`の区別が[11-quality-and-ci.md](11-quality-and-ci.md)と一致。
- 実装statusとCI gateが一致し、未実装claimがない。
- 日本語message用語、no-color/狭幅でも重要警告を見落とさない。

検査command/result/report pathを[13-progress.md](13-progress.md)の該当taskへ記録する。
