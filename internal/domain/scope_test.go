package domain

import (
	"errors"
	"testing"
)

func TestNewScope(t *testing.T) {
	for _, s := range []string{"user", "project"} {
		if _, err := NewScope(s); err != nil {
			t.Errorf("NewScope(%q): %v", s, err)
		}
	}
	if _, err := NewScope("global"); !errors.Is(err, ErrInvalidScope) {
		t.Errorf("NewScope(global) は ErrInvalidScope: %v", err)
	}
}

func TestNewMode(t *testing.T) {
	for _, s := range []string{"portable", "user", "multi-user"} {
		if _, err := NewMode(s); err != nil {
			t.Errorf("NewMode(%q): %v", s, err)
		}
	}
	if _, err := NewMode("system"); !errors.Is(err, ErrInvalidMode) {
		t.Errorf("NewMode(system) は ErrInvalidMode: %v", err)
	}
}

func TestNewChannel(t *testing.T) {
	for _, s := range []string{"stable", "prerelease", "nightly", "eol"} {
		if _, err := NewChannel(s); err != nil {
			t.Errorf("NewChannel(%q): %v", s, err)
		}
	}
	if _, err := NewChannel("beta"); !errors.Is(err, ErrInvalidChannel) {
		t.Errorf("NewChannel(beta) は ErrInvalidChannel: %v", err)
	}
}
