package domain

import (
	"errors"
	"testing"
)

func TestNewVersion_Valid(t *testing.T) {
	valid := []string{"1.20.0", "22.18.0", "17.0.9+9", "1.88.0", "3.13.5", "go1.24.7-raw", "x86_64-14.2.0"}
	for _, s := range valid {
		got, err := NewVersion(s)
		if err != nil {
			t.Errorf("NewVersion(%q) 予期しないエラー: %v", s, err)
			continue
		}
		if got.String() != s {
			t.Errorf("NewVersion(%q).String() = %q", s, got.String())
		}
	}
}

func TestNewVersion_Invalid(t *testing.T) {
	cases := map[string]string{
		"empty":            "",
		"leading space":    " 1.20.0",
		"trailing space":   "1.20.0 ",
		"internal newline": "1.20\n0",
		"nul":              "1.20\x000",
		"tab":              "1.20\t0",
	}
	for name, s := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := NewVersion(s)
			if err == nil {
				t.Fatalf("NewVersion(%q) はエラーになるべき", s)
			}
			if !errors.Is(err, ErrInvalidVersion) {
				t.Fatalf("NewVersion(%q) のエラーは ErrInvalidVersion を包むべき: %v", s, err)
			}
		})
	}
}

func TestNewVersion_NoImplicitCompletion(t *testing.T) {
	// 08章2.1節: 版比較上等価でも文字列が異なるものを同一入力として扱わない。
	a, _ := NewVersion("22.18")
	b, _ := NewVersion("22.18.0")
	if a.String() == b.String() {
		t.Error("22.18 と 22.18.0 は別の文字列として扱うべき")
	}
}
