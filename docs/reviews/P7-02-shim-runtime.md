# P7-02 決定記録（2/2）: shim実体と呼出名解決

対象タスク: `docs/13-progress.md` P7-02の2本目（最終）。規範仕様は[09-platform.md](../09-platform.md)§2.3・§3.3・§5.1・§7、[08-install-runtime.md](../08-install-runtime.md)§10、[02-architecture.md](../02-architecture.md)§2・§9、[04-storage-and-data.md](../04-storage-and-data.md)§11・§12・§16。

## 1. 着手時の確認事項（1本目の停止記録より）

2件とも**仕様から一意に決まった**ため、利用者判断を求めていない。

### 1.1 CLI/runtime分岐は3層に分かれ、本PRはその下2層を作る

§2の責務表とP8の項目を突き合わせると、分岐は1か所ではない。

| 層 | 責務 | task |
|---|---|---|
| `cmd/gdtvm` | argv0を読み、CLIかshimかで呼び先を変える | P8-04（CLI adapter） |
| `internal/shim` | **呼出名解決**（basename正規化→shim index照会）と**実体委譲** | **本PR** |
| `Services.Runtime` | §9の`ResolveInvocation`／`LaunchInvocation` | P8-03 |

§9の`RuntimeResolver`はP8-03の項目そのものであり、その依存は`P6-04,P8-02`である。**本PRでresolverを作らない**のは、§10手順2〜3がselection・receipt・`command_targets`を要し、`internal/selection`（P6-04）が未着手だからである。P7-02が担うのは手順1（呼出名解決）と、shim実体の配置である。

### 1.2 resolverが受け取るportは本PRでは決めない

1.1のとおりresolverはP8-03である。本PRの範囲で必要なportは`FileSystem`と`LinkManager`の2つだけで、**呼出名解決は純粋関数**（portを取らない）である。§9「Resolverはnetwork、prompt、repair、definition再downloadを行わない」を満たすには、hot pathの判断部分がportを持たないのが最も強い形になる。

### 1.3 `console/signal/exit透過`はP5-04で満たされている

P7-02の項目は「console/signal/exit透過」を含むが、`port.ProcessSpec.PassthroughStdio`がP5-04で実装済みであり、doc commentも「**shim経由の実行がこれに当たり**、gdtvmは内容を保存もmaskもしない」と明記している。production adapter（`internal/platform/process.go`）とfakeの両方が対応し、testも存在する。**同じ機構を作り直さない。** 実際にshimから使う経路はP8-03の`LaunchInvocation`である。

## 2. 判断

### 2.1 path分割をhost platformで行い、`filepath`に委ねない

`filepath.Base`／`filepath.Dir`は**実行中OSの区切りだけ**を見る。`host domain.Platform`を引数に取る関数がこれを使うと、Linuxで動かしたときWindowsのpath全体を1つの名前として扱う。

