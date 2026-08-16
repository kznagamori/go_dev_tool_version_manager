package definition

import (
	"strings"
	"testing"
)

// TestIdentifierGrammarMatchesSpec はdocs/06-tool-definition.md §3の表を固定する。
func TestIdentifierGrammarMatchesSpec(t *testing.T) {
	tests := []struct {
		name     string
		validate func(string) error
		max      int
		valid    []string
		invalid  []string
	}{
		{
			name: "tool ID/alias", validate: ValidateToolID, max: ToolIDMaxBytes,
			valid:   []string{"go", "node", "dotnet-sdk", "a1", "a-1-b"},
			invalid: []string{"1go", "go_lang", "go.lang", "go+lang"},
		},
		{
			name: "platform/storage/probe/profile ID", validate: ValidateScopedID, max: ScopedIDMaxBytes,
			valid:   []string{"windows-amd64", "linux-amd64-glibc", "global-tools", "default", "version"},
			invalid: []string{"1st", "global_tools", "global.tools"},
		},
		{
			name: "command", validate: ValidateCommandName, max: CommandNameMaxBytes,
			// commandは`-+._`を区切りに許す。python3.13やpip3が該当する。
			valid:   []string{"go", "npm", "pip3", "python3.13", "a_b", "a+b", "a-b"},
			invalid: []string{"1go", "go--lang", "go..lang", "go/lang", `go\lang`},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, value := range test.valid {
				if err := test.validate(value); err != nil {
					t.Errorf("%q が落ちた: %v", value, err)
				}
			}
			// grammarに関わらず共通で落ちるもの（§3）。
			common := []string{
				"", "Go", " go", "go ", "-go", "go-", "go--lang", "ゴー", "go\x00",
				".", "..", "go/lang", `go\lang`,
			}
			for _, value := range append(common, test.invalid...) {
				if err := test.validate(value); err == nil {
					t.Errorf("%q が通った", value)
				}
			}
			// 長さ上限。
			if err := test.validate(strings.Repeat("a", test.max)); err != nil {
				t.Errorf("%d byteが落ちた: %v", test.max, err)
			}
			if err := test.validate(strings.Repeat("a", test.max+1)); err == nil {
				t.Errorf("%d byteが通った", test.max+1)
			}
		})
	}
}

// TestIdentifierRejectsWindowsReservedNames は§3の「Windows予約名」を固定する。
//
// grammarは小文字だけを許すため`con`や`com1`は文法上は正しい。予約名の判定は
// [security.ValidateComponent]へ委ねており、その経路が生きていることを見る。
func TestIdentifierRejectsWindowsReservedNames(t *testing.T) {
	reserved := []string{"con", "prn", "aux", "nul", "com1", "lpt1"}
	for _, value := range reserved {
		t.Run(value, func(t *testing.T) {
			if err := ValidateToolID(value); err == nil {
				t.Errorf("tool ID %q が通った", value)
			}
			if err := ValidateScopedID(value); err == nil {
				t.Errorf("scoped ID %q が通った", value)
			}
			// 拡張子を付けても予約は解除されない。
			if err := ValidateCommandName(value + ".exe"); err == nil {
				t.Errorf("command %q が通った", value+".exe")
			}
		})
	}
	// 予約名に見えるが予約でないものは通す。過剰な拒否をしていないことの確認。
	for _, value := range []string{"console", "com0", "com10", "nula"} {
		if err := ValidateToolID(value); err != nil {
			t.Errorf("%q が落ちた: %v", value, err)
		}
	}
}

// TestValidateMetadataKey は§3のmetadata key grammarを固定する。
func TestValidateMetadataKey(t *testing.T) {
	valid := []string{"a", "release_tag", "a1", strings.Repeat("a", 64)}
	for _, value := range valid {
		if err := ValidateMetadataKey(value); err != nil {
			t.Errorf("%q が落ちた: %v", value, err)
		}
	}
	invalid := []string{
		"", "A", "1a", "a-b", "a.b", "_a", " a", "a ",
		// grammarが64文字へ制限する（`[a-z]`＋63文字）。
		strings.Repeat("a", 65),
	}
	for _, value := range invalid {
		if err := ValidateMetadataKey(value); err == nil {
			t.Errorf("%q が通った", value)
		}
	}
}

// TestRequireUniqueIdentifiersIsCaseInsensitive は§3の衝突規則を固定する。
//
// §3は「同一tool内のcase-insensitive衝突を拒否する」と定める。grammarが小文字
// だけを許すため今日はexact一致と同じ結果になるが、比較そのものはcase非依存で
// 書いてある。この関係が崩れたらここで気付ける。
func TestRequireUniqueIdentifiersIsCaseInsensitive(t *testing.T) {
	if err := requireUniqueIdentifiers("tool ID/alias", []string{"go", "node"}); err != nil {
		t.Errorf("衝突しない組が落ちた: %v", err)
	}
	if err := requireUniqueIdentifiers("tool ID/alias", []string{"go", "go"}); err == nil {
		t.Error("exact重複が通った")
	}
	if err := requireUniqueIdentifiers("tool ID/alias", []string{"go", "GO"}); err == nil {
		t.Error("case違いの重複が通った")
	}
	// grammarはuppercaseを拒否するため、case違いの入力はここへ届かない。
	if err := ValidateToolID("GO"); err == nil {
		t.Error("grammarがuppercaseを受理した。衝突検査の前提が変わっている")
	}
}
