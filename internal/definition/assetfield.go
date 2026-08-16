package definition

import (
	"fmt"
	"sort"
)

// AssetField は§6.5のasset field名である。
type AssetField string

// AssetField のexactly 13値。schema 1はこの集合だけを扱う。
const (
	AssetName            AssetField = "name"
	AssetURL             AssetField = "url"
	AssetSize            AssetField = "size"
	AssetDigest          AssetField = "digest"
	AssetDigestAlgorithm AssetField = "digest_algorithm"
	AssetOS              AssetField = "os"
	AssetArch            AssetField = "arch"
	AssetLibc            AssetField = "libc"
	AssetPublishedAt     AssetField = "published_at"
	AssetReleaseTag      AssetField = "release_tag"
	AssetReleaseURL      AssetField = "release_url"
	AssetReleaseID       AssetField = "release_id"
	AssetID              AssetField = "asset_id"
)

// AssetFieldCount は§6.5が定めるasset field数である。
//
// 件数を定数で持つのは、値を足したり消したりしたときにtestが気付けるように
// するためである。
const AssetFieldCount = 13

// assetFieldOrder は§6.5の並び順でasset fieldを持つ。
//
// 診断とstatic assetの必須検査を宣言順に固定し、mapの反復順で揺れないようにする。
var assetFieldOrder = []AssetField{
	AssetName, AssetURL, AssetSize, AssetDigest, AssetDigestAlgorithm,
	AssetOS, AssetArch, AssetLibc, AssetPublishedAt,
	AssetReleaseTag, AssetReleaseURL, AssetReleaseID, AssetID,
}

// parseAssetField は文字列をAssetFieldへ変換する。
func parseAssetField(text string) (AssetField, error) {
	for _, field := range assetFieldOrder {
		if AssetField(text) == field {
			return field, nil
		}
	}
	return "", fmt.Errorf("asset field %q は§6.5の%d値に含まれない", text, AssetFieldCount)
}

// buildAssetFields は§6.5の`asset_fields`を検証する。
//
// keyは§6.5のexact集合、値はJSON pointerである。宣言していないfieldは
// artifact templateの`{{asset.<field>}}`から参照できない（§7.1）。
func buildAssetFields(
	raw *map[string]string, field string, diagnostics *Diagnostics,
) map[AssetField]string {
	if raw == nil {
		return nil
	}
	source := *raw
	if len(source) == 0 {
		diagnostics.Add(field, reason(reasonConditional), "`asset_fields`が空table")
		return nil
	}
	value := make(map[AssetField]string, len(source))
	for _, key := range sortedKeys(source) {
		assetField, err := parseAssetField(key)
		if err != nil {
			diagnostics.Add(field+"."+key, reason(reasonEnum), err.Error())
			return nil
		}
		if err := validatePointer(source[key], field+"."+key); err != nil {
			diagnostics.Add(field+"."+key, reason(reasonPointer), err.Error())
			return nil
		}
		value[assetField] = source[key]
	}
	return value
}

// buildMetadataFields は§6.5の`metadata_fields`を検証する。
//
// keyは§3のmetadata key grammar、値はJSON pointerである。宣言したkeyだけが
// URL/file templateの`{{metadata.<key>}}`から参照できる（§7.1・§12）。
func buildMetadataFields(
	raw *map[string]string, field string, diagnostics *Diagnostics,
) map[string]string {
	if raw == nil {
		return nil
	}
	source := *raw
	if len(source) == 0 {
		diagnostics.Add(field, reason(reasonConditional), "`metadata_fields`が空table")
		return nil
	}
	if len(source) > ArrayMax {
		diagnostics.Add(field, reason(reasonLimit),
			fmt.Sprintf("metadata_fieldsが%d件を超える（%d件）", ArrayMax, len(source)))
		return nil
	}
	value := make(map[string]string, len(source))
	for _, key := range sortedKeys(source) {
		if err := ValidateMetadataKey(key); err != nil {
			diagnostics.Add(field+"."+key, reason(reasonIdentifier), err.Error())
			return nil
		}
		if err := validatePointer(source[key], field+"."+key); err != nil {
			diagnostics.Add(field+"."+key, reason(reasonPointer), err.Error())
			return nil
		}
		value[key] = source[key]
	}
	return value
}

// sortedKeys はmapのkeyをbyte順で返す。
//
// mapの反復順は不定である。診断の順序を宣言内容だけで決めるために使う。
func sortedKeys(source map[string]string) []string {
	keys := make([]string, 0, len(source))
	for key := range source {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// AssetFieldOrder は§6.5のasset field exactly 13値を仕様の並び順で返す。
//
// catalog側がasset fieldを宣言順に解決するために使う。内部sliceを共有しない
// のは、呼出し側の変更でdiagnosticsとstatic assetの必須検査の順序が変わらない
// ようにするためである（docs/02-architecture.md §5）。
func AssetFieldOrder() []AssetField {
	return append([]AssetField(nil), assetFieldOrder...)
}
