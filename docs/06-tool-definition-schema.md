# ツール定義スキーマ

## 1. 目的とファイル単位

ツールごとの版発見、artifact選択、検証、展開、配置、公開command、環境変数、依存、確認事項をTOMLへ移す。1ファイルは正規tool ID 1件を定義し、ファイル名は `<tool-id>.toml` とする。

初期schema IDは `https://github.com/kznagamori/go_dev_tool_version_manager/schemas/tool-definition/v1`、整数majorは1とする。すべての標準定義はclient release archiveへ同梱し、source CI、archive SHA-256、binary埋込みregistry tree hashで整合性を検証する。

## 2. 最上位

最上位とplatform識別部の最小骨格を示す。実行可能な完全定義には後続節のversion source、artifact、install、runtime、validationも必要であり、この断片だけでは受入れない。

```toml
schema = 1
schema_id = "https://github.com/kznagamori/go_dev_tool_version_manager/schemas/tool-definition/v1"

[tool]
id = "example-tool"
name = "Example Tool"
aliases = ["example"]
description = "Example developer tool"
homepage = "https://example.invalid/"
license = "Apache-2.0"
version_scheme = "semver"
default_channel = "stable"
manager = "archive"

[[platforms]]
id = "windows-amd64"
os = "windows"
arch = "amd64"
priority = 100
supported = true
artifact_kind = "official"
relocation = "portable"
selection_strategy = "link"
```

最上位で許すkeyは `schema`, `schema_id`, `tool`, `platforms`, `extensions` だけである。

## 3. `[tool]`

| key | 型 | 必須 | 規則 |
|---|---|---:|---|
| id | string | yes | 正規ToolID |
| name | string | yes | 表示名、1～80文字 |
| aliases | string[] | no | 全registry内で衝突禁止 |
| description | string | yes | ja/en翻訳はregistryのmessage IDでも可 |
| homepage | HTTPS URL | yes | sourceの公式性判断に使用 |
| license | SPDX expression | yes | 不明時 `NOASSERTION` とwarning |
| version_scheme | enum | yes | `semver`, `numeric`, `jep223`, `date`, `lexical`, `regex` |
| version_regex | string | conditional | scheme=regex時、named group `version`, `key1`…必須 |
| default_channel | enum | yes | 通常stable |
| manager | enum | no | `archive`既定、初期backendは`rustup` |
| conflicts | ToolID[] | no | 同時環境へ入れられないtool |
| notes | string[] | no | message IDまたは短文 |

toolのlicenseとartifact個別licenseが異なる場合、artifact側を表示に用いる。

`conflicts` は、同じ有効実行環境へ同時に含められないtool IDを表す。standard definitionでは相互に同じIDを列挙して対称にし、registry CIで非対称を拒否する。local definitionとの組合せは片側の宣言だけでも競合として扱う。`use --project`、`install --use`、`exec`、shim環境mergeは競合する有効選択を `E_DEPENDENCY_CONFLICT` で拒否し、一方を暗黙解除しない。

version comparison keyはschemeごとに決定的に作る。`semver`はSemVer 2.0.0、`numeric`はdot区切り非負整数列、`jep223`はfeature/interim/update/patchとpre/build、`date`はdefinitionが正規化したUTC年月日時、`lexical`は正規versionのUTF-8 byte列を使う。`regex`はRE2のnamed capture `version`と、連続した`key1`～`keyN`（N=1～8、符号なし64-bit十進整数）を必須とし、keyを番号順の整数列として比較する。comparison keyが等しいのに正規versionが異なるentryは曖昧としてcatalog更新を失敗させる。

## 4. `[[platforms]]`

同じOS/archに複数variantを許す。選択は `supported=true`、OS完全一致、arch完全一致、libc条件一致の候補をpriority降順で評価し、同順位複数なら定義エラーとする。

| key | 型 | 必須 | 内容 |
|---|---|---:|---|
| id | string | yes | definition内一意 |
| os | enum | yes | `windows`, `linux`。将来値はschema更新 |
| arch | enum | yes | `amd64`, `arm64` |
| libc | enum | no | `any`, `glibc`, `musl`, `none`。既定はWindowsで`none`、Linuxで`any` |
| variant | string | no | host/toolchain等。既定 `default` |
| priority | integer | yes | 候補優先度 |
| supported | bool | yes | falseなら理由表示だけ |
| unsupported_reason | string | conditional | supported=false時必須 |
| artifact_kind | enum | conditional | supported=true時、`official`, `third-party` |
| artifact_reason | string | conditional | third-party時、公式を使えない理由 |
| source_repository | HTTPS URL | conditional | third-party時必須 |
| license | SPDX expression | no | artifact個別license |
| relocation | enum | conditional | supported=true時、`portable`, `repair-required`, `fixed` |
| mutable_payload | bool | no | 既定false。SDK自身が管理root内payloadを更新する場合だけtrue |
| selection_strategy | enum | conditional | supported=true時、`link`または`backend`。archiveはlink、rustupはbackend |
| executable_suffix | string | no | Windows `.exe`等 |
| path_separator | string | no | runtimeがOSから決定するため通常省略 |

platform table配下に `version_source`, `artifacts`, `dependencies`, `install`, `backend`, `repair`, `runtime`, `validation`, `hooks` を置く。`supported=true`ではversion source、1件以上のartifact、1件以上のruntime command、1件以上のrequired validation probeを必須とする。`selection_strategy="link"`は1件以上のinstall stepを必須、backend tableを禁止する。`selection_strategy="backend"`はbackend tableを必須とし、schema 1では固定backend adapterが導入DAGを生成するため`platforms.install`を禁止する。dependencies、environment、hooksは任意だが、schema 1のbackendではpayload前提hookを禁止する。`supported=false`では識別fieldと`unsupported_reason`だけを許し、実行tableを持たせない。

`relocation="portable"` は管理root absolute pathをpayloadへ埋め込まない。`repair-required` は埋込み先と冪等なrepair step/probeを必須とし、root移動後に自動実行せずplanを表示する。`fixed` は安全な修復手段がなく、portable modeでもinstall前に「root移動時は再導入が必要」と警告し、移動検出後は実行を拒否する。いずれも管理root外へのsystem installを意味しない。

## 5. バージョン発見 `[platforms.version_source]`

共通keyは次のとおり。

