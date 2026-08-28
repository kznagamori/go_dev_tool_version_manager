package catalog

import (
	"errors"
	"testing"
	"time"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain/port/fake"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/store"
)

const (
	testDataRoot = "/data/gdtvm"
	testToolID   = "node"
	testPlatform = "linux-amd64-glibc"
)

// baseTime はcache testの基準時刻である。
func baseTime() time.Time { return time.Date(2026, 8, 7, 9, 0, 0, 0, time.UTC) }

// mustPathValue はrole付きpathを作る。
func mustPathValue(t *testing.T, role domain.PathRole, path string) domain.PathValue {
	t.Helper()
	value, err := domain.NewPathValue(role, path)
	if err != nil {
		t.Fatalf("NewPathValue(%s, %q): %v", role, path, err)
	}
	return value
}

// sampleCatalog は保存・読取りに使うcatalogを作る。
//
// expiresがzeroならstatic source相当（`expires_at=null`）にする。
func sampleCatalog(t *testing.T, expires time.Time, items ...store.CatalogItem) store.Catalog {
	t.Helper()
	return store.Catalog{
		Tool:             mustToolID(t, testToolID),
		Platform:         mustPlatform(t, testPlatform),
		DefinitionSHA256: sampleDigestHex,
		SourceIdentity:   "https://nodejs.org/dist/index.json",
		FetchedAt:        baseTime(),
		ExpiresAt:        expires,
		Items:            items,
	}
}

// sampleDigestHex はdefinition digestに使う64 hexである。
const sampleDigestHex = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

// sampleItem はcatalog itemを1件作る。
func sampleItem(t *testing.T, version string, installable bool) store.CatalogItem {
	t.Helper()
	parsed := mustParse(t, domain.SchemeSemver, version)
	digest, err := domain.ParseUpstreamDigest(
		"sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc")
	if err != nil {
		t.Fatalf("ParseUpstreamDigest: %v", err)
	}
	item := store.CatalogItem{
		Version:             parsed,
		VersionText:         version,
		Channel:             domain.ChannelStable,
		Lifecycle:           domain.LifecycleSupported,
		LifecycleEvidence:   "https://github.com/nodejs/Release",
		LifecycleAssessedAt: baseTime(),
		PublishedAt:         baseTime(),
		Installable:         installable,
		ProviderKind:        store.ProviderOfficial,
		ProviderRelease:     "v" + version,
		ArtifactFile:        "node-v" + version + "-linux-x64.tar.gz",
		ArtifactURL:         "https://nodejs.org/dist/v" + version + "/node.tar.gz",
		ArtifactSize:        1,
		ArtifactDigest:      digest,
		ChecksumSource:      store.ChecksumTextFile,
	}
	if !installable {
		item.UnavailableReason = messageArtifactNotFound
		item.ArtifactFile = ""
		item.ArtifactURL = ""
		item.ArtifactSize = 0
		item.ArtifactDigest = domain.Digest{}
		item.ChecksumSource = ""
	}
	return item
}

// cachePathOf は既定のcache pathを返す。
func cachePathOf(t *testing.T, host string) domain.PathValue {
	t.Helper()
	value, err := CachePath(CachePathRequest{
		DataRoot: mustPathValue(t, domain.RoleDataRoot, testDataRoot),
		Tool:     mustToolID(t, testToolID),
		Platform: mustPlatform(t, testPlatform),
		Host:     mustPlatform(t, host),
	})
	if err != nil {
		t.Fatalf("CachePath: %v", err)
	}
	return value
}

// TestCachePathFollowsSpecLayout は§15の保存pathを固定する。
func TestCachePathFollowsSpecLayout(t *testing.T) {
	tests := []struct {
		host string
		want string
	}{
		{"linux-amd64-glibc", testDataRoot + "/cache/catalogs/node/linux-amd64-glibc.json"},
		{"windows-amd64", testDataRoot + `\cache\catalogs\node\linux-amd64-glibc.json`},
	}
	for _, test := range tests {
		t.Run(test.host, func(t *testing.T) {
			got := cachePathOf(t, test.host)
			if got.Path() != test.want {
				t.Errorf("path = %q, want %q", got.Path(), test.want)
			}
			// §17.2「最も具体的なroleを使う」。data root配下だがcatalogである。
			if got.Role() != domain.RoleCatalog {
				t.Errorf("role = %s, want %s", got.Role(), domain.RoleCatalog)
			}
		})
	}
}

