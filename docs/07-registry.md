# 標準定義レジストリ仕様

## 1. 配置と配布単位

標準定義はclient sourceと同じ開発branchのrepository rootにある`/registry/`で管理する。専用branch、orphan branch、registry専用tag、別repositoryを使用しない。

source tree:

```text
/
├─ VERSION
├─ registry/
├─ cmd/
├─ internal/
├─ docs/
├─ README.md
├─ USER_GUIDE.md
└─ LICENSE
```

repositoryの`.gitattributes`は少なくとも`registry/**/*.toml`, `registry/**/*.json`, `registry/**/*.txt`, `registry/**/*.sh`をLFへ固定し、binary形式のupstream keyだけを`-text`とする。release workflowは13章どおりtag commitのGit object bytesを正とし、runner checkoutの自動改行変換へ依存しない。

clientとregistryは同じclient release tag `vYYYY.mm.DD.XX`のcommitからGitHub Actionsでbuild・検証・packageする。registryだけを独立更新せず、定義変更は新しいclient releaseとして公開する。

manifest、manifest署名、registry file一覧hash、registry用鍵、`registry-v<SemVer>` tag、GitHubからregistryだけを取得するbootstrap/update処理は使用しない。release archive全体をSHA-256で照合し、そのarchiveに含まれるregistryをclientと同じ配布単位として扱う。

## 2. registry構造

```text
registry/
├─ registry.toml
├─ schemas/
│  ├─ tool-definition-v1.json
│  └─ helper-definition-v1.json
├─ tools/
│  ├─ android-sdk.toml
│  └─ ...
├─ helpers/
│  ├─ seven-zip.toml
│  └─ wix.toml
├─ scripts/
│  └─ <tool-id>/...
├─ messages/
│  ├─ ja.toml
│  └─ en.toml
├─ licenses/
│  └─ license-ref-7zip-unrar.txt
├─ upstream-keys/
│  └─ <provider-key-file>
└─ revoked.toml
```

`registry.toml`はfile manifestではなく、同梱registryのschema情報だけを持つ。

```toml
schema = 1
tool_definition_schema = 1
helper_definition_schema = 1
minimum_client_version = "2026.07.24.00"
```

許可keyは上記4件と任意の`maximum_client_version`だけである。client versionは`YYYY.mm.DD.XX`として13章の規則で比較する。release時に`minimum_client_version`がreleaseの`/VERSION`を超える構成を拒否する。`maximum_client_version`省略は上限なしを表す。

schemaの規範は本仕様であり、registry内JSON SchemaはCI・editor補助とする。食い違い時は仕様を先に改訂し、同じ変更でschemaとtestを更新する。

registry rootで許可するentryは`registry.toml`, `revoked.toml`, `schemas/`, `tools/`, `helpers/`, `scripts/`, `messages/`, `licenses/`, `upstream-keys/`だけである。rootのfile 2件と`schemas`, `tools`, `helpers`, `messages`は必須、その他のdirectoryは参照fileがある場合だけ作成し、空directoryをarchiveへ含めない。

- `schemas/`は`tool-definition-v<schema>.json`と`helper-definition-v<schema>.json`だけを許す。
- `tools/`と`helpers/`は正規IDと同じbasenameの`.toml` regular fileだけを許し、subdirectoryを禁止する。
- `messages/`はschema 1では`ja.toml`と`en.toml`だけを許す。
- `licenses/`はdefinition/helperが参照するUTF-8 text regular fileだけを許し、未参照fileを拒否する。
- `upstream-keys/`はsignature定義が参照する公開鍵regular fileだけを許し、private key marker、未参照file、同一fingerprint重複を拒否する。
- `scripts/<tool-id>/`はそのtool definitionが参照するUTF-8 script regular fileだけを許す。NUL、BOM、1 MiB超、未参照file、別toolからの参照を拒否し、拡張子と06章で宣言したinterpreterを一致させる。

すべてのpath componentはASCII lowercaseの正規表現`^[a-z0-9][a-z0-9._-]{0,127}$`に一致させ、relative path全体をPOSIX `/`区切り・512 bytes以下とする。これによりOS間のcase/Unicode差を排除し、07章8節のpath正規化と衝突検査も通す。TOML、JSON、message、license、scriptはUTF-8 BOMなし・LFをsource正規形とし、runtime parserがBOM/CRLFを許すかは各schemaの規則に従う。registry内に生成cache、editor backup、dotfileを含めない。

## 3. 初期registryの標準定義

初回releaseには旧`anyvm_win`が対応していた全17ツールを登録する。各tool fileは12章の同名節を規範とし、06章の完全なtool definition TOMLとして作成する。

