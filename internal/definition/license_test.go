package definition

import (
	"strings"
	"testing"
)

// TestLicenseExpressionSyntax はSPDX expressionのsyntax検査を固定する（§4・§5.1）。
//
// **SPDX license listへの登録有無は見ない。** listをclientへ同梱すると
// registryとは別の更新経路ができ、上流のlist更新で既存definitionが読めなくなる。
// 実在確認はdocs/07-registry-and-tools.md §5のregistry reviewが担う。
func TestLicenseExpressionSyntax(t *testing.T) {
	valid := []string{
		// 標準4 toolが使う値。
		"MIT", "Apache-2.0", "BSD-3-Clause", "MPL-2.0",
		"LicenseRef-dotnet-library",
		"LicenseRef-python-build-standalone",
		// SPDX 2.3の演算子と接尾辞。
		"GPL-2.0-only OR MIT",
		"Apache-2.0 AND MIT",
		"GPL-2.0-or-later WITH Classpath-exception-2.0",
		"(MIT OR Apache-2.0) AND BSD-3-Clause",
		"MIT+",
		"DocumentRef-spdx-tool:LicenseRef-custom",
		// listに無い識別子もsyntaxが正しければ通る。実在確認はregistry reviewである。
		"NotARealLicense-9.9",
	}
	for _, text := range valid {
		if err := checkLicenseExpression(text); err != nil {
			t.Errorf("%q が落ちた: %v", text, err)
		}
	}

	invalid := []struct{ name, text string }{
		{"空", ""},
		{"前後空白", " MIT"},
		{"演算子だけ", "AND"},
		{"演算子で終わる", "MIT OR"},
		{"演算子で始まる", "OR MIT"},
		{"括弧が閉じない", "(MIT OR Apache-2.0"},
		{"括弧が開かない", "MIT OR Apache-2.0)"},
		{"括弧が空", "()"},
		{"WITHの右辺が無い", "GPL-2.0-only WITH"},
		{"WITHの右辺が演算子", "GPL-2.0-only WITH OR"},
		{"idstring外の文字", "MIT/Apache-2.0"},
		{"ASCII以外", "ＭＩＴ"},
		{"LicenseRefの本体が空", "LicenseRef-"},
		{"DocumentRefにcolonが無い", "DocumentRef-spdx-tool"},
		{"DocumentRefの後がLicenseRefでない", "DocumentRef-spdx-tool:custom"},
		{"小文字の演算子", "MIT or Apache-2.0"},
		{"式の後に余分な語", "MIT Apache-2.0"},
		{"上限超過", strings.Repeat("A", LicenseMaxBytes+1)},
	}
	for _, test := range invalid {
		t.Run(test.name, func(t *testing.T) {
			if err := checkLicenseExpression(test.text); err == nil {
				t.Errorf("%q が通った", test.text)
			}
		})
	}
}

// TestParseRejectsInvalidLicense はdefinition経由でlicense検査が効くことを固定する。
func TestParseRejectsInvalidLicense(t *testing.T) {
	tests := []struct{ name, old, value string }{
		{"tool.license", `license = "MIT"
version_scheme`, `license = "MIT OR"
version_scheme`},
		{"provider.license", `license = "MIT"

[platforms.version_source]`, `license = "MIT/x"

[platforms.version_source]`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := parseSpec(t, test.old, test.value)
			if err == nil {
				t.Fatal("Parse = nil, want error")
			}
			assertReason(t, err, reasonLicense)
		})
	}
}