shimは自分のhost上で動くのでproductionでは一致するが、規則を実行環境に委ねると**両OS分の規則を1か所で確かめられない**。引数の意味とも食い違う。`splitPath(path, host)`を用意し、Windowsは`\`・`/`・`:`、Linuxは`/`を区切りとした。

これによりWindowsのpath規則（backslash、slash、drive root、`SHIMS`のcase差）をLinuxのtest実行で確かめられる。P7-01の`parseReparsePoint`、P7-02(1/2)の`ClassifyHost`と同じ形である。

### 2.2 OSのcase規則をどちらかへ寄せない

Windowsで`GO.EXE`と`go.exe`は同じfileだが、Linuxで`GO`と`go`は**別のfile**である。片方の規則で両方を扱うと、一方で別commandを同一視し、もう一方で同一commandを取り違える。

`.exe`は**1つだけ**落とす。`go.exe.exe`はcommand名`go.exe`であり、繰り返し落とすと別のcommandになる。

### 2.3 見つからない呼出名をCLIとして実行しない

§3.3「unknown basenameをCLIとして実行しない」。CLIへ落とすと、利用者が意図しないcommandがgdtvm本体として動く。0件は`ErrUnknownCommand`、複数件は`ErrAmbiguousCommand`とし、**破損したindexで片方を選んで進めない**（§10手順1「0件/複数件は失敗する」）。

### 2.4 data rootをshim pathから逆算し、決められなければ止める

§11「shim pathはdata root相対`shims`固定」を逆に辿る。§2.3「別rootのstate/linkを混在させない」ため、**data rootを決められないままstateを読みに行かない**。`shims`がfilesystem root直下にある場合（`/shims/go`、`C:\shims\go.exe`）は§2.3がfilesystem root自体をdata rootとして拒否するため、`ErrNotShimPath`とする。

CLI起動ではdata rootを逆算しない。`--home`やmodeでrootが変わるためで、決定は`internal/config`のlocatorが持つ。

### 2.5 `argv0`と`module path`の両方を持つ

§9の`InvocationRequest`が両方を持つのは、**一致しないため**である。Linuxのshimはclientへのrelative symlinkであり、argv0はshim path、module pathは辿った先のclient本体を指す。Windowsのhardlinkでは同じfileの別名なので一致する。

argv0がなければどのcommandとして呼ばれたか分からず、module pathがなければ動いているclientの同一性を確かめられない。

### 2.6 想定外の実体を黙って置き換えない

shim pathに通常のfileやdirectoryがあった場合は`ErrForeignShim`で止める。利用者が置いたものを消しうるためで、§3.2が「自動置換せず`doctor`診断とする」と定めるのと同じ扱いである。link同士の置換（方式変更、target変更）は実体を消さないので行う。

### 2.7 配置は冪等にし、結果をcommand名順で返す

§7「`setup`は冪等とする。…既に一致する項目をno-opとして報告する」。作り直すと、実行中のshimが指すfileを差し替える。

結果の順序を入力順に依存させないのは、setupの報告と`doctor`の出力が実行ごとに入れ替わると利用者が差分を読めないためである。

### 2.8 `fallback-resolver`を本packageで扱わない

利用者判断（1本目）により、内蔵fallback resolverはP11-04で扱う。`Strategy`は`hardlink`／`symlink`の2値だけを持ち、`fallback-resolver`を渡すと要求検査で落ちる。

## 3. fakeの契約違反を1件修正した

`fake.LinkManager.CreateSymlink`が`relative`引数を**無視して**いた（`_ = relative`）。productionは§5.1に従い相対形へ直して保存するため、**同じ呼出しでfakeとproductionの保存値が違っていた**。

保存値を読み返して一致を判定する呼出し側にとって、これは検査が成立しないことを意味する。実際にDeployの冪等性testが「常にreplace」で落ち、そこで発覚した。fake側を契約どおりに直し、`TestCreateSymlinkStoresRelativeTarget`で固定した。

計算は`filepath.Rel`ではなくslash前提で行う。`filepath.Rel`はWindowsで`\`区切りの結果を返し、同じfixtureがOSごとに違う値になる。fakeのpathは`clean`が`/`へ正規化しているため、slash計算が一貫する。

**判定に`clean`を通した値を使わない**という点も直した。`clean`は相対pathへ`/`を前置するため、すべてがabsoluteに見えてしまう。生の値で判定する。

## 4. 検査が固定したこと

`internal/shim`で29 case、fakeで5 caseを追加した。

| 検査 | 対象 |
|---|---|
| `TestNormalizeCommandNameAppliesOSRules` | 両OSの正規化7 case（suffix除去、大文字、**二重suffix**、suffixなし） |
| `TestNormalizeCommandNameKeepsLinuxCase` | **Linuxのcaseを畳まないこと**、`.exe`がcommand名の一部になること |
| `TestNormalizeCommandNameRejectsUnusableArgv0` | 空・空白・separatorだけ・suffixだけ・host未設定の6 case |
| `TestResolveCommandFindsOwningTool` | 1 toolが複数commandを持つ形（node/npm） |
| `TestResolveCommandRejectsUnusableIndex` | **0件／複数件／tool_id空／空index** |
| `TestIdentifyDerivesDataRootFromShimPath` | §11を逆に辿ること、argv0とmodule pathを両方保つこと |
| `TestIdentifyHandlesWindowsPaths` | **Linuxの実行でWindowsのpath規則3 case**（backslash、slash、`SHIMS`大文字） |
| `TestIdentifyRejectsShimOutsideShimDir` / `RejectsWindowsDriveRoot` | data rootを決められない起動を止めること |
| `TestDriveRelativeArgv0IsRejectedByIdentify` | `C:go`のcommand名は`go`だが、data rootは決められないこと |
| `TestIdentifyLeavesRootUnsetForCLI` | CLIでrootを逆算しないこと |
| `TestShimFileNameUsesPlatformSuffix` | 生成した名前を正規化すると元へ戻ること（往復） |
| `TestDeployCreatesShimsForBothStrategies` | 両方式の実体種別、**command名順の結果** |
| `TestDeploySymlinkTargetIsRelative` | §5.1のrelative保存 |
| `TestDeployIsIdempotent` | §7の冪等性（両方式） |
| `TestDeployReplacesShimPointingElsewhere` | 張り替えで**旧clientを消さない**こと |
| `TestDeployReplacesShimOfWrongKind` | 方式変更時の作り直し |
| `TestDeployRefusesForeignEntry` | **利用者が置いたfile/directoryを消さない**こと |
| `TestDeployRejectsMismatchedStrategy` | §16のplatform別方式に反する組合せ |
| `TestDeployRejectsUnusableRequest` | 7 case、**拒否がfilesystemへ何も残さないこと** |
| `TestDeployStopsOnFirstFailure` | 途中で止め、処理済みを返すこと |
| `TestCreateSymlinkStoresRelativeTarget` | fakeがport契約どおり保存すること（5 case） |

### 4.1 変異test

10件入れ、いずれも検査が落ちた。生き残りは無い。

| 変異 | 結果 |
|---|---|
| 未知の呼出名をCLIへ落とす | 落ちた |
| 複数件でも先頭を採る | 落ちた |
| Windowsのsuffixを繰り返し落とす | 落ちた |
| Linuxもcase-insensitiveに畳む | 落ちた |
| shim directory名の検査を外す | 落ちた |
| filesystem rootをdata rootとして受ける | 落ちた |
| 想定外の実体を置き換える | 落ちた |
| 既に一致していても作り直す | 落ちた |
| symlinkをabsoluteで作る | 落ちた |
| 結果をcommand名順に並べない | 落ちた |

### 4.2 test自身の誤りを1件直した

`C:go`（drive相対path）を`NormalizeCommandName`が拒否すると想定していたが、**command名としては`go`が正しい** —— drive指定はcomponent境界であり名前の一部ではない。fail closedが必要なのはdata rootの逆算側であり、`Identify`が`ErrNotShimPath`で止める。検査対象をそちらへ移し、code側の誤ったコメントも直した。

## 5. 検証

Linux containerで実行した（Go 1.26.6）。両OSの判定はCI matrixで行う。

| command | 結果 |
|---|---|
| `gofmt -l .` / `go build ./...` / `GOOS=windows go build ./...` | 出力なし・成功 |
| `go vet ./...` / `GOOS=windows go vet ./...` | 成功 |
| `GOOS=windows go test -c` | 成功 |
| `go test ./... -race -shuffle=on -covermode=atomic` | 全package成功。`internal/shim` 86.8% |
| `check_policy.py` / `check_imports.py` / `check_docs.py` / `check_licenses.py` / `check_messages.py` | すべて成功 |
| `git diff --check` | 出力なし |

`scripts/ci/check_imports.py`へ`internal/shim`のimport 4件を仕様根拠付きで追加し、`allowedGlobals`へsentinel error 6件を登録した。

## 6. 未実施・制約

- **§10手順2〜6を実装していない。** selection・receipt・`command_targets`の検査（手順3）、environment profileのmerge（手順4）、argv組立てと起動（手順5）、signal/exit透過（手順6）は`RuntimeResolver`＝**P8-03**の範囲であり、その依存は`P6-04,P8-02`である。本PRは手順1と実体配置だけを担う。
- **`cmd/gdtvm`の分岐を書いていない。** argv0からCLI/shimを振り分ける呼び先はP8-04（CLI adapter）である。本PRは判断（`Identify`／`ResolveMode`）を提供するにとどまる。
- **再帰marker（depth 8）を実装していない。** §10「再帰markerはdepth 8で拒否する」は起動時のenvironmentを見る検査であり、`LaunchInvocation`（P8-03）が持つ。
- **実shimからの起動をE2Eで確かめていない。** fakeでの配置判断と純粋関数の検査であり、実際にPATH経由でshimを叩く経路は[11-quality-and-ci.md](../11-quality-and-ci.md)§8のE2E scenarioとして合成後に行う。
- **`sameFile`がinode/file indexを使えない。** [port.FileInfo](../../internal/domain/port/filesystem.go)がそれらを持たないため、hardlinkの同一性はsize・mode・mtimeで判定する。**取り違えても不一致側へ倒れる** —— 一致と誤れば作り直しを1回省くだけ、不一致と誤ってもlinkを張り直すだけである。portへfield追加が要ると判断されれば§4.1の同期修正から始める。
- **P7-02(1/2)から継続**: `GetNativeSystemInfo`／`RtlAreLongPathsEnabled`は`windows-latest` jobだけが動かす。arm64・musl hostの`uname(2)`実値は観測していない（表に無い値はすべて対象外へ落ちる）。
- **P7-01から継続**: junction経路のsyscallは`windows-latest`だけが動かす（PR #145で確認済み）。`port.LinkCapabilities`は失敗理由を運べず、理由付き拒否はP7-03が行う。
- **P6-03から継続する未実装**: 合成側の`InstallEngine` adapter、`app.Guard`を噛ませた経路のE2E照合（§7.2）、receipt indexの再構築、`port.FileSystem`／`port.Environment`のproduction実装（いずれもP8-01）。
- **P6-02から継続する食い違いが1件**: `internal/store`のtemplate grammarが`internal/definition`と一致しない。fail closedは保たれ、正当なdefinitionからは生じない値である。§2の責務表を要する判断であり未着手。
- **P5-03から継続する未決が1件**（`./`始まりのtar entryを[10-security.md](../10-security.md)§5に従って拒否している）。**P6-01で埋めた仕様の空白が1件継続**（exact指定で`installable=false`のときの`E_PLATFORM_UNSUPPORTED`）。**仕様側の未決が2件継続**（[07-registry-and-tools.md](../07-registry-and-tools.md)§5第2項のlicense file size上限が§21の表に無い、§2の「license file名はASCII kebab grammar」が§2自身の例と食い違う）。`python.toml`の`lifecycle = "unknown"`はP3-04から継続。source error専用のerror code、version数値要素の64 bit上限、`logs/` file名規約と§11「専用lock」の解釈は未決である。