| key | 型 | 内容 |
|---|---|---|
| kind | enum | adapter種類 |
| url | template URL | 取得先 |
| method | enum | `GET`既定、`HEAD`は存在確認用だけ |
| headers | table | 固定header。secret値とenv展開は禁止 |
| header_env | table[] | secret headerの環境変数参照 |
| cache_ttl | duration | 省略時24h。catalogのexpires_at算出 |
| version_regex | RE2 string | capture `version`必須 |
| channel_rules | rule[] | channel分類 |
| minimum | version | 下限、含む |
| maximum | version | 上限、含む |
| exclude | string[] | 既知不良版の完全一致 |
| strip_prefix | string | `v`, `go`, `jdk-`等 |
| replace | replace[] | 順序付きliteral/regex置換 |
| sort | enum | `version-asc`, `version-desc`, `source` |
| deduplicate | bool | 正規versionで重複排除、既定true |
| catalog_mode | enum | `snapshot`既定、または`append-observed` |
| max_pages | integer | 1～100 |
| max_items | integer | 1～10000 |

`kind`は必須。`url`は`static`とGitHub adapter以外で必須とする。`headers` tableの値は公開してよいliteralだけとする。secret参照は別の `header_env` 配列で `name`, `env`, `prefix`, `required` を指定する。`env` は環境変数名、`prefix` は例として`Bearer `のような公開文字列であり、値本体をdefinitionへ書かない。Authorization、Proxy-Authorization、Cookieは常にlog mask対象。任意の文字列内で`${...}`を展開する方式は採用しない。

```toml
[platforms.version_source.headers]
Accept = "application/vnd.github+json"

[[platforms.version_source.header_env]]
name = "Authorization"
env = "GITHUB_TOKEN"
prefix = "Bearer "
required = false
```

adapter固有fieldは次のとおり。記載外keyは拒否する。

| kind | 必須field | 任意fieldと規則 |
|---|---|---|
| `static` | `versions` | 各要素は`value`必須、`raw`, `channel`, `published_at`, `metadata`任意 |
| `github-tags` | `repository`, `version_regex` | `tag_prefix`, `max_pages`。repositoryはGitHub HTTPS repository URL |
| `github-releases` | `repository`, `version_regex` | `include_drafts=false`固定、`include_prereleases=true`既定。releaseのtag/name/timeとassetsをmetadata化 |
| `github-release-assets` | `repository`, `asset_regex` | `release_regex`, `include_prereleases`。asset regexに`version` capture必須 |
| `json` | `url`, `items_path`, `fields.version` | `fields.raw_version/published_at/channel`, `fields.metadata.<name>`, `assets` mapping |
| `html-links` | `url`, `selector`, `value_source`, `version_regex` | value_sourceは`href`/`text`、`base_url` |
| `directory-listing` | `url`, `version_regex` | anchor hrefの末尾1 segmentだけを入力とする |
| `text-regex` | `url`, `item_regex` | item regexに`version` capture必須、metadata capture名を宣言可 |
| `head-probe` | `candidate_versions`, `url` | candidate_versionsは完全版配列、URL templateを版ごとにHEADし、405時だけ上限付きGETへfallback |
| `rust-manifest` | `url`, `host` | channel manifestの`pkg.rustc.version`先頭tokenを完全版、target availabilityをhostで検査 |

`json`のpointerはRFC 6901とする。`items_path` は1～8個のpointer文字列の配列である。最初のpointerをresponse rootへ適用し、得たarrayをsource順に展開し、次のpointerを各要素へ適用してarrayをさらに平坦化する。top-level自体がarrayなら`items_path = [""]`とする。途中または末尾がarrayでない、要素数が`max_items`を超える、同じobjectを循環参照する実装上のaliasがあれば失敗する。filter、再帰descent、scriptは許さない。

各`fields.*`は最終itemからの相対pointerである。`fields.version`はstring必須。`raw_version`, `published_at`, `channel`, `metadata.<name>`は存在する場合string必須とし、型が違えばentryを黙ってskipせずrefreshを失敗させる。旧名`items_pointer`はschema 1で受理しない。

version itemがartifact候補のarrayを持つJSONでは、次のasset mappingを使う。

```toml
[platforms.version_source.assets]
items_pointer = "/files"
scalar_field = ""

[platforms.version_source.assets.fields]
name = "/filename"
url = "/url"
size = "/size"
os = "/os"
arch = "/arch"
kind = "/kind"
digest = "/sha256"
digest_algorithm = "sha256"
```

`assets.items_pointer`の先はversion itemごとのarrayでなければならない。object要素を使う通常形では`scalar_field`を省略または空にし、`fields.name`をstring pointerとして必須にする。`url`, `os`, `arch`, `kind`, `format`, `digest`, `signature_url`は存在時string、`size`は非負integer、`digest_algorithm`はpointerではなく`sha256`というliteralまたは省略とする。追加の`fields.metadata.<name>`はstringだけを許す。URLをsource JSONが持たない場合は省略し、artifact recipeのURL templateで`{{asset.name}}`等から構築する。digestを持つ場合はalgorithmとhex形式をrefresh時に検査する。

array要素自体がstringのsource（例: Node indexの配布形式token）では、`scalar_field="name"`だけを許し、`fields` tableを持たせない。各stringを`asset.name`へ格納する。object/stringの混在、空string、同じasset.name重複を拒否する。

adapterがasset情報を持つ場合はversionごとに正規化asset配列を生成する。予約fieldは`asset.name`, `asset.url`, `asset.size`, `asset.os`, `asset.arch`, `asset.kind`, `asset.format`, `asset.digest`, `asset.digest_algorithm`, `asset.signature_url`, `asset.metadata.<name>`である。GitHub release adapterは少なくともasset name、browser download URL、size、content type、updated timeを予約fieldまたはmetadataへ提供する。`html-links`/`directory-listing`はmatchしたanchorの最終path segmentをname、解決後hrefをURLとして同じversion itemのassetへできる。`text-regex`は`asset_name`/`asset_url`という宣言済みcaptureが両方ある場合だけassetを生成する。GitHub APIのdownload URLはHTTPSかつ同repository releaseに属することを検査する。asset配列はsource順を意味あるpriorityとして使わず、後述のartifact条件で一意に選ぶ。asset情報がないadapterは空配列でよい。

`replace`の各要素は`kind="literal"|"regex"`, `from`, `to`, `count`を持つ。countは省略時all、指定時1～100。regexはRE2。channel ruleはpriority降順で評価し、同priority複数matchは定義エラーとする。

adapter fieldの追加規則:

