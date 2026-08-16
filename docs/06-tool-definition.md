# tool definition schema 1

## 1. 範囲

schema 1はGo、Node.js、Python、.NET SDKを`windows-amd64`と`linux-amd64-glibc`へ導入するために必要な機能だけを定義する。fileは`registry/tools/<tool-id>.toml`、UTF-8 BOMなしTOML 1.0。unknown key/table、重複、型違い、enum外、上限外を拒否する。

次はschema 1に存在しない。追加toolがこれらを必要とする場合、definitionへ未知keyを足さず[14-maintenance.md](14-maintenance.md)§6のschema拡張gateを先に行う。

- backend manager、managed helper、tool dependency graph
- hook、inline/file shell script、任意external process
- install stepのDAG、任意step種別、step間output受け渡し
- 署名検証（PGP/Minisign等）、artifact lock
- external/system tool候補の探索と固定
- local definition、companion bundle
- variant/priority/condition DSL、HTML scraping、regexだけによる任意web解析
- Windows arm64、Linux arm64/muslのplatform entry

## 2. top-level

```toml
schema = 1
schema_id = "https://github.com/kznagamori/go_dev_tool_version_manager/schemas/tool-definition/v1"

[tool]
# ...

[[platforms]]
# ...
```

top-level keyは`schema`, `schema_id`, `tool`, `platforms`だけ。全件必須。`schema=1`固定、`schema_id`は上記完全一致。platformは1件以上、ID一意。

## 3. identifier grammar

| 型 | grammar |
|---|---|
| tool ID/alias | `^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$`、1～64 byte |
| platform/storage/probe/profile ID | 同上、1～96 byte |
| command | Windows/Linux共通basename。`^[a-z][a-z0-9]*(?:[-+._][a-z0-9]+)*$`、1～64 byte |
| metadata key | `^[a-z][a-z0-9_]{0,63}$` |
| version | sourceから抽出しtoolのversion grammarへ完全一致。path grammarとは別 |

ASCII以外、uppercase、前後空白、連続separator、`.`/`..`、Windows予約名、同一tool内のcase-insensitive衝突を拒否する。aliasは全registryでtool ID/aliasと衝突不可。

## 4. `[tool]`

| key | type | 必須 | 内容 |
|---|---|---:|---|
| `id` | string | yes | file basenameと一致 |
| `name` | string | yes | 表示名、1～128 UTF-8 byte |
| `aliases` | string[] | yes | 空可 |
| `description` | string | yes | 1～512 byte |
| `homepage` | HTTPS URL | yes | credentialなし |
| `license` | SPDX expression | yes | upstream tool license |
| `version_scheme` | enum | yes | `semver\|go\|python` |

`semver`はSemVer 2.0.0の`MAJOR.MINOR.PATCH`と任意の正規prereleaseを受けるが、leading `v`とbuild metadataを含めない。

`go`は`MAJOR.MINOR`, `MAJOR.MINOR.PATCH`と、`MAJOR.MINORbetaN|rcN`を受ける。`N`は1以上、数値要素は不要なleading zero禁止。比較はmajor/minor、`beta < rc < final`、prerelease番号、finalのpatch（省略は0）の順。同じcomparison keyになる`1.20`と`1.20.0`を同一catalogへ併存させないが、利用者入力は登録された正規文字列との完全一致だけを受ける。

`python`は`MAJOR.MINOR.PATCH`と任意の`aN|bN|rcN`を受ける。比較は数値3要素、`a < b < rc < final`、prerelease番号の順。Nは1以上、leading zero禁止。

すべてのschemeで入力versionはcatalogの正規文字列完全一致であり、comparison keyへ変換した近似一致をしない。

## 5. `[[platforms]]`

| key | type | 必須 | 内容 |
|---|---|---:|---|
| `id` | enum | yes | `windows-amd64\|linux-amd64-glibc` |
| `os` | enum | yes | `windows\|linux` |
| `arch` | enum | yes | `amd64` |
| `libc` | enum | yes | Windows=`none`, Linux=`glibc` |
| `artifact_kind` | enum | yes | `official\|third-party` |
| `license_notice` | string | no | 配布物がOSI承認OSS licenseでない場合のmessage ID |
| `provider` | table | yes | 取得主体 |
| `version_source` | table | yes | version発見 |
| `artifact` | table | yes | primary 1件 |
| `install` | table | yes | 展開parameter |
| `storage` | table[] | yes | 空可 |
| `runtime` | table | yes | command/environment |
| `validation` | table | yes | probe |

IDとOS/arch/libcの組は上表どおり一致させ、同一tupleは1件。schema 1のplatform entryは対応済みtupleだけを表し、unsupported placeholderを置かない。

payloadは全platformで再配置可能であることを前提とする。再配置できないtoolはschema 1の対象外とし、[14-maintenance.md](14-maintenance.md)§6のschema拡張gateを経る。

`license_notice`は、公式providerの配布物であってもlicenseがOSI承認OSS licenseでないplatformへ宣言する。宣言されたplatformのPlanは`W_RESTRICTIVE_LICENSE`を重要要約へ出し、明示承認を必須とする（[04-storage-and-data.md](04-storage-and-data.md)§16.1）。`provider.license`に正確な識別子を書いたうえで、利用者が見落とさないための宣言であり、`provider.license`の代替ではない。OSS licenseのplatformへ宣言しない。

### 5.1 provider

許可keyは`name`, `repository`, `homepage`, `license`, `adoption_reason`。

- officialはname/homepage/license必須、repositoryは任意、adoption_reason禁止。
- third-partyは全件必須。Planでprovider、repository、license、adoption_reasonを常に表示する。
- URLはHTTPS、credential/query secretなし。artifact provider licenseはtool language licenseと異なってよい。

