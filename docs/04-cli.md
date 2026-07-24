# CLI仕様

## 1. 基本構文

```text
gdtvm [global-options] <command> [command-options] [arguments]
```

コマンド名、tool ID、option名は英小文字を正規形とする。Windowsでもversion文字列とファイルパス以外はcase-sensitiveとして扱い、誤入力を早期検出する。alias tool IDは受理後に正規IDを表示する。

## 2. グローバルオプション

| オプション | 説明 |
|---|---|
| `-h, --help` | ヘルプ。command後でも利用可 |
| `--version` | CLI自身の版だけを1行表示 |
| `--lang ja|en` | 表示言語 |
| `--json` | stdoutを単一JSON documentまたはNDJSON eventにする |
| `--quiet` | 成功情報とprogressを抑制。warning/errorは残す |
| `-v, --verbose` | debug情報。重ね指定はしない |
| `--no-color` | ANSI色を無効化 |
| `-y, --yes` | 確認にyes。ただしwarning/planは表示 |
| `--non-interactive` | 入力待ち禁止。安全でない既定値は拒否 |
| `--offline` | ネットワーク禁止 |
| `--home <path>` | この実行だけ管理ルートを上書き |
| `--mode portable|user|multi-user` | この実行だけのmode。`setup`のcommand固有optionとは別 |
| `--project-file <path>` | project探索をせず指定ファイルを使う |
| `--no-project` | project選択を無視する |
| `--project-search-beyond-vcs-root` | この実行だけGit境界を越える |

`--json` では人向け文字列をstdoutへ混ぜない。progressを必要とする長時間操作はNDJSONをstdoutへ出し、各行に `type`, `operation_id`, `timestamp`, `data` を持たせる。診断用ログはstderrとする。

出力形式はcommand/optionから決定でき、実行途中で単一JSONからNDJSONへ変えない。

- 単一JSON: `tools`, refreshなし`available`, `installed`, `current`, 通常`doctor`, `version`。
- NDJSON: `setup`, `refresh`, `install`, `uninstall`, `use`, `disable`, `self-update`, `repair`, `available --refresh`, `doctor --deep`。
- `completion` はshell scriptそのものをstdoutへ出すため `--json` と併用不可。
- `exec` は子processのstdioと競合するため `--json` と併用不可。

`--quiet` は`--json`時に意味を持たず、JSON/NDJSONを省略しない。`--version` は他command、`--json`、`--quiet`と併用せず、併用時はusage errorとする。`--project-file`と`--no-project`の同時指定はusage errorである。`--home`はuser/multi-userの可変data rootだけを上書きし、portable modeとの併用はusage errorとする。`setup`との併用は`--skip-shell`を指定した一時directory作成・診断だけに許し、永続shell integrationを作らない。

グローバルオプションの正規位置はcommandより前とする。`-h, --help`だけは対象commandの直後にも置ける。初心者が最初に使う`setup`に限り、`--mode`をcommand optionとしても受け付ける。command固有`setup --mode`は隣接`gdtvm.toml`の`application.mode`を永続変更するため、グローバル`--mode`との同時指定は値が同じでもusage errorとする。その他のグローバルオプションをcommand固有optionの位置へ黙って移動解釈しない。

## 2.1 コマンド名の再検討結果

旧名の機械的移植ではなく、初心者が読む動詞、一般的な開発CLIとの整合、対象範囲の誤解しにくさで再評価した。採用判断は次のとおりである。

| 採用名 | 比較した候補 | 採用理由 |
|---|---|---|
| `setup` | `init` | directory初期化だけでなくregistry、shim、shell統合まで扱うため |
| `tools` | `list`, `plugins` | 版一覧と区別し、設定駆動toolをplugin実行物と誤認させないため |
| `available` / `installed` | `list-all` / `list` | 取得可能版と導入済み版を名詞で明確に分けるため |
| `refresh` | `update` | tool本体の更新ではなく上流catalogの再取得であることを示すため |
| `install` / `uninstall` | `add` / `remove` | SDK管理で広く理解され、downloadだけでなく展開・検証を含むため |
| `use` | `set`, `select` | 「この版を使う」という初心者向け表現で、project/userのscopeにも共通化できるため |
| `disable` | `unset`, `clear` | project scopeでは単なるkey削除でなくuser版へのfallbackを明示的に止めるため |
| `current` | `version`, `which` | CLI自身の`version`と区別し、実体pathだけでなく有効版と由来を返すため |
| `doctor` / `repair` | `check` / `rehash` | 診断と変更を分離し、link以外の状態修復も表すため |
| `self-update` | `update`, `registry update` | client、標準定義、設定template、文書を同じrelease archiveから一括更新することを明示するため |