- `static.versions` はarray of tableで、`value` string必須、`raw`, `channel`, RFC 3339 `published_at`, string→string `metadata`だけを許す。TOML表現は`[[platforms.version_source.versions]]`とする。
- GitHub `repository` は`https://github.com/<owner>/<repo>`のpath 2 segmentだけを許し、`.git` suffix、query、fragmentを正規化して保存しない。owner/repoをGitHub REST endpointへpercent encodeして使い、HTMLをAPI responseとしてparseしない。
- `include_prereleases`, `include_drafts`, `required`はbool、page/item上限はinteger、prefix/base URLはstring、pointer/regex/selectorはそれぞれ専用型とする。stringの`"false"`等をboolへ変換しない。
- `html-links.selector`のgrammarは、任意のASCII tag名、任意の`#id` 1件、`.class` 0件以上、`[attr="literal"]` 0件以上を空白なしで連結した単一simple selectorだけとする。descendant/child/sibling combinator、comma、pseudo class、namespace、substring属性match、escapeは拒否する。HTML tag/attribute名はASCII case-insensitive、id/class/valueはbyte exact。malformed HTMLはerrorにする。
- `text-regex.item_regex`でmetadataを得る場合、`metadata_groups = ["name", ...]`を明示し、regexの同名captureと完全一致させる。未宣言captureは捨て、宣言済みcapture欠落はerror。1 matchの最大長は64 KiB。
- `head-probe.candidate_versions`はdefinition内の完全版string配列であり、別adapterの出力参照ではない。動的な版一覧＋artifact実在確認はartifact `availability`を使う。
- `rust-manifest.host`は完全target triple。manifest signature/checksumが別artifactとして提供される場合は通常artifact規則で検証し、TOML parserはduplicate key、過大nest、未知必須構造を拒否する。

組込みadapterは次を必須実装する。

| kind | 入力と抽出 |
|---|---|
| `static` | `versions` 配列を定義に保持。緊急・固定helper用 |
| `github-tags` | GitHub REST tags/refsをページング。tag名へregex適用 |
| `github-releases` | GitHub REST releases/assets。draftを除外、prerelease分類 |
| `github-release-assets` | asset名からversionとartifact変数を抽出 |
| `json` | RFC 6901 JSON Pointerの`items_path`でarray階層を平坦化し、各最終要素と任意のasset arrayから宣言fieldを抽出 |
| `html-links` | anchor href/textへregex適用。CSS selectorは限定実装 |
| `directory-listing` | HTML link一覧からversion directoryを抽出 |
| `text-regex` | response textへglobal regex適用 |
| `head-probe` | 候補URLへのHEAD/小容量GETで存在確認。候補は別sourceから受ける |
| `rust-manifest` | Rust公式channel manifestを解析し完全rustc版へ正規化 |

HTML/JSON構造変化はrefresh失敗として扱い、空リストを成功保存してはならない。前回1件以上だったcatalogが0件になった場合も安全弁で拒否する。

`catalog_mode="snapshot"` は成功結果で旧一覧を置換する。`append-observed` はAndroid download pageやRust stable manifest等、現在版しか返さないsource向けで、既存の検証済みentryへ新規版をmergeし、同一versionのartifact metadataが変われば履歴を上書きせずsecurity conflictとして失敗する。削除は同梱registryの`exclude`/revocationまたは明示migrationだけで行う。

`json` adapterはfilter式、script、再帰descentを実行しない。`html-links` はselectorをtag名、ID、class、属性完全一致の組合せに限定し、JavaScriptを実行しない。GitHub adapterはAPI responseの型を固定検証し、paginationの同一URL loopを拒否する。

`selection_strategy="backend"` はmanager backendが完全selectorを解決できる場合だけ許す。初期実装では `manager="rustup"` と組み合わせ、version directoryへtoolchain treeを複製せず、shared rustup storeと版別receiptを使う。

### 5.1 変換

変換はsource文字列に次の順で適用する。

1. regex capture
2. trim ASCII whitespace
3. strip_prefix
4. replace配列
5. version scheme parse
6. min/max/exclude
7. channel rule
8. deduplicate/sort

regexはGoのRE2互換だけを許し、backtracking固有構文を禁止する。source itemが`version_regex`へmatchしない場合は「対象外」としてskipし、debug countへ記録する。matchしたのにnamed captureが空、変換後にversion scheme parseへ失敗、同じsource itemのrequired fieldが型不正の場合はskipせずrefresh全体を失敗させる。全itemが非match、またはfilter後0件なら成功catalogを保存しない。

### 5.2 channel rule

ruleは `when_regex`, `channel`, `priority` を持つ。priority降順で最初に一致したruleを使い、同priorityで複数一致すれば定義エラー、未一致はdefault_channelとする。stable候補にalpha、beta、rc、preview、nightly等のmarkerが残る場合はvalidation errorにする。

### 5.3 backend `[platforms.backend]`

`selection_strategy="backend"`ではbackend tableを必須とする。schema 1で許すkindは`rustup`だけであり、`tool.manager`も`rustup`でなければならない。

```toml
[platforms.backend]
kind = "rustup"
bootstrap_artifact = "primary"
host = "x86_64-pc-windows-gnu"
selector = "{{version}}-{{backend.host}}"
profile = "default"
components = []
targets = []
rustup_home = "{{shared}}/rustup"
cargo_home = "{{shared}}/cargo"
dist_server = "https://static.rust-lang.org"
update_root = "https://static.rust-lang.org/rustup"
install_timeout = "30m"
uninstall_timeout = "10m"

[[platforms.backend.cargo_config]]
target = "x86_64-pc-windows-gnu"
rustflags = ["-C", "link-arg=-Wl,--exclude-libs=ALL"]
```

| key | 型 | 必須 | 規則 |
|---|---|---:|---|
| kind | enum | yes | schema 1は`rustup` |
| bootstrap_artifact | artifact role | yes | rustup-init native executable。digest検証必須 |
| host | string | yes | rustup target triple。空白、slash禁止 |
| selector | template | yes | render後が`<exact-version>-<host>`と完全一致 |
| profile | enum | yes | `minimal`, `default`, `complete` |
| components | string[] | no | rustup component名、byte順・重複禁止 |
| targets | string[] | no | 追加target triple、byte順・重複禁止 |
| rustup_home | path template | yes | `{{shared}}`配下 |
| cargo_home | path template | yes | `{{shared}}`配下、rustup_homeと非重複 |
| dist_server | HTTPS origin URL | yes | `RUSTUP_DIST_SERVER` |
| update_root | HTTPS URL | no | rustup自体を更新しない通常runtimeでは未使用 |
| install_timeout | duration | yes | bootstrap＋toolchain install各processに適用 |
| uninstall_timeout | duration | yes | exact selector除去processに適用 |
| cargo_config | table[] | no | target別managed Cargo rustflags |

