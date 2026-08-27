# P5-04 決定記録（2/2）: 書込み範囲記録wrapperとPlan外作用の拒否

対象タスク: `docs/13-progress.md` P5-04の2本目。規範仕様は[11-quality-and-ci.md](../11-quality-and-ci.md)§7.2、[02-architecture.md](../02-architecture.md)§2・§4・§8、[10-security.md](../10-security.md)§6・§7・§9.2、[04-storage-and-data.md](../04-storage-and-data.md)§17.2。

## 1. PR #109 のmergeが1 commitを取りこぼした

**本PRの1つ目のcommitはその復旧である。**

PR #109 のsquash mergeは、GitHubが認識していた古いhead `1b874f1`までを取り込み、branch tip `d106a63`を含まなかった。3回目のpushに対する`synchronize` webhookが発火せず、PR headが同期されなかったためである。merge前にこの不整合を報告し`workflow_dispatch`で`d106a63`のCIを通していたが、merge自体は古いheadで行われた。

| | tree |
|---|---|
| `1b874f1`（GitHubが認識していたhead） | `5284c22` |
| `d106a63`（branch tip） | `d978462` |
| merge後の`claude/work`／`develop/work`／`main` | `5284c22` |

失われたのは次のdoc 2 fileだけで、Go code・仕様本体・`go.mod`はすべて`1b874f1`に含まれ正しくmergeされている。

- `docs/reviews/P5-04-process-runner.md`（1本目の決定記録、137行）
- `docs/13-progress.md`（§3.3のP5-04停止記録とsnapshot）

`d106a63`から同内容を復元した。決定記録と停止記録はCLAUDE.md §4が完了条件として要求する証跡であり、merge後の履歴に無い状態を残さない。

**再発防止の観点で記録しておく。** PR headの同期はGitHub側のwebhookに依存し、こちらから保証できない。merge直前にPR headのSHAとbranch tipの一致を確認するのが唯一の防御である。今回はmerge前にその不一致を報告できていた。

## 2. 着手時の確認事項（1本目の停止記録より）

2件とも解決した。

### 2.1 Registry portは包まない

[11-quality-and-ci.md](../11-quality-and-ci.md)§7.2は「FileSystem、Registry、Process portを記録用wrapperで包み」と定めるが、`Registry` portは[02-architecture.md](../02-architecture.md)§4.1で「Windows HKCU valueのraw/type読書き、再読、通知」と定義され、用途はsetupのWindows user PATH integration（[10-security.md](../10-security.md)§8）専用である。実装はP7で、現時点でRegistryへの書込み経路は存在しない。

包むためだけに中身の無いportを先に作るのは、CLAUDE.md §7の「未使用のenum値、kind、fieldを『将来のため』に残さない」に反する。**P7でport本体を作るときに同じ形のwrapperを足す**方針とし、`Guard`のdocumentation commentと本記録へ明記した。

### 2.2 Plan `probes[]`は最小allowlist型で表す

`Plan`本体はP6だが、wrapperが「Plan外probe/write/download拒否」を**今**実行するには許可集合が要る。投機ではなくwrapperが実際に消費するため、消費する形だけを置いた。

| 対象 | 許可集合 | 根拠 |
|---|---|---|
| write/move/delete | `domain.PathValue`のroot列（role付き） | §7.2「判定は[04-storage-and-data.md](../04-storage-and-data.md)§17.2の`path_role`で行う」 |
| probe process | executable / argv / cwd の**完全一致** | §7.2「Plan `probes[]`のexecutable/argv/cwd/write pathと一致」 |
| download | 許可URL列 | [02-architecture.md](../02-architecture.md)§8手順5「Execute中のdownload/extract/probeがPlanの列挙と一致」 |

argvはprefix一致にしない。宣言したargsの後ろへ任意の引数を足して別の動作をさせられるためで、probeは`--version`のような固定argvしか使わない。

P6の`Plan`が`writes[]`・`probes[]`・artifact URLからこの`Scope`を組み立てる側になる。**`Plan`の代替物は作らない。**