`list`のような多義的親commandに全一覧を階層化すると日常操作の単語数が増えるため採用しない。旧`DartVm install`等のmixed-case tool commandや旧optionを隠しaliasとして残すと、二つの構文を長期保守することになるため初期版では受理しない。移行案内はエラー時の1回の提案と製品要求の対応表で行う。

## 3. コマンド一覧

### 3.1 `setup`

```text
gdtvm setup [--mode portable|user|multi-user] [--shell <name>...] [--skip-shell]
gdtvm setup --remove [--shell <name>...]
```

管理ルート、実行file隣接設定、標準registry、shim、shell integrationを初期化する。再実行は冪等である。

- `<name>`: `cmd`, `powershell`, `pwsh`, `bash`, `zsh`, `fish`。
- オプションなしでは現在OSで検出できた対応shellを一覧表示し、変更単位ごとに確認する。
- Windows PowerShell execution policyを変更しない。現在policyでprofileを読めない場合は変更なしで警告し、管理者policyを回避しない安全な手動選択肢を案内する。
- `--remove` はgdtvm marker、生成したstartup file、shim PATH integrationを除去し、保存済みの既存値を復元する。tool本体は削除しない。
- `--yes` は通常のprofile marker追加を承認できるが、既存の非gdtvm内容を置換する判断には使わない。
- command固有`--mode`で現在値を変更する場合、変更前後のmode、distribution root、旧/新data root、既存tool/stateが自動移動されないことをPlanに表示し、対話確認または`--yes`を必須とする。
- 承認後、既存TOMLのコメントとkey順を可能な限り保持して`application.mode`だけをbackup付きatomic更新する。configへ書込み権限がないmulti-user一般利用者は変更せず、配置管理者による編集を案内する。
- mode変更後は新mode側をsetupし、旧mode側のdataを自動削除・移行しない。`--remove`とcommand固有`--mode`、`--skip-shell`は排他とする。

### 3.2 `tools`

```text
gdtvm tools [--installed] [--supported]
```

tool ID、表示名、alias、現在platform対応、definition origin、導入済み件数を表示する。`--supported` は現在platformだけ、`--installed` は導入済みだけに絞る。

### 3.3 `available`

```text
gdtvm available <tool> [--channel stable|prerelease|nightly|eol] [--refresh]
```

導入可能な完全版を新しい順に表示する。既定channelはstable。cacheなしでオンラインなら自動refresh、offlineなら `E_CATALOG_MISSING`。版ごとにplatform artifactの有無、official/third-party、検証状態を保持するが、通常表示は簡潔にする。

### 3.4 `refresh`

```text
gdtvm refresh [tool] [--force]
```

tool定義が指定する公式配布元からバージョンカタログを更新する。tool省略時は現在platform対応の全toolを更新する。ETag等があっても `--force` は再取得する。個別失敗があっても残りを継続し、最後に成功/skip/failedを集約する。成功または有効cacheによるskipが1件以上あり、かつfailedも1件以上なら終了コード11の部分成功とする。対象がすべてfailedなら、代表原因に対応する終了コードを返し、複数categoryにまたがる場合は終了コード1とする。対象0件は変更不要として終了コード0とする。

### 3.5 `install`

```text
gdtvm install <tool>@<version> [--use] [--project]
gdtvm install <tool> --latest [--use] [--project]
```

