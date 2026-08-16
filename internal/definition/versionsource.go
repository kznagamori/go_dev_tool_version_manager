package definition

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
)

// VersionSourceKind はversion発見の方式である（§6.1）。
type VersionSourceKind string

// VersionSourceKind の値。schema 1はこの3件だけを扱う。
const (
	// SourceJSON はHTTPS GETで1文書だけを読む。
	SourceJSON VersionSourceKind = "json"
	// SourceJSONIndex はindex文書から子文書URL群を得て、各子文書を読む。
	SourceJSONIndex VersionSourceKind = "json-index"
	// SourceStatic はnetworkを使わず、definitionへ書いたversionだけを使う。
	SourceStatic VersionSourceKind = "static"
)

// Lifecycle は上流のsupport状態である（§6.1・§6.6）。
type Lifecycle string

// Lifecycle の値。
const (
	// LifecycleSupported は上流がsupportしている状態である。
	LifecycleSupported Lifecycle = "supported"
	// LifecycleEOL は上流がsupportを終了した状態である。
	LifecycleEOL Lifecycle = "eol"
	// LifecycleUnknown は判断できない状態である。
	LifecycleUnknown Lifecycle = "unknown"
)

// Channel はversionの安定度である（§6.1・§6.6）。
type Channel string

// Channel の値。lifecycleとは独立で、全6組合せを表現できる。
const (
	// ChannelStable は正式版である。
	ChannelStable Channel = "stable"
	// ChannelPrerelease はprerelease版である。
	ChannelPrerelease Channel = "prerelease"
)

// CacheTTLMin / CacheTTLMax はcache TTLの範囲である。
//
// 仕様は範囲を定めていない。0以下を許すとrefreshのたびに上流を叩き、極端に長い
// 値はEOL情報の更新を止める。docs/05-configuration.md §3.4のnetwork durationが
// 「Go duration grammarの正値」であることに合わせ、正値であることと現実的な
// 上限だけを課す。標準4 toolはいずれも`24h`である。
const (
	CacheTTLMin = time.Minute
	CacheTTLMax = 30 * 24 * time.Hour
)

// MaxItemsLimit は1 sourceが返せるversion item数の組込み上限である。
//
// docs/04-storage-and-data.md §21の「catalog item 10,000」。`max_items`はこの値を
// 縮小する方向にだけ設定できる（§6.1）。
const MaxItemsLimit = 10000

// MaxDocumentsLimit は`json-index`の子文書数の組込み上限である（同§21）。
const MaxDocumentsLimit = 32

// VersionSource は§6の`[platforms.version_source]`である。
type VersionSource struct {
	// Kind は発見方式である。
	Kind VersionSourceKind

	// URL は取得先である。`static`では空になる。
	//
	// `json-index`ではindex文書のURLであり、catalogの`source_identity`にもなる
	// （§6.2）。
	URL string
	// IndexItemsPointer はindex文書内の配列を指す（`json-index`のみ）。
	IndexItemsPointer string
	// IndexDocumentPointer は各index要素から子文書URLを取り出す（`json-index`のみ）。
	IndexDocumentPointer string
	// MaxDocuments は読む子文書数の上限である（`json-index`のみ）。
	MaxDocuments int
	// DocumentLifecyclePointer は子文書top-levelのlifecycle値を指す（`json-index`のみ）。
	DocumentLifecyclePointer string
	// LifecyclePointer はitem相対のlifecycle値を指す（`json-index`のみ）。
	LifecyclePointer string
	// LifecycleMap はsourceのstring値からLifecycleへの写像である。
	//
	// 2つのlifecycle pointerのどちらかを宣言したら必須になる。写像先が無い
	// pointerは全itemをsource errorにするだけで、lifecycleを決められない。
	LifecycleMap map[string]Lifecycle

	// ItemsPointer はversion itemの配列を指す。
	ItemsPointer string
	// ItemFlattenPointer は各itemを1段だけ展開する（任意）。
	ItemFlattenPointer string
	// ItemParentPublishedAtPointer は展開前の親itemから公開日時を読む（任意）。
	ItemParentPublishedAtPointer string
	// VersionPointer はitem内のraw version文字列を指す。
	VersionPointer string
	// VersionRegex はraw versionから正規versionを取り出す。
	VersionRegex string
	// ChannelPointer はitem内のchannel値を指す（任意）。
	ChannelPointer string
	// PublishedAtPointer はitem内の公開日時を指す（任意）。
	PublishedAtPointer string
	// AssetsPointer はitem内のasset配列を指す（任意）。
	AssetsPointer string
	// AssetFields はasset fieldからJSON pointerへの写像である（任意）。
	AssetFields map[AssetField]string
	// MetadataFields はmetadata keyからJSON pointerへの写像である（任意）。
	MetadataFields map[string]string
	// RequiredTokensPointer はitem内のtoken配列を指す（任意）。
	RequiredTokensPointer string
	// RequiredTokens は必要なtokenである（`required_tokens_pointer`と組）。
	RequiredTokens []string

	// LifecycleOverrides はexact versionへのlifecycle上書きである（§6.4）。
	LifecycleOverrides []LifecycleOverride
	// StaticVersions は`static`のversion集合である（§6.6）。
	StaticVersions []StaticVersion

	// MaxItems はversion item数の上限である。
	MaxItems int
	// CacheTTL はcatalog cacheの有効期間である。`static`では0になる。
	CacheTTL time.Duration
}

