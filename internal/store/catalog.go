package store

import (
	"fmt"
	"time"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
)

// CatalogItem はcatalog JSONの1 versionである（docs/04-storage-and-data.md §15）。
type CatalogItem struct {
	// Version はcatalogが正規化した完全versionである。
	//
	// CLI JSON envelope（§17）から読んだitemはschemeを持たないためzeroになる。
	// その場合は[CatalogItem.VersionText]を使う。
	Version domain.Version
	// VersionText はversionの元文字列である。schemeの有無によらず常に入る。
	VersionText string
	// Channel はstableかprereleaseかである。
	Channel domain.Channel
	// Lifecycle はsupported/eol/unknownである。
	Lifecycle domain.Lifecycle
	// LifecycleEvidence は判定根拠の公式/provider HTTPS URLである。
	LifecycleEvidence string
	// LifecycleAssessedAt はlifecycleを判定した時刻である。全状態で必須。
	LifecycleAssessedAt time.Time
	// PublishedAt はproviderが公開した日時である。pointerが無い場合だけzero。
	PublishedAt time.Time
	// Installable は導入可能かである。
	Installable bool
	// UnavailableReason は導入できない理由のmessage IDである。可能なら空。
	UnavailableReason string
	// ProviderKind はofficialかthird-partyかである。
	ProviderKind ProviderKind
	// ProviderRelease は配布元のrelease識別子である。
	ProviderRelease string
	// ArtifactFile はartifactのfile名である。
	ArtifactFile string
	// ArtifactURL はartifactのHTTPS URLである。
	ArtifactURL string
	// ArtifactSize はartifactのbyte数である。0はprovider上でunknown。
	ArtifactSize int64
	// ArtifactDigest はproviderが公開したupstream digestである。
	ArtifactDigest domain.Digest
	// ChecksumSource はdigestの取得元である。
	ChecksumSource ChecksumSource
}

// Catalog は`cache/catalogs/<tool-id>/<platform-id>.json`のtyped表現である（§15）。
//
// catalogはcacheでありdefinition/platform不一致時に利用しない。offline exactは
// identity/digestが完全なら期限切れを`W_CACHE_STALE`付きで利用できる。
type Catalog struct {
	// Tool は対象toolの正規IDである。
	Tool domain.ToolID
	// Platform は対象platformである。
	Platform domain.Platform
	// DefinitionSHA256 は取得時のdefinition内容のdigestである。
	DefinitionSHA256 string
	// SourceIdentity はsourceのHTTPS URLまたはdefinition記録である。
	SourceIdentity string
	// FetchedAt は取得時刻である。
	FetchedAt time.Time
	// ExpiresAt は期限である。static sourceではzeroで、[Catalog.HasExpiry]がfalseになる。
	ExpiresAt time.Time
	// Items はversion comparison降順、同値ならversion byte順のitemである。
	Items []CatalogItem
}

// HasExpiry は期限を持つcatalogかを返す。
//
// §15が「static sourceは`expires_at=null`を許す」と定める。zero timeで表すと
// 「期限が過去」と区別できないため、明示的な判定を用意する。
func (c Catalog) HasExpiry() bool { return !c.ExpiresAt.IsZero() }

// CatalogRequest はcatalog JSONの読込み入力である。
type CatalogRequest struct {
	// Data はcatalog JSONのbytesである。
	Data []byte
	// Scheme はversionのschemeである。
	//
	// §15が「itemsはversion comparison降順、同値ならversion byte順」と定める。
	// comparisonはschemeを要するため、state fileと違いcatalogでは必須にする。
	// catalogは必ずdefinitionと組で使われる（同§「definition/platform不一致時に
	// 利用しない」、`definition_sha256`を持つ）ため、呼出し側は必ずschemeを持つ。
	Scheme domain.VersionScheme
}

// catalogFile は§15のexact key集合である。
type catalogFile struct {
	Schema           *int64             `json:"schema"`
	ToolID           *string            `json:"tool_id"`
	PlatformID       *string            `json:"platform_id"`
	DefinitionSHA256 *string            `json:"definition_sha256"`
	SourceIdentity   *string            `json:"source_identity"`
	FetchedAt        *string            `json:"fetched_at"`
	ExpiresAt        *string            `json:"expires_at"`
	Items            []*catalogItemJSON `json:"items"`
}

