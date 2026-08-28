package catalog

import (
	"errors"
	"fmt"
	"time"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain/port"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/security"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/store"
)

// catalog cacheのpath構成（docs/04-storage-and-data.md §15「保存pathは
// `cache/catalogs/<tool-id>/<platform-id>.json`」）。
const (
	cacheDirName    = "cache"
	catalogsDirName = "catalogs"
	catalogExt      = ".json"
)

// CatalogMaxBytes はcatalog JSON 1 fileの上限である（同§21「catalog JSON各file 64 MiB」）。
const CatalogMaxBytes int64 = 64 << 20

// cacheFilePerm はcatalog cache fileのpermissionである。
//
// docs/10-security.md §6「state、receipt、cache metadata、setup backupは利用者
// だけが書けるようにする」。
const cacheFilePerm = 0o600

// cacheDirPerm はcatalog cache directoryのpermissionである。
const cacheDirPerm = 0o700

// Freshness はcacheの鮮度である。
type Freshness string

// Freshness の値。
const (
	// FreshnessFresh は期限内で、そのまま使える。
	FreshnessFresh Freshness = "fresh"
	// FreshnessStale は期限切れである。offline exactだけが`W_CACHE_STALE`付きで使える。
	FreshnessStale Freshness = "stale"
	// FreshnessMissing はcacheが無い、読めない、または現在のdefinition/platformと
	// 一致しない。いずれも再取得が要る。
	FreshnessMissing Freshness = "missing"
)

// CachePathRequest はcatalog cache pathの組み立て入力である。
type CachePathRequest struct {
	// DataRoot はactive data rootである。
	DataRoot domain.PathValue
	// Tool と Platform はcatalogの対象である。Platformはfile名になる。
	Tool     domain.ToolID
	Platform domain.Platform
	// Host はpath区切りを決めるplatformである。
	//
	// Platformと別に受けるのは、Linux上のCIがWindows platformのcatalog pathを
	// 組み立てるtestを書けるようにするためである。
	Host domain.Platform
}

// CachePath はcatalog cacheのpathを組み立てる（docs/04-storage-and-data.md §15）。
//
// roleは`catalog`とする。§17.2が同roleを「tool/platform catalog JSON」と定める。
func CachePath(req CachePathRequest) (domain.PathValue, error) {
	if req.DataRoot.IsZero() || req.DataRoot.Path() == "" {
		return domain.PathValue{}, errors.New("catalog: data rootが未設定")
	}
	if req.Tool.IsZero() {
		return domain.PathValue{}, errors.New("catalog: tool IDが未設定")
	}
	if req.Platform.IsZero() {
		return domain.PathValue{}, errors.New("catalog: platformが未設定")
	}
	if req.Host.IsZero() {
		return domain.PathValue{}, errors.New("catalog: host platformが未設定")
	}
	joined, err := security.Join(security.JoinRequest{
		Root: req.DataRoot,
		Components: []string{
			cacheDirName, catalogsDirName, req.Tool.String(), req.Platform.ID() + catalogExt,
		},
		Host: req.Host,
	})
	if err != nil {
		return domain.PathValue{}, fmt.Errorf("catalog: cache pathを組み立てられない: %w", err)
	}
	// Joinはrootのroleを引き継ぐ。catalog fileはdata root配下だが、§17.2が
	// 「最も具体的なroleを使う」と定めるためcatalogへ付け替える。
	return domain.NewPathValue(domain.RoleCatalog, joined.Path())
}

// CacheEntry は読み込んだcatalog cacheとその鮮度である。
type CacheEntry struct {
	// Catalog は読めた場合の内容である。Freshnessがmissingならzero値である。
	Catalog store.Catalog
	// Freshness は鮮度である。
	Freshness Freshness
}

// Usable はそのまま使える鮮度かを返す。
func (e CacheEntry) Usable() bool { return e.Freshness == FreshnessFresh }

// LoadCacheRequest はcache読取りの入力である。
type LoadCacheRequest struct {
	// Path はcatalog cache fileのpathである。
	Path domain.PathValue
	// Tool と Platform は期待する対象である。一致しないcacheは使わない。
	Tool     domain.ToolID
	Platform domain.Platform
	// Scheme はtoolのversion schemeである。catalog itemのversion解釈に使う。
	Scheme domain.VersionScheme
	// DefinitionSHA256 は現在のdefinition内容のdigestである。
	//
	// docs/04-storage-and-data.md §15「catalogはcacheであり**definition/platform
	// 不一致時に利用しない**」。definitionが変わればartifact templateもpointerも
	// 変わりうるため、同じversion一覧でも中身が古い。
	DefinitionSHA256 string
	// Now は期限判定の基準時刻である。
	Now time.Time
}

