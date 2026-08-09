package port

// LinkKind はlinkの種別である。
type LinkKind string

const (
	// LinkNone はlinkではない実体を表す。
	LinkNone LinkKind = "none"
	// LinkJunction はWindowsのdirectory junctionである。
	LinkJunction LinkKind = "junction"
	// LinkSymlink はsymbolic linkである。Linuxではrelative symlinkを使う。
	LinkSymlink LinkKind = "symlink"
	// LinkHardlink はhard linkである。
	LinkHardlink LinkKind = "hardlink"
)

// LinkCapabilities は対象filesystemが実際に作れるlink種別である。
//
// 能力を事前に問い合わせるのは、docs/09-platform.md がWindows標準ユーザーでは
// symlinkを作れない場合があると定めるためである。作ってみて失敗したら別方式へ
// 落とすのではなく、Planを作る前に判定して利用者へ提示する。
type LinkCapabilities struct {
	Junction bool
	Symlink  bool
	Hardlink bool
}

// LinkManager はlinkの作成、種別取得、除去、能力検査を抽象化する
// （docs/02-architecture.md §4.1）。
type LinkManager interface {
	// Capabilities はdir上で作成可能なlink種別を返す。
	Capabilities(dir string) (LinkCapabilities, error)

	// CreateJunction はWindowsのdirectory junctionを作る。
	CreateJunction(linkPath, targetDir string) error

	// CreateSymlink はsymbolic linkを作る。relativeがtrueならlinkPathからの
	// 相対targetで作る。distribution rootごと移動しても壊れないようにするためである。
	CreateSymlink(linkPath, target string, relative bool) error

	// CreateHardlink はhard linkを作る。
	CreateHardlink(linkPath, targetFile string) error

	// Kind はpath自体のlink種別を返す。辿った先ではなくpath自体を見る。
	Kind(path string) (LinkKind, error)

	// ReadLink はlinkが指すtargetを解決せずに返す。
	ReadLink(path string) (string, error)

	// RemoveLink はlinkだけを外す。
	//
	// link先の実体を消してはならない。currentの張り替えでtool本体を消す事故を
	// 防ぐため、実体削除はFileSystem側の操作と明確に分ける。
	RemoveLink(path string) error
}
