package ports

import (
	"io"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
)

// HashCalculator は streaming で digest を計算する抽象 port である（02章5節）。検証前の
// ファイルを実行・展開しないため、download 中の stream からも digest を得られる。
type HashCalculator interface {
	// SumSHA256Stream は reader を最後まで読み、SHA-256 digest を返す。
	SumSHA256Stream(r io.Reader) (domain.Digest, error)
	// SumSHA256File はファイルを開いて SHA-256 digest を返す。
	SumSHA256File(path string) (domain.Digest, error)
}
