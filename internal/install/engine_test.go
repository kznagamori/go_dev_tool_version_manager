package install

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/definition"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain/port/fake"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/progress"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/store"
)

const (
	engineOperationsDir = "/data/gdtvm/tmp/operations"
	engineOperation     = "cccccccccccccccccccccccccccccccc"
	engineOperationDir  = engineOperationsDir + "/" + engineOperation
	enginePayloadDir    = engineOperationDir + "/payload"
	engineDestination   = "/data/gdtvm/tools/go/versions/1.25.0/linux-amd64-glibc"
	engineIndexPath     = "/data/gdtvm/state/receipt-index.toml"
)

// engineHarness はengine 1件分のfakeをまとめる。
type engineHarness struct {
	fs      *fake.FileSystem
	http    *fake.HTTPClient
	process *fake.ProcessRunner
	inject  *fake.Injector
	stager  *Stager
	engine  *Engine
}

func newEngineHarness(t *testing.T) *engineHarness {
	t.Helper()
	injector := fake.NewInjector()
	filesystem := fake.NewFileSystem(injector)
	for _, dir := range []string{cacheDir, engineOperationsDir, "/data/gdtvm/state"} {
		if err := filesystem.MkdirAll(dir, 0o700); err != nil {
			t.Fatalf("MkdirAll(%q): %v", dir, err)
		}
	}
	client := fake.NewHTTPClient(injector)
	process := fake.NewProcessRunner(injector)
	clock := fake.NewClock(time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC))
	reporter := progress.NewReporter(&recordingSink{})

	downloader, err := NewDownloader(client, filesystem, reporter)
	if err != nil {
		t.Fatalf("NewDownloader: %v", err)
	}
	extractor, err := NewExtractor(filesystem, reporter)
	if err != nil {
		t.Fatalf("NewExtractor: %v", err)
	}
	stager, err := NewStager(filesystem, downloader, extractor, reporter)
	if err != nil {
		t.Fatalf("NewStager: %v", err)
	}
	probes, err := NewProbeRunner(filesystem, process, clock)
	if err != nil {
		t.Fatalf("NewProbeRunner: %v", err)
	}
	committer, err := NewCommitter(filesystem)
	if err != nil {
		t.Fatalf("NewCommitter: %v", err)
	}
	engine, err := NewEngine(filesystem, stager, probes, committer)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	return &engineHarness{
		fs: filesystem, http: client, process: process, inject: injector,
		stager: stager, engine: engine,
	}
}

// enginePlan はdownload・extract・probeを1件ずつ持つPlanを返す。
func enginePlan(t *testing.T, payload []byte) store.Plan {
	t.Helper()
	plan := stagePlan(t, payload)
	operation, err := domain.ParseOperationID(engineOperation)
	if err != nil {
		t.Fatalf("ParseOperationID: %v", err)
	}
	plan.Operation = operation
	plan.Probes = []store.PlanProbe{{
		ID:               "go-version",
		RuntimeCommand:   "go",
		Executable:       stagePathValue(t, domain.RolePayload, enginePayloadDir+"/bin/go"),
		Args:             []store.PlanArg{{Kind: store.ArgLiteral, Value: "version"}},
		WorkingDirectory: stagePathValue(t, domain.RoleStaging, engineOperationDir+"/probes/go-version"),
		Stream:           store.StreamStdout,
		Expect:           store.ExpectVersion,
		Regex:            `go(\d+\.\d+\.\d+)`,
		ExpectedVersion:  "1.25.0",
		TimeoutMillis:    30_000,
		Required:         true,
	}}
	return plan
}

func engineRequest(t *testing.T, payload []byte) EngineRequest {
	t.Helper()
	roots := testRoots(t, "linux-amd64-glibc")
	roots.ProbeTemp = domain.PathValue{}
	receipt := conflictReceipt(t)
	receipt.CommandTargets = nil
	receipt.Probes = nil
	return EngineRequest{
		Plan: enginePlan(t, payload),
		Platform: definition.Platform{
			Runtime: definition.Runtime{Commands: []definition.Command{{
				Name: "go", Target: "{{payload}}/bin/go", Required: true,
			}}},
		},
		Roots:          roots,
		OperationsRoot: stagePathValue(t, domain.RoleStaging, engineOperationsDir),
		MaxRedirects:   5,
		Receipt:        receipt,
		Destination:    stagePathValue(t, domain.RoleVersionData, engineDestination),
		ReceiptName:    commitReceiptName,
		Index:          store.ReceiptIndex{Revision: 1, RootID: strings.Repeat("2", 32)},
		IndexPath:      stagePathValue(t, domain.RoleReceiptIndex, engineIndexPath),
		IndexEntryPath: "tools/go/versions/1.25.0/linux-amd64-glibc/" + commitReceiptName,
		Now:            time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC),
	}
}

