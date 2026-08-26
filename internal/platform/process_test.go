package platform

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain/port"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain/port/fake"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/security"
)

// helperEnv はtest binaryをhelper processとして動かすためのmarkerである。
//
// 実processを起こさないとtree終了、timeout、stdio、環境継承のいずれも検査
// できない。外部programへ依存すると両OSで同じtestが書けないため、test binary
// 自身をhelperとして起動する。
const helperEnv = "GDTVM_PROCESS_HELPER_MODE"

// helperArgEnv はhelperへ渡す補助parameterである。
const helperArgEnv = "GDTVM_PROCESS_HELPER_ARG"

// TestMain はmarkerがあればhelperとして振る舞う。
func TestMain(m *testing.M) {
	if mode := os.Getenv(helperEnv); mode != "" {
		os.Exit(runHelper(mode, os.Getenv(helperArgEnv)))
	}
	os.Exit(m.Run())
}

// runHelper はhelper modeを実行してexit codeを返す。
func runHelper(mode, arg string) int {
	switch mode {
	case "ok":
		fmt.Fprint(os.Stdout, "out-line")
		fmt.Fprint(os.Stderr, "err-line")
		return 0
	case "exit":
		return 3
	case "args":
		fmt.Fprint(os.Stdout, strings.Join(os.Args[1:], "\n"))
		return 0
	case "cwd":
		dir, err := os.Getwd()
		if err != nil {
			return 1
		}
		fmt.Fprint(os.Stdout, dir)
		return 0
	case "env":
		environment := os.Environ()
		sort.Strings(environment)
		fmt.Fprint(os.Stdout, strings.Join(environment, "\n"))
		return 0
	case "stdin":
		if _, err := os.Stdout.ReadFrom(os.Stdin); err != nil {
			return 1
		}
		return 0
	case "secret":
		fmt.Fprintf(os.Stdout, "token=%s url=https://user:pw@example.invalid/a?access_token=%s",
			os.Getenv("EXAMPLE_TOKEN"), os.Getenv("EXAMPLE_TOKEN"))
		return 0
	case "flood":
		// 上限ちょうどまで`a`、その後ちょうど末尾保持ぶんの`z`を書く。
		// 保持されるのが末尾かどうかを内容で判定できるようにするためである。
		_, _ = os.Stdout.Write(bytes.Repeat([]byte("a"), int(ProcessOutputMaxBytes)))
		_, _ = os.Stdout.Write(bytes.Repeat([]byte("z"), int(ProcessOutputTailBytes)))
		return 0
	case "sleep":
		time.Sleep(60 * time.Second)
		return 0
	case "stubborn":
		// gracefulを無視する子。tree終了まで進むことを検査する。
		signal.Ignore(os.Interrupt, syscall.SIGTERM)
		time.Sleep(60 * time.Second)
		return 0
	case "spawn":
		return spawnGrandchild(arg)
	case "grandchild":
		return grandchildMain(arg)
	default:
		fmt.Fprintf(os.Stderr, "unknown helper mode %q", mode)
		return 2
	}
}

// spawnGrandchild は孫processを起こしてから眠る。
//
// 孫は起動直後にmarker fileを作り、少し待ってから2つ目を作る。tree終了が効いて
// いれば1つ目だけが残る。1つ目の存在を確かめることで、孫が本当に起動したのに
// 検査が素通りしていないことも同時に固定できる。
func spawnGrandchild(dir string) int {
	self, err := os.Executable()
	if err != nil {
		return 1
	}
	child := exec.Command(self)
	child.Env = []string{
		helperEnv + "=grandchild",
		helperArgEnv + "=" + dir,
	}
	child.Dir = dir
	if err := child.Start(); err != nil {
		return 1
	}
	time.Sleep(60 * time.Second)
	return 0
}

