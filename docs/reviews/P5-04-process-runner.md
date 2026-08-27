# P5-04 決定記録（1/2）: production ProcessRunner

対象タスク: `docs/13-progress.md` P5-04の1本目。規範仕様は[10-security.md](../10-security.md)§7、[02-architecture.md](../02-architecture.md)§4.1・§10、[04-storage-and-data.md](../04-storage-and-data.md)§21、[08-install-runtime.md](../08-install-runtime.md)§7、[11-quality-and-ci.md](../11-quality-and-ci.md)§1。

## 1. 着手時の確認事項（P5-03の停止記録より）

2件とも解決した。

### 1.1 sanitized環境とprobe用cwdは、どちらのportも組み立てない

[08-install-runtime.md](../08-install-runtime.md)§7手順2の「sanitized最小環境で、probe専用のowner-only temp directoryをcwdとしてrequired probeを実行する」は、次の3者に分かれる。

| 対象 | 担当 | 根拠 |
|---|---|---|
| probe専用temp directoryの作成 | install engine（`port.FileSystem`経由） | directory作成はfilesystem効果である。[04-storage-and-data.md](../04-storage-and-data.md)§17.2のrole `staging`が「operation staging/**probe-temp** directory」を明示する |
| sanitized環境mapの組立て | `Environment` port（未実装） | [02-architecture.md](../02-architecture.md)§4.1が同portへ「process block生成」を割り当てている |
| 受け取った`Dir`/`Env`の検査と使用 | `ProcessRunner` | `ProcessSpec`が両方を完成形で受け取る契約である |

`ProcessSpec`が`Dir`と`Env`を完全な形で持つため、ProcessRunnerは組み立てず、検査してそのまま使えばよい。**新しいportは要らない。**

### 1.2 tree終了はOSごとに単位が違う

[02-architecture.md](../02-architecture.md)§10「graceful signal→組込み5秒猶予→所有するprocess tree終了を行い、無関係processをkillしない」を次のとおり写した。

| | graceful | tree終了 | 単位 |
|---|---|---|---|
| Linux | `kill(-pid, SIGTERM)` | `kill(-pid, SIGKILL)` | `Setpgid`で作った新しいprocess group |
| Windows | `GenerateConsoleCtrlEvent(CTRL_BREAK_EVENT, pid)` | `TerminateJobObject` | job object |

Linuxは子をprocess groupのleaderにすればgroup IDが子のpidと一致し、負のpidで子とその子孫だけへ届く。pidを1件ずつ辿ると、辿っている間に生まれた孫を取りこぼす。

Windowsはprocess treeを辿るAPIを持たず、親子関係も終了時に切れる。job objectだけが「この起動から派生したprocess全体」を1単位として扱える。**`taskkill /T`は使わない**——[10-security.md](../10-security.md)§7の「helper、hook、backend、shell scriptを実行しない」「Plan `probes[]`にないexternal executableをExecute中に発見して起動しない」に、終了処理のための起動がそれ自体で反するためである。

## 2. 利用者判断: sanitized環境へOS要求の最小変数を補う規定を追加した

**Windows CIが契約違反を検出した。** Goの`os/exec`はWindowsでのみ、`SYSTEMROOT`を持たない環境へ親processの値を黙って追加する（`exec.go`の`addCriticalEnv`）。`port.ProcessSpec.Env`の「nilは空環境を意味し、親環境の暗黙継承はしない」に対して見えない例外ができ、§7の「sanitized allowlist環境」と、[11-quality-and-ci.md](../11-quality-and-ci.md)§7.2がPlanと照合する起動記録が、実際に渡る環境とずれる。

3案を提示して利用者判断を求めた。

| 案 | 内容 | 採否 |
|---|---|---|
| A | adapterが明示的に足すが仕様は変えない | 不採用。矛盾が実装側に残る |
| B | Windowsで`SystemRoot`未宣言を拒否する | 不採用。仕様に無い必須keyを呼出し側へ課す |
| **C** | **仕様へ「OSが要求する最小変数はadapterが補う」と明記し、補う変数を列挙したうえでadapterが足す** | **採用** |

[10-security.md](../10-security.md)§7へ次を追加し、`port.ProcessSpec.Env`のコメントを同期した。

> sanitized環境へは、**OSが起動に要求する最小変数だけ**をprocess adapterが補う。補う変数はWindowsの`SystemRoot`だけとし、Linuxでは何も補わない。呼出し側が同名（Windowsはcase非依存）を宣言していればその値を優先する。これ以外の変数を親環境から引き継がない。この集合は固定であり、Planと§7.2の記録照合はこの差分を既知として扱う。

最後の一文が2本目の前提になる。集合が仕様で固定されているため、記録wrapperもPlan側もこの差分を既知として引ける。

case違いの2重登録を避けるため、比較はcase非依存で行う。両方入れると、どちらが効くかが`os/exec`のdedup順に依存する。

## 3. 依存module追加: `golang.org/x/sys`

job object API（`CreateJobObject`／`SetInformationJobObject`／`AssignProcessToJobObject`／`TerminateJobObject`）と`GenerateConsoleCtrlEvent`をGo標準library（`syscall`）が公開していない。

`golang.org/x/sys` v0.47.0（BSD-3-Clause、module依存0件）を採用した。**既にindirectで同versionが入っており、新しいmoduleではない**（`golang.org/x/vuln`の依存）。直接依存へ昇格し、[11-quality-and-ci.md](../11-quality-and-ci.md)§1の依存module表へ§17が求める記録を追加した。Linux buildには含まれない。

## 4. 判断

### 4.1 空環境は非nilの空sliceで渡す

`exec.Cmd`は`Env`がnilのとき呼出しprocessの環境を継承する。`ProcessSpec`が「nilは空環境を意味し、親環境の暗黙継承はしない」と定めるため、nilをそのまま渡すと契約が逆になる。

環境blockは**name順に固定**する。map iteration順を子processへ渡すと、同じ指定でも起動ごとに§7.2の記録対象が変わる。

### 4.2 出力上限は打ち切りではなく失敗

[04-storage-and-data.md](../04-storage-and-data.md)§21「captureするprocess stdout/stderr 各16 MiB、超過は**末尾1 MiB保持して失敗**」。黙って打ち切ると、probeが読む出力が実体と違ってもそれと分からない。

**上限を超えても読取りは止めない。** 読むのをやめるとpipeが詰まり、子processが書込みでblockして終わらなくなる。保持だけを末尾へ切り替え、memoryを有界に保つ。

### 4.3 timeoutはerrorではなく結果

[08-install-runtime.md](../08-install-runtime.md)§7がtimeoutをprobe側の判断材料としており、起動できなかったこととは区別が要る。timeoutは`TimedOut=true`の結果、呼出し側のcancelは`context`のerrorとする。両者を同じcontextへ載せると理由が区別できないため、timeoutは別contextにした。

### 4.4 出力maskは3段で行う

§7「install/probeでcaptureするstdout/stderrを組込み上限で打ち切り、secretをmaskする」。自由文字列が対象のため既存の`PathMasker`だけでは足りず、`security.OutputMasker`を足した。

1. 渡した環境のうちsecret名（§9.2の`*_TOKEN`等）を持つentryの**値**
2. 文字列中のURLのuserinfoとquery値
3. home/user名/hostname

1を最初に行うのは、URLやpathの一部として現れたsecret値も落とすためである。**値の長さで下限を設けない**——短い値を見逃さないためで、出力が読みにくくなる代わりに漏えいしない側へ倒す。透過時（shim経由）は保存もmaskもしない。

### 4.5 実行時間は`port.Clock`の単調時間で測る

`time.Now`を直接読まないことをtestで固定した。`Monotonic`を呼ぶたびに一定量進むClockを注入し、`Duration`がその値と一致することを見る。実時間に依存しない。

## 5. 検査が固定したこと

実processを起こさないとtree終了、timeout、stdio、環境継承のいずれも検査できない。外部programへ依存すると両OSで同じtestが書けないため、**test binary自身をhelper processとして再実行する**。

| 検査 | 対象 |
|---|---|
| `TestProcessRunnerCapturesOutputAndExitCode` | stdout/stderrの分離captureとexit code、注入Clock由来のDuration |
| `TestProcessRunnerReportsNonzeroExit` | 失敗exitをerrorにしないこと |
| `TestProcessRunnerPassesArgvSeparately` | 空白・`;`・`$()`・`*`・引用符・`\|`・`&&`を含むargvがshellを経由しないこと |
| `TestProcessRunnerUsesGivenDir` | 指定cwdで動き、呼出し元のcurrent directoryを継承しないこと |
| `TestProcessRunnerDoesNotInheritEnvironment` | 渡した環境＋§7が認めた最小変数だけになること |
| `TestWithOSRequiredEnvSuppliesOnlySpecifiedVariables` | 補う変数がplatformごとに仕様どおり1件だけであること |
| `TestWithOSRequiredEnvKeepsDeclaredValue` | case違いで宣言しても2重に足さないこと |
| `TestProcessRunnerPassesStdin` | stdinの受渡し |
| `TestProcessRunnerMasksCapturedOutput` | secret値・URL userinfo・path/hostの除去 |
| `TestProcessRunnerPassthroughDoesNotCapture` | 透過時に内容を保存しないこと |
| `TestProcessRunnerEnforcesOutputLimit` / `TestProcessRunnerKeepsExactLimit` | 上限超過での失敗と末尾保持、上限ちょうどを失敗にしないこと |
| `TestProcessRunnerTimesOut` / `TestProcessRunnerStopsOnCancel` / `TestProcessRunnerRejectsCancelledContext` | timeoutとcancelの区別、開始前cancel |
| `TestProcessRunnerTerminatesStubbornChild` | gracefulを無視する子を強制終了まで進めること |
| `TestProcessRunnerTerminatesProcessTree` | 孫processまで終了させること |
| `TestProcessRunnerRejectsInvalidSpec` | 相対path、NUL、空Dir、負Timeout、環境変数名の`=`、上限超過（11件） |
| `TestEnvironmentSliceIsDeterministic` | name順の固定と、nilで非nilの空sliceを返すこと |

`TestProcessRunnerTerminatesProcessTree`は孫が**本当に起動したこと**をmarker fileで先に確かめてから、生き残っていないことを見る。`kill(-pid)`を`kill(pid)`へ変えると失敗することを実際に確認した——子だけを終了させるbugをこのtestが検出する。

## 6. 検証

Linux containerで実行した（Go 1.26.6）。Windowsの実挙動はCI matrixで判定した。

| command | 結果 |
|---|---|
| `gofmt -l .` / `go build ./...` / `GOOS=windows go build ./...` | 出力なし・成功 |
| `go vet ./...` / `GOOS=windows go vet ./...` | 成功 |
| `go test ./... -race -shuffle=on -covermode=atomic` | 全package成功。`internal/platform` 93.1%・`internal/security` 95.8% |
| `check_policy.py` / `check_imports.py` / `check_docs.py` / `check_licenses.py` / `check_messages.py` | すべて成功 |
| `git diff --check` | 出力なし |
| CI（PR #109） | 12 check成功。job object・CTRL_BREAK_EVENT・tree終了が`windows-latest`で通った |

## 7. 未実施・制約

- **起動とjob割当ての間にわずかな隙間がある（Windows）。** `os/exec`はCREATE_SUSPENDEDで起動したthread handleを公開しておらず、割当て後にresumeする方法が無い。この隙間に子が孫を作ると、その孫はjobの外へ出る。install engineが起動する外部processはdefinition宣言のvalidation probeだけ（§7）で、いずれも自身のversionを出力して終わる短命processのため許容し、コメントへ明記した。
- **executable containmentと完全versionの実行直前再確認は呼出し側の責務**とした。判定に必要な管理rootとversionを`ProcessSpec`が持たない。
- **`signalGraceful`はconsoleを持たない状況（Windows）で失敗する。** その場合はgraceful猶予を省いてtree終了へ進む。§10の「graceful→猶予→tree終了」のうち1段目が使えないだけで、終了自体は保証される。
- **`ProcessRunner`をまだ誰も使っていない。** 呼出し側はP6のprobe実行である。`port.Environment`（sanitized環境の組立て）も未実装で、最初に必要とするtaskで追加する。
- **2本目の範囲が残っている。** [11-quality-and-ci.md](../11-quality-and-ci.md)§7.2の書込み範囲記録wrapperとPlan外probe/write/download拒否は`claude/feature-p5-04-record-wrapper`で実装する。
- **仕様側の未決が2件継続**（§5第2項のlicense file size上限が§21の表に無い、§2の「license file名はASCII kebab grammar」が§2自身の例と食い違う）。`python.toml`の`lifecycle = "unknown"`はP3-04から継続。P5-03から継続する「`./`始まりtar entryの扱い」も未決である。