## 6. version source

```toml
[platforms.version_source]
kind = "json"
url = "https://example.invalid/versions.json"
items_pointer = ""
version_pointer = "/version"
version_regex = "^v(?P<version>[0-9]+[.][0-9]+[.][0-9]+)$"
channel_pointer = "/channel"
published_at_pointer = "/published_at"
assets_pointer = "/assets"
max_items = 10000
cache_ttl = "24h"
```

### 6.1 共通key

許可keyは`kind`, `url`, `index_items_pointer`, `index_document_pointer`, `max_documents`, `document_lifecycle_pointer`, `lifecycle_map`, `items_pointer`, `item_flatten_pointer`, `item_parent_published_at_pointer`, `version_pointer`, `version_regex`, `channel_pointer`, `lifecycle_pointer`, `lifecycle_overrides`, `published_at_pointer`, `assets_pointer`, `asset_fields`, `metadata_fields`, `required_tokens_pointer`, `required_tokens`, `max_items`, `cache_ttl`, `static_versions`。

`kind`は`json|json-index|static`の3値。

| kind | 契約 |
|---|---|
| `json` | HTTPS GETで**1文書**だけを読む。`index_*`, `max_documents`, `document_lifecycle_pointer`, `lifecycle_pointer`, `lifecycle_map`, `static_versions`を禁止する |
| `json-index` | index文書から子文書URL群を得て、**各子文書を読む**。§6.2の追加契約に従う。`static_versions`を禁止する |
| `static` | networkなし。`static_versions` arrayと`max_items`だけを使用し、他pointer/url/index/cache fieldを禁止する |

pointerはすべてRFC 6901。**空文字が文書全体を指し、`/`はkeyが空文字のmemberを指す。** top-levelが配列の文書のitemsは空文字で指す（Node.jsとGoが該当する）。配列やobjectの型に応じて`/`をrootへ読み替える等の代替解釈を行わない。1文書あたり16 MiB、全文書合計のitemsは10,000の組込み上限以下。`max_items`は1以上で上限を縮小する値だけ。redirect後もHTTPS。unknown JSON fieldは無視できるが、definitionが参照するfieldの欠落/型違いはそのitemを黙ってskipせずsource errorにする。

`item_flatten_pointer`は`json`と`json-index`で任意。指定した場合、`items_pointer`が指す配列の各要素へこのpointerを適用し、得られた配列を**1段だけ**連結した結果をversion itemの集合とする。pointer先が配列でない、または存在しないrequirementはsource errorとする。展開は1段までで、入れ子の再帰展開を行わない。`items_pointer`自体にwildcardを書かない。

`item_parent_published_at_pointer`は`item_flatten_pointer`と組でだけ使用でき、展開前の各親itemから公開日時を読み、その親から展開した全version itemの`published_at`へ継承する。子itemの`published_at_pointer`との同時指定は禁止する。公開日時はUTC RFC 3339またはISO 8601 full-date（`YYYY-MM-DD`）のstringだけを受け、full-dateは`T00:00:00Z`へ正規化する。catalogの`published_at`はitem pointer、親pointer、選択assetの`published_at`の順で最初の宣言済み値を使い、複数の非空値が異なればsource errorにする。どれも宣言しないsourceは空文字とし、取得時刻を公開日時として代用しない。

`channel_pointer`を省略した場合、正規versionが各schemeのprerelease構文を持てば`prerelease`、それ以外は`stable`とする。指定したpointer先はstringの`stable|prerelease`、またはbooleanだけを受け、booleanは`true`を`stable`、`false`を`prerelease`へ写像する。数値や文字列への暗黙変換、未知stringのfallbackを行わない。

### 6.2 `json-index`

```toml
[platforms.version_source]
kind = "json-index"
url = "https://example.invalid/index.json"
index_items_pointer = "/index"
index_document_pointer = "/document-url"
max_documents = 16
document_lifecycle_pointer = "/support-phase"
items_pointer = "/releases"
item_flatten_pointer = "/sdks"
version_pointer = "/version"
assets_pointer = "/files"
max_items = 10000
cache_ttl = "24h"

[platforms.version_source.lifecycle_map]
active = "supported"
maintenance = "supported"
eol = "eol"
```

- `url`はindex文書。`index_items_pointer`はindex内の配列、`index_document_pointer`は各要素から子文書のHTTPS絶対URLを取り出すpointer。3件とも必須。
- 子文書URLのhostはindex文書のhost、または`artifact.redirect_hosts`と同じ規則で宣言した完全hostだけを許す。index応答から任意hostを動的に信頼しない。
- `max_documents`は必須で1以上、組込み上限32以下。重複URLは1回だけ取得する。子文書は宣言順に処理し、1件でも取得・parseに失敗したらcatalog全体をsource errorにする。部分catalogを公開しない。
- `items_pointer`以降のpointerは**各子文書へ**適用する。全子文書の結果を連結してcatalogを作る。
- `document_lifecycle_pointer`は任意で、子文書の**top-level**から1つの値を読み、その子文書由来の全itemへ同じlifecycleを与える。`lifecycle_pointer`（item相対）と同時指定できない。
- `lifecycle_map`は`document_lifecycle_pointer`または`lifecycle_pointer`と組で使う任意table。**どちらか一方を宣言したら必須**とし、片方だけの宣言を拒否する。sourceのstring値から`supported|eol|unknown`への写像を全件明示する。**mapに無い値はsource error**とし、黙って`unknown`へ倒さない。上流がenum値を増やした場合にlive smokeで検出するための規定である。
- `cache_ttl`はindex文書と全子文書へ同じ値を適用する。`source_identity`はindex文書のURLとする。

