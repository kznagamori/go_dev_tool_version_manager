# go_dev_tool_version_manager 仕様書

本ディレクトリは、WindowsとLinuxで開発toolの完全versionを導入・切替するGo製CLI `go_dev_tool_version_manager`（コマンド名`gdtvm`）の規範仕様である。

## 1. 製品優先順位

判断が競合する場合は、次の順序を正とする。

1. WindowsとLinuxで開発toolを簡単に導入できること。
2. 初心者が書籍・公式サイト・projectに記載された完全versionを間違えずに導入・使用でき、version違いの問題を起こしにくいこと。操作数の少なさより、完全版の明示と予測可能性を優先する。
3. 中級者以降が各toolの公式command・設定fileを使い、gdtvm管理領域内で通常の設定を行えること。
4. 企業向けsecurity・監査機能は必要最小限とすること。企業は開発toolを公式のinstall文書に従って導入すると想定し、gdtvmはその代替を目指さない。

## 2. release段階とv0.1

初期完成対象は**v0.1**である。ここでいうv0.1は機能scopeと完成段階の名称であり、client version、Git tag、Go module versionではない。実際のclient versionは§5.3のCalVerを使う。v0.1に含まれない機能は本仕様から除いてあり、[15-deferred.md](15-deferred.md)へ延期理由、再導入gate、実装promptを記録する。

- v0.1の記述へ「将来用」のfield、enum値、予約keyを先行追加しない。
- 延期機能を実装したくなった場合は、実装から始めず[15-deferred.md](15-deferred.md)の該当節と[14-maintenance.md](14-maintenance.md)の拡張手順を先に読む。
- v0.1は**ドッグフーディング前提**である。利用者が実際に使って問題を報告し、修正する循環を最短で回すことを、機能の網羅性より優先する。

## 3. 文書一覧

| 文書 | 規範領域 |
|---|---|
| [01-requirements.md](01-requirements.md) | 製品範囲、対象利用者、機能要件、非目標、用語 |
| [02-architecture.md](02-architecture.md) | layer、package、port、Application Service契約 |
| [03-cli.md](03-cli.md) | CLI構文、option、出力、対話、終了code |
| [04-storage-and-data.md](04-storage-and-data.md) | path、state、永続dataの機械契約、組込み上限 |
| [05-configuration.md](05-configuration.md) | 任意global設定、project設定、環境変数 |
| [06-tool-definition.md](06-tool-definition.md) | tool definition schema 1と正規例 |
| [07-registry-and-tools.md](07-registry-and-tools.md) | 同梱registryと標準4 toolの具体契約 |
| [08-install-runtime.md](08-install-runtime.md) | install、selection、shim、runtime、uninstall |
| [09-platform.md](09-platform.md) | Windows/Linux、PATH、shell、link |
| [10-security.md](10-security.md) | 最小security境界、artifact、path、process、log |
| [11-quality-and-ci.md](11-quality-and-ci.md) | branch/PR、test、CI matrix、version、build、release |
| [12-public-docs.md](12-public-docs.md) | 公開README・USER_GUIDEの作成仕様 |
| [13-progress.md](13-progress.md) | 実装順序、gate、証跡、停止・再開 |
| [14-maintenance.md](14-maintenance.md) | tool追加、不具合調査・修正、拡張手順、prompt |
| [15-deferred.md](15-deferred.md) | v0.1で延期した機能、再導入gate、実装prompt |

別文書の要約は上表の正本を置き換えない。同じ意味の規則に差がある場合、番号や記載順で一方を選ばず実装を停止し、仕様、fixture、test、進捗を同じ変更で同期する。enum、default、上限、失敗時動作、platform差をGo library、OS、既存コードの暗黙既定値で補わない。

## 4. v0.1の固定範囲

- 正式targetは`windows/amd64`と`linux/amd64/glibc`。
- 標準toolは`go`、`node`、`python`、`dotnet`の4件。
- modeは`portable`と`user`。
- CLIは9 command。UIはCLIだけ。
- 標準registryだけを扱い、local definition、local script、pluginを提供しない。
- schemaは4 toolのWindows/Linux導入に必要な機能だけを持つ。未使用のbackend、helper、hook、署名方式を先行実装しない。

