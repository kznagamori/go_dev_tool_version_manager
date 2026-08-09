package domain

import (
	"strings"
	"testing"
)

func TestScalarKindsAndAccessors(t *testing.T) {
	tests := []struct {
		name    string
		value   Scalar
		kind    ScalarKind
		display string
	}{
		{"null", NullScalar(), ScalarNull, "null"},
		{"zero値はnull", Scalar{}, ScalarNull, "null"},
		{"string", StringScalar("node"), ScalarString, "node"},
		{"空string", StringScalar(""), ScalarString, ""},
		{"bool true", BoolScalar(true), ScalarBool, "true"},
		{"bool false", BoolScalar(false), ScalarBool, "false"},
		{"int", IntScalar(-42), ScalarInt, "-42"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.value.Kind(); got != test.kind {
				t.Errorf("Kind = %v, want %v", got, test.kind)
			}
			if got := test.value.String(); got != test.display {
				t.Errorf("String = %q, want %q", got, test.display)
			}
			if got := test.value.IsNull(); got != (test.kind == ScalarNull) {
				t.Errorf("IsNull = %v", got)
			}
		})
	}

	// 型違いのaccessorはokをfalseにする。数値0と「値なし」を混同させない。
	if _, ok := StringScalar("x").Int(); ok {
		t.Error("string値のInt()がok=trueになった")
	}
	if _, ok := IntScalar(1).Str(); ok {
		t.Error("int値のStr()がok=trueになった")
	}
	if _, ok := NullScalar().Bool(); ok {
		t.Error("null値のBool()がok=trueになった")
	}
	if value, ok := IntScalar(7).Int(); !ok || value != 7 {
		t.Errorf("Int() = %d, %v", value, ok)
	}
}

func TestValidateParameterKey(t *testing.T) {
	valid := []string{"a", "tool_id", "version", "x1", "a_b_c_1", strings.Repeat("a", ParameterKeyMaxLength)}
	for _, key := range valid {
		if err := ValidateParameterKey(key); err != nil {
			t.Errorf("ValidateParameterKey(%q) = %v, want nil", key, err)
		}
	}

	invalid := []string{
		"",         // 空
		"ToolID",   // 大文字
		"tool-id",  // hyphen
		"1tool",    // 数字始まり
		"_tool",    // underscore始まり
		"tool id",  // 空白
		"tool.id",  // dot
		"tool_id ", // 末尾空白
		"ツール",      // 非ASCII
		strings.Repeat("a", ParameterKeyMaxLength+1), // 長さ超過
	}
	for _, key := range invalid {
		if err := ValidateParameterKey(key); err == nil {
			t.Errorf("ValidateParameterKey(%q) = nil, want error", key)
		}
	}
}

func TestParametersValidateAndLimit(t *testing.T) {
	good := Parameters{"tool_id": StringScalar("go"), "count": IntScalar(3)}
	if err := good.Validate(); err != nil {
		t.Fatalf("Validate = %v, want nil", err)
	}
	bad := Parameters{"tool_id": StringScalar("go"), "Bad-Key": NullScalar()}
	if err := bad.Validate(); err == nil {
		t.Fatal("Validate = nil, want error")
	}

	if err := good.ValidateWithLimit(2); err != nil {
		t.Errorf("ValidateWithLimit(2) = %v, want nil", err)
	}
	if err := good.ValidateWithLimit(1); err == nil {
		t.Error("ValidateWithLimit(1) = nil, want error")
	}

	if got := good.SortedKeys(); len(got) != 2 || got[0] != "count" || got[1] != "tool_id" {
		t.Errorf("SortedKeys = %v, want [count tool_id]", got)
	}

	// Cloneは共有を切る。境界通過後にimmutableとして扱うため。
	clone := good.Clone()
	clone["tool_id"] = StringScalar("node")
	if value, _ := good["tool_id"].Str(); value != "go" {
		t.Errorf("Clone後の書換えが元へ伝わった: %q", value)
	}
	if Parameters(nil).Clone() != nil {
		t.Error("nilのCloneがnilでない")
	}
}

