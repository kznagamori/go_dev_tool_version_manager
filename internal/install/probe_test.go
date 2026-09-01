package install

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain/port/fake"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/store"
)

const (
	probeExecutable = "/data/gdtvm/tmp/operations/op1/payload/bin/go"
	probeTempDir    = "/data/gdtvm/tmp/operations/op1/probes/go-version"
	probePayloadDir = "/data/gdtvm/tmp/operations/op1/payload"
)

// probeHarness はprobe実行1件分のfakeをまとめる。
type probeHarness struct {
	fs      *fake.FileSystem
	process *fake.ProcessRunner
	inject  *fake.Injector
	runner  *ProbeRunner
}

func newProbeHarness(t *testing.T) *probeHarness {
	t.Helper()
	injector := fake.NewInjector()
	filesystem := fake.NewFileSystem(injector)
	if err := filesystem.MkdirAll(probePayloadDir+"/bin", 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	filesystem.AddFile(probeExecutable, []byte("go"), 0o755)
	process := fake.NewProcessRunner(injector)
	clock := fake.NewClock(time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC))
	runner, err := NewProbeRunner(filesystem, process, clock)
	if err != nil {
		t.Fatalf("NewProbeRunner: %v", err)
	}
	return &probeHarness{fs: filesystem, process: process, inject: injector, runner: runner}
}

// probePlan はversion probeを1件持つPlanを返す。
func probePlan(t *testing.T) store.Plan {
	t.Helper()
	return store.Plan{
		Kind: store.OperationInstall,
		Probes: []store.PlanProbe{{
			ID:               "go-version",
			RuntimeCommand:   "go",
			Executable:       stagePathValue(t, domain.RolePayload, probeExecutable),
			Args:             []store.PlanArg{{Kind: store.ArgLiteral, Value: "version"}},
			WorkingDirectory: stagePathValue(t, domain.RoleStaging, probeTempDir),
			Stream:           store.StreamStdout,
			Expect:           store.ExpectVersion,
			Regex:            `go(\d+\.\d+\.\d+)`,
			ExpectedVersion:  "1.25.0",
			TimeoutMillis:    30_000,
			Required:         true,
		}},
	}
}

func probeRequest(t *testing.T) ProbeRequest {
	t.Helper()
	return ProbeRequest{Plan: probePlan(t), Host: platformOf(t, "linux-amd64-glibc")}
}

// TestProbeRunsInSanitizedEnvironmentAndTemp はprobeの起動条件を固定する。
//
// docs/08-install-runtime.md §7手順2「**sanitized最小環境で、probe専用の
// owner-only temp directoryをcwdとして**required probeを実行する」。
// docs/10-security.md §7「sanitized環境へは、OSが起動に要求する最小変数だけを
// **process adapterが**補う」。呼出し側は空環境を渡す。
func TestProbeRunsInSanitizedEnvironmentAndTemp(t *testing.T) {
	h := newProbeHarness(t)
	h.process.Stub(probeExecutable, fake.ProcessStub{Stdout: "go version go1.25.0 linux/amd64"})

	outcomes, err := h.runner.Run(context.Background(), probeRequest(t))
	if err != nil {
		t.Fatalf("Run = %v", err)
	}
	if len(outcomes) != 1 || outcomes[0].Status != store.ProbePassed {
		t.Fatalf("outcomes = %+v", outcomes)
	}
	if outcomes[0].ReportedVersion != "1.25.0" {
		t.Errorf("reported version = %q, want 1.25.0", outcomes[0].ReportedVersion)
	}

	if len(h.process.Invocations) != 1 {
		t.Fatalf("起動 = %d回, want 1", len(h.process.Invocations))
	}
	invocation := h.process.Invocations[0]
	if invocation.Executable != probeExecutable {
		t.Errorf("executable = %q, want %q", invocation.Executable, probeExecutable)
	}
	// **cwdはprobe tempであり、payloadではない**（§11）。
	if invocation.Dir != probeTempDir {
		t.Errorf("cwd = %q, want %q", invocation.Dir, probeTempDir)
	}
	if invocation.Dir == probePayloadDir {
		t.Error("cwdがpayloadになっている（§11違反）")
	}
	// **親環境を継承しない。** 呼出し側は空環境を渡し、OS必須変数の補填は
	// process adapterの責務である。
	if len(invocation.Env) != 0 {
		t.Errorf("env = %v, want 空", invocation.Env)
	}
}

