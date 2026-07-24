package ports

// LinkKind は link の種別である（09章）。
type LinkKind string

// LinkKind の値。
const (
	// LinkNone は対象が link ではない（通常ファイル/directory）ことを表す。
	LinkNone LinkKind = "none"
	// LinkJunction は Windows directory junction を表す。
	LinkJunction LinkKind = "junction"
	// LinkSymlink は symbolic link を表す。
	LinkSymlink LinkKind = "symlink"
	// LinkHardlink は hard link を表す。
	LinkHardlink LinkKind = "hardlink"
)

// LinkCapabilities は現在環境で作成可能な link 種別を表す（09章2.1節の非破壊 probe 結果）。
type LinkCapabilities struct {
	Junction bool
	Symlink  bool
	Hardlink bool
}

// LinkManager は junction/symlink/hardlink の作成・種別取得・安全な除去・能力検査を担う
// 抽象 port である（02章5節, 09章）。tool 本体を切替のために copy しない。除去は link
// 自身だけを消し、target directory を再帰削除しない。
type LinkManager interface {
	// Capabilities は実際の一時領域での probe により作成可能な link 種別を返す。
	Capabilities(probeDir string) (LinkCapabilities, error)
	// LinkKindOf は対象 path の link 種別を返す。
	LinkKindOf(path string) (LinkKind, error)
	// CreateJunction は Windows directory junction を作る（同一 tool root の payload のみ）。
	CreateJunction(linkPath, target string) error
	// CreateSymlink は symbolic link を作る（Linux は相対 symlink を優先）。
	CreateSymlink(linkPath, target string) error
	// CreateHardlink は hard link を作る（shim 用）。
	CreateHardlink(linkPath, target string) error
	// ReadTarget は symlink/junction の target を返す。
	ReadTarget(linkPath string) (string, error)
	// RemoveLink は link/reparse point 自身だけを除去し、target を保持する。
	RemoveLink(linkPath string) error
}
