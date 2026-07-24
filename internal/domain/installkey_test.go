package domain

import (
	"errors"
	"testing"
)

func mustToolID(t *testing.T, s string) ToolID {
	t.Helper()
	id, err := NewToolID(s)
	if err != nil {
		t.Fatalf("NewToolID(%q): %v", s, err)
	}
	return id
}

func mustVersion(t *testing.T, s string) Version {
	t.Helper()
	v, err := NewVersion(s)
	if err != nil {
		t.Fatalf("NewVersion(%q): %v", s, err)
	}
	return v
}

func mustPlatform(t *testing.T, os OS, arch Arch, libc Libc, variant string) Platform {
	t.Helper()
	p, err := NewPlatform(os, arch, libc, variant)
	if err != nil {
		t.Fatalf("NewPlatform: %v", err)
	}
	return p
}

func TestNewInstallKey(t *testing.T) {
	id := mustToolID(t, "node")
	ver := mustVersion(t, "22.18.0")
	pf := mustPlatform(t, OSWindows, ArchAMD64, LibcNone, "")
	k, err := NewInstallKey(id, ver, pf)
	if err != nil {
		t.Fatalf("NewInstallKey: %v", err)
	}
	if k.ToolID() != id || k.Version() != ver || k.Platform() != pf {
		t.Error("InstallKey の要素が一致しない")
	}
	want := "node@22.18.0#windows/amd64/none/default"
	if k.String() != want {
		t.Errorf("String = %q, want %q", k.String(), want)
	}
}

func TestNewInstallKey_Invalid(t *testing.T) {
	id := mustToolID(t, "node")
	ver := mustVersion(t, "22.18.0")
	pf := mustPlatform(t, OSWindows, ArchAMD64, LibcNone, "")

	if _, err := NewInstallKey(ToolID{}, ver, pf); !errors.Is(err, ErrInvalidSelection) {
		t.Errorf("tool ID 欠落は ErrInvalidSelection: %v", err)
	}
	if _, err := NewInstallKey(id, Version{}, pf); !errors.Is(err, ErrInvalidSelection) {
		t.Errorf("version 欠落は ErrInvalidSelection: %v", err)
	}
	if _, err := NewInstallKey(id, ver, Platform{}); !errors.Is(err, ErrInvalidSelection) {
		t.Errorf("platform 欠落は ErrInvalidSelection: %v", err)
	}
}

func TestInstallKey_Equal(t *testing.T) {
	id := mustToolID(t, "go")
	ver := mustVersion(t, "1.26.5")
	pf := mustPlatform(t, OSLinux, ArchAMD64, LibcAny, "")
	a, _ := NewInstallKey(id, ver, pf)
	b, _ := NewInstallKey(id, ver, pf)
	c, _ := NewInstallKey(id, mustVersion(t, "1.26.4"), pf)
	if !a.Equal(b) {
		t.Error("同一要素の InstallKey は Equal であるべき")
	}
	if a.Equal(c) {
		t.Error("version 違いの InstallKey は Equal でないべき")
	}
}
