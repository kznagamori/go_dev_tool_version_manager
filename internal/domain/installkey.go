package domain

// InstallKey は導入物を一意に識別する複合キーである（02章3節）。ToolID、Version、
// Platform（variant を含む）の組で、install lock の直列化単位や receipt 照合に用いる。
type InstallKey struct {
	toolID   ToolID
	version  Version
	platform Platform
}

// NewInstallKey は各要素が設定済みであることを確認して InstallKey を生成する。
// いずれかがゼロ値なら ErrInvalidSelection を返す。
func NewInstallKey(toolID ToolID, version Version, platform Platform) (InstallKey, error) {
	if toolID.IsZero() {
		return InstallKey{}, wrapf(ErrInvalidSelection, "install key に tool ID が必要")
	}
	if version.IsZero() {
		return InstallKey{}, wrapf(ErrInvalidSelection, "install key に version が必要")
	}
	if platform.OS() == "" || platform.Arch() == "" {
		return InstallKey{}, wrapf(ErrInvalidSelection, "install key に platform が必要")
	}
	return InstallKey{toolID: toolID, version: version, platform: platform}, nil
}

// ToolID は tool ID を返す。
func (k InstallKey) ToolID() ToolID { return k.toolID }

// Version は完全版を返す。
func (k InstallKey) Version() Version { return k.version }

// Platform は platform を返す。
func (k InstallKey) Platform() Platform { return k.platform }

// Equal は 2 つの InstallKey が全要素で一致するかを返す。
func (k InstallKey) Equal(other InstallKey) bool {
	return k.toolID == other.toolID &&
		k.version == other.version &&
		k.platform == other.platform
}

// String は install lock の辞書順比較に使える安定した識別文字列を返す。
func (k InstallKey) String() string {
	return k.toolID.String() + "@" + k.version.String() + "#" + k.platform.String()
}
