package catalog

import (
	"fmt"
	"regexp"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/definition"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
)

// VersionItem は上流文書から読んだversion item 1件である。
//
// asset選択、checksum解決、catalog itemの組立てはこの後段で行うため、item本体の
// JSON nodeを[VersionItem.Node]へ残す。
type VersionItem struct {
	// Version は`version_regex`が取り出した正規完全versionである。
	Version domain.Version
	// RawVersion はregex適用前のraw version文字列である。
	//
	// §6.5のcatalog/Plan/receiptの`provider_release`が、選択assetの
	// `release_tag`が空のときにこの値を使う。
	RawVersion string
	// Channel は§6.1で決めたchannelである。
	Channel domain.Channel
	// PublishedAt はUTC RFC 3339の公開日時である。宣言が無ければ空文字とする。
	//
	// 選択assetの`published_at`との突き合わせは後段で行う（§6.1の
	// 「item pointer、親pointer、選択assetの順で最初の宣言済み値」）。
	PublishedAt string
	// Node はitem本体のJSON nodeである。asset/tokenの抽出に使う。
	Node any
}

// ItemsRequest は1文書からversion itemを読む要求である。
type ItemsRequest struct {
	// Source はdefinitionのversion sourceである。
	Source definition.VersionSource
	// Scheme はtoolのversion schemeである。
	Scheme domain.VersionScheme
	// Document は復号済みの文書rootである。
	Document any
	// Origin は診断へ出す文書の識別子である。通常は取得元URLを渡す。
	Origin string
}

// rawItem は展開後のversion item候補である。
type rawItem struct {
	node any
	// parentPublishedAt は`item_parent_published_at_pointer`が親から読んだ値である。
	parentPublishedAt string
}

// BuildItems は1文書からversion itemを読む（docs/06-tool-definition.md §6.1・§6.3）。
//
// `items_pointer`の解決、`item_flatten_pointer`の1段展開、親公開日時の継承、
// `version_regex`の適用、channelの決定、公開日時の正規化までを行う。
//
// **失敗時に部分結果を返さない。** §6.1は欠落と型違いをsource errorと定め、
// §6.2は「1件でも取得・parseに失敗したらcatalog全体をsource error」とする。
// 読めたitemだけでcatalogを作ると、上流のlayout変更が「versionが減っただけ」に
// 見えて気付けない。
//
// itemの件数上限は適用しない。`json-index`は全子文書の合計で判定するため
// （§6.1「全文書合計のitemsは10,000の組込み上限以下」）、呼出し側が連結後に
// [CheckItemLimit]を呼ぶ。ただし1文書だけで組込み上限を超える入力は、per-item
// 処理へ進む前にここで止める。
func BuildItems(req ItemsRequest) ([]VersionItem, *domain.Error) {
	if req.Scheme == "" {
		return nil, domain.Internal(fmt.Errorf("catalog: version schemeが未指定"))
	}
	raws, err := collectRawItems(req)
	if err != nil {
		return nil, sourceError(req.Origin, err)
	}
	// 組込み上限は1文書でも超えられる。per-item処理の前に止めて、上流の暴走した
	// 応答へ比例した計算量を持たないようにする。
	if len(raws) > definition.MaxItemsLimit {
		return nil, sourceError(req.Origin, fmt.Errorf(
			"1文書のversion itemが組込み上限%d件を超える（%d件）",
			definition.MaxItemsLimit, len(raws)))
	}

	versionRe, compileErr := regexp.Compile(req.Source.VersionRegex)
	if compileErr != nil {
		// definitionのschema検証を通った正規表現だけが渡る（§6.3）。
		return nil, domain.Internal(fmt.Errorf("catalog: version_regexをcompileできない: %w", compileErr))
	}

	items := make([]VersionItem, 0, len(raws))
	for index, raw := range raws {
		item, itemErr := buildItem(req, versionRe, raw)
		if itemErr != nil {
			return nil, sourceError(fmt.Sprintf("%s items[%d]", req.Origin, index), itemErr)
		}
		items = append(items, item)
	}
	return items, nil
}

// collectRawItems は`items_pointer`と`item_flatten_pointer`でitem候補を集める。
func collectRawItems(req ItemsRequest) ([]rawItem, error) {
	parents, err := pointerArray(req.Document, req.Source.ItemsPointer)
	if err != nil {
		return nil, fmt.Errorf("items_pointer: %w", err)
	}
	flatten := req.Source.ItemFlattenPointer
	if !flatten.Declared() {
		raws := make([]rawItem, len(parents))
		for index, node := range parents {
			raws[index] = rawItem{node: node}
		}
		return raws, nil
	}

	parentStamp := req.Source.ItemParentPublishedAtPointer
	var raws []rawItem
	for index, parent := range parents {
		// pointer先が配列でない、または存在しない親はsource errorである（§6.1）。
		// 該当の親だけ読み飛ばすと、上流が構造を変えたときにversionが静かに消える。
		children, childErr := pointerArray(parent, flatten.Value())
		if childErr != nil {
			return nil, fmt.Errorf("item_flatten_pointer items[%d]: %w", index, childErr)
		}
		stamp := ""
		if parentStamp.Declared() {
			text, stampErr := pointerString(parent, parentStamp.Value())
			if stampErr != nil {
				return nil, fmt.Errorf("item_parent_published_at_pointer items[%d]: %w", index, stampErr)
			}
			normalized, normErr := normalizeTimestamp(text, "item_parent_published_at_pointer")
			if normErr != nil {
				return nil, fmt.Errorf("items[%d]: %w", index, normErr)
			}
			stamp = normalized
		}
		// 展開は1段までである。子の中の配列を再帰的に展開しない（§6.1）。
		for _, child := range children {
			raws = append(raws, rawItem{node: child, parentPublishedAt: stamp})
		}
	}
	return raws, nil
}

