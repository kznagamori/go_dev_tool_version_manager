# 設定仕様

## 1. 設定ファイル

| 種類 | file | 用途 |
|---|---|---|
| distribution/global | 実行fileと同じdirectoryの`gdtvm.toml` | mode、path、network、update、security、runtime、shell、platform、log、上限 |
| project | `.gdtvm.toml` | projectの完全version選択とdisabled |
| local tool definition | 利用者指定directoryの`*.toml` | 標準外toolまたは標準定義override |

release archiveは既定値をすべて明記した`gdtvm.toml`を同梱する。別のbootstrap file、OS user config、registryから取得するconfigを設けない。portable/user/multi-userのいずれも実行file隣の同じfileを正本とする。

UTF-8、BOMなし、LFを正規形とする。読込みはUTF-8 BOMとCRLFを許容する。keyはcase-sensitive。`gdtvm.toml`と`.gdtvm.toml`は各1 MiBをhard maximumとし、parse前に超過を拒否する。未知keyはerrorとし、`[extensions.<vendor>]`だけを将来拡張用に許す。

`vendor`は正規表現`^[a-z][a-z0-9-]{0,31}$`とし、そのtable以下はTOML scalar、array、tableを保持できる。schema 1 clientはextension内容を解釈せず、動作・security policy・path・network・tool選択へ影響させない。編集時はraw内容を保持し、extension部分だけで64 KiBを超えるfileを拒否する。将来extensionを機能化する場合はschemaとtrust境界を先に改訂する。

`self-update`は既存`gdtvm.toml`を上書きしない。新releaseで追加された省略可能keyはbuilt-in既定値を用い、`doctor`で追加項目と既定値を案内する。新しい必須keyが必要な破壊変更は自動移行plan、backup、確認を提供し、未確認の既存fileを書き換えない。

## 2. 設定優先順位

同じ意味の値は高い順に次を採用する。

1. CLI option
2. project設定で許可されたtool選択
3. 実行file隣の`gdtvm.toml`
4. built-in既定値

gdtvm固有環境変数を一般設定のoverrideに使用しない。唯一の例外は`mode="multi-user"`かつ`paths.allow_user_home_env=true`の場合の`GDTVM_USER_HOME`である。標準proxy環境変数とmanaged toolへ渡すruntime環境は7章の例外規則に従う。

## 3. `gdtvm.toml`完全schema

release同梱fileは次を正規既定値とする。

```toml
schema = 1

[application]
mode = "portable"                # portable | user | multi-user
language = "auto"                # auto | ja | en
color = "auto"                   # auto | always | never

[paths]
allow_user_home_env = false
user_home_env = "GDTVM_USER_HOME"
user_home = ""

[project]
search_beyond_vcs_root = false

[updates]
repository = "https://github.com/kznagamori/go_dev_tool_version_manager"
auto_check = true
check_interval = "24h"
retain_previous = true

[definitions]
local_dirs = []
precedence = "local"             # local | registry

[network]
offline = false
connect_timeout = "15s"
request_timeout = "10m"
max_redirects = 10
retry_count = 3
user_agent_suffix = ""

[downloads]
max_parallel = 3
retain = false
resume = true
cache_max_bytes = 10737418240

[security]
local_definition_policy = "prompt"      # prompt | deny | allow
unverified_artifact_policy = "prompt"   # prompt | deny
allow_insecure_http = false
allow_network_roots = false
allow_shell_hooks = true

[runtime]
auto_install_on_use = true
prefer_current_link = true
inherit_parent_environment = true

[shell]
setup_cmd = true
setup_powershell = true
setup_pwsh = true
setup_bash = true
setup_zsh = true
setup_fish = true

[windows]
current_link = "junction"        # junction | shim-only
shim_link = "auto"               # auto | hardlink | copy-small-file
allow_small_shim_copy = true

[logs]
level = "info"                   # error | warn | info | debug | trace
max_files = 5
max_bytes_per_file = 5242880

[limits]
definition_bytes = 2097152
catalog_response_bytes = 33554432
release_metadata_bytes = 2097152
release_archive_bytes = 1073741824
artifact_bytes = 68719476736
archive_entries = 1000000
extracted_bytes = 137438953472
single_extracted_file_bytes = 68719476736
compression_ratio = 1000
process_output_bytes = 8388608
```

