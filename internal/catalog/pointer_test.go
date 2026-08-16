package catalog

import (
	"testing"
)

// TestResolvePointerFollowsRFC6901 はdocs/06-tool-definition.md §6.1の
// 「pointerはすべてRFC 6901」を境界で固定する。
func TestResolvePointerFollowsRFC6901(t *testing.T) {
	// RFC 6901 §5の例をそのまま使う。
	root := mustDecode(t, `{
		"foo": ["bar", "baz"],
		"": 0,
		"a/b": 1,
		"c%d": 2,
		"e^f": 3,
		"g|h": 4,
		"i\\j": 5,
		"k\"l": 6,
		" ": 7,
		"m~n": 8
	}`)
	cases := []struct {
		pointer string
		want    string
	}{
		// 空文字は文書全体を指す。
		{"", "object"},
		{"/foo", "array"},
		{"/foo/0", "bar"},
		// `/`はkeyが空文字のmemberを指す。文書全体ではない。
		{"/", "0"},
		{"/a~1b", "1"},
		{"/c%d", "2"},
		{"/e^f", "3"},
		{"/g|h", "4"},
		{"/i\\j", "5"},
		{"/k\"l", "6"},
		{"/ ", "7"},
		{"/m~0n", "8"},
	}
	for _, c := range cases {
		node, err := resolvePointer(root, c.pointer)
		if err != nil {
			t.Errorf("resolvePointer(%q) = %v, want nil", c.pointer, err)
			continue
		}
		if got := describeNode(node); got != c.want {
			t.Errorf("resolvePointer(%q) = %s, want %s", c.pointer, got, c.want)
		}
	}
}

// TestResolvePointerRejectsRootAlias は`/`を文書全体へ読み替えないことを固定する。
//
// §6.1が「配列やobjectの型に応じて`/`をrootへ読み替える等の代替解釈を行わない」と
// 定める。読み替えを許すと、同じdefinitionが上流文書の形の変化で黙って別のnodeを
// 指すようになり、source errorで気付けない。
func TestResolvePointerRejectsRootAlias(t *testing.T) {
	array := mustDecode(t, `[{"version":"v1.0.0"}]`)

	// top-levelが配列の文書のitemsは空文字で指す（Node.jsとGoが該当する）。
	node, err := resolvePointer(array, "")
	if err != nil {
		t.Fatalf(`resolvePointer("") = %v, want nil`, err)
	}
	if describeNode(node) != "array" {
		t.Fatalf(`resolvePointer("") = %s, want array`, describeNode(node))
	}

	// 同じ文書へ`/`を当てると、keyが空文字のmemberが無いので解決できない。
	if _, err := resolvePointer(array, "/"); err == nil {
		t.Fatal(`resolvePointer("/") が配列に対して成功した`)
	}
}

// TestResolvePointerRejectsBadTokens は解決できないtokenを固定する。
func TestResolvePointerRejectsBadTokens(t *testing.T) {
	root := mustDecode(t, `{"list":[10,20],"obj":{"k":"v"},"text":"scalar"}`)
	cases := []struct {
		pointer string
		why     string
	}{
		{"list/0", "先頭が`/`でない"},
		{"/missing", "objectにkeyが無い"},
		{"/list/2", "配列の範囲外"},
		{"/list/-", "`-`は既存要素を指さない"},
		{"/list/01", "配列indexのleading zero"},
		{"/list/+1", "符号付きの配列index"},
		{"/list/x", "配列indexが数値でない"},
		{"/list/", "配列indexが空"},
		{"/text/k", "scalarを辿ろうとした"},
		{"/obj/k/deeper", "stringを辿ろうとした"},
	}
	for _, c := range cases {
		if _, err := resolvePointer(root, c.pointer); err == nil {
			t.Errorf("resolvePointer(%q) が成功した（%s）", c.pointer, c.why)
		}
	}
}

// TestUnescapeTokenOrder はescape解除の順序を固定する。
//
// `~1`を`/`へ戻してから`~0`を`~`へ戻す。逆順にすると`~01`が`/`になり、本来の
// `~1`（`/`）と区別できなくなる。
func TestUnescapeTokenOrder(t *testing.T) {
	cases := map[string]string{
		"~0":   "~",
		"~1":   "/",
		"~01":  "~1",
		"~10":  "/0",
		"a~1b": "a/b",
		"m~0n": "m~n",
		"~0~1": "~/",
	}
	for input, want := range cases {
		if got := unescapeToken(input); got != want {
			t.Errorf("unescapeToken(%q) = %q, want %q", input, got, want)
		}
	}
}

// TestPointerArrayAndString は型を要求する解決を固定する。
func TestPointerArrayAndString(t *testing.T) {
	root := mustDecode(t, `{"list":[1],"text":"v","num":1}`)

	if _, err := pointerArray(root, "/list"); err != nil {
		t.Errorf("pointerArray(/list) = %v, want nil", err)
	}
	if _, err := pointerArray(root, "/text"); err == nil {
		t.Error("pointerArray(/text) が成功した（配列でない）")
	}
	if got, err := pointerString(root, "/text"); err != nil || got != "v" {
		t.Errorf("pointerString(/text) = %q, %v", got, err)
	}
	// 数値をstringへ暗黙変換しない。上流が型を変えたらsource errorにする（§6.1）。
	if _, err := pointerString(root, "/num"); err == nil {
		t.Error("pointerString(/num) が成功した（stringでない）")
	}
}
