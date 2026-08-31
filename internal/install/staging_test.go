package install

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain/port"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain/port/fake"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/progress"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/store"
)

const (
	operationsDir  = "/data/gdtvm/tmp/operations"
	stageOperation = "cccccccccccccccccccccccccccccccc"
)

// stageHarness はstaging 1件分のfakeをまとめる。
type stageHarness struct {
	http   *fake.HTTPClient
	fs     *fake.FileSystem
	inject *fake.Injector
	stager *Stager
}

func newStageHarness(t *testing.T) *stageHarness {
	t.Helper()
	injector := fake.NewInjector()
	filesystem := fake.NewFileSystem(injector)
	if err := filesystem.MkdirAll(cacheDir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := filesystem.MkdirAll(operationsDir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	client := fake.NewHTTPClient(injector)
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
	return &stageHarness{http: client, fs: filesystem, inject: injector, stager: stager}
}

// stagePlan はdownload 1件とextract 1件を持つinstall Planを返す。
func stagePlan(t *testing.T, payload []byte) store.Plan {
	t.Helper()
	operation, err := domain.ParseOperationID(stageOperation)
	if err != nil {
		t.Fatalf("ParseOperationID: %v", err)
	}
	tool, err := domain.ParseToolID("go")
	if err != nil {
		t.Fatalf("ParseToolID: %v", err)
	}
	return store.Plan{
		Kind:      store.OperationInstall,
		Operation: operation,
		Summary:   store.PlanSummary{Tool: tool, Version: "1.25.0"},
		Downloads: []store.PlanDownload{{
			ID:             "artifact",
			URL:            testURL,
			ExpectedDigest: upstreamOf(t, payload, domain.AlgoSHA256),
			Size:           int64(len(payload)),
			Destination:    cachePath(t),
		}},
		Extracts: []store.PlanExtract{{
			ID:               "artifact-extract",
			SourceDownloadID: "artifact",
			Format:           store.FormatTarGz,
			StripComponents:  1,
			Destination:      stagePathValue(t, domain.RoleStaging, operationsDir),
		}},
	}
}

func stagePathValue(t *testing.T, role domain.PathRole, path string) domain.PathValue {
	t.Helper()
	value, err := domain.NewPathValue(role, path)
	if err != nil {
		t.Fatalf("NewPathValue(%s, %q): %v", role, path, err)
	}
	return value
}

func stageRequest(t *testing.T, payload []byte) StageRequest {
	t.Helper()
	return StageRequest{
		Plan:           stagePlan(t, payload),
		OperationsRoot: stagePathValue(t, domain.RoleStaging, operationsDir),
		Host:           platformOf(t, "linux-amd64-glibc"),
		MaxRedirects:   5,
	}
}

// TestStageDownloadsAndExtractsIntoOperationDir はstagingの配置を固定する。
//
// docs/08-install-runtime.md §6「operation tmpは…`tmp/operations/<operation-id>/`
// 配下だけを書く。payload/storage/currentへ直接書かない」。
func TestStageDownloadsAndExtractsIntoOperationDir(t *testing.T) {
	payload := goTarGz(t)
	h := newStageHarness(t)
	h.http.Stub(testURL, fake.HTTPStub{StatusCode: http.StatusOK, Body: payload})

	result, err := h.stager.Stage(context.Background(), stageRequest(t, payload))
	if err != nil {
		t.Fatalf("Stage = %v", err)
	}

	wantDir := operationsDir + "/" + stageOperation
	if result.OperationDir.Path() != wantDir {
		t.Errorf("operation dir = %q, want %q", result.OperationDir.Path(), wantDir)
	}
	// §17.2はstaging内の展開後内容をrole=payloadと定める。
	if result.PayloadDir.Role() != domain.RolePayload {
		t.Errorf("payload role = %s, want payload", result.PayloadDir.Role())
	}
	if want := wantDir + "/payload"; result.PayloadDir.Path() != want {
		t.Errorf("payload dir = %q, want %q", result.PayloadDir.Path(), want)
	}
	if len(result.Downloads) != 1 || len(result.Extracts) != 1 {
		t.Fatalf("downloads=%d extracts=%d, want 1/1",
			len(result.Downloads), len(result.Extracts))
	}
	// strip_components=1でtop-level `go/`が落ちること。
	if _, statErr := h.fs.Stat(wantDir + "/payload/bin/go"); statErr != nil {
		t.Errorf("payload内のfileが無い: %v", statErr)
	}

	// **operation directoryの外へ書いていないこと。** download cacheは
	// Planのdestinationが指す別領域で、それ以外の書込みが無いことを見る。
	walkErr := h.fs.Walk("/", func(path string, _ port.FileInfo) error {
		if path == "/" || strings.HasPrefix(path, "/data") {
			return nil
		}
		t.Errorf("data root外へ書いた: %q", path)
		return nil
	})
	if walkErr != nil {
		t.Fatalf("Walk: %v", walkErr)
	}
}

// TestStageCleanupRemovesOperationDir は後始末がdirectory単位であることを固定する。
//
// §6「中断・失敗・cancel時は`tmp/operations/<operation-id>/`をdirectory単位で
// 削除すれば復旧する」。
func TestStageCleanupRemovesOperationDir(t *testing.T) {
	payload := goTarGz(t)
	h := newStageHarness(t)
	h.http.Stub(testURL, fake.HTTPStub{StatusCode: http.StatusOK, Body: payload})

	result, err := h.stager.Stage(context.Background(), stageRequest(t, payload))
	if err != nil {
		t.Fatalf("Stage = %v", err)
	}
	operation, parseErr := domain.ParseOperationID(stageOperation)
	if parseErr != nil {
		t.Fatalf("ParseOperationID: %v", parseErr)
	}
	if cleanErr := h.stager.Cleanup(result.OperationDir, operation); cleanErr != nil {
		t.Fatalf("Cleanup = %v", cleanErr)
	}
	if _, statErr := h.fs.Stat(result.OperationDir.Path()); statErr == nil {
		t.Error("operation directoryが残っている")
	}
	// download cacheは消さない。再実行で再利用できる派生dataである。
	if _, statErr := h.fs.Stat(cacheFile); statErr != nil {
		t.Errorf("download cacheまで消された: %v", statErr)
	}
}

// TestStageCleanupRejectsForeignDirectory は他operationのdirectoryを消さないことを固定する。
//
// **誤って別operationのstagingを渡されても消さない。** 同時実行中の別operationの
// 作業領域を消すと、そちらが不整合な状態で継続する。
func TestStageCleanupRejectsForeignDirectory(t *testing.T) {
	h := newStageHarness(t)
	operation, err := domain.ParseOperationID(stageOperation)
	if err != nil {
		t.Fatalf("ParseOperationID: %v", err)
	}
	other := strings.Repeat("d", 32)
	tests := []struct {
		name string
		dir  domain.PathValue
	}{
		{"別operationのdirectory",
			stagePathValue(t, domain.RoleStaging, operationsDir+"/"+other)},
		{"operations root自体",
			stagePathValue(t, domain.RoleStaging, operationsDir)},
		// 末尾が一致するだけのdirectoryを消さない。
		{"末尾が一致するだけ",
			stagePathValue(t, domain.RoleStaging, operationsDir+"/x"+stageOperation)},
		{"roleが違う",
			stagePathValue(t, domain.RolePayload, operationsDir+"/"+stageOperation)},
		{"未設定", domain.PathValue{}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if cleanErr := h.stager.Cleanup(test.dir, operation); cleanErr == nil {
				t.Fatal("対象外のdirectoryが削除された")
			}
		})
	}
}

// TestStageStopsOnCancel はcancel境界を固定する。
//
// §2「cancelはdownload、checksum取得、展開entry、probeの境界で確認する」。
func TestStageStopsOnCancel(t *testing.T) {
	payload := goTarGz(t)
	h := newStageHarness(t)
	h.http.Stub(testURL, fake.HTTPStub{StatusCode: http.StatusOK, Body: payload})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := h.stager.Stage(ctx, stageRequest(t, payload))
	if err == nil {
		t.Fatal("cancel済みcontextで成功した")
	}
	if err.Code != domain.CodeCancelled {
		t.Errorf("code = %s, want %s", err.Code, domain.CodeCancelled)
	}
	// downloadすら始めていないこと。
	if len(h.http.Requests) != 0 {
		t.Errorf("cancel後にHTTPを%d回呼んだ", len(h.http.Requests))
	}
}

// TestStageFailsWhenDownloadFails は失敗を伝播し、部分削除しないことを固定する。
//
// §6の後始末はdirectory単位の削除1回で足りる。途中で部分削除すると、
// 何が残っているかが失敗経路ごとに変わる。
func TestStageFailsWhenDownloadFails(t *testing.T) {
	payload := goTarGz(t)
	h := newStageHarness(t)
	h.http.Stub(testURL, fake.HTTPStub{StatusCode: http.StatusInternalServerError})

	result, err := h.stager.Stage(context.Background(), stageRequest(t, payload))
	if err == nil {
		t.Fatal("download失敗で成功した")
	}
	// operation directoryは作ったまま返る。呼出し側がCleanupを1回呼ぶ。
	if result.OperationDir.IsZero() {
		t.Fatal("失敗時にoperation directoryを返していない")
	}
	if _, statErr := h.fs.Stat(result.OperationDir.Path()); statErr != nil {
		t.Errorf("失敗時にoperation directoryが消えている: %v", statErr)
	}
}

// TestStageFailsWhenExtractFails は展開失敗の伝播を固定する。
func TestStageFailsWhenExtractFails(t *testing.T) {
	// tar.gzを宣言しながらzipを渡す。
	payload := goZip(t)
	h := newStageHarness(t)
	h.http.Stub(testURL, fake.HTTPStub{StatusCode: http.StatusOK, Body: payload})

	if _, err := h.stager.Stage(context.Background(), stageRequest(t, payload)); err == nil {
		t.Fatal("形式違いの展開が成功した")
	}
}

// TestStageReportsFilesystemFailure は失敗注入を固定する。
func TestStageReportsFilesystemFailure(t *testing.T) {
	payload := goTarGz(t)
	h := newStageHarness(t)
	h.http.Stub(testURL, fake.HTTPStub{StatusCode: http.StatusOK, Body: payload})
	h.inject.Fail(fake.OpMkdirAll, 0, 1, errors.New("注入したmkdir失敗"))

	if _, err := h.stager.Stage(context.Background(), stageRequest(t, payload)); err == nil {
		t.Fatal("mkdir失敗で成功した")
	}
}

// TestStageRejectsInvalidRequest は前提違反を拒否することを固定する。
func TestStageRejectsInvalidRequest(t *testing.T) {
	payload := goTarGz(t)
	tests := []struct {
		name   string
		mutate func(*StageRequest)
	}{
		{"operationがinstallでない", func(r *StageRequest) {
			r.Plan.Kind = store.OperationUninstall
		}},
		{"operation IDが未設定", func(r *StageRequest) {
			r.Plan.Operation = domain.OperationID{}
		}},
		{"operations rootが未設定", func(r *StageRequest) {
			r.OperationsRoot = domain.PathValue{}
		}},
		{"operations rootのroleが違う", func(r *StageRequest) {
			r.OperationsRoot = stagePathValue(t, domain.RolePayload, operationsDir)
		}},
		{"hostが未設定", func(r *StageRequest) { r.Host = domain.Platform{} }},
		{"downloadが無い", func(r *StageRequest) { r.Plan.Downloads = nil }},
		{"extractのsourceがPlanに無い", func(r *StageRequest) {
			r.Plan.Extracts[0].SourceDownloadID = "missing"
		}},
		{"archive formatが未知", func(r *StageRequest) {
			r.Plan.Extracts[0].Format = store.ArchiveFormat("7z")
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			h := newStageHarness(t)
			h.http.Stub(testURL, fake.HTTPStub{StatusCode: http.StatusOK, Body: payload})
			req := stageRequest(t, payload)
			test.mutate(&req)
			if _, err := h.stager.Stage(context.Background(), req); err == nil {
				t.Fatal("前提違反が通った")
			}
		})
	}
}

// TestNewStagerRequiresDependencies は依存不足を拒否することを固定する。
func TestNewStagerRequiresDependencies(t *testing.T) {
	injector := fake.NewInjector()
	filesystem := fake.NewFileSystem(injector)
	reporter := progress.NewReporter(&recordingSink{})
	downloader, err := NewDownloader(fake.NewHTTPClient(injector), filesystem, reporter)
	if err != nil {
		t.Fatalf("NewDownloader: %v", err)
	}
	extractor, err := NewExtractor(filesystem, reporter)
	if err != nil {
		t.Fatalf("NewExtractor: %v", err)
	}
	if _, stagerErr := NewStager(nil, downloader, extractor, reporter); stagerErr == nil {
		t.Error("FileSystem無しで作れた")
	}
	if _, stagerErr := NewStager(filesystem, nil, extractor, reporter); stagerErr == nil {
		t.Error("Downloader無しで作れた")
	}
	if _, stagerErr := NewStager(filesystem, downloader, nil, reporter); stagerErr == nil {
		t.Error("Extractor無しで作れた")
	}
}
