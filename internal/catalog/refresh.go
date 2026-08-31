package catalog

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/definition"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain/port"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/progress"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/store"
)

// messageCatalogMissing はcacheが無くofflineであることを示すmessage IDである。
const messageCatalogMissing = "catalog.missing_offline"

// Intent はcatalogを何のために引くかである。
//
// 期限切れcacheを使ってよいかが用途で変わる。docs/04-storage-and-data.md §15は
// 「offline exactはidentity/digestが完全なら期限切れを`W_CACHE_STALE`付きで
// 利用できる。**`--latest`は期限切れを黙って使わない**」と定める。
// exactは利用者が版を名指ししており、古い一覧でもその版の情報は変わらない。
// latestは「今の最大」を問う操作なので、古い一覧から答えると別の版を返しうる。
type Intent string

// Intent の値。
const (
	// IntentExact は完全version指定の解決である。期限切れcacheをofflineで使える。
	IntentExact Intent = "exact"
	// IntentLatest は`--latest`の解決である。期限切れcacheを使わない。
	IntentLatest Intent = "latest"
	// IntentList は`available`の一覧である。
	//
	// latestと同じ扱いにする。§3.2が「cacheなしでonlineならrefresh、offlineなら
	// `E_CATALOG_MISSING`」と定め、期限切れを黙って表示してよいとは書いていない。
	// 一覧は「今どの版があるか」を問う操作であり、latestと同じ性質である。
	IntentList Intent = "list"
)

// RefreshRequest はcatalog取得の入力である。
type RefreshRequest struct {
	// CachePath はcatalog cacheのpathである（[CachePath]で組み立てる）。
	CachePath domain.PathValue
	// Tool と Platform はcatalogの対象である。
	Tool     domain.ToolID
	Platform domain.Platform
	// Scheme はtoolのversion schemeである。
	Scheme domain.VersionScheme
	// Source と Artifact はdefinitionの宣言である。
	Source   definition.VersionSource
	Artifact definition.Artifact
	// ArtifactKind はcatalogの`provider_kind`になる。
	ArtifactKind definition.ArtifactKind
	// DefinitionSHA256 は現在のdefinition内容のdigestである。
	DefinitionSHA256 string
	// Intent は用途である。期限切れcacheを使ってよいかを決める。
	Intent Intent
	// Force は`--refresh`である。cacheが期限内でも再取得する。
	//
	// docs/03-cli.md §3.2「`--refresh`はcatalog cacheをatomic置換する運用data
	// 更新であり、Plan/確認を要求しない」。
	Force bool
}

// RefreshResult は取得結果である。
type RefreshResult struct {
	// Catalog は使えるcatalogである。
	Catalog store.Catalog
	// Refreshed は上流から取り直したかを表す。
	Refreshed bool
	// Stale は期限切れcacheを使ったかを表す。
	//
	// trueのとき、呼出し側は§16.2の`W_CACHE_STALE`を結果warningへ載せる。
	Stale bool
	// Warnings は取得中に出たresult warningである（§6.4の
	// `W_LIFECYCLE_OVERRIDE_UNUSED`）。
	Warnings []progress.ResultWarning
}

// Refresher はcatalogのcache利用と再取得を決める。
type Refresher struct {
	fs     port.FileSystem
	client port.HTTPClient
	clock  port.Clock
}

// NewRefresher はRefresherを作る。
func NewRefresher(
	filesystem port.FileSystem, client port.HTTPClient, clock port.Clock,
) (*Refresher, error) {
	switch {
	case filesystem == nil:
		return nil, errors.New("catalog: FileSystemが無い")
	case client == nil:
		return nil, errors.New("catalog: HTTPClientが無い")
	case clock == nil:
		return nil, errors.New("catalog: Clockが無い")
	}
	return &Refresher{fs: filesystem, client: client, clock: clock}, nil
}