`{{backend.host}}`はbackend table内、runtime environment、validationだけで使用できる。selectorにはchannel名、`stable`, `latest`, 部分版をrenderしてはならない。

各`cargo_config`は`target`と1件以上の`rustflags` string配列を持ち、targetはbackend hostまたは`targets`の1件と完全一致させる。backend adapterはmanaged `CARGO_HOME/config.toml`の`[target.<target>] rustflags`だけをTOML-aware editorで更新し、操作前backupと書込み後hashをjournal/receiptへ残す。既存の同target tableがgdtvm所有記録なしで異なる場合は上書きせず承認付きconflict errorとする。他table、comment、利用者が追加したCargo commandを削除しない。uninstall時は他selectorが同targetを使う可能性があるため既定保持し、最後のselectorで明示shared削除が承認された場合だけCargo homeと共に削除する。

rustup adapterの処理は次で固定し、definitionが任意argvへ置換するfieldは設けない。

1. shared lockを取得し、bootstrap artifactを通常artifact規則でdownload・SHA-256検証する。
2. managed `CARGO_HOME/bin/rustup[.exe]`がない場合だけ、検証済みrustup-initを `-y --no-modify-path --default-host <host> --default-toolchain none` で起動する。`RUSTUP_HOME`, `CARGO_HOME`, `RUSTUP_DIST_SERVER`をmanaged pathへ固定する。
3. managed rustupを絶対pathで `toolchain install <selector> --profile <profile>` と起動し、componentごとに`--component`、targetごとに`--target`をbyte順で追加する。一般PATH、userのrustup、rustup self-updateを使用しない。
4. `toolchain list`でselector実在を確認し、`run <selector> rustc --version --verbose`のrelease/hostを完全照合する。
5. 成功後にbackend receiptをversion directoryへcommitする。shared toolchain treeをpayloadへcopyしない。

uninstallは同じmanaged envと絶対rustupを使い、inventory再確認後に`toolchain uninstall <selector>`だけを実行する。他selector、`CARGO_HOME/bin`の利用者導入物、registry/cache/sccacheを削除しない。bootstrap途中失敗でこのoperationが初めて作った空のmanaged homeだけはjournal照合後にrollbackできるが、既存shared homeを再帰削除しない。

runtime command targetはmanaged`CARGO_HOME/bin`内のrustup proxyであり、子環境へ`RUSTUP_TOOLCHAIN=<selector>`を必須設定する。`RUSTUP_HOME`, `CARGO_HOME`, `RUSTUP_DIST_SERVER`もreceiptへ確定保存する。deprecatedな`RUSTUP_DIST_ROOT`を生成しない。`update_root`は将来の明示rustup helper更新にだけ使用し、通常toolchain install/runtimeへ無条件設定しない。

## 6. artifact `[[platforms.artifacts]]`

一つのversionに主artifactと補助artifact（例: .NET SDKに追加するNuGet実体）があるため、artifactをrole単位で定義する。同じroleに複数recipeがある場合だけ`when`とpriorityで1件に決める。各recipeはURLをtemplateで構築する`template`方式と、version sourceが正規化したassetを選ぶ`asset`方式のどちらかである。

| key | 型 | 必須 | 内容 |
|---|---|---:|---|
| id | string | yes | platform内一意 |
| role | string | no | 既定`primary`。platform内の論理artifact名 |
| priority | integer | yes | 高い順 |
| when | condition | no | version/platform/metadata条件 |
| source | enum | no | `template`既定、または`asset` |
| asset_when | condition | conditional | source=asset時必須。`asset.*`だけを左辺に使う |
| url | URL template | conditional | template時必須。asset時は省略すれば`asset.url` |
| file | path segment template | conditional | template時必須。asset時は省略すれば`asset.name`。separator禁止 |
| format | enum | yes | `zip`, `tar`, `tar-gz`, `tar-xz`, `tar-zst`, `7z`, `exe`, `msi`, `raw` |
| size | integer/template | no | 既知サイズ |
| kind | enum | no | platform既定を上書き。third-partyへ変える場合はrepository/reason必須 |
| homepage | URL | no | third-party表示 |
| repository | URL | conditional | artifactがthird-party時のsource repository |
| reason | string | conditional | artifactがthird-party時、公式配布物を使えない理由 |
| license | SPDX | no | artifact license |
| checksum | table | yes | 下記 |
| signature | table | no | upstream signature |
| redirect_hosts | string[] | no | 許可する正規host完全名。既定は元hostだけ |
| availability | table | no | refresh時の実在確認 |

roleはoutput名と同じkebab-case制約で、同一roleのrecipe IDだけが選択候補になる。`primary` roleを1件以上定義し、install planで必ず参照する。補助roleはinstall stepまたはbackendから参照されるものだけ解決・取得し、未参照roleを勝手にdownloadしない。

recipe選択はroleごとにversion/platform変数だけで`when`を評価し、matchしたpriority最大を選ぶ。同priorityが複数なら定義エラー。`primary`が0件ならそのversionは現在platform非対応としてcatalogへ理由を記録する。参照された補助roleが0件ならinstall planを作れない版として失敗する。`source="asset"`ではその後、versionの正規化asset配列へ`asset_when`を評価し、exactly 1件を要求する。0件はそのrole非対応、2件以上は曖昧な定義としてrefreshを失敗させる。source順やfilename辞書順で1件を暗黙選択しない。

`availability` は `kind="source"|"head"|"get-range"`, `required`, `max_bytes` を持つ。省略時はasset sourceなら`source`、templateなら`head`とする。`source`は上流metadataにURL/asset実在が列挙された場合だけ使用できる。`head`はrender済みURLへHEADし、405時だけ`get-range`相当へfallbackする。`get-range`は`Range: bytes=0-0`で最大`max_bytes`（既定4096）まで読む。2xxだけを存在とし、認証error、redirect policy違反、上限超過を存在扱いしない。これによりdirectoryで版を列挙して個別installerの存在を確認するPython等を、adapter固有コードなしで表現する。

asset方式で利用できるtemplate値は前節の`asset.*`である。`size`省略時は`asset.size`、checksum literalのvalue省略時に`asset.digest`を暗黙使用してはならず、definitionが`value="{{asset.digest}}"`と明示する。URL templateでasset URLをそのまま使う場合も`url="{{asset.url}}"`と明示できる。render後のURL/file/size/checksumはcatalogへ確定保存する。

### 6.1 checksum