// buildItem は1件のversion itemを組み立てる。
func buildItem(req ItemsRequest, versionRe *regexp.Regexp, raw rawItem) (VersionItem, error) {
	source := req.Source
	rawVersion, err := pointerString(raw.node, source.VersionPointer)
	if err != nil {
		return VersionItem{}, fmt.Errorf("version_pointer: %w", err)
	}
	canonical, err := applyVersionRegex(versionRe, rawVersion)
	if err != nil {
		return VersionItem{}, err
	}
	version, parseErr := domain.ParseVersion(req.Scheme, canonical)
	if parseErr != nil {
		return VersionItem{}, parseErr
	}

	channel, err := resolveChannel(source, raw.node, version)
	if err != nil {
		return VersionItem{}, err
	}
	publishedAt, err := resolvePublishedAt(source, raw)
	if err != nil {
		return VersionItem{}, err
	}
	return VersionItem{
		Version:     version,
		RawVersion:  rawVersion,
		Channel:     channel,
		PublishedAt: publishedAt,
		Node:        raw.node,
	}, nil
}

// applyVersionRegex は`version_regex`のnamed capture`version`を取り出す（§6.3）。
//
// matchしないitemはsource layout違反であり、refresh全体を失敗させる。skipすると
// leading prefixの変更のような上流の表記変更を検出できない。
func applyVersionRegex(versionRe *regexp.Regexp, rawVersion string) (string, error) {
	match := versionRe.FindStringSubmatch(rawVersion)
	if match == nil {
		return "", fmt.Errorf("version %q が`version_regex`に一致しない", rawVersion)
	}
	for index, name := range versionRe.SubexpNames() {
		if name == "version" {
			return match[index], nil
		}
	}
	// definitionのschema検証がnamed capture`version`をexactly 1件要求している。
	return "", fmt.Errorf("`version_regex`にnamed capture`version`が無い")
}

// resolveChannel は§6.1のchannelを決める。
//
// `channel_pointer`を宣言していれば厳密に写像し、省略していれば正規versionの
// prerelease構文から導出する。**省略と空文字宣言を同一視しない**
// （[definition.OptionalPointer]）。
func resolveChannel(
	source definition.VersionSource, node any, version domain.Version,
) (domain.Channel, error) {
	if !source.ChannelPointer.Declared() {
		return DeriveChannel(version)
	}
	value, err := resolvePointer(node, source.ChannelPointer.Value())
	if err != nil {
		return "", fmt.Errorf("channel_pointer: %w", err)
	}
	scalar, err := toScalar(value)
	if err != nil {
		return "", fmt.Errorf("channel_pointer: %w", err)
	}
	return MapChannel(scalar)
}

// resolvePublishedAt は§6.1の公開日時を決める。
//
// item pointerと親pointerは同時に宣言できない（definitionのschema検証）。どちらも
// 宣言しないsourceは空文字とし、**取得時刻を公開日時として代用しない**。
func resolvePublishedAt(source definition.VersionSource, raw rawItem) (string, error) {
	if !source.PublishedAtPointer.Declared() {
		return raw.parentPublishedAt, nil
	}
	text, err := pointerString(raw.node, source.PublishedAtPointer.Value())
	if err != nil {
		return "", fmt.Errorf("published_at_pointer: %w", err)
	}
	return normalizeTimestamp(text, "published_at_pointer")
}

// CheckItemLimit はversion item数が§6.1の上限に収まることを確かめる。
//
// 組込み上限は10,000で、`max_items`はそれを縮小する方向にだけ働く
// （docs/04-storage-and-data.md §21、§6.1）。**超過を切り捨てない。** 黙って
// 打ち切ると、上限に達した以降のversionが存在しないことと区別できなくなる。
func CheckItemLimit(count int, source definition.VersionSource, origin string) *domain.Error {
	limit := definition.MaxItemsLimit
	if source.MaxItems > 0 && source.MaxItems < limit {
		limit = source.MaxItems
	}
	if count > limit {
		return sourceError(origin, fmt.Errorf("version itemが上限%d件を超える（%d件）", limit, count))
	}
	return nil
}
