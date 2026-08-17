package security

import (
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
)

// StreamHasher はstreamを1 passで走査し、複数algorithmのdigestを同時に計算する。
//
// docs/05-configuration.md §3.5が「1 artifactはstreamで処理し全量memoryへ載せない」
// と定め、docs/04-storage-and-data.md §21がartifact downloadの上限を20 GiBとする。
// 全量をmemoryへ載せる[SHA256Hex]はこの用途に使えない。
//
// **2 passにしない。** downloadしたbytesをもう一度読み直すと、読み直しの間に
// fileが差し替わったときに「検証したbytes」と「使うbytes」が別物になりうる。
// 1 passで内部digestとupstream digestの両方を得て、同じbytesに対する結論に
// することが目的である。
//
// portにしないのは、digest計算が外部作用を持たない純計算だからである
// （docs/02-architecture.md §7）。同じ入力は常に同じ結果を返し、差し替える
// 意味がない。
type StreamHasher struct {
	// internal はgdtvm自身が計算するSHA-256である（§7「gdtvm自身が計算する
	// digestはSHA-256固定」）。常に計算する。
	internal hash.Hash
	// upstream はproviderが公開したalgorithmのdigestである。
	//
	// 内部SHA-256と同じalgorithmでも別のhash instanceを持つ。共有すると、
	// 片方だけを読み出した時点でもう片方の状態に影響する書き方を誘発する。
	upstream hash.Hash
	// upstreamAlgo はupstream digestのalgorithmである。
	upstreamAlgo domain.DigestAlgorithm
	// size は走査したbyte数である。
	size int64
	// limit は許す最大byte数である。0以下は指定なしを表さず、構築時に拒否する。
	limit int64
	// exceeded は上限を超えたかである。
	exceeded bool
}

// NewStreamHasher はupstream algorithmと上限を指定してhasherを作る。
//
// `limit`はdocs/04-storage-and-data.md §21の該当上限を呼出し側が渡す。artifact
// downloadは20 GiB、checksum textは2 MiB、upstream metadataは16 MiBと用途ごとに
// 別の値であり、hasher側で1つに決められない。
func NewStreamHasher(upstreamAlgo domain.DigestAlgorithm, limit int64) (*StreamHasher, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("security: 上限が正でない（%d）", limit)
	}
	upstream, err := newHash(upstreamAlgo)
	if err != nil {
		return nil, err
	}
	return &StreamHasher{
		internal:     sha256.New(),
		upstream:     upstream,
		upstreamAlgo: upstreamAlgo,
		limit:        limit,
	}, nil
}

// newHash はalgorithmに対応するhashを作る。
func newHash(algo domain.DigestAlgorithm) (hash.Hash, error) {
	switch algo {
	case domain.AlgoSHA256:
		return sha256.New(), nil
	case domain.AlgoSHA512:
		return sha512.New(), nil
	default:
		return nil, fmt.Errorf(
			"security: digest algorithm %q は %s|%s のいずれでもない",
			algo, domain.AlgoSHA256, domain.AlgoSHA512)
	}
}

// ErrSizeLimit はstreamが上限を超えたことを表す。
//
// sentinelにするのは、呼出し側が「上限超過」と「読取り失敗」を区別して
// partial fileの破棄理由を決められるようにするためである。
var ErrSizeLimit = errors.New("security: streamが上限を超えた")

// Write は[io.Writer]としてbytesを取り込む。
//
// 上限を超えた時点でErrSizeLimitを返す。超過後は状態を汚さないよう、以降の
// 書込みも同じerrorを返す。上限超過を静かに切り詰めると、途中までのarchiveを
// 完全なものとして扱ってしまう。
func (h *StreamHasher) Write(p []byte) (int, error) {
	if h.exceeded {
		return 0, ErrSizeLimit
	}
	if h.size+int64(len(p)) > h.limit {
		h.exceeded = true
		return 0, fmt.Errorf("%w（上限 %d byte）", ErrSizeLimit, h.limit)
	}
	// hash.HashのWriteは仕様上errorを返さない。
	h.internal.Write(p)
	h.upstream.Write(p)
	h.size += int64(len(p))
	return len(p), nil
}

// ReadFrom は[io.ReaderFrom]としてstreamを取り込む。
//
// `io.Copy`がこれを使い、中間bufferの確保を1回に抑える。
func (h *StreamHasher) ReadFrom(r io.Reader) (int64, error) {
	// 上限＋1まで読み、超過を検出できるようにする。LimitReaderだけでは
	// EOFとして静かに切り詰められる。
	buffer := make([]byte, 32*1024)
	var total int64
	for {
		n, err := r.Read(buffer)
		if n > 0 {
			written, writeErr := h.Write(buffer[:n])
			total += int64(written)
			if writeErr != nil {
				return total, writeErr
			}
		}
		if errors.Is(err, io.EOF) {
			return total, nil
		}
		if err != nil {
			return total, err
		}
	}
}

// Size は走査したbyte数を返す。
func (h *StreamHasher) Size() int64 { return h.size }

// Internal はgdtvm自身が計算したSHA-256を返す。
//
// 複数回呼んでも同じ値を返し、hashの状態を進めない。
//
// SHA-256のSumは常に32 byteでhexは64文字になるため、parseは失敗しない。失敗した
// 場合もzero値を返してpanicさせない。digest計算の途中でprocessを落とすと、
// 書きかけの`.part`が残る（CLAUDE.md §9）。zero値は[domain.Digest.IsZero]で
// 検出でき、照合は必ず失敗する。
func (h *StreamHasher) Internal() domain.Digest {
	digest, _ := domain.ParseInternalDigest(hex.EncodeToString(h.internal.Sum(nil)))
	return digest
}

// Upstream はproviderが公開したalgorithmでのdigestを返す。
//
// algorithmは構築時に検証済みで、hex長はalgorithmから決まるためparseは失敗しない。
// [StreamHasher.Internal]と同じ理由でpanicさせない。
func (h *StreamHasher) Upstream() domain.Digest {
	text := string(h.upstreamAlgo) + ":" + hex.EncodeToString(h.upstream.Sum(nil))
	digest, _ := domain.ParseUpstreamDigest(text)
	return digest
}

// VerifyUpstream は計算したdigestが期待値と一致するかを返す。
//
// algorithmが違う場合もerrorにする。providerが公開したalgorithmでの照合を
// 必須とするため（docs/10-security.md §7.2）、別algorithmの値と突き合わせて
// 「一致しない」で片付けない。
func (h *StreamHasher) VerifyUpstream(expected domain.Digest) error {
	if expected.IsZero() {
		return errors.New("security: 期待するupstream digestが未設定")
	}
	if expected.Algorithm() != h.upstreamAlgo {
		return fmt.Errorf(
			"security: 期待digestのalgorithmが %q、計算側が %q である",
			expected.Algorithm(), h.upstreamAlgo)
	}
	got := h.Upstream()
	if !got.Equal(expected) {
		return fmt.Errorf(
			"security: upstream digestが一致しない（期待 %s / 実際 %s）",
			expected.Upstream(), got.Upstream())
	}
	return nil
}
