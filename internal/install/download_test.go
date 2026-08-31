package install

import (
	"context"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain/port"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain/port/fake"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/progress"
)

const (
	testURL   = "https://example.invalid/tool-1.0.0.tar.gz"
	cacheDir  = "/data/cache/downloads"
	cacheFile = cacheDir + "/tool-1.0.0.tar.gz"
)

// recordingSink はprogress通知を記録する。
//
// onReportは通知のたびに呼ばれる。展開の途中でcancelするなど、通知を起点に
// 状態を変えるtestで使う。時間ではなく通知回数で切るため決定的になる。
type recordingSink struct {
	reports  []progress.Progress
	onReport func()
}

func (s *recordingSink) Report(p progress.Progress) {
	s.reports = append(s.reports, p)
	if s.onReport != nil {
		s.onReport()
	}
}

// harness はdownload 1件分のfakeをまとめる。
type harness struct {
	http       *fake.HTTPClient
	fs         *fake.FileSystem
	sink       *recordingSink
	downloader *Downloader
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	injector := fake.NewInjector()
	filesystem := fake.NewFileSystem(injector)
	if err := filesystem.MkdirAll(cacheDir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	client := fake.NewHTTPClient(injector)
	sink := &recordingSink{}
	downloader, err := NewDownloader(client, filesystem, progress.NewReporter(sink))
	if err != nil {
		t.Fatalf("NewDownloader: %v", err)
	}
	return &harness{http: client, fs: filesystem, sink: sink, downloader: downloader}
}

func upstreamOf(t *testing.T, payload []byte, algo domain.DigestAlgorithm) domain.Digest {
	t.Helper()
	var text string
	switch algo {
	case domain.AlgoSHA512:
		sum := sha512.Sum512(payload)
		text = "sha512:" + hex.EncodeToString(sum[:])
	default:
		sum := sha256.Sum256(payload)
		text = "sha256:" + hex.EncodeToString(sum[:])
	}
	digest, err := domain.ParseUpstreamDigest(text)
	if err != nil {
		t.Fatalf("ParseUpstreamDigest: %v", err)
	}
	return digest
}

func cachePath(t *testing.T) domain.PathValue {
	t.Helper()
	value, err := domain.NewPathValue(domain.RoleDownloadCache, cacheFile)
	if err != nil {
		t.Fatalf("NewPathValue: %v", err)
	}
	return value
}

func testRequest(t *testing.T, payload []byte) Request {
	t.Helper()
	return Request{
		URL:            testURL,
		ExpectedDigest: upstreamOf(t, payload, domain.AlgoSHA256),
		ExpectedSize:   int64(len(payload)),
		CachePath:      cachePath(t),
		MaxRedirects:   5,
	}
}

// TestDownloadWritesVerifiedFile は`.part`書込み→digest照合→renameの手順を
// 固定する。
func TestDownloadWritesVerifiedFile(t *testing.T) {
	payload := []byte(strings.Repeat("artifact", 128))
	h := newHarness(t)
	h.http.Stub(testURL, fake.HTTPStub{StatusCode: http.StatusOK, Body: payload})

	result, err := h.downloader.Download(context.Background(), testRequest(t, payload))
	if err != nil {
		t.Fatalf("Download = %v", err.Cause)
	}
	if result.FromCache {
		t.Error("最初の取得でFromCacheがtrue")
	}
	if result.Size != int64(len(payload)) {
		t.Errorf("Size = %d, want %d", result.Size, len(payload))
	}
	sum := sha256.Sum256(payload)
	if result.InternalDigest.Hex() != hex.EncodeToString(sum[:]) {
		t.Errorf("InternalDigest = %s", result.InternalDigest.Hex())
	}

	// 完成fileが最終pathにあり、`.part`は残っていない。
	if _, statErr := h.fs.Stat(cacheFile); statErr != nil {
		t.Errorf("完成fileが無い: %v", statErr)
	}
	if _, statErr := h.fs.Stat(cacheFile + PartSuffix); statErr == nil {
		t.Error("`.part`が残っている")
	}
}

// TestDownloadReportsByteProgress はbyte単位のprogressを通知することを固定する。
func TestDownloadReportsByteProgress(t *testing.T) {
	payload := []byte(strings.Repeat("x", 4096))
	h := newHarness(t)
	h.http.Stub(testURL, fake.HTTPStub{StatusCode: http.StatusOK, Body: payload})

	if _, err := h.downloader.Download(context.Background(), testRequest(t, payload)); err != nil {
		t.Fatalf("Download = %v", err.Cause)
	}

	var downloads, verifies int
	var last int64
	for _, report := range h.sink.reports {
		switch report.Phase {
		case progress.PhaseDownload:
			downloads++
			if report.Unit != progress.UnitBytes {
				t.Errorf("download unit = %q, want bytes", report.Unit)
			}
			// Currentは単調非減少である（docs/02-architecture.md §10）。
			if report.Current < last {
				t.Errorf("Currentが減った: %d → %d", last, report.Current)
			}
			last = report.Current
			if report.Total == nil || *report.Total != int64(len(payload)) {
				t.Errorf("Total = %v, want %d", report.Total, len(payload))
			}
		case progress.PhaseVerify:
			verifies++
		}
	}
	if downloads == 0 {
		t.Error("download progressが1件も無い")
	}
	if verifies == 0 {
		t.Error("verify progressが無い")
	}
	if last != int64(len(payload)) {
		t.Errorf("最後のCurrent = %d, want %d", last, len(payload))
	}
}

// TestDownloadFallsBackToContentLength は宣言sizeが不明でも応答の
// `Content-Length`を総量として使うことを固定する。
//
// §16の`size=0`は「providerが公開していない」を表すが、応答が長さを示すなら
// 進捗の総量として使える。
func TestDownloadFallsBackToContentLength(t *testing.T) {
	payload := []byte("unknown declared size")
	h := newHarness(t)
	h.http.Stub(testURL, fake.HTTPStub{StatusCode: http.StatusOK, Body: payload})

	req := testRequest(t, payload)
	req.ExpectedSize = 0
	if _, err := h.downloader.Download(context.Background(), req); err != nil {
		t.Fatalf("Download = %v", err.Cause)
	}
	var seen bool
	for _, report := range h.sink.reports {
		if report.Phase != progress.PhaseDownload {
			continue
		}
		seen = true
		if report.Total == nil || *report.Total != int64(len(payload)) {
			t.Errorf("Total = %v, want %d", report.Total, len(payload))
		}
	}
	if !seen {
		t.Error("download progressが1件も無い")
	}
}

// TestCountingReaderOmitsUnknownTotal は総量が分からないときTotalを付けない
// ことを固定する。
//
// 0を総量として出すと、進捗率が常に100%か0%になる。宣言sizeも`Content-Length`も
// 無い配布元はこの状態になる。
func TestCountingReaderOmitsUnknownTotal(t *testing.T) {
	sink := &recordingSink{}
	reader := &countingReader{
		inner:    strings.NewReader("body"),
		total:    0,
		reporter: progress.NewReporter(sink),
	}
	if _, err := io.Copy(io.Discard, reader); err != nil {
		t.Fatalf("Copy: %v", err)
	}
	if len(sink.reports) == 0 {
		t.Fatal("progressが1件も無い")
	}
	for _, report := range sink.reports {
		if report.Total != nil {
			t.Errorf("総量不明なのにTotal = %d", *report.Total)
		}
	}
}

// TestDownloadDiscardsPartialOnDigestMismatch はdigest不一致で`.part`を残さない
// ことを固定する。
//
// 途中まで書けたfileを次回の再開に使わない（docs/15-deferred.md D-24）。
func TestDownloadDiscardsPartialOnDigestMismatch(t *testing.T) {
	payload := []byte("actual bytes")
	h := newHarness(t)
	h.http.Stub(testURL, fake.HTTPStub{StatusCode: http.StatusOK, Body: payload})

	req := testRequest(t, payload)
	// providerが公開したdigestと違うbytesが返る状況を作る。
	req.ExpectedDigest = upstreamOf(t, []byte("expected bytes"), domain.AlgoSHA256)
	req.ExpectedSize = int64(len(payload))

	_, err := h.downloader.Download(context.Background(), req)
	if err == nil {
		t.Fatal("digest不一致で成功した")
	}
	if err.Code != domain.CodeChecksumMismatch {
		t.Errorf("code = %s, want %s", err.Code, domain.CodeChecksumMismatch)
	}
	// checksum不一致はretryableにしない（docs/02-architecture.md §14）。
	if err.Retryable {
		t.Error("checksum不一致がretryable=trueになっている")
	}
	if _, statErr := h.fs.Stat(cacheFile + PartSuffix); statErr == nil {
		t.Error("digest不一致なのに`.part`が残っている")
	}
	if _, statErr := h.fs.Stat(cacheFile); statErr == nil {
		t.Error("未検証のbytesが最終pathへ現れた")
	}
}

// TestDownloadRejectsSizeMismatch は宣言sizeと違う応答を拒否することを固定する。
func TestDownloadRejectsSizeMismatch(t *testing.T) {
	payload := []byte("short")
	h := newHarness(t)
	h.http.Stub(testURL, fake.HTTPStub{StatusCode: http.StatusOK, Body: payload})

	req := testRequest(t, payload)
	req.ExpectedSize = int64(len(payload)) + 10

	_, err := h.downloader.Download(context.Background(), req)
	if err == nil {
		t.Fatal("size不一致で成功した")
	}
	if err.Code != domain.CodeChecksumMismatch {
		t.Errorf("code = %s", err.Code)
	}
	if _, statErr := h.fs.Stat(cacheFile + PartSuffix); statErr == nil {
		t.Error("`.part`が残っている")
	}
}

// TestDownloadReusesCache は完成cacheをdigest照合のうえ再利用することを固定する。
//
// docs/10-security.md §10「download cacheはURL identityとdigestが一致する
// complete fileだけを再利用し、partial fileのRange再開は行わない」。
func TestDownloadReusesCache(t *testing.T) {
	payload := []byte("cached artifact")
	h := newHarness(t)
	h.fs.AddFile(cacheFile, payload, 0o600)
	// stubを登録しない。取得しに行けば「未登録URL」でerrorになる。

	result, err := h.downloader.Download(context.Background(), testRequest(t, payload))
	if err != nil {
		t.Fatalf("Download = %v", err.Cause)
	}
	if !result.FromCache {
		t.Error("cacheを再利用していない")
	}
	if result.Size != int64(len(payload)) {
		t.Errorf("Size = %d", result.Size)
	}
	if len(h.http.Requests) != 0 {
		t.Errorf("cacheがあるのに取得した: %v", h.http.Requests)
	}
}

// TestDownloadRefetchesCorruptedCache は壊れたcacheを使わず取り直すことを
// 固定する。
func TestDownloadRefetchesCorruptedCache(t *testing.T) {
	payload := []byte("correct artifact")
	h := newHarness(t)
	// digestが合わない内容をcacheへ置く。
	h.fs.AddFile(cacheFile, []byte("corrupted"), 0o600)
	h.http.Stub(testURL, fake.HTTPStub{StatusCode: http.StatusOK, Body: payload})

	result, err := h.downloader.Download(context.Background(), testRequest(t, payload))
	if err != nil {
		t.Fatalf("Download = %v", err.Cause)
	}
	if result.FromCache {
		t.Error("壊れたcacheを再利用した")
	}
	if len(h.http.Requests) != 1 {
		t.Errorf("取得回数 = %d, want 1", len(h.http.Requests))
	}
}

// TestDownloadIgnoresPartialAsCache は`.part`だけがある状態をcacheとして
// 扱わないことを固定する。
//
// partial fileのRange再開を行わない（D-24）。完成fileと途中のfileをbasenameで
// 区別する理由がこれである。
func TestDownloadIgnoresPartialAsCache(t *testing.T) {
	payload := []byte("artifact body")
	h := newHarness(t)
	h.fs.AddFile(cacheFile+PartSuffix, payload[:4], 0o600)
	h.http.Stub(testURL, fake.HTTPStub{StatusCode: http.StatusOK, Body: payload})

	result, err := h.downloader.Download(context.Background(), testRequest(t, payload))
	if err != nil {
		t.Fatalf("Download = %v", err.Cause)
	}
	if result.FromCache {
		t.Error("`.part`をcacheとして再利用した")
	}
	if len(h.http.Requests) != 1 {
		t.Errorf("取得回数 = %d, want 1", len(h.http.Requests))
	}
}

// TestDownloadSupportsSHA512 はproviderがsha512を公開するartifactを扱えることを
// 固定する。
//
// 内部digestはalgorithmによらず常にSHA-256である（§7）。
func TestDownloadSupportsSHA512(t *testing.T) {
	payload := []byte("dotnet sdk archive")
	h := newHarness(t)
	h.http.Stub(testURL, fake.HTTPStub{StatusCode: http.StatusOK, Body: payload})

	req := testRequest(t, payload)
	req.ExpectedDigest = upstreamOf(t, payload, domain.AlgoSHA512)

	result, err := h.downloader.Download(context.Background(), req)
	if err != nil {
		t.Fatalf("Download = %v", err.Cause)
	}
	if result.InternalDigest.Algorithm() != domain.AlgoSHA256 {
		t.Errorf("Internal algorithm = %q, want sha256", result.InternalDigest.Algorithm())
	}
}

// TestDownloadReportsNetworkFailure は取得失敗をtyped errorにすることを固定する。
func TestDownloadReportsNetworkFailure(t *testing.T) {
	payload := []byte("never delivered")
	h := newHarness(t)
	// stubを登録しないので未登録URLのerrorになる。

	_, err := h.downloader.Download(context.Background(), testRequest(t, payload))
	if err == nil {
		t.Fatal("取得失敗で成功した")
	}
	if err.Code != domain.CodeNetwork && err.Code != domain.CodeOffline {
		t.Errorf("code = %s, want E_NETWORK か E_OFFLINE", err.Code)
	}
	if !err.Retryable {
		t.Error("network失敗がretryable=falseになっている")
	}
	if _, statErr := h.fs.Stat(cacheFile); statErr == nil {
		t.Error("失敗したのに最終pathへfileができた")
	}
}

// TestDownloadMasksCredentialInError はerror messageへtokenを載せないことを
// 固定する（docs/10-security.md §9.2）。
func TestDownloadMasksCredentialInError(t *testing.T) {
	h := newHarness(t)
	req := testRequest(t, []byte("x"))
	req.URL = "https://example.invalid/a.tar.gz?access_token=SECRETVALUE"

	_, err := h.downloader.Download(context.Background(), req)
	if err == nil {
		t.Fatal("未登録URLで成功した")
	}
	if strings.Contains(err.Cause.Error(), "SECRETVALUE") {
		t.Fatalf("error messageへsecretが出た: %v", err.Cause)
	}
}

// TestDownloadRejectsInvalidRequest は要求の前提を固定する。
func TestDownloadRejectsInvalidRequest(t *testing.T) {
	payload := []byte("x")
	valid := testRequest(t, payload)

	stagingPath, err := domain.NewPathValue(domain.RoleStaging, "/data/staging/x")
	if err != nil {
		t.Fatalf("NewPathValue: %v", err)
	}
	partPath, err := domain.NewPathValue(domain.RoleDownloadCache, cacheFile+PartSuffix)
	if err != nil {
		t.Fatalf("NewPathValue: %v", err)
	}

	cases := []struct {
		name  string
		alter func(*Request)
	}{
		{"URLが空", func(r *Request) { r.URL = "" }},
		// v0.1はchecksumを公開しないartifactを扱わない（§7.2）。
		{"digestが未設定", func(r *Request) { r.ExpectedDigest = domain.Digest{} }},
		{"cache pathが空", func(r *Request) { r.CachePath = domain.PathValue{} }},
		// download成果物はdownload-cache roleへ置く（§17.2）。
		{"roleがstaging", func(r *Request) { r.CachePath = stagingPath }},
		// 完成fileのpathが`.part`で終わると、cache再利用が途中のfileを拾う。
		{"cache pathが.part", func(r *Request) { r.CachePath = partPath }},
		{"sizeが負", func(r *Request) { r.ExpectedSize = -1 }},
		{"MaxRedirectsが負", func(r *Request) { r.MaxRedirects = -1 }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := valid
			c.alter(&req)
			h := newHarness(t)
			if _, err := h.downloader.Download(context.Background(), req); err == nil {
				t.Fatal("不正な要求が通った")
			} else if err.Code != domain.CodeUsage {
				t.Fatalf("code = %s, want %s", err.Code, domain.CodeUsage)
			}
		})
	}
}

