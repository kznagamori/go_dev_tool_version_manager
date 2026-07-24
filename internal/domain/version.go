package domain

import (
	"fmt"
	"strings"
	"unicode"
)

// Version は tool の完全バージョンを表す値型である。catalog に保存された正規
// 完全版に対応し、入力一致は比較用キーではなく本文字列のバイト単位完全一致で
// 行う（02章3節、08章2.1節）。scheme 依存の比較用キーは catalog/definition 層が
// 別途算出する。
type Version struct {
	value string
}

// NewVersion は文字列を検証して Version を生成する。空、前後空白付き（04章4節で
// trim 後に拒否）、制御文字・NUL を含む場合は ErrInvalidVersion を包んで返す。
// 版比較上の妥当性（scheme 適合）は definition/catalog 層で検査する。
func NewVersion(s string) (Version, error) {
	if s == "" {
		return Version{}, fmt.Errorf("%w: 空文字は完全版ではない", ErrInvalidVersion)
	}
	if strings.TrimSpace(s) != s {
		return Version{}, fmt.Errorf("%w: %q: 前後の空白は許可しない", ErrInvalidVersion, s)
	}
	for _, r := range s {
		if r == 0 || unicode.IsControl(r) {
			return Version{}, fmt.Errorf("%w: %q: 制御文字を含む", ErrInvalidVersion, s)
		}
	}
	return Version{value: s}, nil
}

// String は完全バージョン文字列を返す。
func (v Version) String() string { return v.value }

// IsZero はゼロ値（未設定）の Version かどうかを返す。
func (v Version) IsZero() bool { return v.value == "" }
