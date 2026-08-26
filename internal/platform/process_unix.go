//go:build !windows

package platform

import (
	"errors"
	"os/exec"
	"syscall"
)

// processControl はLinuxで所有するprocess treeを終了させるhandleである。
//
// process groupを単位にする。子を新しいgroupのleaderにすればgroup IDが子のpidと
// 一致し、`kill(-pid)`が子とその子孫だけへ届く。pidを1件ずつ辿ると、辿っている
// 間に生まれた孫を取りこぼす。
type processControl struct {
	// pid は子のpidであり、同時にprocess group IDでもある。
	pid int
}

func newProcessControl() *processControl { return &processControl{} }

// prepare は起動前のattributeを設定する。
func (c *processControl) prepare(cmd *exec.Cmd) {
	// Setpgidが無いとgdtvm自身と同じprocess groupで動く。その状態で`kill(-pid)`を
	// 送ると自分自身と無関係な兄弟processまで巻き込む
	// （docs/02-architecture.md §10「無関係processをkillしない」）。
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// attach は起動直後に呼ぶ。Linuxではprocess groupが起動時に確定するため何もしない。
func (c *processControl) attach(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return errors.New("platform: 起動したprocessが取得できない")
	}
	c.pid = cmd.Process.Pid
	return nil
}

// signalGraceful はprocess group全体へSIGTERMを送る。
func (c *processControl) signalGraceful() error {
	return c.signalGroup(syscall.SIGTERM)
}

// terminateTree はprocess group全体へSIGKILLを送る。
func (c *processControl) terminateTree() error {
	return c.signalGroup(syscall.SIGKILL)
}

func (c *processControl) signalGroup(signal syscall.Signal) error {
	if c.pid <= 0 {
		return errors.New("platform: process group IDが未設定")
	}
	// 負のpidはprocess group全体を指す（kill(2)）。pidそのものを渡すと
	// 子だけが死に、子が起こした孫が残る。
	return syscall.Kill(-c.pid, signal)
}

// release は解放するhandleが無いため何もしない。
func (c *processControl) release() {}

// withOSRequiredEnv はOSが起動に要求する最小変数を補う。
//
// docs/10-security.md §7は補う変数を「WindowsのSystemRootだけ」と定める。
// Linuxは何も補わない。空環境の子processもそのまま起動できるためである。
func withOSRequiredEnv(env map[string]string) map[string]string { return env }
