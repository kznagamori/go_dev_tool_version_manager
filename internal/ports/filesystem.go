package ports

import (
	"io"
	"io/fs"
	"time"
)

// FileInfo は FileSystem port が返す最小のファイル情報である。具体的な os.FileInfo を
// 境界へ漏らさない。
type FileInfo struct {
	Name      string
	Size      int64
	Mode      fs.FileMode
	ModTime   time.Time
	IsDir     bool
	IsSymlink bool
}

// FileSystem は filesystem 操作の抽象 port である（02章5節）。書換えは一時ファイル +
// flush + 同一ボリューム rename を基本とし、path containment は上位の security port で
// 検査する。realpath は symlink/reparse point を解決した絶対 path を返す。
type FileSystem interface {
	// Stat は symlink を辿った対象の情報を返す。
	Stat(path string) (FileInfo, error)
	// Lstat は symlink 自身の情報を返す。
	Lstat(path string) (FileInfo, error)
	// ReadFile はファイル全体を読む。上限は呼び出し側が別途強制する。
	ReadFile(path string) ([]byte, error)
	// Open は読取り用の stream を返す。大きなファイルの逐次処理に用いる。
	Open(path string) (io.ReadCloser, error)
	// WriteFileAtomic は一時ファイル + flush + rename で原子的に書き込む。
	WriteFileAtomic(path string, data []byte, mode fs.FileMode) error
	// MkdirAll は親を含めて directory を作成する。
	MkdirAll(path string, mode fs.FileMode) error
	// Rename は同一ボリューム内で名前を変更する。
	Rename(oldPath, newPath string) error
	// Remove は 1 件のファイル/空 directory または link 自身を除去する。
	Remove(path string) error
	// RemoveAll は再帰的に除去する。呼び出し側が root containment を保証する。
	RemoveAll(path string) error
	// ReadDir は directory 直下の entry を返す。
	ReadDir(path string) ([]FileInfo, error)
	// RealPath は symlink を解決した絶対 path を返す。
	RealPath(path string) (string, error)
	// Chmod は permission を変更する（Linux）。Windows では実行 permission に用いない。
	Chmod(path string, mode fs.FileMode) error
}
