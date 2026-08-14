# P2-05 決定記録（1/2）: LockManager portとlock順・§19 metadata

対象task: `docs/13-progress.md` P2-05（2分割の1本目）
規範仕様: [02-architecture.md](../02-architecture.md) §4.1・§12、[04-storage-and-data.md](../04-storage-and-data.md) §5・§19・§21

## 1. 2分割の判断

P2-05の対象はatomic write、revision採番、backup、破損復旧、lock順、timeout/cancel、process間競合、log rotationに加え、P2-04から送った§19 lock metadataと、未実装の`LockManager` portに及ぶ。P2-04と同規模のため、利用者判断で2 PRへ分割した。

| # | 範囲 | branch |
|---|---|---|
| 1（本PR） | `LockManager` port＋fake、§19 lock metadata codec、§12のlock順6 role、timeout/cancel | `claude/feature-p2-05-lock-manager` |
| 2 | §4 atomic writeの7段階、revision採番、`.bak`、破損復旧、log rotation・保持上限 | `claude/feature-p2-05-atomic-write` |

task IDはP2-05のままとし、台帳項目は2本目のmerge後に`[x]`とする。

## 2. 着手時に確認した2点

### 2.1 §4 atomic writeを既存portで組む

`port.FileSystem.AtomicWrite`はtemp書込みとrenameを1呼出しで行う不可分な操作であり、§4のstep 3〜6（temp書込み→flush→**tempのstrict再parse**→`.bak`退避→atomic replace→directory metadata flush）の途中に割り込めない。利用者判断で**portを変えずに既存methodで組む**方針とした。

[02-architecture.md](../02-architecture.md) §4.1のport表がFileSystemの責務を「stat、read、atomic write、mkdir、rename、remove、walk、permission、realpath」と列挙し、flush/syncを挙げていない。flushとdirectory metadataの永続化はadapter実装（P9）の責務と読む。2本目では次のように組む。

- step 4のstrict再parseは`AtomicWrite`へ渡す**同じbytes**に対して行う
- `.bak`は`ReadFile`＋`AtomicWrite`で先に退避する
- step 7の公開file再読でdisk上の内容を検証する

temp fileそのものを再parseしない点が仕様の字面との差である。同じbytesの検査で等価だが、disk上の検証はstep 7まで遅れる。

### 2.2 `LockManager` portが未実装だった

§4.1のport表に`LockManager`（「process間共有/排他ロック、所有情報、timeout」）があるが、`port.Ports`は8件でこれを持っていなかった。本PRで新設し9件にした。

## 3. 判断

### 3.1 lock分類を数値順のenumにする

§12が6分類を順番に定めるため、定数の数値をそのまま取得順にした。`CompareLocks`はclassの数値順→keyのASCII byte順で比較する。§12がcatalogをToolID順、installをToolID/version/platform順、storageをToolID/storage ID順と定めており、`LockKey`がその順にqualifierを連結するため、keyのbyte順が仕様の順序と一致する。

### 3.2 lock keyの区切りに`~`を使う

§19は保存先を`locks/<role>.lock`と定めるが、同一class内に複数対象を持つ場合（catalog/install/storage）のrole名を定めていない。別fileへ分けるには一意なrole名が要るため`<class>~<qualifier>~...`で組み立てる。

**区切りに`-`を使わない。** tool ID・platform ID・storage IDはkebab-caseで`-`を含み、semverのprerelease（`1.0.0-rc.1`）も`-`を含む。`-`区切りだと tool `a-b`＋version `c` と tool `a`＋version `b-c` が同じkeyになり、**別の対象が同じlockを共有してしまう**。どの構成要素にも現れない`~`で区切り、衝突しないことをtestで固定した。

qualifierはpath componentとして安全でなければならない。区切りや相対参照が入るとlock fileがlock directoryの外へ出る（§6）。

**この区切り文字は§19が定めておらず、本PRで導入した規約である。** 別の表現が望ましければ変更できる。

### 3.3 fakeがlock順序を強制する

fake `LockManager`はin-processのmapで排他を模すが、**§12の順序は実装と同じく強制する**。順序違反を素通りさせると、本番でだけdeadlockするcodeがtestを通ってしまう。順序違反は`ErrLockOrder`として、待てば解消する`ErrLockTimeout`と別のerrorにした。実装の誤りと競合状態を混同しないためである。

待機はfake clockを進めて再現し、実時間のsleepを入れない。testの実行時間をclockの進め方で制御できるようにするためである。

### 3.4 二重解放を成功として扱う

