//go:build windows

package platform

import (
	"errors"
	"fmt"
	"os/exec"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// processControl はWindowsで所有するprocess treeを終了させるhandleである。
//
// Windowsはprocess treeを辿るAPIを持たず、親子関係も終了時に切れる。**job object**
// だけが「この起動から派生したprocess全体」を1単位として扱える
// （docs/02-architecture.md §10「所有するprocess tree終了」「無関係processを
// killしない」）。
//
// `taskkill /T`は使わない。docs/10-security.md §7が「helper、hook、backend、
// shell scriptを実行しない」「Plan `probes[]`にないexternal executableを
// Execute中に発見して起動しない」と定めており、終了処理のために別programを
// 起動することがそれ自体で違反になる。
type processControl struct {
	job windows.Handle
	// pid はCTRL_BREAK_EVENTの宛先となるprocess group IDである。
	// CREATE_NEW_PROCESS_GROUPで起動した子は、自身のpidがgroup IDになる。
	pid int
}

func newProcessControl() *processControl { return &processControl{} }

// prepare は起動前のattributeを設定する。
func (c *processControl) prepare(cmd *exec.Cmd) {
	// CTRL_BREAK_EVENTはprocess group単位でしか送れない。既定のまま起動すると
	// 子はgdtvm自身と同じgroupに入り、graceful signalが自分へ返ってくる。
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP}
}

// attach は起動直後にjob objectを作って子を割り当てる。
//
// **起動とjob割当ての間にはわずかな隙間がある。** `os/exec`はCREATE_SUSPENDEDで
// 起動したthread handleを公開しておらず、割当て後にresumeする方法が無い。この隙間に
// 子が孫を作ると、その孫はjobの外へ出る。install engineが起動する外部processは
// definition宣言のvalidation probeだけ（docs/10-security.md §7）で、いずれも
// 自身のversionを出力して終わる短命processのため、この隙間を許容する。
func (c *processControl) attach(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return errors.New("platform: 起動したprocessが取得できない")
	}
	c.pid = cmd.Process.Pid

	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return fmt.Errorf("platform: job objectを作れない: %w", err)
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
		BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{
			// gdtvmが異常終了してjob handleが閉じられた場合でも、残った子を
			// OSに終了させる。handleを閉じ忘れた経路でprocessが残らない。
			LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
		},
	}
	if _, err := windows.SetInformationJobObject(
		job,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	); err != nil {
		_ = windows.CloseHandle(job)
		return fmt.Errorf("platform: job objectを設定できない: %w", err)
	}

	// pidではなくGoが保持しているhandleで割り当てる。pidから開き直すと、
	// 子が既に終了してpidが再利用された場合に無関係processをjobへ入れてしまう。
	var assignErr error
	if err := cmd.Process.WithHandle(func(handle uintptr) {
		assignErr = windows.AssignProcessToJobObject(job, windows.Handle(handle))
	}); err != nil {
		_ = windows.CloseHandle(job)
		return fmt.Errorf("platform: process handleを取得できない: %w", err)
	}
	if assignErr != nil {
		_ = windows.CloseHandle(job)
		return fmt.Errorf("platform: processをjob objectへ割り当てられない: %w", assignErr)
	}
	c.job = job
	return nil
}

// signalGraceful はprocess groupへCTRL_BREAK_EVENTを送る。
//
// WindowsにSIGTERMは無い。console control eventが、子へ「片付けてから終われ」と
// 伝える唯一の手段である。console を持たない状況では失敗するが、その場合は
// 呼出し側がgraceful猶予を省いてtree終了へ進む。
func (c *processControl) signalGraceful() error {
	if c.pid <= 0 {
		return errors.New("platform: process group IDが未設定")
	}
	return windows.GenerateConsoleCtrlEvent(windows.CTRL_BREAK_EVENT, uint32(c.pid))
}

// terminateTree はjobへ属する全processを終了させる。
func (c *processControl) terminateTree() error {
	if c.job == 0 {
		return errors.New("platform: job objectが未設定")
	}
	return windows.TerminateJobObject(c.job, 1)
}

// release はjob handleを閉じる。
//
// `JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE`があるため、閉じた時点で残っている
// job内のprocessは終了する。Runは子の終了を待ってから呼ぶので、正常経路で
// 生きているprocessは無い。
func (c *processControl) release() {
	if c.job != 0 {
		_ = windows.CloseHandle(c.job)
		c.job = 0
	}
}