`required_tokens_pointer`と`required_tokens`は組で指定する。pointer先は一意string array、requiredはASCII stringの一意非空配列。required tokenが1件でもないversion itemはsource errorではなく現在platformで`installable=false/artifact-not-found`。Node.js index等、URL templateは作れるがplatform archiveの公開有無を別fieldで示すsourceに使う。

### 6.3 version変換

`version_regex`はRE2互換でnamed capture`version`をexactly 1件持つ。matchしないitemはsource layout違反としてrefreshを失敗させる。leading prefix除去等はregex captureだけで明示し、Goコードへtool分岐を置かない。

channelは§6.1の規則で、`channel_pointer`のstring/booleanを厳密に写像する。pointer省略時だけversion schemeのprerelease構文から導出する。EOLをchannelへ入れない。

lifecycleは次の優先順位で決める。gdtvm codeが公開日やversionの古さからEOLを推測しない。

1. §6.4のexact version override。
2. `lifecycle_pointer`（item相対）または`document_lifecycle_pointer`（子文書top-level）が読んだ値を`lifecycle_map`で写像した結果。mapに無い値はsource error。どちらのpointerも`json-index`でだけ使え、`lifecycle_map`と組でなければ宣言できない。写像先が無いpointerは全itemをsource errorにするだけで、lifecycleを決められないためである。
3. どれも無ければ`unknown`。`json` sourceのlifecycleは1と3だけで決まる。

### 6.4 lifecycle override

```toml
[[platforms.version_source.lifecycle_overrides]]
version = "18.20.8"
status = "eol"
evidence = "https://example.invalid/official-lifecycle"
assessed_at = 2026-08-07T00:00:00Z
```

override keyは`version`, `status`, `evidence`, `assessed_at`だけで全件必須。versionは正規完全version、同一source内で一意。status=`supported|eol`、evidenceはprovider/official projectのHTTPS一次資料、時刻はUTC RFC 3339。source lifecycle fieldと同じversionで矛盾するoverrideを拒否する。

matching source itemだけへ適用し、sourceにないoverrideはcatalog itemを合成せず`W_LIFECYCLE_OVERRIDE_UNUSED`として報告する。channelとlifecycleは独立なのでprereleaseへeol overrideも設定できる。registry review時に根拠を再確認し、根拠なしの一括「古いversion」判定をしない。static sourceはitem自身へlifecycleを書くためoverrideを禁止する。

### 6.5 asset field

`asset_fields`は`name`, `url`, `size`, `digest`, `digest_algorithm`, `os`, `arch`, `libc`, `published_at`, `release_tag`, `release_url`, `release_id`, `asset_id`からJSON pointerへのmap。`metadata_fields`は宣言済みmetadata keyからJSON pointer。値はstring、sizeだけ非負integer。IDもprecision lossを避けるためdecimal stringとして扱う。asset listがないsourceではartifact templateを使う。

catalog/Plan/receiptの`provider_release`は、選択assetの`release_tag`が非空ならその値、なければ`version_pointer`が読んだregex適用前のraw version stringを使う。static sourceはassetの必須`release_tag`を使う。取得時刻、URL pathの推測、tool ID分岐でprovider releaseを合成しない。

sourceの`digest`はalgorithm prefixなしのlowercase hexとして読む。algorithmはsourceの`digest_algorithm`（あれば）またはcheck sumの`algorithm`から決まり、gdtvmが内部で`<algorithm>:<hex>`へ正規化する。hex長がalgorithmと一致しない値を拒否する。

`metadata_fields`はURL/file templateの`{{metadata.<key>}}`だけに使う。catalog itemや表示へ任意metadataを持ち込む契約はschema 1にない。

### 6.6 static version

```toml
[[platforms.version_source.static_versions]]
version = "3.13.7"
channel = "stable"
lifecycle = "supported"
lifecycle_evidence = "https://example.invalid/official-lifecycle"
lifecycle_assessed_at = 2026-08-07T00:00:00Z
published_at = "2025-08-14T00:00:00Z"

[[platforms.version_source.static_versions.assets]]
name = "cpython-3.13.7+20250814-x86_64-pc-windows-msvc-install_only_stripped.tar.gz"
url = "https://github.com/example/releases/download/20250814/example.tar.gz"
size = 1
digest = "<64 lowercase hex>"
digest_algorithm = "sha256"
os = "windows"
arch = "amd64"
libc = "none"
release_tag = "20250814"
release_url = "https://github.com/example/releases/tag/20250814"
release_id = "123"
asset_id = "456"
published_at = "2025-08-14T00:00:00Z"
```

`static_versions` itemの許可keyは`version`, `channel`, `lifecycle`, `lifecycle_evidence`, `lifecycle_assessed_at`, `published_at`, `assets`。全件必須。channel=`stable|prerelease`、lifecycle=`supported|eol|unknown`。両者は独立であり全6組合せを表現できる。evidenceはprovider/official projectのHTTPS一次資料、assessment/publication時刻はUTC RFC 3339。unknownでも「不明と判断した調査根拠」をevidenceへ残す。versionは正規完全versionで一意。asset itemの許可keyは前節のasset field集合で、全件必須。sizeは正整数、`digest_algorithm`は`sha256|sha512`、`digest`はalgorithmに応じた64または128 lowercase hex、URLはHTTPS、IDは非負decimal string。

static sourceはversion itemをfile記載順で解釈せず、正規version byteで一意検査してcomparison keyでsortする。同一versionのasset identityをregistry更新で差し替えると既存receiptの再現性が変わるため禁止し、version grammar/schema改訂を要求する。

