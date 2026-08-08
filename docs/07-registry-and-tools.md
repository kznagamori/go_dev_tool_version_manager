# 標準registryと標準tool仕様

## 1. 配置と配布

標準registryのsource正本はrepository rootの`/registry/`である。client releaseはtag commitの同じtreeをarchiveへ同梱する。

- registry専用branch、registry単体release、runtime単体download/updateを作らない。
- tool追加・definition修正はclient versionを上げたreleaseで配布する。
- runtimeはdistribution rootの`registry/`だけを読む。user/project/networkから別registryを探索しない。
- local definition、override directory、pluginを提供しない。

## 2. directory

```text
registry/
  registry.toml
  tools/
    dotnet.toml
    go.toml
    node.toml
    python.toml
  schemas/
    tool-definition-v1.json
    registry-v1.json
  messages/
    ja.toml
  licenses/
    python-build-standalone-MPL-2.0.txt
```

上記以外のentryをrelease registryへ含めない。helper、key、script、local bundle directoryは存在しない。license file名はASCII kebab grammar、内容はupstream取得元とlicense identifierをregistry reviewで照合する。

## 3. `registry.toml`

```toml
schema = 1
tool_definition_schema = 1
client_min_version = "2026.08.07.00"

[[tools]]
id = "dotnet"
path = "tools/dotnet.toml"
sha256 = "<64 lowercase hex>"

[[tools]]
id = "go"
path = "tools/go.toml"
sha256 = "<64 lowercase hex>"

[[tools]]
id = "node"
path = "tools/node.toml"
sha256 = "<64 lowercase hex>"

[[tools]]
id = "python"
path = "tools/python.toml"
sha256 = "<64 lowercase hex>"
```

top-level keyは`schema`, `tool_definition_schema`, `client_min_version`, `client_max_version`, `tools`。前4件のうちmaxだけ任意。tool entryは`id`, `path`, `sha256`だけで全件必須。toolsはID ASCII byte順、exactly 4件、ID一意、path=`tools/<id>.toml`、digestはraw file bytes。この`sha256`はgdtvm自身が計算するdigestであり、upstream digestと違いalgorithm prefixを持たない。

client versionがmin未満/max超過、schema不一致、entry欠落/extra、digest不一致なら`E_REGISTRY_INVALID`。

archive全体の完全性はrelease archiveのSHA-256（`checksums.txt`）で担保する。registry treeに対する別のcanonical hashをbinaryへ埋め込まない。

## 4. runtime load

command別に必要な範囲を検証する。

- `--help|<command> --help|--version|version`: registryを読まず、binaryへ埋め込んだbuild/schema情報だけを返す。
- `doctor`: 破損箇所を診断するため読めるfileを継続する。
- `setup`: registry headerと4 definitionを必須検証し、required command集合からshim indexを作る。
- `available|install|use`: registry headerと対象definition digestを必須検証する。`use`は導入済みreceiptをactive definitionで再解釈せず、alias正規化と未導入時のauto-install Planにdefinitionを使う。
- `installed|current|uninstall`: state、receipt、indexを正本とする。正規tool IDならregistryなしで扱い、alias入力を正規化するときだけ対象definitionを要求する。
- `shim runtime`: registry/networkを読まず、strict検証済みshim index、selection、receiptだけを使う。

shim indexのclient versionが現在値と違う場合はruntimeをfail closedにし、`gdtvm setup`による再生成を案内する。shim起動中にregistryを読んでindexを書き換えない。

## 5. source validation

release前に次をすべて検査する。

1. directory/entry集合が§2と一致。
2. registry TOML、definition TOML、message/licenseがsize上限内。
3. ID/file/path/digest/schemaが一致。
4. aliasが4 tool全体で衝突しない。
5. platform tupleが`windows-amd64|linux-amd64-glibc`だけ。
6. required command、typed storage、provider、checksum、channel/lifecycleとその根拠が§7〜§10の表と一致。
7. Python third-party license textが存在する。
8. static sourceの正規version集合が両platformで完全一致する。
9. `license_notice`を宣言したplatformの`provider.license`がOSI承認OSS license識別子でなく、宣言しないplatformの`provider.license`がOSI承認OSS license識別子である。
10. `lifecycle_map`がupstream enumの全値を明示しており、live smokeで未知値が出ないこと。