// grandchildMain はspawnが起こす孫の本体である。
func grandchildMain(dir string) int {
	if err := os.WriteFile(filepath.Join(dir, "started"), []byte("1"), 0o600); err != nil {
		return 1
	}
	time.Sleep(1500 * time.Millisecond)
	if err := os.WriteFile(filepath.Join(dir, "survived"), []byte("1"), 0o600); err != nil {
		return 1
	}
	return 0
}

// steppingClock はMonotonicを呼ぶたびに一定量進むClockである。
//
// 実行時間が注入したClockから来ることを、実時間に依存せず固定する。
type steppingClock struct {
	*fake.Clock
	step time.Duration
}

func (c *steppingClock) Monotonic() port.Monotonic {
	value := c.Clock.Monotonic()
	c.Clock.AdvanceMonotonic(c.step)
	return value
}

// testRunner はtest用のProcessRunnerを作る。
func testRunner(t *testing.T, paths *security.PathMasker) (*ProcessRunner, *steppingClock) {
	t.Helper()
	clock := &steppingClock{
		Clock: fake.NewClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)),
		step:  7 * time.Millisecond,
	}
	runner, err := NewProcessRunner(clock, paths)
	if err != nil {
		t.Fatalf("NewProcessRunner: %v", err)
	}
	// gracefulを無視する子のtestを実時間5秒で回さない。
	runner.gracePeriod = 80 * time.Millisecond
	return runner, clock
}

// helperSpec はhelper modeを起動するProcessSpecを返す。
func helperSpec(t *testing.T, mode string) port.ProcessSpec {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	return port.ProcessSpec{
		Executable: self,
		Dir:        t.TempDir(),
		Env: map[string]string{
			helperEnv:    mode,
			coverDirName: t.TempDir(),
		},
	}
}

// coverDirName はcoverage計測時のwarningをhelperのstderrへ混ぜないための環境変数である。
//
// coverage付きでbuildしたbinaryは、この変数が無いまま[os.Exit]すると
// 「GOCOVERDIR not set」をstderrへ書く。helperはtest binary自身なので、
// captureしたstderrの検査がその警告に汚される。書き先を与えて黙らせる。
// coverage無しのbuildではこの変数は使われない。
const coverDirName = "GOCOVERDIR"

// TestProcessRunnerCapturesOutputAndExitCode は基本の実行結果を固定する。
func TestProcessRunnerCapturesOutputAndExitCode(t *testing.T) {
	runner, _ := testRunner(t, nil)
	result, err := runner.Run(context.Background(), helperSpec(t, "ok"))
	if err != nil {
		t.Fatalf("Run = %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", result.ExitCode)
	}
	if result.Stdout != "out-line" || result.Stderr != "err-line" {
		t.Errorf("stdout/stderr = %q / %q", result.Stdout, result.Stderr)
	}
	if result.TimedOut {
		t.Error("TimedOutがtrue")
	}
	// 実行時間は注入したClockから来る。time.Nowを直接読んでいれば0にならない。
	if result.Duration != 7*time.Millisecond {
		t.Errorf("Duration = %s, want 7ms（注入したClock由来でない）", result.Duration)
	}
}

// TestProcessRunnerReportsNonzeroExit は失敗exitをerrorにしないことを固定する。
//
// 起動できなかったことと、起動して失敗したことは呼出し側の判断が違う
// （docs/08-install-runtime.md §7）。
func TestProcessRunnerReportsNonzeroExit(t *testing.T) {
	runner, _ := testRunner(t, nil)
	result, err := runner.Run(context.Background(), helperSpec(t, "exit"))
	if err != nil {
		t.Fatalf("Run = %v", err)
	}
	if result.ExitCode != 3 {
		t.Errorf("ExitCode = %d, want 3", result.ExitCode)
	}
}

