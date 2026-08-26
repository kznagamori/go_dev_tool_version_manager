// Package platform のprocess adapterである。
package platform

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain/port"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/security"
)

// process関連の組込み上限（docs/04-storage-and-data.md §21）。
const (
	// ProcessOutputMaxBytes はcaptureする1 streamの上限である
	// （同§21「captureするprocess stdout/stderr 各16 MiB」）。
	ProcessOutputMaxBytes int64 = 16 << 20
	// ProcessOutputTailBytes は上限超過時に保持する末尾のbyte数である
	// （同「超過は末尾1 MiB保持して失敗」）。
	ProcessOutputTailBytes int64 = 1 << 20
	// ProcessGracePeriod はcancel/timeout後のgraceful猶予である
	// （同§21「cancel後process graceful猶予 5秒」）。
	ProcessGracePeriod = 5 * time.Second
	// ProcessEnvMaxEntries は環境変数entryの上限である（同§21「environment entry 4,096」）。
	ProcessEnvMaxEntries = 4096
)

// ErrOutputLimit はcaptureした出力が上限を超えたことを表す。
//
// docs/04-storage-and-data.md §21は「超過は末尾1 MiB保持して**失敗**」と定める。
// 黙って打ち切ると、probeが読む出力が実体と違ってもそれと分からない。
var ErrOutputLimit = errors.New("platform: process出力が上限を超えた")

// ProcessRunner は子processを起動するport.ProcessRunner実装である。
//
// docs/10-security.md §7の規則をここで満たす。executableとargvを分離してshellを
// 介さず、環境は与えられたものだけを渡し、cwd・stdio・timeout・cancel・
// process tree終了を明示する。
type ProcessRunner struct {
	// paths はcaptureした出力へ適用するpath maskerである。nilを許す。
	//
	// home/user名/hostnameはOS user lookupの結果であり、port組立て時にしか
	// 決まらない。未設定でもprocessの起動そのものは成立する。
	paths *security.PathMasker
	// clock は実行時間の計測に使う。
	clock port.Clock
	// gracePeriod はgraceful signal後の猶予である。既定は[ProcessGracePeriod]。
	//
	// package外から変更できないfieldにしている。§21の5秒をそのまま使うと、
	// 「gracefulを無視する子をtree終了できること」のtestが1件で5秒かかる。
	// production pathは常に既定値である。
	gracePeriod time.Duration
}

var _ port.ProcessRunner = (*ProcessRunner)(nil)

// NewProcessRunner はProcessRunnerを作る。
//
// clockは必須である。実行時間はwall clockではなく単調時間で測る
// （docs/02-architecture.md §4.1、[port.Clock.Since]）。
func NewProcessRunner(clock port.Clock, paths *security.PathMasker) (*ProcessRunner, error) {
	if clock == nil {
		return nil, errors.New("platform: Clockが無い")
	}
	return &ProcessRunner{paths: paths, clock: clock, gracePeriod: ProcessGracePeriod}, nil
}