registry parserはunknown key/entryを拒否する。schema JSONはTOML parser/semantic validatorの補助成果物であり、JSON Schemaだけで適合を宣言しない。

## 6. 標準tool集合と共通規則

| ID | 表示名 | provider | Windows | Linux |
|---|---|---|---|---|
| `dotnet` | .NET SDK | Microsoft公式 | amd64 | amd64/glibc |
| `go` | Go | Go project公式 | amd64 | amd64/glibc |
| `node` | Node.js | Node.js project公式 | amd64 | amd64/glibc |
| `python` | Python (CPython) | Astral `python-build-standalone` third-party build | amd64 | amd64/glibc |

その他のtool ID、unsupported placeholder、helper definitionをregistryへ入れない。追加時は[14-maintenance.md](14-maintenance.md)で上流調査、仕様、schema、definition、fixture、実装、CI検証を完了してから本章とregistryへ追加する。

- 利用者入力はcatalogに存在する完全versionだけ。部分version、range、wildcardを受けない。`--latest`だけstableかつlifecycleがEOLでない最大versionへ解決する。
- Windows/Linuxの同じ完全versionは同じtool versionとして表示するが、artifact/receiptはplatform別identityを持つ。
- version発見、artifact URL/selector、checksum、展開、command、environment、storage、probeをTOMLへ書き、Goコードにtool ID分岐を置かない。
- official artifactを優先する。Pythonだけは公式CPythonが両OSで同一の非root portable archive契約を提供しないため、採用理由を明記したthird-party artifactを使う。
- **provider checksumがあるartifactだけを採用する**。checksumを公開しないproviderのtoolはv0.1で扱わない。algorithmはproviderが公開したもの（`sha256`または`sha512`）をそのまま使う。
- 公式providerでも配布物がOSI承認OSS licenseでないplatformには`license_notice`を宣言し、Planの重要要約で明示承認を求める。.NET SDKのWindows配布物が該当する。
- archive内link、installer、MSI/EXE、shell script、helper、hook、backendを使わない。
- install probeはpayloadを変更せず、operation tmp以外へ書かない。
- required command集合は本章とdefinition/fixture/contract testで完全一致させる。
- tool設定/cache/global packageはtyped storageへ隔離し、payloadを設定保存先にしない。

catalog itemは`version`, `channel=stable|prerelease`, `lifecycle=supported|eol|unknown`, platformごとのartifact、provider build/release、digest、installable statusを持つ。上流metadataだけでEOLを確定できないversionをgdtvmが推測でEOL扱いしない。lifecycleの決め方はtoolごとに次のとおりで、いずれも上流一次資料に基づく。

| tool | lifecycleの由来 |
|---|---|
| Go / Node.js | 公式lifecycle根拠と評価日をexact versionの`lifecycle_overrides`として保守する |
| Python | `static_versions` itemが`lifecycle`と根拠を直接持つ |
| .NET SDK | 子文書top-levelの`support-phase`を`document_lifecycle_pointer`で読み、`lifecycle_map`で写像する |

## 7. Go

### 7.1 provider/version/artifact

| 項目 | 契約 |
|---|---|
| tool license | `BSD-3-Clause` |
| provider | Go project（`official`） |
| provider license | `BSD-3-Clause`（配布archiveも同一。`license_notice`は宣言しない） |
| homepage | `https://go.dev/` |
| version source | `https://go.dev/dl/?mode=json&include=all` の公式JSON |
| version | JSONの`version`からleading `go`を除いたversion。beta/rcはprerelease |
| Windows artifact | `go<version>.windows-amd64.zip` |
| Linux artifact | `go<version>.linux-amd64.tar.gz` |
| checksum | 同じJSON file entryのSHA-256（`checksum.kind="asset-field"`） |
| layout | top-level `go/`を1 component除去 |
| redirect host | `dl.google.com` |

file entryは`selector`の`name_regex`と`os`/`arch`に一致するexactly 1件を要求する。同じentryにinstallerやsource archiveが並ぶため、`name_regex`で目的のarchiveだけへ絞る。0件はそのplatformでunavailable、複数/型違い/digest不正はsource error。Goの`stable` fieldとversion prerelease表現からchannelを決め、Go公式release policyを根拠にlifecycle overrideをclient releaseごとに確認する。

### 7.2 command

