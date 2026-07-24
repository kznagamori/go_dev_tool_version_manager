# 完全ツール定義リファレンス

## 1. 目的

本章は[06-tool-definition-schema.md](06-tool-definition-schema.md)のfieldを、一つの実行可能なplatform recipeへ組み立てる正規例を示す。Go実装がこの例を特別扱いしてはならず、同じparser、validator、planner、step executor、runtime resolverでローカル定義も処理する。

例はGo SDKのWindows amd64 recipeである。上流URLや最新版をclientへhard-codeする意図ではなく、registryのTOMLだけで次を表せることを検証するcontract fixtureである。

- JSONから完全版とasset候補を抽出する。
- OS/arch/kindが一致するassetを一意に選ぶ。
- source metadataのSHA-256をdownload後に照合する。
- stagingへ展開してpayloadをatomic commitする。
- 二つのcommandと共有cache環境を公開する。
- commit前に実体の完全版をprobeする。

## 2. 正規TOML例

```toml
schema = 1
schema_id = "https://github.com/kznagamori/go_dev_tool_version_manager/schemas/tool-definition/v1"

[tool]
id = "go"
name = "Go"
aliases = []
description = "Go programming language SDK"
homepage = "https://go.dev/"
license = "BSD-3-Clause"
version_scheme = "semver"
default_channel = "stable"
manager = "archive"

[[platforms]]
id = "windows-amd64"
os = "windows"
arch = "amd64"
libc = "none"
variant = "default"
priority = 100
supported = true
artifact_kind = "official"
relocation = "portable"
mutable_payload = false
selection_strategy = "link"
executable_suffix = ".exe"

[platforms.version_source]
kind = "json"
url = "https://go.dev/dl/?mode=json&include=all"
method = "GET"
cache_ttl = "24h"
items_path = [""]
version_regex = "^(?P<version>go[0-9]+[.][0-9]+[.][0-9]+)$"
strip_prefix = "go"
sort = "version-desc"
deduplicate = true
catalog_mode = "snapshot"
max_items = 10000

[platforms.version_source.fields]
version = "/version"

[platforms.version_source.assets]
items_pointer = "/files"

[platforms.version_source.assets.fields]
name = "/filename"
size = "/size"
os = "/os"
arch = "/arch"
kind = "/kind"
digest = "/sha256"
digest_algorithm = "sha256"

[[platforms.artifacts]]
id = "windows-amd64-archive"
role = "primary"
priority = 100
source = "asset"
asset_when = { all = [{ eq = { left = "asset.os", value = "windows" } }, { eq = { left = "asset.arch", value = "amd64" } }, { eq = { left = "asset.kind", value = "archive" } }] }
url = "https://go.dev/dl/{{asset.name}}"
file = "{{asset.name}}"
format = "zip"
redirect_hosts = ["go.dev", "dl.google.com"]
availability = { kind = "source", required = true, max_bytes = 4096 }

[platforms.artifacts.checksum]
algorithm = "sha256"
kind = "literal"
value = "{{asset.digest}}"
required = true

[[platforms.install.steps]]
id = "download-primary"
kind = "download"
artifact = "primary"
output = "primary-archive"
progress_weight = 50

[[platforms.install.steps]]
id = "verify-primary"
kind = "verify-digest"
depends_on = ["download-primary"]
input = "{{outputs.primary-archive}}"
artifact = "primary"
progress_weight = 10

[[platforms.install.steps]]
id = "extract-primary"
kind = "extract"
depends_on = ["verify-primary"]
input = "{{outputs.primary-archive}}"
destination = "{{staging}}/payload"
format = "artifact"
strip_components = 1
output = "staged-payload"
progress_weight = 40

[[platforms.runtime.commands]]
name = "go"
launcher = "native"
target = "{{payload}}/bin/go.exe"
args = []
codepage = "inherit"
working_directory = "inherit"
env_profile = "default"
passthrough_signals = true

[[platforms.runtime.commands]]
name = "gofmt"
launcher = "native"
target = "{{payload}}/bin/gofmt.exe"
args = []
codepage = "inherit"
working_directory = "inherit"
env_profile = "default"
passthrough_signals = true

[[platforms.runtime.environment]]
id = "default"
path_prepend = ["{{payload}}/bin", "{{shared}}/gopath/bin"]
path_append = []
unset = []
shell_export = ["GOROOT", "GOPATH", "GOBIN", "GOCACHE", "GOENV", "GOMODCACHE"]
shell_export_path = true
override_allowed = []

[platforms.runtime.environment.set_paths]
GOROOT = "{{tool}}"
GOPATH = "{{shared}}/gopath"
GOBIN = "{{shared}}/gopath/bin"
GOCACHE = "{{shared}}/cache/go-build"
GOENV = "{{shared}}/env/go.env"
GOMODCACHE = "{{shared}}/gopath/pkg/mod"

[[platforms.validation.probes]]
id = "go-version"
runtime_command = "go"
args = ["version"]
stdout_regex = "go version go(?P<version>[0-9]+[.][0-9]+[.][0-9]+) windows/amd64"
version_capture = "stdout"
expected_version = "{{version}}"
timeout = "30s"
required = true
```

