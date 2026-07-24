# 標準ツール仕様

## 1. 共通方針

本章は初期registryに必ず収録する17ツールの機能仕様である。実際のURL、asset名、checksum位置は上流変更に追随して`/registry/`のTOMLを変更し、新しいclient releaseで更新できるが、取得元、処理の意味、公開command、環境は本章を満たす。

各節は説明資料ではなく、client sourceと同じ開発branchの`/registry/tools/<正規ID>.toml`を作成するための規範要件である。必須file名、旧`anyvm_win`との対応、helperおよび付随fileの収録範囲は[07-registry.mdの3節](07-registry.md#3-初期registryの標準定義)に従う。

旧実装がWindows amd64で提供した17ツールはすべて引き継ぐ。Linux/arm64では上流が検証可能なportable artifactを提供する組合せだけを公開し、見つからない版を他architectureのemulation前提で表示しない。

各節で別記しないarchive toolは`selection_strategy="link"`, `relocation="portable"`, `mutable_payload=false`とする。Android SDKとFlutterは節記載どおりmutable、Rustはbackendである。実registry CIはportable root移動後のrequired probeも実施し、埋込み絶対pathが判明したtoolを根拠なくportableのままにしない。

## 2. platform対応表

記号: **必須**は初期registry受入対象、**条件**は上流の公式artifactと検証情報が存在する版だけを公開、**第三者**は警告付きthird-party、**非対応**は理由を返す。

| Tool | Win amd64 | Win arm64 | Linux amd64 | Linux arm64 |
|---|---|---|---|---|
| android-sdk | 必須 | 条件 | 必須 | 条件 |
| bazel | 必須 | 条件 | 必須 | 条件 |
| cmake | 必須 | 条件 | 必須 | 条件 |
| dart | 必須 | 条件 | 必須 | 条件 |
| dotnet | 必須 | 必須 | 必須 | 必須 |
| flutter | 必須 | 条件 | 必須 | 条件 |
| go | 必須 | 必須 | 必須 | 必須 |
| gradle | 必須 | 必須（JDK依存） | 必須 | 必須 |
| jdk | 必須 | 必須 | 必須 | 必須 |
| kotlin | 必須 | 必須（JDK依存） | 必須 | 必須 |
| llvm | 必須 | 条件 | 条件 | 条件 |
| mingw | 必須 | 非対応 | 非対応 | 非対応 |
| ninja | 必須 | 条件 | 必須 | 条件 |
| node | 必須 | 必須 | 必須 | 必須 |
| python | 必須 | 条件 | 第三者 | 第三者 |
| rust | 必須 | 必須 | 必須 | 必須 |
| winlibs | 必須 | 非対応 | 非対応 | 非対応 |

「条件」はtool自体を非対応にする意味ではなく、`available` が検証済みassetのある完全版だけを返す。registry CIで少なくとも1版をsmokeできなければ、そのplatform entryを `supported=false` にする。

## 3. 共通catalog・導入規則

- GitHub tagを旧来の `git ls-remote` で取得していたtoolは、GitHub REST tags/releases adapterへ置換し、利用者環境のGitに依存しない。
- `v`, `go`, `llvmorg-` 等は表示版から定義どおり除く。
- stable以外を通常一覧と`--latest`から除く。
- archive内の単一top-level directoryはstrip/renameし、完成payload直下がtool rootになるようにする。
- download archive/installerは成功後削除する。
- link方式のcurrent linkはuser選択だけ。runtime表の `{{tool}}` は選択payload、shell exportでは利用可能ならcurrent linkを指す。backend方式はshared rootと完全selectorを使う。
- checksumは上流提供SHA-256を優先する。上流が提供しない既存toolでは、標準定義更新工程で取得・検証したdigestを版catalog metadataとして`/registry/`へ固定する方式を許す。この場合もsource CIで根拠を監査し、artifact download後にSHA-256照合する。
- command名がtool間で重なる場合、shimは有効選択が一つに定まるときだけ起動する。標準定義が同時利用を安全に構成できないtoolは相互`conflicts`を宣言し、列挙順で暗黙選択しない。
- script launcherはinterpreterを暗黙PATH探索しない。definitionに`sh`/`bash`等のsystem dependencyとinterpreter IDを記載し、install planで解決した絶対pathをreceiptへ固定する。不足時はpackage導入hintだけを表示する。

## 4. Android SDK Command-line Tools

**ID/alias**: `android-sdk`; `androidsdk`

**上流**:

- version discovery: `https://developer.android.com/studio` のcommand-line tools download情報。
- artifact base: `https://dl.google.com/android/repository/`
- Windows名: `commandlinetools-win-<build>_latest.zip`
- Linux名: `commandlinetools-linux-<build>_latest.zip`

ページ/asset名から数値buildを完全versionとして使う。複数match時に「最後のHTML出現」を採らず、対象OS、stable/latest label、最大数値buildを検証して1件に決める。過去build一覧を上流が提供しない場合はregistry/catalogで観測済み履歴を保持する。

**導入**:

1. zipを検証・展開。
2. payloadに `cmdline-tools/latest` を作り、archiveの`cmdline-tools`内容をそこへ配置。
3. 空のSDK rootとしてpayloadを使用。`platform-tools`、`emulator`は後から`SDK Manager`が同rootへ追加する。

**公開command**: `sdkmanager`, `avdmanager`。Windowsは`.bat`実体を`launcher="cmd-script"`、Linuxはshell scriptを`launcher="sh-script", interpreter="sh"`として定義し、native shimが固定interpreter経由で起動する。

**環境**:

- PATH: `platform-tools`, `cmdline-tools/latest/bin`, `emulator`
- `ANDROID_HOME={{tool}}` を主SDK rootとする。
- 互換のため `ANDROID_SDK_ROOT={{tool}}` も同値にできるが、これはAndroid公式でdeprecatedであるためdoctor infoを出し、両者を異なる値にしない。
- `ANDROID_USER_HOME={{shared}}/user-home`、`ANDROID_AVD_HOME={{shared}}/avd`。emulatorを導入した構成では`ANDROID_EMULATOR_HOME={{shared}}/emulator-home`も設定できる。

旧`ANDROID_SDK_HOME`はSDK rootではなく、古いtoolが`.android`を作る親directoryを表す。互換対象版で必要な場合だけ`{{shared}}`側の適切な親へ設定し、`{{tool}}`を誤って指定しない。

SDK Managerがpayloadを書き換えるため、このtoolはimmutable payloadではなくmanaged mutable payloadとしてreceiptに記録する。uninstallは追加packageも含め対象版rootを削除することを明示確認する。

## 5. Bazel

**ID**: `bazel`

**上流**: 版照合は `https://github.com/bazelbuild/bazel` tags、配布物は公式 `https://releases.bazel.build/<version>/release/index.html` を正とする。旧互換として6.0.0未満は標準catalogから除外する。release indexに列挙されたOS/arch一致の`bazel-<version>-<platform>` binaryまたはzipだけを選び、同ページのSHA-256とsignatureを取得する。Windows例は `bazel-<version>-windows-x86_64.zip` / `.exe`、Linux例は `bazel-<version>-linux-x86_64`。`bazel_nojdk-*`を採る場合はJDK dependencyを別途完全指定し、通常assetとの意味を混在させない。

**導入**: zipまたはraw executableをpayload/binへ置き、実行permissionを設定。launcherが書くcacheはpayload外sharedへ向ける定義を優先する。

**公開command**: `bazel`。PATHにpayload rootまたはbin。

system prerequisitesがある版はprobe/hintを定義し、自動導入しない。

## 6. CMake

**ID**: `cmake`

**上流**: `https://github.com/Kitware/CMake` tags/releases。完全版tagからrelease assetsを照合する。

- Windows amd64互換template: `cmake-<version>-windows-x86_64.zip`
- Linux: `cmake-<version>-linux-x86_64.tar.gz`、`linux-aarch64.tar.gz` 等、実assetを列挙して決定。

**導入**: top-level `cmake-<version>-<platform>` をstripしてpayloadへ。

**公開command**: `cmake`, `ctest`, `cpack`, `cmake-gui`（存在platformのみ）。PATH=`{{tool}}/bin`。

## 7. Dart SDK

**ID**: `dart`

**上流**:

- version: `https://github.com/dart-lang/sdk` stable tagsまたは公式archive version metadata。
- artifact: `https://storage.googleapis.com/dart-archive/channels/stable/release/<version>/sdk/`
- Windows amd64: `dartsdk-windows-x64-release.zip`
- Linux amd64: `dartsdk-linux-x64-release.zip`
- arm64は同release metadataに公式assetがある場合だけ。

**導入**: archiveの`dart-sdk`をpayloadへ。

**公開command**: `dart`, `dartaotruntime`等、payload/binの公式実体。

**環境**:

- PATH: `{{tool}}/bin`, `{{shared}}/pub-cache/bin`
- `PUB_CACHE={{shared}}/pub-cache`

pub cacheはversion間共有し、uninstallで既定削除しない。

## 8. .NET SDK

**ID/alias**: `dotnet`; `.net`

**上流**: Microsoft公式release metadata JSONを優先し、SDK完全版、RID、URL、SHA-512/SHA-256を得る。旧Windows amd64互換URLは `https://builds.dotnet.microsoft.com/dotnet/Sdk/<version>/dotnet-sdk-<version>-win-x64.zip`。固定templateだけに依存せずmetadataの実URLを採る。

NuGet command-lineはWindows互換機能として `https://dist.nuget.org/win-x86-commandline/latest/nuget.exe` をartifact role `nuget-cli`として検証し、主SDKと同じstaging payloadへ追加する。再現性のため `latest` URLの最終版/digestをcatalogに固定し、Execute時の再redirect結果がcatalogと違えば失敗する。Linuxではdotnet内蔵NuGetを基本とし、`nuget.exe`を必須にしない。

**導入**: SDK archiveをpayloadへ直接展開。version probeは `dotnet --list-sdks` と `dotnet --version` を隔離homeで確認する。

**公開command**: `dotnet`; Windowsで`nuget`。

**環境**:

- PATH: `{{tool}}`, `{{shared}}/tools`
- `DOTNET_ROOT={{tool}}`
- amd64 payloadでは `DOTNET_ROOT_X64={{tool}}`、arm64 payloadでは `DOTNET_ROOT_ARM64={{tool}}`
- `DOTNET_CLI_HOME={{shared}}/home`
- `DOTNET_ADD_GLOBAL_TOOLS_TO_PATH=false`
- `DOTNET_CLI_TELEMETRY_OPTOUT=true`
- `NUGET_PACKAGES={{shared}}/nuget/packages`
- `NUGET_HTTP_CACHE_PATH={{shared}}/nuget/http-cache`
- `NUGET_SCRATCH={{shared}}/nuget/scratch`
- `NUGET_PLUGINS_CACHE_PATH={{shared}}/nuget/plugins-cache`

`NUGET_FALLBACK_PACKAGES` は既定設定しない。対象の旧SDK/構成が実際のfallback package directoryを必要とし、probeで内容を確認できる互換definitionだけが設定する。空directoryを指定して上流の探索動作を変えない。`DOTNET_ROOT(x86)`/`DOTNET_ROOT_X86` はx86 runtimeを同じpayloadへ実際に導入した場合だけ設定する。amd64/arm64 payloadをx86 rootとして偽らない。旧実装の `NUGET_PERSIST_DG` は公開NuGet環境変数として確認できないため設定せず、documented cache変数へ置き換える。

## 9. Flutter SDK

**ID**: `flutter`

**上流**:

- version: `https://github.com/flutter/flutter` stable tagsまたは公式releases manifest。
- Windows amd64: `https://storage.googleapis.com/flutter_infra_release/releases/stable/windows/flutter_windows_<version>-stable.zip`
- Linux amd64: 同baseの `stable/linux/flutter_linux_<version>-stable.tar.xz`
- arm64は公式manifestにartifactがある版だけ。

**導入**: archiveの`flutter`をpayloadへ。Flutter SDKは内部cacheを生成・自己更新しうるためmutable payloadとする。`flutter upgrade` によりreceipt版から変わった場合doctor errorを出し、gdtvm管理下では新しい完全版をinstallするよう案内する。

**公開command**: `flutter`。Windowsの`flutter.bat`は`cmd-script`、Linuxの`flutter` scriptは`sh-script`＋`bash` system dependencyを使う。`dart` shimの正規所有者は`dart` toolとし、Flutter同梱Dartは`flutter`子環境内だけPATH先頭にする。Flutter定義は`dart` command shimを公開せず、全registryでのcommand衝突を発生させない。

**環境**:

- PATH: `{{tool}}/bin`, `{{shared}}/pub-cache/bin`
- `PUB_CACHE={{shared}}/pub-cache`

## 10. Go

**ID**: `go`

**上流**:

- version: `https://go.dev/dl/?mode=json&include=all` を優先。GitHub `https://github.com/golang/go` tagは照合用。
- Windows: `https://go.dev/dl/go<version>.windows-<amd64|arm64>.zip`
- Linux: `https://go.dev/dl/go<version>.linux-<amd64|arm64>.tar.gz`
- checksumは公式download metadata。

tag/sourceの `go` prefixを除いた `1.x.y` を完全版とする。beta/rcはprerelease。

**導入**: archiveの`go`をpayloadへ。

**公開command**: `go`, `gofmt`。

**環境**:

- PATH: `{{tool}}/bin`, `{{shared}}/gopath/bin`
- `GOROOT={{tool}}`
- `GOPATH={{shared}}/gopath`
- `GOBIN={{shared}}/gopath/bin`
- `GOCACHE={{shared}}/cache/go-build`
- `GOENV={{shared}}/env/go.env`
- `GOMODCACHE={{shared}}/gopath/pkg/mod`
- `GO111MODULE=on` は旧版互換が必要なdefinitionだけ。現行版へ無条件設定しない。

旧実装のcacheをversion payload内へ置かずsharedへ移し、SDKを汚さない。

## 11. Gradle

**ID**: `gradle`

**上流**: `https://github.com/gradle/gradle-distributions` releases。artifactは `gradle-<version>-bin.zip`。旧release URL形式 `releases/download/v<version>/...` をadapterで扱う。

platform-neutralだがJDK managed dependencyまたはsystem JDKが必要。特定Gradle版が要求するJava範囲をmetadataで検査し、registryがmanaged JDKを自動導入するrecipeでは推奨JDKの完全版をcatalog entryへ固定する。範囲からclientが勝手に「最新」を永続選択しない。

**導入**: top-level `gradle-<version>` をpayloadへ。

**公開command**: `gradle`。Windowsは`gradle.bat`を`cmd-script`、Linuxは`gradle`を`sh-script`＋`sh` system dependencyで起動する。

**環境**:

- PATH: `{{tool}}/bin`
- `GRADLE_HOME={{tool}}`
- `GRADLE_USER_HOME={{shared}}/cache`

## 12. JDK (Eclipse Temurin)

**ID/alias**: `jdk`; `java`

**上流**: `https://github.com/adoptium/temurin<feature>-binaries/releases` またはAdoptium API。少なくともfeature 11, 17, 21を旧互換として扱い、現行LTS/対応featureをregistry更新で追加できる。

旧Windows amd64asset regexは概念上 `OpenJDK..._jdk_x64_windows_hotspot_<version>.zip` / `jdk_x64_windows_hotspot_<version>.zip`。`_` build separatorを表示完全版の`+`へ正規化する。OS/arch、JDK（JREでない）、HotSpot、archive形式をasset metadataから厳密選択する。

**導入**: `jdk-<version>` 等のtop-levelをpayloadへ。

**公開command**: `java`, `javac`, `jar`, `javadoc`, `jshell`ほかpayload/bin。

**環境**:

- PATH: `{{tool}}/bin`
- `JAVA_HOME={{tool}}`

## 13. Kotlin Compiler

**ID**: `kotlin`

**上流**: `https://github.com/JetBrains/kotlin` tags/releases。artifact `kotlin-compiler-<version>.zip`、release URL `releases/download/v<version>/...`。

JDK dependency必須。Kotlin版とJava版の互換probeを定義する。

**導入**: archiveの`kotlinc`をpayloadへ正規化。

**公開command**: `kotlin`, `kotlinc`, `kotlinc-jvm`, `kotlinc-js`（存在するもの）。PATH=`{{tool}}/bin`。`.bat`は`cmd-script`、shell scriptは`sh-script`＋`bash` system dependencyを使用する。

## 14. LLVM

**ID**: `llvm`

**競合**: `winlibs`。両方を同じ有効環境へ入れず、明示した一方だけを選択する。

**上流**: `https://github.com/llvm/llvm-project` の `llvmorg-<version>` tags/releases。

- Windows amd64旧asset: `LLVM-<version>-win64.exe`
- 他platformはrelease assetを列挙し、公式prebuilt archiveがある版だけ公開。

**helper**: Windows self-extracting installerの管理root内展開にrelease同梱registryで固定した7-Zip helperを使う。旧来のように7-Zip download HTMLからその時点の`-x64.exe`を都度選ばず、helper definitionが完全版、公式URL、SHA-256を固定する。bootstrap `7zr.exe` が必要ならそれもhelper digestで固定する。

**導入**: helperでinstaller/archiveをstagingへ展開し、LLVM rootをpayloadへmove。systemへinstaller登録しない。

**公開command**: `clang`, `clang++`, `clang-cl`, `lld`, `lldb`等、存在assetに応じる。

**環境**:

- PATH: `{{tool}}/bin`
- `LIBCLANG_PATH={{tool}}/bin`（共有libraryの実在をprobe）

## 15. MinGW Builds

**ID**: `mingw`

**競合**: `winlibs`。`gcc`, `g++`, `gdb`等の所有が曖昧になるため同時選択を拒否する。

**上流**: `https://github.com/niXman/mingw-builds-binaries` tags/releases。Windows amd64のみ。

旧asset template:

`x86_64-<version>-release-posix-seh-ucrt-<version-info>.7z`

long tagからversion/version-infoをdefinition regexのnamed metadataとして抽出し、release asset実在を確認する。versionの表示には曖昧さが出ないlong release identifierを完全版として使う。

**helper**: 固定・検証済み7-Zip。

**導入**: archive top directoryをpayloadへ。大容量treeのcopyはせずstaging rename。

**公開command**: `gcc`, `g++`, `gdb`, `mingw32-make`等。

**環境**: PATHに、存在検査した `{{tool}}/bin` と互換layoutの `{{tool}}/mingw32/bin`。存在しないpathは追加しない。

## 16. Ninja

**ID**: `ninja`

**上流**: `https://github.com/ninja-build/ninja` tags/releases。

- Windows amd64: `ninja-win.zip`
- Linux amd64: `ninja-linux.zip`
- arm64はrelease asset名を列挙し公式assetがある版だけ。

**導入**: `ninja[.exe]`をpayloadへ。

**公開command**: `ninja`。

**runtime互換**:

- targetはexact payloadの`ninja.exe`/`ninja`
- Windows codepageはUTF-8（65001）
- fixed argsなし
- PATHへpayloadをprepend

これは旧 `symexe.exe`（参照元 `https://github.com/kznagamori/symexe`）と `ninja.ini` の `[CONFIG] OPTS/CODEPAGE`, `[OPT] PATH`, `[EXE] PATH` 相当を内蔵Go shim定義で再現する。外部 `symexe.exe` は配布・download・実行しない。

## 17. Node.js

**ID/alias**: `node`; `nodejs`

**上流**:

- version metadata: `https://nodejs.org/dist/index.json` を優先し、`https://github.com/nodejs/node` tagsで照合。
- 旧互換として14.0.0未満を標準catalogから除外する。
- Windows: `https://nodejs.org/dist/v<version>/node-v<version>-win-<x64|arm64>.zip`
- Linux: `node-v<version>-linux-<x64|arm64>.tar.xz`
- SHASUMS256.txtと署名を利用可能なら検証。

**導入**: top-level directoryをstripしてpayloadへ。

**公開command**: `node`, `npm`, `npx`, `corepack`。`node`はnativeとする。`npm`, `npx`, `corepack`は同じpayloadの固定`node[.exe]`をnative targetとし、対応する同梱JS entrypointを固定args先頭へ渡すrecipeを標準とする。target JSとnode実体をdefinition/receiptへ明示し、PATHやshebangへ依存しない。上流layoutに該当JSがない版だけ、Windowsの公式`.cmd` entryを検証済み`cmd-script`として使える。

**環境**: PATH=`{{tool}}`/`bin`（layout別）およびtool-specific shared global binを必要に応じ追加。npm cacheを管理rootへ固定する場合は`{{shared}}/npm-cache`を用い、userの既存cacheを削除しない。

## 18. Python

**ID**: `python`

### 18.1 Windows

**上流**:

- version discovery: `https://www.python.org/downloads/windows/` と `https://www.python.org/ftp/python/`
- amd64 full installer: `https://www.python.org/ftp/python/<version>/python-<version>-amd64.exe`
- arm64は公式Windows installer/embeddable assetが存在し、pipを完全構成できる版だけ。

完全版directoryを列挙し、artifactへのHEAD/metadataで存在確認する。EOLをchannel分類し、stable latestから除外できる。

**helper**: WiX Toolset 3の検証済み完全版。旧互換sourceは `https://github.com/wixtoolset/wix3/releases/download/wix3112rtm/wix311-binaries.zip` で、`dark.exe`を利用する。helper digestをregistryで固定する。

**導入手順**:

1. Python installerとWiX helperを検証。
2. `dark.exe <installer> -x <staging-extract>` をargv分離で実行。
3. `discover-files`で`AttachedContainer`内のMSIを列挙し、上限64件、1件以上を要求する。
4. `select-files`でbasename完全一致の`appendpath.msi`, `launcher.msi`, `path.msi`, `pip.msi`を除外する。大文字小文字はWindows ordinal case-insensitiveで比較する。
5. 残MSIのpath-listを`extract-msi-admin`の`for_each`へ渡し、正規化path byte順に `msiexec.exe /quiet /a <msi> TARGETDIR=<staging-payload>` でadministrative extractionする。UACを要求するinstall modeを使わない。
6. 完成`python.exe`で `-E -s -m ensurepip -U --default-pip` を実行してpipを構成。
7. `python --version` と `python -m pip --version` を検証。

msiexecが現在Windows/policyで非昇格実行できなければ、別公式portable recipeがregistryにある場合だけ切替え、なければmanual/unsupported errorにする。

### 18.2 Linux

Python公式は一般的な再配置可能binaryを提供しないため、Astralが管理する `python-build-standalone` のthird-party portable artifactを標準定義に採用する。

- provider/repository: `Astral Software Inc. / https://github.com/astral-sh/python-build-standalone`
- license: `MPL-2.0`
- machine-readable metadata: `https://raw.githubusercontent.com/astral-sh/python-build-standalone/latest-release/latest-release.json`

Pythonの言語versionとproviderのbuild/release tagは別の識別子として扱う。catalog itemにはPython完全version、provider release tag、target triple、artifact filename、artifact URL、SHA-256を保存し、install時に完全一致する1 artifactだけを選ぶ。`latest-release` branchは発見元としてだけ使用し、導入済みreceiptやdownload URLへ可変branch名を保存して再解決してはならない。各install前に [11-security.md](11-security.md) のthird-party警告を必須とし、provider、repository、release tag、license、採用理由、digest、target tripleを表示する。

system Pythonのcopyやsystem package manager実行は行わない。source buildを自動実行する標準recipeも初期版では提供しない。

**公開command**: `python`, `python3`, `pip`, `pip3`。`pip`/`pip3`は選択payloadの固定Python実体をnative targetとし、固定args `-m`, `pip` を前置する。生成scriptの絶対shebangへ依存しない。command名が同じPython tool内に属するため同selectionへ解決する。

**環境**: PATH=`{{tool}}` と `{{tool}}/Scripts`（Windows）、`{{tool}}/bin`（Linux）。pip cacheをsharedへ向ける場合はversion間互換に注意し、既定では上流標準動作を変えない。

## 19. Rust

**ID**: `rust`

Rustは単純archiveではなくrustup backendを使うが、CLI上は他toolと同じ完全version modelにする。

platformは `manager="rustup"`, `selection_strategy="backend"` とし、[06-tool-definition-schema.md](06-tool-definition-schema.md)の`[platforms.backend]`にbootstrap artifact role、host、完全selector、profile、managed home、timeoutをすべて記載する。版別toolchain treeを`versions/<version>/<variant>/payload`へコピーせず、shared rustup storeと版別backend receiptを使用する。`use` はcurrent junction/symlinkを作らない。

**上流**:

- rustup-init: `https://static.rust-lang.org/rustup/dist/<host>/rustup-init[.exe]`
- distribution: `https://static.rust-lang.org`
- manifest root: `https://static.rust-lang.org/rustup`
- version discovery: 公式channel manifestからstable rustc完全版を得る。

`install rust@<version>` は `stable` 文字列を保存せず、例 `1.88.0` の完全toolchainをrustupへ指定する。

**管理path**:

- `RUSTUP_HOME={{shared}}/rustup`
- `CARGO_HOME={{shared}}/cargo`
- `SCCACHE_DIR={{shared}}/sccache`
- PATH=`{{shared}}/cargo/bin`
- `RUSTUP_DIST_SERVER=https://static.rust-lang.org`
- `RUSTUP_TOOLCHAIN=<完全version>-<host>`

deprecatedな `RUSTUP_DIST_ROOT` は設定しない。mirrorを使う場合もdocumentedな `RUSTUP_DIST_SERVER` を使う。backend定義の`update_root`はrustup自体を明示更新する将来処理のmetadataであり、通常runtimeへ`RUSTUP_UPDATE_ROOT`を設定しない。gdtvm管理下ではrustup self-updateを通常実行せず、bootstrap/backend版をregistryで固定する。

**host**:

- Windows amd64のlegacy互換variantは `x86_64-pc-windows-gnu` を提供する。
- Windows arm64は上流対応host（通常MSVC）を使用し、必要なVisual C++ Build Toolsをsystem prerequisiteとして表示する。
- Linux amd64/arm64は対応する`unknown-linux-gnu`等、artifact manifestに存在するhostを使う。

variantが複数ある場合も完全toolchain/hostをreceiptへ固定する。初期defaultはWindows amd64でlegacy GNU、その他は上流標準hostとするが、registryで明示する。

**導入**:

1. rustup-initをSHA-256で検証。
2. `--no-modify-path`, noninteractive、指定host、完全toolchainで管理shared homeへ導入。
3. `rustc --version` から完全版を照合。
4. rustup active toolchainをreceiptへ記録。

runtimeでは`CARGO_HOME/bin`のrustup proxyをshared内の絶対pathで起動し、子環境の`RUSTUP_TOOLCHAIN`で完全toolchainを選ぶ。rustup独自のdirectory overrideやprojectの`rust-toolchain.toml`より、gdtvmの明示/project選択を優先するための環境overrideであることを`current --explain`へ表示する。

Windows GNU legacy variantでは旧設定を保持するため、managed Cargo configのtarget tableへ次の`rustflags`配列をvariant限定で設定する。

- `"-C", "link-arg=-Wl,--exclude-libs=ALL"`
- `"-C", "link-arg=-Wl,--exclude-all-symbols"`
- `"-C", "link-arg=-Wl,--allow-multiple-definition"`

配列へは上記6要素をこの順で格納する。既存userのCargo configを上書きせず、gdtvm専用CARGO_HOME内だけにTOML-aware更新する。

`sccache` がmanaged CARGO_HOME/binに存在するとき:

- `RUSTC_WRAPPER=sccache`
- `SCCACHE_CACHE_SIZE=1G`
- `SCCACHE_DIR={{shared}}/sccache`

を自動追加する。存在しなければ導入後に任意導入案内を出すだけで、`cargo install sccache`を自動実行しない。

**公開command**: `rustc`, `cargo`, `rustup`, `rustdoc`, `rustfmt`, `clippy-driver`等、存在するもの。

uninstallは指定toolchainだけをrustupで除去する。最後のtoolchain削除時もCARGO_HOMEとsccacheを既定保持し、明示確認でshared削除する。旧実装の「Rust全体削除」機能は `uninstall` 最終版＋shared削除確認で踏襲する。

## 20. WinLibs

**ID**: `winlibs`

**競合**: `mingw`, `llvm`。GCC-only構成でも初心者向けの決定性を優先して同時選択を許さず、必要な一方を明示選択する。

**上流**: `https://github.com/brechtsanders/winlibs_mingw` tags/releases。Windows amd64のみ。

long versionからGCC、任意LLVM、MinGW-w64、revisionを正規抽出する。

- GCC only: `winlibs-x86_64-posix-seh-gcc-<gcc>-mingw-w64ucrt-<mingw>-r<revision>.7z`
- GCC+LLVM: `winlibs-x86_64-posix-seh-gcc-<gcc>-llvm-<llvm>-mingw-w64ucrt-<mingw>-r<revision>.7z`

release URLは `releases/download/<long-version>/<file>`。表示完全版は構成を一意に表すlong versionとする。

**helper**: 固定・検証済み7-Zip。

**導入**: stagingへ展開し、実top-levelをpayloadへrename。tree copy禁止。

**公開command**: GCC/G++/GDB/Clang/Make等、構成に実在するcommand。

**環境**:

- PATH: 実在する `{{tool}}/mingw64/bin` または正規化したcompiler bin、および互換 `mingw32/bin`
- `LIBCLANG_PATH=<実在するLLVM/Clang binary/shared library directory>`

LLVMを含まない構成ではLIBCLANG_PATHを設定しない。

## 21. helper定義

### 21.1 7-Zip

- 上流: `https://www.7-zip.org/`
- license: `LGPL-2.1-or-later AND BSD-3-Clause AND LicenseRef-7zip-unRAR` と対応する上流license textをregistryへ収録。
- Windows architectureごとの完全版をregistryで固定。
- 公式SHA-256が得られない版をhelperとして自動導入しない。標準定義更新工程で信頼可能に検証したdigestをhelper definitionへ固定し、source CIとrelease package validationを通す。
- `7zr.exe` bootstrapとfull `7z.exe`の二段が必要なら両方を別artifactとして検証。
- toolごとに重複downloadせずcache helperを共有する。
- `7zr.exe`とfull packageが両方必要ならartifact role `bootstrap`と`primary`に分け、両方のdigest検証後だけbootstrapを起動する。download page HTMLをinstall時に解析して「その時点の最新版」を選ばない。

### 21.2 WiX

- 上流: `https://github.com/wixtoolset/wix3`
- Python Windows recipeに必要な`dark.exe`だけをhelper receiptで公開。
- helper archiveから不要実体を実行しない。
- helper entrypointは`dark.exe`のpayload相対pathとdigestを固定し、Python definitionの`run-helper`から絶対path起動する。利用者PATH上のWiXへfallbackしない。

## 22. standard definition受入検査

各tool/platform recipeはregistry発行前に次を満たす。

1. 最新stable 1版と旧stable 1版のcatalog解決。
2. URL/assetが対象OS/archと一致。
3. checksum取得とartifact照合。
4. clean homeへのinstall、required probe成功。
5. user use/current/shim commandの版一致。
6. 2版切替でtree copyが発生しない。
7. project selectionがuser currentを変更しない。
8. uninstall後、他版/shared/他toolが残る。
9. offline cache install（対応artifact）または明確な不足error。
10. Windowsは標準ユーザー、Linuxは非rootで成功。

上流仕様変更でこれを満たせないplatform/versionはcatalogから除外し、未検証の推測URLを公開しない。