required commandは次の完全な集合。

| command | target |
|---|---|
| `go` | Windows `bin/go.exe`、Linux `bin/go` |
| `gofmt` | Windows `bin/gofmt.exe`、Linux `bin/gofmt` |

probeは`go version`の完全version・OS・arch、`gofmt`起動成功を検査する。runtime environmentは`GOROOT={{payload}}`と`GOTOOLCHAIN=local`を設定し、選択した完全versionから別toolchainを暗黙download/実行させない。

### 7.3 storage

| ID | kind | scope | environment/用途 |
|---|---|---|---|
| `config` | config | tool | `GOENV=<dir>/go.env`、`go env -w` |
| `workspace` | runtime-data | tool | `GOPATH` |
| `module-cache` | content-cache | tool | `GOMODCACHE` |
| `build-cache` | build-cache | tool | `GOCACHE` |
| `global-tools` | global-bin | tool | `GOBIN`、PATH prepend |

Goでbuildしたglobal binaryはstandalone executableであるため、全managed Go versionから同じ`global-tools`を共有する。利用者は`go install pkg@version`、`go env -w`等の上流標準commandを使う。gdtvmはGOENV等の保存先だけをredirectし、設定keyを独自解釈しない。

## 8. Node.js

### 8.1 provider/version/artifact

| 項目 | 契約 |
|---|---|
| tool license | `MIT` |
| provider | Node.js project（`official`） |
| provider license | `MIT`（配布archiveも同一。`license_notice`は宣言しない） |
| homepage | `https://nodejs.org/` |
| version source | `https://nodejs.org/dist/index.json` の公式JSON |
| version | `version`のleading `v`を除いたsemver完全version |
| Windows artifact | `node-v<version>-win-x64.zip` |
| Linux artifact | `node-v<version>-linux-x64.tar.gz` |
| checksum | 同versionのdirectoryにある`SHASUMS256.txt`からexact filename 1件 |
| layout | artifact名と同じtop-level directoryを1 component除去 |
| redirect host | 追加なし（`nodejs.org`だけ） |

index entryの`files`に対象platform token（Windows=`win-x64-zip`、Linux=`linux-x64`）がなければunavailable。semver prerelease以外のchannelはstable。Node.js公式release scheduleを根拠にlifecycle overrideをclient releaseごとに確認する。checksum fileのduplicate、path付きfilename、別algorithmを拒否する。

Node.js公式はLinux向けに`.tar.gz`と`.tar.xz`を同じ`SHASUMS256.txt`で配布する。schema 1は`.tar.gz`だけを採用し、圧縮形式のための外部moduleを持たない。

index entryの`lts` fieldはschema 1のcatalog契約に格納先がないため保持しない。安定版の選択にはchannelとlifecycleを使う。

### 8.2 command

required commandは次の完全な集合。

| command | Windows | Linux |
|---|---|---|
| `node` | `node.exe` | `bin/node` |
| `npm` | `node.exe` + `node_modules/npm/bin/npm-cli.js` | `bin/node` + `lib/node_modules/npm/bin/npm-cli.js` |
| `npx` | `node.exe` + `node_modules/npm/bin/npx-cli.js` | `bin/node` + `lib/node_modules/npm/bin/npx-cli.js` |

`.cmd`/shell wrapperをtargetにせず、managed `node`へ同梱JS entrypointをfixed argvとして渡す。probeは`node --version`完全一致、`npm --version`、`npx --version`の成功、3 targetのpayload containmentを検査する。

### 8.3 storage

| ID | kind | scope | environment/用途 |
|---|---|---|---|
| `config` | config | tool | `NPM_CONFIG_USERCONFIG=<dir>/npmrc` |
| `cache` | content-cache | tool | `NPM_CONFIG_CACHE` |
| `history` | runtime-data | tool | `NODE_REPL_HISTORY` |
| `global-packages` | global-packages | version | `NPM_CONFIG_PREFIX`、version固有global package/bin |

Node native addonはNode/ABI差を持ち得るため、global package prefixはversion scopeかつ`purge=with-version`とする。Windowsはprefix root、Linuxはprefix`/bin`をPATHへprependする。設定は`npm config`、package導入は`npm install -g`等の標準commandを使う。全version共有へ変更する場合は実機互換調査とmigrationを[14-maintenance.md](14-maintenance.md)で仕様化する。

