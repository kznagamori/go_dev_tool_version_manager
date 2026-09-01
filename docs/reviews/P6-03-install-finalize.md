# P6-03 決定記録（4/5）: probe実行

対象タスク: `docs/13-progress.md` P6-03の4本目。規範仕様は[08-install-runtime.md](../08-install-runtime.md)§2・§7、[06-tool-definition.md](../06-tool-definition.md)§11、[10-security.md](../10-security.md)§7、[04-storage-and-data.md](../04-storage-and-data.md)§16、[03-cli.md](../03-cli.md)§7。

## 1. 着手時の確認事項（3本目の停止記録より）

2件とも**仕様から一意に決まった**ため、利用者判断を求めていない。

### 1.1 P5-04の`ProcessRunner`の環境構築で足りる

[10-security.md](../10-security.md)§7「install/probeはsanitized allowlist環境…**sanitized環境へは、OSが起動に要求する最小変数だけ**をprocess adapterが補う。補う変数はWindowsの`SystemRoot`だけとし、Linuxでは何も補わない」。

`port.ProcessSpec.Env`の`nil`が空環境を意味し、adapterの`withOSRequiredEnv`がこの規則を実装済みである。**PATHは空でよい** — probeのexecutableはPlanが確定した絶対pathであり、PATH探索を必要としない（`ProcessSpec.Executable`のdoc commentも「PATH探索は呼出側が済ませる」と定める）。

なおこの§7の規則は、P5-04でWindows CIが`SYSTEMROOT`注入を検出したときに利用者判断で仕様側へ追加したものである。その決定がそのまま今回の答えになった。

### 1.2 receipt indexの再構築は「次回起動時」であり、installの責務ではない

§7手順8「receipt indexをatomic更新する。indexが古い状態で中断しても**receipt走査から再構築できる**」、同§「rename後の中断は導入成功でindexだけ古い状態であり、**次回起動時の再構築**で解消する」。

再構築の実施時点は「次回起動時」と明示されている。installが行うのはatomic更新までであり、再構築は起動時処理（`Initialize`、P8-01の範囲）が持つ。**P6-03では実装しない。**

## 2. 分割の見直し

本PRで§7手順2〜3（probe実行）を実装した。手順1（staging payloadのroot内再検査）と手順6〜8（receipt書込み、atomic rename、receipt index更新）、および§8手順3〜5（`inputs`実体再取得、lock、`app.Guard`適用）は5本目へ送る。

**P6-03は当初2分割の想定だった。** §7の9手順が、失敗時の巻き戻し先・依存する仕様・必要な新規primitiveの点で素直に1本へ収まらず、着手のたびに線を引き直している。分割の経緯を台帳の停止記録へ残してある。

## 3. 判断

### 3.1 probe tempは起動前に空にする

§11「probeごとに**空の**owner-only probe tempを作り」。既存があれば`RemoveAll`してから作る。**前回の中断が残した内容がprobe結果を変えうる**ためである。permissionは`0o700`（同§「owner-only」）。

### 3.2 成功・失敗・cancelのいずれでもtempを削除する

§11「成功/失敗/cancel後にengineが削除する」。`defer`で行い、経路ごとの書き分けをしない。書き分けると、新しい失敗経路を足したときに削除を書き忘れる。

### 3.3 required probeが失敗したらそこで止める

§11「required probe failureはcommit前にinstall全体を失敗させる」。後続を走らせても結果は捨てるため、利用者を待たせるだけである。

### 3.4 起動失敗とprobe失敗を別のerror codeにする

§7「required probeの起動後nonzero、timeout、output上限、version/root/path/能力不一致は`E_PROBE_FAILED`。実行file欠落やdefinition参照不正は`E_DEFINITION_INVALID`、**permission/OS起動失敗は対応するplatform/filesystem error**」。

| 事象 | code |
|---|---|
| exit code≠0、timeout、regex不一致、version/path不一致、required path欠落 | `E_PROBE_FAILED` |
| regexがcompileできない、capture groupが無い | `E_DEFINITION_INVALID` |
| OSがprocessを起動できない | `E_PERMISSION` |
| probe tempを作れない | `E_FILESYSTEM`（role=staging） |

**「probeが走って落ちた」と「probeを走らせられなかった」を混ぜない。** 前者はtoolやdefinitionの問題、後者は環境の問題であり、利用者が取る行動が違う。

### 3.5 stderrは`ProcessResult`のものをそのまま載せる

§7「probe stderr末尾はmask/上限後だけhuman errorへ含める」。`port.ProcessResult.Stderr`はadapterがmaskと上限適用を済ませた値である（同型のdoc comment）。ここで再度maskしない——2箇所でmaskすると、規則を変えたときに片方だけが古くなる。

## 4. 検査が固定したこと

27 subtestを追加した。