// TestProcessRunnerPassesArgvSeparately はargvがshellを経由しないことを固定する。
//
// docs/10-security.md §7「executableとargvを分離し、shellへ再結合しない」。
// shellを経由していれば、空白やmetacharacterを含む1 argが複数へ割れる。
func TestProcessRunnerPassesArgvSeparately(t *testing.T) {
	runner, _ := testRunner(t, nil)
	spec := helperSpec(t, "args")
	spec.Args = []string{"a b", "c;d", "$(echo x)", "*", "'q'", `"r"`, "|", "&&"}

	result, err := runner.Run(context.Background(), spec)
	if err != nil {
		t.Fatalf("Run = %v", err)
	}
	got := strings.Split(result.Stdout, "\n")
	if len(got) != len(spec.Args) {
		t.Fatalf("受け取ったarg = %d件（%q）, want %d件", len(got), got, len(spec.Args))
	}
	for index, want := range spec.Args {
		if got[index] != want {
			t.Errorf("arg[%d] = %q, want %q", index, got[index], want)
		}
	}
}

// TestProcessRunnerUsesGivenDir は指定cwdで動くことを固定する。
//
// docs/08-install-runtime.md §7手順2「probe専用のowner-only temp directoryを
// cwdとして」。呼出し元のcurrent directoryを継承しない。
func TestProcessRunnerUsesGivenDir(t *testing.T) {
	runner, _ := testRunner(t, nil)
	spec := helperSpec(t, "cwd")

	result, err := runner.Run(context.Background(), spec)
	if err != nil {
		t.Fatalf("Run = %v", err)
	}
	want, err := filepath.EvalSymlinks(spec.Dir)
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}
	got, err := filepath.EvalSymlinks(strings.TrimSpace(result.Stdout))
	if err != nil {
		t.Fatalf("EvalSymlinks(got): %v", err)
	}
	if got != want {
		t.Errorf("cwd = %q, want %q", got, want)
	}
	if wd, err := os.Getwd(); err == nil && got == wd {
		t.Error("呼出し元のcurrent directoryを継承した")
	}
}

// TestProcessRunnerDoesNotInheritEnvironment は親環境を継承しないことを固定する。
//
// [port.ProcessSpec]は「nilは空環境を意味し、親環境の暗黙継承はしない」と定める。
// exec.CmdはEnvがnilのとき親環境を継承するため、明示的に空sliceを渡す必要がある。
func TestProcessRunnerDoesNotInheritEnvironment(t *testing.T) {
	t.Setenv("GDTVM_MUST_NOT_LEAK", "leaked")

	runner, _ := testRunner(t, nil)
	spec := helperSpec(t, "env")
	// 渡す環境をexactに検査するため、helperが要る最小限だけを置き直す。
	// coverage用の変数はstderrにしか効かず、この検査はstdoutだけを見る。
	spec.Env = map[string]string{
		helperEnv:    "env",
		helperArgEnv: "",
		"EXPECTED":   "value",
	}

	result, err := runner.Run(context.Background(), spec)
	if err != nil {
		t.Fatalf("Run = %v", err)
	}
	got := strings.Split(result.Stdout, "\n")
	want := []string{
		"EXPECTED=value",
		helperArgEnv + "=",
		helperEnv + "=env",
	}
	sort.Strings(want)
	if len(got) != len(want) {
		t.Fatalf("環境 = %q, want %q", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Errorf("環境[%d] = %q, want %q", index, got[index], want[index])
		}
	}
	if strings.Contains(result.Stdout, "GDTVM_MUST_NOT_LEAK") {
		t.Error("親環境の変数が子へ漏れた")
	}
}

// TestProcessRunnerPassesStdin はstdinを渡せることを固定する。
func TestProcessRunnerPassesStdin(t *testing.T) {
	runner, _ := testRunner(t, nil)
	spec := helperSpec(t, "stdin")
	spec.Stdin = strings.NewReader("piped-input")

	result, err := runner.Run(context.Background(), spec)
	if err != nil {
		t.Fatalf("Run = %v", err)
	}
	if result.Stdout != "piped-input" {
		t.Errorf("stdout = %q, want %q", result.Stdout, "piped-input")
	}
}