// versionSourceTable は§6.1のexact key集合である。
type versionSourceTable struct {
	Kind                         *string            `toml:"kind"`
	URL                          *string            `toml:"url"`
	IndexItemsPointer            *string            `toml:"index_items_pointer"`
	IndexDocumentPointer         *string            `toml:"index_document_pointer"`
	MaxDocuments                 *int64             `toml:"max_documents"`
	DocumentLifecyclePointer     *string            `toml:"document_lifecycle_pointer"`
	LifecycleMap                 *map[string]string `toml:"lifecycle_map"`
	ItemsPointer                 *string            `toml:"items_pointer"`
	ItemFlattenPointer           *string            `toml:"item_flatten_pointer"`
	ItemParentPublishedAtPointer *string            `toml:"item_parent_published_at_pointer"`
	VersionPointer               *string            `toml:"version_pointer"`
	VersionRegex                 *string            `toml:"version_regex"`
	ChannelPointer               *string            `toml:"channel_pointer"`
	LifecyclePointer             *string            `toml:"lifecycle_pointer"`
	LifecycleOverrides           *[]overrideTable   `toml:"lifecycle_overrides"`
	PublishedAtPointer           *string            `toml:"published_at_pointer"`
	AssetsPointer                *string            `toml:"assets_pointer"`
	AssetFields                  *map[string]string `toml:"asset_fields"`
	MetadataFields               *map[string]string `toml:"metadata_fields"`
	RequiredTokensPointer        *string            `toml:"required_tokens_pointer"`
	RequiredTokens               *[]string          `toml:"required_tokens"`
	MaxItems                     *int64             `toml:"max_items"`
	CacheTTL                     *string            `toml:"cache_ttl"`
	StaticVersions               *[]staticTable     `toml:"static_versions"`
}

// buildVersionSource は§6の`version_source`を検証してmodelへ入れる。
//
// 検証順序は§13-5に従う。kindを先に決め、kindごとの許可/禁止keyを見てから、
// 個々のpointer/regex/field契約を検査する。kindが決まらなければ許可keyも
// 決まらないため、以降の検査を行わない。
func buildVersionSource(
	table *versionSourceTable, field string, scheme domain.VersionScheme, diagnostics *Diagnostics,
) VersionSource {
	var value VersionSource
	if table == nil {
		diagnostics.Add(field, reason(reasonMissing), "`[platforms.version_source]`が無い")
		return value
	}
	value.Kind = buildSourceKind(table.Kind, field, diagnostics)
	if value.Kind == "" {
		return value
	}
	if !checkKindKeys(table, value.Kind, field, diagnostics) {
		return value
	}
	value.MaxItems = requireBoundedInt(
		table.MaxItems, field+".max_items", 1, MaxItemsLimit, diagnostics)

	if value.Kind == SourceStatic {
		value.StaticVersions = buildStaticVersions(
			table.StaticVersions, field+".static_versions", scheme, diagnostics)
		return value
	}
	buildNetworkSource(table, field, scheme, &value, diagnostics)
	return value
}