### 3.1 application・path

- `mode`は保存場所を決め、CLIの`--mode`でその実行だけoverrideできる。
- `gdtvm setup --mode <mode>`のcommand固有optionだけは、確認後に`application.mode`を隣接fileへ永続化する。global `gdtvm --mode <mode> setup`は永続化しない。
- `language`は`auto|ja|en`。`auto`はOS localeから`ja`または`en`を選び、判定不能時は`en`とする。
- `color`は`auto|always|never`。`auto`は人向け出力先がterminalで、かつ`NO_COLOR`等の環境変数を参照せずgdtvm自身のterminal判定が有効な場合だけ色を使う。`--json`、redirect、pipeでは常に無効とする。
- 標準registry pathはdistribution root直下の`registry/`に固定し、設定で変更できない。testはFileSystem portで注入し、production設定へtest用pathを露出しない。
- `user_home=""`はOS既定user data rootを使う。portableとmulti-userでは空文字だけを許し、非空値を禁止する。user modeだけは空文字または現在user所有のabsolute pathを許す。共有configに全user共通の可変rootを書いて分離を壊さないためである。
- `allow_user_home_env=false`では`GDTVM_USER_HOME`が存在しても読まない。
- `allow_user_home_env=true`はmulti-user modeだけで許可し、`user_home_env`が`GDTVM_USER_HOME`以外ならschema 1では拒否する。
- `GDTVM_USER_HOME`はabsolute existingまたは作成可能directoryでなければならず、空、relative、filesystem root、distribution root、他user所有、symlink loopを拒否する。
- multi-userで環境変数が未設定の場合はOS既定user data rootを使う。環境変数を必須にはしない。

user modeは非空の`user_home`、OS既定rootの順で選ぶ。multi-userは許可された`GDTVM_USER_HOME`、OS既定rootの順で選ぶ。CLI `--home`はuser/multi-userのいずれでもさらに高い優先順位でその実行だけ上書きする。

### 3.2 update

- `repository`は公式GitHub repositoryのHTTPS URL。別repositoryへの変更はstandard updateとして扱わず、schema 1では拒否する。
- `auto_check`は通常の対話CLI起動時にmetadata確認だけを行い、自動download・自動置換しない。shim起動、`exec`、completion、`--json`、`--offline`、`--non-interactive`、非TTY、`self-update`自身では暗黙checkしない。暗黙check失敗は本来commandを失敗させずdebug logへ残し、official repository/release identityとcanonical checksum entryの確認前にversionを更新通知へ使わない。
- `check_interval`は1時間～720時間のduration。`state/update.toml`の最後にofficial release metadataとcanonical checksum entryの確認まで成功したcheck時刻から経過した場合だけ暗黙checkする。明示`self-update --check`はintervalを無視する。system clockが前回時刻より24時間以上戻った場合は時刻を破損扱いにせず、次回に1回checkしてstateを更新する。
- `retain_previous=true`は更新transaction完了まで直前releaseの実行file、registry、文書をrollback領域へ保持する。成功確認後の保持期間は08章に従う。
- official repository固定、HTTPS検証、archive SHA-256照合を無効化する設定は提供しない。

### 3.3 project・definition

- `project.search_beyond_vcs_root`はGit worktree境界を越えて親を探索するかを決める。Git外では値に関係なくfilesystem rootまで探索する。
- `definitions.local_dirs`は重複しないdirectory pathの配列。relative pathは`gdtvm.toml`の親から解決し、空文字、file、distributionの`registry/`自身、管理root外を指すsymlink loopを拒否する。存在しないdirectoryはwarningとしてskipし、作成しない。
- `definitions.precedence`は`local|registry`。衝突しないlocal definitionは値に関係なく追加する。衝突時だけ先に指定したoriginを候補とするが、local承認拒否時に別originへfallbackしない。

### 3.4 network・download・security