// TestEngineRunsFullSequence は§7の手順が順に走ることを固定する。
func TestEngineRunsFullSequence(t *testing.T) {
	payload := goTarGz(t)
	h := newEngineHarness(t)
	h.http.Stub(testURL, fake.HTTPStub{StatusCode: http.StatusOK, Body: payload})
	h.process.Stub(enginePayloadDir+"/bin/go",
		fake.ProcessStub{Stdout: "go version go1.25.0 linux/amd64"})

	result, err := h.engine.Run(context.Background(), engineRequest(t, payload))
	if err != nil {
		t.Fatalf("Run = %v", err)
	}

	// 完成先へ移動している。
	if _, statErr := h.fs.Stat(engineDestination + "/payload/bin/go"); statErr != nil {
		t.Errorf("完成先にpayloadが無い: %v", statErr)
	}
	// 手順4: command_targetsが埋まっている。
	if len(result.Receipt.CommandTargets) != 1 {
		t.Fatalf("command_targets = %d件, want 1", len(result.Receipt.CommandTargets))
	}
	if result.Receipt.CommandTargets[0].Path != "payload/bin/go" {
		t.Errorf("command_target = %q", result.Receipt.CommandTargets[0].Path)
	}
	// probe結果がreceiptへ入っている。
	if len(result.Receipt.Probes) != 1 ||
		result.Receipt.Probes[0].Status != store.ProbePassed {
		t.Errorf("receipt probes = %+v", result.Receipt.Probes)
	}
	if result.Receipt.Probes[0].ReportedVersion != "1.25.0" {
		t.Errorf("reported version = %q", result.Receipt.Probes[0].ReportedVersion)
	}
	// 手順8: indexが進んでいる。
	if result.Index.Revision != 2 {
		t.Errorf("index revision = %d, want 2", result.Index.Revision)
	}
	if result.IndexStale {
		t.Error("index更新が成功したのにIndexStaleになった")
	}
	// operation directoryは呼出し側が消す。
	if result.OperationDir.Path() != engineOperationDir {
		t.Errorf("operation dir = %q", result.OperationDir.Path())
	}
}

// TestEngineHardensPayloadBeforeCommit はpermission正規化の順序を固定する。
//
// §7手順5→7。**正規化してからrenameする** — 完成先へ移してから正規化すると、
// 正規化前の状態が一瞬でも利用可能になる。
func TestEngineHardensPayloadBeforeCommit(t *testing.T) {
	payload := goTarGz(t)
	h := newEngineHarness(t)
	h.http.Stub(testURL, fake.HTTPStub{StatusCode: http.StatusOK, Body: payload})
	h.process.Stub(enginePayloadDir+"/bin/go",
		fake.ProcessStub{Stdout: "go version go1.25.0 linux/amd64"})

	if _, err := h.engine.Run(context.Background(), engineRequest(t, payload)); err != nil {
		t.Fatalf("Run = %v", err)
	}
	// 正規化はstaging側のpathに対して行われている（renameより前）。
	records := h.fs.Hardened()
	if len(records) == 0 {
		t.Fatal("permission正規化が行われていない")
	}
	for _, record := range records {
		if !strings.HasPrefix(record.Path, enginePayloadDir) {
			t.Errorf("staging外を正規化した: %q", record.Path)
		}
	}
}