`version_source`はplatform配下にあるため、static sourceは両platformへ同じversion集合を記述する。registry validatorは両platformの正規version集合が完全一致することを検査し、片方だけの更新漏れを拒否する。

## 7. primary artifact

```toml
[platforms.artifact]
id = "primary"
source = "template"
url = "https://example.invalid/{{version}}/tool.zip"
file = "tool-{{version}}.zip"
format = "zip"
size = 0

[platforms.artifact.selector]
name_regex = "^tool-(?P<version>[0-9.]+)-windows-amd64[.]zip$"

[platforms.artifact.checksum]
kind = "text-file"
url = "https://example.invalid/{{version}}/SHASUMS256.txt"
```

### 7.1 artifact key

許可keyは`id`, `source`, `url`, `file`, `format`, `size`, `selector`, `checksum`, `redirect_hosts`。primary 1件だけ、id=`primary`固定。

- `source=template|asset`。
- templateはrender後URL/fileを使う。assetはversion itemのassetsからselectorでexactly 1件選ぶ。
- `source=asset`の`url`/`file`は、**空なら選択assetの`url`/`name`を使い、非空なら選択assetを`{{asset.<field>}}`で参照できるtemplateとしてrenderする**。upstreamがasset listにdownload URLを載せず、file名からURLを組み立てる配布元（Go）に使う。selectorはどちらの場合も必須で、artifactの同一性はselectorが決める。
- `format=zip|tar.gz`。schema 1はこの2形式だけを扱う。
- `size=0`はunknown、正値はexpected size。download responseと一致必須。
- `redirect_hosts`はartifact URLとchecksum URLに共通の追加許可host。省略時はそれぞれの元hostだけ。各値はASCII lowercase完全hostでwildcard不可。redirect先を最終URLから動的allowlist化しない。

selectorの許可keyは`name_regex`, `os`, `arch`, `libc`。指定条件すべてに一致するassetをexactly 1件要求する。0件はそのversionを`installable=false/artifact-not-found`、2件以上はdefinition/source error。source順で選ばない。sourceのassetが持つ他のfield（kind、種別等）で絞り込みたい場合は`name_regex`で表現する。

URL/file templateは`{{version}}`と宣言済み`{{metadata.<key>}}`, `{{asset.<field>}}`だけ。URL componentは値をpercent encode、fileはbasename grammarを検査する。template値が欠落したitemをinstallable扱いしない。

### 7.2 checksum

checksum keyは`kind`, `url`, `line_format`, `algorithm`だけ。kindは次の2値。

| kind | 契約 |
|---|---|
| `asset-field` | source assetのdigestを使う。sourceにalgorithm fieldがあればその値と`algorithm`が完全一致。なければdefinitionの`algorithm`必須 |
| `text-file` | URLのUTF-8 textからartifact basenameとdigestのexact 1行を得る |

`text-file`の`line_format`はschema 1で`sha256-space-filename`だけ。`<64 hex><1個以上ASCII space><optional '*'><basename>`を受け、BOM、NUL、path、duplicate、別algorithmを拒否する。file最大2 MiB。

`algorithm`は`sha256|sha512`。`asset-field`の上記条件だけに許し、他kindやsource algorithm fieldとの同時指定を拒否する。`text-file`は`line_format`がalgorithmを含むため`algorithm`を書かない。

解決したdigestは内部で`<algorithm>:<hex>`へ正規化し、catalog、Plan、receiptへこの形式で保存する（[04-storage-and-data.md](04-storage-and-data.md)§7）。hex長がalgorithmと一致しない値を拒否する。

checksumはPlan前にdigestを解決し、download直後に照合する。不一致は`--yes|--force|--offline`で回避できない。**v0.1はchecksumを公開しないartifactを扱わない**。checksumを提供しないproviderのtoolを追加する場合は[15-deferred.md](15-deferred.md) D-06の再導入gateを先に完了する。

署名関連fieldはunknown keyとして拒否する。

## 8. typed storage

```toml
[[platforms.storage]]
id = "global-tools"
kind = "global-bin"
scope = "tool"
path = "bin"
purge = "explicit"
```

許可keyは`id`, `kind`, `scope`, `path`, `purge`。全件必須。

- kind=`config|content-cache|build-cache|global-bin|global-packages|runtime-data`。
- scope=`tool|version`。
- pathはstorage root内POSIX relative path、空/absolute/`.`/`..`禁止。
- purge=`retain|explicit|with-version`。tool scopeは`retain|explicit`だけで通常uninstall時に保持し、`explicit`だけ最後のmanaged versionで`--purge-shared`対象にできる。version scopeは`with-version`だけで、対象version uninstall時にversion directoryと一緒に削除する。

同一platform内でrender後pathが重複/包含しないこと。payload、receipt、current、shim、stateと交差しないこと。runtime environmentは`{{storage.<id>}}`で参照する。未参照storageも作成できるがPlanへ目的を表示しreceiptへ保存する。

標準definitionのscopeは[07-registry-and-tools.md](07-registry-and-tools.md)の有限表とcontract testで一致させる。engineはtool IDを見てscopeを変更しない。

## 9. install

```toml
[platforms.install]
strip_components = 1
```

許可keyは`strip_components`だけで必須。engineは全toolで次の固定順序を実行し、definitionは展開parameterだけを与える。

1. primary artifactをoperation download areaへ取得する。
2. providerが宣言したalgorithm（`sha256|sha512`）の解決済みdigestと照合する。
3. `zip|tar.gz`を安全にstagingの`payload`へ展開する。

`strip_components`はintegerの`0|1`とし、除去後に空/衝突となる場合は拒否する。schema 1はentry filterを持たず、安全検査を通過したarchive entryをすべて展開する。2階層以上の除去が必要なartifactはv0.1の標準registryへ採用しない。