```toml
[platforms.artifacts.checksum]
algorithm = "sha256"
kind = "url-template"
url = "https://example.invalid/v{{version}}/SHA256SUMS"
line_regex = "^(?P<digest>[0-9A-Fa-f]{64})[ *](?P<file>.+)$"
file = "{{artifact.file}}"
required = true
```

`algorithm` はschema 1では`sha256`だけを検証済み判定に使用する。`kind` は `literal`, `url-template`, `release-asset`, `json-field`, `html-text`, `none`。`url-template`/`release-asset`/`html-text`は `line_regex` のnamed capture `digest`を必須とし、`file` captureがある場合はrender後の`file`と文字列完全一致する行だけを採る。0件または複数の異なるdigestは失敗する。`json-field`はRFC 6901 `digest_pointer`を必須とする。`literal`は `value` templateを必須とし、版ごとのrelease同梱registry metadataから得てもよい。digestはalgorithmのhex桁数・文字種を検査して小文字へ正規化する。

`none` では `required=false` が必須で、unverified artifact policyと確認対象になる。SHA-1やMD5は同一性補助として記録できるが、検証済み判定には使わない。checksum file自体のdownloadにもresponse size/TLS/redirect制限を適用する。

upstreamが署名を提供する場合、schema 1の`signature.kind`は`pgp-detached`または`minisign`とする。`url`, `key_file`, `key_fingerprint`, `required`を持ち、key fileは同梱registryの`upstream-keys/`内にある検証済み相対pathに限る。PGPはprimary key fingerprintを大文字空白なしhexへ正規化し、subkey署名ならprimary keyへの有効なbindingと署名時刻を検査する。Minisignはtrusted public key IDを照合する。`required=true`のsignature取得不能/不正は導入失敗、falseはwarningだがSHA-256検証を省略しない。このtableはtool artifact専用でありclient release検証には適用しない。

redirectは各hopで再検査し、`redirect_hosts`はIDNA正規化後のhost完全一致だけを許す。文字列suffix一致、任意subdomain、URL userinfo、scheme downgradeを許さない。CDNが必要なら実hostをdefinitionへ列挙する。

## 7. 依存関係 `[[platforms.dependencies]]`

| key | 型 | 内容 |
|---|---|---|
| id | string | platform内一意 |
| kind | enum | `managed-tool`, `helper`, `system-command`, `system-library` |
| tool | ToolID | managed-tool時 |
| version | 完全版/template | managed-tool時。range禁止 |
| helper | helper ID | helper時 |
| command | string | system-command時 |
| resolution | enum | system-command時、`windows-system`, `path`, `absolute-candidates` |
| candidates | path[] | absolute-candidates時必須 |
| probe_args | string[] | system-commandの版/能力確認。shell不使用 |
| stdout_regex/stderr_regex | RE2 | 任意probe条件。両方指定時は両方match |
| version_capture | enum | `stdout`/`stderr`。対応regexにnamed `version`必須 |
| version_scheme | enum | version_capture時、toolと同じscheme集合 |
| minimum/maximum | version | system dependencyだけrangeを許す。境界を含む |
| library | string | system-libraryの表示名 |
| sonames | string[] | system-library時、許可するELF/Windows DLL basename |
| search_roots | absolute path[] | system-libraryの既知root。PATH検索しない |
| required | bool | falseならwarning |
| reason | string | plan表示 |
| install_hint | string[] | OS別手動手順。自動実行しない |

kindごとに関係しないfieldを持たせない。`windows-system`はWindows system directory APIで得たdirectory直下のcommandだけを解決し、親環境のPATH/ComSpecを使わない。`path`は現在PATHを順に解決するが、見つけたabsolute path、file identity、versionをPlanへ固定し、Execute直前に再検査する。`absolute-candidates`は宣言順で実在する最初を採るが、相対pathとtemplateは禁止する。実行fileがworld-writable、現在user以外が書込み可能な不審directory、symlinkで管理root外の予期しない場所へ出る場合はpolicy warning/errorとする。

system-libraryは各`search_roots`直下でsoname basenameを探し、ELF/PE形式と対象archを検査する。再帰filesystem scan、`ldconfig`文字列のparse、任意DLL loadによる実行はしない。より複雑なABI probeが必要ならsystem-command dependencyを明示する。

managed dependencyは同じinstall planへ入り、循環を拒否する。helperはregistryの `helpers/<id>.toml` に別定義し、download/digest/license/commandを持つ。7-Zip、WiXなどが該当する。system dependencyは絶対に自動導入・昇格しない。optional dependencyが欠けた場合、それを参照するstep/runtime commandも`when`で除外されなければdefinition errorとし、実行途中で偶然skipしない。

## 8. 導入処理 `[[platforms.install.steps]]`

各step共通key:

| key | 型 | 内容 |
|---|---|---|
| id | string | platform内一意、receipt/journal識別子 |
| kind | enum | 下表 |
| when | condition | 省略時always |
| depends_on | step ID[] | 明示する先行step。循環禁止 |
| output | string | 0または1個の論理出力名。platform内一意 |
| for_each | output名 | path-listを順次処理する場合だけ |
| timeout | duration | 外部process時必須 |
| on_error | enum | `fail`既定、`warn`, `continue`。security stepはfail固定 |
| progress_weight | integer | progress配分 |

必須組込みstep:

| kind | 必須field | 任意fieldと既定 |
|---|---|---|
| `ensure-dir` | `path` | `mode`; outputを指定した場合は作成path |
| `download` | `artifact`, `output` | なし。artifact roleを1件指定し`.part`へstream |
| `verify-digest` | `input`, `artifact` | なし。artifact roleのchecksumと照合 |
| `verify-signature` | `input`, `artifact` | artifact roleのsignature required時必須。任意signature欠落はwarning |
| `extract` | `input`, `destination`, `format` | `strip_components=0`, `include=[]`, `exclude=[]`, `mode_policy="preserve-safe"`; outputはdestination |
| `extract-msi-admin` | `input`, `destination` | `properties={}`, `ui="quiet"`; Windows限定 |
| `move` | `from`, `to` | `replace=false`; outputはto |
| `rename` | `from`, `to` | moveと同じvolume内rename、`replace=false`; outputはto |
| `copy-file` | `from`, `to` | `mode`, `max_bytes=16777216`, `replace=false`; outputはto |
| `remove` | `path` | `recursive=false`, `missing_ok=true` |
| `write-file` | `path`, `content`, `encoding` | `mode`, `replace=false`; outputはpath |
| `patch-text` | `path`, `encoding`, `search`, `replace`, `expected_count` | outputはpath |
| `run` | `executable`, `args`, `cwd`, `timeout` | `env={}`, `unset_env=[]`, `exit_codes=[0]`, `capture="none"` |
| `run-helper` | `helper`, `args`, `cwd`, `timeout` | `env={}`, `unset_env=[]`, `exit_codes=[0]`, `capture="none"` |
| `probe` | `executable`, `args`, `timeout` | `cwd`, `env={}`, `exit_codes=[0]`, stdout/stderr条件 |
| `discover-files` | `root`, `glob`, `output` | `regex`, `type="file"`, `min_matches=1`, `max_matches=1` |
| `select-files` | `input`, `output` | `include=[]`, `exclude=[]`, `regex`, `min_matches=1`, `max_matches` |
| `set-output` | `value`, `value_type`, `output` | value_typeは`string`または`path` |