// Refresh は用途に応じてcacheを使うか上流から取り直す。
//
// 判断は次の順で行う。
//
//  1. `--refresh`なら鮮度によらず取り直す（docs/03-cli.md §3.2）。
//  2. cacheがfreshならそれを使う。
//  3. それ以外は取り直す。成功すればその結果を使う。
//  4. 取り直しがofflineで失敗した場合だけ、期限切れcacheへ退避できる。
//     退避できるのは[IntentExact]のときだけで、`W_CACHE_STALE`相当の
//     [RefreshResult.Stale]を立てる（docs/04-storage-and-data.md §15）。
//  5. 退避先が無ければ`E_CATALOG_MISSING`（offline）またはfetchのerrorを返す。
//
// **取得に失敗しても既存cacheを消さない。** §3.2の「atomic置換」は、失敗時に
// 旧内容がそのまま残ることを意味する。消してしまうと、§15が認めるoffline exactの
// 退避先も同時に失われる。
func (r *Refresher) Refresh(
	ctx context.Context, req RefreshRequest,
) (RefreshResult, *domain.Error) {
	if err := validateRefreshRequest(req); err != nil {
		return RefreshResult{}, err
	}
	entry, loadErr := LoadCache(r.fs, LoadCacheRequest{
		Path:             req.CachePath,
		Tool:             req.Tool,
		Platform:         req.Platform,
		Scheme:           req.Scheme,
		DefinitionSHA256: req.DefinitionSHA256,
		Now:              r.clock.Now(),
	})
	if loadErr != nil {
		return RefreshResult{}, loadErr
	}
	if !req.Force && entry.Usable() {
		return RefreshResult{Catalog: entry.Catalog}, nil
	}

	built, warnings, fetchErr := r.fetch(ctx, req)
	if fetchErr == nil {
		if saveErr := SaveCache(r.fs, req.CachePath, built); saveErr != nil {
			return RefreshResult{}, saveErr
		}
		return RefreshResult{Catalog: built, Refreshed: true, Warnings: warnings}, nil
	}

	// ここから先はfallbackである。取得に失敗した理由で扱いが変わる。
	if !errors.Is(fetchErr, port.ErrOffline) {
		// offline以外の失敗（source layout違反、5xx、TLS）は退避しない。
		// 上流やdefinitionの問題であり、古いcacheで隠すと気付けない。
		return RefreshResult{}, fetchErr
	}
	if entry.Freshness == FreshnessStale && req.Intent == IntentExact {
		return RefreshResult{Catalog: entry.Catalog, Stale: true}, nil
	}
	return RefreshResult{}, catalogMissing(req, fetchErr)
}

// fetch はsource kindに応じて上流から取り直す。
func (r *Refresher) fetch(
	ctx context.Context, req RefreshRequest,
) (store.Catalog, []progress.ResultWarning, *domain.Error) {
	items, identity, err := r.collectItems(ctx, req)
	if err != nil {
		return store.Catalog{}, nil, err
	}
	return BuildCatalog(ctx, r.client, BuildRequest{
		Tool:             req.Tool,
		Platform:         req.Platform,
		Source:           req.Source,
		Artifact:         req.Artifact,
		ArtifactKind:     req.ArtifactKind,
		DefinitionSHA256: req.DefinitionSHA256,
		SourceIdentity:   identity,
		FetchedAt:        r.clock.Now(),
		Items:            items,
	})
}

// collectItems はsource kindごとにversion itemを集め、source identityを返す。
//
// docs/06-tool-definition.md §6.2「1件でも取得・parseに失敗したらcatalog全体を
// source error」。部分的な結果を返さない。
func (r *Refresher) collectItems(
	ctx context.Context, req RefreshRequest,
) ([]VersionItem, string, *domain.Error) {
	switch req.Source.Kind {
	case definition.SourceStatic:
		// staticはnetworkを使わない。identityはdefinition記録とする（§15）。
		items, err := BuildStaticItems(req.Source)
		if err != nil {
			return nil, "", err
		}
		return items, staticSourceIdentity(req), nil

	case definition.SourceJSON:
		document, err := FetchDocument(ctx, r.client, req.Source.URL)
		if err != nil {
			return nil, "", err
		}
		items, buildErr := BuildItems(ItemsRequest{
			Source: req.Source, Scheme: req.Scheme,
			Document: document.Root, Origin: req.Source.URL,
		})
		if buildErr != nil {
			return nil, "", buildErr
		}
		if limitErr := CheckItemLimit(len(items), req.Source, req.Source.URL); limitErr != nil {
			return nil, "", limitErr
		}
		return items, req.Source.URL, nil

	case definition.SourceJSONIndex:
		return r.collectIndexItems(ctx, req)

	default:
		return nil, "", sourceError("kind", fmt.Errorf("未知のsource kind %q", req.Source.Kind))
	}
}

