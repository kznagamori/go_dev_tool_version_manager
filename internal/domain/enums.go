package domain

import "fmt"

// Mode は導入形態である（docs/02-architecture.md §3、docs/04-storage-and-data.md §17.1）。
type Mode string

// Mode の値。§17.1で閉じており、未定義値を受理しない。
const (
	ModePortable Mode = "portable"
	ModeUser     Mode = "user"
)

// ParseMode は文字列をModeへ変換する。
func ParseMode(text string) (Mode, error) {
	switch Mode(text) {
	case ModePortable, ModeUser:
		return Mode(text), nil
	}
	return "", fmt.Errorf("domain: mode %q は portable|user のいずれでもない", text)
}

// Scope は選択の適用範囲である（docs/02-architecture.md §3）。
type Scope string

// Scope の値。
const (
	ScopeUser    Scope = "user"
	ScopeProject Scope = "project"
)

// ParseScope は文字列をScopeへ変換する。
func ParseScope(text string) (Scope, error) {
	switch Scope(text) {
	case ScopeUser, ScopeProject:
		return Scope(text), nil
	}
	return "", fmt.Errorf("domain: scope %q は user|project のいずれでもない", text)
}

// Channel はversionの公開channelである（docs/04-storage-and-data.md §17.1）。
type Channel string

// Channel の値。対象toolがないoperationでは空を使うため、空も正当な状態として扱う。
const (
	ChannelStable     Channel = "stable"
	ChannelPrerelease Channel = "prerelease"
	// ChannelNone は対象toolがないoperationで使う空値である。
	ChannelNone Channel = ""
)

// ParseChannel は文字列をChannelへ変換する。空は対象toolなしを表す。
func ParseChannel(text string) (Channel, error) {
	switch Channel(text) {
	case ChannelStable, ChannelPrerelease, ChannelNone:
		return Channel(text), nil
	}
	return "", fmt.Errorf("domain: channel %q は stable|prerelease または空のいずれでもない", text)
}

// Lifecycle はupstreamのsupport状態である（docs/04-storage-and-data.md §17.1）。
type Lifecycle string

// Lifecycle の値。
const (
	LifecycleSupported Lifecycle = "supported"
	LifecycleEOL       Lifecycle = "eol"
	// LifecycleUnknown はupstreamが状態を示さない場合である。
	// このenumは`unknown`を持つため「不明」を表現できる（§17.1）。
	LifecycleUnknown Lifecycle = "unknown"
	// LifecycleNone は対象toolがないoperationで使う空値である。
	LifecycleNone Lifecycle = ""
)

// ParseLifecycle は文字列をLifecycleへ変換する。
//
// mapに無い値を黙って`unknown`へ倒さない。upstreamがphaseを増やした場合に
// 誤った状態を表示せずsource errorにするためである
// （docs/07-registry-and-tools.md）。
func ParseLifecycle(text string) (Lifecycle, error) {
	switch Lifecycle(text) {
	case LifecycleSupported, LifecycleEOL, LifecycleUnknown, LifecycleNone:
		return Lifecycle(text), nil
	}
	return "", fmt.Errorf("domain: lifecycle %q は supported|eol|unknown または空のいずれでもない", text)
}

// SelectionSource は有効な選択の由来である（docs/04-storage-and-data.md §17.1の`source`）。
type SelectionSource string

// SelectionSource の値。
const (
	SelectionSourceProject SelectionSource = "project"
	SelectionSourceUser    SelectionSource = "user"
	SelectionSourceNone    SelectionSource = "none"
)

// ParseSelectionSource は文字列をSelectionSourceへ変換する。
func ParseSelectionSource(text string) (SelectionSource, error) {
	switch SelectionSource(text) {
	case SelectionSourceProject, SelectionSourceUser, SelectionSourceNone:
		return SelectionSource(text), nil
	}
	return "", fmt.Errorf("domain: source %q は project|user|none のいずれでもない", text)
}

// Health は導入物の健全性である（docs/04-storage-and-data.md §17.1の`health`）。
type Health string

// Health の値。
const (
	HealthHealthy   Health = "healthy"
	HealthUnhealthy Health = "unhealthy"
	HealthUnknown   Health = "unknown"
)

// ParseHealth は文字列をHealthへ変換する。
func ParseHealth(text string) (Health, error) {
	switch Health(text) {
	case HealthHealthy, HealthUnhealthy, HealthUnknown:
		return Health(text), nil
	}
	return "", fmt.Errorf("domain: health %q は healthy|unhealthy|unknown のいずれでもない", text)
}