| 検査 | 対象 |
|---|---|
| `TestProbeRunsInSanitizedEnvironmentAndTemp` | **cwdがprobe tempでpayloadでないこと**、**envが空であること**、executable |
| `TestProbeCreatesAndRemovesTemp` | 成功・失敗・timeoutのいずれでもtempを残さないこと、起動前に空にすること |
| `TestProbeDetectsVersionMismatch` | version不一致で`E_PROBE_FAILED`（終了code 6） |
| `TestProbeFailsOnNonZeroAndTimeout` | nonzero exit、timeout、regex不一致 |
| `TestProbeIncludesStderrInDiagnosis` | 診断へstderrが載ること |
| `TestProbeChecksExpectPathWithin` | root内/外、**prefixが一致するだけのpathを配下と誤判定しないこと** |
| `TestProbeChecksRequiredPaths` | file/directoryの種別判定と欠落 |
| `TestProbeStopsAtFirstFailure` | 最初の失敗で止まること（起動回数で見る） |
| `TestProbeStopsOnCancel` | cancel境界（1回も起動しない） |
| `TestProbeReportsStartFailure` | 起動失敗を`E_PROBE_FAILED`にしないこと |
| `TestProbeReportsTempFailure` / `TestNewProbeRunnerRequiresDependencies` | 失敗注入、依存不足 |

### 4.1 変異test

3件入れ、いずれも検査が落ちた。生き残りは無い。

| 変異 | 結果 |
|---|---|
| 環境を渡す（sanitized違反） | 落ちた |
| probe tempを削除しない | 落ちた（3 case） |
| cwdをpayloadにする | 落ちた |

## 5. 検証

Linux containerで実行した（Go 1.26.6）。両OSの判定はCI matrixで行う。

| command | 結果 |
|---|---|
| `gofmt -l .` / `go build ./...` / `GOOS=windows go build ./...` | 出力なし・成功 |
| `go vet ./...` / `GOOS=windows go vet ./...` | 成功 |
| `go test ./... -race -shuffle=on -covermode=atomic` | 全package成功。`internal/install` 88.4% |
| `check_policy.py` / `check_imports.py` / `check_docs.py` / `check_licenses.py` / `check_messages.py` | すべて成功 |
| `git diff --check` | 出力なし |

`internal/install`のcoverageが89.9%→88.4%へ下がった。`ProbeRunner`のerror経路（未知enum、`expected_root`未設定など、Planのcodecが先に弾く内部不整合）が未到達で残っているためである。

## 6. 未実施・制約

- **`ProbeRunner`をまだ誰も呼んでいない。** 呼出し側は5本目のcommit transactionである。
- **`required_paths`の「probe tempにfileがある」caseを検査できていない。** runnerがprobe起動前にtempを空にするため（§11）、事前に置いたfileは消える。実際にはprobeが自分でvenv等を作るが、**fake `port.ProcessRunner`は副作用を持てない**。存在するcaseはpayload内のpathで見ており、判定logic自体は同じである。実probeを繋ぐE2E（P6以降）で埋める。
- **§7手順1（staging payloadのroot内再検査）と手順6〜8（receipt書込み、atomic rename、receipt index更新）、§8手順3〜5は未実装。** 5本目で扱う。
- **receipt indexの再構築は実装しない。** §7が「次回起動時の再構築」と定めており、起動時処理（`Initialize`、P8-01）の責務である。
- **P6-02から継続する仕様の食い違いが1件**: `internal/store`のtemplate grammarが`internal/definition`と一致しない。fail closedは保たれ正当なdefinitionからは生じない値である。5本目で扱う。
- **P6-01から継続する未実装**: Registry portと`LinkManager` portの記録wrapper（P7）、[11-quality-and-ci.md](../11-quality-and-ci.md)§7.2のE2E照合（P6以降）、`port.FileSystem`／`port.Environment`のproduction実装（P8-01。`HardenReadExecute`のproduction実装もここ）、Windowsの起動とjob割当ての隙間、起動時の孤児staging cleanup。
- **P5-03から継続する未決が1件**（`./`始まりのtar entryを[10-security.md](../10-security.md)§5に従って拒否しており、標準4 toolの実archiveを繋ぐ5本目で該当があれば仕様側で扱いを決める）。**P6-01で埋めた仕様の空白が1件継続**（exact指定で`installable=false`のときの`E_PLATFORM_UNSUPPORTED`）。**仕様側の未決が2件継続**（[07-registry-and-tools.md](../07-registry-and-tools.md)§5第2項のlicense file size上限が§21の表に無い、§2の「license file名はASCII kebab grammar」が§2自身の例と食い違う）。`python.toml`の`lifecycle = "unknown"`はP3-04から継続。source error専用のerror code（`E_DEFINITION_INVALID`）、version数値要素の64 bit上限、`logs/` file名規約と§11「専用lock」の解釈は未決である。