## 9. Python

### 9.1 provider選択

CPython公式Windows installerはsystem/user installation動作を持ち、公式sourceからLinuxでbuildする方式はcompilerとsystem libraryを要求する。「Windows/Linux共通・非admin/nonroot・短時間・複数完全version・portable」を優先し、両OSともAstral `python-build-standalone`のCPython install-only archiveを使う。

| 項目 | 契約 |
|---|---|
| tool license | `PSF-2.0` |
| provider | `astral-sh/python-build-standalone`（`third-party`） |
| provider license | `MPL-2.0`（`license_notice`は宣言しない。archive内Python/dependency license bundleも保持） |
| repository | `https://github.com/astral-sh/python-build-standalone` |
| Windows target | `x86_64-pc-windows-msvc-install_only_stripped.tar.gz` |
| Linux target | `x86_64-unknown-linux-gnu-install_only_stripped.tar.gz` |
| checksum | static assetへ固定したSHA-256（`checksum.kind="asset-field"`） |
| layout | top-level `python/`を1 component除去 |
| redirect host | `release-assets.githubusercontent.com` |

Planの重要要約にthird-partyを表示し、詳細にrepository、MPL-2.0、採用理由、provider build tagを出す。

### 9.2 固定catalog

Pythonは同じCPython完全versionが後日のprovider releaseで再buildされ得るため、live releaseから毎回artifactを選ばない。`python.toml`の`static_versions`へ次をversionごとに固定する。

- CPython完全version、channel、lifecycle、lifecycle根拠と評価日
- immutable provider release tag/URL/ID
- Windows/Linux exact asset name/URL/ID/size/SHA-256

同じCPython完全versionの採用assetを将来registryでも変更しない。既存assetに問題があれば新しいCPython完全versionとして追加するか、そのentryをregistryから外した新clientをreleaseする。同じversionへ黙って差し替えない。

追加候補はproviderのimmutable release、asset digest、両targetのinstall-only-stripped archive、version/layout/probe/licenseが確認できるものだけ。digestまたはlicense根拠が欠けるartifactを登録しない。registry更新はclient releaseでのみ配布する。

### 9.3 command

required commandは次の完全な集合。

| command | target/固定args |
|---|---|
| `python` | Windows `python.exe`、Linux `bin/python3` |
| `python3` | `python`と同じ実体 |
| `pip` | `python`実体＋`-m pip` |
| `pip3` | `python`実体＋`-m pip` |

provider同梱のlauncher/script名へ依存せず、pip/pip3は選択Pythonのmoduleとして起動する。probeは次を必須とする。

1. `python --version`の完全version一致。
2. `python -m pip --version`成功とpayload内module path。
3. `ssl`, `sqlite3`, `venv`のimport成功。
4. probe tmpへvenvを作成し、venv Python/pipのrequired pathを確認する。
5. `sys.prefix`とexecutableがpayload内、user site rootが宣言storageと一致。

### 9.4 storageとpackage

| ID | kind | scope | environment/用途 |
|---|---|---|---|
| `config` | config | tool | `PIP_CONFIG_FILE=<dir>/pip.ini\|pip.conf` |
| `cache` | content-cache | tool | `PIP_CACHE_DIR` |
| `history` | runtime-data | tool | `PYTHON_HISTORY`等、対応versionだけ |
| `user-packages` | global-packages | version | `PYTHONUSERBASE`、Python X.Y固有site-packages/scripts |

Python packageはPython X.Y/ABI差があるためversion scopeかつ`purge=with-version`。project dependencyは標準`python -m venv .venv`を推奨し、activate後はvenv内Python/pipがPATH先頭になる。base interpreterのplain `pip install`でread-only payloadを変更しようとした場合は失敗させ、global用途は`python -m pip install --user ...`を案内する。`PYTHONUSERBASE`はgdtvm version固有storageへredirectする。

gdtvmはpip package解決、requirements/pyproject編集、venv activationを独自実装しない。`pip config`、`python -m pip`、`python -m venv`という上流標準手段をそのまま使える環境を提供する。

## 10. .NET SDK

### 10.1 provider/version/artifact

