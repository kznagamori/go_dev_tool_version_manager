# P1-03 決定記録: portの依存注入とpackage global mutable state不存在

対象task: `docs/13-progress.md` P1-03
規範仕様: [02-architecture.md](../02-architecture.md) §4・§5・§12・§18、[11-quality-and-ci.md](../11-quality-and-ci.md) §2・§3・§4・§7.1

## 1. 実装した範囲

| 対象 | 実装 |
|---|---|
| Ports組立て | `internal/domain/port` の `Ports.Missing()` |
| constructor | `internal/app` の `NewServices(BuildInfo, port.Ports) (Services, error)` |
| build metadata | `internal/app` の `BuildInfo` と `BuildInfo.Validate()` |
| global state検査 | `internal/app/globalstate_test.go` の3 test |

## 2. `NewServices`が`App`/`Runtime`を持たない理由

[02-architecture.md](../02-architecture.md) §4は概念上のconstructorを次と定める。

```go
type Services struct {
    App     ApplicationService
    Runtime RuntimeResolver
}

func NewServices(build BuildInfo, ports Ports) (Services, error)
```

`ApplicationService`は§6の`Initialize`、§7の読取り5 operation、§8のPlan/Execute operationで構成され、実装task はP8-01・P8-02である。`RuntimeResolver`は§9の`ResolveInvocation`/`LaunchInvocation`で、実装taskはP8-03である。いずれもP1-03時点では型を一意に決められない。

中身の無いinterfaceを先に置く案は採らなかった。`CLAUDE.md` §7が「未使用のenum値、kind、fieldを『将来のため』に残さない」と定めており、method setの無いinterfaceは後で必ず書き換わるためである。§18が内部Go APIを同一module内で変更してよいと定めるため、2 fieldは対応するtaskで追加する。observable CLI・JSON・state・receipt・Plan・message codeのいずれも変えていない。

P1-03で実装したのは、§4が constructor の責務として明示した「依存の存在検査」と「build metadata形式検査」、および「filesystem/network変更を行わない」ことの固定である。

## 3. 判断

### 3.1 完全性検査を`Ports`側へ置いた

`Missing()`を`Ports`のmethodにした。port追加時に更新すべき箇所を構造体と同じfileへ閉じ込めるためである。検査を`internal/app`側へ書くと、fieldを増やしたときに検査の追随が漏れても、その場ではnilのまま組立てが成功する。

戻り値はfield宣言順の`[]string`とした。欠落portの列挙順が実行ごとに変わるとCI logやbug報告の突き合わせができないため、順序をtestで固定している。

typed nil pointerを入れたinterfaceは非nilになるため検出できない。注入側の誤りであり、ここで救う対象にしないことをdocumentation commentへ書いた。

### 3.2 `BuildInfo`は搬送用struct、検査は`Validate()`

[11-quality-and-ci.md](../11-quality-and-ci.md) §4の全項目をfieldにした。値はbuild時のlink flagで決まり、runtimeにVERSION fileやnetworkから読み直さない（同§4）ため、型としては搬送用structとし、正しさの判定を`Validate()`へ集約した。§4が検査の呼出し点をconstructorと定めるため、`NewServices`から呼ぶ。

検査規則と根拠は次のとおり。

| 検査 | 根拠 |
|---|---|
| `YYYY.MM.DD.XX` grammarと実在日付 | §2のgrammarと「実在日付検査」 |
| `devel`はdevelopment buildだけ | §2「development buildだけ`client_version="devel"`…を持てる」 |
| release binaryは`dirty=false` | §4「client version/release bool/commit/dirty=false」 |
| commitは40桁小文字hex | §2 |
| build時刻はUTC | §4「UTC build time」 |
| toolchainは`goMAJOR.MINOR[.PATCH]` | §4。具体versionとの照合は§1.2の`lint`が`go.mod`と行う |
| GOOS/GOARCHは`windows/amd64`または`linux/amd64` | §3のrelease target exact 2件 |
| CGO無効 | §3の表（CGO列が0） |
| 各schema versionは1以上 | §4「definition/registry/state schema対応version」 |
| repository owner/nameは非空で`/`・空白を含まない | §4。`owner/name`へ合成したとき別repositoryを指さないための形式検査 |

誤りは1件目で打ち切らず`errors.Join`で全件返す。build metadataはlink flagの組立て誤りで複数fieldが同時に欠けるため、1件ずつ直すと再buildを繰り返す。

schema versionに上限を設けず「1以上」だけを見る。上限を書くと、clientが新しいschemaへ対応するたびにこの検査も変える必要が出る。具体値との照合はregistry読込み側の責務である。

GitHub側の命名規則を完全に再現する案は採らなかった。§4が求めるのは「official GitHub repository owner/nameの埋込み」であって命名規則の検証ではなく、仕様に無い規則を実装で補うことになるためである。

### 3.3 global mutable stateはtestで検査する

[02-architecture.md](../02-architecture.md) §4は「package global mutable stateを使わない」、§12は「package global singletonを置かない」と定める。Goには map、slice、pointerを「初期化後は変更しない」と型で宣言する手段が無いため、次の二段構えにした。

