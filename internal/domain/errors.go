package domain

import (
	"errors"
	"fmt"
)

// wrapf は sentinel error を先頭に置いた %w 付きエラーを生成する共通ヘルパーである。
func wrapf(sentinel error, format string, args ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{sentinel}, args...)...)
}

// value 型コンストラクタが不変条件違反時に包んで返す sentinel error。
// 呼び出し側は errors.Is で種別を判定でき、Application Service 境界では
// 文脈に応じた CoreError（coreerror.go）へ変換する。
var (
	// ErrInvalidToolID は tool ID が 01章6節の形式規則を満たさないことを表す。
	ErrInvalidToolID = errors.New("invalid tool id")
	// ErrInvalidVersion は version 文字列が完全版として不正であることを表す。
	ErrInvalidVersion = errors.New("invalid version")
	// ErrInvalidVariant は variant 文字列が不正であることを表す。
	ErrInvalidVariant = errors.New("invalid variant")
	// ErrInvalidOS は OS enum が未対応値であることを表す。
	ErrInvalidOS = errors.New("invalid os")
	// ErrInvalidArch は arch enum が未対応値であることを表す。
	ErrInvalidArch = errors.New("invalid arch")
	// ErrInvalidLibc は libc enum が未対応値であることを表す。
	ErrInvalidLibc = errors.New("invalid libc")
	// ErrInvalidScope は scope enum が未対応値であることを表す。
	ErrInvalidScope = errors.New("invalid scope")
	// ErrInvalidMode は mode enum が未対応値であることを表す。
	ErrInvalidMode = errors.New("invalid mode")
	// ErrInvalidChannel は channel enum が未対応値であることを表す。
	ErrInvalidChannel = errors.New("invalid channel")
	// ErrInvalidDigest は digest の algorithm または hex 表現が不正であることを表す。
	ErrInvalidDigest = errors.New("invalid digest")
	// ErrInvalidSelection は selection の必須要素が欠落・不整合であることを表す。
	ErrInvalidSelection = errors.New("invalid selection")
)