| registry path | 正規tool ID | 入力alias | 旧対応名 |
|---|---|---|---|
| `tools/android-sdk.toml` | `android-sdk` | `androidsdk` | `AndroidSDKVm` |
| `tools/bazel.toml` | `bazel` | なし | `BazelVm` |
| `tools/cmake.toml` | `cmake` | なし | `CMakeVm` |
| `tools/dart.toml` | `dart` | なし | `DartVm` |
| `tools/dotnet.toml` | `dotnet` | `.net` | `dotnetVm` |
| `tools/flutter.toml` | `flutter` | なし | `FlutterVm` |
| `tools/go.toml` | `go` | なし | `GoVm` |
| `tools/gradle.toml` | `gradle` | なし | `GradleVm` |
| `tools/jdk.toml` | `jdk` | `java` | `JDKVm` |
| `tools/kotlin.toml` | `kotlin` | なし | `KotlinVm` |
| `tools/llvm.toml` | `llvm` | なし | `LLVMVm` |
| `tools/mingw.toml` | `mingw` | なし | `MinGWVm` |
| `tools/ninja.toml` | `ninja` | なし | `NinjaVm` |
| `tools/node.toml` | `node` | `nodejs` | `NodejsVm` |
| `tools/python.toml` | `python` | なし | `PythonVm` |
| `tools/rust.toml` | `rust` | なし | `RustVm` |
| `tools/winlibs.toml` | `winlibs` | なし | `WinLibsVm` |

各platform recipeはID、version discovery、artifact、checksum、install step、runtime command、environment、dependency、warning、probe、unsupported reasonをTOMLへ完全記載する。clientがpayloadを走査してcommandや環境を推測してはならない。

外部programはbinaryをregistryへ直接格納せず、取得・検証・展開・entrypointをhelper definitionに記載する。

| registry path | helper ID | 利用tool | 用途 |
|---|---|---|---|
| `helpers/seven-zip.toml` | `seven-zip` | `llvm`, `mingw`, `winlibs` | 7zおよびself-extracting配布物の展開 |
| `helpers/wix.toml` | `wix` | `python`（Windows） | Python installerを処理する`dark.exe` |

hookが必要な場合だけ`scripts/<tool-id>/`へ配置する。組込みstepで表現できる処理にscriptを使用しない。上流署名鍵とSPDX `LicenseRef-*`本文は対応directoryへ同梱する。

## 4. `revoked.toml`

失効がなくてもfileを必須とし、初期値は次とする。

```toml
schema = 1
artifacts = []
definitions = []
```

artifact entryは`tool_id`, `version`, `platform_id`, `variant`, `artifact_sha256`, `reason`, `severity`, `advisory_url`, `revoked_at`, `replacement_version`を持つ。definition entryは`path`, `definition_sha256`, `reason`, `severity`, `advisory_url`, `revoked_at`を持つ。任意値はfield省略で表す。

digestは64桁小文字hex、severityは`warning|error|critical`、時刻はUTC RFC 3339、pathは安全な相対POSIX pathとする。完全重複を拒否し、同じ対象の複数entryは最も高いseverityを適用する。

- `warning`: install前警告、doctor warning。
- `error`: 新規install拒否、既存installをdoctor error。
- `critical`: 新規installとruntime起動を拒否し、selectionを無効扱いにする。

payloadを自動削除しない。失効はoffline、`--force`、旧catalogで回避できない。緊急失効もregistry fileだけを別配布せず、新しいclient releaseを発行する。

## 5. source validation

pull requestとrelease workflowは`/registry/`に対して次を検査する。

1. `registry.toml`、全TOML、JSON Schemaをstrict parseする。
2. 17 tool、2 helper、ja/en message、必要license、upstream key、revocationの存在を確認する。
3. tool ID、alias、command owner、helper、dependency DAG、conflictを横断検査する。
4. platform recipeが有限のruntime command、artifact、checksum、install step、required probeを持つことを確認する。
5. URLがHTTPSで、redirect host、checksum、third-party policyに適合することを確認する。
6. script path、内容hash、interpreter、writes、timeoutを確認する。
7. registry内にbinary、private key、secret、symlink、submodule、生成途中fileがないことを確認する。
8. Windows/Linuxのcontract fixtureと最新stable・旧stableのcatalog解決を実行する。

registry file個別の署名・hash一覧は生成しない。Git commit review、GitHub Actions、archive SHA-256、binary埋込みregistry tree hashが配布・構成の整合性を担当する。

## 6. build・test時の配置

sourceから実行するtestはrepository rootの`registry/`をfixtureとして直接読める。release配置を検証するintegration/E2Eでは、実行fileを置く一時distribution rootへ次をcopyする。

```text
<test-distribution-root>/
├─ gdtvm[.exe]
├─ gdtvm.toml
├─ registry/
├─ README.md
├─ USER_GUIDE.md
└─ LICENSE
```