extractはentry一覧を先に検査し、absolute、`..`、NUL、ADS、symlink/hardlink/device、非NFC、Windows case collision、予約名、entry/size/ratio上限を拒否する。完成payloadへのcommitはengine固定処理であり、definitionから制御できない。

## 10. runtime

```toml
[[platforms.runtime.commands]]
name = "go"
target = "{{payload}}/bin/go.exe"
args = []
environment_profile = "default"
required = true
working_directory = "inherit"
passthrough_signals = true

[[platforms.runtime.environment]]
id = "default"
path_prepend = ["{{payload}}/bin", "{{storage.global-tools}}"]
path_append = []
unset = []
override_allowed = []
shell_export = ["GOROOT", "GOBIN"]

[platforms.runtime.environment.set]
GOROOT = "{{payload}}"
GOBIN = "{{storage.global-tools}}"
```

### 10.1 command

command keyは`name`, `target`, `args`, `environment_profile`, `required`, `working_directory`, `passthrough_signals`。全件必須。launcherはschema 1でnativeだけなのでfieldを設けない。

- targetは`{{payload}}`配下のregular executable、または固定interpreterとして別required command targetと同じ実体を指せる。
- argsはliteralまたはentry全体が`{{payload}}`/storage内path templateとその子pathであるものだけ。path templateへliteral prefix/suffixを連結しない。shell文字列、環境展開、command substitutionは禁止し、Planではliteral/pathを`PlanArg`へ分ける。
- working_directory=`inherit|payload`。
- nameはplatform内一意。[07-registry-and-tools.md](07-registry-and-tools.md)のrequired command集合と一致する。
- Node npm/npxはpayload node実体＋同梱JS entrypoint、Python pip/pip3はpayload python実体＋`-m pip`で表す。

### 10.2 environment

profile keyは`id`, `path_prepend`, `path_append`, `set`, `unset`, `override_allowed`, `shell_export`。配列/mapは空可。command参照先はexactly 1件。

- set値はliteral、`{{payload}}`、`{{storage.<id>}}`とその子pathだけ。
- unset/set重複禁止。Windows env keyはcase-insensitiveに一意。
- `override_allowed`は親環境値を優先できるkey、`shell_export`はshim経由の子processへ渡す公開値。未列挙keyはreceipt値が親を上書きする。
- PATHはlogical path配列としてmergeし、canonical重複を除去する。raw separator文字列をdefinitionへ書かない。

## 11. validation

```toml
[[platforms.validation.probes]]
id = "version"
runtime_command = "go"
args = ["version"]
stream = "stdout"
expect = "version"
regex = "go version go(?P<version>[0-9]+[.][0-9]+(?:[.][0-9]+|(?:beta|rc)[1-9][0-9]*)?)"
expected_version = "{{version}}"
timeout = "30s"
required = true
```

probe keyは`id`, `runtime_command`, `args`, `stream`, `expect`, `regex`, `expected_version`, `expected_root`, `required_paths`, `timeout`, `required`。`id`, `runtime_command`, `args`, `stream`, `expect`, `timeout`, `required`は全件必須。stream=`stdout|stderr|combined`。expectは次のいずれか。

| expect | 追加契約 |
|---|---|
| `version` | regexとexpected_version必須。RE2 named capture`version`exactly 1件をtool schemeで正規化し、`{{version}}`と一致 |
| `success` | exit code 0を要求。regexは指定時に完全matchを1件以上要求。expected fields禁止 |
| `path-within` | regexとexpected_root必須。named capture`path`exactly 1件をabsolute path化し、`payload\|probe-temp\|storage.<id>`の指定root内にcontain |

`required_paths`は`{{payload}}`, `{{probe_temp}}`, `{{storage.<id>}}`配下のpath templateで、probe成功直後に指定種別の存在を要求する。entryは`file:<template>|directory:<template>`の文字列として記述し、unknown prefixを拒否する。

argsはliteralに加えて、entry全体として`{{payload}}`, `{{probe_temp}}`, `{{storage.<id>}}`とその子pathを使える。path templateへのliteral prefix/suffix連結は拒否し、render後はPlanのtyped `PlanArg`へする。probeごとに空のowner-only probe tempを作り、成功/失敗/cancel後にengineが削除する。**probeのcwdはその probe temp とし、呼出し元のcurrent directoryを継承しない。** 利用者のproject file（`global.json`、`.nvmrc`等）がprobe結果を変えないようにするための規定である。timeoutは1s～2m。required probe failureはcommit前にinstall全体を失敗させる。

Pythonはversion、`python -m pip --version`、stdlib import、operation tmpへのvenv作成とrequired path検査を持てる。利用者projectやtool storageへprobe fileを書かない。

## 12. templateとpath

許可rootは`{{version}}`, `{{platform.id}}`, `{{payload}}`, `{{probe_temp}}`, `{{storage.<id>}}`, version sourceで宣言したmetadata/assetだけ。`{{probe_temp}}`はvalidation probe内だけ。未知変数、再帰展開、function、condition、shell evaluationを禁止する。render結果は32 KiB、path component 255 byte、URL 8 KiBの組込み上限。

logical pathはPOSIX slashで記述し、OS adapterがseparatorへ変換する。render後にcanonical containmentを再検査する。URL contextとpath contextを混同せず、それぞれのescape規則を適用する。

### 12.1 構造上限

定義の構造上限の正本は[04-storage-and-data.md](04-storage-and-data.md)§21とする。数値をここで再掲せず、同表の`definition`関連行を全definitionへ適用する。上限はconfig/definitionから拡大できず、重複をcount前後どちらでも黙って除去しない。

