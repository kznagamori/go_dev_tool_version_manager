package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain/port"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain/port/fake"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/store"
)

// stubInputSource は`inputs`の現在値を返すstubである。
type stubInputSource struct {
	inputs store.PlanInputs
	err    error
	// calls は再取得回数である。lock前後の2回を数えるために持つ。
	calls int
	// after は2回目以降に返す値である。zeroならinputsを返す。
	after *store.PlanInputs
}

func (s *stubInputSource) PlanInputs(context.Context) (store.PlanInputs, error) {
	s.calls++
	if s.err != nil {
		return store.PlanInputs{}, s.err
	}
	if s.calls > 1 && s.after != nil {
		return *s.after, nil
	}
	return s.inputs, nil
}

// stubEngine は決められた結果を返すengineである。
type stubEngine struct {
	result InstallEngineResult
	err    error
	calls  int
	// lockHeld は呼ばれた時点で保持していたlockを記録する。
	lockHeld []port.LockClass
	locks    port.LockManager
}

func (s *stubEngine) Run(context.Context, store.Plan) (InstallEngineResult, error) {
	s.calls++
	if s.locks != nil {
		s.lockHeld = s.locks.Held()
	}
	return s.result, s.err
}

// executeHarness はExecuteInstall 1件分のstubをまとめる。
type executeHarness struct {
	inputs   *stubInputSource
	engine   *stubEngine
	locks    *fake.LockManager
	injector *fake.Injector
}

func newExecuteHarness(t *testing.T) *executeHarness {
	t.Helper()
	injector := fake.NewInjector()
	clock := fake.NewClock(time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC))
	locks := fake.NewLockManager(injector, clock)
	engine := &stubEngine{locks: locks}
	return &executeHarness{
		inputs:   &stubInputSource{inputs: testPlanInputs()},
		engine:   engine,
		locks:    locks,
		injector: injector,
	}
}

// executePlan は承認不要なinstall Planを返す。
func executePlan(t *testing.T, invocation domain.InvocationID) store.Plan {
	t.Helper()
	tool, err := domain.ParseToolID("go")
	if err != nil {
		t.Fatalf("ParseToolID: %v", err)
	}
	operation, err := domain.ParseOperationID(strings.Repeat("b", 32))
	if err != nil {
		t.Fatalf("ParseOperationID: %v", err)
	}
	return store.Plan{
		Kind:          store.OperationInstall,
		ClientVersion: "2026.09.01.01",
		Invocation:    invocation,
		Operation:     operation,
		Summary: store.PlanSummary{
			Tool:     tool,
			Version:  "1.25.0",
			Platform: hostOf(t, "linux-amd64-glibc"),
		},
		Inputs: testPlanInputs(),
	}
}

func executeRequest(t *testing.T, h *executeHarness) ExecuteInstallRequest {
	t.Helper()
	invocation, err := domain.ParseInvocationID(strings.Repeat("a", 32))
	if err != nil {
		t.Fatalf("ParseInvocationID: %v", err)
	}
	approval, err := NewApproval(ApprovalInteractive, nil)
	if err != nil {
		t.Fatalf("NewApproval: %v", err)
	}
	return ExecuteInstallRequest{
		Plan:       executePlan(t, invocation),
		Approval:   approval,
		Build:      BuildInfo{ClientVersion: "2026.09.01.01"},
		Invocation: invocation,
		Inputs:     h.inputs,
		Engine:     h.engine,
		Locks:      h.locks,
	}
}

// TestExecuteInstallRunsEngineUnderLock は§8手順4の順序を固定する。
//
// docs/02-architecture.md §8「**lock取得後に**同じ検査を繰り返す」。
// engineはlockを保持した状態で走らなければならない。
func TestExecuteInstallRunsEngineUnderLock(t *testing.T) {
	h := newExecuteHarness(t)
	result, err := ExecuteInstall(context.Background(), executeRequest(t, h))
	if err != nil {
		t.Fatalf("ExecuteInstall = %v", err)
	}
	if h.engine.calls != 1 {
		t.Fatalf("engine起動 = %d回, want 1", h.engine.calls)
	}
	// **engineはlock保持中に走る。**
	if len(h.engine.lockHeld) != 1 || h.engine.lockHeld[0] != port.ClassInstall {
		t.Errorf("engine起動時のlock = %v, want [install]", h.engine.lockHeld)
	}
	// **`inputs`の再取得はlock前後で2回。**
	if h.inputs.calls != 2 {
		t.Errorf("inputs再取得 = %d回, want 2（lock前後）", h.inputs.calls)
	}
	if len(result.Warnings) != 0 {
		t.Errorf("warnings = %+v, want 空", result.Warnings)
	}
	// lockは解放されている。
	if held := h.locks.Held(); len(held) != 0 {
		t.Errorf("lockが解放されていない: %v", held)
	}
}

