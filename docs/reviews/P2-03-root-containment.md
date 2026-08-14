# P2-03 決定記録: path containmentとrootのfilesystem安全検査

対象task: `docs/13-progress.md` P2-03
規範仕様: [04-storage-and-data.md](../04-storage-and-data.md) §6・§21、[09-platform.md](../09-platform.md) §2.3、[10-security.md](../10-security.md) §6、[02-architecture.md](../02-architecture.md) §1

## 1. 実装した範囲

| 対象 | 実装 |
|---|---|
| path組み立てとcomponent検査 | `internal/security` の `Join`、`JoinRequest`、`ValidateComponent`、`PathSeparator` |
| containment判定 | `internal/security` の `IsContained` |
| 上限 | `internal/security` の `PathComponentMaxBytes`（255）、`LogicalPathMaxBytes`（32 KiB） |
| rootのfilesystem安全検査 | `internal/config` の `CheckRoot`、`RootCheckRequest`、`RootCheckResult` |

P2-01・P2-02がfilesystem操作を要するとして送った`--home`と`paths.user_data_root`の検査（owner、network share、symlink/reparse、書けないroot）と、pathのcanonical化を本taskで扱った。

## 2. 判断

### 2.1 `Join`は文字列でなくcomponent列を受け取る

`Join`の入力を`Components []string`にした。呼出し側が`filepath.Join`や文字列連結で先に`..`を潰してから渡すと、[04-storage-and-data.md](../04-storage-and-data.md) §6が拒否対象に挙げる`..`を検出できなくなる。componentのまま受け取れば、`..`が混ざった時点で拒否できる。