copy元は同じcheckoutのfileに限定し、test開始時に`/VERSION`と`registry/registry.toml`のclient互換性を検査する。testがsource registryを書き換えないよう、test distribution側を独立temporary directoryにする。

unit testはFileSystem portでregistry rootを注入できるようにする。productionでは実行fileと同じdistribution rootの`registry/`だけを標準定義として読み、current working directoryやGit checkoutを探索しない。

## 7. release archiveへの収録

GitHub Actionsはclient tagの同じcommitから各targetをpackageする。

Windows ZIP:

```text
<archive-root>/
├─ gdtvm.exe
├─ gdtvm.toml
├─ registry/
├─ README.md
├─ USER_GUIDE.md
└─ LICENSE
```

archive名は13章どおり`gdtvm_<version>_windows_<arch>.zip`とする。archive内に製品名やversionの親directoryを挟まず、展開先directoryがそのままdistribution rootになる。Linuxの`gdtvm_<version>_linux_<arch>.tar.gz`は`gdtvm.exe`をmode `0755`の`gdtvm`へ置き換え、その他は同じ構造とする。directoryは`0755`、通常fileは`0644`を正規modeとし、Windows ZIPのUnix modeは検証対象外とする。全targetでregistry内容は同一commit由来とし、platformで不要なdefinitionも省略しない。

## 8. runtime load

起動時にdistribution root、`gdtvm.toml`、`registry/registry.toml`を順に解決し、標準definitionを使用する全processでregistry tree hashをraw file bytesから計算してbinary埋込みexpected hashと照合する。file timestampや以前のstateだけでこの照合を省略しない。registry全体のsemantic strict parseはsetup、doctor deep、release checkで行い、通常起動はhash一致後に同じtree revisionの検証済みparse cacheと必要definitionを読む。shimはreceiptだけで完結するためregistryを読まず、この起動時検査の対象外とする。

標準definitionのrevisionはclient version、registry directory treeのdeterministic SHA-256、各definition SHA-256で識別する。tree hashは独立したfile manifestではない。release build時に同じcommitのregistry tree hashをclient binaryへ埋め込み、package validationとruntimeの変更検出・cache invalidationに使用する。配布元は公式GitHub repositoryのHTTPS Releaseであることを確認し、tree hash単独で任意のarchiveを公式配布物と判断してはならない。

tree hashは次の手順で計算する。

1. `registry/`配下のregular fileだけを列挙する。directory、symlink、reparse point、device、socket、submoduleは拒否する。
2. registry rootからのrelative pathが2章のASCII lowercase規則を満たすことを確認し、区切りを`/`とする。空path、absolute、`.`、`..`、NULを拒否する。
3. pathのASCII byte列昇順でsortし、完全一致する重複pathがあれば拒否する。
4. 1つのSHA-256 streamを初期化し、ASCII文字列`gdtvm-registry-tree-v1`とNULをtree全体で1回だけ追加する。その後、各fileについてrelative pathのbyte長を8 byte unsigned big-endian、path bytes、file sizeを8 byte unsigned big-endian、file raw bytesの順に追加する。
5. digestは64文字lowercase hexで表す。TOMLやtextの改行・BOMをhash時に正規化しない。

空treeはregistry構造違反としてhash計算前に拒否する。同じalgorithmをrelease build metadata生成、package validation、runtime cache、`doctor --deep`、receipt/state生成に使用する。archive内registry hashがbinary埋込み値と一致しないpackageを公開・起動しない。

registry欠落、schema不正、client非互換は`E_REGISTRY_INVALID`とし、networkから補完せず同じrelease archiveの再展開または`self-update`を案内する。標準registryを利用者が直接編集した場合はdoctor errorとし、local definition directoryを使用するよう案内する。

## 9. 更新

registry単体のupdate commandを設けない。`gdtvm self-update`がGitHub ReleaseからOS/architecture一致のclient archiveを取得し、client、既定`gdtvm.toml`、registry、README、USER_GUIDE、LICENSEを同時に更新する。

既存の利用者`gdtvm.toml`は保持し、新releaseの同梱既定fileで上書きしない。registryと文書は同じreleaseの内容へ置換する。更新transaction、checksum、GitHub Release信頼境界、rollbackは08章・11章・13章に従う。

## 10. local definition

利用者が追加・変更するtool definitionはglobal`gdtvm.toml`の`[definitions].local_dirs`から読む。標準`registry/`へ直接追加しない。

local definitionは内容hash承認、origin表示、hook警告を維持する。標準IDと衝突した場合は設定したprecedenceに従うが、未承認localから標準へ黙ってfallbackしない。client更新後もlocal directoryはdistribution registry置換の対象外とする。