type catalogItemJSON struct {
	Version             *string `json:"version"`
	Channel             *string `json:"channel"`
	Lifecycle           *string `json:"lifecycle"`
	LifecycleEvidence   *string `json:"lifecycle_evidence"`
	LifecycleAssessedAt *string `json:"lifecycle_assessed_at"`
	PublishedAt         *string `json:"published_at"`
	Installable         *bool   `json:"installable"`
	UnavailableReason   *string `json:"unavailable_reason"`
	ProviderKind        *string `json:"provider_kind"`
	ProviderRelease     *string `json:"provider_release"`
	ArtifactFile        *string `json:"artifact_file"`
	ArtifactURL         *string `json:"artifact_url"`
	ArtifactSize        *int64  `json:"artifact_size"`
	ArtifactDigest      *string `json:"artifact_digest"`
	ChecksumSource      *string `json:"checksum_source"`
}

// ParseCatalog はcatalog JSONを読む（§15）。
func ParseCatalog(request CatalogRequest) (Catalog, *domain.Error) {
	if _, err := domain.ParseVersionScheme(string(request.Scheme)); err != nil {
		return Catalog{}, catalogError(fmt.Errorf("version scheme: %w", err))
	}
	if err := requireSize("catalog JSON", request.Data, CatalogFileMaxBytes); err != nil {
		return Catalog{}, catalogError(err)
	}
	var file catalogFile
	if err := decodeJSON(request.Data, &file); err != nil {
		return Catalog{}, catalogError(err)
	}
	value, err := buildCatalog(file, request.Scheme)
	if err != nil {
		return Catalog{}, catalogError(err)
	}
	return value, nil
}

// EncodeCatalog はcatalog JSONを書き出す（§15）。
func EncodeCatalog(value Catalog) ([]byte, *domain.Error) {
	file, scheme, err := catalogFileOf(value)
	if err != nil {
		return nil, catalogError(err)
	}
	if _, err := buildCatalog(file, scheme); err != nil {
		return nil, catalogError(err)
	}
	data, encodeErr := encodeJSON(file)
	if encodeErr != nil {
		return nil, catalogError(encodeErr)
	}
	if err := requireSize("catalog JSON", data, CatalogFileMaxBytes); err != nil {
		return nil, catalogError(err)
	}
	return data, nil
}

// catalogError はcatalog cacheの破損を表すtyped errorを作る。
//
// docs/03-cli.md §7の終了code表がcatalogの不在・不正を`E_CATALOG_MISSING`とする。
// catalogはcacheであり、破損しても再取得で回復できるため、state破損と分ける。
func catalogError(cause error) *domain.Error {
	return typedError(domain.CodeCatalogMissing, "catalog.invalid", domain.RoleCatalog, cause)
}

func buildCatalog(file catalogFile, scheme domain.VersionScheme) (Catalog, error) {
	var value Catalog
	if err := requireSchema("schema", file.Schema); err != nil {
		return value, err
	}
	tool, err := requireToolID("tool_id", file.ToolID)
	if err != nil {
		return value, err
	}
	platformText, err := requirePresent("platform_id", file.PlatformID)
	if err != nil {
		return value, err
	}
	platform, err := domain.ParsePlatform(platformText)
	if err != nil {
		return value, fmt.Errorf("platform_id: %w", err)
	}
	definitionDigest, err := requireDigestField("definition_sha256", file.DefinitionSHA256)
	if err != nil {
		return value, err
	}
	sourceIdentity, err := requirePresent("source_identity", file.SourceIdentity)
	if err != nil {
		return value, err
	}
	// §15は「`json-index` sourceの`source_identity`はindex文書のURL」と定め、
	// 「override/staticならdefinition記録を使う」とする。URLの形をしている場合だけ
	// HTTPS制約を課し、definition記録の文字列は非空であることだけを求める。
	if err := requireSourceIdentity("source_identity", sourceIdentity); err != nil {
		return value, err
	}
	fetchedAt, err := requireTimestampField("fetched_at", file.FetchedAt)
	if err != nil {
		return value, err
	}
	expiresAt, err := buildCatalogExpiry(file.ExpiresAt, fetchedAt)
	if err != nil {
		return value, err
	}
	items, err := buildCatalogItems(file.Items, scheme)
	if err != nil {
		return value, err
	}
	return Catalog{
		Tool: tool, Platform: platform, DefinitionSHA256: definitionDigest,
		SourceIdentity: sourceIdentity, FetchedAt: fetchedAt, ExpiresAt: expiresAt,
		Items: items,
	}, nil
}