fieldの型は、`args`, `unset_env`, `include`, `exclude`, `exit_codes`が配列、`env`と`properties`がstring→string table、`strip_components`, `expected_count`, `min_matches`, `max_matches`, `max_bytes`が非負integer、それ以外は上表で明記しない限りstringである。`mode`はLinux octal permissionを文字列（例`"0755"`）で表し、Windowsでは実行permission判断に使わない。unknown fieldは拒否する。

### 8.1 step DAGと型付き出力

TOML記載順は同時実行可能stepの決定規則に使わない。`depends_on`でDAGを作り、同時実行可能なstepは実装が並列化できる。ただし同じpathへ書くstep、外部process、`for_each`は宣言順ではなくplanが競合を検出して直列化する。security上、`verify-digest`/required `verify-signature`が成功する前にarchiveを展開・実行してはならないため、definitionに依存edgeが欠けていてもschema validationで拒否する。

output名はASCII小文字で始まるkebab-case、2～64文字とする。型はstep kindから決まり、`download`, `ensure-dir`, `extract`, `move`, `rename`, `copy-file`, `write-file`, `patch-text`が`path`、`discover-files`/`select-files`が`path-list`、`set-output`が宣言型である。`run`/`run-helper`は`capture="stdout"|"stderr"`かつoutput指定時だけUTF-8 `string`を返し、末尾CR/LFだけを除く。binary output captureは許さない。

後続fieldで出力を参照する構文はfield全体が`{{outputs.<name>}}`である場合だけとする。path-listを単一path fieldへ渡せない。文字列への埋込みは`set-output`を明示して型検査する。存在しないoutput、後続stepのoutput、型不一致、未宣言の暗黙file探索を拒否する。

`for_each`は`extract-msi-admin`, `copy-file`, `move`, `rename`, `remove`, `run`, `run-helper`, `probe`だけに許す。参照先は先行`path-list` outputで、正規化相対pathのUTF-8 byte順に一つずつ直列実行する。各反復中だけ`{{item}}`, `{{item.basename}}`, `{{item.index}}`を使える。反復数は対象outputの`max_matches`以下でなければならず、途中失敗時は通常の`on_error`を反復単位で適用する。for-each stepがoutputを持つ場合、結果型は元の型のlistとし、schema 1ではそのlistをさらにfor_eachへ渡す以外のflattenを行わない。

`glob`/`include`/`exclude`はseparatorを`/`へ正規化したroot相対pathに対するglobで、`*`, `?`, `**`, character classを許す。先頭`/`、`..` component、NULを拒否する。結果は重複排除してbyte順にする。`regex`はその正規化path全体へRE2で適用する。cardinality外はstep failureであり、先頭1件を暗黙採用しない。

### 8.2 各stepの実行契約

- `download.artifact` は現在platformで選択済みのartifact roleでなければならない。別roleを追加取得する場合、そのrecipeも同じversionで一意選択できなければならない。最終URL、size、digestをcatalog entryと再照合する。
- `verify-digest` はinput fileをstreaming SHA-256で検証し、成功記録をoperation内のfile identity＋size＋mtimeに結び付ける。検証後にfileが変われば展開前に再検証する。
- `extract.format` はartifact formatと一致するliteralまたは`"artifact"`。archive entryの安全規則は[08-installation-and-runtime.md](08-installation-and-runtime.md)を適用する。`strip_components`後に空、衝突、root逸脱となるentryを拒否する。include適用後にexcludeを適用する。
- `extract-msi-admin` はsystem dependencyで検証したWindows system `msiexec.exe`を絶対pathで起動し、argvを`/quiet`, `/a`, input, `TARGETDIR=<destination>`の順に渡す。`properties`はASCII MSI property名→値で、`TARGETDIR`とUI/restart制御propertyの上書きを禁止する。exit code 0と3010を成功とするが、3010では再起動要求warningを出し、gdtvmは再起動しない。
- `move`/`rename`は同一volumeだけ。`replace=false`で既存先があれば失敗する。`replace=true`はoperationが所有するstaging/cacheの既知一時pathだけに許し、完成payloadや利用者fileを置換しない。
- `patch-text`はliteral searchだけで、`expected_count`と実一致数が完全一致しなければ一切書き換えない。regex置換が必要ならschema追加までhookを明示する。
- `run`はshellを介さず`executable`と`args`をargvで渡す。executableはsystem dependencyの検証済み絶対path、staging内検証済みfile、または同operationで検証済みartifactに限る。`run-helper`はhelper receiptのentrypointを絶対path解決し、一般PATHを検索しない。
- `probe`の条件は`stdout_regex`, `stderr_regex`, `combined_regex`のうち0～1件、`version_capture`, `expected_version`を持てる。regex指定時はmatch必須。version_capture指定時はnamed capture `version`を正規化しexpected_version templateと完全一致させる。
- `discover-files`はsymlink/reparse pointを結果に含めず、`type`は`file`または`directory`。follow-links optionはschema 1で提供しない。

`extract-msi-admin` は既存のPython方式を表現するために標準化する。`msiexec.exe` のコマンドラインはargvで渡し、文字列全体を実行ファイル名として渡してはならない。

tree全体の`copy` stepは提供しない。download stagingから完成payloadへの`move`を用いる。`copy-file`はshim/helper/config等の小型ファイルに限定し、既定上限16 MiB、定義で縮小のみ可能とする。

### 8.3 install root

stepが書き込める論理rootは `staging`, `download-cache`, `tool-shared`, `tool-helpers` だけである。完成 `payload` はcommit時にstagingからrenameして生成する。管理root外書込みが必要な標準定義はsystem prerequisiteとして扱い、自動実行しない。

### 8.4 relocation repair `[platforms.repair]`

