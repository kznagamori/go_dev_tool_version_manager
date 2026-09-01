package security

import (
	"bytes"
	"errors"
	"testing"
	"testing/iotest"
)

// TestSHA256HexMatchesKnownVector は既知vectorとの一致を固定する。
//
// 空入力のSHA-256はRFC 6234の既知値である。
func TestSHA256HexMatchesKnownVector(t *testing.T) {
	const empty = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	if got := SHA256Hex(nil); got != empty {
		t.Errorf("SHA256Hex(nil) = %q, want %q", got, empty)
	}
	if got := SHA256Hex([]byte{}); got != empty {
		t.Errorf("SHA256Hex(空) = %q, want %q", got, empty)
	}
}

// TestInternalStreamDigestMatchesBufferDigest はstream版とbuffer版の一致を固定する。
//
// 2つの計算が食い違うと、receiptへ書いた値と`doctor`の照合値が合わなくなる。
func TestInternalStreamDigestMatchesBufferDigest(t *testing.T) {
	for _, size := range []int{0, 1, 4096, 100_000} {
		data := bytes.Repeat([]byte("x"), size)
		digest, read, err := InternalStreamDigest(bytes.NewReader(data), 1<<20)
		if err != nil {
			t.Fatalf("size %d: %v", size, err)
		}
		if read != int64(size) {
			t.Errorf("size %d: read = %d", size, read)
		}
		if want := SHA256Hex(data); digest != want {
			t.Errorf("size %d: digest = %q, want %q", size, digest, want)
		}
	}
}

// TestInternalStreamDigestEnforcesLimit は上限を固定する。
//
// 上限ちょうどは通し、超過だけを拒否する。上限なしで読むと、壊れたpayloadや
// symlink差し替えで無制限にreadしうる。
func TestInternalStreamDigestEnforcesLimit(t *testing.T) {
	data := bytes.Repeat([]byte("y"), 100)
	if _, _, err := InternalStreamDigest(bytes.NewReader(data), 100); err != nil {
		t.Errorf("上限ちょうどが拒否された: %v", err)
	}
	if _, _, err := InternalStreamDigest(bytes.NewReader(data), 99); err == nil {
		t.Error("上限超過が通った")
	}
	if _, _, err := InternalStreamDigest(bytes.NewReader(data), 0); err == nil {
		t.Error("上限0が通った")
	}
	if _, _, err := InternalStreamDigest(nil, 100); err == nil {
		t.Error("reader未設定が通った")
	}
}

// TestInternalStreamDigestReportsReadError は読取り失敗を伝えることを固定する。
//
// 途中まで読めた内容のdigestを返すと、壊れたfileの記録が正常値として残る。
func TestInternalStreamDigestReportsReadError(t *testing.T) {
	failing := iotest.ErrReader(errors.New("読取り失敗"))
	if _, _, err := InternalStreamDigest(failing, 1<<20); err == nil {
		t.Fatal("読取り失敗で成功した")
	}
}
