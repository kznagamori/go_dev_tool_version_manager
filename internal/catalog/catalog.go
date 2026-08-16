package catalog

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/definition"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain/port"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/progress"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/store"
)

// messageArtifactNotFound は現在platformでartifactが無いことを示すmessage IDである。
//
// docs/06-tool-definition.md §6.2・§7.1は概念名を`artifact-not-found`と書くが、
// §15の`unavailable_reason`は**message ID**であり、その grammar
// （`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)*$`、docs/04-storage-and-data.md §7）は
// hyphenを許さない。同じ概念をmessage ID表記へ写したものである。
const messageArtifactNotFound = "catalog.artifact_not_found"

// BuildRequest はcatalog組立ての入力である（docs/04-storage-and-data.md §15）。
type BuildRequest struct {
	// Tool と Platform はcatalogの対象である。
	Tool     domain.ToolID
	Platform domain.Platform
	// Source と Artifact はdefinitionの宣言である。
	Source   definition.VersionSource
	Artifact definition.Artifact
	// ArtifactKind はcatalogの`provider_kind`になる。
	ArtifactKind definition.ArtifactKind
	// DefinitionSHA256 は取得時のdefinition内容のdigestである。
	DefinitionSHA256 string
	// SourceIdentity はsourceのHTTPS URL、または`static`のdefinition記録である。
	//
	// `json-index`ではindex文書のURLとする（§6.2）。
	SourceIdentity string
	// FetchedAt は取得時刻である。
	FetchedAt time.Time
	// Items は評価済みのversion itemである。
	Items []VersionItem
}

// BuildCatalog はversion itemからcatalogを組み立てる（§15）。
//
// asset選択、artifact template、checksum解決、`published_at`の突き合わせ、
// `provider_release`の決定、並べ替えまでを行う。`text-file` checksumだけは
// networkを使うため、HTTPClient portを受け取る。
//
// 戻り値の警告は§6.4の`W_LIFECYCLE_OVERRIDE_UNUSED`である。
func BuildCatalog(
	ctx context.Context, client port.HTTPClient, req BuildRequest,
) (store.Catalog, []progress.ResultWarning, *domain.Error) {
	items := make([]store.CatalogItem, 0, len(req.Items))
	// 同じchecksum URLを何度も取得しない。`text-file`のURLは`{{version}}`を含む
	// ためversionごとに違うのが普通だが、共通fileを指すsourceもある。
	texts := make(map[string]string)

	for index := range req.Items {
		item, err := buildCatalogItem(ctx, client, req, &req.Items[index], texts)
		if err != nil {
			return store.Catalog{}, nil, err
		}
		items = append(items, item)
	}
	if err := sortCatalogItems(items); err != nil {
		return store.Catalog{}, nil, sourceError(req.SourceIdentity, err)
	}

	catalog := store.Catalog{
		Tool:             req.Tool,
		Platform:         req.Platform,
		DefinitionSHA256: req.DefinitionSHA256,
		SourceIdentity:   req.SourceIdentity,
		FetchedAt:        req.FetchedAt,
		Items:            items,
	}
	// static sourceは`expires_at=null`を許す（§15）。networkのcatalogは必ず
	// 期限を持つ（P3-01が`cache_ttl`をnetwork sourceの必須keyとした根拠）。
	if req.Source.Kind != definition.SourceStatic {
		catalog.ExpiresAt = req.FetchedAt.Add(req.Source.CacheTTL)
	}
	return catalog, LifecycleOverrideWarnings(req.Source.LifecycleOverrides, req.Items), nil
}

