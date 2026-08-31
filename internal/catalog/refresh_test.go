package catalog

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/definition"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain/port"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain/port/fake"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/progress"
)

// refreshHarness はrefresh 1件分のfakeをまとめる。
type refreshHarness struct {
	fs        *fake.FileSystem
	http      *fake.HTTPClient
	clock     *fake.Clock
	refresher *Refresher
	path      domain.PathValue
}

func newRefreshHarness(t *testing.T) *refreshHarness {
	t.Helper()
	injector := fake.NewInjector()
	filesystem := fake.NewFileSystem(injector)
	filesystem.AddDir(testDataRoot, 0o755)
	client := fake.NewHTTPClient(injector)
	clock := fake.NewClock(baseTime())

	refresher, err := NewRefresher(filesystem, client, clock)
	if err != nil {
		t.Fatalf("NewRefresher: %v", err)
	}
	// goのcatalog pathを使う。上流fixtureがgoのものだからである。
	path, pathErr := CachePath(CachePathRequest{
		DataRoot: mustPathValue(t, domain.RoleDataRoot, testDataRoot),
		Tool:     mustToolID(t, "go"),
		Platform: mustPlatform(t, "windows-amd64"),
		Host:     mustPlatform(t, "linux-amd64-glibc"),
	})
	if pathErr != nil {
		t.Fatalf("CachePath: %v", pathErr)
	}
	return &refreshHarness{
		fs: filesystem, http: client, clock: clock, refresher: refresher, path: path,
	}
}

// request は上流fixtureに合わせたRefreshRequestを返す。
func (h *refreshHarness) request(t *testing.T, intent Intent) RefreshRequest {
	t.Helper()
	return RefreshRequest{
		CachePath:        h.path,
		Tool:             mustToolID(t, "go"),
		Platform:         mustPlatform(t, "windows-amd64"),
		Scheme:           domain.SchemeGo,
		Source:           goAssetSource(),
		Artifact:         goArtifact(),
		ArtifactKind:     definition.KindOfficial,
		DefinitionSHA256: strings.Repeat("a", 64),
		Intent:           intent,
	}
}

// stubUpstream は上流のversion一覧を登録する。
func (h *refreshHarness) stubUpstream(t *testing.T) {
	t.Helper()
	h.http.Stub(goSourceURL, fake.HTTPStub{
		StatusCode: 200,
		Body:       []byte(documentJSON(t)),
	})
}

// documentJSON はgoDocumentと同じ内容のJSON文字列である。
func documentJSON(t *testing.T) string {
	t.Helper()
	return `[
	  {"version":"go1.25.0","stable":true,"files":[
	    {"filename":"go1.25.0.windows-amd64.zip","size":1,"sha256":"` + digest64 + `",
	     "url":"https://go.dev/dl/go1.25.0.windows-amd64.zip","os":"windows","arch":"amd64"}
	  ]},
	  {"version":"go1.24.9","stable":true,"files":[
	    {"filename":"go1.24.9.windows-amd64.zip","size":4,"sha256":"` + digest64 + `",
	     "url":"https://go.dev/dl/go1.24.9.windows-amd64.zip","os":"windows","arch":"amd64"}
	  ]}
	]`
}

// offlineError はoffline判定されるHTTP失敗を作る。
//
// port境界がsyscall事情を[port.ErrOffline]へ正規化しているため、fakeでも
// 同じsentinelで再現できる。
func offlineError() error {
	return fmt.Errorf("%w: dialできない", port.ErrOffline)
}

