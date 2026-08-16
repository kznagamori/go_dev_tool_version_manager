package catalog

import (
	"fmt"
	"strconv"
	"strings"
)

// resolvePointer はRFC 6901のJSON pointerを解決する（docs/06-tool-definition.md §6.1）。
//
// 空文字は文書全体を指し、`/`はkeyが空文字のmemberを指す。**配列かobjectかで
// `/`をrootへ読み替える等の代替解釈をしない**（同§）。読み替えを許すと、同じ
// definitionが上流文書の形の変化で黙って別のnodeを指すようになり、source error
// で気付けなくなる。
//
// 解決できない場合はerrorを返す。呼出し側がsource errorへ変換する。欠落を
// 「値なし」として黙って通さないのは、§6.1が欠落と型違いをsource errorと定める
// ためである。
func resolvePointer(root any, pointer string) (any, error) {
	if pointer == "" {
		return root, nil
	}
	if !strings.HasPrefix(pointer, "/") {
		// definitionのschema検証（internal/definition）で弾いている形である。
		// evaluationでも同じ判定を持つのは、pointerがdefinition以外から渡る
		// 経路を作らないためである。
		return nil, fmt.Errorf("pointer %q が`/`で始まらない", pointer)
	}
	current := root
	// 先頭の`/`を落としてから分割する。`/a/b`は["a","b"]、`/`は[""]になる。
	for index, token := range strings.Split(pointer[1:], "/") {
		key := unescapeToken(token)
		next, err := resolveToken(current, key)
		if err != nil {
			return nil, fmt.Errorf("pointer %q の第%d要素 %q: %w", pointer, index+1, key, err)
		}
		current = next
	}
	return current, nil
}

// unescapeToken はRFC 6901のescapeを戻す。
//
// `~1`を`/`へ戻してから`~0`を`~`へ戻す。順序を逆にすると`~01`が`/`になり、
// 本来の`~1`と区別できなくなる。
func unescapeToken(token string) string {
	return strings.ReplaceAll(strings.ReplaceAll(token, "~1", "/"), "~0", "~")
}

func resolveToken(current any, key string) (any, error) {
	switch node := current.(type) {
	case map[string]any:
		value, found := node[key]
		if !found {
			return nil, fmt.Errorf("objectにkeyが無い")
		}
		return value, nil
	case []any:
		index, err := arrayIndex(key)
		if err != nil {
			return nil, err
		}
		if index >= len(node) {
			return nil, fmt.Errorf("配列の範囲外（要素数%d）", len(node))
		}
		return node[index], nil
	default:
		return nil, fmt.Errorf("objectでも配列でもないnodeを辿れない")
	}
}

// arrayIndex はRFC 6901の配列indexを読む。
//
// `0`または先頭が0でない10進数だけを受ける。`-`（末尾の次の要素）は既存の
// 要素を指さないため解決できない。leading zeroや符号付きを受けると、同じ要素を
// 複数の表記で指せてしまう。
func arrayIndex(key string) (int, error) {
	if key == "-" {
		return 0, fmt.Errorf("`-`は既存要素を指さない")
	}
	if key == "" {
		return 0, fmt.Errorf("配列indexが空")
	}
	if key != "0" && key[0] == '0' {
		return 0, fmt.Errorf("配列indexにleading zeroがある")
	}
	for i := 0; i < len(key); i++ {
		if key[i] < '0' || key[i] > '9' {
			return 0, fmt.Errorf("配列indexが10進数でない")
		}
	}
	index, err := strconv.Atoi(key)
	if err != nil {
		return 0, fmt.Errorf("配列indexが範囲を超える")
	}
	return index, nil
}

// pointerArray はpointer先を配列として解決する。
func pointerArray(root any, pointer string) ([]any, error) {
	node, err := resolvePointer(root, pointer)
	if err != nil {
		return nil, err
	}
	values, ok := node.([]any)
	if !ok {
		return nil, fmt.Errorf("pointer %q の先が配列でない（%s）", pointer, jsonKind(node))
	}
	return values, nil
}

// pointerString はpointer先をstringとして解決する。
func pointerString(root any, pointer string) (string, error) {
	node, err := resolvePointer(root, pointer)
	if err != nil {
		return "", err
	}
	text, ok := node.(string)
	if !ok {
		return "", fmt.Errorf("pointer %q の先がstringでない（%s）", pointer, jsonKind(node))
	}
	return text, nil
}