// TestProcessRunnerMasksCapturedOutput は出力のsecret除去を固定する。
//
// docs/10-security.md §7「install/probeでcaptureするstdout/stderrを組込み上限で
// 打ち切り、secretをmaskする」。
func TestProcessRunnerMasksCapturedOutput(t *testing.T) {
	runner, _ := testRunner(t, security.NewPathMasker("/home/tester", "tester", "devbox"))
	spec := helperSpec(t, "secret")
	spec.Env["EXAMPLE_TOKEN"] = "s3cr3t-value"

	result, err := runner.Run(context.Background(), spec)
	if err != nil {
		t.Fatalf("Run = %v", err)
	}
	if strings.Contains(result.Stdout, "s3cr3t-value") {
		t.Errorf("secret値が残った: %q", result.Stdout)
	}
	if strings.Contains(result.Stdout, "user:pw@") {
		t.Errorf("URL userinfoが残った: %q", result.Stdout)
	}
	if !strings.Contains(result.Stdout, security.Redacted) {
		t.Errorf("置換の痕跡が無い: %q", result.Stdout)
	}
}

// TestProcessRunnerPassthroughDoesNotCapture は透過時に内容を保存しないことを固定する。
//
// docs/10-security.md §7「shim経由の直接stdio透過はgdtvmが内容を保存・maskせず、
// 利用者processへそのまま渡す」。
func TestProcessRunnerPassthroughDoesNotCapture(t *testing.T) {
	runner, _ := testRunner(t, nil)
	spec := helperSpec(t, "ok")
	spec.PassthroughStdio = true

	result, err := runner.Run(context.Background(), spec)
	if err != nil {
		t.Fatalf("Run = %v", err)
	}
	if result.Stdout != "" || result.Stderr != "" {
		t.Errorf("透過時にcaptureした: %q / %q", result.Stdout, result.Stderr)
	}
	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", result.ExitCode)
	}
}

// TestProcessRunnerEnforcesOutputLimit は出力上限と末尾保持を固定する。
//
// docs/04-storage-and-data.md §21「captureするprocess stdout/stderr 各16 MiB、
// 超過は末尾1 MiB保持して失敗」。黙って打ち切らず失敗させる。
func TestProcessRunnerEnforcesOutputLimit(t *testing.T) {
	runner, _ := testRunner(t, nil)
	result, err := runner.Run(context.Background(), helperSpec(t, "flood"))
	if !errors.Is(err, ErrOutputLimit) {
		t.Fatalf("err = %v, want ErrOutputLimit", err)
	}
	if int64(len(result.Stdout)) != ProcessOutputTailBytes {
		t.Fatalf("保持したstdout = %d byte, want %d byte",
			len(result.Stdout), ProcessOutputTailBytes)
	}
	// 保持されるのは末尾である。先頭を残していれば`a`が混じる。
	if strings.ContainsRune(result.Stdout, 'a') {
		t.Error("先頭側の内容が残っている")
	}
	if strings.Count(result.Stdout, "z") != int(ProcessOutputTailBytes) {
		t.Error("末尾がそのまま保持されていない")
	}
}