// TestProbeCreatesAndRemovesTemp はprobe tempのlifecycleを固定する。
//
// docs/06-tool-definition.md §11「probeごとに**空の**owner-only probe tempを作り、
// **成功/失敗/cancel後にengineが削除する**」。
func TestProbeCreatesAndRemovesTemp(t *testing.T) {
	tests := []struct {
		name string
		stub fake.ProcessStub
		ok   bool
	}{
		{"成功", fake.ProcessStub{Stdout: "go version go1.25.0 linux/amd64"}, true},
		{"失敗", fake.ProcessStub{ExitCode: 1}, false},
		{"timeout", fake.ProcessStub{TimedOut: true}, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			h := newProbeHarness(t)
			h.process.Stub(probeExecutable, test.stub)
			// 前回の中断が残したfileを置く。§11の「空の」を確かめる。
			h.fs.AddFile(probeTempDir+"/stale", []byte("old"), 0o644)

			_, err := h.runner.Run(context.Background(), probeRequest(t))
			if test.ok != (err == nil) {
				t.Fatalf("Run = %v, want ok=%v", err, test.ok)
			}
			// **成功・失敗のいずれでもtempを残さない。**
			if _, statErr := h.fs.Stat(probeTempDir); statErr == nil {
				t.Error("probe tempが残っている")
			}
		})
	}
}

// TestProbeDetectsVersionMismatch はversion不一致を固定する。
//
// §7「version/root/path/能力不一致は`E_PROBE_FAILED`」。
func TestProbeDetectsVersionMismatch(t *testing.T) {
	h := newProbeHarness(t)
	h.process.Stub(probeExecutable, fake.ProcessStub{Stdout: "go version go1.24.0 linux/amd64"})

	_, err := h.runner.Run(context.Background(), probeRequest(t))
	if err == nil {
		t.Fatal("version不一致で成功した")
	}
	if err.Code != domain.CodeProbeFailed {
		t.Errorf("code = %s, want %s", err.Code, domain.CodeProbeFailed)
	}
	// docs/03-cli.md §7の終了codeは6である。
	if err.ExitCode() != 6 {
		t.Errorf("終了code = %d, want 6", err.ExitCode())
	}
}

// TestProbeFailsOnNonZeroAndTimeout は起動結果の判定を固定する。
func TestProbeFailsOnNonZeroAndTimeout(t *testing.T) {
	tests := []struct {
		name string
		stub fake.ProcessStub
	}{
		{"nonzero exit", fake.ProcessStub{ExitCode: 2, Stderr: "boom"}},
		{"timeout", fake.ProcessStub{TimedOut: true}},
		// regexが一致しない出力。
		{"regex不一致", fake.ProcessStub{Stdout: "unexpected output"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			h := newProbeHarness(t)
			h.process.Stub(probeExecutable, test.stub)
			_, err := h.runner.Run(context.Background(), probeRequest(t))
			if err == nil {
				t.Fatal("失敗するはずのprobeが成功した")
			}
			if err.Code != domain.CodeProbeFailed {
				t.Errorf("code = %s, want %s", err.Code, domain.CodeProbeFailed)
			}
		})
	}
}

