package definition

import (
	"strings"
	"testing"
)

// TestReplaceTemplateVarsCoversEveryOccurrence は未知変数を素通りさせない切り出しを固定する。
//
// docs/06-tool-definition.md §12「未知変数、再帰展開、function、condition、shell
// evaluationを禁止する」。中身を限定せずすべての`{{...}}`を拾ってから許可集合と
// 突き合わせるため、限定した正規表現なら取りこぼす形も1件として渡ることを見る。
func TestReplaceTemplateVarsCoversEveryOccurrence(t *testing.T) {
	tests := []struct {
		name   string
		text   string
		tokens []string
	}{
		{"変数なし", "go1.25.0", nil},
		{"1件", "go{{version}}", []string{"{{version}}"}},
		{"複数", "{{version}}-{{platform.id}}",
			[]string{"{{version}}", "{{platform.id}}"}},
		{"引数付き", "{{storage.npm-cache}}", []string{"{{storage.npm-cache}}"}},
		// 未知変数も1件として渡る。呼出し側がerrorにできる。
		{"未知変数", "x {{unknown}} y", []string{"{{unknown}}"}},
		{"空の変数", "{{}}", []string{"{{}}"}},
		// 閉じていない`{{`はtokenにならない。literalとして残るため、
		// path contextでは[splitTemplateRoot]相当の検査が別途拒否する。
		{"閉じていない", "{{version", nil},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var got []string
			ReplaceTemplateVars(test.text, func(token string) string {
				got = append(got, token)
				return ""
			})
			if strings.Join(got, "|") != strings.Join(test.tokens, "|") {
				t.Errorf("tokens = %q, want %q", got, test.tokens)
			}
		})
	}
}

// TestReplaceTemplateVarsSubstitutes は置換結果を固定する。
func TestReplaceTemplateVarsSubstitutes(t *testing.T) {
	got := ReplaceTemplateVars("go{{version}}/{{platform.id}}", func(token string) string {
		if token == VersionTemplate {
			return "1.25.0"
		}
		return "linux-amd64"
	})
	if want := "go1.25.0/linux-amd64"; got != want {
		t.Errorf("rendered = %q, want %q", got, want)
	}
}

// TestStorageTemplateIDFollowsIdentifierGrammar はIDのgrammarを固定する。
//
// §3のkebab-case identifier（先頭は英字、区切りは単一の`-`）。先頭数字や
// underscoreを通すと、§3と食い違うIDがrender側だけで通る。
func TestStorageTemplateIDFollowsIdentifierGrammar(t *testing.T) {
	tests := []struct {
		token string
		want  string
		ok    bool
	}{
		{"{{storage.npm-cache}}", "npm-cache", true},
		{"{{storage.a}}", "a", true},
		{"{{storage.a1}}", "a1", true},
		{"{{storage.a-b-c}}", "a-b-c", true},
		{"{{storage.9x}}", "", false},
		{"{{storage.Npm}}", "", false},
		{"{{storage.npm_cache}}", "", false},
		{"{{storage.npm--cache}}", "", false},
		{"{{storage.-npm}}", "", false},
		{"{{storage.npm-}}", "", false},
		{"{{storage.}}", "", false},
		{"{{storage}}", "", false},
		{"{{payload}}", "", false},
		// 前後にliteralが付く形は1つのrootとして扱わない。
		{"{{storage.npm-cache}}/x", "", false},
		{"x{{storage.npm-cache}}", "", false},
	}
	for _, test := range tests {
		t.Run(test.token, func(t *testing.T) {
			got, ok := StorageTemplateID(test.token)
			if ok != test.ok || got != test.want {
				t.Errorf("StorageTemplateID(%q) = %q, %v; want %q, %v",
					test.token, got, ok, test.want, test.ok)
			}
		})
	}
}
