package fake

import (
	"encoding/binary"
	"sync"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain/port"
)

// OpNewIDBytes はRandom.NewIDBytesの失敗注入名である。
const OpNewIDBytes = "random.NewIDBytes"

// Random は決定的な連番を返すfake Randomである。
//
// 乱数ではなく連番にするのは、Plan、receipt、lock metadata、structured logへ
// 現れるIDをgolden比較できるようにするためである。1回目は
// `00000000000000000000000000000001` を返す。
type Random struct {
	mu       sync.Mutex
	injector *Injector
	counter  uint64
}

// NewRandom は連番0から始まるfake Randomを作る。
func NewRandom(injector *Injector) *Random {
	return &Random{injector: injector}
}

// NewIDBytes は連番を128 bitのbig endian値として返す。
func (r *Random) NewIDBytes() ([domain.IDByteLength]byte, error) {
	if err := r.injector.Check(OpNewIDBytes); err != nil {
		return [domain.IDByteLength]byte{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.counter++
	var raw [domain.IDByteLength]byte
	binary.BigEndian.PutUint64(raw[8:], r.counter)
	return raw, nil
}

var _ port.Random = (*Random)(nil)
