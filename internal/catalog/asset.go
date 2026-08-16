package catalog

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/definition"
)

// Asset はversion itemのasset 1件である（docs/06-tool-definition.md §6.5）。
//
// 宣言していないfieldは空のままにする。asset listを持たないsourceでは
// artifact templateを使うため、[Asset]自体が作られない。
type Asset struct {
	Name string
	URL  string
	// Size は非負のbyte数である。§6.5で唯一integerを許すfieldである。
	Size            int64
	Digest          string
	DigestAlgorithm definition.DigestAlgorithm
	OS              string
	Arch            string
	Libc            string
	PublishedAt     string
	ReleaseTag      string
	ReleaseURL      string
	ReleaseID       string
	AssetID         string
}

// buildAssets は`assets_pointer`と`asset_fields`でasset listを読む（§6.5）。
//
// `assets_pointer`を宣言しないsourceはnilを返す。「asset listがないsourceでは
// artifact templateを使う」（同§）ため、空listと未宣言を区別する必要はない。
func buildAssets(source definition.VersionSource, node any) ([]Asset, error) {
	if !source.AssetsPointer.Declared() {
		return nil, nil
	}
	nodes, err := pointerArray(node, source.AssetsPointer.Value())
	if err != nil {
		return nil, fmt.Errorf("assets_pointer: %w", err)
	}
	assets := make([]Asset, 0, len(nodes))
	for index, assetNode := range nodes {
		asset, assetErr := buildAsset(source.AssetFields, assetNode)
		if assetErr != nil {
			return nil, fmt.Errorf("assets[%d]: %w", index, assetErr)
		}
		assets = append(assets, asset)
	}
	return assets, nil
}

// buildAsset は宣言済みasset fieldを1件のassetへ読み込む。
func buildAsset(fields map[definition.AssetField]string, node any) (Asset, error) {
	var asset Asset
	for _, field := range definition.AssetFieldOrder() {
		pointer, declared := fields[field]
		if !declared {
			continue
		}
		if err := assignAssetField(&asset, field, node, pointer); err != nil {
			return Asset{}, fmt.Errorf("%s: %w", field, err)
		}
	}
	return asset, nil
}

// assignAssetField は1つのasset fieldを解決して代入する。
//
// §6.5が「値はstring、sizeだけ非負integer。IDもprecision lossを避けるため
// decimal stringとして扱う」と定める。数値をstringへ、stringを数値へ暗黙変換
// しない。上流が型を変えたらsource errorにしてlive smokeで気付く。
func assignAssetField(
	asset *Asset, field definition.AssetField, node any, pointer string,
) error {
	if field == definition.AssetSize {
		size, err := assetSize(node, pointer)
		if err != nil {
			return err
		}
		asset.Size = size
		return nil
	}
	text, err := pointerString(node, pointer)
	if err != nil {
		return err
	}
	switch field {
	case definition.AssetName:
		asset.Name = text
	case definition.AssetURL:
		asset.URL = text
	case definition.AssetDigest:
		asset.Digest = text
	case definition.AssetDigestAlgorithm:
		algorithm, algoErr := definition.ParseDigestAlgorithm(text)
		if algoErr != nil {
			return algoErr
		}
		asset.DigestAlgorithm = algorithm
	case definition.AssetOS:
		asset.OS = text
	case definition.AssetArch:
		asset.Arch = text
	case definition.AssetLibc:
		asset.Libc = text
	case definition.AssetPublishedAt:
		normalized, timeErr := normalizeTimestamp(text, "asset.published_at")
		if timeErr != nil {
			return timeErr
		}
		asset.PublishedAt = normalized
	case definition.AssetReleaseTag:
		asset.ReleaseTag = text
	case definition.AssetReleaseURL:
		asset.ReleaseURL = text
	case definition.AssetReleaseID:
		asset.ReleaseID = text
	case definition.AssetID:
		asset.AssetID = text
	}
	return nil
}

// assetSize はasset sizeを非負integerとして読む（§6.5）。
func assetSize(node any, pointer string) (int64, error) {
	value, err := resolvePointer(node, pointer)
	if err != nil {
		return 0, err
	}
	number, ok := value.(json.Number)
	if !ok {
		return 0, fmt.Errorf("sizeがintegerでない（%s）", jsonKind(value))
	}
	size, convErr := number.Int64()
	if convErr != nil {
		return 0, fmt.Errorf("sizeが64 bit整数でない（%s）", number.String())
	}
	if size < 0 {
		return 0, fmt.Errorf("sizeが負（%d）", size)
	}
	return size, nil
}

// buildMetadata は`metadata_fields`を読む（§6.5）。
//
// 値はURL/file templateの`{{metadata.<key>}}`だけに使う。catalog itemや表示へ
// 任意metadataを持ち込む契約はschema 1に無い。
func buildMetadata(source definition.VersionSource, node any) (map[string]string, error) {
	if len(source.MetadataFields) == 0 {
		return nil, nil
	}
	keys := make([]string, 0, len(source.MetadataFields))
	for key := range source.MetadataFields {
		keys = append(keys, key)
	}
	// mapの反復順は不定である。診断の順序を宣言内容だけで決めるためkeyでsortする。
	sort.Strings(keys)

	values := make(map[string]string, len(source.MetadataFields))
	for _, key := range keys {
		text, err := pointerString(node, source.MetadataFields[key])
		if err != nil {
			return nil, fmt.Errorf("metadata_fields.%s: %w", key, err)
		}
		values[key] = text
	}
	return values, nil
}

// hasRequiredTokens はversion itemが必要なtokenをすべて持つかを返す（§6.2）。
//
// 「required tokenが1件でもないversion itemは**source errorではなく**現在
// platformで`installable=false/artifact-not-found`」。Node.js index等、URL
// templateは作れるがplatform archiveの公開有無を別fieldで示すsourceに使う。
//
// pointer自体が解決できない場合はsource errorである。tokenが無いことと、
// definitionが参照するfieldが無いことは別の失敗である。
func hasRequiredTokens(source definition.VersionSource, node any) (bool, error) {
	if !source.RequiredTokensPointer.Declared() {
		return true, nil
	}
	nodes, err := pointerArray(node, source.RequiredTokensPointer.Value())
	if err != nil {
		return false, fmt.Errorf("required_tokens_pointer: %w", err)
	}
	present := make(map[string]struct{}, len(nodes))
	for index, value := range nodes {
		text, ok := value.(string)
		if !ok {
			return false, fmt.Errorf(
				"required_tokens_pointer[%d]がstringでない（%s）", index, jsonKind(value))
		}
		if _, duplicate := present[text]; duplicate {
			// §6.2は「pointer先は一意string array」と定める。重複は上流の
			// layout違反であり、黙って畳むと一意性の前提が崩れる。
			return false, fmt.Errorf("required_tokens_pointerのtoken %q が重複している", text)
		}
		present[text] = struct{}{}
	}
	for _, token := range source.RequiredTokens {
		if _, found := present[token]; !found {
			return false, nil
		}
	}
	return true, nil
}