// TestExecuteInstallDetectsStaleAfterLock はlock取得後の変化を捕まえることを固定する。
//
// **1回目だけだと、lockを待っている間の変化を見逃す。**
func TestExecuteInstallDetectsStaleAfterLock(t *testing.T) {
	h := newExecuteHarness(t)
	changed := testPlanInputs()
	changed.SetupRevision = 99
	h.inputs.after = &changed

	_, err := ExecuteInstall(context.Background(), executeRequest(t, h))
	if err == nil {
		t.Fatal("lock取得後の変化を見逃した")
	}
	if err.Code != domain.CodePlanStale {
		t.Errorf("code = %s, want %s", err.Code, domain.CodePlanStale)
	}
	// engineを走らせていないこと。
	if h.engine.calls != 0 {
		t.Errorf("staleなのにengineを%d回起動した", h.engine.calls)
	}
}

// TestExecuteInstallChecksInOrder は§8手順1〜3の順序を固定する。
//
// **前の手順で落ちたら後の手順へ進まない。** 承認前にstale判定をすると、
// 承認されていないPlanの入力を読むことになる。
func TestExecuteInstallChecksInOrder(t *testing.T) {
	tests := []struct {
		name       string
		mutate     func(*ExecuteInstallRequest, *executeHarness)
		wantCode   domain.ErrorCode
		wantInputs int
	}{
		{"手順1: client不一致", func(r *ExecuteInstallRequest, _ *executeHarness) {
			r.Build.ClientVersion = "2020.01.01.01"
		}, domain.CodePlanStale, 0},
		{"手順2: 未承認", func(r *ExecuteInstallRequest, _ *executeHarness) {
			messageID, _ := domain.ParseMessageID("plan.eol")
			r.Plan.Warnings = []store.PlanWarning{
				store.NewPlanWarning(store.WarnEOL, messageID, nil),
			}
		}, domain.CodeApprovalRequired, 0},
		{"手順3: stale", func(r *ExecuteInstallRequest, h *executeHarness) {
			h.inputs.inputs.ConfigSHA256 = strings.Repeat("9", 64)
		}, domain.CodePlanStale, 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			h := newExecuteHarness(t)
			req := executeRequest(t, h)
			test.mutate(&req, h)

			_, err := ExecuteInstall(context.Background(), req)
			if err == nil {
				t.Fatal("検査が通ってしまった")
			}
			if err.Code != test.wantCode {
				t.Errorf("code = %s, want %s", err.Code, test.wantCode)
			}
			if h.inputs.calls != test.wantInputs {
				t.Errorf("inputs再取得 = %d回, want %d", h.inputs.calls, test.wantInputs)
			}
			if h.engine.calls != 0 {
				t.Errorf("検査に失敗したのにengineを%d回起動した", h.engine.calls)
			}
		})
	}
}

// TestExecuteInstallCleansUpOnBothPaths は成功・失敗のどちらでも後始末することを固定する。
//
// docs/08-install-runtime.md §6「中断・失敗・cancel時は
// `tmp/operations/<operation-id>/`をdirectory単位で削除すれば復旧する」。
func TestExecuteInstallCleansUpOnBothPaths(t *testing.T) {
	tests := []struct {
		name      string
		engineErr error
	}{
		{"成功", nil},
		{"失敗", errors.New("engine失敗")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			h := newExecuteHarness(t)
			cleaned := 0
			h.engine.err = test.engineErr
			h.engine.result = InstallEngineResult{
				Cleanup: func() error { cleaned++; return nil },
			}

			_, err := ExecuteInstall(context.Background(), executeRequest(t, h))
			if (test.engineErr == nil) != (err == nil) {
				t.Fatalf("ExecuteInstall = %v", err)
			}
			if cleaned != 1 {
				t.Errorf("cleanup = %d回, want 1", cleaned)
			}
		})
	}
}

// TestExecuteInstallWarnsOnCleanupFailure は清掃失敗の扱いを固定する。
//
// docs/08-install-runtime.md §2「commit後の一時file清掃失敗は成功＋
// `W_CLEANUP_INCOMPLETE`とし、**正常payloadを巻き戻さない**」。
func TestExecuteInstallWarnsOnCleanupFailure(t *testing.T) {
	h := newExecuteHarness(t)
	h.engine.result = InstallEngineResult{
		Cleanup: func() error { return errors.New("清掃失敗") },
	}

	result, err := ExecuteInstall(context.Background(), executeRequest(t, h))
	if err != nil {
		t.Fatalf("清掃失敗がinstallの失敗になった: %v", err)
	}
	if len(result.Warnings) != 1 ||
		result.Warnings[0].Code != store.WarnCleanupIncomplete {
		t.Errorf("warnings = %+v, want [W_CLEANUP_INCOMPLETE]", result.Warnings)
	}
}

