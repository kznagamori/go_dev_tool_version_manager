# 設定仕様

## 1. 設定ファイルの種類

| 種類 | ファイル | 用途 |
|---|---|---|
| bootstrap | 実行ファイル隣の `gdtvm.bootstrap.toml` | mode locatorだけ |
| global | portableの `config/gdtvm.toml` またはOS設定領域 | 全体ポリシー |
| project | `.gdtvm.toml` | プロジェクトの完全版選択だけ |
| tool definition | `*.toml` | ツール処理定義。別仕様 |

UTF-8、BOMなし、LFを標準とする。読込み時はUTF-8 BOMとCRLFを許容する。キーはcase-sensitiveである。未知キーはtypo防止のため既定でエラーとし、`[extensions.<vendor>]` 配下だけを将来拡張用に許可する。

## 2. 設定優先順位

同じ意味の値は高い順に次を採用する。

1. CLIオプション
2. `GDTVM_*` 環境変数
3. project設定（許可されたtool選択だけ）
4. global設定
5. built-in既定値

project設定はnetwork、security、registry、project search自体を変更できない。

## 3. グローバル設定スキーマ

完全な設定例を示す。省略値はコメントに頼らず以下の値を既定とする。

```toml
schema = 1

[application]
language = "auto"               # auto | ja | en
color = "auto"                  # auto | always | never

[project]
search_beyond_vcs_root = false

[registry]
repository = "https://github.com/kznagamori/go_dev_tool_version_manager"
tag_prefix = "registry-v"
auto_bootstrap = true
auto_update = true
update_interval = "24h"
keep_snapshots = 2

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
github_token_env = "GITHUB_TOKEN"

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
setup_windows_powershell = true
setup_pwsh = true
setup_bash = true
setup_zsh = true
setup_fish = true

[windows]
current_link = "junction"        # junction | shim-only
shim_link = "auto"               # auto | hardlink | copy-small-file
allow_small_shim_copy = true
change_execution_policy = false

[logs]
level = "info"                   # error | warn | info | debug | trace
max_files = 5
max_bytes_per_file = 5242880

[limits]
manifest_bytes = 2097152
definition_bytes = 2097152
catalog_response_bytes = 33554432
artifact_bytes = 68719476736
archive_entries = 1000000
extracted_bytes = 137438953472
single_extracted_file_bytes = 68719476736
compression_ratio = 1000
process_output_bytes = 8388608
```

### 3.1 値制約

- durationは正の整数＋`ms`, `s`, `m`, `h` の単位とする。
- URLはHTTPSを必須とする。`allow_insecure_http=true` はローカル定義だけに効き、標準registryには効かない。
- `max_parallel` は1～16、`retry_count` は0～10、`max_redirects` は0～20。
- `github_token_env` は環境変数名だけを保存し、token値を保存しない。
- `[limits]` は正の整数で、built-in最大値以下だけを許す。security上限を増やす変更はlocal/standard definitionから行えない。
- `local_definition_policy=allow` でも警告と監査ログを省略しない。
- `unverified_artifact_policy=prompt` は対話時だけ承認可能で、非対話時はdenyとして扱う。
- `change_execution_policy` は既定falseであり、trueでも `setup` は変更内容を事前表示して個別承認を得る。

## 4. プロジェクト検索

探索開始はコマンドの現在ディレクトリとする。shimではshimプロセスの現在ディレクトリを使う。

1. 現在ディレクトリに `.gdtvm.toml` があれば採用する。
2. なければ親へ一階層ずつ進む。
3. Git worktreeでは `.git` ファイルまたはディレクトリのあるrootを含めて検査し、既定ではその親へ進まない。
4. Git外ではfilesystem rootまで進む。
5. `search_beyond_vcs_root=true` の場合だけGit境界を越える。
6. 最初に発見した1ファイルだけを読み、複数階層をmergeしない。

この検索設定はglobal/CLI/envだけが変更できる。projectファイル自身に書かれていてもunknown keyとして拒否する。

symlink経由のcwdでは論理cwdを優先して探索し、各候補のrealpathが同一root方針から逸脱しないことを検査する。loopを検出する。