- durationは正整数＋`ms|s|m|h`。
- `connect_timeout`はTCP/TLS接続単位、`request_timeout`はredirectとbody受信を含む1 HTTP request全体に適用する。0と無期限を禁止する。
- HTTPSを必須とし、`allow_insecure_http=true`は承認済みlocal definitionだけに効く。
- `max_parallel`は1～16、`retry_count`は0～10、`max_redirects`は0～20。
- `user_agent_suffix`は空または印字可能ASCII 1～64 bytesとし、改行、括弧、token、pathを拒否する。
- `downloads.retain`は成功後のtool artifact保持、`resume`は同じURL・ETag・期待digestに一致する`.part`だけの再開を制御する。`cache_max_bytes`は1 GiB以上、14章のdownload cache hard maximum以下とする。現在利用中のself-update archive、進行中`.part`、導入Planが参照するcacheは上限超過でも操作終了まで削除しない。
- security緩和はprojectから変更できず、次回操作で警告・auditする。
- unverified artifactは対話時だけ承認可能で、非対話ではdeny。
- `local_definition_policy`は`prompt|deny|allow`、`unverified_artifact_policy`は`prompt|deny`。`allow`はunverified artifactには存在しない。
- `allow_network_roots=false`はUNC、mapped drive、network filesystemをdata/install rootとして拒否する。trueでもdistribution root、config、registryをnetwork上へ移せることを意味せず、tool data rootだけに適用して毎回warningを出す。
- `allow_shell_hooks=false`はlocal/standardを問わずdefinitionのshell hookをPlan前に拒否する。組込みinstall stepと明示argvのnative processはshell hookではない。
- limitsは正整数かつ14章built-in hard maximum以下。

### 3.5 runtime・shell・Windows

- `runtime.auto_install_on_use=true`は未導入の完全版を`use`した対話実行でinstall Planを提示できることを意味する。非対話では`--yes`があっても04章のpolicyを満たさなければ変更しない。`false`は対話・非対話・`--yes`を問わず`use`からの自動導入を無効化し、未導入版の`use`を`E_VERSION_NOT_INSTALLED`として`gdtvm install`を案内する。
- `runtime.prefer_current_link=true`はlink方式でjunction/symlinkを優先する。falseはcurrent linkを作らずshim direct-targetを使う。backend方式には影響しない。
- `runtime.inherit_parent_environment=true`は安全な親環境を基底にreceipt環境を重ねる。falseでもOSがprocess起動に必須とする変数とgdtvmが明示生成する変数は残す。secret mask対象を子へ追加する設定ではない。
- `shell.setup_*`は`setup`で明示shell指定がない場合の候補を制御する。現在OSに存在しないshellはskipする。すべてfalseでも`setup --skip-shell`相当にはせず、shell変更なしをPlanへ明示する。
- `windows.current_link`は`junction|shim-only`。`junction`で作成不能ならwarning付きshim-onlyへfallbackする。
- `windows.shim_link`は`auto|hardlink|copy-small-file`。`auto`は同一volume hardlinkを優先し、許可時だけsmall fallback shimをcopyする。`copy-small-file`はtool本体copyを意味しない。
- `windows.allow_small_shim_copy=false`では`shim_link=copy-small-file`をschema errorとし、`auto`のcopy fallbackも禁止する。
- gdtvmがPowerShell execution policyを変更する設定は提供しない。

### 3.6 log・limits

- `logs.level`は`error|warn|info|debug|trace`、`max_files`は1～100、`max_bytes_per_file`は1 MiB～1 GiB。
- `limits.*`は該当operationのsoft limitであり、14章hard maximumを超えられない。release metadata/archiveのlimitはself-updateにも適用する。
- bytes値は10進整数で指定し、負数、浮動小数、単位付き文字列を拒否する。

## 4. mode別path

### 4.1 portable

distribution rootを可変data rootとして使用する。実行file、config、registry、文書と、tools/state/cache等が同じtreeにある。folder全体を移動可能とする。

### 4.2 user

実行file、`gdtvm.toml`、registry、文書はdistribution rootから読む。可変dataは次へ置く。

- Windows: `%LOCALAPPDATA%\gdtvm`
- Linux: OS APIで得たuser home配下の`.local/share/gdtvm`を中心に、cacheを`.cache/gdtvm`、stateを`.local/state/gdtvm`へ分離

Windows pathはKnown Folder APIで解決する。Linux pathはOS user lookup APIから得たhomeを基準にし、`HOME`/XDG環境変数を通常のpath overrideとして読まない。owner不一致を拒否する。

### 4.3 multi-user

