package definition

import (
	"fmt"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
)

// Definition は1 tool definition fileのtyped modelである。
//
// docs/06-tool-definition.md §2のtop-level key（`schema`, `schema_id`, `tool`,
// `platforms`）だけを持つ。
type Definition struct {
	// Path はregistry rootからのrelative pathである（`tools/node.toml`）。
	Path string
	// Tool は§4の`[tool]`である。
	Tool Tool
	// Platforms は§5の`[[platforms]]`である。宣言順を保つ。
	Platforms []Platform
}

// Tool は§4の`[tool]`である。7 key全件必須。
type Tool struct {
	// ID はfile basenameと一致する正規tool IDである。
	ID domain.ToolID
	// Name は表示名である。
	Name string
	// Aliases はtool IDの別名である。空可。
	Aliases []string
	// Description は説明文である。
	Description string
	// Homepage はtoolのHTTPS URLである。
	Homepage string
	// License はupstream toolのSPDX expressionである。
	License string
	// VersionScheme はversionのgrammarと比較規則である。
	VersionScheme domain.VersionScheme
}

// ArtifactKind は配布物の提供主体である（§5）。
type ArtifactKind string

// ArtifactKind の値。
const (
	// KindOfficial はtool本体のprojectが公開する配布物である。
	KindOfficial ArtifactKind = "official"
	// KindThirdParty は第三者build配布物である。Planで取得元と採用理由を常に示す。
	KindThirdParty ArtifactKind = "third-party"
)

// Platform は§5の`[[platforms]]` 1件である。
//
// §6〜§11のtableは本structが宣言だけを持ち、内容の検証はP3-01の2本目・3本目で
// 実装する。ここで型を与えずrawのまま保持しているのは、未実装の検証を「通った」
// ように見せないためである（docs/13-progress.md P3-01）。
type Platform struct {
	// Platform はIDとOS/arch/libcのtupleである。
	//
	// §5の表どおりの組だけを表す。IDと個別fieldの一致は[domain.ParsePlatform]の
	// 固定表と突き合わせて検査する。
	Platform domain.Platform
	// ArtifactKind は配布物の提供主体である。
	ArtifactKind ArtifactKind
	// LicenseNotice はOSI承認OSS licenseでない配布物へ宣言するmessage IDである。
	//
	// 宣言しないplatformでは未設定になる。
	LicenseNotice domain.MessageID
	// Provider は§5.1の取得主体である。
	Provider Provider

	// VersionSource は§6の`version_source`である（2本目で型を与える）。
	VersionSource RawTable
	// Artifact は§7の`artifact`である（3本目で型を与える）。
	Artifact RawTable
	// Install は§9の`install`である（3本目で型を与える）。
	Install RawTable
	// Storage は§8の`storage`である（3本目で型を与える）。空可。
	Storage []RawTable
	// Runtime は§10の`runtime`である（3本目で型を与える）。
	Runtime RawTable
	// Validation は§11の`validation`である（3本目で型を与える）。
	Validation RawTable
}

// RawTable は未検証のTOML tableである。
//
// 本PRの範囲外の節を、内容を解釈せずにそのまま保持する。keyの存在だけを見て、
// 中身の許可key・enum・上限は後続PRが検査する。
type RawTable map[string]any

// Provider は§5.1の取得主体である。
//
// officialはname/homepage/license必須、repositoryは任意、adoption_reason禁止。
// third-partyは全件必須。
type Provider struct {
	// Name は提供主体の表示名である。
	Name string
	// Repository はsource repositoryのHTTPS URLである。officialでは空可。
	Repository string
	// Homepage は提供主体のHTTPS URLである。
	Homepage string
	// License は配布物のSPDX expressionである。tool本体のlicenseと異なってよい。
	License string
	// AdoptionReason はthird-partyを採用した理由である。officialでは空。
	AdoptionReason string
}

// definitionFile は§2のtop-level key集合である。
//
// 全fieldをpointerにするのは、keyの欠落と型のzero値を区別するためである。
// `schema = 0`とkey欠落を同じ扱いにすると、必須keyの検査がすり抜ける。
type definitionFile struct {
	Schema    *int64          `toml:"schema"`
	SchemaID  *string         `toml:"schema_id"`
	Tool      *toolTable      `toml:"tool"`
	Platforms []platformTable `toml:"platforms"`
}

type toolTable struct {
	ID            *string   `toml:"id"`
	Name          *string   `toml:"name"`
	Aliases       *[]string `toml:"aliases"`
	Description   *string   `toml:"description"`
	Homepage      *string   `toml:"homepage"`
	License       *string   `toml:"license"`
	VersionScheme *string   `toml:"version_scheme"`
}