// TestCachePathRejectsMissingInput は入力不足を拒否することを固定する。
func TestCachePathRejectsMissingInput(t *testing.T) {
	full := CachePathRequest{
		DataRoot: mustPathValue(t, domain.RoleDataRoot, testDataRoot),
		Tool:     mustToolID(t, testToolID),
		Platform: mustPlatform(t, testPlatform),
		Host:     mustPlatform(t, testPlatform),
	}
	tests := []struct {
		name   string
		mutate func(*CachePathRequest)
	}{
		{"data rootが空", func(r *CachePathRequest) { r.DataRoot = domain.PathValue{} }},
		{"toolが空", func(r *CachePathRequest) { r.Tool = domain.ToolID{} }},
		{"platformが空", func(r *CachePathRequest) { r.Platform = domain.Platform{} }},
		{"hostが空", func(r *CachePathRequest) { r.Host = domain.Platform{} }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := full
			test.mutate(&req)
			if _, err := CachePath(req); err == nil {
				t.Fatal("不正な入力が通った")
			}
		})
	}
}

// TestSaveAndLoadCacheRoundTrips は保存と読取りが往復することを固定する。
func TestSaveAndLoadCacheRoundTrips(t *testing.T) {
	filesystem := fake.NewFileSystem(nil)
	filesystem.AddDir(testDataRoot, 0o755)
	path := cachePathOf(t, testPlatform)
	original := sampleCatalog(t, baseTime().Add(24*time.Hour), sampleItem(t, "22.18.0", true))

	if err := SaveCache(filesystem, path, original); err != nil {
		t.Fatalf("SaveCache = %v", err)
	}
	// §10-security.md §6「cache metadataは利用者だけが書けるようにする」。
	info, statErr := filesystem.Stat(path.Path())
	if statErr != nil {
		t.Fatalf("Stat: %v", statErr)
	}
	if info.Mode.Perm() != cacheFilePerm {
		t.Errorf("perm = %o, want %o", info.Mode.Perm(), cacheFilePerm)
	}

	entry, err := LoadCache(filesystem, LoadCacheRequest{
		Path:             path,
		Tool:             original.Tool,
		Platform:         original.Platform,
		Scheme:           domain.SchemeSemver,
		DefinitionSHA256: sampleDigestHex,
		Now:              baseTime(),
	})
	if err != nil {
		t.Fatalf("LoadCache = %v", err.Cause)
	}
	if entry.Freshness != FreshnessFresh || !entry.Usable() {
		t.Fatalf("freshness = %s", entry.Freshness)
	}
	if len(entry.Catalog.Items) != 1 || entry.Catalog.Items[0].VersionText != "22.18.0" {
		t.Errorf("items = %+v", entry.Catalog.Items)
	}
}

// TestLoadCacheJudgesFreshness は期限判定を固定する。
//
// docs/04-storage-and-data.md §15「static sourceは`expires_at=null`を許す」。
// 期限を持たないcatalogは上流を見ても内容が変わらないため常にfreshである。
func TestLoadCacheJudgesFreshness(t *testing.T) {
	tests := []struct {
		name    string
		expires time.Time
		now     time.Time
		want    Freshness
	}{
		{"期限内", baseTime().Add(time.Hour), baseTime(), FreshnessFresh},
		{"期限ちょうど", baseTime(), baseTime(), FreshnessStale},
		{"期限切れ", baseTime(), baseTime().Add(time.Hour), FreshnessStale},
		{"期限なし（static）", time.Time{}, baseTime().Add(10000 * time.Hour), FreshnessFresh},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			filesystem := fake.NewFileSystem(nil)
			filesystem.AddDir(testDataRoot, 0o755)
			path := cachePathOf(t, testPlatform)
			value := sampleCatalog(t, test.expires, sampleItem(t, "22.18.0", true))
			if err := SaveCache(filesystem, path, value); err != nil {
				t.Fatalf("SaveCache = %v", err.Cause)
			}

			entry, err := LoadCache(filesystem, LoadCacheRequest{
				Path: path, Tool: value.Tool, Platform: value.Platform,
				Scheme: domain.SchemeSemver, DefinitionSHA256: sampleDigestHex, Now: test.now,
			})
			if err != nil {
				t.Fatalf("LoadCache = %v", err.Cause)
			}
			if entry.Freshness != test.want {
				t.Errorf("freshness = %s, want %s", entry.Freshness, test.want)
			}
		})
	}
}