// TestExecuteInstallKeepsEngineErrorOverCleanupFailure は失敗時の優先順位を固定する。
//
// **元の失敗のほうが利用者に必要な情報である。** 清掃失敗で上書きすると、
// 何が起きたのか分からなくなる。
func TestExecuteInstallKeepsEngineErrorOverCleanupFailure(t *testing.T) {
	h := newExecuteHarness(t)
	engineErr := &domain.Error{Code: domain.CodeProbeFailed, Cause: errors.New("probe失敗")}
	h.engine.err = engineErr
	h.engine.result = InstallEngineResult{
		Cleanup: func() error { return errors.New("清掃も失敗") },
	}

	_, err := ExecuteInstall(context.Background(), executeRequest(t, h))
	if err == nil {
		t.Fatal("失敗しなかった")
	}
	if err.Code != domain.CodeProbeFailed {
		t.Errorf("code = %s, want %s（元の失敗）", err.Code, domain.CodeProbeFailed)
	}
}

// TestExecuteInstallWarnsOnStaleIndex はindex更新だけの失敗の扱いを固定する。
//
// §7「手順7のrenameが完了した時点でinstallは成功とみなす」。
func TestExecuteInstallWarnsOnStaleIndex(t *testing.T) {
	h := newExecuteHarness(t)
	h.engine.result = InstallEngineResult{IndexStale: true}

	result, err := ExecuteInstall(context.Background(), executeRequest(t, h))
	if err != nil {
		t.Fatalf("index更新失敗がinstallの失敗になった: %v", err)
	}
	if len(result.Warnings) != 1 {
		t.Errorf("warnings = %+v, want 1件", result.Warnings)
	}
}

// TestExecuteInstallReportsAlreadyInstalled は冪等installの伝播を固定する。
func TestExecuteInstallReportsAlreadyInstalled(t *testing.T) {
	h := newExecuteHarness(t)
	h.engine.result = InstallEngineResult{AlreadyInstalled: true}

	result, err := ExecuteInstall(context.Background(), executeRequest(t, h))
	if err != nil {
		t.Fatalf("ExecuteInstall = %v", err)
	}
	if !result.AlreadyInstalled {
		t.Error("AlreadyInstalledが伝播していない")
	}
}

// TestExecuteInstallFailsWhenInputsUnavailable は再取得できない場合を固定する。
//
// **「変わっていない」と見なして進めない。** 再取得できなければstaleかどうかを
// 判定できず、判定できないまま作用を始めるのは危険である。
func TestExecuteInstallFailsWhenInputsUnavailable(t *testing.T) {
	h := newExecuteHarness(t)
	h.inputs.err = errors.New("読めない")

	_, err := ExecuteInstall(context.Background(), executeRequest(t, h))
	if err == nil {
		t.Fatal("inputs再取得失敗で進んだ")
	}
	if h.engine.calls != 0 {
		t.Errorf("再取得できないのにengineを%d回起動した", h.engine.calls)
	}
}

// TestExecuteInstallMapsLockTimeout はlock timeoutのcodeを固定する。
func TestExecuteInstallMapsLockTimeout(t *testing.T) {
	h := newExecuteHarness(t)
	h.injector.Fail(fake.OpAcquireLock, 0, 1,
		fmt.Errorf("%w: 待ち時間超過", port.ErrLockTimeout))

	_, err := ExecuteInstall(context.Background(), executeRequest(t, h))
	if err == nil {
		t.Fatal("lock timeoutで成功した")
	}
	if err.Code != domain.CodeLockTimeout {
		t.Errorf("code = %s, want %s", err.Code, domain.CodeLockTimeout)
	}
	// docs/03-cli.md §7の終了codeは8である。
	if err.ExitCode() != 8 {
		t.Errorf("終了code = %d, want 8", err.ExitCode())
	}
}

// TestExecuteInstallRejectsInvalidRequest は前提違反を拒否することを固定する。
func TestExecuteInstallRejectsInvalidRequest(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*ExecuteInstallRequest)
	}{
		{"operationがinstallでない", func(r *ExecuteInstallRequest) {
			r.Plan.Kind = store.OperationUninstall
		}},
		{"input供給元が未設定", func(r *ExecuteInstallRequest) { r.Inputs = nil }},
		{"engineが未設定", func(r *ExecuteInstallRequest) { r.Engine = nil }},
		{"LockManagerが未設定", func(r *ExecuteInstallRequest) { r.Locks = nil }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			h := newExecuteHarness(t)
			req := executeRequest(t, h)
			test.mutate(&req)
			if _, err := ExecuteInstall(context.Background(), req); err == nil {
				t.Fatal("前提違反が通った")
			}
		})
	}
}