type platformTable struct {
	ID            *string        `toml:"id"`
	OS            *string        `toml:"os"`
	Arch          *string        `toml:"arch"`
	Libc          *string        `toml:"libc"`
	ArtifactKind  *string        `toml:"artifact_kind"`
	LicenseNotice *string        `toml:"license_notice"`
	Provider      *providerTable `toml:"provider"`
	VersionSource *RawTable      `toml:"version_source"`
	Artifact      *RawTable      `toml:"artifact"`
	Install       *RawTable      `toml:"install"`
	Storage       *[]RawTable    `toml:"storage"`
	Runtime       *RawTable      `toml:"runtime"`
	Validation    *RawTable      `toml:"validation"`
}

type providerTable struct {
	Name           *string `toml:"name"`
	Repository     *string `toml:"repository"`
	Homepage       *string `toml:"homepage"`
	License        *string `toml:"license"`
	AdoptionReason *string `toml:"adoption_reason"`
}

// 本packageが出すstable reason codeである（§13）。
//
// 定数で持つのは、同じ失敗が呼出し箇所ごとに別のIDになるのを防ぐためである。
const (
	reasonDecode        = "definition.decode_failed"
	reasonMissing       = "definition.key_missing"
	reasonSchema        = "definition.schema_unsupported"
	reasonSchemaID      = "definition.schema_id_mismatch"
	reasonIdentifier    = "definition.identifier_invalid"
	reasonBasename      = "definition.id_basename_mismatch"
	reasonText          = "definition.text_invalid"
	reasonURL           = "definition.url_invalid"
	reasonLicense       = "definition.license_invalid"
	reasonEnum          = "definition.enum_invalid"
	reasonLimit         = "definition.limit_exceeded"
	reasonDuplicate     = "definition.duplicate_entry"
	reasonPlatformTuple = "definition.platform_tuple_mismatch"
	reasonProviderKey   = "definition.provider_key_invalid"
	reasonMessageID     = "definition.message_id_invalid"
)

// Parse はdefinition TOMLを検証してtyped modelへ変換する。
//
// `path`はregistry rootからのrelative pathで、§4の`id`がそのbasenameと一致する
// ことの検査と、§13の診断へ載せるpathに使う。
//
// 検証順序は§13に従う。byte/TOML/unknown/duplicate（1）、schema/schema_id（2）、
// identifier/URL/enum/型/上限（3）、platform tupleと`license_notice`（4）まで
// を本PRが担当する。§6以降（5〜10）はP3-01の2本目・3本目、registry全体の衝突
// 検査（11）はP4-01の範囲である。
//
// 1件目で止めず[DiagnosticMax]件まで集約する。registry更新のたびに1件ずつしか
// 直せないと、修正の往復が実用にならない。
func Parse(path string, data []byte) (*Definition, *domain.Error) {
	diagnostics := NewDiagnostics(path)
	var file definitionFile

	// §13-1: byte/TOML/unknown/duplicate。ここで落ちると後続の検査へ渡す値が
	// 無いため、1件だけ報告して打ち切る。
	if decodeErr := decodeFile(data, &file); decodeErr != nil {
		diagnostics.AddAt(decodeErr.field, decodeErr.line, decodeErr.column,
			reason(reasonDecode), decodeErr.Error())
		return nil, diagnostics.Err()
	}

	checkSchema(&file, diagnostics)
	value := &Definition{Path: path}
	buildTool(file.Tool, path, value, diagnostics)
	buildPlatforms(file.Platforms, value, diagnostics)

	if err := diagnostics.Err(); err != nil {
		return nil, err
	}
	return value, nil
}

// checkSchema は§13-2のschema/schema_idを検査する。
func checkSchema(file *definitionFile, diagnostics *Diagnostics) {
	switch {
	case file.Schema == nil:
		diagnostics.Add("schema", reason(reasonMissing), "top-level `schema`が無い")
	case *file.Schema != SchemaVersion:
		diagnostics.Add("schema", reason(reasonSchema),
			fmt.Sprintf("schemaは%dだけを読める（%d）", SchemaVersion, *file.Schema))
	}
	switch {
	case file.SchemaID == nil:
		diagnostics.Add("schema_id", reason(reasonMissing), "top-level `schema_id`が無い")
	case *file.SchemaID != SchemaID:
		diagnostics.Add("schema_id", reason(reasonSchemaID),
			fmt.Sprintf("schema_idが%qと一致しない", SchemaID))
	}
}
