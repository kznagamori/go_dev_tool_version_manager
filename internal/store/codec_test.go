package store

import (
	"strings"
	"testing"
)

// TestCheckDuplicateJSONKeys は`encoding/json`の後勝ち受理を塞げていることを固定する。
//
// DisallowUnknownFieldsは重複keyを検出しない。この検査だけが担当するため、
// decodeを通さずに直接呼び、他の理由で拒否されていないことを確かめる。
func TestCheckDuplicateJSONKeys(t *testing.T) {
	rejects := []struct {
		name string
		json string
	}{
		{"top-level重複", `{"a":1,"a":2}`},
		{"nested object重複", `{"outer":{"a":1,"a":2}}`},
		{"array内object重複", `{"items":[{"a":1,"a":2}]}`},
		{"値がstringの重複", `{"a":"x","a":"y"}`},
		{"深いnestの重複", `{"a":{"b":{"c":{"d":1,"d":2}}}}`},
	}
	for _, test := range rejects {
		t.Run(test.name, func(t *testing.T) {
			if err := checkDuplicateJSONKeys([]byte(test.json)); err == nil {
				t.Error("重複keyが通った")
			}
		})
	}

	accepts := []struct {
		name string
		json string
	}{
		{"別objectの同名key", `{"a":{"x":1},"b":{"x":2}}`},
		{"array内の別要素で同名key", `{"items":[{"x":1},{"x":2}]}`},
		// key名と同じ文字列を値に持つ場合。値をkeyと誤認すると誤検出になる。
		{"値がkeyと同名", `{"a":"a","b":"a"}`},
		{"key名の値が2回出る", `{"a":"dup","b":"dup"}`},
		{"array of string", `{"a":["x","x"]}`},
		{"空object", `{}`},
		{"空array", `{"a":[]}`},
	}
	for _, test := range accepts {
		t.Run(test.name, func(t *testing.T) {
			if err := checkDuplicateJSONKeys([]byte(test.json)); err != nil {
				t.Errorf("正当なJSONが落ちた: %v", err)
			}
		})
	}
}

// TestDecodeJSONRejectsTrailingData は§17の「exactly 1 JSON document」を固定する。
func TestDecodeJSONRejectsTrailingData(t *testing.T) {
	type payload struct {
		A int `json:"a"`
	}
	var target payload
	if err := decodeJSON([]byte(`{"a":1}`), &target); err != nil {
		t.Fatalf("単一documentが落ちた: %v", err)
	}
	for _, source := range []string{`{"a":1}{"a":2}`, `{"a":1} 2`, `{"a":1}[]`} {
		if err := decodeJSON([]byte(source), &target); err == nil {
			t.Errorf("trailing data %q が通った", source)
		}
	}
	// 末尾の空白とLFは1 documentのままである。永続fileは末尾LFを持つ（§7）。
	if err := decodeJSON([]byte("{\"a\":1}\n"), &target); err != nil {
		t.Errorf("末尾LFが落ちた: %v", err)
	}
}

// TestNormalizeTrailingLF は§7の「末尾LFちょうど1つ」を固定する。
func TestNormalizeTrailingLF(t *testing.T) {
	tests := []struct{ in, want string }{
		{"a", "a\n"},
		{"a\n", "a\n"},
		{"a\n\n\n", "a\n"},
		{"a\nb", "a\nb\n"},
	}
	for _, test := range tests {
		if got := string(normalizeTrailingLF([]byte(test.in))); got != test.want {
			t.Errorf("normalizeTrailingLF(%q) = %q, want %q", test.in, got, test.want)
		}
	}
}

// TestParseTimestampRequiresUTCZ は§7のtimestamp表現を固定する。
func TestParseTimestampRequiresUTCZ(t *testing.T) {
	if _, err := parseTimestamp("t", "2026-08-07T09:00:00Z"); err != nil {
		t.Errorf("秒精度のUTCが落ちた: %v", err)
	}
	// 「秒精度以上」のため、小数秒は許す。
	if _, err := parseTimestamp("t", "2026-08-07T09:00:00.123456Z"); err != nil {
		t.Errorf("小数秒が落ちた: %v", err)
	}
	rejects := []string{
		"", "2026-08-07", "2026-08-07T09:00:00", "2026-08-07T09:00:00+09:00",
		"2026-08-07T09:00:00-00:00", "2026-08-07 09:00:00Z", "not a time",
		// offsetがゼロでも`Z`表記でなければ拒否する。byte列でstateの同一性を
		// 判定するため、同じ時刻に2通りの表現を許さない。
		"2026-08-07T09:00:00+00:00",
	}
	for _, text := range rejects {
		if _, err := parseTimestamp("t", text); err == nil {
			t.Errorf("timestamp %q が通った", text)
		}
	}
}

