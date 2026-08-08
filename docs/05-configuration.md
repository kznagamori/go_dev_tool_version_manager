# 設定仕様

## 1. 方針

global `gdtvm.toml`は任意である。存在しない場合も組込み既定値で全通常操作が動く。release archiveは設定fileを必須にせず、comment付きsampleを文書へ載せる。

公開設定は利用者が継続的に調整する項目だけにする。archive安全上限、redirect上限、schema bytes、path component等の内部制限は[04-storage-and-data.md](04-storage-and-data.md)§21の組込み値とし、先回りして設定keyを公開しない。高度設定を追加する場合は[14-maintenance.md](14-maintenance.md)の設定schema拡張手順に従う。

設定はUTF-8 BOMなしTOML 1.0。unknown key、重複key/table、型違い、enum外、上限外を位置付き`E_CONFIG_INVALID`として拒否する。暗黙変換、deprecated alias、環境変数によるgdtvm設定上書きを行わない。

## 2. 発見と優先順位

active distribution rootの`gdtvm[.exe]`と同じdirectoryにある`gdtvm.toml`だけをglobal設定として読む。current directory、user home、registry、networkから別設定を探索しない。

値の優先順位は次のとおり。

1. commandで明示した一時option。
2. project `.gdtvm.toml`が規定するtool selectionだけ。
3. global `gdtvm.toml`の明示key。
4. setup stateに保存したmode/integration。
5. 組込み既定値。

projectはsecurity、network、root、log、timeoutを変更できない。global fileの変更を長時間operation中に検出した場合、古いsnapshotで続行せず`E_PLAN_STALE`。

## 3. global schema 1

全keyを明示した例を示す。各table/keyは任意で、省略時は右コメントの既定値を使う。

```toml
schema = 1

[application]
color = "auto"                  # auto | always | never

[paths]
user_data_root = ""             # user modeだけ。空はOS既定

[project]
filename = ".gdtvm.toml"
stop_at_vcs_root = true

[network]
connect_timeout = "30s"
request_timeout = "5m"

[download]
cache_max_bytes = 10737418240

[runtime]
auto_install_on_use = true

[logs]
level = "info"                  # error | warn | info | debug | trace
max_files = 5
max_bytes_per_file = 5242880
```

許可するtop-level keyは`schema`, `application`, `paths`, `project`, `network`, `download`, `runtime`, `logs`だけ。`schema=1`はfile存在時に必須。

### 3.1 application

- `color=auto|always|never`。autoはstderr/stdoutごとのTTY能力で決める。
- 表示言語はv0.1で日本語固定とし、設定keyを設けない。
- modeとPATH integrationはsetup stateが正本。永続変更は`setup --mode`、`setup --path-integration`を使い、手編集keyを設けない。

### 3.2 paths

`user_data_root`は空または現在user所有のabsolute directory。user modeだけに許し、portableでは非空を拒否する。filesystem root、distribution root、network share、他user所有、symlink/reparse loopを拒否する。Linuxでも明示configだけはOS既定home以外を選べる。`HOME`/XDGはこの値を暗黙設定しない。

### 3.3 project

- `filename`はschema 1で`.gdtvm.toml`固定。他値を拒否する。将来keyを増やしやすい型を維持するためfieldとして存在する。
- `stop_at_vcs_root=true`は探索開始directoryが属する最寄りGit worktree rootを越えない。falseはfilesystem rootまで探索する。
- CLI `--project-search-beyond-vcs-root`はその実行だけfalse相当。

### 3.4 network

- durationはGo duration grammarの正値。connect 1s～5m、request 10s～1h。
- proxyはGo toolchain固定versionの`net/http.ProxyFromEnvironment`契約を使用し、global config keyを設けない。
- OS trust storeを使う。TLS検証無効、HTTP許可、任意CA bundle、credential headerを設定するkeyはschema 1にない。

### 3.5 download

- 1 artifactはstreamで処理し全量memoryへ載せない。downloadは逐次実行で、並行download設定はv0.1に存在しない（[15-deferred.md](15-deferred.md) D-26）。
- `cache_max_bytes`は1 GiB～1 TiB。
- 中断したdownloadは再開せず、次回実行時に最初から取得し直す（[15-deferred.md](15-deferred.md) D-24）。
- retry count、redirect count、artifact/extract hard maximumは[04-storage-and-data.md](04-storage-and-data.md)§21で固定。