// Run は子processを実行して終了を待つ。
//
// 終了経路は3つある。
//
//   - 正常終了: exit codeをそのまま返す。
//   - Timeout超過: process treeを終了し`TimedOut=true`の結果を返す。errorにしない。
//     docs/08-install-runtime.md §7がtimeoutをprobe側の判断材料としており、
//     起動できなかったこととは区別する必要がある。
//   - 呼出し側のcancel: process treeを終了し、contextのerrorを返す。
//
// 打ち切りはどちらもgraceful signal → 猶予[ProcessGracePeriod] → 所有する
// process tree終了の順で行う（docs/02-architecture.md §10）。無関係processへは
// 一切触れない。
func (r *ProcessRunner) Run(ctx context.Context, spec port.ProcessSpec) (port.ProcessResult, error) {
	if err := validateProcessSpec(spec); err != nil {
		return port.ProcessResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return port.ProcessResult{}, err
	}

	// timeoutを別contextにするのは、打ち切りの理由を区別するためである。
	// 同じcontextへ載せると、呼出し側のcancelとtimeoutが同じ`ctx.Err()`になる。
	runCtx := ctx
	var timeoutCtx context.Context
	if spec.Timeout > 0 {
		timed, cancelTimeout := context.WithTimeout(ctx, spec.Timeout)
		defer cancelTimeout()
		runCtx, timeoutCtx = timed, timed
	}

	cmd := exec.Command(spec.Executable, spec.Args...)
	cmd.Dir = spec.Dir
	cmd.Env = environmentSlice(withOSRequiredEnv(spec.Env))
	cmd.Stdin = spec.Stdin

	var stdout, stderr *limitedBuffer
	if spec.PassthroughStdio {
		// shim経由の透過はgdtvmが内容を保存もmaskもしない（docs/10-security.md §7）。
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	} else {
		stdout = newLimitedBuffer()
		stderr = newLimitedBuffer()
		cmd.Stdout, cmd.Stderr = stdout, stderr
	}

	control := newProcessControl()
	control.prepare(cmd)

	started := r.clock.Monotonic()
	if err := cmd.Start(); err != nil {
		return port.ProcessResult{}, fmt.Errorf("platform: %s を起動できない: %w", spec.Executable, err)
	}
	if err := control.attach(cmd); err != nil {
		// treeを終了させる手段が無いまま走らせない。ここで諦めると、
		// timeoutやcancelで子孫processを残すことになる。
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		control.release()
		return port.ProcessResult{}, err
	}
	defer control.release()

	// exitedはcloseで通知する。値を送ると、待つ側が2箇所（打ち切り判定と
	// 最終待ち）にあるため片方しか起きられない。
	exited := make(chan struct{})
	var waitErr error
	go func() {
		waitErr = cmd.Wait()
		close(exited)
	}()

	var timedOut bool
	select {
	case <-exited:
	case <-runCtx.Done():
		timedOut = timeoutCtx != nil && timeoutCtx.Err() != nil && ctx.Err() == nil
		r.terminate(control, exited)
		// 子の後始末が終わるまで待つ。ここを待たないとpipe copyのgoroutineが
		// 生きたままbufferを読み、結果が確定しない。
		<-exited
	}
	if cmd.ProcessState == nil {
		return port.ProcessResult{}, fmt.Errorf(
			"platform: %s の終了状態を取得できない: %w", spec.Executable, waitErr)
	}

	result := port.ProcessResult{
		ExitCode: cmd.ProcessState.ExitCode(),
		TimedOut: timedOut,
		Duration: r.clock.Since(started),
	}
	if !spec.PassthroughStdio {
		masker := security.NewOutputMasker(r.paths, spec.Env)
		result.Stdout = masker.Mask(stdout.String())
		result.Stderr = masker.Mask(stderr.String())
	}
	if err := ctx.Err(); err != nil && !timedOut {
		return result, err
	}
	if stdout.exceeded() || stderr.exceeded() {
		return result, fmt.Errorf("platform: %s: %w（各%d byte）",
			spec.Executable, ErrOutputLimit, ProcessOutputMaxBytes)
	}
	return result, nil
}

// terminate は所有するprocess treeを段階的に終了させる。
//
// docs/02-architecture.md §10「graceful signal→組込み5秒猶予→所有するprocess
// tree終了」。gracefulを先に試すのは、子processが自分でtemp fileを片付ける機会を
// 与えるためである。
func (r *ProcessRunner) terminate(control *processControl, exited <-chan struct{}) {
	if err := control.signalGraceful(); err == nil {
		// 猶予中に自分から終わればtree終了は要らない。
		timer := time.NewTimer(r.gracePeriod)
		defer timer.Stop()
		select {
		case <-exited:
			return
		case <-timer.C:
		}
	}
	_ = control.terminateTree()
}

