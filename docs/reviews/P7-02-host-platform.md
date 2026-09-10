# P7-02 決定記録（1/2）: host platform判定とmusl/arm64 fail closed

対象タスク: `docs/13-progress.md` P7-02の1本目。規範仕様は[09-platform.md](../09-platform.md)§1・§3.1・§5.3・§9、[02-architecture.md](../02-architecture.md)§1・§2・§3。

## 1. 着手時の確認事項と利用者判断

### 1.1 内蔵fallback resolverの扱い（利用者判断）

P7-02の項目とCLAUDE.md §8は内蔵fallback resolverを**Windows専用**とするが、番号付き仕様3件はLinuxでも使えるとしている。

| 出典 | 記述 |
|---|---|
| [04-storage-and-data.md](../04-storage-and-data.md)§16 | Linuxも`shim_strategy=symlink\|fallback-resolver` |
| [09-platform.md](../09-platform.md)§5.1 | 「rootを安全に決定できないfilesystemでは同release内蔵native resolverを利用できる」 |
| [11-quality-and-ci.md](../11-quality-and-ci.md)§3 | 「root導出にlinkを使えない場合の小型resolverは同じsource/releaseからbuildしてclient resourceへ内蔵し」 |
| CLAUDE.md §8 / P7-02項目 | 「**Windowsの**小型fallback resolver**だけ**」 |

CLAUDE.md §2の優先順位では番号付き仕様が上位だが、CLAUDE.mdは利用者の指示fileであり、独断で解釈を確定しない。§13に従って選択肢を提示し、**利用者判断で「A: P11-04へ回す」を選択した**。

したがってP7-02は`hardlink`（Windows）／`symlink`（Linux）方式だけを実装し、`fallback-resolver`は**archive生成を作るP11-04で扱う**。二段構えbuild（clientへresolver binaryを内蔵する）のrecipeが`package` jobと同じtaskで決まるためである。link を作れない環境は`E_PLATFORM_UNSUPPORTED`とする。

`fallback-resolver`はP11-04まで未使用のenum値として残る。CLAUDE.md §7に抵触しうるため、`docs/13-progress.md`のP11-04へ実装予定として明記した。

### 1.2 resolver identityの保存先（仕様から一意）

§3.3「resolverのSHA-256/version/ownerを検査する」の保存先を調べたが、**どのstate fileにも置けない**。

- [04-storage-and-data.md](../04-storage-and-data.md)§12 `state/shim-index.toml`の許可keyは`schema`／`revision`／`root_id`／`client_version`／`receipt_index_revision`／`updated_at`／`[[commands]]`だけで、「許可keyは上記だけ」と閉じている。
- 同§11 setup stateのtop-levelも10 keyで「全件必須」と閉じている。

したがってresolver identityは**永続stateではなく、展開時と起動時の自己検査**である。新しいstate keyを足す必要はない。実装はP11-04（1.1）へ回すが、この判断は先に固定した。

## 2. 分割

P7-02は次の5点を持つ。1本に収まらないため**2 PRへ分割**する。

| 本 | 内容 |
|---|---|
| 1（本PR） | glibc判定とmusl/arm64 fail closed、long path利用可否 |
| 2 | shim実体（hardlink／relative symlink）、argv0/module/root identity、console/signal/exit透過 |

1本目を先にするのは、2本目のshim方式選択がhost platformの確定を前提にするためである。

## 3. 判断

### 3.1 観測と判定を分ける

§5.3は「OS/archはkernel API、libcは実体loader/libc identityとprobeで判定」と定める。どちらもsyscallとfilesystemを読むため、実機を用意しない限りtestで再現できない。

そこで**判定を純粋関数`ClassifyHost(HostFacts)`へ切り出した**。musl・arm64・未知kernel・観測なし・glibc/musl同時を、観測値を組み立てるだけで網羅できる。P7-01で`parseReparsePoint`を切り出したのと同じ形である。

`HostFacts`は判定失敗時も返す。`doctor`が「何を見て対象外と判定したか」を利用者へ示せるようにするためである（CLAUDE.md §12）。

### 3.2 `runtime.GOOS`／`runtime.GOARCH`で代用しない

どちらもbuildした対象を表す定数であり、hostの実体ではない。§5.3が「kernel API」と明示するのはこの差のためである。

**Windows on ARMのx64 emulationやqemuでは、amd64 binaryがarm64 kernel上で動く。** `runtime.GOARCH`はamd64を返すため、§1が非対応と定めるarm64 hostを対象と誤判定する。Linuxは`uname(2)`、Windowsは`GetNativeSystemInfo`を使う。

