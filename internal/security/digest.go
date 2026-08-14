package security

import (
	"crypto/sha256"
	"encoding/hex"
)

// SHA256Hex はbytesの内部digestを64 lowercase hexで返す。
//
// docs/02-architecture.md §2が「内部SHA-256」を本領域の責務とする。
// docs/04-storage-and-data.md §7の「gdtvm自身が計算するdigestはSHA-256固定」に
// あたる値であり、`<algorithm>:`を付けない。
//
// state fileのatomic write（§4 step 7）と`.bak`の復元候補判定が、書いた内容と
// diskの内容が一致することの確認に使う。
func SHA256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