// TestRefreshFetchesWhenCacheMissing はcacheが無ければ取り直すことを固定する。
func TestRefreshFetchesWhenCacheMissing(t *testing.T) {
	harness := newRefreshHarness(t)
	harness.stubUpstream(t)

	result, err := harness.refresher.Refresh(context.Background(), harness.request(t, IntentList))
	if err != nil {
		t.Fatalf("Refresh = %v", err.Cause)
	}
	if !result.Refreshed || result.Stale {
		t.Errorf("Refreshed/Stale = %v/%v, want true/false", result.Refreshed, result.Stale)
	}
	if len(result.Catalog.Items) != 2 {
		t.Fatalf("items = %d件, want 2", len(result.Catalog.Items))
	}
	// 取得結果はcacheへ保存される。
	if _, statErr := harness.fs.Stat(harness.path.Path()); statErr != nil {
		t.Errorf("cacheが保存されていない: %v", statErr)
	}
}

// TestRefreshUsesFreshCacheWithoutNetwork は期限内cacheで上流へ行かないことを
// 固定する。
func TestRefreshUsesFreshCacheWithoutNetwork(t *testing.T) {
	harness := newRefreshHarness(t)
	harness.stubUpstream(t)
	// 1回目で保存する。
	if _, err := harness.refresher.Refresh(
		context.Background(), harness.request(t, IntentList)); err != nil {
		t.Fatalf("1回目: %v", err.Cause)
	}
	before := harness.http.Injector().Calls(fake.OpHTTPGet)

	result, err := harness.refresher.Refresh(context.Background(), harness.request(t, IntentList))
	if err != nil {
		t.Fatalf("2回目: %v", err.Cause)
	}
	if result.Refreshed {
		t.Error("期限内cacheがあるのに取り直した")
	}
	if after := harness.http.Injector().Calls(fake.OpHTTPGet); after != before {
		t.Errorf("HTTP呼出しが %d → %d と増えた", before, after)
	}
}

// TestRefreshForceRefetchesFreshCache は`--refresh`が鮮度によらず取り直すことを
// 固定する。
//
// docs/03-cli.md §3.2「`--refresh`はcatalog cacheをatomic置換する運用data更新」。
func TestRefreshForceRefetchesFreshCache(t *testing.T) {
	harness := newRefreshHarness(t)
	harness.stubUpstream(t)
	if _, err := harness.refresher.Refresh(
		context.Background(), harness.request(t, IntentList)); err != nil {
		t.Fatalf("1回目: %v", err.Cause)
	}

	req := harness.request(t, IntentList)
	req.Force = true
	result, err := harness.refresher.Refresh(context.Background(), req)
	if err != nil {
		t.Fatalf("--refresh: %v", err.Cause)
	}
	if !result.Refreshed {
		t.Error("--refreshで取り直していない")
	}
}

// TestRefreshFallsBackToStaleCacheForExactOffline は§15のoffline exact退避を
// 固定する。
//
// 「offline exactはidentity/digestが完全なら期限切れを`W_CACHE_STALE`付きで
// 利用できる。**`--latest`は期限切れを黙って使わない**」。
func TestRefreshFallsBackToStaleCacheForExactOffline(t *testing.T) {
	tests := []struct {
		name      string
		intent    Intent
		wantStale bool
	}{
		{"exactは退避できる", IntentExact, true},
		{"latestは退避しない", IntentLatest, false},
		{"listも退避しない", IntentList, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			harness := newRefreshHarness(t)
			harness.stubUpstream(t)
			if _, err := harness.refresher.Refresh(
				context.Background(), harness.request(t, IntentList)); err != nil {
				t.Fatalf("初回取得: %v", err.Cause)
			}
			// 期限を越えさせ、上流をofflineにする。
			harness.clock.Advance(48 * time.Hour)
			harness.http.Stub(goSourceURL, fake.HTTPStub{})
			harness.http.Injector().Fail(fake.OpHTTPGet, 0, 0, offlineError())

			result, err := harness.refresher.Refresh(
				context.Background(), harness.request(t, test.intent))
			if test.wantStale {
				if err != nil {
					t.Fatalf("exact offlineで失敗した: %v", err.Cause)
				}
				if !result.Stale {
					t.Error("Staleが立っていない")
				}
				if len(result.Catalog.Items) == 0 {
					t.Error("退避したcatalogが空")
				}
				return
			}
			if err == nil {
				t.Fatal("期限切れcacheを黙って使った")
			}
			if err.Code != domain.CodeCatalogMissing {
				t.Errorf("code = %s, want %s", err.Code, domain.CodeCatalogMissing)
			}
		})
	}
}