### 3.6 runtime/logs

- `auto_install_on_use=false`では未導入versionの`use`は常に`E_VERSION_NOT_INSTALLED`。
- log levelは表のenum。file logは`logs/`にUTC structured lineで保存し、secret maskする。
- `max_files`は1～100、`max_bytes_per_file`は1 MiB～1 GiB。
- 専用security audit logとaudit設定keyは存在しない。

## 4. project `.gdtvm.toml`

```toml
schema = 1

[tools]
go = "1.26.5"
node = "22.18.0"
python = "3.13.5"
```

top-levelは`schema`, `tools`だけ。`schema=1`必須。`tools` keyは正規tool IDだけを受け、aliasを保存したfileは`E_PROJECT_CONFIG_INVALID`とする。CLIへaliasを入力した場合はregistryで正規IDへ変換してから新規作成・更新する。値はcatalog正規完全versionだけ。latest、channel、range、配列、provider、storage設定を保存しない。未知toolは通常operationで`E_TOOL_UNKNOWN`、`doctor`は診断を継続する。

project単位でtoolを無効化する`disabled`配列はv0.1に存在しない（[15-deferred.md](15-deferred.md) D-04）。

### 4.1 探索

1. `--project-file`があればそのfileだけ。
2. `--no-project`なら探索なし。
3. canonical current directoryから親へ`.gdtvm.toml`を探索。
4. 既定では最寄りGit worktree rootで停止。`.git` directory/fileの両方を認識する。
5. 最初に見つけた1件だけを使う。複数mergeしない。

symlink loop、permission error、競合caseを明確に失敗させる。project fileの更新はraw bytes digestをPlanへ固定し、確認後に別process変更があれば`E_PLAN_STALE`。

## 5. tool設定

標準toolの固有設定を`gdtvm.toml`へ複製しない。definitionのtyped storageとenvironment profileが上流設定file/cacheの管理root内pathを確定し、利用者は次の上流interfaceを使う。

- Go: `go env -w/-u`。
- Node.js: `npm config`と`.npmrc`。
- Python: `pip config`、`python -m venv`、`python -m pip`。
- .NET SDK: `dotnet nuget`、`dotnet tool`、`NuGet.Config`。

definitionは設定storageをtool scope、runtime-bound packageをversion scope等として宣言する。設定内容をgdtvmがparse・変換・migrationしない。上流が設定形式を変更した場合もdefinitionのpath/environment変更で対応し、必要なら[14-maintenance.md](14-maintenance.md)に従ってschemaを拡張する。

上流が管理rootへ隔離する公開手段を持たない領域は、typed storageとして宣言せず[12-public-docs.md](12-public-docs.md)で管理外と明示する。.NETのuser-level `NuGet.Config`と`dotnet tool install -g`の配置先が該当する。

## 6. 許可する環境変数

| 環境変数 | 用途 |
|---|---|
| Go標準proxy環境変数 | `ProxyFromEnvironment`が解釈するproxy/NO_PROXY |
| `NO_COLOR` | color auto時の抑制 |
| tool definitionが設定する`GOROOT`, `GOPATH`, `GOBIN`, npm/pip関連, `DOTNET_*`/`NUGET_*`等 | managed child runtime |

`GDTVM_HOME`, `GDTVM_MODE`, `GDTVM_CONFIG`, `GDTVM_YES`, `GDTVM_OFFLINE`, `GDTVM_REGISTRY_*`、credential header変換用変数を外部設定interfaceとして使用しない。CLI optionまたはTOMLを正とする。

## 7. 編集

schema 1のCLIはglobal fileを自動編集しない。mode/PATH integrationはsetup state、project selectionは`.gdtvm.toml`へ保存する。将来global設定を変更するcommandを追加する場合、存在しなければ最小fileを作成し、存在すればcomment/key順を可能な限り保ち、対象key以外をformatし直さずbackup、temporary、strict reparse、atomic replaceを行う。

設定key追加は[14-maintenance.md](14-maintenance.md)の手順で、意味、default、範囲、platform差、既存fileの挙動、unknown key、positive/negative fixture、documentationを同時に追加する。実装がまだ読まない予約keyを先に公開しない。
