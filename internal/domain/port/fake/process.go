package fake

import (
	"context"
	"errors"
	"strings"
	"sync"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain/port"
)

// ProcessRunner操作名。
const OpProcessRun = "process.Run"

// ErrProbeFailed はprobe失敗のfailure injectionで使う。
// docs/11-quality-and-ci.md §8 のscenario 10が要求する。
var ErrProbeFailed = errors.New("fake: probe failed")

// ProcessStub は1つのexecutableに対する結果定義である。
type ProcessStub struct {
	ExitCode int
	Stdout   string
	Stderr   string
	TimedOut bool
}

// ProcessInvocation は実行要求の記録である。
//
// docs/11-quality-and-ci.md §7.2 は「変更operationで起動した全probe processが
// Plan probes[] のexecutable/argv/cwd/write pathと一致し、任意helper processが
// ないこと」を要求する。その照合に使うため、要求内容をそのまま残す。
type ProcessInvocation struct {
	Executable  string
	Args        []string
	Dir         string
	Env         map[string]string
	Passthrough bool
}

// ProcessRunner は登録した結果だけを返す決定的なport.ProcessRunnerである。
type ProcessRunner struct {
	mu       sync.Mutex
	stubs    map[string]ProcessStub
	injector *Injector
	// Invocations は実行要求を順に記録する。
	Invocations []ProcessInvocation
}

var _ port.ProcessRunner = (*ProcessRunner)(nil)

// NewProcessRunner は空のProcessRunnerを作る。
func NewProcessRunner(injector *Injector) *ProcessRunner {
	if injector == nil {
		injector = NewInjector()
	}
	return &ProcessRunner{stubs: make(map[string]ProcessStub), injector: injector}
}

// Injector は失敗注入器を返す。
func (p *ProcessRunner) Injector() *Injector { return p.injector }

// Stub はexecutableに対する結果を登録する。
func (p *ProcessRunner) Stub(executable string, stub ProcessStub) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stubs[executable] = stub
}

// Run は子processの実行を模倣する。
//
// 未登録のexecutableはerrorにする。仕様が禁止する任意helper processの起動を
// testが黙って通さないためである。
func (p *ProcessRunner) Run(ctx context.Context, spec port.ProcessSpec) (port.ProcessResult, error) {
	if err := ctx.Err(); err != nil {
		return port.ProcessResult{}, err
	}
	if err := p.injector.Check(OpProcessRun); err != nil {
		return port.ProcessResult{}, err
	}
	if spec.Dir == "" {
		return port.ProcessResult{}, errors.New("fake: ProcessSpec.Dir must not be empty")
	}

	p.mu.Lock()
	p.Invocations = append(p.Invocations, ProcessInvocation{
		Executable:  spec.Executable,
		Args:        append([]string(nil), spec.Args...),
		Dir:         spec.Dir,
		Env:         copyEnv(spec.Env),
		Passthrough: spec.PassthroughStdio,
	})
	stub, ok := p.stubs[spec.Executable]
	p.mu.Unlock()

	if !ok {
		return port.ProcessResult{}, errors.New("fake: no stub registered for " + spec.Executable)
	}
	result := port.ProcessResult{
		ExitCode: stub.ExitCode,
		TimedOut: stub.TimedOut,
	}
	// PassthroughStdio時はgdtvmが内容を保存しない（docs/10-security.md）。
	// fakeも同じ契約に従い、captureした文字列を結果へ入れない。
	if !spec.PassthroughStdio {
		result.Stdout = stub.Stdout
		result.Stderr = stub.Stderr
	}
	return result, nil
}

// ExecutablesRun は実行されたexecutableを順に返す。
func (p *ProcessRunner) ExecutablesRun() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]string, 0, len(p.Invocations))
	for _, inv := range p.Invocations {
		out = append(out, inv.Executable)
	}
	return out
}

// CommandLines は「executable arg1 arg2」形式で実行記録を返す。
// 期待argvとの照合をtestで書きやすくするための補助である。
func (p *ProcessRunner) CommandLines() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]string, 0, len(p.Invocations))
	for _, inv := range p.Invocations {
		parts := append([]string{inv.Executable}, inv.Args...)
		out = append(out, strings.Join(parts, " "))
	}
	return out
}

func copyEnv(env map[string]string) map[string]string {
	if env == nil {
		return nil
	}
	out := make(map[string]string, len(env))
	for k, v := range env {
		out[k] = v
	}
	return out
}