呼出し側は§12の「cancel/timeoutでも取得済みlockを必ず解放する」に従いdeferで解放を登録するため、明示解放とdeferの両方が走る。二重解放をerrorにすると正常経路が失敗する。一方で**解放に失敗したlockは保持中のまま**にする。失敗を握りつぶして解放済みにすると、実際には残っているlockを解放済みと誤認する。

### 3.5 lock metadataの破損を`E_INTERNAL`にする

§19が「OS lockを正本とし、file contentは診断metadataだけ」と定める。lock fileが読めなくても**排他性は失われない**ため、`E_STATE_CORRUPT`にして操作全体を止める扱いにしない。roleは`port.LockKey`が作る正規形と一致することを検査する。未知classや非正規形のlock fileを診断へそのまま出すと、存在しないlockを保持しているように見える。

## 4. 検証

### 4.1 ローカル検証

Linux container（Go 1.26.5、Python 3.11.15）で実行した。

| command | 結果 |
|---|---|
| `gofmt -l .` | 0 file |
| `go vet ./...` | 成功 |
| `go test ./... -race -shuffle=on -covermode=atomic -coverprofile=coverage.out` | 成功。total 90.2% |
| `scripts/ci/check_policy.py` | 成功。production Go file 79件 |
| `scripts/ci/check_imports.py` | 成功。package 20件 / internal import 73件 |
| `scripts/ci/check_docs.py` | 成功。file 36件 |
| `scripts/ci/check_licenses.py` | 成功。module 14件 |
| `scripts/ci/check_pr_refs.py` | 成功（task-id=p2-05, slug=lock-manager） |
| `git diff --check` | 差分なし |

package別coverage: `internal/progress` 100.0%、`internal/security` 99.0%、`internal/domain` 95.8%、`internal/app` 94.9%、`internal/domain/port` 93.8%、`internal/config` 93.3%、`internal/store` 89.0%、`internal/domain/port/fake` 87.0%、`cmd/gdtvm` 66.7%。

### 4.2 port完全性の検査が3か所で反応した

`LockManager`の追加を、既存の3つの検査がすべて検出した。P1-03で入れたport完全性の仕組みが空振りしていないことの実地確認になっている。

| 検査 | 反応 |
|---|---|
| `Ports.Missing()`のtest | 8件→9件の宣言順listを更新 |
| `NewServices`の欠落report test | 期待文字列へ`LockManager`を追加 |
| global state検査 | `lockClassNames`と2つのsentinel errorを許可表へ追加 |

### 4.3 主なnegative test

| 対象 | 内容 |
|---|---|
| §12 lock順 | 逆順（shim→state）、同一class内の降順（catalog node→go）を`ErrLockOrder`で拒否。飛び越しは許す |
| lock key | 7件。空、区切りを含む、slash、backslash、NUL、相対参照、カレント |
| classとqualifier | 6件。qualifier不要classに指定、qualifier必須classで空 |
| 二重取得 | 同一lockの二重取得を拒否し、timeoutと区別する |
| timeout/cancel | 競合時に`ErrLockTimeout`、cancel済みctxで`context.Canceled`。両者を区別する |
| 失敗注入 | 取得・解放の両方。解放失敗時にlockが保持中のまま残ることも確認 |
| §19 role | 8件。未定義class、空、qualifierの過不足、path区切り、相対参照、大文字 |
| §19 exact key | 15件。unknown key、重複key、schema 2、lock_id/operation_idの形式、pidの0/負/小数/欠落、created_atの非UTC/zero、trailing data、BOM、空 |
| encode経路 | 5件 |

衝突しないことも固定した。tool `a-b`＋version `c` と tool `a`＋version `b-c`、prerelease version `1.0.0-rc.1` が別のlock keyになることを確認している。

### 4.4 CI

<!-- PR作成後にworkflow runとcheck結果を追記する -->

## 5. 未実施・制約

- `go tool govulncheck ./...` はこのcontainerから`vuln.go.dev`が`Forbidden`で到達できず、ローカルでは未実行である（P0-02から継続）。本PRは新規依存を追加していない。
- §4 atomic writeの7段階、revision採番、`.bak`、破損復旧、log rotation・保持上限は**2本目**の範囲である。
- 実OS lockのadapter実装（Windows/Linuxのfile lock）は**P9**の範囲である。本PRはportとfakeまでとし、fakeはin-processのmapで排他を模す。
- 「同一InstallKeyの同時導入は後発が待機し、先発成功後に整合性検査だけを行う」（§12）の整合性検査はinstall engineの責務であり、P6以降である。
- lock keyの区切り`~`は§19が定めておらず、本PRで導入した規約である。
- `ToolID`の長さ上限は仕様に規定が無く未設定のままである（P1-02から継続）。
- Windowsでの実行はCI matrixでのみ確認する。