// buildCatalogExpiry は`expires_at`を読む。nullはstatic sourceの期限なしを表す。
func buildCatalogExpiry(raw *string, fetchedAt time.Time) (time.Time, error) {
	if raw == nil {
		// JSONのnullはpointerがnilになる。§15の「static sourceは
		// `expires_at=null`を許す」に対応する。keyの欠落と区別できないが、
		// どちらも「期限なし」として同じ意味になる。
		return time.Time{}, nil
	}
	expiresAt, err := parseTimestamp("expires_at", *raw)
	if err != nil {
		return time.Time{}, err
	}
	// 取得時刻より前の期限はcatalogとして意味を成さない。作った瞬間に
	// 期限切れのcatalogを書けてしまうと、`--latest`が常に再取得になる。
	if expiresAt.Before(fetchedAt) {
		return time.Time{}, fmt.Errorf("expires_atがfetched_atより前である")
	}
	return expiresAt, nil
}

// requireSourceIdentity はsource identityの形を確かめる（§15）。
func requireSourceIdentity(field, value string) error {
	if value == "" {
		return fmt.Errorf("%sが空", field)
	}
	// scheme付きの値はURLとして扱う。HTTP等の非HTTPSを黙って通さないためである。
	if looksLikeURL(value) {
		if _, err := requireHTTPSURL(field, value); err != nil {
			return err
		}
	}
	return nil
}

