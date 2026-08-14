package definition

import (
	"strings"
	"testing"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
)

// TestDiagnosticsStopsAtLimit は§13の「上限100件で停止する」を固定する。
//
// 上限はdocs/04-storage-and-data.md §21の「diagnostic/error集約 100件」である。
func TestDiagnosticsStopsAtLimit(t *testing.T) {
	if DiagnosticMax != 100 {
		t.Fatalf("DiagnosticMax = %d, want 100", DiagnosticMax)
	}
	diagnostics := NewDiagnostics(specDefinitionPath)
	for index := 0; index < DiagnosticMax; index++ {
		if !diagnostics.Add("field", reason(reasonMissing), "detail") {
			t.Fatalf("%d件目で打ち切られた", index+1)
		}
	}
	if diagnostics.Truncated() {
		t.Error("上限ちょうどでtruncatedになった")
	}
	if diagnostics.Add("field", reason(reasonMissing), "detail") {
		t.Error("上限を超えて追加できた")
	}
	if !diagnostics.Truncated() {
		t.Error("上限超過がtruncatedに現れない")
	}
	if diagnostics.Len() != DiagnosticMax {
		t.Errorf("Len = %d, want %d", diagnostics.Len(), DiagnosticMax)
	}
	truncated, ok := diagnostics.Err().Parameters["truncated"]
	if !ok {
		t.Fatal("parametersにtruncatedが無い")
	}
	if value, _ := truncated.Bool(); !value {
		t.Error("truncatedがfalse")
	}
}

// TestDiagnosticsAggregatesInsteadOfFirstFailure は§13の集約を固定する。
//
// 1件目で止めると、registry更新のたびに1件ずつしか直せず修正の往復が実用に
// ならない。同じfileの複数errorが1回の実行で出ることを確かめる。
func TestDiagnosticsAggregatesInsteadOfFirstFailure(t *testing.T) {
	// schema、schema_id、tool.name、artifact_kindの4か所を同時に壊す。
	source := strings.NewReplacer(
		"schema = 1", "schema = 2",
		`schema_id = "`+SchemaID+`"`, `schema_id = "x"`,
		`name = "Node.js"`, `name = ""`,
		`artifact_kind = "official"`, `artifact_kind = "vendor"`,
	).Replace(specDefinitionTOML)

	_, err := Parse(specDefinitionPath, []byte(source))
	if err == nil {
		t.Fatal("Parse = nil, want error")
	}
	count, ok := err.Parameters["count"]
	if !ok {
		t.Fatalf("parametersにcountが無い: %v", err.Parameters)
	}
	value, _ := count.Int()
	if value != 4 {
		t.Errorf("count = %d, want 4（cause=%v）", value, err.Cause)
	}
}

// TestDiagnosticCarriesPositionAndField は§13の4項目を固定する。
//
// 「definition relative path、line/column、field path、stable reason code」。
// unknown keyはTOML parserが位置を返すため、4項目すべてが埋まる経路である。
func TestDiagnosticCarriesPositionAndField(t *testing.T) {
	source := strings.Replace(specDefinitionTOML, `id = "node"`, `id = "node"`+"\nextra = 1", 1)
	diagnostics := NewDiagnostics(specDefinitionPath)
	var file definitionFile
	decodeErr := decodeFile([]byte(source), &file)
	if decodeErr == nil {
		t.Fatal("unknown keyが通った")
	}
	diagnostics.AddAt(decodeErr.field, decodeErr.line, decodeErr.column,
		reason(reasonDecode), decodeErr.Error())

	items := diagnostics.Items()
	if len(items) != 1 {
		t.Fatalf("診断 = %d件", len(items))
	}
	item := items[0]
	if item.Path != specDefinitionPath {
		t.Errorf("Path = %q", item.Path)
	}
	if item.Line <= 0 || item.Column <= 0 {
		t.Errorf("位置が入っていない: %d行%d列", item.Line, item.Column)
	}
	if !strings.Contains(item.Field, "extra") {
		t.Errorf("Field = %q, want extraを含む", item.Field)
	}
	if item.Reason.String() != reasonDecode {
		t.Errorf("Reason = %q, want %q", item.Reason, reasonDecode)
	}
	// Stringは4項目を1行へまとめる。log/決定記録で原因を追うために使う。
	if !strings.Contains(item.String(), specDefinitionPath) ||
		!strings.Contains(item.String(), reasonDecode) {
		t.Errorf("String = %q", item.String())
	}
}

// TestDiagnosticItemsAreCopied は返した診断の書換えが集約へ波及しないことを固定する。
func TestDiagnosticItemsAreCopied(t *testing.T) {
	diagnostics := NewDiagnostics(specDefinitionPath)
	diagnostics.Add("schema", reason(reasonSchema), "detail")
	items := diagnostics.Items()
	items[0].Field = "書換え"
	if diagnostics.Items()[0].Field != "schema" {
		t.Error("呼出し側の書換えが集約へ波及した")
	}
}

// TestDiagnosticsErrIsNilWhenEmpty は診断が無いときにerrorを作らないことを固定する。
func TestDiagnosticsErrIsNilWhenEmpty(t *testing.T) {
	if err := NewDiagnostics(specDefinitionPath).Err(); err != nil {
		t.Errorf("Err = %s, want nil", describe(err))
	}
}

// TestReasonCodesAreValidMessageIDs は全reason codeがmessage ID grammarに従うことを
// 固定する（docs/04-storage-and-data.md §7）。
//
// 壊れたIDはcatalog lookupが静かに失敗するため、定数の時点で弾く。
func TestReasonCodesAreValidMessageIDs(t *testing.T) {
	codes := []string{
		reasonDecode, reasonMissing, reasonSchema, reasonSchemaID,
		reasonIdentifier, reasonBasename, reasonText, reasonURL,
		reasonLicense, reasonEnum, reasonLimit, reasonDuplicate,
		reasonPlatformTuple, reasonProviderKey, reasonMessageID,
	}
	seen := make(map[string]struct{}, len(codes))
	for _, code := range codes {
		if _, err := domain.ParseMessageID(code); err != nil {
			t.Errorf("reason code %q がmessage ID grammarに合わない: %v", code, err)
		}
		if !strings.HasPrefix(code, "definition.") {
			t.Errorf("reason code %q が`definition.`で始まらない", code)
		}
		if _, duplicate := seen[code]; duplicate {
			t.Errorf("reason code %q が重複している", code)
		}
		seen[code] = struct{}{}
	}
}