// collectIndexItems は`json-index`のindexと子文書からitemを集める（§6.2）。
func (r *Refresher) collectIndexItems(
	ctx context.Context, req RefreshRequest,
) ([]VersionItem, string, *domain.Error) {
	index, err := FetchDocument(ctx, r.client, req.Source.URL)
	if err != nil {
		return nil, "", err
	}
	urls, urlErr := ChildDocumentURLs(IndexRequest{
		Source:        req.Source,
		RedirectHosts: req.Artifact.RedirectHosts,
		Document:      index.Root,
		IndexURL:      req.Source.URL,
	})
	if urlErr != nil {
		return nil, "", urlErr
	}
	documents, fetchErr := FetchChildDocuments(ctx, r.client, urls)
	if fetchErr != nil {
		return nil, "", fetchErr
	}

	items := make([]VersionItem, 0, len(documents))
	for index, document := range documents {
		built, buildErr := BuildItems(ItemsRequest{
			Source: req.Source, Scheme: req.Scheme,
			Document: document.Root, Origin: urls[index],
		})
		if buildErr != nil {
			return nil, "", buildErr
		}
		items = append(items, built...)
	}
	// 件数は全子文書の合計で判定する（§6.1「全文書合計のitemsは10,000の
	// 組込み上限以下」）。文書ごとに判定すると合計で上限を超えられる。
	if limitErr := CheckItemLimit(len(items), req.Source, req.Source.URL); limitErr != nil {
		return nil, "", limitErr
	}
	// §6.2「`source_identity`はindex文書のURLとする」。
	return items, req.Source.URL, nil
}

// staticSourceIdentity はstatic sourceのidentityを返す。
//
// §15「source fieldならsource URL/fetch時刻、override/staticなら**definition記録**を
// 使う」。URLを持たないsourceなので、tool/platformで一意に決まる記録を使う。
func staticSourceIdentity(req RefreshRequest) string {
	return "definition:" + req.Tool.String() + "/" + req.Platform.ID()
}

// validateRefreshRequest は取得要求の前提を検査する。
func validateRefreshRequest(req RefreshRequest) *domain.Error {
	switch {
	case req.CachePath.IsZero() || req.CachePath.Path() == "":
		return domain.Internal(errors.New("catalog: cache pathが未設定"))
	case req.Tool.IsZero():
		return domain.Internal(errors.New("catalog: tool IDが未設定"))
	case req.Platform.IsZero():
		return domain.Internal(errors.New("catalog: platformが未設定"))
	case req.Scheme == "":
		return domain.Internal(errors.New("catalog: version schemeが未設定"))
	}
	switch req.Intent {
	case IntentExact, IntentLatest, IntentList:
	default:
		return domain.Internal(fmt.Errorf("catalog: 未知のintent %q", req.Intent))
	}
	return nil
}

// catalogMissing はofflineでcacheも使えないことをtyped errorにする。
//
// docs/03-cli.md §3.2「cacheなしでonlineならrefresh、offlineなら
// `E_CATALOG_MISSING`」。
func catalogMissing(req RefreshRequest, cause error) *domain.Error {
	return &domain.Error{
		Code:      domain.CodeCatalogMissing,
		MessageID: messageID(messageCatalogMissing),
		Parameters: domain.Parameters{
			"tool":     domain.StringScalar(req.Tool.String()),
			"platform": domain.StringScalar(req.Platform.ID()),
		},
		// 接続が戻れば取得できる（docs/02-architecture.md §14）。
		Retryable: true,
		PathRole:  domain.RoleCatalog,
		Cause: fmt.Errorf("catalog: %s / %s のcatalogが無く上流へ到達できない: %w",
			req.Tool.String(), req.Platform.ID(), cause),
	}
}

// staleWarning は期限切れcacheを使ったことを表すresult warningを作る。
//
// docs/04-storage-and-data.md §16.2「`W_CACHE_STALE`: offline exact解決で
// 期限切れだがidentity/digestが完全なcatalogを利用した」。
//
// warningの組立てをここへ置くのは、条件（offline・exact・identity/digest完全）を
// 判定しているのが[Refresher.Refresh]だからである。呼出し側が同じ条件を組み直すと、
// 条件を変えたときに片方だけが古いままになる。
func StaleWarning(tool domain.ToolID, platform domain.Platform, expiresAt time.Time) progress.ResultWarning {
	return progress.ResultWarning{
		Code:      progress.WarnCacheStale,
		MessageID: messageID(messageCacheStale),
		Parameters: domain.Parameters{
			"tool":       domain.StringScalar(tool.String()),
			"platform":   domain.StringScalar(platform.ID()),
			"expires_at": domain.StringScalar(expiresAt.UTC().Format(time.RFC3339)),
		},
	}
}

// messageCacheStale は期限切れcache利用を示すmessage IDである。
const messageCacheStale = "catalog.cache_stale"