**`GetSystemInfo`ではなく`GetNativeSystemInfo`を使う。** 前者はemulation下のprocessから見た値を返すため、同じ誤判定になる。

### 3.3 muslの語彙を`domain`へ足さない

`domain.Libc`は`none`／`glibc`だけを持ち、`platforms`表にmusl entryは無い。muslを足すと**どの`Platform`も取り得ない値がdomainに残る**（CLAUDE.md §7）。

観測の語彙は`internal/platform`の`LibcKind`に置いた。`domain.Libc`は「対象platformが持つlibc」、`LibcKind`は「loader観測が示すlibc」で意味が違い、対象外を名指しするために後者が要る。

### 3.4 libc判定を4分岐にし、既定値へ落とさない

| 観測 | 判定 |
|---|---|
| glibcとmuslが同時 | 対象外（**一意に決められない**） |
| muslのみ | 対象外 |
| glibcのみ | `glibc` |
| 観測なし | 対象外 |

同時観測はgcompatを入れたAlpineや多重libcのcontainerで起こる。**どちらのartifactを導入すべきか決められない状態を、片方に決めて進めない。**

観測なしを`glibc`と見なさないのは、導入するartifactがglibc向けであり、根拠なく進めると**実行時に初めて壊れる**ためである。

### 3.5 loaderをpath名ではなくELF identityで確かめる

§5.3「libcは**実体loader/libc identity**とprobeで判定し、環境変数や`ldd`表示だけで決めない」。存在確認だけでは、名前が同じだけの別fileや壊れたlinkをloaderと見なす。

ELF headerを読み、magic・ELFCLASS64・ELFDATA2LSB・`EM_X86_64`まで確かめる。この判定はbyte列だけを見る純粋な変換であり、両OSのtestで走る。

`ldd`を実行しないのは§5.3の要求であると同時に、[10-security.md](../10-security.md)§7が外部programの実行を制限しているためでもある。

### 3.6 long path利用可否を`RtlAreLongPathsEnabled`で得る

§3.1「long path利用可否を**API**で扱い、`MAX_PATH`へ暗黙truncateしない」。

**registry値を自分で読まない。** 読んだ値とprocessの実効値はmanifest次第で食い違い、扱えるつもりで扱えないpathが生まれる。`RtlAreLongPathsEnabled`はmanifestとregistryの両方を反映した実効値を返す。関数が無い古いWindowsではlong pathを扱えないためfalseとする。

Linuxに`MAX_PATH`相当の切替は無いため常にtrueとする。

## 4. 検査が固定したこと

判定は純粋関数なので、両OSの実行で同じcaseが走る。

| 検査 | 対象 |
|---|---|
| `TestClassifyHostAcceptsSupportedHosts` | 対象2 platform、uname値のcase/空白の揺れ、`amd64`表記 |
| `TestClassifyHostRejectsUnsupportedHosts` | **arm64 kernel／Windows on ARM／32 bit x86／musl／観測なし／glibc+musl同時／未知kernel／空** の9 case。codeが`E_PLATFORM_UNSUPPORTED`、`Retryable=false`、**拒否時にplatformを返さない**こと |
| `TestClassifyHostIgnoresLibcOnWindows` | Windowsでloader観測を判定へ持ち込まないこと |
| `TestClassifyHostErrorNamesEveryObservation` | 観測が1件残らず診断へ載ること |
| `TestDescribeEvidenceIsStable` | 観測順で診断文が変わらないこと |
| `TestIsX8664ELFHeader` | ELF identity 8 case（aarch64、32 bit、big endian、magic 1 byte差ほか） |
| `TestIsX8664ELFReadsRealFiles` | **loaderらしい名前のshell script**をloaderと判定しないこと、短いfile・不在path・directoryでpanicしないこと |
| `TestProbeLibcCollectsFirstEvidencePerKind` | 同一libcの重複を畳むこと |
| `TestDetectHostMatchesThisMachine` | **CI matrixの両OSで実際の判定が通ること**（uname／GetNativeSystemInfoの呼出しと`linuxLibcProbes`の表が実環境に合っている証拠） |
| `TestDetectHostIsDeterministic` | 同じhostで結果が変わらないこと |

### 4.1 変異test

6件入れ、いずれも検査が落ちた。生き残りは無い。