func buildCatalogItems(
	items []*catalogItemJSON, scheme domain.VersionScheme,
) ([]CatalogItem, error) {
	entries := make([]CatalogItem, 0, len(items))
	for index, raw := range items {
		if raw == nil {
			return nil, fmt.Errorf("items[%d]が空", index)
		}
		entry, err := buildCatalogItem(index, *raw, scheme)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	if err := requireCatalogItemOrder(entries); err != nil {
		return nil, err
	}
	return entries, nil
}

// requireCatalogItemOrder は§15の「version comparison降順、同値ならversion byte順」
// を確かめる。
//
// state fileと違いschemeを持つため、comparisonまで検査できる。同値のitemが
// 2件あるのは重複であり、どちらを使うかが決まらないため拒否する。
func requireCatalogItemOrder(entries []CatalogItem) error {
	for index := 1; index < len(entries); index++ {
		previous, current := entries[index-1], entries[index]
		order, err := previous.Version.Compare(current.Version)
		if err != nil {
			return fmt.Errorf("items[%d]の比較に失敗した: %w", index, err)
		}
		if order < 0 {
			return fmt.Errorf(
				"itemsがversion降順でない（%q の後に %q）", previous.Version, current.Version)
		}
		if order == 0 {
			// comparisonが同値でもtextが違えば別itemである（例: 前置zero）。
			// textまで同じなら重複である。
			if previous.Version.String() == current.Version.String() {
				return fmt.Errorf("items のversion %q が重複している", current.Version)
			}
			if previous.Version.String() > current.Version.String() {
				return fmt.Errorf(
					"comparison同値のitemsがversion byte順でない（%q の後に %q）",
					previous.Version, current.Version)
			}
		}
	}
	return nil
}

// catalogItemFields はversion以外のitem fieldである。
//
// §15 itemと§17の`CatalogItem`はexact key集合が同じだが、versionの扱いだけが
// 違う。catalogはschemeを持つため[domain.Version]へ、CLI JSONはschemeを持たない
// ためtextのまま扱う。共通部分を1か所にして、片方だけ検査が緩む余地を無くす。
type catalogItemFields struct {
	channel             domain.Channel
	lifecycle           domain.Lifecycle
	lifecycleEvidence   string
	lifecycleAssessedAt time.Time
	publishedAt         time.Time
	installable         bool
	unavailableReason   string
	providerKind        ProviderKind
	providerRelease     string
	artifactFile        string
	artifactURL         string
	artifactSize        int64
	artifactDigest      domain.Digest
	checksumSource      ChecksumSource
}

func buildCatalogItem(
	index int, raw catalogItemJSON, scheme domain.VersionScheme,
) (CatalogItem, error) {
	var value CatalogItem
	prefix := fmt.Sprintf("items[%d]", index)
	versionText, err := requireExactVersionField(prefix+".version", raw.Version)
	if err != nil {
		return value, err
	}
	version, err := domain.ParseVersion(scheme, versionText)
	if err != nil {
		return value, fmt.Errorf("%s.version: %w", prefix, err)
	}
	fields, err := buildCatalogItemFields(prefix, raw)
	if err != nil {
		return value, err
	}
	return catalogItemOf(fields, version, versionText), nil
}

// requireExactVersionField はscheme非依存に完全versionであることを確かめる。
func requireExactVersionField(field string, raw *string) (string, error) {
	text, err := requirePresent(field, raw)
	if err != nil {
		return "", err
	}
	if err := requireExactVersion(field, text); err != nil {
		return "", err
	}
	return text, nil
}

func catalogItemOf(fields catalogItemFields, version domain.Version, text string) CatalogItem {
	return CatalogItem{
		Version: version, VersionText: text,
		Channel: fields.channel, Lifecycle: fields.lifecycle,
		LifecycleEvidence:   fields.lifecycleEvidence,
		LifecycleAssessedAt: fields.lifecycleAssessedAt,
		PublishedAt:         fields.publishedAt, Installable: fields.installable,
		UnavailableReason: fields.unavailableReason,
		ProviderKind:      fields.providerKind, ProviderRelease: fields.providerRelease,
		ArtifactFile: fields.artifactFile, ArtifactURL: fields.artifactURL,
		ArtifactSize: fields.artifactSize, ArtifactDigest: fields.artifactDigest,
		ChecksumSource: fields.checksumSource,
	}
}

func buildCatalogItemFields(prefix string, raw catalogItemJSON) (catalogItemFields, error) {
	var value catalogItemFields
	channelText, err := requirePresent(prefix+".channel", raw.Channel)
	if err != nil {
		return value, err
	}
	channel, err := domain.ParseChannel(channelText)
	if err != nil {
		return value, fmt.Errorf("%s.channel: %w", prefix, err)
	}
	lifecycleText, err := requirePresent(prefix+".lifecycle", raw.Lifecycle)
	if err != nil {
		return value, err
	}
	lifecycle, err := domain.ParseLifecycle(lifecycleText)
	if err != nil {
		return value, fmt.Errorf("%s.lifecycle: %w", prefix, err)
	}
	evidence, err := requirePresent(prefix+".lifecycle_evidence", raw.LifecycleEvidence)
	if err != nil {
		return value, err
	}
	// §15は「lifecycle evidenceは公式/providerのHTTPS URL」と定める。
	if _, err := requireHTTPSURL(prefix+".lifecycle_evidence", evidence); err != nil {
		return value, err
	}
	assessedText, err := requirePresent(prefix+".lifecycle_assessed_at", raw.LifecycleAssessedAt)
	if err != nil {
		return value, err
	}
	// 「assessmentはUTC RFC 3339で全状態に必須」（§15）。lifecycle=unknownでも
	// 「いつ判定してunknownだったか」を残す。
	assessedAt, err := parseTimestamp(prefix+".lifecycle_assessed_at", assessedText)
	if err != nil {
		return value, err
	}
	publishedText, err := requirePresent(prefix+".published_at", raw.PublishedAt)
	if err != nil {
		return value, err
	}
	publishedAt, err := parseOptionalTimestamp(prefix+".published_at", publishedText)
	if err != nil {
		return value, err
	}
	installable, err := requireBool(prefix+".installable", raw.Installable)
	if err != nil {
		return value, err
	}
	reason, err := requirePresent(prefix+".unavailable_reason", raw.UnavailableReason)
	if err != nil {
		return value, err
	}
	if err := checkUnavailableReason(prefix, installable, reason); err != nil {
		return value, err
	}
	providerKind, err := requireEnum(prefix+".provider_kind", raw.ProviderKind, receiptProviderKinds)
	if err != nil {
		return value, err
	}
	providerRelease, err := requireNonEmpty(prefix+".provider_release", raw.ProviderRelease)
	if err != nil {
		return value, err
	}
	artifactFile, err := requireNonEmpty(prefix+".artifact_file", raw.ArtifactFile)
	if err != nil {
		return value, err
	}
	if _, err := requireFileName(prefix+".artifact_file", artifactFile); err != nil {
		return value, err
	}
	artifactURLText, err := requirePresent(prefix+".artifact_url", raw.ArtifactURL)
	if err != nil {
		return value, err
	}
	artifactURL, err := requireHTTPSURL(prefix+".artifact_url", artifactURLText)
	if err != nil {
		return value, err
	}
	size, err := requireInt64(prefix+".artifact_size", raw.ArtifactSize)
	if err != nil {
		return value, err
	}
	digestText, err := requirePresent(prefix+".artifact_digest", raw.ArtifactDigest)
	if err != nil {
		return value, err
	}
	digest, err := parseUpstreamDigest(prefix+".artifact_digest", digestText)
	if err != nil {
		return value, err
	}
	source, err := requireEnum(prefix+".checksum_source", raw.ChecksumSource, checksumSources)
	if err != nil {
		return value, err
	}
	// §15が「`artifact_digest`は§7のupstream digest形式で、Plan前に未解決のitemは
	// `installable=true`にしない」と定める。digestが必須なので未解決item自体を
	// 作れないが、installableとdigestの整合はここで明示的に確かめる。
	if installable && digest.IsZero() {
		return value, fmt.Errorf("%s: installable=trueなのにartifact_digestが未解決", prefix)
	}
	return catalogItemFields{
		channel: channel, lifecycle: lifecycle, lifecycleEvidence: evidence,
		lifecycleAssessedAt: assessedAt, publishedAt: publishedAt,
		installable: installable, unavailableReason: reason,
		providerKind: providerKind, providerRelease: providerRelease,
		artifactFile: artifactFile, artifactURL: artifactURL, artifactSize: size,
		artifactDigest: digest, checksumSource: source,
	}, nil
}

// checkUnavailableReason は§15の「installable=trueなら空、falseならmessage ID」を
// 確かめる。
func checkUnavailableReason(prefix string, installable bool, reason string) error {
	if installable {
		return requireEmpty(prefix+".unavailable_reason", reason)
	}
	if reason == "" {
		return fmt.Errorf("%s: installable=falseなのにunavailable_reasonが空", prefix)
	}
	if _, err := domain.ParseMessageID(reason); err != nil {
		return fmt.Errorf("%s.unavailable_reason: %w", prefix, err)
	}
	return nil
}

func catalogFileOf(value Catalog) (catalogFile, domain.VersionScheme, error) {
	schema := int64(SchemaVersion)
	toolID := value.Tool.String()
	platformID := value.Platform.ID()
	definitionDigest := value.DefinitionSHA256
	sourceIdentity := value.SourceIdentity
	fetchedAt := formatTimestamp(value.FetchedAt)

	var expiresAt *string
	if value.HasExpiry() {
		text := formatTimestamp(value.ExpiresAt)
		expiresAt = &text
	}

	// schemeは全itemで一致していなければならない。1つのcatalogは1 toolの
	// versionだけを持つため、混在は組立て側の誤りである。
	scheme, err := catalogScheme(value.Items)
	if err != nil {
		return catalogFile{}, "", err
	}

	items := make([]*catalogItemJSON, 0, len(value.Items))
	for _, item := range value.Items {
		items = append(items, catalogItemJSONOf(item))
	}
	return catalogFile{
		Schema: &schema, ToolID: &toolID, PlatformID: &platformID,
		DefinitionSHA256: &definitionDigest, SourceIdentity: &sourceIdentity,
		FetchedAt: &fetchedAt, ExpiresAt: expiresAt, Items: items,
	}, scheme, nil
}

func catalogScheme(items []CatalogItem) (domain.VersionScheme, error) {
	if len(items) == 0 {
		// itemが無いcatalogもcacheとして有効である（sourceが空を返した場合）。
		// schemeの検査対象が無いため、既定のsemverで通す。
		return domain.SchemeSemver, nil
	}
	scheme := items[0].Version.Scheme()
	for index, item := range items {
		if item.Version.Scheme() != scheme {
			return "", fmt.Errorf(
				"items[%d]のversion schemeが%qで、items[0]の%qと異なる", index, item.Version.Scheme(), scheme)
		}
	}
	return scheme, nil
}

func catalogItemJSONOf(item CatalogItem) *catalogItemJSON {
	// schemeを持たないitem（CLI JSON由来）はVersionがzeroになる。元文字列を書く。
	version := item.VersionText
	if version == "" {
		version = item.Version.String()
	}
	channel := string(item.Channel)
	lifecycle := string(item.Lifecycle)
	evidence := item.LifecycleEvidence
	assessedAt := formatTimestamp(item.LifecycleAssessedAt)
	publishedAt := ""
	if !item.PublishedAt.IsZero() {
		publishedAt = formatTimestamp(item.PublishedAt)
	}
	installable := item.Installable
	reason := item.UnavailableReason
	providerKind := string(item.ProviderKind)
	providerRelease := item.ProviderRelease
	artifactFile := item.ArtifactFile
	artifactURL := item.ArtifactURL
	size := item.ArtifactSize
	digest := upstreamDigestText(item.ArtifactDigest)
	source := string(item.ChecksumSource)
	return &catalogItemJSON{
		Version: &version, Channel: &channel, Lifecycle: &lifecycle,
		LifecycleEvidence: &evidence, LifecycleAssessedAt: &assessedAt,
		PublishedAt: &publishedAt, Installable: &installable, UnavailableReason: &reason,
		ProviderKind: &providerKind, ProviderRelease: &providerRelease,
		ArtifactFile: &artifactFile, ArtifactURL: &artifactURL, ArtifactSize: &size,
		ArtifactDigest: &digest, ChecksumSource: &source,
	}
}