`relocation="repair-required"` は `[[platforms.repair.steps]]` を1件以上持つ。step共通契約はinstallと同じだが、許可kindは`write-file`, `patch-text`, `run`, `run-helper`, `probe`, `discover-files`, `set-output`だけとし、network/download/extract/removeを禁止する。書込みrootは対象installのpayload/sharedだけで、変更fileをoperation backupへ保存する。

repairは旧rootと新rootを任意文字列置換せず、definitionが宣言したfile/encoding/expected countへ型付きpathを適用する。required validation probe成功後にimportant file digestとreceiptの`repaired_at`, `repair_definition_sha256`を更新する。失敗時はbackupを復元し、半修復状態を成功扱いしない。

## 9. hook `[[platforms.hooks]]`

組込みstepで表現できない処理のescape hatchである。

| key | 型 | 内容 |
|---|---|---|
| id | string | 一意 |
| phase | enum | `pre-install`, `post-extract`, `post-install`, `pre-uninstall`, `repair` |
| os | enum | platformと一致必須 |
| shell | enum | `none`, `cmd`, `powershell`, `pwsh`, `sh` |
| executable | template | shell=none時必須 |
| args | template[] | argv要素 |
| script | string | shell使用時のみ。registry内別file参照を推奨 |
| cwd | path template | 許可root内 |
| env | table | 明示追加値 |
| timeout | duration | 必須 |
| success_exit_codes | integer[] | 既定[0] |
| writes | path template[] | 予定書込み先を事前宣言 |
| reason | string | plan表示 |

shell=noneを優先する。shell hookはrelease同梱標準定義でもplanへ明示する。ローカル定義では内容hash承認にhook本体を含める。writes外の書込みをOS sandboxで完全防止できない場合でも、実行前後の監査とroot境界検査を行う。

phase順序は固定する。`pre-install`は承認後・download前、`post-extract`は主artifact展開後、`post-install`は全install step後だがvalidation/receipt/commit前にstaging上で実行する。`pre-uninstall`は参照検査後・trash/backend除去前、`repair`は明示repair plan内だけで実行する。commit済みpayloadをpost-install hookで後から変更してはならない。

## 10. 実行公開 `[platforms.runtime]`

### 10.1 command

```toml
[[platforms.runtime.commands]]
name = "example"
launcher = "native"
target = "{{payload}}/bin/example{{exe_suffix}}"
args = []
codepage = "inherit"
working_directory = "inherit"
```

| key | 規則 |
|---|---|
| name | OS command basename。path separator禁止。`gdtvm`, `gdtvm-shim`は予約 |
| when | version/platform/metadata condition。省略時always |
| required | bool、既定true。falseは対象実体がない版でcommand公開を省略可 |
| launcher | `native`, `cmd-script`, `powershell-script`, `sh-script` |
| target | link方式はpayload内、backend方式はtool shared内の実体。realpathが許可root内必須 |
| interpreter | script launcherのinterpreter ID。native/cmd-scriptでは省略 |
| args | 利用者引数より前に追加する固定引数 |
| codepage | Windows `inherit`, `utf-8`, numeric code page。Linuxはinheritのみ |
| working_directory | `inherit`, `payload`, `shared` |
| env_profile | 下記profile ID。省略はdefault |
| passthrough_signals | 既定true |

`native` はtargetを直接起動する。`cmd-script` はWindows system directory APIで得た正規`cmd.exe`を `/d /v:off /s /c` で起動し、親環境の`ComSpec`を実行先として信用しない。利用者引数はcmdのquote/meta/percent展開規則に従って1要素ずつencodeし、script本文へ連結しない。`powershell-script` は`interpreter="powershell"`または`"pwsh"`を必須とし、system dependency probeで固定した絶対hostへ`-NoProfile`とscript file引数を分離して渡す。`sh-script` は`interpreter="sh"`または`"bash"`を必須とし、system dependency probeで固定した絶対pathへscript pathを第1引数として渡す。receiptにinterpreter IDと検証済み絶対pathを記録し、起動時に同じ実体を再検査する。OSとlauncherの不整合、必要なinterpreter dependency欠落はdefinition errorとする。

`when`はinstall plan時に評価しfalseならcommand候補自体を作らない。trueで`required=true`のtarget欠落・非実行可能はinstall失敗。`required=false`のtarget欠落はwarningとreceiptの`omitted_commands`へname/reasonを残してshimを作らない。targetが存在する場合はoptionalでも通常commandとして公開する。実体の有無以外のprobe失敗をoptionalとして黙認しない。

同一tool・同一platform内のcommand名重複はdefinition errorとする。commandの内部IDはschema 1ではnameと同じで、shim indexの`runtime_command_id`にも正規nameを保存する。異なるtoolが同じcommand名を公開することは許可し、registry横断検査で衝突情報をshim indexへ残す。実行時に有効選択が1 toolへ定まる場合だけ起動し、複数toolが有効なら優先度で暗黙選択せず `E_COMMAND_AMBIGUOUS` とする。定義の`tool.conflicts`により、同時利用そのものを事前拒否してもよい。

### 10.2 環境profile

```toml
[[platforms.runtime.environment]]
id = "default"
path_prepend = ["{{payload}}/bin", "{{shared}}/bin"]
path_append = []
unset = ["EXAMPLE_LEGACY_HOME"]
shell_export = ["EXAMPLE_HOME"]
shell_export_path = true
override_allowed = []

[platforms.runtime.environment.set]
EXAMPLE_MODE = "managed"

[platforms.runtime.environment.set_paths]
EXAMPLE_HOME = "{{tool}}"
```

環境変数名はOS上有効なASCII名。Windowsではcase-insensitiveに重複検査する。`path_prepend` は空要素を許さず、重複をcanonical比較で除く。`set` はliteral値、`set_paths` はPathResolverが扱うpath templateであり、後者をreceiptへlogical root＋相対pathとして保存する。両table、`unset`の間で同名を許さない。`shell_export` の各要素は同じenvironment itemの`set`または`set_paths`に存在しなければならず、`override_allowed`も同じ集合のsubsetとする。既存値を保存する `_OLD_*` 方式は不要で、親processを変更せず子環境mapを生成する。

### 10.3 shell export

`shell_export` に列挙した変数だけをshell bootstrapのuser current環境へ出す。`shell_export_path=true` の場合だけ、そのitemの`path_prepend`と`path_append`をPATHへ反映する。tool home（例: `JAVA_HOME`, `GOROOT`）とPATHが主な対象となる。未列挙変数はshim/`exec`の子環境だけに適用する。project選択は親shellへ安全に遡及できないためshim実行環境で適用し、必要に応じ `gdtvm exec -- code.exe .` を使う。