// TestProcessRunnerKeepsExactLimit は上限ちょうどを失敗にしないことを固定する。
func TestProcessRunnerKeepsExactLimit(t *testing.T) {
	buffer := newLimitedBuffer()
	if _, err := buffer.Write(bytes.Repeat([]byte("a"), int(ProcessOutputMaxBytes))); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if buffer.exceeded() {
		t.Error("上限ちょうどで超過扱いになった")
	}
	if int64(len(buffer.String())) != ProcessOutputMaxBytes {
		t.Errorf("保持 = %d byte, want %d byte", len(buffer.String()), ProcessOutputMaxBytes)
	}
	if _, err := buffer.Write([]byte("b")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if !buffer.exceeded() {
		t.Error("1 byte超過が検出されない")
	}
	if int64(len(buffer.String())) != ProcessOutputTailBytes {
		t.Errorf("超過後の保持 = %d byte, want %d byte",
			len(buffer.String()), ProcessOutputTailBytes)
	}
	if !strings.HasSuffix(buffer.String(), "b") {
		t.Error("超過後に末尾が保持されていない")
	}
}

// TestProcessRunnerTimesOut はtimeout打ち切りを固定する。
//
// timeoutはerrorではなく結果として返す。docs/08-install-runtime.md §7が
// timeoutをprobe側の判断材料としており、起動できなかったことと区別する。
func TestProcessRunnerTimesOut(t *testing.T) {
	runner, _ := testRunner(t, nil)
	spec := helperSpec(t, "sleep")
	spec.Timeout = 150 * time.Millisecond

	result, err := runner.Run(context.Background(), spec)
	if err != nil {
		t.Fatalf("Run = %v", err)
	}
	if !result.TimedOut {
		t.Error("TimedOutがfalse")
	}
	if result.ExitCode == 0 {
		t.Error("打ち切った子のExitCodeが0")
	}
}

// TestProcessRunnerStopsOnCancel はcancelでcontextのerrorを返すことを固定する。
func TestProcessRunnerStopsOnCancel(t *testing.T) {
	runner, _ := testRunner(t, nil)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	result, err := runner.Run(ctx, helperSpec(t, "sleep"))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if result.TimedOut {
		t.Error("cancelがtimeoutとして報告された")
	}
}

// TestProcessRunnerRejectsCancelledContext は開始前のcancelを固定する。
func TestProcessRunnerRejectsCancelledContext(t *testing.T) {
	runner, _ := testRunner(t, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := runner.Run(ctx, helperSpec(t, "ok")); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

// TestProcessRunnerTerminatesStubbornChild はgracefulを無視する子を終了させることを
// 固定する。
//
// docs/02-architecture.md §10「graceful signal→組込み5秒猶予→所有するprocess
// tree終了」。猶予で終わらなければ強制終了まで進む。
func TestProcessRunnerTerminatesStubbornChild(t *testing.T) {
	runner, _ := testRunner(t, nil)
	spec := helperSpec(t, "stubborn")
	spec.Timeout = 100 * time.Millisecond

	start := time.Now()
	result, err := runner.Run(context.Background(), spec)
	if err != nil {
		t.Fatalf("Run = %v", err)
	}
	if !result.TimedOut {
		t.Error("TimedOutがfalse")
	}
	// 60秒眠る子が猶予＋強制終了で終わっている。
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Errorf("強制終了まで %s かかった", elapsed)
	}
}

// TestProcessRunnerTerminatesProcessTree は孫processまで終了させることを固定する。
//
// docs/02-architecture.md §10「所有するprocess tree終了」。子だけを終了させると、
// 子が起こした孫が残る。
func TestProcessRunnerTerminatesProcessTree(t *testing.T) {
	runner, _ := testRunner(t, nil)
	spec := helperSpec(t, "spawn")
	spec.Env[helperArgEnv] = spec.Dir
	spec.Timeout = 200 * time.Millisecond

	if _, err := runner.Run(context.Background(), spec); err != nil {
		t.Fatalf("Run = %v", err)
	}
	// 孫が本当に起動したことを先に確かめる。起動していなければ、この検査は
	// 何も見ていないのに成功してしまう。
	waitForFile(t, filepath.Join(spec.Dir, "started"))
	// 孫が生き残っていれば1.5秒後に2つ目のfileを作る。
	time.Sleep(2500 * time.Millisecond)
	if _, err := os.Stat(filepath.Join(spec.Dir, "survived")); err == nil {
		t.Error("孫processが生き残った")
	} else if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Stat: %v", err)
	}
}

// waitForFile はfileが現れるまで待つ。
func waitForFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("%q が作られなかった（孫processが起動していない）", path)
}