| 変異 | 結果 |
|---|---|
| libc未検出を`glibc`と見なす | 落ちた |
| glibc/musl同時を`glibc`に倒す | 落ちた |
| arm64をamd64として受ける | 落ちた |
| ELFの`e_machine`検査を外す | 落ちた |
| ELF magic検査を外す | 落ちた |
| 診断文の観測を並べ替えない | 落ちた |

### 4.2 test自身の誤りを1件直した

拒否理由を`err.Error()`で確かめていたが、[domain.Error](../../internal/domain/error.go)は**Causeを`Error()`へ含めない規約**である（「呼出し側がそのまま利用者へ表示しても内部errorが漏れないようにするため」）。診断はCauseと構造化logから辿るのが正しく、検査対象を`err.Cause`へ直した。

なお`MessageID`を設定していないのは`internal/catalog`の同codeと同じ形である。表示文はpresentation層（P8-05）がcatalogから生成する。

## 5. 検証

Linux containerで実行した（Go 1.26.6）。両OSの判定はCI matrixで行う。

| command | 結果 |
|---|---|
| `gofmt -l .` / `go build ./...` / `GOOS=windows go build ./...` | 出力なし・成功 |
| `go vet ./...` / `GOOS=windows go vet ./...` | 成功 |
| `GOOS=windows go test -c` | 成功 |
| `go test ./... -race -shuffle=on -covermode=atomic` | 全package成功。`internal/platform` 93.7% |
| `check_policy.py` / `check_imports.py` / `check_docs.py` / `check_licenses.py` / `check_messages.py` | すべて成功 |
| `git diff --check` | 出力なし |

`scripts/ci/check_imports.py`へ`internal/platform → internal/domain`を仕様根拠付きで追加した（§1がplatformを「domainとportに依存するInfrastructure adapter」と定め、host判定が`domain.Platform`と`domain.Error`を返すため）。`allowedGlobals`へ表4件を登録した。

## 6. 未実施・制約

- **Windowsの観測経路をローカルで実行できていない。** `GetNativeSystemInfo`と`RtlAreLongPathsEnabled`の呼出しは`windows-latest` jobが動かす。判定は純粋関数へ切り出して両OSで走らせたが、**syscall呼出しそのものはCIでしか確かめられない。**
- **arm64・musl hostを実機で確かめていない。** 判定は`ClassifyHost`のtableで網羅したが、`uname(2)`がそれらのhostで返す実際の値（`aarch64`等）はGitHub Actionsのrunnerに無いため観測していない。表に無い値はすべて対象外へ落ちるため、**誤って対象と判定する側へは倒れない。**
- **`E_PLATFORM_UNSUPPORTED`にmessage IDを付けていない。** `internal/catalog`の同codeと同じく、表示文はP8-05のpresentation層が生成する。
- **filesystem能力（`atomic-replace`等7値）の検査は含まない。** P7-03「setupの`SetupPlan`（…filesystem/link…）」の範囲である。P7-01の`LinkManager.Capabilities`がlink 3種を担う。
- **P7-01から継続**: junction経路のsyscallは`windows-latest`だけが動かす（PR #145で確認済み）。`port.LinkCapabilities`は失敗理由を運べず、理由付き拒否はP7-03が行う。
- **P6-03から継続する未実装**: 合成側の`InstallEngine` adapter、`app.Guard`を噛ませた経路のE2E照合（[11-quality-and-ci.md](../11-quality-and-ci.md)§7.2）、receipt indexの再構築、`port.FileSystem`／`port.Environment`のproduction実装（いずれもP8-01）。
- **P6-02から継続する食い違いが1件**: `internal/store`のtemplate grammarが`internal/definition`と一致しない。fail closedは保たれ、正当なdefinitionからは生じない値である。§2の責務表を要する判断であり未着手。
- **P5-03から継続する未決が1件**（`./`始まりのtar entryを[10-security.md](../10-security.md)§5に従って拒否している）。**P6-01で埋めた仕様の空白が1件継続**（exact指定で`installable=false`のときの`E_PLATFORM_UNSUPPORTED`）。**仕様側の未決が2件継続**（[07-registry-and-tools.md](../07-registry-and-tools.md)§5第2項のlicense file size上限が§21の表に無い、§2の「license file名はASCII kebab grammar」が§2自身の例と食い違う）。`python.toml`の`lifecycle = "unknown"`はP3-04から継続。source error専用のerror code、version数値要素の64 bit上限、`logs/` file名規約と§11「専用lock」の解釈は未決である。
