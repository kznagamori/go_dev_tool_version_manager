package registry

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
)

// specCatalog はdocs/04-storage-and-data.md §20の契約を満たす最小のcatalogである。
const specCatalog = `error.internal = "内部errorが発生しました。"
config.color_invalid = "color の値 {value} は auto、always、never のいずれかにしてください。"
registry.invalid = "同梱registryの内容が不正です: {reason}"
`

// TestParseMessageCatalogAcceptsSpecShape は§20の形が通ることを固定する。
func TestParseMessageCatalogAcceptsSpecShape(t *testing.T) {
	catalog, err := ParseMessageCatalog([]byte(specCatalog))
	if err != nil {
		t.Fatalf("ParseMessageCatalog = %v", err.Cause)
	}
	if catalog.Len() != 3 {
		t.Fatalf("Len = %d, want 3", catalog.Len())
	}
	// 宣言順を保つ。診断をfile上で辿れるようにするためである。
	want := []string{"error.internal", "config.color_invalid", "registry.invalid"}
	got := catalog.IDs()
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("IDs = %v, want %v", got, want)
		}
	}
	if template, ok := catalog.Template("registry.invalid"); !ok ||
		!strings.Contains(template, "{reason}") {
		t.Errorf("Template(registry.invalid) = %q, %t", template, ok)
	}
	if _, ok := catalog.Template("no.such_message"); ok {
		t.Error("存在しないIDでtemplateが返った")
	}
}

// TestMessageCatalogPlaceholders はplaceholderの抽出を固定する。
func TestMessageCatalogPlaceholders(t *testing.T) {
	source := `a.one = "{x} と {y} と {x}"
a.two = "placeholderなし"
a.three = "literal brace {{ }} だけ"
`
	catalog, err := ParseMessageCatalog([]byte(source))
	if err != nil {
		t.Fatalf("ParseMessageCatalog = %v", err.Cause)
	}
	cases := []struct {
		id   string
		want []string
	}{
		// 出現順・重複なし。
		{"a.one", []string{"x", "y"}},
		{"a.two", nil},
		// `{{`/`}}`はliteral braceでありplaceholderではない（§20）。
		{"a.three", nil},
	}
	for _, c := range cases {
		t.Run(c.id, func(t *testing.T) {
			got, ok := catalog.Placeholders(c.id)
			if !ok {
				t.Fatal("Placeholdersがfalseを返した")
			}
			if len(got) != len(c.want) {
				t.Fatalf("= %v, want %v", got, c.want)
			}
			for index := range c.want {
				if got[index] != c.want[index] {
					t.Fatalf("= %v, want %v", got, c.want)
				}
			}
		})
	}
	if _, ok := catalog.Placeholders("no.such_message"); ok {
		t.Error("存在しないIDでplaceholderが返った")
	}
}