## 13. 検証順序

1. byte/TOML/unknown/duplicate制限。
2. schema/schema_id。
3. identifier、URL、enum、型、上限。
4. platform tupleと対応範囲、`license_notice` message ID。
5. version source kindごとの許可key、pointer/regex/field契約、`channel_pointer`のscalar型、`lifecycle_map`の全件明示、`item_flatten_pointer`の1段制約と親公開日時継承。
6. artifact selector/checksum/template、digest algorithmとhex長の一致。
7. storage path/scope/衝突。
8. install parameter。
9. runtime command/environment/storage参照。
10. probe参照とversion capture。
11. registry全体のID/alias/command衝突と[07-registry-and-tools.md](07-registry-and-tools.md) contract。

errorはdefinition relative path、line/column、field path、stable reason codeを返す。複数errorを集約しても上限100件で停止する。

## 14. 互換性

clientは`schema=1`だけを読む。未知major/minorを推測して受理しない。registryはclientと同じreleaseに同梱するため、schema変更はclient version、fixture、本章の正規例、4標準definition、receipt/catalog contract、[14-maintenance.md](14-maintenance.md)手順を同時に更新する。未使用fieldを互換性予約として追加しない。

## 15. 正規例: Node.js `windows-amd64`

本節はschema 1でtool固有Go分岐を作らずに1 platformを完全記述した例である。実registryと本例が食い違う場合、実装で補完せず仕様/definition/fixtureを同期修正する。

```toml
schema = 1
schema_id = "https://github.com/kznagamori/go_dev_tool_version_manager/schemas/tool-definition/v1"

[tool]
id = "node"
name = "Node.js"
aliases = ["nodejs"]
description = "Node.js JavaScript runtime"
homepage = "https://nodejs.org/"
license = "MIT"
version_scheme = "semver"

[[platforms]]
id = "windows-amd64"
os = "windows"
arch = "amd64"
libc = "none"
artifact_kind = "official"

[platforms.provider]
name = "Node.js project"
homepage = "https://nodejs.org/"
license = "MIT"

[platforms.version_source]
kind = "json"
url = "https://nodejs.org/dist/index.json"
items_pointer = ""
version_pointer = "/version"
version_regex = "^v(?P<version>[0-9]+[.][0-9]+[.][0-9]+(?:-[0-9A-Za-z.-]+)?)$"
published_at_pointer = "/date"
required_tokens_pointer = "/files"
required_tokens = ["win-x64-zip"]
max_items = 10000
cache_ttl = "24h"

[[platforms.version_source.lifecycle_overrides]]
version = "18.20.8"
status = "eol"
evidence = "https://github.com/nodejs/Release"
assessed_at = 2026-08-07T00:00:00Z

[platforms.artifact]
id = "primary"
source = "template"
url = "https://nodejs.org/dist/v{{version}}/node-v{{version}}-win-x64.zip"
file = "node-v{{version}}-win-x64.zip"
format = "zip"
size = 0

[platforms.artifact.checksum]
kind = "text-file"
url = "https://nodejs.org/dist/v{{version}}/SHASUMS256.txt"
line_format = "sha256-space-filename"

[platforms.install]
strip_components = 1

[[platforms.storage]]
id = "config"
kind = "config"
scope = "tool"
path = "config"
purge = "explicit"

[[platforms.storage]]
id = "cache"
kind = "content-cache"
scope = "tool"
path = "cache"
purge = "explicit"

[[platforms.storage]]
id = "history"
kind = "runtime-data"
scope = "tool"
path = "history"
purge = "explicit"

[[platforms.storage]]
id = "global-packages"
kind = "global-packages"
scope = "version"
path = "global-packages"
purge = "with-version"

[[platforms.runtime.commands]]
name = "node"
target = "{{payload}}/node.exe"
args = []
environment_profile = "default"
required = true
working_directory = "inherit"
passthrough_signals = true

[[platforms.runtime.commands]]
name = "npm"
target = "{{payload}}/node.exe"
args = ["{{payload}}/node_modules/npm/bin/npm-cli.js"]
environment_profile = "default"
required = true
working_directory = "inherit"
passthrough_signals = true

[[platforms.runtime.commands]]
name = "npx"
target = "{{payload}}/node.exe"
args = ["{{payload}}/node_modules/npm/bin/npx-cli.js"]
environment_profile = "default"
required = true
working_directory = "inherit"
passthrough_signals = true

[[platforms.runtime.environment]]
id = "default"
path_prepend = ["{{payload}}", "{{storage.global-packages}}"]
path_append = []
unset = []
override_allowed = []
shell_export = ["NPM_CONFIG_PREFIX", "NPM_CONFIG_USERCONFIG", "NPM_CONFIG_CACHE"]

[platforms.runtime.environment.set]
NPM_CONFIG_USERCONFIG = "{{storage.config}}/npmrc"
NPM_CONFIG_CACHE = "{{storage.cache}}"
NPM_CONFIG_PREFIX = "{{storage.global-packages}}"
NODE_REPL_HISTORY = "{{storage.history}}/repl_history"

[[platforms.validation.probes]]
id = "version"
runtime_command = "node"
args = ["--version"]
stream = "stdout"
expect = "version"
regex = "^v(?P<version>[0-9]+[.][0-9]+[.][0-9]+(?:-[0-9A-Za-z.-]+)?)$"
expected_version = "{{version}}"
timeout = "30s"
required = true

[[platforms.validation.probes]]
id = "npm"
runtime_command = "npm"
args = ["--version"]
stream = "stdout"
expect = "success"
timeout = "60s"
required = true

[[platforms.validation.probes]]
id = "npx"
runtime_command = "npx"
args = ["--version"]
stream = "stdout"
expect = "success"
timeout = "60s"
required = true
```