## 5. 固定された製品判断

### 5.1 versionと選択

- tool入力はcatalogにある正規完全versionだけを認める。部分版、range、wildcard、近似候補、自動補正を拒否する。
- 導入時だけ`--latest`を認め、stableかつlifecycleがEOLでない最新版を完全版へ解決してPlanで確認する。
- 初心者向け標準操作は`setup`、完全版`install`、完全版`use`、`current`の明示的な4操作とする。`install`は`--use`なしで選択を変更しない。
- prereleaseまたはlifecycle=EOLも完全版指定なら導入できるが、状態をPlanの重要警告として表示する。channelとlifecycleを分け、unknownも明示する。
- 存在しない版は`available`を案内して失敗し、似た別版を提示・選択しない。
- `.gdtvm.toml`へ保存するversionも完全版だけとする。

### 5.2 modeとpath

- 手動archive展開は`portable`、公式bootstrap scriptは`user`を既定にする。
- Windows user data rootはKnown Folder API、Linux user data rootはOS user lookupで得たhomeの`.local/share/gdtvm`とする。Linuxの`HOME`とXDG環境変数で置換しない。
- setup後にportable rootのdirectoryを移動する運用はv0.1で非対応とし、移動時は`gdtvm setup`の再実行を案内する。
- tool home、設定、package、cacheはgdtvm管理領域へ隔離する。設定操作は`go env -w`、`npm config`、`pip config`、`venv`等の上流標準interfaceを使う。
- 共有範囲と削除はtool definitionのtyped storage宣言で決める。設定/content cacheとGoのglobal binaryはtool単位でuninstall後も保持し、Node global packageとPython site-packagesは完全version単位で対象versionと一緒に削除する。project venv/sourceは変更しない。

### 5.3 配布とregistry

- 標準定義は同一repositoryの`/registry/`で管理しclient releaseへ同梱する。registry単体download/update、registry専用branchを設けない。
- tool定義の修正も新しいclient versionで配布する。
- client versionはannotated tag作成時刻の日本時間による`YYYY.MM.DD.XX`とする。同日の最初は`00`、以後は既存最大通番＋1、上限は`99`で、失敗したtagも再利用しない。
- registry metadata更新でもreleaseするため、頻繁に増えるSemVer番号ではなく日付から新しさを判断できるCalVerを採用する。この形式はSemVerではなく、CalVer tagを指定する`go install`は正式導入経路にしない。
- release assetは2 client archive、canonical `checksums.txt`、`install.ps1`、`install.sh`のexactly 5件とする。
- `install.ps1`/`install.sh`はOS/arch、release、checksumを確認し、user領域への展開と対話setupを案内する。手動archive手順も維持する。

### 5.4 security

- HTTPS、OS trust store、Go標準proxy処理、path containment、archive traversal/collision/bomb防止、argv分離、credential mask、atomic commitを必須にする。
- 標準toolのartifactはupstream checksumが公開されているものだけを採用し、providerが公開したalgorithm（SHA-256またはSHA-512）での照合を必須にする。
- 公式artifactを優先し、非root・複数版・portable要件を満たせない場合だけthird-partyを採用して明示承認を求める。PythonはWindows/LinuxともAstral `python-build-standalone`を使う。
- 公式配布物でもOSI承認OSS licenseでない場合は、Planの重要要約へ`W_RESTRICTIVE_LICENSE`として表示し明示承認を求める。.NET SDKのWindows配布物が該当する。
- 署名検証、artifact lock、専用security audit log、SBOM/provenance/attestationをv0.1に設けない。security eventはsecretを除去した通常structured logへ記録する。
- 一般user・非rootで完結し、自動昇格、UAC、`sudo`、HKLM変更、system環境変数変更、system package自動導入を実装しない。

### 5.5 runtimeとintegration