- `<tool>@<version>` は最後の`@`で分割する。version空、複数候補、部分版を拒否する。
- `--latest` と `@version` は排他。stable最新版を完全版に解決し、plan表示後に確認する。
- `--use` は導入成功後に選択する。`--project` があればproject選択、なければuser選択。
- project fileがなく `--project` を指定した場合、現在ディレクトリに新規 `.gdtvm.toml` を作ることを確認する。
- 既に正常導入済みならdownloadせず成功とする。
- third-party artifact、未検証artifact、外部hook、system prerequisiteをplanに表示する。

### 3.6 `uninstall`

```text
gdtvm uninstall <tool>@<version> [--force] [--keep-shared]
```

完全版だけを削除する。variantは初期CLIの入力概念にせず、現在OS/archで同じtool/versionに一致するreceiptを対象とする。通常は1件であり、definition更新等で複数variantが残る場合は全variantと選択状態をplanへ列挙し、一度の確認でまとめて削除する。別OS/archのreceiptは対象外とする。user選択、今回解決したproject fileの選択、または他のmanaged installから依存される場合は拒否する。任意の場所にある全project fileは索引化・走査しないため、今回の探索外に残る参照は検出保証しない。`--force` は既知参照を警告した上で選択を解除できるが、別toolを破壊する依存があればなお拒否する。shared cacheは既定で残し、toolの最後の版削除時に削除提案する。`--keep-shared` は提案を省略する。

### 3.7 `installed`

```text
gdtvm installed [tool]
```

導入済み完全版、variant、導入日時、receipt検証状態、user/project選択マークを表示する。tool省略時は全tool。破損・orphanも隠さず状態を表示する。

### 3.8 `use`

```text
gdtvm use <tool>@<version>
gdtvm use <tool>@<version> --project
```

導入済み完全版を選択する。variantは現在platformのdefinitionから決定し、そのvariantが導入済みなら採用する。definitionの既定variantが変わった後でも、同じtool/version/current OS/archに正常なreceiptが1件だけなら互換fallbackとしてそのreceiptを採用し、`current --explain`へ理由を出す。候補が複数で一意に決まらなければ、`uninstall <tool>@<version>`で旧variantを整理してから再導入する案内を出して失敗する。未導入なら次の確認を行う。

```text
node@22.18.0 は未導入です。導入して選択しますか? [y/N]
```

`runtime.auto_install_on_use=true`のとき、対話でyes、または `--yes` の場合だけinstallを連続実行する。`--non-interactive` か非TTYで `--yes` がなければ `E_VERSION_NOT_INSTALLED`。`auto_install_on_use=false` では対話・`--yes`を問わず導入せず `E_VERSION_NOT_INSTALLED` とし `gdtvm install` を案内する。`latest`、channel、部分指定は禁止。

user選択では解決したreceiptのversion、variant、install IDをstateへ固定してcommitし、link方式ならcurrent link、backend方式ならselector snapshotを更新し、続いてshim indexを更新する。後のdefinition更新で別variantへ自動読み替えない。project選択では最寄り `.gdtvm.toml` を最小差分で更新し、tool本体やglobal user選択を変更しない。

### 3.9 `disable`

```text
gdtvm disable <tool>
gdtvm disable <tool> --project
gdtvm disable --all [--project]
```

user選択を削除する。`--project` ではprojectのversionを削除し、userへフォールバックさせず明示的に無効化するため `disabled` へ追加する。`--all` は旧全unset機能であり、対象一覧を表示し確認する。user scopeの`--all`は現在の全user選択を削除する。project scopeの`--all`は、Plan生成時点で標準registryと承認済みlocal definitionから解決できる全正規tool IDをsnapshotし、projectの`[tools]`を空にして、そのID集合を`disabled`へ保存する。後から追加されたtoolを暗黙に無効化するwildcardは保存しない。対象project fileがない場合の作成規則、同時編集検出、確認規則は`use --project`と同じとする。

### 3.10 `current`

```text
gdtvm current [tool] [--explain]
```

有効な完全版と由来（project/user/explicit/disabled）を表示する。tool省略時は全tool。`--explain` は探索したproject path、fallback、payload、definition originを表示する。

### 3.11 `self-update`

```text
gdtvm self-update [--version <client-version>] [--check]
```

