package i18n

import (
	"strings"
	"testing"
)

func TestBaseCatalogs_LoadAndParity(t *testing.T) {
	ja, err := BaseCatalog("ja")
	if err != nil {
		t.Fatalf("BaseCatalog(ja): %v", err)
	}
	en, err := BaseCatalog("en")
	if err != nil {
		t.Fatalf("BaseCatalog(en): %v", err)
	}
	if ja.Language() != "ja" || en.Language() != "en" {
		t.Fatalf("language 不一致: %q, %q", ja.Language(), en.Language())
	}
	// 13章12節: ja/en の message ID と placeholder を 1 対 1 に揃える。
	if err := ValidateParity(ja, en); err != nil {
		t.Fatalf("同梱 ja/en catalog の parity 違反: %v", err)
	}
	if len(ja.IDs()) == 0 {
		t.Fatal("同梱 catalog が空")
	}
}

func TestBaseCatalog_UnknownLanguage(t *testing.T) {
	if _, err := BaseCatalog("fr"); err == nil {
		t.Fatal("未対応 language はエラーになるべき")
	}
}

func TestRender(t *testing.T) {
	ja, _ := BaseCatalog("ja")
	got, ok := ja.Render("error.tool_unknown", map[string]string{"tool_id": "node"})
	if !ok {
		t.Fatal("error.tool_unknown が見つからない")
	}
	if !strings.Contains(got, "node") {
		t.Errorf("placeholder 置換されていない: %q", got)
	}
	if strings.Contains(got, "{tool_id}") {
		t.Errorf("placeholder が残っている: %q", got)
	}
}

func TestRender_MissingArgKeepsPlaceholder(t *testing.T) {
	ja, _ := BaseCatalog("ja")
	got, ok := ja.Render("error.tool_unknown", nil)
	if !ok {
		t.Fatal("message が見つからない")
	}
	if !strings.Contains(got, "{tool_id}") {
		t.Errorf("arg 欠落時は placeholder を残すべき: %q", got)
	}
}

func TestRender_UnknownID(t *testing.T) {
	ja, _ := BaseCatalog("ja")
	if _, ok := ja.Render("no.such.message", nil); ok {
		t.Error("未知 message ID は ok=false であるべき")
	}
}

func TestLoad_Strict(t *testing.T) {
	cases := map[string]string{
		"unknown top-level key": `schema = 1
language = "en"
extra = "x"
[messages]
"a.b" = "hi"`,
		"bad schema": `schema = 2
language = "en"
[messages]`,
		"bad language": `schema = 1
language = "fr"
[messages]`,
		"invalid message id": `schema = 1
language = "en"
[messages]
"Bad.ID" = "hi"`,
		"malformed brace": `schema = 1
language = "en"
[messages]
"a.b" = "hello {world"`,
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Load([]byte(src)); err == nil {
				t.Fatalf("%s: Load はエラーになるべき", name)
			}
		})
	}
}

func TestLoad_Valid(t *testing.T) {
	src := `schema = 1
language = "en"
[messages]
"error.sample" = "value is {value}"`
	c, err := Load([]byte(src))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	got, _ := c.Render("error.sample", map[string]string{"value": "42"})
	if got != "value is 42" {
		t.Errorf("Render = %q", got)
	}
}

func TestValidateParity_Mismatch(t *testing.T) {
	ja, _ := Load([]byte(`schema = 1
language = "ja"
[messages]
"a.b" = "{x} と {y}"
"only.ja" = "日本語のみ"`))
	en, _ := Load([]byte(`schema = 1
language = "en"
[messages]
"a.b" = "{x} only"`))
	err := ValidateParity(ja, en)
	if err == nil {
		t.Fatal("ID と placeholder の不一致を検出すべき")
	}
	// "only.ja" の欠落と "a.b" の placeholder 不一致（{y}）を報告する。
	if !strings.Contains(err.Error(), "only.ja") {
		t.Errorf("欠落 ID を報告すべき: %v", err)
	}
	if !strings.Contains(err.Error(), "a.b") {
		t.Errorf("placeholder 不一致を報告すべき: %v", err)
	}
}