### 2.3 置き場所は`internal/app`

[02-architecture.md](../02-architecture.md)§2が同領域へ「transaction境界」を割り当てており、§8手順5のExecute時不変条件（download/extract/probeがPlanと一致、全書込みが管理root内、任意helper/backend processを起動しない）はまさにその責務である。`Ports`を組み立てるのも同packageである。§2の18領域は固定なので新packageは作らない。

`internal/domain`と`internal/security`をimport表へ追加した。封じ込め判定にrole付きpathとcanonical containmentを、記録するURLのmaskに同じmask規則を使う。複製すると規則を変えたときに片方だけが古いままになる。

## 3. 判断

### 3.1 記録と拒否を分けない

記録だけのwrapperにすると、違反はE2E後の照合で初めて分かる。その時点では既に書込みが起きており、検査が事後報告になる。**通す前に判定し、通したものだけを記録する。**

§7.2は`e2e` jobの検査として書かれているが、同じ判定は§8手順5がExecute時の不変条件としても要求している。2箇所に別実装を置くと片方だけが古くなるため、1つのwrapperが両方を担う。

### 3.2 封じ込め判定は解決済みpathで行う

[10-security.md](../10-security.md)§6「symlink/reparse point経由の逸脱を拒否する」。解決前のpathで比べると、管理root内の名前で管理外へ書ける。

対象がまだ存在しない場合は、**最も近い既存の祖先まで遡って解決**し、残りのcomponentを繋ぎ直す。これから作る位置は定義上まだ解決できず、祖先が管理root内にあることが判定の要になる。祖先を1段ずつ辿るのは、`a/b/c`のうち`a`しか無い場合でも判定できるようにするためである。

祖先を1つも解決できない場合は拒否する。解決しないままcontainmentを判定すると、判定していないのに通したことになる。

### 3.3 renameは両端を判定する

片方だけ見ると、管理root内へ管理外のものを引き込む経路か、管理root外へ持ち出す経路のどちらかが素通りする。

### 3.4 読取りは縛らない

§7.2の検査対象は「全write/move/delete先」である。読取りはproject fileの探索や既存installの検査のように、管理外pathも正当に対象になる。

### 3.5 入れ子rootでは最も深いroleを記録する

data rootの下にdownload cacheがあるような入れ子で、宣言順に依存して別のroleを記録すると、§7.2のrole単位の照合が入力順で変わる。

### 3.6 記録にsecretを残さない

- process記録は**環境変数名だけ**を持ち値を持たない（[10-security.md](../10-security.md)§9.2「環境変数の全量dumpを出さず、definitionが宣言したkeyの有無だけを示す」）。証跡へ値を残すと、そこがsecretの保管場所になる。
- download記録のURLは`MaskURL`と`PathMasker`を通す。
- typed errorへpathとURLを載せない（[04-storage-and-data.md](../04-storage-and-data.md)§17.2「typed errorは秘密値や個人pathを露出させず、exact keyを保ったまま`path`を空にしてroleだけを伝えられる」）。

### 3.7 HEADもGETと同じ許可listで縛る

HEADは内容を取らないが、宛先へ到達する点はGETと変わらない。Planに無いhostへ到達できると、そこが存在確認の経路になる。

§7.2はwrapする対象にHTTPを挙げていないが、§8手順5がdownloadをPlanとの一致対象に挙げている。§7.2はE2Eが**記録**する対象、§8はExecuteが**強制**する対象であり、両方を満たす形にした。

### 3.8 返した記録はcopyにする

[02-architecture.md](../02-architecture.md)§4「request/resultは境界通過後にimmutableとして扱う」。sliceをそのまま返すと、呼出し側の書換えが証跡へ伝わる。

## 4. 検査が固定したこと