// TestParseMessageCatalogRejects は§20の契約違反を拒否することを固定する。
func TestParseMessageCatalogRejects(t *testing.T) {
	cases := []struct{ name, source string }{
		// keyは§7のmessage ID grammarに従う。
		{"segmentが1件", `internal = "x"`},
		{"大文字", `Error.internal = "x"`},
		{"hyphen", `error-internal.x = "x"`},
		{"先頭が数字", `1error.internal = "x"`},
		{"連続dot", `error..internal = "x"`},
		{"末尾dot", `error.internal. = "x"`},
		// 値の契約。
		{"templateが空", `error.internal = ""`},
		{"key重複", "error.internal = \"x\"\nerror.internal = \"y\"\n"},
		// placeholderの構文。壊れたtemplateを素通しすると、render時に
		// placeholderがそのまま利用者へ出る。
		{"閉じない波括弧", `error.internal = "{value"`},
		{"空placeholder", `error.internal = "{}"`},
		{"対応しない閉じ括弧", `error.internal = "value}"`},
		{"入れ子placeholder", `error.internal = "{a{b}}"`},
		// placeholder名は§7のscalar parameter key grammarに従う。
		{"placeholderが大文字", `error.internal = "{Value}"`},
		{"placeholderがhyphen", `error.internal = "{a-b}"`},
		{"placeholderが数字始まり", `error.internal = "{1a}"`},
		// 秘密値展開を禁じる（§20、docs/10-security.md §9.2）。
		{"secret suffix token", `error.internal = "{github_token}"`},
		{"secret suffix password", `error.internal = "{db_password}"`},
		{"secret suffix secret", `error.internal = "{client_secret}"`},
		{"secret suffix key", `error.internal = "{api_key}"`},
		{"secret header", `error.internal = "{cookie}"`},
		{"secret header authorization", `error.internal = "{authorization}"`},
		// ANSI escapeとterminal controlを禁じる（§20）。TOML basic stringは生の
		// 制御文字を受けないため、TOML側のescapeで書いたESCを対象にする。
		{"ANSI escape", `error.internal = "\u001B[31m赤\u001B[0m"`},
		{"生のESC", "error.internal = \"[31m\""},
		{"改行", "error.internal = \"1行目\\n2行目\""},
		{"tab", "error.internal = \"a\\tb\""},
		{"BEL", "error.internal = \"a\\u0007b\""},
		{"DEL", "error.internal = \"a\\u007fb\""},
		{"format制御文字", "error.internal = \"a\\u200eb\""},
		// multi-line literal stringは改行を含むため通らない。行走査が本文中の
		// `<key> = ...` を2件目の宣言と数えないことも同時に固定する。
		{"multi-line literal", "error.internal = '''\nerror.internal = 本文\n'''\n"},
		// TOMLとして壊れている。
		{"閉じない文字列", `error.internal = "x`},
		{"値が整数", `error.internal = 1`},
		{"値が真偽値", `error.internal = true`},
		{"値がarray", `error.internal = ["x"]`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			catalog, err := ParseMessageCatalog([]byte(c.source + "\n"))
			if err == nil {
				t.Fatalf("%d件のcatalogとして通った", catalog.Len())
			}
			if err.Code != domain.CodeRegistryInvalid {
				t.Fatalf("code = %s, want %s", err.Code, domain.CodeRegistryInvalid)
			}
		})
	}
}

// TestParseMessageCatalogRejectsTableSyntax はtable記法とdotted keyのtableを
// 拒否することを固定する。
//
// §20はcatalogを「ASCII dotted key集合」と定める。table記法を許すと同じmessage
// IDが複数の書き方を持ち、宣言順の走査とdecode結果が食い違う。
func TestParseMessageCatalogRejectsTableSyntax(t *testing.T) {
	sources := []string{
		"[error]\ninternal = \"x\"\n",
		"[config]\ncolor_invalid = \"x\"\n",
	}
	for _, source := range sources {
		if catalog, err := ParseMessageCatalog([]byte(source)); err == nil {
			t.Fatalf("table記法が%d件のcatalogとして通った: %q", catalog.Len(), source)
		}
	}
}

// TestParseMessageCatalogSizeLimits は§20の上限を固定する。
func TestParseMessageCatalogSizeLimits(t *testing.T) {
	t.Run("1 messageの上限", func(t *testing.T) {
		// 8 KiBちょうどは通す。
		atLimit := "error.internal = \"" + strings.Repeat("a", MessageMaxBytes) + "\"\n"
		if _, err := ParseMessageCatalog([]byte(atLimit)); err != nil {
			t.Fatalf("上限ちょうどが拒否された: %v", err.Cause)
		}
		over := "error.internal = \"" + strings.Repeat("a", MessageMaxBytes+1) + "\"\n"
		if _, err := ParseMessageCatalog([]byte(over)); err == nil {
			t.Fatal("1 messageの上限超過が通った")
		} else if !strings.Contains(err.Cause.Error(), "byteを超える") {
			t.Fatalf("size超過として拒否されていない: %v", err.Cause)
		}
	})

	t.Run("file全体の上限", func(t *testing.T) {
		oversized := make([]byte, MessageFileMaxBytes+1)
		for index := range oversized {
			oversized[index] = '\n'
		}
		if _, err := ParseMessageCatalog(oversized); err == nil {
			t.Fatal("file全体の上限超過が通った")
		} else if !strings.Contains(err.Cause.Error(), "byteを超える") {
			t.Fatalf("size超過として拒否されていない: %v", err.Cause)
		}
	})
}