func buildSourceKind(raw *string, field string, diagnostics *Diagnostics) VersionSourceKind {
	if raw == nil {
		diagnostics.Add(field+".kind", reason(reasonMissing), "`kind`が無い")
		return ""
	}
	switch VersionSourceKind(*raw) {
	case SourceJSON, SourceJSONIndex, SourceStatic:
		return VersionSourceKind(*raw)
	default:
		diagnostics.Add(field+".kind", reason(reasonEnum),
			fmt.Sprintf("kindは%s|%s|%sだけ（%q）", SourceJSON, SourceJSONIndex, SourceStatic, *raw))
		return ""
	}
}

// buildNetworkSource は`json`と`json-index`に共通の取得契約を検査する。
func buildNetworkSource(
	table *versionSourceTable, field string,
	scheme domain.VersionScheme, value *VersionSource, diagnostics *Diagnostics,
) {
	value.URL = requireHTTPSURL(table.URL, field+".url", urlEndpoint, diagnostics)
	value.ItemsPointer = requirePointer(table.ItemsPointer, field+".items_pointer", diagnostics)
	value.VersionPointer = requirePointer(table.VersionPointer, field+".version_pointer", diagnostics)
	value.VersionRegex = buildVersionRegex(table.VersionRegex, field+".version_regex", diagnostics)
	value.CacheTTL = requireCacheTTL(table.CacheTTL, field+".cache_ttl", diagnostics)

	value.ChannelPointer = optionalPointer(table.ChannelPointer, field+".channel_pointer", diagnostics)
	value.PublishedAtPointer = optionalPointer(
		table.PublishedAtPointer, field+".published_at_pointer", diagnostics)
	value.AssetsPointer = optionalPointer(table.AssetsPointer, field+".assets_pointer", diagnostics)
	value.ItemFlattenPointer = optionalPointer(
		table.ItemFlattenPointer, field+".item_flatten_pointer", diagnostics)
	value.ItemParentPublishedAtPointer = optionalPointer(
		table.ItemParentPublishedAtPointer, field+".item_parent_published_at_pointer", diagnostics)

	buildFlattenContract(table, field, value, diagnostics)
	buildRequiredTokens(table, field, value, diagnostics)
	value.AssetFields = buildAssetFields(table.AssetFields, field+".asset_fields", diagnostics)
	value.MetadataFields = buildMetadataFields(table.MetadataFields, field+".metadata_fields", diagnostics)
	value.LifecycleOverrides = buildLifecycleOverrides(
		table.LifecycleOverrides, field+".lifecycle_overrides", scheme, diagnostics)

	if value.Kind == SourceJSONIndex {
		buildIndexContract(table, field, value, diagnostics)
	}
}

// buildFlattenContract は§6.1の`item_flatten_pointer`の契約を検査する。
//
// 「`item_parent_published_at_pointer`は`item_flatten_pointer`と組でだけ使用でき、
// 子itemの`published_at_pointer`との同時指定は禁止する」。
func buildFlattenContract(
	table *versionSourceTable, field string, value *VersionSource, diagnostics *Diagnostics,
) {
	if table.ItemParentPublishedAtPointer == nil {
		return
	}
	if table.ItemFlattenPointer == nil {
		diagnostics.Add(field+".item_parent_published_at_pointer", reason(reasonConditional),
			"`item_parent_published_at_pointer`は`item_flatten_pointer`と組でだけ使える")
	}
	if table.PublishedAtPointer != nil {
		// 親と子の両方から公開日時を読むと、どちらを採るかがdefinitionから
		// 決まらない。§6.1は複数の非空値が異なればsource errorとするが、
		// 同時宣言そのものを禁じている。
		diagnostics.Add(field+".item_parent_published_at_pointer", reason(reasonConditional),
			"`item_parent_published_at_pointer`と`published_at_pointer`は同時に指定できない")
	}
	_ = value
}