// TestProcessRunnerRejectsInvalidSpec は起動指定の前提違反を固定する。
func TestProcessRunnerRejectsInvalidSpec(t *testing.T) {
	runner, _ := testRunner(t, nil)
	absolute := filepath.Join(t.TempDir(), "tool")

	tests := []struct {
		name string
		spec port.ProcessSpec
	}{
		{"Executableが空", port.ProcessSpec{Dir: t.TempDir()}},
		{"Executableが相対", port.ProcessSpec{Executable: "tool", Dir: t.TempDir()}},
		{"ExecutableにNUL", port.ProcessSpec{Executable: absolute + "\x00", Dir: t.TempDir()}},
		{"Dirが空", port.ProcessSpec{Executable: absolute}},
		{"Dirが相対", port.ProcessSpec{Executable: absolute, Dir: "work"}},
		{"Timeoutが負", port.ProcessSpec{Executable: absolute, Dir: t.TempDir(), Timeout: -1}},
		{"argにNUL", port.ProcessSpec{
			Executable: absolute, Dir: t.TempDir(), Args: []string{"a\x00b"}}},
		{"環境変数名が空", port.ProcessSpec{
			Executable: absolute, Dir: t.TempDir(), Env: map[string]string{"": "v"}}},
		{"環境変数名に=", port.ProcessSpec{
			Executable: absolute, Dir: t.TempDir(), Env: map[string]string{"A=B": "v"}}},
		{"環境変数値にNUL", port.ProcessSpec{
			Executable: absolute, Dir: t.TempDir(), Env: map[string]string{"A": "v\x00"}}},
		{"環境変数が上限超過", port.ProcessSpec{
			Executable: absolute, Dir: t.TempDir(), Env: tooManyEnv()}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := runner.Run(context.Background(), test.spec); err == nil {
				t.Fatal("不正な起動指定が通った")
			}
		})
	}
}

func tooManyEnv() map[string]string {
	env := make(map[string]string, ProcessEnvMaxEntries+1)
	for index := 0; index <= ProcessEnvMaxEntries; index++ {
		env[fmt.Sprintf("VAR_%05d", index)] = "v"
	}
	return env
}

// TestProcessRunnerReportsStartFailure は起動できない場合を固定する。
func TestProcessRunnerReportsStartFailure(t *testing.T) {
	runner, _ := testRunner(t, nil)
	spec := port.ProcessSpec{
		Executable: filepath.Join(t.TempDir(), "does-not-exist"),
		Dir:        t.TempDir(),
	}
	if _, err := runner.Run(context.Background(), spec); err == nil {
		t.Fatal("存在しないexecutableで成功した")
	}
}

// TestNewProcessRunnerRequiresClock は依存不足を拒否することを固定する。
func TestNewProcessRunnerRequiresClock(t *testing.T) {
	if _, err := NewProcessRunner(nil, nil); err == nil {
		t.Fatal("Clockなしで作れた")
	}
}

// TestEnvironmentSliceIsDeterministic は環境blockの順序と空環境の表現を固定する。
func TestEnvironmentSliceIsDeterministic(t *testing.T) {
	got := environmentSlice(map[string]string{"B": "2", "A": "1", "C": "3"})
	want := []string{"A=1", "B=2", "C=3"}
	for index := range want {
		if got[index] != want[index] {
			t.Errorf("[%d] = %q, want %q", index, got[index], want[index])
		}
	}
	// nilでもnilを返さない。exec.CmdはEnvがnilのとき親環境を継承する。
	if empty := environmentSlice(nil); empty == nil {
		t.Error("nil環境でnil sliceを返した（親環境を継承する）")
	} else if len(empty) != 0 {
		t.Errorf("nil環境で %q を返した", empty)
	}
}
