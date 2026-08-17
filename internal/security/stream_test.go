package security

import (
	"bytes"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
)

// TestStreamHasherComputesBothDigestsInOnePass は1 passで内部SHA-256とupstream
// digestの両方を得られることを固定する。
//
// docs/05-configuration.md §3.5が「1 artifactはstreamで処理し全量memoryへ
// 載せない」と定める。2 passにすると、読み直しの間にfileが差し替わったときに
// 「検証したbytes」と「使うbytes」が別物になりうる。
func TestStreamHasherComputesBothDigestsInOnePass(t *testing.T) {
	payload := []byte("gdtvm artifact payload")

	cases := []struct {
		algo domain.DigestAlgorithm
		want string
	}{
		{domain.AlgoSHA256, sum256(payload)},
		{domain.AlgoSHA512, sum512(payload)},
	}
	for _, c := range cases {
		t.Run(string(c.algo), func(t *testing.T) {
			hasher, err := NewStreamHasher(c.algo, 1<<20)
			if err != nil {
				t.Fatalf("NewStreamHasher: %v", err)
			}
			read, err := io.Copy(hasher, bytes.NewReader(payload))
			if err != nil {
				t.Fatalf("Copy: %v", err)
			}
			if read != int64(len(payload)) {
				t.Errorf("読取り = %d, want %d", read, len(payload))
			}
			if hasher.Size() != int64(len(payload)) {
				t.Errorf("Size = %d, want %d", hasher.Size(), len(payload))
			}

			// 内部digestはalgorithmによらず常にSHA-256である（§7）。
			wantInternal := sum256(payload)
			if got := hasher.Internal(); got.Hex() != wantInternal {
				t.Errorf("Internal = %s, want %s", got.Hex(), wantInternal)
			}
			if got := hasher.Internal().Algorithm(); got != domain.AlgoSHA256 {
				t.Errorf("Internal algorithm = %q, want sha256", got)
			}
			// upstream digestは指定したalgorithmである。
			upstream := hasher.Upstream()
			if upstream.Algorithm() != c.algo {
				t.Errorf("Upstream algorithm = %q, want %q", upstream.Algorithm(), c.algo)
			}
			if upstream.Hex() != c.want {
				t.Errorf("Upstream = %s, want %s", upstream.Hex(), c.want)
			}
		})
	}
}

// TestStreamHasherIsStableAcrossReads は読出しを繰り返してもdigestが変わらない
// ことを固定する。
//
// `hash.Hash.Sum`は状態を進めないが、実装が`Sum`の結果を内部へ書き戻すと
// 2回目以降が変わる。照合とreceipt記録で別の値になるのを防ぐ。
func TestStreamHasherIsStableAcrossReads(t *testing.T) {
	hasher, err := NewStreamHasher(domain.AlgoSHA512, 1<<20)
	if err != nil {
		t.Fatalf("NewStreamHasher: %v", err)
	}
	if _, err := io.Copy(hasher, strings.NewReader("payload")); err != nil {
		t.Fatalf("Copy: %v", err)
	}
	first, firstUp := hasher.Internal(), hasher.Upstream()
	second, secondUp := hasher.Internal(), hasher.Upstream()
	if !first.Equal(second) {
		t.Errorf("Internalが変わった: %s → %s", first.Hex(), second.Hex())
	}
	if !firstUp.Equal(secondUp) {
		t.Errorf("Upstreamが変わった: %s → %s", firstUp.Hex(), secondUp.Hex())
	}
}