## 5. プロジェクト設定スキーマ

```toml
schema = 1

[tools]
node = "22.18.0"
python = "3.13.5"
go = "1.26.5"

disabled = ["rust"]
```

`[tools]` の各tool値は文字列の完全版だけを認める。`latest`、`stable`、`22`、`^22`、`>=22`、wildcard、配列を拒否する。`disabled` は予約キーで、正規tool IDの配列とする。同じtoolをversionとdisabledの両方へ記載した場合は設定エラーとする。

project設定に未知tool IDがある場合、TOML構文読込み自体は成功させ、`E_TOOL_UNKNOWN` 診断とregistry更新の案内を生成する。ただし、そのproject設定を有効入力として使う `current`, shim, install/use/exec等は未知toolを無視せず失敗する。`doctor` と `registry update` だけは修復・解決のため継続できる。

## 6. 選択優先順位

ツールごとの有効版は次の順で決める。

1. `exec` で明示された `<tool>@<version>`
2. 最寄りprojectのversionまたはdisabled
3. user selectionのversion
4. 未選択

projectに当該toolの記載がなければuserへフォールバックする。projectのdisabledはuser selectionを明示的に隠す。

## 7. 環境変数

| 環境変数 | 型/意味 |
|---|---|
| `GDTVM_HOME` | 絶対管理ルート |
| `GDTVM_MODE` | `portable` / `user` |
| `GDTVM_CONFIG` | global設定への絶対パス |
| `GDTVM_LANG` | `ja` / `en` |
| `GDTVM_OFFLINE` | boolean |
| `GDTVM_YES` | boolean。確認を自動承認するが警告表示は残す |
| `GDTVM_NO_COLOR` | boolean |
| `GDTVM_LOG_LEVEL` | level |
| `GDTVM_PROJECT_SEARCH_BEYOND_VCS_ROOT` | boolean |
| `GDTVM_REGISTRY_REPOSITORY` | registry URL |
| `GDTVM_LOCAL_DEFINITION_DIRS` | OS path separator区切り |
| `GDTVM_NONINTERACTIVE` | boolean |
| `GDTVM_OPERATION_ID` | 内部の子shim連鎖用。外部入力は検証する |

booleanはcase-insensitiveな `1,true,yes,on` と `0,false,no,off` を認め、それ以外をエラーにする。

Proxyは標準の `HTTPS_PROXY`, `HTTP_PROXY`, `NO_PROXY` を尊重する。認証情報を表示・保存しない。

## 8. ローカル定義ディレクトリ

global設定の`[definitions]`でローカル定義directoryと衝突時の優先元を指定する。

```toml
[definitions]
local_dirs = ["D:/dev/gdtvm-definitions", "./definitions"]
precedence = "local"             # local | registry
```

相対パスはglobal設定ファイルの親から解決する。projectファイルからローカル定義パスを追加できない。複数のlocal directoryに同じ正規ID、または衝突するaliasがあれば宣言順で選ばず設定エラーとする。localとregistryの正規IDが衝突した場合、既定 `local` は承認済みローカルを優先するが、originをすべての一覧と計画に表示する。`registry` では標準を優先し、同IDのlocal版は一覧・診断には出すが実行に使わない。初期CLIにorigin選択optionは増やさず、切替にはglobal設定を変更する。

`precedence="local"`で選ばれたlocal定義が未承認なら承認手続を行い、拒否・非対話deny時に同名registry定義へ黙ってfallbackしない。originが変われば取得元とhookが変わるため、利用者がprecedenceを明示変更するまで失敗とする。

## 9. 設定変更と検証

- `setup` は存在しないglobal設定を既定値で作るが、既存ファイルを整形し直さない。
- CLIから設定を変更する汎用 `config set` コマンドは初期版では提供しない。概念を増やさず、setupで必要項目だけ扱う。
- 全コマンドは状態変更前にbootstrap、global、project、definitionの順で構文・意味検証する。
- エラーはファイル、line、column、key path、期待値を示す。
- security設定を緩和する変更は次回操作時に一度警告する。
- secretをTOMLに直接置くキーを定義しない。tokenは環境変数参照だけとする。