// TestLoadCacheTreatsMismatchAsMissing は不一致cacheを使わないことを固定する。
//
// docs/04-storage-and-data.md §15「catalogはcacheであり**definition/platform
// 不一致時に利用しない**」。失敗にせずmissingとして返し、再取得へ倒す。
func TestLoadCacheTreatsMismatchAsMissing(t *testing.T) {
	filesystem := fake.NewFileSystem(nil)
	filesystem.AddDir(testDataRoot, 0o755)
	path := cachePathOf(t, testPlatform)
	value := sampleCatalog(t, baseTime().Add(time.Hour), sampleItem(t, "22.18.0", true))
	if err := SaveCache(filesystem, path, value); err != nil {
		t.Fatalf("SaveCache = %v", err.Cause)
	}
	base := LoadCacheRequest{
		Path: path, Tool: value.Tool, Platform: value.Platform,
		Scheme: domain.SchemeSemver, DefinitionSHA256: sampleDigestHex, Now: baseTime(),
	}

	tests := []struct {
		name   string
		mutate func(*LoadCacheRequest)
	}{
		{"toolが違う", func(r *LoadCacheRequest) { r.Tool = mustToolID(t, "go") }},
		{"platformが違う", func(r *LoadCacheRequest) { r.Platform = mustPlatform(t, "windows-amd64") }},
		{"definition digestが違う", func(r *LoadCacheRequest) {
			r.DefinitionSHA256 = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := base
			test.mutate(&req)
			entry, err := LoadCache(filesystem, req)
			if err != nil {
				t.Fatalf("LoadCache = %v", err.Cause)
			}
			if entry.Freshness != FreshnessMissing {
				t.Errorf("freshness = %s, want %s", entry.Freshness, FreshnessMissing)
			}
		})
	}
}

// TestLoadCacheTreatsUnreadableAsMissing は読めないcacheをmissingにすることを固定する。
//
// cacheは再取得できる派生dataである。壊れたcacheでoperationを止めると、利用者は
// 手でfileを消すしかなくなる。
func TestLoadCacheTreatsUnreadableAsMissing(t *testing.T) {
	path := cachePathOf(t, testPlatform)
	tests := []struct {
		name  string
		setup func(*fake.FileSystem)
	}{
		{"file自体が無い", func(*fake.FileSystem) {}},
		{"JSONとして壊れている", func(f *fake.FileSystem) {
			f.AddFile(path.Path(), []byte("{not json"), 0o600)
		}},
		{"schemaが違う", func(f *fake.FileSystem) {
			f.AddFile(path.Path(), []byte(`{"schema":99}`), 0o600)
		}},
		{"読取りが失敗する", func(f *fake.FileSystem) {
			f.AddFile(path.Path(), []byte("{}"), 0o600)
			f.Injector().Fail(fake.OpReadFile, 0, 1, fake.ErrDiskFull)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			filesystem := fake.NewFileSystem(fake.NewInjector())
			filesystem.AddDir(testDataRoot, 0o755)
			test.setup(filesystem)

			entry, err := LoadCache(filesystem, LoadCacheRequest{
				Path: path, Tool: mustToolID(t, testToolID),
				Platform: mustPlatform(t, testPlatform),
				Scheme:   domain.SchemeSemver, Now: baseTime(),
			})
			if err != nil {
				t.Fatalf("LoadCache = %v", err.Cause)
			}
			if entry.Freshness != FreshnessMissing {
				t.Errorf("freshness = %s, want %s", entry.Freshness, FreshnessMissing)
			}
		})
	}
}

