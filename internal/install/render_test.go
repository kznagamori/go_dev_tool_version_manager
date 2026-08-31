package install

import (
	"strings"
	"testing"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/definition"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
)

const (
	payloadRoot   = "/data/gdtvm/tools/go/1.25.0/payload"
	probeTempRoot = "/data/gdtvm/tmp/operations/op1/probe"
	cacheRoot     = "/data/gdtvm/tools/node/shared/npm-cache"
)

// renderPathValue はrole付きpathを作る。
func renderPathValue(t *testing.T, role domain.PathRole, path string) domain.PathValue {
	t.Helper()
	value, err := domain.NewPathValue(role, path)
	if err != nil {
		t.Fatalf("NewPathValue(%s, %q): %v", role, path, err)
	}
	return value
}

// testRoots は既定のrender rootを返す。
func testRoots(t *testing.T, host string) RenderRoots {
	t.Helper()
	return RenderRoots{
		Version:    "1.25.0",
		PlatformID: "linux-amd64-glibc",
		Payload:    renderPathValue(t, domain.RolePayload, payloadRoot),
		ProbeTemp:  renderPathValue(t, domain.RoleStaging, probeTempRoot),
		Storage: map[string]domain.PathValue{
			"npm-cache": renderPathValue(t, domain.RoleSharedStorage, cacheRoot),
		},
		Host: platformOf(t, host),
	}
}

// TestRenderPathResolvesRootsAndChildren はroot単体と子pathの両方を固定する。
//
// docs/06-tool-definition.md §12「logical pathはPOSIX slashで記述し、OS adapterが
// separatorへ変換する」。
func TestRenderPathResolvesRootsAndChildren(t *testing.T) {
	tests := []struct {
		name     string
		template string
		host     string
		want     string
		wantRole domain.PathRole
	}{
		{"payload単体", "{{payload}}", "linux-amd64-glibc", payloadRoot, domain.RolePayload},
		{"payloadの子", "{{payload}}/bin/go", "linux-amd64-glibc",
			payloadRoot + "/bin/go", domain.RolePayload},
		{"probe_tempの子", "{{probe_temp}}/work", "linux-amd64-glibc",
			probeTempRoot + "/work", domain.RoleStaging},
		{"storageの子", "{{storage.npm-cache}}/_logs", "linux-amd64-glibc",
			cacheRoot + "/_logs", domain.RoleSharedStorage},
		// Windowsではseparatorが変換される。子pathはPOSIX slashで書く。
		{"windowsの子path", "{{payload}}/bin/go.exe", "windows-amd64",
			payloadRoot + `\bin\go.exe`, domain.RolePayload},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := RenderPath(test.template, testRoots(t, test.host))
			if err != nil {
				t.Fatalf("RenderPath = %v", err)
			}
			if got.Path() != test.want {
				t.Errorf("path = %q, want %q", got.Path(), test.want)
			}
			// roleはrootから引き継ぐ。子pathでも同じ論理領域に属する。
			if got.Role() != test.wantRole {
				t.Errorf("role = %s, want %s", got.Role(), test.wantRole)
			}
		})
	}
}

// TestRenderPathRejectsLiteralConcatenation はliteralの連結を拒否することを固定する。
//
// docs/06-tool-definition.md §10.1・§11がliteral prefix/suffixの連結を禁じる。
// 連結を許すと、render後にrootの外を指す値を作れる。
func TestRenderPathRejectsLiteralConcatenation(t *testing.T) {
	tests := []struct {
		name     string
		template string
	}{
		{"prefixが付く", "/opt{{payload}}"},
		{"suffixが直結する", "{{payload}}bin"},
		{"suffixが直結する（拡張子）", "{{payload}}.bak"},
		{"rootで始まらない", "bin/{{payload}}"},
		{"templateでない", "/data/gdtvm/payload"},
		{"閉じていない", "{{payload"},
		{"子pathへ変数", "{{payload}}/{{version}}"},
		{"空component", "{{payload}}//bin"},
		{"末尾が区切り", "{{payload}}/bin/"},
		{"空文字列", ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := RenderPath(test.template, testRoots(t, "linux-amd64-glibc")); err == nil {
				t.Fatalf("%q が通った", test.template)
			}
		})
	}
}

// TestRenderPathRejectsEscape は子pathからrootの外へ出られないことを固定する。
//
// `..`と区切り混在は[security.Join]がcomponentごとに拒否する。
func TestRenderPathRejectsEscape(t *testing.T) {
	tests := []string{
		"{{payload}}/../other",
		"{{payload}}/bin/../../etc",
		"{{payload}}/.",
		`{{payload}}/bin\go`,
		"{{payload}}/bin/go\x00",
	}
	for _, template := range tests {
		t.Run(template, func(t *testing.T) {
			if _, err := RenderPath(template, testRoots(t, "linux-amd64-glibc")); err == nil {
				t.Fatalf("%q が通った", template)
			}
		})
	}
}

// TestRenderPathRejectsUnknownRoot は未知rootと未宣言storageを拒否することを固定する。
//
// §12「未知変数、再帰展開、function、condition、shell evaluationを禁止する」。
func TestRenderPathRejectsUnknownRoot(t *testing.T) {
	tests := []struct {
		name     string
		template string
	}{
		{"未知のroot", "{{data_root}}/x"},
		{"未宣言のstorage", "{{storage.unknown}}/x"},
		{"storage IDのgrammar違反", "{{storage.Npm_Cache}}/x"},
		{"path contextでversion", "{{version}}/x"},
		{"path contextでplatform.id", "{{platform.id}}/x"},
		{"空のroot", "{{}}/x"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := RenderPath(test.template, testRoots(t, "linux-amd64-glibc")); err == nil {
				t.Fatalf("%q が通った", test.template)
			}
		})
	}
}