// TestRequireRelativePath は§7のPOSIX relative path表現を固定する。
func TestRequireRelativePath(t *testing.T) {
	accepts := []string{"a", "a/b", "tools/node/versions/22.18.0/.gdtvm-install.toml", ".gdtvm-install.toml"}
	for _, text := range accepts {
		if _, err := requireRelativePath("p", text); err != nil {
			t.Errorf("relative path %q が落ちた: %v", text, err)
		}
	}
	rejects := []string{
		"", "/a", "a/", "//a", "a//b", `a\b`, `C:\a`, "./a", "../a", "a/./b", "a/../b", "..",
	}
	for _, text := range rejects {
		if _, err := requireRelativePath("p", text); err == nil {
			t.Errorf("path %q が通った", text)
		}
	}
}

// TestRequireNonNegative は§7の非負・2^53-1境界を固定する。
func TestRequireNonNegative(t *testing.T) {
	if _, err := requireNonNegative("n", 0); err != nil {
		t.Errorf("0が落ちた: %v", err)
	}
	if _, err := requireNonNegative("n", JSONMaxSafeInteger); err != nil {
		t.Errorf("2^53-1が落ちた: %v", err)
	}
	if _, err := requireNonNegative("n", -1); err == nil {
		t.Error("負値が通った")
	}
	if _, err := requireNonNegative("n", JSONMaxSafeInteger+1); err == nil {
		t.Error("2^53が通った")
	}
}

// TestCheckTextInput は§7のBOM/UTF-8制約を固定する。
func TestCheckTextInput(t *testing.T) {
	if err := checkTextInput([]byte("schema = 1")); err != nil {
		t.Errorf("正当なUTF-8が落ちた: %v", err)
	}
	if err := checkTextInput([]byte("\ufeffschema = 1")); err == nil {
		t.Error("BOMが通った")
	}
	if err := checkTextInput([]byte{0xff, 0xfe}); err == nil {
		t.Error("不正UTF-8が通った")
	}
	// 日本語を含むfileは正当である。BOM検査でUTF-8全体を拒否していない。
	if err := checkTextInput([]byte("name = \"日本語\"")); err != nil {
		t.Errorf("日本語が落ちた: %v", err)
	}
}

// TestDescribeTOMLErrorHasPosition は破損fileの診断に行・列が入ることを固定する。
func TestDescribeTOMLErrorHasPosition(t *testing.T) {
	unknown := specSchemaTOML + "unexpected_key = 1\n"
	_, err := ParseStateSchema([]byte(unknown))
	if err == nil {
		t.Fatal("unknown keyが通った")
	}
	detail := err.Cause.Error()
	if !strings.Contains(detail, "行") || !strings.Contains(detail, "列") {
		t.Errorf("診断に位置が入っていない: %s", detail)
	}
	if !strings.Contains(detail, "unexpected_key") {
		t.Errorf("診断にkey名が入っていない: %s", detail)
	}
}

// TestStateErrorsCarryRoleWithoutPath はdocs/10-security.md §9.2を固定する。
//
// 実pathをmessage parameterへ入れず、roleだけを伝える。
func TestStateErrorsCarryRoleWithoutPath(t *testing.T) {
	_, err := ParseReceiptIndex([]byte("schema = 2\n"))
	if err == nil {
		t.Fatal("schema 2が通った")
	}
	if err.Code != "E_STATE_CORRUPT" {
		t.Errorf("code = %q, want E_STATE_CORRUPT", err.Code)
	}
	if err.PathRole != "receipt-index" {
		t.Errorf("path role = %q, want receipt-index", err.PathRole)
	}
	if len(err.Parameters) != 0 {
		t.Errorf("parametersが空でない: %v", err.Parameters)
	}
	// Error()はCauseを含めない（P1-04）。内部診断が公開文へ漏れないことの確認。
	if strings.Contains(err.Error(), "schema") {
		t.Errorf("Error()にcauseが漏れている: %s", err.Error())
	}
}
