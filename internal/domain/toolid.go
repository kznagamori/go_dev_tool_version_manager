package domain

import (
	"fmt"
	"regexp"
)

// ToolID は正規化済みのtool識別子である（docs/02-architecture.md §3）。
//
// aliasではなく正規IDだけを保持する。aliasの解決はregistry側の責務であり、
// 解決済みかどうかを型で区別できないと、alias混じりのIDがreceiptやstateへ
// 書かれてしまう。
type ToolID struct {
	value string
}

// toolIDRe はkebab-case grammarである。
//
// docs/02-architecture.md §3が「正規化済みkebab-case」と定め、
// docs/06-tool-definition.md §3が`id`をdefinition fileのbasenameと一致させる。
// 大文字、underscore、連続hyphen、先頭末尾hyphenを拒否する。
// 長さ上限は仕様が定めていないため設けない。
var toolIDRe = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// ParseToolID は文字列をToolIDへ変換する。
func ParseToolID(text string) (ToolID, error) {
	if !toolIDRe.MatchString(text) {
		return ToolID{}, fmt.Errorf(
			"domain: tool id %q がkebab-case（小文字英数字を単一hyphenで連結）に合わない", text)
	}
	return ToolID{value: text}, nil
}

// String は正規IDを返す。
func (t ToolID) String() string { return t.value }

// IsZero はParseToolIDを通していない値かどうかを返す。
func (t ToolID) IsZero() bool { return t.value == "" }