// LoadCache はcatalog cacheを読み、鮮度を判定する。
//
// **読めない・一致しないは失敗にせずmissingとして返す。** cacheは再取得できる
// 派生dataであり、壊れたcacheでoperationを止めると利用者は手で消すしかなくなる。
// 一方でnetworkの失敗はcacheでは埋められないため、そちらは呼出し側が扱う。
func LoadCache(filesystem port.FileSystem, req LoadCacheRequest) (CacheEntry, *domain.Error) {
	if filesystem == nil {
		return CacheEntry{}, domain.Internal(errors.New("catalog: FileSystemが未注入"))
	}
	if req.Path.IsZero() || req.Path.Path() == "" {
		return CacheEntry{}, domain.Internal(errors.New("catalog: cache pathが未設定"))
	}
	data, readErr := filesystem.ReadFile(req.Path.Path(), CatalogMaxBytes)
	if readErr != nil {
		return CacheEntry{Freshness: FreshnessMissing}, nil
	}
	parsed, parseErr := store.ParseCatalog(store.CatalogRequest{
		Data:   data,
		Scheme: req.Scheme,
	})
	if parseErr != nil {
		return CacheEntry{Freshness: FreshnessMissing}, nil
	}
	if !matchesRequest(parsed, req) {
		return CacheEntry{Freshness: FreshnessMissing}, nil
	}
	return CacheEntry{Catalog: parsed, Freshness: freshnessOf(parsed, req.Now)}, nil
}

// matchesRequest はcacheが現在の対象と一致するかを返す（§15）。
func matchesRequest(parsed store.Catalog, req LoadCacheRequest) bool {
	if parsed.Tool != req.Tool || parsed.Platform != req.Platform {
		return false
	}
	// digestを指定しない呼出しは一致検査を省く。registry読込み前の診断など、
	// definitionを持たない経路のためである。
	if req.DefinitionSHA256 == "" {
		return true
	}
	return parsed.DefinitionSHA256 == req.DefinitionSHA256
}

// freshnessOf は期限から鮮度を決める。
//
// static sourceは`expires_at=null`を許す（§15）。期限を持たないcatalogは
// 上流を見に行っても内容が変わらないため、常にfreshとして扱う。
func freshnessOf(parsed store.Catalog, now time.Time) Freshness {
	if !parsed.HasExpiry() {
		return FreshnessFresh
	}
	if now.Before(parsed.ExpiresAt) {
		return FreshnessFresh
	}
	return FreshnessStale
}

// SaveCache はcatalogをcacheへatomicに書く。
//
// docs/03-cli.md §3.2「`--refresh`はcatalog cacheを**atomic置換**する運用data
// 更新であり、Plan/確認を要求しない」。
func SaveCache(
	filesystem port.FileSystem, path domain.PathValue, value store.Catalog,
) *domain.Error {
	if filesystem == nil {
		return domain.Internal(errors.New("catalog: FileSystemが未注入"))
	}
	if path.IsZero() || path.Path() == "" {
		return domain.Internal(errors.New("catalog: cache pathが未設定"))
	}
	data, encodeErr := store.EncodeCatalog(value)
	if encodeErr != nil {
		return encodeErr
	}
	parent, dirErr := parentDir(path.Path())
	if dirErr != nil {
		return cacheWriteError(dirErr)
	}
	if err := filesystem.MkdirAll(parent, cacheDirPerm); err != nil {
		return cacheWriteError(fmt.Errorf("cache directoryを作れない: %w", err))
	}
	if err := filesystem.AtomicWrite(path.Path(), data, cacheFilePerm); err != nil {
		return cacheWriteError(fmt.Errorf("cacheを書けない: %w", err))
	}
	return nil
}

// parentDir はpathの親directoryを返す。
//
// `path/filepath`を使わないのは、対象がhost OSのpathとは限らないためである。
// 区切りはWindowsの`\`とLinuxの`/`の両方を見る。
func parentDir(path string) (string, error) {
	for index := len(path) - 1; index >= 0; index-- {
		if path[index] == '/' || path[index] == '\\' {
			if index == 0 {
				return path[:1], nil
			}
			return path[:index], nil
		}
	}
	return "", fmt.Errorf("catalog: cache path %q に親directoryが無い", path)
}

// cacheWriteError はcache書込み失敗をtyped errorにする。
func cacheWriteError(cause error) *domain.Error {
	return &domain.Error{
		Code: domain.CodeFilesystem,
		// disk fullやpermissionは解消しうる（docs/02-architecture.md §14）。
		Retryable: true,
		PathRole:  domain.RoleCatalog,
		Cause:     fmt.Errorf("catalog: %w", cause),
	}
}
