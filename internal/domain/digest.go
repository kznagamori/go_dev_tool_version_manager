package domain

import (
	"fmt"
	"strings"
)

// DigestAlgorithm は digest のアルゴリズム enum である。schema 1 では検証済み判定に
// sha256 だけを用いる（06章6.1節, 11章, 14章2節）。
type DigestAlgorithm string

// AlgorithmSHA256 は schema 1 で検証済み判定に用いる唯一のアルゴリズムである。
const AlgorithmSHA256 DigestAlgorithm = "sha256"

// sha256HexLen は SHA-256 の hex 表現長（64 桁）である。
const sha256HexLen = 64

// Digest は algorithm と小文字 hex 値の組を表す値型である。生成時に hex の桁数・
// 文字種を検査し、小文字へ正規化する（06章6.1節）。
type Digest struct {
	algorithm DigestAlgorithm
	value     string
}

// NewSHA256Digest は SHA-256 hex 文字列を検証して Digest を生成する。大文字を含む
// hex は小文字へ正規化する。桁数違いや hex 以外の文字を含む場合は ErrInvalidDigest
// を包んで返す。
func NewSHA256Digest(hexValue string) (Digest, error) {
	if len(hexValue) != sha256HexLen {
		return Digest{}, fmt.Errorf("%w: sha256 は %d 桁の hex が必要（実際 %d）", ErrInvalidDigest, sha256HexLen, len(hexValue))
	}
	lower := strings.ToLower(hexValue)
	if !isHex(lower) {
		return Digest{}, fmt.Errorf("%w: hex 以外の文字を含む", ErrInvalidDigest)
	}
	return Digest{algorithm: AlgorithmSHA256, value: lower}, nil
}

// isHex は文字列が小文字 hex（0-9, a-f）だけで構成されるかを返す。
func isHex(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// Algorithm はアルゴリズムを返す。
func (d Digest) Algorithm() DigestAlgorithm { return d.algorithm }

// Value は小文字 hex 値を返す。
func (d Digest) Value() string { return d.value }

// IsZero はゼロ値（未設定）の Digest かどうかを返す。
func (d Digest) IsZero() bool { return d.value == "" }

// Equal は 2 つの Digest が algorithm と value の両方で一致するかを返す。
func (d Digest) Equal(other Digest) bool {
	return d.algorithm == other.algorithm && d.value == other.value
}

// String は "algorithm:value" 形式を返す。
func (d Digest) String() string { return string(d.algorithm) + ":" + d.value }
