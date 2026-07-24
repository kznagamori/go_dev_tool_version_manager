package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestNewToolID_Valid(t *testing.T) {
	valid := []string{"go", "node", "cmake", "android-sdk", "a1", "x-y-z", "dotnet", "winlibs", "b2"}
	for _, s := range valid {
		got, err := NewToolID(s)
		if err != nil {
			t.Errorf("NewToolID(%q) 予期しないエラー: %v", s, err)
			continue
		}
		if got.String() != s {
			t.Errorf("NewToolID(%q).String() = %q", s, got.String())
		}
		if got.IsZero() {
			t.Errorf("NewToolID(%q).IsZero() = true", s)
		}
	}
}

func TestNewToolID_Invalid(t *testing.T) {
	cases := map[string]string{
		"empty":           "",
		"too short":       "a",
		"too long":        strings.Repeat("a", 41),
		"reserved":        "disabled",
		"uppercase":       "Go",
		"leading hyphen":  "-go",
		"trailing hyphen": "go-",
		"double hyphen":   "go--x",
		"underscore":      "go_x",
		"leading digit":   "1go",
		"dot":             "go.x",
		"space":           "go x",
	}
	for name, s := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := NewToolID(s)
			if err == nil {
				t.Fatalf("NewToolID(%q) はエラーになるべき", s)
			}
			if !errors.Is(err, ErrInvalidToolID) {
				t.Fatalf("NewToolID(%q) のエラーは ErrInvalidToolID を包むべき: %v", s, err)
			}
		})
	}
}

func TestToolID_ZeroValue(t *testing.T) {
	var z ToolID
	if !z.IsZero() {
		t.Error("ゼロ値 ToolID.IsZero() = false")
	}
	if z.String() != "" {
		t.Errorf("ゼロ値 ToolID.String() = %q", z.String())
	}
}