// TestParseMessageCatalogRejectsInvalidUTF8 は不正なUTF-8を拒否することを固定
// する（§20「値はUTF-8 template string」）。
func TestParseMessageCatalogRejectsInvalidUTF8(t *testing.T) {
	source := append([]byte("error.internal = \""), 0xFF, 0xFE)
	source = append(source, []byte("\"\n")...)
	if _, err := ParseMessageCatalog(source); err == nil {
		t.Fatal("不正なUTF-8が通った")
	}
}

// TestRepositoryMessageCatalog はrepositoryのcatalogが§20の契約を満たすことを
// 固定する。
func TestRepositoryMessageCatalog(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(registryDir, MessageCatalogPath))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	catalog, parseErr := ParseMessageCatalog(data)
	if parseErr != nil {
		t.Fatalf("ParseMessageCatalog = %v", parseErr.Cause)
	}
	if catalog.Len() != MessageCount {
		t.Fatalf("message数 = %d, want %d。message IDを増減させたら"+
			"MessageCountとscripts/ci/check_messages.pyの両方を確認する",
			catalog.Len(), MessageCount)
	}

	// 先頭segmentは分類として使う（§7）。分類を増やすと表示側の扱いも増えるため、
	// 集合を固定する。
	prefixes := make(map[string]int)
	for _, id := range catalog.IDs() {
		prefixes[strings.SplitN(id, ".", 2)[0]]++
	}
	wantPrefixes := []string{
		"catalog", "config", "definition", "error", "license", "plan", "provider",
		"registry"}
	got := make([]string, 0, len(prefixes))
	for prefix := range prefixes {
		got = append(got, prefix)
	}
	sort.Strings(got)
	if len(got) != len(wantPrefixes) {
		t.Fatalf("先頭segment = %v, want %v", got, wantPrefixes)
	}
	for index := range wantPrefixes {
		if got[index] != wantPrefixes[index] {
			t.Fatalf("先頭segment = %v, want %v", got, wantPrefixes)
		}
	}
}

// TestRepositoryMessageCatalogIsSorted はcatalogが分類ごとにID順で並ぶことを
// 固定する。
//
// 並びを契約にするのは、messageを追加する場所が一意に決まり、diffが挿入位置で
// 揺れないようにするためである。
func TestRepositoryMessageCatalogIsSorted(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(registryDir, MessageCatalogPath))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	catalog, parseErr := ParseMessageCatalog(data)
	if parseErr != nil {
		t.Fatalf("ParseMessageCatalog = %v", parseErr.Cause)
	}
	ids := catalog.IDs()

	// 分類の出現順と、分類内のID昇順を見る。`definition.invalid`だけは
	// typed errorの本体で、残りはその診断1件ごとの理由codeなので先頭へ置く。
	var previousPrefix string
	var previousID string
	seenPrefixes := make(map[string]struct{})
	for _, id := range ids {
		prefix := strings.SplitN(id, ".", 2)[0]
		if prefix != previousPrefix {
			if _, repeated := seenPrefixes[prefix]; repeated {
				t.Fatalf("分類 %q が離れた位置に再出現している（%s）", prefix, id)
			}
			seenPrefixes[prefix] = struct{}{}
			previousPrefix, previousID = prefix, id
			continue
		}
		if id == "definition.invalid" || previousID == "definition.invalid" {
			previousID = id
			continue
		}
		if id < previousID {
			t.Errorf("分類 %q 内がID順でない（%q の後に %q）", prefix, previousID, id)
		}
		previousID = id
	}
}