| 項目 | 契約 |
|---|---|
| tool license | `MIT`（`dotnet/sdk` source） |
| homepage | `https://dotnet.microsoft.com/` |
| provider | Microsoft（`official`） |
| **provider license** | **Windows=`LicenseRef-dotnet-library`（.NET Library License）／ Linux=`MIT`** |
| version source | `kind="json-index"`。index=`https://builds.dotnet.microsoft.com/dotnet/release-metadata/releases-index.json`、子文書=各channelの`releases.json` |
| version | `sdks[].version`（`8.0.423`等）。`-preview.N`/`-rc.N`はsemver prerelease |
| published at | 親`releases[].release-date`を`item_parent_published_at_pointer`で各`sdks[]` itemへ継承 |
| Windows artifact | `dotnet-sdk-<version>-win-x64.zip`（asset name は`dotnet-sdk-win-x64.zip`） |
| Linux artifact | `dotnet-sdk-<version>-linux-x64.tar.gz`（asset name は`dotnet-sdk-linux-x64.tar.gz`） |
| checksum | `files[].hash`＝**SHA-512**（`checksum.kind="asset-field"`, `algorithm="sha512"`） |
| layout | **top-level directoryなし**（`strip_components=0`） |
| redirect host | 追加なし（`builds.dotnet.microsoft.com`だけ） |

`releases[]`の各要素は代表SDK 1件（`sdk`）と全feature band（`sdks[]`）を持つ。`item_flatten_pointer="/sdks"`で後者を展開し、`8.0.1xx`系のような古いfeature bandも導入できるようにする。`sdk`だけを読むとこれらがcatalogから欠落する。公開日時は親の`release-date`にだけ存在するため、`item_parent_published_at_pointer="/release-date"`で展開した各SDKへ継承する。

lifecycleは子文書top-levelの`support-phase`から`lifecycle_map`で写像する。

| upstream `support-phase` | gdtvm lifecycle |
|---|---|
| `preview` / `go-live` / `active` / `maintenance` | `supported` |
| `eol` | `eol` |

**mapに無い値はsource error**とし、黙って`unknown`へ倒さない。Microsoftがphaseを増やした場合はlive smokeが失敗するので、`docs/14-maintenance.md`のB01から調査する。preview channelのSDKはversion側がsemver prereleaseになるため`channel=prerelease`となり、既存のprerelease警告と`--yes`要求で覆われる。

### 10.2 Windows配布物のlicense

Windows配布archiveはMITではなく **.NET Library License**（Microsoft独自EULA、SPDX識別子なし）である。`license_notice`を宣言し、Planの冒頭と確認直前に`W_RESTRICTIVE_LICENSE`として表示して明示承認を求める。Linux配布archiveはMITのため宣言しない。

`provider.license`にはWindows=`LicenseRef-dotnet-library`、Linux=`MIT`を書く。`[tool].license`はsource licenseの`MIT`であり、配布物licenseの代替にしない。

### 10.3 command

required commandは次の完全な集合。

| command | target |
|---|---|
| `dotnet` | Windows `dotnet.exe`、Linux `dotnet`（いずれもpayload直下） |

`dotnet build`等はsubcommandであり、個別shimを作らない。probeは`dotnet --version`の完全version一致と`dotnet --list-sdks`の成功を検査する。probeは専用temp directoryをcwdとして起動するため、利用者の`global.json`がprobe結果へ影響しない。

### 10.4 storageとpackage

| ID | kind | scope | environment/用途 |
|---|---|---|---|
| `cli-home` | runtime-data | **version** | `DOTNET_CLI_HOME`。workload pack/manifest、first-run sentinel、local tool |
| `nuget-packages` | content-cache | tool | `NUGET_PACKAGES` |
| `nuget-http-cache` | content-cache | tool | `NUGET_HTTP_CACHE_PATH` |
| `nuget-plugins-cache` | content-cache | tool | `NUGET_PLUGINS_CACHE_PATH` |

`DOTNET_CLI_HOME`をversion scopeにするのは、workload pack/manifest、first-run data、local tool関連dataがSDK versionごとに異なり得るためである。`dotnet tool install --global`の配置先をこのstorage宣言の根拠にしない。

runtime環境は`DOTNET_ROOT={{payload}}`を設定し、`DOTNET_CLI_TELEMETRY_OPTOUT=1`と`DOTNET_NOLOGO=1`を`override_allowed`に入れずに固定する。[01-requirements.md](01-requirements.md)§9「telemetryを追加しない」に従い、利用者が親環境から無効化を解除できないようにする。