## 16. Linux platformと他toolの差分

### 16.1 Node.js `linux-amd64-glibc`

`libc="glibc"`、artifactを`https://nodejs.org/dist/v{{version}}/node-v{{version}}-linux-x64.tar.gz`、`format="tar.gz"`、`required_tokens=["linux-x64"]`へ変える。command targetは`{{payload}}/bin/node`、npm/npx引数は`{{payload}}/lib/node_modules/npm/bin/npm-cli.js`と`npx-cli.js`。`path_prepend`は`{{payload}}/bin`と`{{storage.global-packages}}/bin`。

Node.js公式は同じ`SHASUMS256.txt`で`.tar.gz`と`.tar.xz`の両方を配布する。schema 1は`.tar.gz`だけを扱い、圧縮形式のための外部moduleを持たない。

### 16.2 Go

`version_scheme="go"`、`artifact_kind="official"`、`kind="json"`で`https://go.dev/dl/?mode=json&include=all`を読む。asset listから選ぶため`source="asset"`、`checksum.kind="asset-field"`とし、`digest` pointerからSHA-256を取る。sourceはdigest algorithm fieldを持たないため`algorithm="sha256"`を明示する。

```toml
[platforms.version_source]
kind = "json"
url = "https://go.dev/dl/?mode=json&include=all"
items_pointer = ""
version_pointer = "/version"
version_regex = "^go(?P<version>[0-9]+[.][0-9]+(?:[.][0-9]+)?(?:(?:beta|rc)[1-9][0-9]*)?)$"
channel_pointer = "/stable"
assets_pointer = "/files"
max_items = 10000
cache_ttl = "24h"

[platforms.version_source.asset_fields]
name = "/filename"
size = "/size"
digest = "/sha256"
os = "/os"
arch = "/arch"

[platforms.artifact]
id = "primary"
source = "asset"
url = "https://go.dev/dl/{{asset.name}}"
file = "{{asset.name}}"
format = "zip"
size = 0
redirect_hosts = ["dl.google.com"]

[platforms.artifact.selector]
name_regex = "^go(?P<version>[0-9][0-9A-Za-z.]*)[.]windows-amd64[.]zip$"
os = "windows"
arch = "amd64"

[platforms.artifact.checksum]
kind = "asset-field"
algorithm = "sha256"
```

Go JSONの`files[]`はdownload URLを持たずfile名だけを載せるため、`url`と`file`を選択assetのtemplateとしてrenderする。hostは`go.dev`で、`dl.google.com`へredirectする。

`channel_pointer`はGo JSONの`stable` booleanを`stable|prerelease`へmapする。同じentryにinstaller/source archiveが並ぶため、`name_regex`で目的のarchiveだけをexactly 1件に絞る。selectorはsourceの`kind` fieldを直接参照しない。

Linuxは`name_regex`を`^go(?P<version>[0-9][0-9A-Za-z.]*)[.]linux-amd64[.]tar[.]gz$`、`os="linux"`、`format="tar.gz"`へ変える。

### 16.3 Python

`version_scheme="python"`、`artifact_kind="third-party"`、`kind="static"`。同じCPython versionのprovider buildを将来差し替えない。

```toml
[platforms.provider]
name = "Astral python-build-standalone"
repository = "https://github.com/astral-sh/python-build-standalone"
homepage = "https://github.com/astral-sh/python-build-standalone"
license = "MPL-2.0"
adoption_reason = "provider.python.standalone_reason"

[platforms.version_source]
kind = "static"
max_items = 10000

[[platforms.version_source.static_versions]]
version = "3.13.7"
channel = "stable"
lifecycle = "supported"
lifecycle_evidence = "https://devguide.python.org/versions/"
lifecycle_assessed_at = 2026-08-07T00:00:00Z
published_at = "2025-08-14T00:00:00Z"

[[platforms.version_source.static_versions.assets]]
name = "cpython-3.13.7+20250814-x86_64-pc-windows-msvc-install_only_stripped.tar.gz"
url = "https://github.com/astral-sh/python-build-standalone/releases/download/20250814/cpython-3.13.7%2B20250814-x86_64-pc-windows-msvc-install_only_stripped.tar.gz"
size = 1
digest = "<64 lowercase hex>"
digest_algorithm = "sha256"
os = "windows"
arch = "amd64"
libc = "none"
release_tag = "20250814"
release_url = "https://github.com/astral-sh/python-build-standalone/releases/tag/20250814"
release_id = "0"
asset_id = "0"
published_at = "2025-08-14T00:00:00Z"

[platforms.artifact]
id = "primary"
source = "asset"
url = ""
file = ""
format = "tar.gz"
size = 0
redirect_hosts = ["release-assets.githubusercontent.com"]

[platforms.artifact.selector]
name_regex = "^cpython-(?P<version>[0-9]+[.][0-9]+[.][0-9]+(?:a[1-9][0-9]*|b[1-9][0-9]*|rc[1-9][0-9]*)?)[+][0-9]{8}-x86_64-pc-windows-msvc-install_only_stripped[.]tar[.]gz$"
os = "windows"
arch = "amd64"

[platforms.artifact.checksum]
kind = "asset-field"
```

static assetは`digest_algorithm="sha256"`を自身で持つため、checksumへ`algorithm`を書かない。Linuxは`os="linux"`, `libc="glibc"`、asset nameを`x86_64-unknown-linux-gnu-install_only_stripped.tar.gz`へ変える。