// TestProbeIncludesStderrInDiagnosis は診断へstderrを載せることを固定する。
//
// §7「probe stderr末尾はmask/上限後だけhuman errorへ含める」。
// [port.ProcessResult]のStderrはadapterがmaskと上限適用を済ませた値である。
func TestProbeIncludesStderrInDiagnosis(t *testing.T) {
	h := newProbeHarness(t)
	h.process.Stub(probeExecutable, fake.ProcessStub{ExitCode: 1, Stderr: "missing libc"})

	_, err := h.runner.Run(context.Background(), probeRequest(t))
	if err == nil {
		t.Fatal("失敗しなかった")
	}
	unwrapped := err.Unwrap()
	if unwrapped == nil || !strings.Contains(unwrapped.Error(), "missing libc") {
		t.Errorf("causeがstderrを含まない: %v", unwrapped)
	}
}

// TestProbeChecksExpectPathWithin は`path-within`の判定を固定する。
//
// §11「regexで取り出したpathが指定root内にあることを要求する」。
func TestProbeChecksExpectPathWithin(t *testing.T) {
	payload := stagePathValue(t, domain.RolePayload, probePayloadDir)
	tests := []struct {
		name   string
		stdout string
		ok     bool
	}{
		{"payload内", "sys.prefix = " + probePayloadDir + "/lib", true},
		{"payload外", "sys.prefix = /usr/local/lib", false},
		// prefixが一致するだけのpathを配下と誤判定しない。
		{"prefixが一致するだけ", "sys.prefix = " + probePayloadDir + "-other/lib", false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			h := newProbeHarness(t)
			h.process.Stub(probeExecutable, fake.ProcessStub{Stdout: test.stdout})
			req := probeRequest(t)
			req.Plan.Probes[0].Expect = store.ExpectPathWithin
			req.Plan.Probes[0].Regex = `sys\.prefix = (\S+)`
			req.Plan.Probes[0].ExpectedVersion = ""
			req.Plan.Probes[0].ExpectedRoot = &payload

			_, err := h.runner.Run(context.Background(), req)
			if test.ok != (err == nil) {
				t.Fatalf("Run = %v, want ok=%v", err, test.ok)
			}
		})
	}
}

// TestProbeChecksRequiredPaths は`required_paths`の判定を固定する。
//
// §11「probe成功直後に指定種別の存在を要求する」。
//
// **probe temp内のrequired pathは「無い」caseだけを見る。** runnerがprobe起動前に
// tempを空にするため（§11「**空の**owner-only probe temp」）、事前に置いたfileは
// 消える。実際にはprobeが自分でvenv等を作るが、fake [port.ProcessRunner]は副作用を
// 持てない。存在するcaseはpayload内のpathで見る——§11は`required_paths`へ
// `{{payload}}`も許しており、判定logicは同じである。
func TestProbeChecksRequiredPaths(t *testing.T) {
	tests := []struct {
		name    string
		kind    store.RequiredPathKind
		path    string
		role    domain.PathRole
		prepare func(*probeHarness)
		ok      bool
	}{
		{"fileがある", store.RequiredFile, probePayloadDir + "/bin/go",
			domain.RolePayload, func(*probeHarness) {}, true},
		{"directoryがある", store.RequiredDirectory, probePayloadDir + "/bin",
			domain.RolePayload, func(*probeHarness) {}, true},
		// probeがvenvを作れなかった場合。
		{"probe temp内のfileが無い", store.RequiredFile,
			probeTempDir + "/venv/bin/python", domain.RoleStaging,
			func(*probeHarness) {}, false},
		{"payload内のfileが無い", store.RequiredFile, probePayloadDir + "/bin/missing",
			domain.RolePayload, func(*probeHarness) {}, false},
		{"directoryを期待したがfile", store.RequiredDirectory,
			probePayloadDir + "/bin/go", domain.RolePayload,
			func(*probeHarness) {}, false},
		{"fileを期待したがdirectory", store.RequiredFile, probePayloadDir + "/bin",
			domain.RolePayload, func(*probeHarness) {}, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			h := newProbeHarness(t)
			h.process.Stub(probeExecutable,
				fake.ProcessStub{Stdout: "go version go1.25.0 linux/amd64"})
			test.prepare(h)
			req := probeRequest(t)
			req.Plan.Probes[0].RequiredPaths = []store.PlanRequiredPath{{
				Kind: test.kind,
				Path: stagePathValue(t, test.role, test.path),
			}}

			_, err := h.runner.Run(context.Background(), req)
			if test.ok != (err == nil) {
				t.Fatalf("Run = %v, want ok=%v", err, test.ok)
			}
		})
	}
}

