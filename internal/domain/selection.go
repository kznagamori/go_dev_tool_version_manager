package domain

import "fmt"

// Origin は有効選択の由来を表す enum である（04章3.10節, 10章4.5節）。
type Origin string

// Origin enum の値。
const (
	// OriginNone は選択が存在しないことを表す。
	OriginNone Origin = "none"
	// OriginUser は user 選択に由来することを表す。
	OriginUser Origin = "user"
	// OriginProject は project 選択に由来することを表す。
	OriginProject Origin = "project"
	// OriginExplicit は CLI/exec の明示指定に由来することを表す。
	OriginExplicit Origin = "explicit"
	// OriginDisabled は project により明示的に無効化されていることを表す。
	OriginDisabled Origin = "disabled"
)

// NewOrigin は文字列を検証して Origin を返す。未対応値は ErrInvalidSelection を返す。
func NewOrigin(s string) (Origin, error) {
	switch Origin(s) {
	case OriginNone, OriginUser, OriginProject, OriginExplicit, OriginDisabled:
		return Origin(s), nil
	default:
		return "", fmt.Errorf("%w: 未対応の origin %q", ErrInvalidSelection, s)
	}
}

// Selection は state に永続化する user 選択の 1 件を表す（03章6節, 14章4節）。
// definition 更新後も同じ導入物を指すよう、version だけでなく variant と install_id を
// 必ず保持する。
type Selection struct {
	toolID    ToolID
	version   Version
	variant   string
	installID InstallID
}

// NewSelection は各必須要素を検証して Selection を生成する。variant は空なら既定値へ
// 正規化する。tool ID・version・install ID のいずれかが未設定なら ErrInvalidSelection
// を返す。
func NewSelection(toolID ToolID, version Version, variant string, installID InstallID) (Selection, error) {
	if toolID.IsZero() {
		return Selection{}, wrapf(ErrInvalidSelection, "selection に tool ID が必要")
	}
	if version.IsZero() {
		return Selection{}, wrapf(ErrInvalidSelection, "selection に version が必要")
	}
	if installID.IsZero() {
		return Selection{}, wrapf(ErrInvalidSelection, "selection に install ID が必要")
	}
	v, err := normalizeVariant(variant)
	if err != nil {
		return Selection{}, err
	}
	return Selection{toolID: toolID, version: version, variant: v, installID: installID}, nil
}

// ToolID は tool ID を返す。
func (s Selection) ToolID() ToolID { return s.toolID }

// Version は選択された完全版を返す。
func (s Selection) Version() Version { return s.version }

// Variant は選択された variant を返す。
func (s Selection) Variant() string { return s.variant }

// InstallID は選択された導入物の install ID を返す。
func (s Selection) InstallID() InstallID { return s.installID }

// EffectiveSelection は CLI 明示・project・user の優先順位で解決した最終選択を表す
// （02章3節, 05章6節, 10章4.5節）。origin が user/project/explicit のときだけ version を持つ。
type EffectiveSelection struct {
	toolID  ToolID
	origin  Origin
	version Version
	variant string
}

// NewEffectiveSelection は解決済みの有効選択を生成する。origin が user/project/explicit
// なら version 必須、none/disabled なら version は空でなければならない。整合しない場合は
// ErrInvalidSelection を返す。
func NewEffectiveSelection(toolID ToolID, origin Origin, version Version, variant string) (EffectiveSelection, error) {
	if toolID.IsZero() {
		return EffectiveSelection{}, wrapf(ErrInvalidSelection, "effective selection に tool ID が必要")
	}
	if _, err := NewOrigin(string(origin)); err != nil {
		return EffectiveSelection{}, err
	}
	selected := origin == OriginUser || origin == OriginProject || origin == OriginExplicit
	if selected && version.IsZero() {
		return EffectiveSelection{}, wrapf(ErrInvalidSelection, "origin=%s には version が必要", origin)
	}
	if !selected && !version.IsZero() {
		return EffectiveSelection{}, wrapf(ErrInvalidSelection, "origin=%s に version を持たせない", origin)
	}
	v := variant
	if selected {
		var err error
		if v, err = normalizeVariant(variant); err != nil {
			return EffectiveSelection{}, err
		}
	}
	return EffectiveSelection{toolID: toolID, origin: origin, version: version, variant: v}, nil
}

// ToolID は tool ID を返す。
func (e EffectiveSelection) ToolID() ToolID { return e.toolID }

// Origin は選択の由来を返す。
func (e EffectiveSelection) Origin() Origin { return e.origin }

// Version は有効な完全版を返す。未選択・無効化時はゼロ値の Version を返す。
func (e EffectiveSelection) Version() Version { return e.version }

// Variant は有効な variant を返す。
func (e EffectiveSelection) Variant() string { return e.variant }

// IsSelected は実行可能な選択（user/project/explicit）かどうかを返す。
func (e EffectiveSelection) IsSelected() bool {
	return e.origin == OriginUser || e.origin == OriginProject || e.origin == OriginExplicit
}