// validateProcessSpec は起動指定の前提を検査する（docs/10-security.md §7）。
//
// executable containmentと完全versionの再確認は呼出し側の責務である。判定に必要な
// 管理rootとversionを[port.ProcessSpec]が持たず、ここでは判定できない。
func validateProcessSpec(spec port.ProcessSpec) error {
	switch {
	case spec.Executable == "":
		return errors.New("platform: Executableが空")
	case !filepath.IsAbs(spec.Executable):
		// PATH探索は呼出し側が済ませる契約である。相対pathを受けると、
		// cwdやPATHの内容で起動対象が変わる。
		return fmt.Errorf("platform: Executable %q がabsolute pathでない", spec.Executable)
	case strings.ContainsRune(spec.Executable, 0):
		return errors.New("platform: ExecutableにNULが含まれる")
	case spec.Dir == "":
		// 暗黙のcwdを使わない（docs/08-install-runtime.md §7手順2）。
		return errors.New("platform: Dirが空")
	case !filepath.IsAbs(spec.Dir):
		return fmt.Errorf("platform: Dir %q がabsolute pathでない", spec.Dir)
	case spec.Timeout < 0:
		return fmt.Errorf("platform: Timeoutが負（%s）", spec.Timeout)
	case len(spec.Env) > ProcessEnvMaxEntries:
		return fmt.Errorf("platform: 環境変数が上限%d件を超える（%d件）",
			ProcessEnvMaxEntries, len(spec.Env))
	}
	for index, arg := range spec.Args {
		if strings.ContainsRune(arg, 0) {
			return fmt.Errorf("platform: Args[%d]にNULが含まれる", index)
		}
	}
	for name, value := range spec.Env {
		switch {
		case name == "":
			return errors.New("platform: 環境変数名が空")
		case strings.ContainsRune(name, '='):
			// `=`を許すと、1 entryで別のentryを注入できる。
			return fmt.Errorf("platform: 環境変数名 %q に`=`が含まれる", name)
		case strings.ContainsRune(name, 0) || strings.ContainsRune(value, 0):
			return fmt.Errorf("platform: 環境変数 %q にNULが含まれる", name)
		}
	}
	return nil
}

// environmentSlice はenv mapを`NAME=VALUE`列へ変換する。
//
// **nilでも非nilの空sliceを返す。** [exec.Cmd]はEnvがnilのとき呼出しprocessの
// 環境を継承する。[port.ProcessSpec]は「nilは空環境を意味し、親環境の暗黙継承は
// しない」と定めるため、nilを渡すと契約が逆になる。
//
// 順序はname順に固定する。map iteration順を子processへ渡すと、同じ指定でも
// 起動ごとに環境block（[docs/11-quality-and-ci.md](../../docs/11-quality-and-ci.md)§7.2の記録対象）が変わる。
func environmentSlice(env map[string]string) []string {
	out := make([]string, 0, len(env))
	names := make([]string, 0, len(env))
	for name := range env {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		out = append(out, name+"="+env[name])
	}
	return out
}

// limitedBuffer は上限付きでstreamを保持する。
//
// 上限を超えても読取りは止めない。読むのをやめるとpipeが詰まり、子processが
// 書込みでblockして終わらなくなる。保持だけを末尾[ProcessOutputTailBytes]へ
// 切り替え、memoryを有界に保つ。
type limitedBuffer struct {
	buf   []byte
	total int64
	over  bool
}

func newLimitedBuffer() *limitedBuffer { return &limitedBuffer{} }

func (b *limitedBuffer) Write(p []byte) (int, error) {
	b.total += int64(len(p))
	b.buf = append(b.buf, p...)
	if !b.over && b.total <= ProcessOutputMaxBytes {
		return len(p), nil
	}
	b.over = true
	// 超過が分かった時点で先頭は捨てる。docs/04-storage-and-data.md §21が
	// 保持を「末尾1 MiB」と定めるため、頭を残しても使われない。
	if int64(len(b.buf)) > ProcessOutputTailBytes {
		b.buf = append(b.buf[:0], b.buf[int64(len(b.buf))-ProcessOutputTailBytes:]...)
	}
	return len(p), nil
}

func (b *limitedBuffer) String() string {
	if b == nil {
		return ""
	}
	return string(b.buf)
}

// exceeded は上限を超えたかを返す。
func (b *limitedBuffer) exceeded() bool { return b != nil && b.over }