- Windows通常切替はdirectory junction、Linuxはrelative symlinkを使い、tool本体を切替目的でcopyしない。
- Go製native shimを使い、外部helper executableへ依存しない。
- Windows setupは`user-path|none`を選べ、既定は`user-path`。user PATHはWin32 Registry APIで1 shim directoryだけを追加し、`setx`を使わない。Linux setupは`shell-profile|none`を選べ、現在shellだけを1回確認する。
- 長時間動作中のterminal、editor、serviceは`use`で切り替わらない。project完全版または再起動を案内する。

### 5.6 outputと将来拡張

- 通常CLIはdownload量、速度、残り時間、現在段階をprogress表示する。停止しているように見せない。
- `--json`は読取り専用5 commandだけが持ち、完了時に単一JSONをstdoutへ出す。progress/logはstderrへ出す。状態変更commandの機械判定は終了codeで行う。
- Planは完全版・platform・provider・checksum・channel・lifecycle・警告を冒頭と確認直前へ目立つ要約として出し、その間にURL、license、argv、書込み先等の全詳細を表示する。
- global `gdtvm.toml`は任意で、組込み安全既定値だけでも動作する。利用者が調整する少数keyだけを公開し、内部上限は組込み固定にする。
- 表示言語は日本語だけとする。message ID機構は保持し、後から言語を追加できる形を維持する。

### 5.7 branchとrelease flow

- 統合方向は`<agent>/feature-<task-id>-<slug> → <agent>/work → develop/work → main`だけとする。agentは`claude|codex`で、featureは各agent workから作る。
- feature→agent workはsquash merge、agent work→develop→mainはmerge commitとする。長期branch間でsquash/rebase mergeを使わない。
- feature PRは両OSの`lint/unit/policy`、develop以降は両OSの全CI jobを必須とする。required approvalは0件だが、PR、最新base、CI、conversation解決、指定maintainerのmergeを必須にする。
- main、develop、両agent workを保護する。mainはforce-push・削除の例外なしとし、develop/agent workのrebase・再作成だけを指定maintainerの例外操作にする。
- release PR作成からtag起動CIによる公開完了までagent work→developのmergeを凍結し、release対象commitを固定する。公開後は作業中branchをrebaseし、非作業中branchを最新main/developから再作成する。
- repository再作成時はGitHub生成のREADME、LICENSE、`.gitignore`を持つmain初期commitからdevelopを作り、現在dataをdevelopへ登録した後、両agent workを作る。

命名grammar、作業中判定、保護例外、初期登録、CI gate、tag/releaseの正確な手順は[11-quality-and-ci.md](11-quality-and-ci.md)§5.2～§5.6、§13を正とする。

## 6. 完了判定

v0.1は次をすべて満たした場合だけ完成とする。

1. 全CLIがlocale-neutralなApplication Serviceを経由し、CLIにdomain、filesystem、network、process判断がない。
2. CI matrix（`ubuntu-latest`＋`windows-latest`）で、Go・Node.js・Python・.NET SDKのcatalog、完全版install/use/current/installed/uninstall、shim実行がgreenになる。
3. strict definition/config/state parser、receipt、catalog、Plan、上限、negative security testが成功する。
4. `doctor`と`doctor --report`が、導入状態と失敗状況をsecret除去済みで報告できる。
5. `install.ps1`と`install.sh`のchecksum付きbootstrapがCIで検証できる。
6. [12-public-docs.md](12-public-docs.md)のREADME/USER_GUIDEが実装済み範囲と一致する。
7. [11-quality-and-ci.md](11-quality-and-ci.md)の利用者確認チェックリストが提示され、未確認項目が明示されている。

## 7. 仕様変更手順

observable behavior、CLI、schema、state、registry、security、releaseを変える場合は、該当仕様、fixture、test、公開文書仕様、[13-progress.md](13-progress.md)を同じ変更で更新する。未決事項を実装者の判断だけで挙動へ追加しない。判断が必要な場合は[14-maintenance.md](14-maintenance.md)の選択肢提示形式を使い、質問を1件ずつ行う。
