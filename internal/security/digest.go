package security

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
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

// InternalStreamDigest はreaderの内容の内部SHA-256とbyte数を返す。
//
// docs/04-storage-and-data.md §7「gdtvm自身が計算するdigestはSHA-256固定」。
// receiptの`command_targets[].sha256`（[08-install-runtime.md] §7手順4）のように、
// **upstream digestを伴わない**内部digestを計算する用途で使う。
//
// [StreamHasher]を使わないのは、あちらがupstream algorithmを必ず要求するため
// である。upstreamの無い対象へ渡すと、意味のないalgorithmを選ばせることになる。
//
// payload内のfileはinstall済みの実体で大きくなりうるため、全体をmemoryへ
// 読まずに流す。`limit`を超えたらerrorにする——上限なしで読むと、壊れた
// payloadやsymlink差し替えで無制限にreadしうる。
func InternalStreamDigest(r io.Reader, limit int64) (string, int64, error) {
	if r == nil {
		return "", 0, errors.New("security: readerが未設定")
	}
	if limit <= 0 {
		return "", 0, fmt.Errorf("security: 上限が正でない（%d）", limit)
	}
	digest := sha256.New()
	// limitちょうどを許し、超過だけを拒否するため1 byte多く読む。
	size, err := io.Copy(digest, io.LimitReader(r, limit+1))
	if err != nil {
		return "", 0, fmt.Errorf("security: digest計算中の読取りに失敗した: %w", err)
	}
	if size > limit {
		return "", 0, fmt.Errorf("security: 内容が上限%d byteを超える", limit)
	}
	return hex.EncodeToString(digest.Sum(nil)), size, nil
}
