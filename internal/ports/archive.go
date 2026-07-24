package ports

import (
	"context"
	"io/fs"
)

// ArchiveFormat は archive 形式の enum である（06章6節の format と対応）。
type ArchiveFormat string

// ArchiveFormat の値。
const (
	FormatZip    ArchiveFormat = "zip"
	FormatTar    ArchiveFormat = "tar"
	FormatTarGz  ArchiveFormat = "tar-gz"
	FormatTarXz  ArchiveFormat = "tar-xz"
	FormatTarZst ArchiveFormat = "tar-zst"
	Format7z     ArchiveFormat = "7z"
	FormatExe    ArchiveFormat = "exe"
	FormatMsi    ArchiveFormat = "msi"
	FormatRaw    ArchiveFormat = "raw"
)

// ArchiveEntry は archive 内 entry の事前列挙結果である（08章7節の安全検査対象）。
type ArchiveEntry struct {
	Path       string // archive 内の相対 path
	Size       int64
	Mode       fs.FileMode
	IsDir      bool
	IsSymlink  bool
	LinkTarget string // symlink の場合の target
}

// ExtractRequest は選択展開の指定である。安全規則（path traversal、衝突、bomb 上限）は
// 実装が strip_components 適用後に強制する（08章7節）。
type ExtractRequest struct {
	ArchivePath string
	Format      ArchiveFormat
	Destination string
	// StripComponents は各 entry path の先頭 component 数を除去する。
	StripComponents int
	// Include/Exclude は正規化 POSIX path への glob（Include 適用後に Exclude）。
	Include []string
	Exclude []string
	// Limits は archive bomb 対策の上限。
	MaxEntries        int64
	MaxTotalBytes     int64
	MaxSingleFileByte int64
	MaxRatio          int64
	// Progress は進捗通知（nil 可）。current/total は展開 byte 数。
	Progress func(current, total int64)
}

// ArchiveExtractor は list、安全検査、選択展開、形式判定を担う抽象 port である
// （02章5節, 08章7節）。7z/exe/msi 等 helper や OS component を要する形式は別 port/
// dependency で処理し、本 port は native に扱える形式を対象とする。
type ArchiveExtractor interface {
	// DetectFormat は magic/拡張子から形式を判定する。
	DetectFormat(archivePath string) (ArchiveFormat, error)
	// List は展開前に全 entry を列挙する。安全検査は呼び出し側と実装の双方で行う。
	List(ctx context.Context, archivePath string, format ArchiveFormat) ([]ArchiveEntry, error)
	// Extract は安全規則を強制しつつ選択展開する。
	Extract(ctx context.Context, req ExtractRequest) error
}
