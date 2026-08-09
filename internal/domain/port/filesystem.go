package port

import (
	"io"
	"io/fs"
	"time"
)

// FileSystem はfile操作を抽象化する（docs/02-architecture.md §4.1）。
//
// 操作は stat、read、atomic write、mkdir、rename、remove、walk、permission、
// realpath である。書込みをAtomicWriteだけに絞っているのは、docs/10-security.md
// が要求する「中断しても壊れた状態を残さない」を型で強制するためである。
// 部分書込みが観測できるAPIを置かない。
type FileSystem interface {
	// Stat はfile情報を返す。symlinkを解決しない。
	Stat(path string) (FileInfo, error)

	// Open は読取り用に開く。呼出側がCloseする。
	Open(path string) (io.ReadCloser, error)

	// ReadFile はfile全体を読む。上限を超える場合はerrorを返す。
	// 上限を引数で受けるのは、schema fileとarchiveで許す大きさが異なるためである。
	ReadFile(path string, limit int64) ([]byte, error)

	// AtomicWrite は同一filesystem上のtemporary fileへ書いてからrenameする。
	// 中断時にpathが旧内容のまま残るか、まったく存在しないかのどちらかになる。
	AtomicWrite(path string, data []byte, perm fs.FileMode) error

	// MkdirAll は親を含めてdirectoryを作る。既存なら成功する。
	MkdirAll(path string, perm fs.FileMode) error

	// Rename は同一filesystem上で移動する。commit操作に使う。
	Rename(oldPath, newPath string) error

	// Remove はfileまたは空directoryを消す。存在しない場合もerrorにしない。
	Remove(path string) error

	// RemoveAll はdirectory treeを消す。symlinkを辿らない。
	RemoveAll(path string) error

	// Walk はrootから再帰的に辿る。symlinkを辿らず、entryとして報告する。
	// symlinkを辿らないのは、docs/10-security.md のarchive traversal検査で
	// link先へ抜ける経路を作らないためである。
	Walk(root string, fn WalkFunc) error

	// Chmod はpermissionを設定する。Windows実装はfile属性へ写像できる範囲だけを扱う。
	Chmod(path string, perm fs.FileMode) error

	// RealPath はsymlinkとreparse pointを解決した絶対pathを返す。
	// path containment検査は解決後のpathで行う。
	RealPath(path string) (string, error)
}

// WalkFunc はWalkがentryごとに呼ぶ。errorを返すとWalkは中断してそれを返す。
type WalkFunc func(path string, info FileInfo) error

// FileInfo はfile情報である。fs.FileInfoを直接使わないのは、
// port境界へ標準libraryの実装型を露出させないためである。
type FileInfo struct {
	Name    string
	Size    int64
	Mode    fs.FileMode
	ModTime time.Time
	IsDir   bool
	// IsSymlink はsymlinkまたはWindowsのreparse pointである。
	IsSymlink bool
}
