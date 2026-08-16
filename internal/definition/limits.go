package definition

// SchemaVersion はclientが読めるtool definition schema revisionである。
//
// docs/06-tool-definition.md §14が「clientは`schema=1`だけを読む。未知
// major/minorを推測して受理しない」と定める。
const SchemaVersion = 1

// SchemaID は`schema_id`が完全一致しなければならない値である（§2）。
//
// URLとしてfetchするものではなく、schemaの同一性を宣言する識別子である。
// 一致だけを見るのは、別schemaのfileを内容から推測して受理しないためである。
const SchemaID = "https://github.com/kznagamori/go_dev_tool_version_manager/schemas/tool-definition/v1"

// 構造上限の正本はdocs/04-storage-and-data.md §21である（§12.1）。
//
// 同§21の`definition`関連行をそのまま定数にする。利用者config/definitionから
// 拡大できず、重複をcount前後どちらでも黙って除去しない。
const (
	// FileMaxBytes はtool definition 1 fileの上限である。
	FileMaxBytes = 2 << 20
	// PlatformMax は1 definitionのplatform entry数の上限である。
	PlatformMax = 2
	// AliasMax は1 toolのalias数の上限である。
	AliasMax = 16
	// ArrayMax は上で個別に定めていないdefinition arrayの上限である。
	ArrayMax = 256
	// URLMaxBytes はdefinitionへ書くURLの上限である。
	URLMaxBytes = 8 << 10
	// DiagnosticMax は集約するdiagnosticの上限である（§13）。
	DiagnosticMax = 100
)

// §4 `[tool]`の長さ上限。
const (
	// NameMaxBytes は表示名の上限である。
	NameMaxBytes = 128
	// DescriptionMaxBytes は説明文の上限である。
	DescriptionMaxBytes = 512
)
