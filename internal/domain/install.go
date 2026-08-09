package domain

import "fmt"

// InstallKey は導入物の同一性を決める組である（docs/02-architecture.md §3）。
//
// ToolID、Version、Platformの3つで一意になる。同じtoolの同じversionでも
// platformが違えば別の導入物であり、1つでも欠けた状態を作れないようにする。
type InstallKey struct {
	tool     ToolID
	version  Version
	platform Platform
}

// NewInstallKey は3要素からInstallKeyを作る。いずれかが未初期化ならerrorにする。
func NewInstallKey(tool ToolID, version Version, platform Platform) (InstallKey, error) {
	switch {
	case tool.IsZero():
		return InstallKey{}, fmt.Errorf("domain: InstallKeyのtool idが未設定")
	case version.IsZero():
		return InstallKey{}, fmt.Errorf("domain: InstallKeyのversionが未設定")
	case platform.IsZero():
		return InstallKey{}, fmt.Errorf("domain: InstallKeyのplatformが未設定")
	}
	return InstallKey{tool: tool, version: version, platform: platform}, nil
}

// Tool はtool IDを返す。
func (k InstallKey) Tool() ToolID { return k.tool }

// Version はversionを返す。
func (k InstallKey) Version() Version { return k.version }

// Platform はplatformを返す。
func (k InstallKey) Platform() Platform { return k.platform }

// String は`<tool>@<version>/<platform>`形式の識別子を返す。
//
// log、diagnostic、error messageで導入物を一意に指すために使う。
// path構成へ流用しない。path規則はplatform層が決める。
func (k InstallKey) String() string {
	return k.tool.String() + "@" + k.version.String() + "/" + k.platform.ID()
}

// Equal は3要素すべてが一致するかどうかを返す。
//
// versionはcatalogの正規文字列のbyte完全一致で判定する。comparison keyでの
// 近似一致をしない（docs/06-tool-definition.md §4）。
func (k InstallKey) Equal(other InstallKey) bool {
	return k.tool == other.tool &&
		k.version.scheme == other.version.scheme &&
		k.version.text == other.version.text &&
		k.platform.id == other.platform.id
}

// IsZero はNewInstallKeyを通していない値かどうかを返す。
func (k InstallKey) IsZero() bool { return k.tool.IsZero() }

// EffectiveSelection は解決済みの選択である（docs/02-architecture.md §3）。
//
// 選択値、由来、由来設定file、導入状態を1つにまとめる。由来を持たない選択値や、
// 選択値を持たない由来といった中途半端な状態を作れないようにする。
type EffectiveSelection struct {
	tool    ToolID
	version Version
	source  SelectionSource
	// origin は選択の由来となった設定fileである。
	// source=`none`のときはroleだけを持つ空pathになりうる。
	origin PathValue
	health Health
}

// NewEffectiveSelection は選択が存在する場合の値を作る。
//
// sourceには`project`または`user`だけを渡す。選択が無い場合はNoSelectionを使う。
func NewEffectiveSelection(
	tool ToolID, version Version, source SelectionSource, origin PathValue, health Health,
) (EffectiveSelection, error) {
	switch {
	case tool.IsZero():
		return EffectiveSelection{}, fmt.Errorf("domain: selectionのtool idが未設定")
	case version.IsZero():
		return EffectiveSelection{}, fmt.Errorf("domain: selectionのversionが未設定")
	case source != SelectionSourceProject && source != SelectionSourceUser:
		return EffectiveSelection{}, fmt.Errorf(
			"domain: 選択ありのsourceは project|user だけである（%q が渡された）", source)
	case origin.IsZero():
		return EffectiveSelection{}, fmt.Errorf("domain: selectionのorigin path roleが未設定")
	}
	if _, err := ParseHealth(string(health)); err != nil {
		return EffectiveSelection{}, err
	}
	return EffectiveSelection{
		tool: tool, version: version, source: source, origin: origin, health: health,
	}, nil
}

// NoSelection はtoolに有効な選択が無い状態を返す。
func NoSelection(tool ToolID) (EffectiveSelection, error) {
	if tool.IsZero() {
		return EffectiveSelection{}, fmt.Errorf("domain: selectionのtool idが未設定")
	}
	return EffectiveSelection{tool: tool, source: SelectionSourceNone, health: HealthUnknown}, nil
}

// Tool はtool IDを返す。
func (s EffectiveSelection) Tool() ToolID { return s.tool }

// Version は選択されたversionを返す。選択が無い場合は未初期化のVersionである。
func (s EffectiveSelection) Version() Version { return s.version }

// Source は選択の由来を返す。
func (s EffectiveSelection) Source() SelectionSource { return s.source }

// Origin は由来となった設定fileを返す。
func (s EffectiveSelection) Origin() PathValue { return s.origin }

// Health は導入状態を返す。
func (s EffectiveSelection) Health() Health { return s.health }

// HasSelection は有効な選択があるかどうかを返す。
func (s EffectiveSelection) HasSelection() bool { return s.source != SelectionSourceNone }
