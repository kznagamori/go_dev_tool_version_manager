package domain

import "fmt"

// Scope は選択の適用範囲を表す enum である（user 既定または project、02章3節）。
type Scope string

// Mode は保存領域方式を表す enum である（03章1節）。
type Mode string

// Channel は version の配布チャネルを表す enum である（02章3節, 06章）。
type Channel string

// Scope enum の値。
const (
	ScopeUser    Scope = "user"
	ScopeProject Scope = "project"
)

// Mode enum の値。
const (
	ModePortable  Mode = "portable"
	ModeUser      Mode = "user"
	ModeMultiUser Mode = "multi-user"
)

// Channel enum の値。既定チャネルは stable であり、--latest は stable のみを対象とする。
const (
	ChannelStable     Channel = "stable"
	ChannelPrerelease Channel = "prerelease"
	ChannelNightly    Channel = "nightly"
	ChannelEOL        Channel = "eol"
)

// NewScope は文字列を検証して Scope を返す。未対応値は ErrInvalidScope を返す。
func NewScope(s string) (Scope, error) {
	switch Scope(s) {
	case ScopeUser, ScopeProject:
		return Scope(s), nil
	default:
		return "", fmt.Errorf("%w: %q", ErrInvalidScope, s)
	}
}

// NewMode は文字列を検証して Mode を返す。未対応値は ErrInvalidMode を返す。
func NewMode(s string) (Mode, error) {
	switch Mode(s) {
	case ModePortable, ModeUser, ModeMultiUser:
		return Mode(s), nil
	default:
		return "", fmt.Errorf("%w: %q", ErrInvalidMode, s)
	}
}

// NewChannel は文字列を検証して Channel を返す。未対応値は ErrInvalidChannel を返す。
func NewChannel(s string) (Channel, error) {
	switch Channel(s) {
	case ChannelStable, ChannelPrerelease, ChannelNightly, ChannelEOL:
		return Channel(s), nil
	default:
		return "", fmt.Errorf("%w: %q", ErrInvalidChannel, s)
	}
}