// buildRequiredTokens は§6.2の`required_tokens*`を検査する。
//
// 「`required_tokens_pointer`と`required_tokens`は組で指定する。pointer先は一意
// string array、requiredはASCII stringの一意非空配列」。
func buildRequiredTokens(
	table *versionSourceTable, field string, value *VersionSource, diagnostics *Diagnostics,
) {
	pointerSet := table.RequiredTokensPointer != nil
	tokensSet := table.RequiredTokens != nil
	switch {
	case !pointerSet && !tokensSet:
		return
	case pointerSet != tokensSet:
		diagnostics.Add(field+".required_tokens", reason(reasonConditional),
			"`required_tokens_pointer`と`required_tokens`は組で指定する")
		return
	}
	value.RequiredTokensPointer = requirePointer(
		table.RequiredTokensPointer, field+".required_tokens_pointer", diagnostics)

	tokens := *table.RequiredTokens
	scope := field + ".required_tokens"
	if len(tokens) == 0 {
		diagnostics.Add(scope, reason(reasonConditional), "`required_tokens`が空配列")
		return
	}
	if len(tokens) > ArrayMax {
		diagnostics.Add(scope, reason(reasonLimit),
			fmt.Sprintf("required_tokensが%d件を超える（%d件）", ArrayMax, len(tokens)))
		return
	}
	for index, token := range tokens {
		if err := checkASCIIToken(token); err != nil {
			diagnostics.Add(fmt.Sprintf("%s[%d]", scope, index), reason(reasonText), err.Error())
			return
		}
	}
	if err := requireUniqueIdentifiers("required_tokens", tokens); err != nil {
		diagnostics.Add(scope, reason(reasonDuplicate), err.Error())
		return
	}
	value.RequiredTokens = append([]string{}, tokens...)
}

// buildIndexContract は§6.2の`json-index`固有契約を検査する。
func buildIndexContract(
	table *versionSourceTable, field string, value *VersionSource, diagnostics *Diagnostics,
) {
	value.IndexItemsPointer = requirePointer(
		table.IndexItemsPointer, field+".index_items_pointer", diagnostics)
	value.IndexDocumentPointer = requirePointer(
		table.IndexDocumentPointer, field+".index_document_pointer", diagnostics)
	value.MaxDocuments = requireBoundedInt(
		table.MaxDocuments, field+".max_documents", 1, MaxDocumentsLimit, diagnostics)

	value.DocumentLifecyclePointer = optionalPointer(
		table.DocumentLifecyclePointer, field+".document_lifecycle_pointer", diagnostics)
	value.LifecyclePointer = optionalPointer(
		table.LifecyclePointer, field+".lifecycle_pointer", diagnostics)

	documentSet := table.DocumentLifecyclePointer != nil
	itemSet := table.LifecyclePointer != nil
	if documentSet && itemSet {
		// 子文書top-levelとitem相対の両方からlifecycleを読むと、どちらを採るかが
		// definitionから決まらない（§6.2）。
		diagnostics.Add(field+".lifecycle_pointer", reason(reasonConditional),
			"`document_lifecycle_pointer`と`lifecycle_pointer`は同時に指定できない")
	}
	value.LifecycleMap = buildLifecycleMap(
		table.LifecycleMap, field+".lifecycle_map", documentSet || itemSet, diagnostics)
}

// checkKindKeys は§6.1のkindごとの禁止keyを検査する。
//
// 禁止keyは表で持つ。条件分岐で書くと、kindが増減したときにどのkeyが漏れたかを
// 読み取れない。1件でも違反があればfalseを返し、以降の検査を行わない。kindの
// 前提が崩れた状態で個別契約を見ても、診断が増えるだけで原因が分かりにくくなる。
func checkKindKeys(
	table *versionSourceTable, kind VersionSourceKind, field string, diagnostics *Diagnostics,
) bool {
	present := presentSourceKeys(table)
	forbidden := forbiddenSourceKeys(kind)
	ok := true
	for _, key := range sourceKeyOrder {
		if _, set := present[key]; !set {
			continue
		}
		if _, banned := forbidden[key]; banned {
			diagnostics.Add(field+"."+key, reason(reasonKindKey),
				fmt.Sprintf("kind=%sでは`%s`を書けない", kind, key))
			ok = false
		}
	}
	return ok
}