// TestRefreshKeepsExistingCacheOnFailure は取得失敗で既存cacheを消さないことを
// 固定する。
//
// §3.2の「atomic置換」は、失敗時に旧内容がそのまま残ることを意味する。消すと
// §15が認めるoffline exactの退避先も同時に失われる。
func TestRefreshKeepsExistingCacheOnFailure(t *testing.T) {
	harness := newRefreshHarness(t)
	harness.stubUpstream(t)
	if _, err := harness.refresher.Refresh(
		context.Background(), harness.request(t, IntentList)); err != nil {
		t.Fatalf("初回取得: %v", err.Cause)
	}
	before, readErr := harness.fs.ReadFile(harness.path.Path(), CatalogMaxBytes)
	if readErr != nil {
		t.Fatalf("ReadFile: %v", readErr)
	}

	harness.clock.Advance(48 * time.Hour)
	harness.http.Injector().Fail(fake.OpHTTPGet, 0, 0, offlineError())
	if _, err := harness.refresher.Refresh(
		context.Background(), harness.request(t, IntentList)); err == nil {
		t.Fatal("offlineで成功した")
	}

	after, afterErr := harness.fs.ReadFile(harness.path.Path(), CatalogMaxBytes)
	if afterErr != nil {
		t.Fatalf("失敗後にcacheが消えた: %v", afterErr)
	}
	if string(before) != string(after) {
		t.Error("失敗したのにcacheが書き換わった")
	}
}

// TestRefreshDoesNotFallBackOnNonOfflineFailure はoffline以外の失敗で退避しない
// ことを固定する。
//
// source layout違反や5xxは上流かdefinitionの問題であり、古いcacheで隠すと
// 気付けない。
func TestRefreshDoesNotFallBackOnNonOfflineFailure(t *testing.T) {
	harness := newRefreshHarness(t)
	harness.stubUpstream(t)
	if _, err := harness.refresher.Refresh(
		context.Background(), harness.request(t, IntentList)); err != nil {
		t.Fatalf("初回取得: %v", err.Cause)
	}
	harness.clock.Advance(48 * time.Hour)
	// 到達はできるが失敗する（一時障害）。
	harness.http.Stub(goSourceURL, fake.HTTPStub{StatusCode: 503})

	_, err := harness.refresher.Refresh(context.Background(), harness.request(t, IntentExact))
	if err == nil {
		t.Fatal("5xxで期限切れcacheへ退避した")
	}
	if err.Code == domain.CodeCatalogMissing {
		t.Errorf("code = %s、offline扱いになっている", err.Code)
	}
}

// TestCollectItemsUsesStaticSourceWithoutNetwork はstatic sourceがnetworkを
// 使わず、definition記録をidentityにすることを固定する。
//
// §15「source fieldならsource URL/fetch時刻、override/staticなら**definition記録**を
// 使う」。BuildCatalog側の組立ては既存testが押さえているため、ここではkindの
// 振り分けとidentityだけを見る。
func TestCollectItemsUsesStaticSourceWithoutNetwork(t *testing.T) {
	harness := newRefreshHarness(t)
	req := harness.request(t, IntentList)
	req.Scheme = domain.SchemePython
	req.Source = staticSource(t,
		staticEntry(t, "3.13.7", definition.ChannelStable, definition.LifecycleSupported))

	items, identity, err := harness.refresher.collectItems(context.Background(), req)
	if err != nil {
		t.Fatalf("collectItems = %v", err.Cause)
	}
	if len(items) != 1 {
		t.Fatalf("items = %d件, want 1", len(items))
	}
	if harness.http.Injector().Calls(fake.OpHTTPGet) != 0 {
		t.Error("static sourceでHTTPを呼んだ")
	}
	if !strings.HasPrefix(identity, "definition:") {
		t.Errorf("identity = %q, want definition記録", identity)
	}
}