## 11. 検証 `[[platforms.validation.probes]]`

各probeのfield:

| key | 型 | 必須 | 規則 |
|---|---|---:|---|
| id | string | yes | platform内一意 |
| runtime_command | command ID | xor | runtime commandをreceipt候補どおり起動 |
| executable | path template | xor | payload/shared内の検証済み実体 |
| args | string[] | yes | 空配列可 |
| cwd | enum/path | no | `staging`, `payload`, `shared`または許可root内template。既定staging |
| env | table | no | sanitized環境への追加literal/template |
| stdout_regex | RE2 | no | stdout全体にmatch必須 |
| stderr_regex | RE2 | no | stderr全体にmatch必須 |
| version_capture | enum | no | `stdout`または`stderr`。対応regexにnamed group`version`必須 |
| expected_version | template | conditional | version_capture時必須、通常`{{version}}` |
| timeout | duration | yes | 正値 |
| required | bool | yes | 1件以上true必須 |

`runtime_command`と`executable`はexactly one。runtime command probeではlauncher、固定args、environment profileを実運用と同じように適用し、その後へprobe argsを追加する。executable probeはshellを介さず直接起動する。exit code 0だけを成功とし、別codeが必要ならinstall `probe` stepを使うのではなくschema改訂で明示する。stdout/stderrはUTF-8として検査し、不正byteがあればversion probeを失敗させる。表示用には末尾上限内のsafe byteだけを保持する。

- install commit前にrequired probeをすべて実行する。
- `expected_version` は定義のtransformを通した結果と完全一致させる。
- probeが初回設定を作るツールでは、隔離したHOME/sharedを渡す。
- ネットワークを必要とするprobeはrequiredにしない。

## 12. テンプレート

構文は `{{name}}` だけを用いる。関数呼出し、任意式、環境変数展開、shell展開を禁止する。利用可能値:

| 変数 | 内容 |
|---|---|
| `tool.id`, `tool.name` | tool情報 |
| `version`, `version.raw` | 正規版、source元版 |
| `version.major/minor/patch` | schemeで存在する場合 |
| `os`, `arch`, `libc`, `variant` | platform |
| `exe_suffix` | `.exe`または空 |
| `artifact.file/url` | 選択artifact。checksum解決後のstepでは`checksum.value`も利用可 |
| `asset.name/url/size/os/arch/kind/format/digest/digest_algorithm/signature_url` | source=assetの候補。artifact選択・URL/file/checksumだけで利用可 |
| `root`, `staging`, `payload`, `shared`, `helpers`, `cache` | PathResolver値。backend方式で`payload`参照はerror |
| `tool` | runtime environment限定の有効tool root。link方式はexact payload、user shell snapshotでは利用可能なら`current`、backend方式では使用禁止 |
| `metadata.<name>` | source regex/APIからschema宣言された値 |

`tool`はinstall、checksum、hook、command targetでは使用できず、runtime environmentのpathと変数値だけで使用できる。`asset.*`はsource=asset recipeの評価中だけ存在し、receipt runtimeへ残さない。path contextではrender後にclean/canonical/root検査する。URL contextではplaceholderがURL全体なのかpath segment/query値なのかfield schemaで区別する。`url="{{asset.url}}"`の全体置換だけは検証済みabsolute URLをそのまま採用し、それ以外はpath segment/query valueとしてpercent encodeする。raw文字列連結でcontextを跨がない。

## 13. 条件式

条件は文字列プログラムではなくTOML inline tableで表す。Conditionは次のkeyをexactly 1個だけ持つ。

| key | value形 | 意味 |
|---|---|---|
| `all` | Condition[]、1件以上 | 全件true |
| `any` | Condition[]、1件以上 | 1件以上true |
| `not` | Condition | 否定 |
| `eq` | `{ left = "...", value = <scalar> }` | 型を変換せず完全一致 |
| `in` | `{ left = "...", values = [<scalar>...] }` | 1件以上の同型literalに一致 |
| `matches` | `{ left = "...", pattern = "..." }` | string全体へRE2検索 |
| `version_gte` | `{ left = "version", value = "<exact>" }` | toolのversion schemeで以上 |
| `version_lte` | `{ left = "version", value = "<exact>" }` | toolのversion schemeで以下 |
| `exists` | `{ left = "..." }` | 宣言済みsource/metadata値がnull/欠落でない |

例:

```toml
when = { all = [{ eq = { left = "os", value = "windows" } }, { version_gte = { left = "version", value = "1.20.0" } }] }
```

`left`はtemplate bracesを付けない識別子で、評価contextごとに許可された`version`, `version.raw`, `version.major/minor/patch`, `os`, `arch`, `libc`, `variant`, `metadata.<name>`、asset選択中の`asset.*`だけを使える。未宣言metadata名はschema errorであり、`exists`でも任意環境変数やfilesystem pathを調べられない。filesystemの存在確認は`discover-files`/`probe`を使う。

scalarはstring、integer、boolのいずれか。leftの型とliteralの型が違えばfalseへ丸めずschema errorとする。`matches`はstringだけ。version比較のvalueも正規完全版でなければならない。最大nest 8、1 Condition tree 256 node、配列64件までとする。短絡評価してよいがvalidationは未評価branchも全件行う。任意コード評価、関数、環境変数、時刻、乱数は禁止する。

## 14. encoding

`write-file` と `patch-text` は `utf-8`, `utf-8-bom`, `shift-jis`, `utf-16le` を初期対応する。Windows command fileが必要な互換処理ではShift-JISを選べるが、gdtvm自身のstate/definitionは常にUTF-8。変換不能文字は置換せず失敗する。

## 15. スキーマ検証順序

1. TOML構文
2. top-level/unknown key
3. 型、必須、値範囲
4. IDとalias一意性
5. platform候補一意性
6. source/regex/template静的検証
7. step DAG、input/output、root capability
8. dependency DAG
9. command衝突
10. security policy（HTTP、shell、checksum）

標準registryのCIは全platform定義を検証する。クライアントも使用platformだけでなくファイル全体を検証し、破損した未使用platformを黙認しない。

## 16. schema互換

- クライアントは同じmajor内の未知optional minor featureを`registry.toml`のschema互換情報でskipできる。
- 必須featureが未対応なら同梱registry全体を有効化しない。
- major変更は別schema IDとし、`registry/registry.toml`が`minimum_client_version`を宣言する。
- definitionを自動書換えしない。registry側でmigrationする。
