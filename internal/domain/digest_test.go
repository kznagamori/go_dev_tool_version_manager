package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestNewSHA256Digest_Valid(t *testing.T) {
	hex := strings.Repeat("ab", 32) // 64 桁
	d, err := NewSHA256Digest(hex)
	if err != nil {
		t.Fatalf("NewSHA256Digest: %v", err)
	}
	if d.Algorithm() != AlgorithmSHA256 {
		t.Errorf("Algorithm = %q", d.Algorithm())
	}
	if d.Value() != hex {
		t.Errorf("Value = %q", d.Value())
	}
	if d.String() != "sha256:"+hex {
		t.Errorf("String = %q", d.String())
	}
}

func TestNewSHA256Digest_NormalizesUppercase(t *testing.T) {
	upper := strings.Repeat("AB", 32)
	d, err := NewSHA256Digest(upper)
	if err != nil {
		t.Fatalf("NewSHA256Digest(upper): %v", err)
	}
	if d.Value() != strings.ToLower(upper) {
		t.Errorf("大文字 hex を小文字へ正規化するべき: %q", d.Value())
	}
}

func TestNewSHA256Digest_Invalid(t *testing.T) {
	cases := map[string]string{
		"too short": strings.Repeat("a", 63),
		"too long":  strings.Repeat("a", 65),
		"non hex":   strings.Repeat("g", 64),
		"empty":     "",
	}
	for name, s := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := NewSHA256Digest(s); !errors.Is(err, ErrInvalidDigest) {
				t.Fatalf("NewSHA256Digest(%q) は ErrInvalidDigest: %v", name, err)
			}
		})
	}
}

func TestDigest_Equal(t *testing.T) {
	a, _ := NewSHA256Digest(strings.Repeat("ab", 32))
	b, _ := NewSHA256Digest(strings.Repeat("AB", 32))
	c, _ := NewSHA256Digest(strings.Repeat("cd", 32))
	if !a.Equal(b) {
		t.Error("正規化後に等しい digest は Equal であるべき")
	}
	if a.Equal(c) {
		t.Error("異なる digest は Equal でないべき")
	}
}