// sourceKeyOrder は§6.1の許可key 24件を仕様の並び順で持つ。
//
// 診断の順序を宣言順に固定し、mapの反復順で揺れないようにする。
var sourceKeyOrder = []string{
	"kind", "url", "index_items_pointer", "index_document_pointer", "max_documents",
	"document_lifecycle_pointer", "lifecycle_map", "items_pointer", "item_flatten_pointer",
	"item_parent_published_at_pointer", "version_pointer", "version_regex", "channel_pointer",
	"lifecycle_pointer", "lifecycle_overrides", "published_at_pointer", "assets_pointer",
	"asset_fields", "metadata_fields", "required_tokens_pointer", "required_tokens",
	"max_items", "cache_ttl", "static_versions",
}

// forbiddenSourceKeys はkindごとの禁止key集合を返す（§6.1の表）。
func forbiddenSourceKeys(kind VersionSourceKind) map[string]struct{} {
	switch kind {
	case SourceJSON:
		// 利用者判断により`lifecycle_pointer`も禁止する。`lifecycle_map`が無ければ
		// 写像先が1件も無く、宣言しても全itemがsource errorになるだけである。
		return keySet("index_items_pointer", "index_document_pointer", "max_documents",
			"document_lifecycle_pointer", "lifecycle_pointer", "lifecycle_map", "static_versions")
	case SourceJSONIndex:
		return keySet("static_versions")
	case SourceStatic:
		// 「`static_versions` arrayと`max_items`だけを使用し、他pointer/url/index/
		// cache fieldを禁止する」（§6.1）。`version_regex`はstatic versionが正規
		// 完全versionを直接書くため適用対象が無い。§6.4はoverrideも明示的に禁じる。
		return keySet("url", "index_items_pointer", "index_document_pointer", "max_documents",
			"document_lifecycle_pointer", "lifecycle_pointer", "lifecycle_map", "items_pointer",
			"item_flatten_pointer", "item_parent_published_at_pointer", "version_pointer",
			"version_regex", "channel_pointer", "lifecycle_overrides", "published_at_pointer",
			"assets_pointer", "asset_fields", "metadata_fields", "required_tokens_pointer",
			"required_tokens", "cache_ttl")
	default:
		return nil
	}
}

func keySet(keys ...string) map[string]struct{} {
	set := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		set[key] = struct{}{}
	}
	return set
}

// presentSourceKeys は宣言されているkeyの集合を返す。
func presentSourceKeys(table *versionSourceTable) map[string]struct{} {
	present := make(map[string]struct{}, len(sourceKeyOrder))
	mark := func(key string, set bool) {
		if set {
			present[key] = struct{}{}
		}
	}
	mark("kind", table.Kind != nil)
	mark("url", table.URL != nil)
	mark("index_items_pointer", table.IndexItemsPointer != nil)
	mark("index_document_pointer", table.IndexDocumentPointer != nil)
	mark("max_documents", table.MaxDocuments != nil)
	mark("document_lifecycle_pointer", table.DocumentLifecyclePointer != nil)
	mark("lifecycle_map", table.LifecycleMap != nil)
	mark("items_pointer", table.ItemsPointer != nil)
	mark("item_flatten_pointer", table.ItemFlattenPointer != nil)
	mark("item_parent_published_at_pointer", table.ItemParentPublishedAtPointer != nil)
	mark("version_pointer", table.VersionPointer != nil)
	mark("version_regex", table.VersionRegex != nil)
	mark("channel_pointer", table.ChannelPointer != nil)
	mark("lifecycle_pointer", table.LifecyclePointer != nil)
	mark("lifecycle_overrides", table.LifecycleOverrides != nil)
	mark("published_at_pointer", table.PublishedAtPointer != nil)
	mark("assets_pointer", table.AssetsPointer != nil)
	mark("asset_fields", table.AssetFields != nil)
	mark("metadata_fields", table.MetadataFields != nil)
	mark("required_tokens_pointer", table.RequiredTokensPointer != nil)
	mark("required_tokens", table.RequiredTokens != nil)
	mark("max_items", table.MaxItems != nil)
	mark("cache_ttl", table.CacheTTL != nil)
	mark("static_versions", table.StaticVersions != nil)
	return present
}