公式GitHub Releasesから現在OS/architectureに一致するrelease archiveと`checksums.txt`を取得し、対象archiveのSHA-256を照合する。version省略は現在版より新しい最新client release、`--version`は完全な`YYYY.mm.DD.XX`だけを認める。`--check`はrelease metadataとchecksum entryの確認だけでarchive download・変更を行わない。

archiveにはclient、既定`gdtvm.toml`、registry、README、USER_GUIDE、LICENSEが含まれる。既存`gdtvm.toml`は上書きせず、registryと文書をclientと同じreleaseへ更新する。tools、user/project selection、state、catalog、local definition、shared cacheを変更しない。

Planにcurrent/new version、official repository、release/tag/asset ID、target asset、size、checksum、置換path、config保持、restart案内を表示する。自動downgradeを禁止し、古いversionの明示指定もschema 1では拒否する。

Linuxは同一volumeのstagingからatomic renameする。Windowsは新しい`gdtvm.exe`をtemporary update directoryから子processとして起動し、親終了を待ってdistribution fileを置換する。完了後にclientを自動再実行せず、利用者へ再実行を案内する。置換失敗時は旧releaseを復元し、混在releaseを起動可能状態にしない。

multi-userのread-only共有distributionでは書込み権限がなければ変更せず、配置管理者による実行を案内する。

### 3.12 `doctor`

```text
gdtvm doctor [--tool <tool>] [--deep]
```

読取り専用診断。`--deep` は重要ファイルhash、外部配布元到達、各exposed commandのversion probeも行う。項目ごとに `ok`, `warning`, `error`, `skipped` と修復可否を返す。

必須診断項目は、home書込み、mode、state schema、同梱registry schema/tree、definition互換、catalog、lock、残存journal、download一時物、link方式のlink能力/current一致、backend方式のinventory/selector一致、shim index、shell marker、PATH順位、project設定、receipt、disk容量、system prerequisite、VS Code再起動注意である。

### 3.13 `repair`

```text
gdtvm repair [--tool <tool>] [--dry-run]
```

doctor結果から安全な修復planを作る。link方式のcurrent link、backend receipt/index、shim hardlink/index、state backup復元、stale staging、移動後の相対path、shell markerの重複を対象にする。tool payloadの再ダウンロードやローカル定義の再承認は自動で行わない。`--dry-run` はplanのみ。

### 3.14 `exec`

```text
gdtvm exec <tool>@<version> [<tool>@<version> ...] -- <command> [args...]
gdtvm exec -- <command> [args...]
```

明示tool版を一時環境へ重ね、`--`以降を起動する。tool省略時は現在ディレクトリの有効選択一式を使う。状態、current link、project fileを変更しない。

同じtoolの重複、依存版競合、未導入版は実行前に失敗する。Windowsでproject固有の環境変数をVS Code本体にも確実に継承させる高度な使用例は `gdtvm exec -- code.exe .` とする。

### 3.15 `completion`

```text
gdtvm completion cmd|powershell|pwsh|bash|zsh|fish
```

shellに適したcompletion定義をstdoutへ出す。cmdは可能な範囲の補助またはdoskey macroを出す。動的候補取得はネットワークへ接続せず、registry/catalog cacheだけを読む。

### 3.16 `version`

```text
gdtvm version [--short]
```

CLI版、commit、build time、Go版、platform、state schema、tool/helper definition schema、同梱registry schema/tree revisionを表示する。`--short` とグローバル `--version` はクライアント版 `YYYY.mm.DD.XX` だけを出す。

## 4. 共通入力規則

- version前後の空白はtrim後に拒否し、黙って意味を変えない。
- Unicode look-alikeのhyphen、at signを拒否し、ASCII表記を案内する。
- `@`を含むversionを定義が必要とする場合は将来の別構文まで非対応とする。
- tool aliasは受けるが、出力、state、projectは正規IDにする。
- pathはOSの規則で解決し、表示時は利用者入力とcanonical pathを必要に応じ併記する。

## 5. 対話規則

確認promptの既定は原則Noとする。次は `--yes` があっても警告本文を必ず表示・監査する。