| 検査 | 対象 |
|---|---|
| `TestGuardAllowsWritesInsideRoot` | 許可root内のcreate/permission/removeが通り、role付きで順に記録されること |
| `TestGuardRejectsWritesOutsideRoot` | 6 methodすべてで許可root外を拒否し、拒否したものを記録しないこと |
| `TestGuardRejectsRenameWhenEitherSideIsOutside` | 移動元・移動先の両方を判定すること |
| `TestGuardRejectsWriteThroughSymlink` | link経由の逸脱を拒否すること |
| `TestGuardDoesNotRestrictReads` | 6つの読取りmethodを縛らず、書込みとして記録しないこと |
| `TestGuardAllowsDeclaredProcess` | 宣言どおりのprocessが通り、環境変数**名だけ**が記録されること |
| `TestGuardRejectsUndeclaredProcess` | executable／args違い／args追加／args欠落／cwd違いの5件を拒否し、**内側のportへ到達しない**こと |
| `TestGuardAllowsDeclaredDownload` / `TestGuardRejectsUndeclaredDownload` | GET・HEADを同じ許可listで縛ること |
| `TestGuardMasksRecordedDownloadURL` | 記録するURLからuserinfoとtokenを落とすこと |
| `TestGuardErrorsDoNotLeakPaths` | typed errorへpath・URLを載せないこと |
| `TestGuardPicksDeepestRootForRole` | 入れ子rootで最も深いroleを記録すること |
| `TestGuardRecordsAreImmutable` | 返した記録の書換えが内部状態へ伝わらないこと |
| `TestGuardRecordsConcurrently` | 並行呼出しで記録が壊れないこと |
| `TestGuardResolvesMissingLeafThroughExistingAncestor` / `TestGuardRejectsUnresolvableWrite` | 未作成pathを既存の祖先経由で判定できること、祖先を解決できなければ拒否すること |
| `TestGuardPropagatesInnerError` | 内側portのerrorをそのまま返し、通した書込みは失敗しても記録に残ること |

`TestGuardRejectsWriteThroughSymlink`は、`resolveForContainment`を生pathの比較へ変えると失敗することを実際に確認した——「解決前のpathで比べる」bugをこのtestが検出する。

`TestGuardRejectsUndeclaredProcess`は、拒否したprocessが**内側のportまで到達していない**ことも見る。到達していれば「起動してから記録を見る」実装になっている。

## 5. 検証

Linux containerで実行した（Go 1.26.6）。両OSの判定はCI matrixで行った。

| command | 結果 |
|---|---|
| `gofmt -l .` / `go build ./...` / `GOOS=windows go build ./...` | 出力なし・成功 |
| `go vet ./...` / `GOOS=windows go vet ./...` | 成功 |
| `go test ./... -race -shuffle=on -covermode=atomic` | 全package成功。`internal/app` 88.0% |
| `check_policy.py` / `check_imports.py` / `check_docs.py` / `check_licenses.py` / `check_messages.py` | すべて成功 |
| `git diff --check` | 出力なし |
| CI（PR #112） | 12 check成功 |

## 6. 未実施・制約

- **Registry portのwrapperがまだ無い**（§2.1のとおりP7）。§7.2が包む対象に挙げる3 portのうち2つを包んでいる。
- **`LinkManager` portのwrapperも無い。** current linkとshimを作るP7で同じ形を足す。§7.2の「全write/move/delete先」にはlink作成も含まれる。
- **`Guard`をまだ誰も使っていない。** `Ports`へ差し込むのはP6のExecuteとP8-01の組立てである。
- **§7.2のE2E照合そのものは未実装。** 記録を取る側を実装した段階で、記録とPlanを突き合わせる`e2e` jobの中身は`Plan`が存在するP6以降である。
- **P5-03から継続する未決が1件**（`./`始まりのtar entryを§5に従って拒否しており、標準4 toolの実archiveを繋ぐP6で該当があれば仕様側で扱いを決める）。**仕様側の未決が2件継続**（§5第2項のlicense file size上限が§21の表に無い、§2の「license file名はASCII kebab grammar」が§2自身の例と食い違う）。`python.toml`の`lifecycle = "unknown"`はP3-04から継続。