同じ理由でcomponent内のseparator（`/`と`\`の両方）を両OSで拒否する。componentへ区切りを埋めると、component単位の検査を通したまま階層を1段抜けられる。

### 2.2 拒否条件をOS共通とWindows固有に分ける

`ValidateComponent`は`windows bool`を取り、Windows固有の条件だけを分岐させる。Linuxで`:`や`CON`を拒否すると正当なfile名を扱えなくなるためである。

| 拒否する | 両OS | Windowsのみ |
|---|---|---|
| 空component | ○ | |
| `.` / `..` | ○ | |
| 255 byte超過 | ○ | |
| NUL | ○ | |
| `/` または `\` の混在 | ○ | |
| 不正UTF-8 | ○ | |
| ADS区切り `:` | | ○ |
| 予約device名（`con` `prn` `aux` `nul` `com1`〜`com9` `lpt1`〜`lpt9`） | | ○ |
| 末尾の空白またはdot | | ○ |

予約名は最初の`.`より前をcase非依存で比較する。Windowsは拡張子を付けても予約を解除しないため`CON.txt`も拒否する。逆に`console`、`com0`、`com10`、`lpt`は予約ではないため通す（過剰拒否しないことをtestで固定した）。

末尾の空白・dotは§6の明示listには無い。Windowsが末尾の空白とdotを暗黙に落とすため、検査した名前と実際に作られる名前がずれる。ずれた入力を通すとcase collision検査の前提が崩れるため拒否対象へ入れた。**§6の明示条件ではないため、過剰と判断されれば外せる。**

### 2.3 `IsContained`はcomponent単位で比較する

文字列prefixで比較すると`/data/gdtvm`に対して`/data/gdtvm-evil`が配下と誤判定される。componentへ分割して先頭から比較する。

case規則はplatformで変える。Windowsは`C:\Data`と`c:\data`が同じ位置を指すため`EqualFold`で比較し、Linuxはcase sensitiveのため完全一致で比較する。Linuxをcase非依存にすると`/data/GDTVM`という別directoryを配下と誤判定して管理外へ書ける。

root自身は配下として扱う。rootのmkdirやrenameをcontainment違反にすると、setupがrootを作れなくなる。

区切りは`runtime.GOOS`ではなく引数の`domain.Platform`から決める。どちらのrunnerからでも両OSの規則をtestできるようにするためである（`CLAUDE.md` §5）。

### 2.4 rootが存在しない場合は最寄りの既存祖先で判定する

`CheckRoot`はrootが未作成でも拒否しない。setupはrootを作るのが仕事であり、存在しないことを理由に拒否すると初回setupができない。代わりに最寄りの既存祖先までさかのぼり、そこが現在user所有でgroup/other書込み不可なら「作れる見込みがある」と判断する。実際の作成可否はsetupの`mkdir`が確かめる。

親chainも検査する。[09-platform.md](../09-platform.md) §2.3が「world-writable parent」を拒否対象に挙げており、上位directoryが他user所有やworld-writableならroot自体を差し替えられる。rootだけを見ても安全とは言えない。

さかのぼりは`ancestorScanMax`（4096段）で打ち切る。symlink/reparse loopや異常に深いpathで無限loopにしないためである。打ち切りは成功ではなく`E_PATH_UNSAFE`にする。

### 2.5 mode bit検査をLinuxだけで行う

`checkOwnerAndMode`はowner一致を両OSで検査し、group/other書込みbitの検査をLinuxだけで行う。WindowsのACLは`fs.FileMode`へ写らず、mode bitを見ても他user書込み可否を判定できないためである。Windowsでのworld-writable相当の判定はACLを読むport実装の責務であり、`fs.FileMode`から推測しない。

判定を`req.Host.OS()`で行い`runtime.GOOS`を使わないため、Linux runnerからWindows規則のtestができる。

`otherWritableMask`は`0o022`とし、sticky bit付きの`/tmp`のような構成も例外にしない。[10-security.md](../10-security.md) §6が「group/other書込み不可を基本とする」と定めており、例外をここで作らない。

### 2.6 診断へ実pathを載せない

`CheckRoot`が返す`*domain.Error`には`PathRole`だけを載せ、実pathをmessage parameterへ入れない（[10-security.md](../10-security.md) §9.2、P2-02と同じ方針）。port呼出しの失敗は`E_FILESYSTEM`とし、原因を`Cause`へ入れて表示経路から切り離す。

error codeは条件で分ける。filesystem root・network share・symlink root・非directory・祖先無し・scan打ち切りは`E_PATH_UNSAFE`、他user所有とgroup/world-writableは`E_PERMISSION`である。前者はpathの選択が誤っており、後者は環境の権限が問題であり、利用者の次の行動が異なる。

### 2.7 決定と検査を分ける

`DecideRoots`（P2-01）はfilesystemに触らず再現可能に決め、`CheckRoot`が実体を見る。分けているのは、root決定が環境非依存でtestできることと、検査が実体を要することの両方を型で示すためである。`CheckRoot`が`port.FileSystem`と`port.UserLookup`をrequestで受け取り、`DecideRoots`が受け取らない構造がその分担である。

## 3. 検証

### 3.1 CI

PR #37（commit `367c56c`、workflow run 31697446157）で、6 job×2 OSの **12 checkすべてがsuccess** になった。

### 3.2 ローカル検証

Linux container（Go 1.26.5、Python 3.11.15）で実行した。

| command | 結果 |
|---|---|
| `gofmt -l .` | 0 file |
| `go vet ./...` | 成功 |
| `go test ./... -race -shuffle=on -covermode=atomic -coverprofile=coverage.out` | 成功。total 92.5% |
| `scripts/ci/check_policy.py` | 成功。production Go file 59件 |
| `scripts/ci/check_imports.py` | 成功。package 20件 / internal import 42件 |
| `scripts/ci/check_docs.py` | 成功。file 32件 |
| `scripts/ci/check_pr_refs.py --head claude/feature-p2-03-root-containment --base claude/work` | 成功 |
| `scripts/ci/check_licenses.py` | 成功。module 14件 |
| `git diff --check` | 差分なし |

package別coverage: `internal/progress` 100.0%、`internal/security` 99.0%、`internal/domain` 95.8%、`internal/app` 94.9%、`internal/config` 93.3%、`internal/domain/port` 92.7%、`internal/domain/port/fake` 86.1%、`cmd/gdtvm` 66.7%。

test件数（subtest込み）: `internal/security` 62件、`internal/config` 142件。

### 3.3 主なnegative test

`internal/security`のnegative caseは36件（component 15、Windows固有 10、request不備 3、上限超過 1、containment偽 7）である。内訳は次のとおりである。

| 対象 | 件数と内容 |
|---|---|
| component拒否（両OS） | 15件。空、`.`、`..`、`/`混在、`\`混在、絶対component（`/etc`・`C:\Windows`）、NUL、不正UTF-8、255 byte超過 |
| Windows固有 | 10件。ADS `file:stream`、`CON`/`con`/`NUL`/`aux`/`COM1`/`lpt9`、`CON.txt`、末尾空白、末尾dot。**同じ入力がLinuxでは通ることも同じtestで確認** |
| 過剰拒否しない | 5件。`console`、`com0`、`com10`、`lpt`、`connection`をWindowsで通す |
| 上限 | logical path 32 KiB。上限内は通し、超過で拒否 |
| request不備 | 3件。root未設定、rootのpathが空、host未設定 |
| containment | 12件。prefixが同じ別directory（`/data/gdtvm-evil`）、上位、兄弟、無関係、Windows別drive、Windows case違いは同一、Linux case違いは別 |
| root検査 | 28件。filesystem root（linux）、drive root（windows）、UNC network share、rootがsymlink/reparse point、rootが非directory、root自身が他user所有、親が他user所有、root自身がworld-writable、root自身がgroup-writable、親がworld-writable、未作成rootの祖先が他user所有、未作成rootに既存祖先が無い、Stat失敗、RealPath失敗、OwnerOf失敗、request不備6件（root未設定・path空・host未設定・FileSystem未設定・UserLookup未設定・現在userのIDが空）。加えて安全rootの受理、canonical化、未作成rootの許容、Windowsでmode bitを見ないこと |

P1-03のglobal state検査が新規の`windowsReservedNames`を検出したため、根拠を添えて`allowedGlobals`へ追加した。P2-02の`colorModes`に続き、この検査が空振りしていないことの実地確認になっている。

## 4. 未実施・制約

- `go tool govulncheck ./...` はこのcontainerから`vuln.go.dev`が`Forbidden`で到達できず、ローカルでは未実行である（P0-02から継続）。CI `lint` jobの結果を証跡とした。
- root配下のdirectory tree生成（[04-storage-and-data.md](../04-storage-and-data.md) §2のroot layout）は**P2-05**の範囲である。本taskはlayoutを作らず、pathを安全に組み立てて検査するところまでとした。
- filesystemのcase sensitivity・reparse対応・long path対応といった**実probe**は[09-platform.md](../09-platform.md) §4のfilesystem capability検査であり、**P7-03**の範囲である。本taskの`IsContained`はplatform規則からcase比較を決めており、実際のfilesystemが規則と異なる場合の検出は行わない。
- WindowsのACLによる「他user書込み不可」判定は`port.UserLookup`/`port.FileSystem`の実装（P9）の責務である。本taskはowner一致までを見る。
- `CheckRoot`を`Initialize`のroot決定経路へ接続するのは**P8-01**である。本taskは関数までとした。
- 実際のsymlink/reparse pointを作る統合testは、port実装が入るP9まで行えない。本taskはfake `port.FileSystem`が`IsSymlink`を返すcaseで検証した。
- Windowsでの実行はCI matrixでのみ確認する。