このblockはTOML 1.0としてparse可能でなければならない。fixtureで実networkを使うtestと、同形のfake upstreamを使うdeterministic testを分ける。上流がredirect hostやJSON fieldを変更した場合はregistry definitionを更新し、client codeへ例外分岐を足さない。

## 3. plannerが確定する値

上のdefinitionと完全版入力から、Plan作成時に次をすべて確定する。未確定値をExecute中に「最新」へ再解決しない。

| 値 | 確定元 |
|---|---|
| ToolID、version scheme | `[tool]` |
| platform ID、variant | OS/arch/libcとpriority |
| exact version、raw version | catalog item |
| asset recipe、source asset | role、when、asset_when |
| URL、file、size、SHA-256 | render済みasset contract |
| step DAGと全書込み先 | install stepsとPathResolver |
| runtime command、environment | runtime tablesをlogical pathへ型付けした値 |
| validation target/expected version | validation probe |
| definition origin/hash | release同梱registryまたは承認済みlocal bundle |

catalog/definition/state/configのfingerprintがPlan後に変われば`E_PLAN_STALE`とし、download済みfileがあっても古いplanのままcommitしない。

## 4. 設定駆動性の受入条件

engineに新toolを追加したと主張できるのは、Goのsource変更なしで次を満たした場合だけである。

1. 新TOMLをlocal definition directoryへ置く。
2. schema validationと承認を通す。
3. `refresh`, `available`, `install`, `installed`, `use`, `current`, shim実行, `disable`, `uninstall`, `doctor`が動く。
4. URL、asset名、archive layout、environment、commandをTOML変更だけで差し替えられる。
5. 外部helperが必要ならrelease同梱registryのhelper definition追加だけで導入・共有・実行できる。
6. 組込みstepで不足する場合だけ明示hookを使い、plan、承認、writes監査へ現れる。

新しいversion source adapter、archive format、backend manager、OS primitiveそのものを増やす場合はGo実装とschema minor/major更新が必要である。個別tool名や個別versionを条件にしたGo分岐は、security emergency denylistを除き禁止する。denylistを追加した場合もregistry revocationへの移行期限とtestを記録する。

## 5. 定義作者向け最小変更例

上流のarchive内top-levelがなくなった場合は`strip_components`だけを変更する。commandが増えた場合はruntime commandとvalidation probeを追加する。cache環境を変える場合はenvironment profileを変更する。OS/archを増やす場合はplatform tableを追加する。

これらの変更でCLI、Application Service、installer、shimへtool固有codeを追加してはならない。definition SHA-256が変わるため既存receiptは元runtime契約を保持し、新規installだけが新definitionを使う。