実行file、`gdtvm.toml`、registry、README、USER_GUIDE、LICENSEは管理者が配置するread-only共有distributionを使用する。各userのtools、shims、state、cache、logs、tmp、locksはuser modeと同じuser別rootへ分離する。

`allow_user_home_env=true`の場合だけ、Windows/Linux共通の`GDTVM_USER_HOME`でuser別rootを指定できる。shell setupは共有distributionのbinary pathとuser別shim pathを組み立てる。他userのstate、tool、cacheを探索・共有しない。

共有`gdtvm.toml`の変更は管理者の責務であり、gdtvmは書き換えない。`self-update`は共有distributionへ書込み権限がなければ実行せず、管理者に更新を依頼する。

## 5. project探索・schema

探索はcwdから親へ進み、最初の`.gdtvm.toml`だけを読む。Git worktree rootを含めて検査し、`search_beyond_vcs_root=false`ではその親へ進まない。Git外はfilesystem rootまで進む。symlink loopとroot逸脱を拒否する。

```toml
schema = 1
disabled = ["rust"]

[tools]
node = "22.18.0"
python = "3.13.5"
go = "1.26.5"
```

top-levelで許可するkeyは`schema`, `disabled`, `tools`だけである。`schema`は整数1、`disabled`は正規tool IDまたはalias文字列の配列、`tools`はtool IDから完全version文字列へのtableとする。`disabled`省略は空配列、`tools`省略は空tableとして扱うが、両方空のfileも有効とする。保存時はaliasを正規tool IDへ変換し、`disabled`と`tools`内keyをそれぞれUTF-8 byte順へ整列する。

tool値は完全version文字列だけ。latest、channel、range、wildcard、配列を拒否する。同じtoolをversionとdisabledへ重複させない。aliasを正規化した結果の重複も拒否する。未知toolは`E_TOOL_UNKNOWN`とし、`self-update`または定義確認を案内する。`doctor`と`self-update`は修復のため継続できる。

## 6. 選択優先順位

1. `exec`の明示tool/version
2. project versionまたはdisabled
3. user selection
4. 未選択

projectに記載がなければuserへfallbackし、disabledはfallbackを止める。

## 7. 許可する環境変数

| 環境変数 | 条件 |
|---|---|
| `GDTVM_USER_HOME` | multi-userかつ`allow_user_home_env=true`の場合だけuser data rootとして読む |
| `HTTPS_PROXY`, `HTTP_PROXY`, `NO_PROXY` | 標準HTTP proxy設定として利用可能 |
| tool definitionが生成する`JAVA_HOME`, `GOROOT`等 | child process/runtime environmentとして生成 |

`GDTVM_HOME`, `GDTVM_MODE`, `GDTVM_CONFIG`, `GDTVM_LANG`, `GDTVM_OFFLINE`, `GDTVM_YES`, `GDTVM_NO_COLOR`, `GDTVM_LOG_LEVEL`, `GDTVM_PROJECT_SEARCH_BEYOND_VCS_ROOT`, `GDTVM_REGISTRY_REPOSITORY`, `GDTVM_LOCAL_DEFINITION_DIRS`, `GDTVM_NONINTERACTIVE`, `GDTVM_OPERATION_ID`は外部設定interfaceとして使用しない。

proxy credentialを表示・保存しない。tool runtime環境は08章のreceiptから生成し、gdtvm自身の設定overrideと混同しない。

## 8. local definition

`local_dirs`のrelative pathは`gdtvm.toml`の親から解決する。projectから追加できない。複数local directoryのID/alias衝突はerror。標準との衝突は`precedence`に従うがoriginを表示する。

未承認localを拒否した場合に標準へ黙ってfallbackしない。client`self-update`でlocal directoryを変更・削除しない。

## 9. 編集・検証

- release archiveの`gdtvm.toml`は全既定keyを含む。
- `setup`は欠落可変directoryを作成する。唯一、command固有`setup --mode`を利用者が承認した場合は、backup後に既存configの`application.mode`だけを最小差分でatomic更新できる。他fieldの整形・上書きは行わない。
- 汎用`config set` commandは初期版に設けない。
- 全commandはglobal、project、definitionの順で検証する。
- errorはfile、line、column、key path、期待値を示す。
- secretをTOMLへ直接保存するfieldを設けない。
