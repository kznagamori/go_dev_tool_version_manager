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
// docs/06-tool-definition.md §3の「tool ID/alias `^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$`、
// 1～64 byte」が正本である。大文字、underscore、連続hyphen、先頭末尾hyphen、
// 先頭の数字を拒否する。
var toolIDRe = regexp.MustCompile(`^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$`)

// ToolIDMaxBytes はtool IDの上限である（docs/06-tool-definition.md §3）。
//
// grammarがASCIIへ限るためbyte数とrune数は一致する。
const ToolIDMaxBytes = 64

// ParseToolID は文字列をToolIDへ変換する。
//
// registryへ登録されたdefinitionだけがtool IDを持ち込むため、definition schema
// の§3 grammarをdomain値の不変条件としてそのまま使う。CLI入力やstateから読んだ
// 値もここを通り、definitionに存在しえない形のIDが型として作れないようにする。
func ParseToolID(text string) (ToolID, error) {
	if len(text) > ToolIDMaxBytes {
		return ToolID{}, fmt.Errorf(
			"domain: tool id %q が%d byteを超える（%d byte）", text, ToolIDMaxBytes, len(text))
	}
	if !toolIDRe.MatchString(text) {
		return ToolID{}, fmt.Errorf(
			"domain: tool id %q がkebab-case（小文字英字で始まり英数字を単一hyphenで連結）に合わない", text)
	}
	return ToolID{value: text}, nil
}

// String は正規IDを返す。
func (t ToolID) String() string { return t.value }

// IsZero はParseToolIDを通していない値かどうかを返す。
func (t ToolID) IsZero() bool { return t.value == "" }