// buildCatalogItem は1件のcatalog itemを組み立てる。
func buildCatalogItem(
	ctx context.Context, client port.HTTPClient,
	req BuildRequest, item *VersionItem, texts map[string]string,
) (store.CatalogItem, *domain.Error) {
	scope := fmt.Sprintf("%s %s", req.SourceIdentity, item.Version)

	assets, metadata, available, err := itemInputs(req.Source, item)
	if err != nil {
		return store.CatalogItem{}, sourceError(scope, err)
	}

	values := templateValues{version: item.Version.String(), metadata: metadata}
	var artifact *resolvedArtifact
	if available {
		artifact, err = resolveArtifact(req.Artifact, values, assets)
		if err != nil {
			return store.CatalogItem{}, sourceError(scope, err)
		}
	}

	evidence, assessedAt := lifecycleRecord(req, item)
	value := store.CatalogItem{
		Version:             item.Version,
		VersionText:         item.Version.String(),
		Channel:             item.Channel,
		Lifecycle:           item.Lifecycle.Lifecycle,
		LifecycleEvidence:   evidence,
		LifecycleAssessedAt: assessedAt,
		ProviderKind:        providerKind(req.ArtifactKind),
	}
	published, parseErr := parseItemTimestamp(item.PublishedAt)
	if parseErr != nil {
		return store.CatalogItem{}, sourceError(scope, parseErr)
	}
	value.PublishedAt = published

	if artifact == nil {
		// §6.2・§7.1の「required tokenが無い」「selectorに0件一致」は
		// source errorではなく現在platformでのunavailableである。
		value.Installable = false
		value.UnavailableReason = messageArtifactNotFound
		value.ProviderRelease = providerRelease(item, nil)
		value.ChecksumSource = checksumSource(req.Artifact.Checksum.Kind)
		return value, nil
	}

	merged, err := mergePublishedAt(item.PublishedAt, artifact.asset)
	if err != nil {
		return store.CatalogItem{}, sourceError(scope, err)
	}
	stamp, mergeErr := parseItemTimestamp(merged)
	if mergeErr != nil {
		return store.CatalogItem{}, sourceError(scope, mergeErr)
	}
	value.PublishedAt = stamp

	digest, digestErr := resolveDigest(ctx, client, req, values, artifact, texts)
	if digestErr != nil {
		return store.CatalogItem{}, digestErr
	}
	value.Installable = true
	value.ProviderRelease = providerRelease(item, artifact.asset)
	value.ArtifactFile = artifact.file
	value.ArtifactURL = artifact.url
	value.ArtifactSize = artifact.size
	value.ArtifactDigest = digest
	value.ChecksumSource = checksumSource(req.Artifact.Checksum.Kind)
	return value, nil
}

// itemInputs はitemのasset、metadata、必要tokenの充足を読む。
func itemInputs(
	source definition.VersionSource, item *VersionItem,
) ([]Asset, map[string]string, bool, error) {
	if item.Static != nil {
		return staticAssets(item.Static), nil, true, nil
	}
	available, err := hasRequiredTokens(source, item.Node)
	if err != nil {
		return nil, nil, false, err
	}
	assets, err := buildAssets(source, item.Node)
	if err != nil {
		return nil, nil, false, err
	}
	metadata, err := buildMetadata(source, item.Node)
	if err != nil {
		return nil, nil, false, err
	}
	return assets, metadata, available, nil
}

// staticAssets は§6.6のstatic assetを共通のasset表現へ移す。
func staticAssets(entry *definition.StaticVersion) []Asset {
	assets := make([]Asset, 0, len(entry.Assets))
	for _, raw := range entry.Assets {
		assets = append(assets, Asset{
			Name: raw.Name, URL: raw.URL, Size: raw.Size,
			Digest: raw.Digest, DigestAlgorithm: raw.DigestAlgorithm,
			OS: string(raw.OS), Arch: string(raw.Arch), Libc: string(raw.Libc),
			PublishedAt: formatTimestamp(raw.PublishedAt),
			ReleaseTag:  raw.ReleaseTag, ReleaseURL: raw.ReleaseURL,
			ReleaseID: raw.ReleaseID, AssetID: raw.AssetID,
		})
	}
	return assets
}

// lifecycleRecord は§15のlifecycle evidenceとassessment時刻を決める。
//
// 「source fieldならsource URL/fetch時刻、override/staticならdefinition記録を
// 使う。**上流がlifecycleを示さず既定の`unknown`になったitemもsource URL/fetch
// 時刻を使う。**」
func lifecycleRecord(req BuildRequest, item *VersionItem) (string, time.Time) {
	switch item.Lifecycle.From {
	case LifecycleFromOverride:
		for _, override := range req.Source.LifecycleOverrides {
			if override.Version.String() == item.Version.String() {
				return override.Evidence, override.AssessedAt
			}
		}
	case LifecycleFromStatic:
		if item.Static != nil {
			return item.Static.LifecycleEvidence, item.Static.LifecycleAssessedAt
		}
	}
	// `source`と既定の`unknown`はどちらも「この公式sourceをこの時刻に取得した」
	// ことを記録する。取得元を持たない根拠不明のitemを作らない。
	return req.SourceIdentity, req.FetchedAt
}