// TestEngineStopsWhenProbeFails はprobe失敗でcommitしないことを固定する。
//
// §11「required probe failureは**commit前に**install全体を失敗させる」。
func TestEngineStopsWhenProbeFails(t *testing.T) {
	payload := goTarGz(t)
	h := newEngineHarness(t)
	h.http.Stub(testURL, fake.HTTPStub{StatusCode: http.StatusOK, Body: payload})
	h.process.Stub(enginePayloadDir+"/bin/go", fake.ProcessStub{ExitCode: 1})

	result, err := h.engine.Run(context.Background(), engineRequest(t, payload))
	if err == nil {
		t.Fatal("probe失敗で成功した")
	}
	if err.Code != domain.CodeProbeFailed {
		t.Errorf("code = %s, want %s", err.Code, domain.CodeProbeFailed)
	}
	// **完成先を作っていない。**
	if _, statErr := h.fs.Stat(engineDestination); statErr == nil {
		t.Error("probe失敗なのにcommitした")
	}
	// 後始末できるようoperation directoryを返す。
	if result.OperationDir.IsZero() {
		t.Error("失敗時にoperation directoryを返していない")
	}
}

// TestEngineReportsStaleIndexWithoutFailing はindex失敗の扱いを固定する。
//
// §7「手順7のrenameが完了した時点でinstallは成功とみなす」。
func TestEngineReportsStaleIndexWithoutFailing(t *testing.T) {
	payload := goTarGz(t)
	h := newEngineHarness(t)
	h.http.Stub(testURL, fake.HTTPStub{StatusCode: http.StatusOK, Body: payload})
	h.process.Stub(enginePayloadDir+"/bin/go",
		fake.ProcessStub{Stdout: "go version go1.25.0 linux/amd64"})
	// receipt、indexの順にAtomicWriteが走る（downloadはWriteStreamを使う）。
	// indexだけ落とす。
	h.inject.Fail(fake.OpAtomicWrite, 1, 1, errors.New("注入したindex書込み失敗"))

	result, err := h.engine.Run(context.Background(), engineRequest(t, payload))
	if err != nil {
		t.Fatalf("index更新失敗がinstallの失敗になった: %v", err)
	}
	if !result.IndexStale {
		t.Error("IndexStaleが立っていない")
	}
	// **完成先は残る。**
	if _, statErr := h.fs.Stat(engineDestination + "/payload/bin/go"); statErr != nil {
		t.Errorf("index失敗で完成先が巻き戻された: %v", statErr)
	}
}

// TestEngineRejectsInvalidRequest は前提違反を拒否することを固定する。
func TestEngineRejectsInvalidRequest(t *testing.T) {
	payload := goTarGz(t)
	tests := []struct {
		name   string
		mutate func(*EngineRequest)
	}{
		{"operationがinstallでない", func(r *EngineRequest) {
			r.Plan.Kind = store.OperationUninstall
		}},
		{"hostが未設定", func(r *EngineRequest) { r.Roots.Host = domain.Platform{} }},
		{"完成先が未設定", func(r *EngineRequest) { r.Destination = domain.PathValue{} }},
		{"receipt名が未設定", func(r *EngineRequest) { r.ReceiptName = "" }},
		{"時刻が未設定", func(r *EngineRequest) { r.Now = time.Time{} }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			h := newEngineHarness(t)
			req := engineRequest(t, payload)
			test.mutate(&req)
			if _, err := h.engine.Run(context.Background(), req); err == nil {
				t.Fatal("前提違反が通った")
			}
		})
	}
}

// TestNewEngineRequiresDependencies は依存不足を拒否することを固定する。
func TestNewEngineRequiresDependencies(t *testing.T) {
	h := newEngineHarness(t)
	probes, err := NewProbeRunner(h.fs, h.process, fake.NewClock(time.Now()))
	if err != nil {
		t.Fatalf("NewProbeRunner: %v", err)
	}
	committer, err := NewCommitter(h.fs)
	if err != nil {
		t.Fatalf("NewCommitter: %v", err)
	}
	if _, engineErr := NewEngine(nil, h.stager, probes, committer); engineErr == nil {
		t.Error("FileSystem無しで作れた")
	}
	if _, engineErr := NewEngine(h.fs, nil, probes, committer); engineErr == nil {
		t.Error("Stager無しで作れた")
	}
	if _, engineErr := NewEngine(h.fs, h.stager, nil, committer); engineErr == nil {
		t.Error("ProbeRunner無しで作れた")
	}
	if _, engineErr := NewEngine(h.fs, h.stager, probes, nil); engineErr == nil {
		t.Error("Committer無しで作れた")
	}
}