// TestNewDownloaderRequiresPorts は必須portの検査を固定する。
func TestNewDownloaderRequiresPorts(t *testing.T) {
	injector := fake.NewInjector()
	if _, err := NewDownloader(nil, fake.NewFileSystem(injector), nil); err == nil {
		t.Error("HTTPClientなしで作れた")
	}
	if _, err := NewDownloader(fake.NewHTTPClient(injector), nil, nil); err == nil {
		t.Error("FileSystemなしで作れた")
	}
	// reporterはnilを許す。progress通知先が無い呼出しでもdownloadは成立する。
	if _, err := NewDownloader(fake.NewHTTPClient(injector), fake.NewFileSystem(injector), nil); err != nil {
		t.Errorf("reporter=nilが拒否された: %v", err)
	}
}

// TestDownloadWithoutReporter はreporterなしでも動くことを固定する。
func TestDownloadWithoutReporter(t *testing.T) {
	payload := []byte("no reporter")
	injector := fake.NewInjector()
	filesystem := fake.NewFileSystem(injector)
	if err := filesystem.MkdirAll(cacheDir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	client := fake.NewHTTPClient(injector)
	client.Stub(testURL, fake.HTTPStub{StatusCode: http.StatusOK, Body: payload})

	downloader, err := NewDownloader(client, filesystem, nil)
	if err != nil {
		t.Fatalf("NewDownloader: %v", err)
	}
	if _, downloadErr := downloader.Download(context.Background(), testRequest(t, payload)); downloadErr != nil {
		t.Fatalf("Download = %v", downloadErr.Cause)
	}
}

// TestDownloadDiscardsPartialOnWriteFailure は書込み失敗で`.part`を残さない
// ことを固定する。
func TestDownloadDiscardsPartialOnWriteFailure(t *testing.T) {
	payload := []byte("write fails")
	h := newHarness(t)
	h.http.Stub(testURL, fake.HTTPStub{StatusCode: http.StatusOK, Body: payload})
	h.fs.Injector().Fail(fake.OpWriteStream, 0, 0, fake.ErrDiskFull)

	_, err := h.downloader.Download(context.Background(), testRequest(t, payload))
	if err == nil {
		t.Fatal("書込み失敗で成功した")
	}
	if !errors.Is(err.Cause, fake.ErrDiskFull) {
		t.Errorf("cause = %v, want ErrDiskFull", err.Cause)
	}
	if _, statErr := h.fs.Stat(cacheFile + PartSuffix); statErr == nil {
		t.Error("書込み失敗なのに`.part`が残っている")
	}
}

// TestNetworkErrorSelectsCode はofflineとnetworkでcodeが変わることを固定する。
func TestNetworkErrorSelectsCode(t *testing.T) {
	req := Request{URL: testURL}
	// offlineの判定はHTTPClient adapterがport.ErrOfflineで正規化する。
	// installはsyscall errnoを見ず、sentinelだけで分ける。
	offline := networkError(req, fmt.Errorf("%w: dial失敗", port.ErrOffline))
	if offline.Code != domain.CodeOffline {
		t.Errorf("offline時のcode = %s, want %s", offline.Code, domain.CodeOffline)
	}
	transient := networkError(req, errors.New("HTTP 503"))
	if transient.Code != domain.CodeNetwork {
		t.Errorf("一時障害のcode = %s, want %s", transient.Code, domain.CodeNetwork)
	}
	// どちらも状態が変われば再実行できる。
	if !offline.Retryable || !transient.Retryable {
		t.Error("network系がretryable=falseになっている")
	}
}