// mergePublishedAt は§6.1の公開日時の優先順位を適用する。
//
// 「item pointer、親pointer、選択assetの`published_at`の順で最初の宣言済み値を
// 使い、**複数の非空値が異なればsource error**にする。」
func mergePublishedAt(itemValue string, asset *Asset) (string, error) {
	assetValue := ""
	if asset != nil {
		assetValue = asset.PublishedAt
	}
	switch {
	case itemValue == "":
		return assetValue, nil
	case assetValue == "" || assetValue == itemValue:
		return itemValue, nil
	default:
		return "", fmt.Errorf(
			"公開日時がitem（%s）と選択asset（%s）で異なる", itemValue, assetValue)
	}
}

// providerRelease は§6.5のprovider releaseを決める。
//
// 「選択assetの`release_tag`が非空ならその値、なければ`version_pointer`が読んだ
// regex適用前のraw version stringを使う。取得時刻、URL pathの推測、tool ID分岐で
// provider releaseを合成しない。」
func providerRelease(item *VersionItem, asset *Asset) string {
	if asset != nil && asset.ReleaseTag != "" {
		return asset.ReleaseTag
	}
	return item.RawVersion
}

// resolveDigest は§7.2の2 kindでdigestを解決する。
func resolveDigest(
	ctx context.Context, client port.HTTPClient, req BuildRequest,
	values templateValues, artifact *resolvedArtifact, texts map[string]string,
) (domain.Digest, *domain.Error) {
	checksum := req.Artifact.Checksum
	scope := fmt.Sprintf("%s %s", req.SourceIdentity, artifact.file)

	if checksum.Kind == definition.ChecksumAssetField {
		digest, err := resolveAssetFieldDigest(checksum, artifact.asset)
		if err != nil {
			return domain.Digest{}, sourceError(scope, err)
		}
		return digest, nil
	}

	values.asset = artifact.asset
	checksumURL, err := renderTemplate(checksum.URL, values, true)
	if err != nil {
		return domain.Digest{}, sourceError(scope, fmt.Errorf("checksum.url: %w", err))
	}
	if urlErr := checkArtifactURL(checksumURL); urlErr != nil {
		return domain.Digest{}, sourceError(scope, urlErr)
	}
	text, cached := texts[checksumURL]
	if !cached {
		fetched, fetchErr := FetchChecksumText(ctx, client, checksumURL)
		if fetchErr != nil {
			return domain.Digest{}, fetchErr
		}
		texts[checksumURL] = fetched
		text = fetched
	}
	// 照合はartifact URLのbasenameで行う。file名templateと配布物の名前が違う
	// sourceがあるため、checksum fileの行はURL側の名前で引く。
	digest, parseErr := ParseChecksumText(text, artifactBasename(artifact.url))
	if parseErr != nil {
		return domain.Digest{}, sourceError(scope, parseErr)
	}
	return digest, nil
}

// sortCatalogItems は§15の「version comparison降順、同値ならversion byte順」へ並べる。
func sortCatalogItems(items []store.CatalogItem) error {
	var failure error
	sort.SliceStable(items, func(left, right int) bool {
		order, err := items[left].Version.Compare(items[right].Version)
		if err != nil {
			if failure == nil {
				failure = err
			}
			return false
		}
		if order != 0 {
			return order > 0
		}
		// comparison keyが同値なら正規version文字列のbyte順で決める。goの
		// `1.20`と`1.20.0`のように、同じkeyで別文字列のversionがありうる。
		return items[left].VersionText < items[right].VersionText
	})
	return failure
}

func providerKind(kind definition.ArtifactKind) store.ProviderKind {
	if kind == definition.KindThirdParty {
		return store.ProviderThirdParty
	}
	return store.ProviderOfficial
}

func checksumSource(kind definition.ChecksumKind) store.ChecksumSource {
	if kind == definition.ChecksumTextFile {
		return store.ChecksumTextFile
	}
	return store.ChecksumAssetField
}

// parseItemTimestamp はUTC RFC 3339文字列をtime.Timeへ戻す。空はzeroである。
func parseItemTimestamp(text string) (time.Time, error) {
	if text == "" {
		return time.Time{}, nil
	}
	stamp, err := time.Parse(time.RFC3339, text)
	if err != nil {
		return time.Time{}, fmt.Errorf("published_atを読めない（%q）", text)
	}
	return stamp.UTC(), nil
}

// formatTimestamp はUTC RFC 3339文字列にする。zeroは空文字である。
func formatTimestamp(stamp time.Time) string {
	if stamp.IsZero() {
		return ""
	}
	return stamp.UTC().Format(time.RFC3339)
}