// TestCollectItemsRejectsUnknownKind は未知のsource kindを拒否することを固定する。
func TestCollectItemsRejectsUnknownKind(t *testing.T) {
	harness := newRefreshHarness(t)
	req := harness.request(t, IntentList)
	req.Source.Kind = definition.VersionSourceKind("rss")

	if _, _, err := harness.refresher.collectItems(context.Background(), req); err == nil {
		t.Fatal("未知のsource kindが通った")
	}
}

// TestRefreshRejectsInvalidRequest は要求の前提違反を固定する。
func TestRefreshRejectsInvalidRequest(t *testing.T) {
	harness := newRefreshHarness(t)
	tests := []struct {
		name   string
		mutate func(*RefreshRequest)
	}{
		{"cache pathが空", func(r *RefreshRequest) { r.CachePath = domain.PathValue{} }},
		{"toolが空", func(r *RefreshRequest) { r.Tool = domain.ToolID{} }},
		{"platformが空", func(r *RefreshRequest) { r.Platform = domain.Platform{} }},
		{"schemeが空", func(r *RefreshRequest) { r.Scheme = "" }},
		{"intentが未知", func(r *RefreshRequest) { r.Intent = Intent("guess") }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := harness.request(t, IntentList)
			test.mutate(&req)
			if _, err := harness.refresher.Refresh(context.Background(), req); err == nil {
				t.Fatal("不正な要求が通った")
			}
		})
	}
}

// TestNewRefresherRequiresDependencies は依存不足を拒否することを固定する。
func TestNewRefresherRequiresDependencies(t *testing.T) {
	filesystem := fake.NewFileSystem(nil)
	client := fake.NewHTTPClient(nil)
	clock := fake.NewClock(baseTime())

	if _, err := NewRefresher(nil, client, clock); err == nil {
		t.Error("FileSystemなしで作れた")
	}
	if _, err := NewRefresher(filesystem, nil, clock); err == nil {
		t.Error("HTTPClientなしで作れた")
	}
	if _, err := NewRefresher(filesystem, client, nil); err == nil {
		t.Error("Clockなしで作れた")
	}
}

// TestStaleWarningCarriesCode は退避時のwarningが§16.2のcodeを持つことを固定する。
func TestStaleWarningCarriesCode(t *testing.T) {
	warning := StaleWarning(
		mustToolID(t, "go"), mustPlatform(t, "windows-amd64"), baseTime())
	if warning.Code != progress.WarnCacheStale {
		t.Errorf("code = %s, want %s", warning.Code, progress.WarnCacheStale)
	}
	if warning.MessageID.String() != messageCacheStale {
		t.Errorf("message ID = %q, want %q", warning.MessageID.String(), messageCacheStale)
	}
	if _, ok := warning.Parameters["expires_at"].Str(); !ok {
		t.Error("expires_atが無い")
	}
}

// TestRefreshReportsSaveFailure はcache保存の失敗を返すことを固定する。
func TestRefreshReportsSaveFailure(t *testing.T) {
	harness := newRefreshHarness(t)
	harness.stubUpstream(t)
	harness.fs.Injector().Fail(fake.OpAtomicWrite, 0, 1, fake.ErrDiskFull)

	_, err := harness.refresher.Refresh(context.Background(), harness.request(t, IntentList))
	if err == nil {
		t.Fatal("保存失敗で成功した")
	}
	if !errors.Is(err.Cause, fake.ErrDiskFull) {
		t.Errorf("cause = %v, want ErrDiskFull", err.Cause)
	}
}