func TestParseMessageID(t *testing.T) {
	valid := []string{
		"install.started", "error.state_corrupt", "warning.third_party",
		"a.b", "a.b.c.d", "e1.f2_3",
	}
	for _, text := range valid {
		id, err := ParseMessageID(text)
		if err != nil {
			t.Errorf("ParseMessageID(%q) = %v, want nil", text, err)
			continue
		}
		if id.String() != text {
			t.Errorf("String = %q, want %q", id.String(), text)
		}
		if id.IsZero() {
			t.Errorf("ParseMessageID(%q) がzero値を返した", text)
		}
	}

	invalid := []string{
		"",                // 空
		"install",         // segmentが1件
		".install",        // 先頭dot
		"install.",        // 末尾dot
		"install..start",  // 連続dot
		"Install.Started", // 大文字
		"install-started", // segmentが1件、hyphen
		"install.1st",     // segmentが数字始まり
		"install._start",  // segmentがunderscore始まり
		"install.start ",  // 空白
		"インストール.開始",       // 非ASCII
		strings.Repeat("a", MessageIDMaxLength) + ".b", // 長さ超過
	}
	for _, text := range invalid {
		if _, err := ParseMessageID(text); err == nil {
			t.Errorf("ParseMessageID(%q) = nil, want error", text)
		}
	}
}

func TestMessageIDSegments(t *testing.T) {
	id, err := ParseMessageID("error.state_corrupt")
	if err != nil {
		t.Fatalf("ParseMessageID = %v", err)
	}
	got := id.Segments()
	if len(got) != 2 || got[0] != "error" || got[1] != "state_corrupt" {
		t.Errorf("Segments = %v, want [error state_corrupt]", got)
	}
	var zero MessageID
	if zero.Segments() != nil {
		t.Error("zero値のSegmentsがnilでない")
	}
}

func TestParseIDs(t *testing.T) {
	const valid = "33333333333333333333333333333333"

	invocation, err := ParseInvocationID(valid)
	if err != nil {
		t.Fatalf("ParseInvocationID = %v", err)
	}
	if invocation.String() != valid || invocation.IsZero() {
		t.Errorf("InvocationID = %q, IsZero=%v", invocation, invocation.IsZero())
	}
	operation, err := ParseOperationID(valid)
	if err != nil {
		t.Fatalf("ParseOperationID = %v", err)
	}
	if operation.String() != valid {
		t.Errorf("OperationID = %q", operation)
	}

	invalid := []string{
		"",
		"3333333333333333333333333333333",      // 31桁
		"333333333333333333333333333333333",    // 33桁
		"3333333333333333333333333333333G",     // hex外
		"33333333333333333333333333333333 ",    // 空白
		"ABCDEF33333333333333333333333333",     // 大文字hex
		"33333333-3333-3333-3333-333333333333", // UUID表記
	}
	for _, text := range invalid {
		if _, err := ParseInvocationID(text); err == nil {
			t.Errorf("ParseInvocationID(%q) = nil, want error", text)
		}
		if _, err := ParseOperationID(text); err == nil {
			t.Errorf("ParseOperationID(%q) = nil, want error", text)
		}
	}
}

// TestNewIDFromBytes はport由来のbyte列が正規形へ変換されることを固定する。
func TestNewIDFromBytes(t *testing.T) {
	var raw [IDByteLength]byte
	raw[IDByteLength-1] = 1

	const want = "00000000000000000000000000000001"
	if got := NewInvocationID(raw).String(); got != want {
		t.Errorf("NewInvocationID = %q, want %q", got, want)
	}
	if got := NewOperationID(raw).String(); got != want {
		t.Errorf("NewOperationID = %q, want %q", got, want)
	}

	// 生成した値はParseを通る。encodingとgrammarがずれていないことの確認。
	if _, err := ParseInvocationID(NewInvocationID(raw).String()); err != nil {
		t.Errorf("生成したIDがParseで落ちた: %v", err)
	}
	if IDLength != 2*IDByteLength {
		t.Errorf("IDLength = %d, want %d", IDLength, 2*IDByteLength)
	}
}
