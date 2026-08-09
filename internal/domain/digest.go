package domain

import (
	"fmt"
	"regexp"
	"strings"
)

// DigestAlgorithm はdigestのalgorithmである（docs/04-storage-and-data.md §6）。
type DigestAlgorithm string

// DigestAlgorithm の値。upstreamが公開するのはこの2件だけを採用する。
const (
	AlgoSHA256 DigestAlgorithm = "sha256"
	AlgoSHA512 DigestAlgorithm = "sha512"
)

// hexLength はalgorithmごとの小文字hex長である。
var hexLength = map[DigestAlgorithm]int{
	AlgoSHA256: 64,
	AlgoSHA512: 128,
}

var lowerHexRe = regexp.MustCompile(`^[0-9a-f]+$`)

// Digest はalgorithmと小文字hex値の組である（docs/02-architecture.md §3）。
//
// gdtvm自身が計算する値はSHA-256固定、upstream由来の値は`sha256`または`sha512`
// である。algorithmを型に持たせることで、長さ検査と照合を取り違えないようにする。
type Digest struct {
	algo DigestAlgorithm
	hex  string
}

// ParseUpstreamDigest は`<algorithm>:<lowercase hex>`形式を解析する。
//
// upstream由来のdigestはproviderが公開したalgorithmでの照合を必須とするため
// （docs/10-security.md）、algorithmを落とさずに保持する。
func ParseUpstreamDigest(text string) (Digest, error) {
	algoText, hex, found := strings.Cut(text, ":")
	if !found {
		return Digest{}, fmt.Errorf("domain: upstream digest %q が `<algorithm>:<hex>` 形式でない", text)
	}
	algo := DigestAlgorithm(algoText)
	length, ok := hexLength[algo]
	if !ok {
		return Digest{}, fmt.Errorf("domain: digest algorithm %q は sha256|sha512 のいずれでもない", algoText)
	}
	if err := validateHex(hex, length, algo); err != nil {
		return Digest{}, err
	}
	return Digest{algo: algo, hex: hex}, nil
}

// ParseInternalDigest はgdtvm自身が計算した64桁小文字hexを解析する。
//
// 内部digestにalgorithmを選ぶ理由がないためSHA-256固定であり、
// 永続形式にも`<algorithm>:`を付けない（docs/04-storage-and-data.md §6）。
func ParseInternalDigest(hex string) (Digest, error) {
	if err := validateHex(hex, hexLength[AlgoSHA256], AlgoSHA256); err != nil {
		return Digest{}, err
	}
	return Digest{algo: AlgoSHA256, hex: hex}, nil
}

func validateHex(hex string, length int, algo DigestAlgorithm) error {
	if len(hex) != length {
		return fmt.Errorf("domain: %s のhexは%d桁だが %d桁だった", algo, length, len(hex))
	}
	if !lowerHexRe.MatchString(hex) {
		return fmt.Errorf("domain: digest hexに小文字16進数以外が含まれる")
	}
	return nil
}

// Algorithm はalgorithmを返す。
func (d Digest) Algorithm() DigestAlgorithm { return d.algo }

// Hex は小文字hex値を返す。
func (d Digest) Hex() string { return d.hex }

// Upstream は`<algorithm>:<hex>`形式を返す。upstream digestの永続・表示に使う。
func (d Digest) Upstream() string { return string(d.algo) + ":" + d.hex }

// Internal は内部digestの永続形式（hexのみ）を返す。
//
// SHA-256以外でこれを呼ぶのは形式の取り違えであり、errorにする。
func (d Digest) Internal() (string, error) {
	if d.algo != AlgoSHA256 {
		return "", fmt.Errorf("domain: 内部digest形式はSHA-256固定だが %s が渡された", d.algo)
	}
	return d.hex, nil
}

// Equal は同じalgorithmと同じhexかどうかを返す。
//
// algorithmが異なる値を等しいとしない。片方がsha256、片方がsha512のときに
// hexだけを比べると常に不一致になるが、意図しない比較を見逃さないよう
// algorithmから明示的に判定する。
func (d Digest) Equal(other Digest) bool {
	return d.algo == other.algo && d.hex == other.hex
}

// IsZero はParseを通していない値かどうかを返す。
func (d Digest) IsZero() bool { return d.hex == "" }