1. **宣言検査**: production pathのpackage-level varは`allowedGlobals`表に無ければ失敗する（fail closed）。`var _ T = (*Impl)(nil)`のblank識別子assertionは記憶域を持たず参照もできないため表に載せずに通す。`_test.go`は対象外とする。
2. **変更検査**: 表に載せた識別子が、代入（`x = v`、`x[k] = v`、`*x = v`、`x.f = v`）、increment/decrement、address取得（`&x`）の対象になっていれば失敗する。「初期化後に変更しない」という表の宣言が後から静かに破られるのを防ぐ。

加えて、表に残った未使用entry（次に同名varを作ったとき無審査で通る）と、根拠の無いentryも失敗にする。

`scripts/ci/check_policy.py`側ではなくGo testにしたのは、Go自身のASTで宣言と変更を判定できるためである。text走査ではlocal変数、string literal、commentとの区別に手当てが要る。台帳が「testする」と書いている点とも一致する。

配置は`internal/app`とした。§4がこの不変条件をconstructorの規定と同じ段落で定めており、その規定を実装するのが`NewServices`である。module全体をsource levelで走査するため、`internal/app`のtestでありながら他packageも対象にする。

scope解決は持ち込まず、package-level varと同名のlocal変数への代入も違反として報告する。fail closedへ倒しており、衝突した場合はlocalを改名すれば解消できる。

`allowedGlobals`の初期entryは19件で、内訳はcompile済みregexp 10件、読取り専用の対応表 4件（`buildTargets`、`hexLength`、`platforms`、`pathRoles`）、fakeのsentinel error 5件である。

### 3.4 `fake.DefaultNow`をfunctionへ変えた

`var DefaultNow = time.Date(...)`はtestが起点を書き換えて他testへ影響させられるpackage global mutable stateだった。`time.Time`はconstにできないため、`func DefaultNow() time.Time`へ変えて表に載せずに済ませた。呼出し側は`internal/domain/port/fake/ports.go`と`fake_test.go`。

### 3.5 `check_imports.py`のALLOWED表

- `internal/app` → `internal/domain/port`: §4の`NewServices`が`port.Ports`を受け取る。
- `internal/app` → `internal/domain/port/fake`: `_test.go`からの注入だけに使う。production pathからのfake importは[11-quality-and-ci.md](../11-quality-and-ci.md) §7.1に従い`check_policy.py`が別途禁止する。

2件目は、`check_imports.py`が`_test.go`も走査するために必要になった。表がtest/productionを区別しないため、production pathの禁止は`check_policy.py`側の責務であることをcommentへ明記した。

## 4. 検証

すべてLinux container（Go 1.26.5、Python 3.11.15）で実行した。

| command | 結果 |
|---|---|
| `gofmt -l .` | 0 file |
| `go vet ./...` | 成功 |
| `go test ./... -race -shuffle=on -covermode=atomic -coverprofile=coverage.out` | 成功。total 86.8% |
| `scripts/ci/check_policy.py` | 成功。production Go file 42件 |
| `scripts/ci/check_imports.py` | 成功。package 20件 / internal import 11件 |
| `scripts/ci/check_docs.py` | 成功。file 28件 |
| `scripts/ci/check_pr_refs.py --head claude/feature-p1-03-port-injection --base claude/work` | 成功（task-id=p1-03, slug=port-injection） |
| `scripts/ci/check_licenses.py` | 成功。module 13件 |
| `git diff --check` | 差分なし |

package別coverage: `internal/app` 94.9%、`internal/domain` 94.8%、`internal/domain/port/fake` 85.2%、`cmd/gdtvm` 66.7%。

test件数は`internal/app`が42件（うち`NewServices`のnegative 23件、欠落port 7件）。

### 4.1 検査が効くことの確認（negative）

検査自体が空振りしていないことを、一時fileを置いて確認し、確認後に削除した。

| 仕込んだ違反 | 結果 |
|---|---|
| `internal/store`へ未登録の`var cache = map[string]string{}` | `TestNoPackageGlobalMutableState`が宣言違反として検出 |
| `pathRoles[role] = struct{}{}`（許可済みmapへの書込み） | 同testが変更違反として検出 |
| `toolIDRe = re`（許可済みregexpへの再代入） | 同testが変更違反として検出 |
| `allowedGlobals`へ存在しないentryを追加 | `TestAllowedGlobalsHasNoStaleEntry`が検出 |
| ALLOWED表から`internal/domain/port`を削除 | `check_imports.py`が2件のERRORで失敗 |

## 5. 未実施・制約

- `go tool govulncheck ./...`はこのcontainerから`vuln.go.dev`が`Forbidden`で到達できず、実行できていない。検証手段はCIだけである（P0-02から継続）。
- `internal/domain/port`のpackage coverageは0.0%と表示される。`Missing()`を呼ぶtestが`internal/app`にあり、§1.2が固定した`go test`commandは`-coverpkg`を使わないためである。動作自体は`NewServices`のtest 7件（欠落port別6件＋全欠落1件）と順序testで検査している。coverageは§1.2により計測のみで閾値を持たないため、command変更は行わなかった。
- Windowsでの実行はCI matrixでのみ確認する（`ubuntu-latest`/`windows-latest`の12 check）。