Python probeはversion、`python -m pip --version`、`ssl`/`sqlite3`/`venv`のimport、`{{probe_temp}}`へのvenv作成とrequired path確認、`sys.prefix`のpayload内containを持つ。required pathはWindowsが`file:{{probe_temp}}/venv/Scripts/python.exe`、Linuxが`file:{{probe_temp}}/venv/bin/python`であり、engineがPython IDからscript名を推測しない。

### 16.4 .NET SDK

`version_scheme="semver"`、`artifact_kind="official"`、`kind="json-index"`。Windows配布物はMITではなく独自EULAのため`license_notice`を宣言する。

```toml
[tool]
id = "dotnet"
name = ".NET SDK"
aliases = ["dotnet-sdk"]
description = ".NET SDK (dotnet CLI, compilers, shared runtimes)"
homepage = "https://dotnet.microsoft.com/"
license = "MIT"
version_scheme = "semver"

[[platforms]]
id = "windows-amd64"
os = "windows"
arch = "amd64"
libc = "none"
artifact_kind = "official"
license_notice = "license.dotnet.windows_library_license"

[platforms.provider]
name = "Microsoft"
repository = "https://github.com/dotnet/sdk"
homepage = "https://dotnet.microsoft.com/"
license = "LicenseRef-dotnet-library"

[platforms.version_source]
kind = "json-index"
url = "https://builds.dotnet.microsoft.com/dotnet/release-metadata/releases-index.json"
index_items_pointer = "/releases-index"
index_document_pointer = "/releases.json"
max_documents = 32
document_lifecycle_pointer = "/support-phase"
items_pointer = "/releases"
item_flatten_pointer = "/sdks"
item_parent_published_at_pointer = "/release-date"
version_pointer = "/version"
version_regex = "^(?P<version>[0-9]+[.][0-9]+[.][0-9]+(?:-[0-9A-Za-z.-]+)?)$"
assets_pointer = "/files"
max_items = 10000
cache_ttl = "24h"

[platforms.version_source.lifecycle_map]
preview = "supported"
"go-live" = "supported"
active = "supported"
maintenance = "supported"
eol = "eol"

[platforms.version_source.asset_fields]
name = "/name"
url = "/url"
digest = "/hash"

[platforms.artifact]
id = "primary"
source = "asset"
url = ""
file = ""
format = "zip"
size = 0

[platforms.artifact.selector]
name_regex = "^dotnet-sdk-win-x64[.]zip$"

[platforms.artifact.checksum]
kind = "asset-field"
algorithm = "sha512"

[platforms.install]
strip_components = 0

[[platforms.storage]]
id = "cli-home"
kind = "runtime-data"
scope = "version"
path = "cli-home"
purge = "with-version"

[[platforms.storage]]
id = "nuget-packages"
kind = "content-cache"
scope = "tool"
path = "nuget-packages"
purge = "explicit"

[[platforms.storage]]
id = "nuget-http-cache"
kind = "content-cache"
scope = "tool"
path = "nuget-http-cache"
purge = "explicit"

[[platforms.storage]]
id = "nuget-plugins-cache"
kind = "content-cache"
scope = "tool"
path = "nuget-plugins-cache"
purge = "explicit"

[[platforms.runtime.commands]]
name = "dotnet"
target = "{{payload}}/dotnet.exe"
args = []
environment_profile = "default"
required = true
working_directory = "inherit"
passthrough_signals = true

[[platforms.runtime.environment]]
id = "default"
path_prepend = ["{{payload}}"]
path_append = []
unset = []
override_allowed = []
shell_export = ["DOTNET_ROOT", "NUGET_PACKAGES"]

[platforms.runtime.environment.set]
DOTNET_ROOT = "{{payload}}"
DOTNET_CLI_HOME = "{{storage.cli-home}}"
DOTNET_CLI_TELEMETRY_OPTOUT = "1"
DOTNET_NOLOGO = "1"
NUGET_PACKAGES = "{{storage.nuget-packages}}"
NUGET_HTTP_CACHE_PATH = "{{storage.nuget-http-cache}}"
NUGET_PLUGINS_CACHE_PATH = "{{storage.nuget-plugins-cache}}"

[[platforms.validation.probes]]
id = "version"
runtime_command = "dotnet"
args = ["--version"]
stream = "stdout"
expect = "version"
regex = "^(?P<version>[0-9]+[.][0-9]+[.][0-9]+(?:-[0-9A-Za-z.-]+)?)$"
expected_version = "{{version}}"
timeout = "120s"
required = true

[[platforms.validation.probes]]
id = "list-sdks"
runtime_command = "dotnet"
args = ["--list-sdks"]
stream = "stdout"
expect = "success"
timeout = "120s"
required = true
```

Linux platformは`os="linux"`, `libc="glibc"`、`license_notice`を**宣言しない**（Linux配布物はMIT）、`provider.license="MIT"`、`name_regex="^dotnet-sdk-linux-x64[.]tar[.]gz$"`、`format="tar.gz"`、command targetを`{{payload}}/dotnet`へ変える。

補足:

- upstream metadataの`files[].name`はversionを含まない固定名（`dotnet-sdk-win-x64.zip`）で、versionはURL側にある。selectorは`name_regex`のexact一致でexactly 1件へ絞る。`rid`はselector keyにないため参照しない。
- `files[].hash`はSHA-512（128 hex）で、sourceにalgorithm fieldがないため`checksum.algorithm="sha512"`を明示する。
- archiveにtop-level directoryがないため`strip_components=0`。
- `DOTNET_CLI_TELEMETRY_OPTOUT`と`DOTNET_NOLOGO`は`override_allowed`へ入れない。[01-requirements.md](01-requirements.md)§9「telemetryを追加しない」に従い、利用者が親環境から無効化を解除できないようにする。
