package infra

import (
	"crypto/rand"
	"fmt"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/ports"
)

// uuidGenerator は UUID version 4 を生成する ports.IDGenerator の既定実装である。
type uuidGenerator struct{}

// NewUUIDGenerator は暗号学的乱数から UUIDv4 を生成する IDGenerator を返す。
func NewUUIDGenerator() ports.IDGenerator { return uuidGenerator{} }

// NewID は UUIDv4 形式の ID を返す。crypto/rand.Read は go1.24 以降エラーを返さない契約
// （OS RNG が使えない場合は runtime が致命終了する）ため、返り値のエラーは扱わない。
func (uuidGenerator) NewID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	// version(4) と variant(RFC 4122) bit を設定する。
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