- third-party portable build
- SHA-256以上で検証できないartifact
- shell hook
- local definitionの初回/変更後実行
- current選択や依存を破壊するforce削除
- 既存shell profile/AutoRunへの変更

third-party警告には、非公式であること、source/repository URL、license、対象version/platform、checksum/signature状態、公式portable artifactを使えない理由を含める。

非TTY、`CI=true`、`--non-interactive` のいずれかでは非対話とする。`--yes` は通常確認を承認するが、unverified artifactは設定policyが`prompt`の場合でも非対話では拒否する。

## 6. stdout/stderr

- 正常結果と要求されたデータ: stdout
- warning、error、debug、対話prompt: stderr
- 起動した子プロセス: そのstdioを直接継承
- progress: TTY時stderrの一行更新、非TTY時は節目ごとの行、JSON時はevent
- secretを含むURL/argv/envはマスク

## 7. 終了コード

| code | 意味 |
|---:|---|
| 0 | 成功、または変更不要 |
| 1 | 一般的操作失敗 |
| 2 | CLI usage/構文エラー |
| 3 | 設定/definition schemaエラー |
| 4 | tool/version/platformが見つからない |
| 5 | network/offline/cache不足 |
| 6 | digest/signature/security policy違反 |
| 7 | 権限、link、filesystem失敗 |
| 8 | 外部process/hook失敗 |
| 9 | lock競合/timeout |
| 10 | 操作cancel |
| 11 | 部分成功 |
| 12 | doctorでerror検出 |

`E_*` code（[10-internal-api.md](10-internal-api.md)9節）と終了コードの対応は次のとおりとする。code 0はerrorがない成功・変更不要、code 12はdoctorが診断結果にerror項目を検出した場合であり、いずれも`E_*`を持たない。

| 終了コード | 対応する `E_*` |
|---:|---|
| 1 | `E_STATE_CORRUPT`, `E_PLAN_STALE`, `E_COMMAND_AMBIGUOUS`, `E_UPDATE_FAILED`, `E_DEPENDENCY_CONFLICT`、および上記以外の一般的操作失敗 |
| 2 | `E_USAGE` |
| 3 | `E_CONFIG_PARSE`, `E_CONFIG_SCHEMA`, `E_REGISTRY_INVALID`, `E_DEPENDENCY_CYCLE` |
| 4 | `E_TOOL_UNKNOWN`, `E_VERSION_INVALID`, `E_VERSION_NOT_FOUND`, `E_VERSION_NOT_INSTALLED`, `E_PLATFORM_UNSUPPORTED` |
| 5 | `E_CATALOG_MISSING`, `E_OFFLINE`, `E_NETWORK` |
| 6 | `E_DIGEST_MISMATCH`, `E_SIGNATURE_INVALID`, `E_POLICY_DENIED`, `E_ARCHIVE_UNSAFE`, `E_DEFINITION_UNAPPROVED` |
| 7 | `E_HOME_NOT_WRITABLE`, `E_LINK_FAILED` |
| 8 | `E_PROCESS_FAILED` |
| 9 | `E_LOCKED`（lock競合とlock待機timeoutを含む） |
| 10 | `E_CANCELLED` |
| 11 | `E_PARTIAL` |

`E_TIMEOUT` は原因の文脈で決め、network起因は5、外部process/hook起因は8とする。code 9の「timeout」はlock待機のtimeoutを指す。上表にないcodeを追加する場合は同じ変更で本表へ登録する。

`exec` はgdtvmが子を正常起動した場合、子プロセスの終了コードをそのまま返す。0～12と衝突しても子の値を優先し、`--json`は禁止のため再mappingしない。起動前失敗だけ上表を使い、verbose logではchild開始済みかを構造化fieldで判別できるようにする。

## 8. ヘルプとエラー

エラーは次の順で表示する。

1. 何が失敗したか
2. 対象tool/version/path
3. 安全な修正方法を1～3件
4. 詳細error codeとoperation ID

typoには編集距離が近い既知command/toolを1件だけ提案する。自動実行や黙ったalias追加をしない。