### 10.5 管理外領域と既知の制限

次はv0.1で**gdtvm管理外**とし、公開文書へ明記する。

| 領域 | 理由 |
|---|---|
| user-level `NuGet.Config` | 移動する環境変数がNuGet公式docに存在しない。私有feed設定が全.NET versionで共有されるのは実用上むしろ望ましい |
| `dotnet tool install --global` の配置先 | `DOTNET_CLI_HOME`による隔離は公式契約として確認できないため現時点では管理外。P10-06で両OSの実配置先を測定し、結果が出るまでstorageへ宣言しない |

既知の制限:

- `dotnet workload install`はSDK rootへ書き込むため、read-only payload（[08-install-runtime.md](08-install-runtime.md)§7手順5）で失敗する。
- Linuxはnative package（glibc / libgcc / libstdc++ / ca-certificates / openssl / tzdata / krb5、およびICU）を必要とする。gdtvmはこれらを導入しない。不足環境ではinstall probeが`E_PROBE_FAILED`となり、.NET自身のエラーがstderr末尾に出る。ICU無しで動かす場合は利用者が`DOTNET_SYSTEM_GLOBALIZATION_INVARIANT`を設定する。

## 11. standard definition contract test

各tool/platform/version fixtureで最低限次を検査する。

1. strict schema、ID/alias、provider/license、platform tuple、`license_notice`の有無。
2. version source fixtureからchannel、supported/eol/unknown lifecycle、unavailableを決定的に生成。`json-index`は複数子文書の連結と、1文書失敗時に部分catalogを作らないことを含む。
3. `item_flatten_pointer`が1段だけ展開し、展開前後のitem数が期待どおりであること。.NETは親`release-date`が全子SDKへ正規化・継承されること。
4. `lifecycle_map`に無い値でsource errorになること。
5. artifact selectorが0/1/2件の各caseを正しく扱う。
6. checksum source、algorithm（`sha256`/`sha512`）、hex長一致、filename、size、redirect host。
7. archive layoutを安全展開してrequired targetがexactly存在（`strip_components`が0と1の両方）。
8. command target/fixed args/profile、required command集合が本章と一致。
9. storage kind/scope/pathが本章表と一致し、payload/stateと非交差。
10. required probe、version mismatch、pip/venv、npm/npx、Go toolchain固定、`dotnet --list-sdks`。
11. Planのofficial/third-party、license、checksum、warning表示と、`W_RESTRICTIVE_LICENSE`が承認なしでは進めないこと。

## 12. live metadata smoke

PR CIのtest（unit、contract、e2e）はfake upstream fixtureと合成archiveを正とし、networkなしで決定的に実行する。live smokeは上流layout変更を検出する補助であり、release workflowだけが実行する。PR CIのgateにしない。

Go/Node.js/.NETはstable最新1件と過去stable 1件、Pythonはregistryへ固定した新旧各1件をrelease前に取得し、metadata、digest、archive layout、probeを検査する。さらに両OSのrelease runnerで、標準4 toolの実artifactによる`install`/`use`/`current`/`uninstall`の一巡を実行する。live失敗をfixture更新だけで隠さず、原因を調査して仕様/definition/schemaを先に同期する。test fixtureをlive最新versionへ自動書換えせず、Python固定artifactのdigest/build tagを自動更新しない。

.NETのlive smokeは追加で次を検査する。index文書のchannel件数が`max_documents`以下であること、`support-phase`の全出現値が`lifecycle_map`に存在すること、`files[].name`の固定名と`files[].hash`のhex長（128）が変わっていないこと。

§7〜§10のredirect hostはclient releaseごとにlive fixtureで再確認する。変更されていた場合はwildcardや最終host自動受理で通さず、公式一次資料/asset identityを調査してdefinition・fixture・仕様を同じclient releaseで更新する。

## 13. 将来tool

本章にないtoolは未仕様・未対応である。追加候補の共有領域は言語名の印象で決めない。例えば将来Rust/Cargo toolを共有する案は有力だが、複数toolchainでの実体互換、metadata、uninstall、更新競合を調査し、definitionのtyped storageとして明記してから採用する。調査だけでregistryへplaceholderを追加しない。