// TestStreamHasherSplitsWritesIdentically は分割書込みでも結果が同じことを
// 固定する。
//
// networkからのstreamは任意の境界で分割されて届く。
func TestStreamHasherSplitsWritesIdentically(t *testing.T) {
	payload := bytes.Repeat([]byte("abcdefghij"), 1000)

	whole, err := NewStreamHasher(domain.AlgoSHA256, 1<<20)
	if err != nil {
		t.Fatalf("NewStreamHasher: %v", err)
	}
	if _, err := whole.Write(payload); err != nil {
		t.Fatalf("Write: %v", err)
	}

	split, err := NewStreamHasher(domain.AlgoSHA256, 1<<20)
	if err != nil {
		t.Fatalf("NewStreamHasher: %v", err)
	}
	for offset := 0; offset < len(payload); offset += 7 {
		end := offset + 7
		if end > len(payload) {
			end = len(payload)
		}
		if _, err := split.Write(payload[offset:end]); err != nil {
			t.Fatalf("Write: %v", err)
		}
	}

	if !whole.Internal().Equal(split.Internal()) {
		t.Errorf("Internalが分割で変わった")
	}
	if !whole.Upstream().Equal(split.Upstream()) {
		t.Errorf("Upstreamが分割で変わった")
	}
	if whole.Size() != split.Size() {
		t.Errorf("Size = %d / %d", whole.Size(), split.Size())
	}
}

// TestStreamHasherEnforcesLimit は上限超過をerrorにすることを固定する。
//
// 静かに切り詰めると、途中までのarchiveを完全なものとして扱ってしまう。
func TestStreamHasherEnforcesLimit(t *testing.T) {
	payload := bytes.Repeat([]byte("x"), 100)

	t.Run("上限ちょうどは通る", func(t *testing.T) {
		hasher, err := NewStreamHasher(domain.AlgoSHA256, int64(len(payload)))
		if err != nil {
			t.Fatalf("NewStreamHasher: %v", err)
		}
		if _, err := io.Copy(hasher, bytes.NewReader(payload)); err != nil {
			t.Fatalf("上限ちょうどが拒否された: %v", err)
		}
		if hasher.Size() != int64(len(payload)) {
			t.Errorf("Size = %d", hasher.Size())
		}
	})

	t.Run("1 byte超過を拒否", func(t *testing.T) {
		hasher, err := NewStreamHasher(domain.AlgoSHA256, int64(len(payload))-1)
		if err != nil {
			t.Fatalf("NewStreamHasher: %v", err)
		}
		_, copyErr := io.Copy(hasher, bytes.NewReader(payload))
		if copyErr == nil {
			t.Fatal("上限超過が通った")
		}
		if !errors.Is(copyErr, ErrSizeLimit) {
			t.Fatalf("err = %v, want ErrSizeLimit", copyErr)
		}
	})

	t.Run("超過後の書込みも拒否", func(t *testing.T) {
		hasher, err := NewStreamHasher(domain.AlgoSHA256, 1)
		if err != nil {
			t.Fatalf("NewStreamHasher: %v", err)
		}
		if _, err := hasher.Write([]byte("ab")); !errors.Is(err, ErrSizeLimit) {
			t.Fatalf("1回目 = %v", err)
		}
		// 超過後も同じerrorを返し、hashの状態を進めない。
		if _, err := hasher.Write([]byte("c")); !errors.Is(err, ErrSizeLimit) {
			t.Fatalf("2回目 = %v", err)
		}
		if hasher.Size() != 0 {
			t.Errorf("超過分がSizeへ加算された: %d", hasher.Size())
		}
	})
}

// TestStreamHasherPropagatesReadError は読取り失敗をそのまま返すことを固定する。
//
// 上限超過（[ErrSizeLimit]）と読取り失敗は、partial fileの破棄理由として
// 呼出し側が区別する。
func TestStreamHasherPropagatesReadError(t *testing.T) {
	sentinel := errors.New("network切断")
	hasher, err := NewStreamHasher(domain.AlgoSHA256, 1<<20)
	if err != nil {
		t.Fatalf("NewStreamHasher: %v", err)
	}
	reader := io.MultiReader(strings.NewReader("partial"), errReader{sentinel})

	read, copyErr := io.Copy(hasher, reader)
	if !errors.Is(copyErr, sentinel) {
		t.Fatalf("err = %v, want %v", copyErr, sentinel)
	}
	if errors.Is(copyErr, ErrSizeLimit) {
		t.Error("読取り失敗が上限超過として報告された")
	}
	// 途中まで読んだ分はSizeへ反映される。破棄判断に使う。
	if read != int64(len("partial")) || hasher.Size() != int64(len("partial")) {
		t.Errorf("read = %d / Size = %d, want %d", read, hasher.Size(), len("partial"))
	}
}