// TestProbeStopsAtFirstFailure は最初の失敗で止めることを固定する。
//
// §11「required probe failureはcommit前にinstall全体を失敗させる」。
// 後続を走らせても結果は捨てるため、利用者を待たせるだけである。
func TestProbeStopsAtFirstFailure(t *testing.T) {
	h := newProbeHarness(t)
	h.process.Stub(probeExecutable, fake.ProcessStub{ExitCode: 1})
	req := probeRequest(t)
	second := req.Plan.Probes[0]
	second.ID = "go-env"
	req.Plan.Probes = append(req.Plan.Probes, second)

	if _, err := h.runner.Run(context.Background(), req); err == nil {
		t.Fatal("失敗しなかった")
	}
	if len(h.process.Invocations) != 1 {
		t.Errorf("起動 = %d回, want 1（最初の失敗で止まる）", len(h.process.Invocations))
	}
}

// TestProbeStopsOnCancel はcancel境界を固定する。
func TestProbeStopsOnCancel(t *testing.T) {
	h := newProbeHarness(t)
	h.process.Stub(probeExecutable, fake.ProcessStub{Stdout: "go version go1.25.0 linux/amd64"})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := h.runner.Run(ctx, probeRequest(t))
	if err == nil {
		t.Fatal("cancel済みcontextで成功した")
	}
	if err.Code != domain.CodeCancelled {
		t.Errorf("code = %s, want %s", err.Code, domain.CodeCancelled)
	}
	if len(h.process.Invocations) != 0 {
		t.Errorf("cancel後に%d回起動した", len(h.process.Invocations))
	}
}

// TestProbeReportsStartFailure は起動失敗の注入を固定する。
//
// §7「permission/OS起動失敗は対応するplatform/filesystem error」。
func TestProbeReportsStartFailure(t *testing.T) {
	h := newProbeHarness(t)
	// stubを登録しない。fakeは未登録executableをerrorにする——仕様が禁じる
	// 任意helper processの起動をtestが黙って通さないためである。
	_, err := h.runner.Run(context.Background(), probeRequest(t))
	if err == nil {
		t.Fatal("未登録executableで成功した")
	}
	if err.Code == domain.CodeProbeFailed {
		t.Errorf("起動失敗をprobe失敗として扱っている: %s", err.Code)
	}
}

// TestProbeReportsTempFailure はtemp作成失敗の注入を固定する。
func TestProbeReportsTempFailure(t *testing.T) {
	h := newProbeHarness(t)
	h.process.Stub(probeExecutable, fake.ProcessStub{Stdout: "go version go1.25.0 linux/amd64"})
	h.inject.Fail(fake.OpMkdirAll, 0, 1, errors.New("注入したmkdir失敗"))

	if _, err := h.runner.Run(context.Background(), probeRequest(t)); err == nil {
		t.Fatal("temp作成失敗で成功した")
	}
}

// TestNewProbeRunnerRequiresDependencies は依存不足を拒否することを固定する。
func TestNewProbeRunnerRequiresDependencies(t *testing.T) {
	injector := fake.NewInjector()
	filesystem := fake.NewFileSystem(injector)
	process := fake.NewProcessRunner(injector)
	clock := fake.NewClock(time.Now())
	if _, err := NewProbeRunner(nil, process, clock); err == nil {
		t.Error("FileSystem無しで作れた")
	}
	if _, err := NewProbeRunner(filesystem, nil, clock); err == nil {
		t.Error("ProcessRunner無しで作れた")
	}
	if _, err := NewProbeRunner(filesystem, process, nil); err == nil {
		t.Error("Clock無しで作れた")
	}
}