// TestLoadCacheSkipsDigestCheckWhenUnspecified はdigest未指定時の扱いを固定する。
func TestLoadCacheSkipsDigestCheckWhenUnspecified(t *testing.T) {
	filesystem := fake.NewFileSystem(nil)
	filesystem.AddDir(testDataRoot, 0o755)
	path := cachePathOf(t, testPlatform)
	value := sampleCatalog(t, baseTime().Add(time.Hour), sampleItem(t, "22.18.0", true))
	if err := SaveCache(filesystem, path, value); err != nil {
		t.Fatalf("SaveCache = %v", err.Cause)
	}

	entry, err := LoadCache(filesystem, LoadCacheRequest{
		Path: path, Tool: value.Tool, Platform: value.Platform,
		Scheme: domain.SchemeSemver, Now: baseTime(),
	})
	if err != nil {
		t.Fatalf("LoadCache = %v", err.Cause)
	}
	if entry.Freshness != FreshnessFresh {
		t.Errorf("freshness = %s, want fresh", entry.Freshness)
	}
}

// TestSaveCacheReportsFailure は書込み失敗をtyped errorにすることを固定する。
func TestSaveCacheReportsFailure(t *testing.T) {
	tests := []struct {
		name string
		op   string
	}{
		{"directory作成", fake.OpMkdirAll},
		{"file書込み", fake.OpAtomicWrite},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			injector := fake.NewInjector()
			filesystem := fake.NewFileSystem(injector)
			filesystem.AddDir(testDataRoot, 0o755)
			injector.Fail(test.op, 0, 1, fake.ErrDiskFull)

			err := SaveCache(filesystem, cachePathOf(t, testPlatform),
				sampleCatalog(t, baseTime().Add(time.Hour), sampleItem(t, "22.18.0", true)))
			if err == nil {
				t.Fatal("失敗注入下で成功した")
			}
			if err.Code != domain.CodeFilesystem {
				t.Errorf("code = %s, want %s", err.Code, domain.CodeFilesystem)
			}
			if !errors.Is(err.Cause, fake.ErrDiskFull) {
				t.Errorf("cause = %v, want ErrDiskFull", err.Cause)
			}
		})
	}
}

// TestCacheRejectsMissingDependencies は依存不足を拒否することを固定する。
func TestCacheRejectsMissingDependencies(t *testing.T) {
	path := cachePathOf(t, testPlatform)
	if _, err := LoadCache(nil, LoadCacheRequest{Path: path}); err == nil {
		t.Error("FileSystemなしでLoadCacheが通った")
	}
	if _, err := LoadCache(fake.NewFileSystem(nil), LoadCacheRequest{}); err == nil {
		t.Error("path未設定でLoadCacheが通った")
	}
	if err := SaveCache(nil, path, store.Catalog{}); err == nil {
		t.Error("FileSystemなしでSaveCacheが通った")
	}
	if err := SaveCache(fake.NewFileSystem(nil), domain.PathValue{}, store.Catalog{}); err == nil {
		t.Error("path未設定でSaveCacheが通った")
	}
}

// TestParentDirHandlesBothSeparators は両OSの区切りを扱うことを固定する。
func TestParentDirHandlesBothSeparators(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/data/gdtvm/cache/x.json", "/data/gdtvm/cache"},
		{`D:\gdtvm\cache\x.json`, `D:\gdtvm\cache`},
		{"/x.json", "/"},
	}
	for _, test := range tests {
		got, err := parentDir(test.path)
		if err != nil {
			t.Fatalf("parentDir(%q): %v", test.path, err)
		}
		if got != test.want {
			t.Errorf("parentDir(%q) = %q, want %q", test.path, got, test.want)
		}
	}
	if _, err := parentDir("x.json"); err == nil {
		t.Error("親directoryを持たないpathが通った")
	}
}