// buildVersionRegex は§6.3の`version_regex`を検査する。
//
// 「RE2互換でnamed capture`version`をexactly 1件持つ」。Goの`regexp`はRE2
// なので、compileが通ればRE2互換である。captureを数えるのは、0件だとversionを
// 取り出せず、2件以上だとどちらを使うかがdefinitionから決まらないためである。
func buildVersionRegex(raw *string, field string, diagnostics *Diagnostics) string {
	if raw == nil {
		diagnostics.Add(field, reason(reasonMissing), fmt.Sprintf("`%s`が無い", field))
		return ""
	}
	if err := checkNamedCaptureRegex(*raw, "version", field); err != nil {
		diagnostics.Add(field, reason(reasonRegex), err.Error())
		return ""
	}
	return *raw
}

// checkNamedCaptureRegex は指定名のnamed captureをexactly 1件持つRE2を検査する。
func checkNamedCaptureRegex(pattern, name, field string) error {
	if pattern == "" {
		return fmt.Errorf("%sが空", field)
	}
	if len(pattern) > RegexMaxBytes {
		return fmt.Errorf("%sが%d byteを超える（%d byte）", field, RegexMaxBytes, len(pattern))
	}
	compiled, err := regexp.Compile(pattern)
	if err != nil {
		return fmt.Errorf("%sがRE2として不正（%v）", field, err)
	}
	count := 0
	for _, capture := range compiled.SubexpNames() {
		if capture == name {
			count++
		}
	}
	if count != 1 {
		return fmt.Errorf("%sはnamed capture `%s` をちょうど1件持たなければならない（%d件）",
			field, name, count)
	}
	return nil
}

// RegexMaxBytes はdefinitionへ書く正規表現1件の上限である。
//
// 仕様は個別の上限を定めていない。標準4 toolのregexはいずれも200 byte未満で
// あり、上限を設けないとcompileの費用をdefinition側から膨らませられる。
const RegexMaxBytes = 1024

// requireCacheTTL は§6.1の`cache_ttl`を検査する。
//
// docs/04-storage-and-data.md §15が「static sourceは`expires_at=null`を許す」と
// 定める。裏を返せばnetwork sourceのcatalogは必ず期限を持つため、`cache_ttl`は
// `json`と`json-index`で必須である。
func requireCacheTTL(raw *string, field string, diagnostics *Diagnostics) time.Duration {
	if raw == nil {
		diagnostics.Add(field, reason(reasonMissing), fmt.Sprintf("`%s`が無い", field))
		return 0
	}
	value, err := time.ParseDuration(*raw)
	if err != nil {
		diagnostics.Add(field, reason(reasonDuration),
			fmt.Sprintf("%sがGo duration文字列として解釈できない（%q）", field, *raw))
		return 0
	}
	if value < CacheTTLMin || value > CacheTTLMax {
		diagnostics.Add(field, reason(reasonDuration),
			fmt.Sprintf("%sが%v〜%vの範囲外（%v）", field, CacheTTLMin, CacheTTLMax, value))
		return 0
	}
	return value
}

// requireBoundedInt は必須のintegerを範囲込みで検査する。
func requireBoundedInt(raw *int64, field string, min, max int, diagnostics *Diagnostics) int {
	if raw == nil {
		diagnostics.Add(field, reason(reasonMissing), fmt.Sprintf("`%s`が無い", field))
		return 0
	}
	value := *raw
	if value < int64(min) || value > int64(max) {
		diagnostics.Add(field, reason(reasonLimit),
			fmt.Sprintf("%sが%d〜%dの範囲外（%d）", field, min, max, value))
		return 0
	}
	return int(value)
}

// checkASCIIToken はASCII非空stringを検査する（§6.2）。
func checkASCIIToken(token string) error {
	if token == "" {
		return fmt.Errorf("tokenが空")
	}
	if len(token) > NameMaxBytes {
		return fmt.Errorf("token %q が%d byteを超える", token, NameMaxBytes)
	}
	if strings.TrimSpace(token) != token {
		return fmt.Errorf("token %q の前後に空白がある", token)
	}
	for _, char := range token {
		if char > 0x7F || char < 0x21 {
			return fmt.Errorf("token %q にASCII可視文字以外が含まれる", token)
		}
	}
	return nil
}