// TestNewStreamHasherRejectsInvalidInput は構築時の検査を固定する。
func TestNewStreamHasherRejectsInvalidInput(t *testing.T) {
	cases := []struct {
		name  string
		algo  domain.DigestAlgorithm
		limit int64
	}{
		{"上限が0", domain.AlgoSHA256, 0},
		{"上限が負", domain.AlgoSHA256, -1},
		{"未知のalgorithm", domain.DigestAlgorithm("md5"), 1 << 20},
		{"空のalgorithm", domain.DigestAlgorithm(""), 1 << 20},
		{"sha1", domain.DigestAlgorithm("sha1"), 1 << 20},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := NewStreamHasher(c.algo, c.limit); err == nil {
				t.Fatal("不正な入力が通った")
			}
		})
	}
}

// TestStreamHasherVerifyUpstream は期待digestとの照合を固定する。
func TestStreamHasherVerifyUpstream(t *testing.T) {
	payload := []byte("artifact")
	newHasher := func(t *testing.T, algo domain.DigestAlgorithm) *StreamHasher {
		t.Helper()
		hasher, err := NewStreamHasher(algo, 1<<20)
		if err != nil {
			t.Fatalf("NewStreamHasher: %v", err)
		}
		if _, err := hasher.Write(payload); err != nil {
			t.Fatalf("Write: %v", err)
		}
		return hasher
	}

	t.Run("一致", func(t *testing.T) {
		hasher := newHasher(t, domain.AlgoSHA256)
		expected := mustUpstream(t, "sha256:"+sum256(payload))
		if err := hasher.VerifyUpstream(expected); err != nil {
			t.Fatalf("一致するdigestが拒否された: %v", err)
		}
	})

	t.Run("不一致", func(t *testing.T) {
		hasher := newHasher(t, domain.AlgoSHA256)
		other := mustUpstream(t, "sha256:"+sum256([]byte("別のbytes")))
		if err := hasher.VerifyUpstream(other); err == nil {
			t.Fatal("違うdigestが通った")
		}
	})

	t.Run("未設定", func(t *testing.T) {
		hasher := newHasher(t, domain.AlgoSHA256)
		if err := hasher.VerifyUpstream(domain.Digest{}); err == nil {
			t.Fatal("未設定のdigestが通った")
		}
	})

	// providerが公開したalgorithmでの照合を必須とする（docs/10-security.md）。
	// 別algorithmの値と突き合わせて「一致しない」で片付けない。
	t.Run("algorithm違い", func(t *testing.T) {
		hasher := newHasher(t, domain.AlgoSHA256)
		wrongAlgo := mustUpstream(t, "sha512:"+sum512(payload))
		err := hasher.VerifyUpstream(wrongAlgo)
		if err == nil {
			t.Fatal("別algorithmの期待値が通った")
		}
		if !strings.Contains(err.Error(), "algorithm") {
			t.Errorf("algorithm不一致として報告されない: %v", err)
		}
	})
}

// --- helper ---

// errReader は指定したerrorを返すだけのreaderである。
type errReader struct{ err error }

func (r errReader) Read([]byte) (int, error) { return 0, r.err }

// hexOf は固定長配列のdigestをhex文字列にする。
//
// `[32]byte`と`[64]byte`はunderlying typeが違い1つのtype parameterで扱えないため、
// sliceを受ける。呼出し側は`sum256(...)`／`sum512(...)`を通す。
func hexOf(sum []byte) string {
	return hex.EncodeToString(sum)
}

func sum256(data []byte) string {
	digest := sha256.Sum256(data)
	return hexOf(digest[:])
}

func sum512(data []byte) string {
	digest := sha512.Sum512(data)
	return hexOf(digest[:])
}

func mustUpstream(t *testing.T, text string) domain.Digest {
	t.Helper()
	digest, err := domain.ParseUpstreamDigest(text)
	if err != nil {
		t.Fatalf("ParseUpstreamDigest(%q): %v", text, err)
	}
	return digest
}