// TestRenderPathRejectsUnsetRoot は未設定rootを空文字列で代替しないことを固定する。
//
// 空で代替すると`{{probe_temp}}/x`がfilesystem rootからの絶対pathになる。
//
// **子path付きとroot単体の両方を並べる。** 子path付きだけだと[security.Join]の
// zero root検査が先に落とすため、[RenderPath]自身の未設定検査を外しても検出
// できない。root単体（`rest == ""`）は[security.Join]を通らないので、この文脈
// だけが[RenderPath]の検査を直接固定する。
func TestRenderPathRejectsUnsetRoot(t *testing.T) {
	tests := []struct {
		name     string
		template string
		mutate   func(*RenderRoots)
	}{
		{"payloadが未設定", "{{payload}}/bin",
			func(r *RenderRoots) { r.Payload = domain.PathValue{} }},
		{"payloadが未設定（root単体）", "{{payload}}",
			func(r *RenderRoots) { r.Payload = domain.PathValue{} }},
		// §12「`{{probe_temp}}`はvalidation probe内だけ」。probe以外の文脈では
		// 呼出し側がProbeTempを渡さず、解決できないことで担保する。
		{"probe以外の文脈", "{{probe_temp}}/x",
			func(r *RenderRoots) { r.ProbeTemp = domain.PathValue{} }},
		{"probe以外の文脈（root単体）", "{{probe_temp}}",
			func(r *RenderRoots) { r.ProbeTemp = domain.PathValue{} }},
		{"storageが1件も無い", "{{storage.npm-cache}}/x",
			func(r *RenderRoots) { r.Storage = nil }},
		{"storageが1件も無い（root単体）", "{{storage.npm-cache}}",
			func(r *RenderRoots) { r.Storage = nil }},
		// storage rootのzero値。宣言はあるが値が入っていない場合を分けて見る。
		{"storage rootがzero値", "{{storage.npm-cache}}",
			func(r *RenderRoots) {
				r.Storage = map[string]domain.PathValue{"npm-cache": {}}
			}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			roots := testRoots(t, "linux-amd64-glibc")
			test.mutate(&roots)
			if _, err := RenderPath(test.template, roots); err == nil {
				t.Fatalf("%q が通った", test.template)
			}
		})
	}
}

// TestRenderPathEnforcesRenderedLimit はrender結果の上限を固定する。
//
// §12「render結果は32 KiB、path component 255 byte、URL 8 KiBの組込み上限」。
func TestRenderPathEnforcesRenderedLimit(t *testing.T) {
	roots := testRoots(t, "linux-amd64-glibc")
	// component上限（255 byte）内のcomponentを多数並べて全体長を超えさせる。
	component := strings.Repeat("a", 200)
	var builder strings.Builder
	builder.WriteString("{{payload}}")
	for total := 0; total < definition.RenderedMaxBytes; total += len(component) + 1 {
		builder.WriteString("/")
		builder.WriteString(component)
	}
	if _, err := RenderPath(builder.String(), roots); err == nil {
		t.Fatal("render結果の上限超過が通った")
	}
}

// TestRenderTextSubstitutesScalars はpath以外の文脈の置換を固定する。
func TestRenderTextSubstitutesScalars(t *testing.T) {
	roots := testRoots(t, "linux-amd64-glibc")
	got, err := RenderText("go version go{{version}} {{platform.id}}", roots)
	if err != nil {
		t.Fatalf("RenderText = %v", err)
	}
	want := "go version go1.25.0 linux-amd64-glibc"
	if got != want {
		t.Errorf("rendered = %q, want %q", got, want)
	}
	if empty, err := RenderText("", roots); err != nil || empty != "" {
		t.Errorf("空入力 = %q, %v", empty, err)
	}
}

// TestRenderTextRejectsUnknownVariable は未知変数を素通りさせないことを固定する。
//
// 素通りさせると、literalとして残った`{{...}}`がそのまま比較対象になる。
func TestRenderTextRejectsUnknownVariable(t *testing.T) {
	roots := testRoots(t, "linux-amd64-glibc")
	tests := []string{
		"{{unknown}}",
		"go{{version}} {{payload}}",
		"{{}}",
		"prefix {{metadata.key}} suffix",
	}
	for _, text := range tests {
		t.Run(text, func(t *testing.T) {
			if _, err := RenderText(text, roots); err == nil {
				t.Fatalf("%q が通った", text)
			}
		})
	}
}

// TestRenderTextRejectsUnsetScalar は未設定の値を空文字列で代替しないことを固定する。
func TestRenderTextRejectsUnsetScalar(t *testing.T) {
	roots := testRoots(t, "linux-amd64-glibc")
	roots.Version = ""
	if _, err := RenderText("go{{version}}", roots); err == nil {
		t.Fatal("versionが未設定でも通った")
	}
	roots = testRoots(t, "linux-amd64-glibc")
	roots.PlatformID = ""
	if _, err := RenderText("{{platform.id}}", roots); err == nil {
		t.Fatal("platform IDが未設定でも通った")
	}
}
