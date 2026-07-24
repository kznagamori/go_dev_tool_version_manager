package domain

import (
	"fmt"
	"regexp"
)

// ToolID は正規化済みの tool 識別子（小文字 kebab-case）を表す値型である。
// alias ではなく、registry 内で一意な正規 ID に対応する。alias から正規 ID への
// 解決は registry 層の責務であり、本型は 01章6節の形式規則だけを保証する。
type ToolID struct {
	value string
}

const (
	toolIDMinLen = 2
	toolIDMaxLen = 40
	// reservedProjectKeyDisabled は project schema（05章5節）の予約 key であり、
	// tool ID として使用できない（01章6節）。
	reservedProjectKeyDisabled = "disabled"
)

// toolIDPattern は 01章6節の規則を表す。ASCII 小文字で始まり、ASCII 小文字・
// 数字・単一ハイフンだけを含む。先頭・末尾ハイフンと連続ハイフンを許さない。
var toolIDPattern = regexp.MustCompile(`^[a-z][a-z0-9]*(-[a-z0-9]+)*$`)

// NewToolID は文字列を検証して ToolID を生成する。長さが 2〜40 文字でない、
// 予約語 "disabled" である、または形式規則に反する場合は ErrInvalidToolID を
// 包んで返す。入力は alias でもよいが、alias 解決は行わない。
func NewToolID(s string) (ToolID, error) {
	if len(s) < toolIDMinLen || len(s) > toolIDMaxLen {
		return ToolID{}, fmt.Errorf("%w: %q: 長さは %d〜%d 文字", ErrInvalidToolID, s, toolIDMinLen, toolIDMaxLen)
	}
	if s == reservedProjectKeyDisabled {
		return ToolID{}, fmt.Errorf("%w: %q は予約語のため使用できない", ErrInvalidToolID, s)
	}
	if !toolIDPattern.MatchString(s) {
		return ToolID{}, fmt.Errorf("%w: %q: 小文字で始まる小文字・数字・単一ハイフンのみ", ErrInvalidToolID, s)
	}
	return ToolID{value: s}, nil
}

// String は正規 tool ID 文字列を返す。
func (t ToolID) String() string { return t.value }

// IsZero はゼロ値（未設定）の ToolID かどうかを返す。
func (t ToolID) IsZero() bool { return t.value == "" }
