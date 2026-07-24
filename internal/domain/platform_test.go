package domain

import (
	"errors"
	"testing"
)

func TestNewOSArchLibc(t *testing.T) {
	if _, err := NewOS("windows"); err != nil {
		t.Errorf("NewOS(windows): %v", err)
	}
	if _, err := NewOS("darwin"); !errors.Is(err, ErrInvalidOS) {
		t.Errorf("NewOS(darwin) は ErrInvalidOS: %v", err)
	}
	if _, err := NewArch("arm64"); err != nil {
		t.Errorf("NewArch(arm64): %v", err)
	}
	if _, err := NewArch("386"); !errors.Is(err, ErrInvalidArch) {
		t.Errorf("NewArch(386) は ErrInvalidArch: %v", err)
	}
	if _, err := NewLibc("musl"); err != nil {
		t.Errorf("NewLibc(musl): %v", err)
	}
	if _, err := NewLibc("uclibc"); !errors.Is(err, ErrInvalidLibc) {
		t.Errorf("NewLibc(uclibc) は ErrInvalidLibc: %v", err)
	}
}

func TestDefaultLibcFor(t *testing.T) {
	if got := DefaultLibcFor(OSWindows); got != LibcNone {
		t.Errorf("DefaultLibcFor(windows) = %q, want none", got)
	}
	if got := DefaultLibcFor(OSLinux); got != LibcAny {
		t.Errorf("DefaultLibcFor(linux) = %q, want any", got)
	}
}

func TestNewPlatform_Defaults(t *testing.T) {
	// libc 空 → OS 既定、variant 空 → "default"。
	p, err := NewPlatform(OSLinux, ArchAMD64, "", "")
	if err != nil {
		t.Fatalf("NewPlatform: %v", err)
	}
	if p.Libc() != LibcAny {
		t.Errorf("libc = %q, want any", p.Libc())
	}
	if p.Variant() != DefaultVariant {
		t.Errorf("variant = %q, want %q", p.Variant(), DefaultVariant)
	}
	if p.ExecutableSuffix() != "" {
		t.Errorf("linux ExecutableSuffix = %q, want empty", p.ExecutableSuffix())
	}
}

func TestNewPlatform_WindowsSuffix(t *testing.T) {
	p, err := NewPlatform(OSWindows, ArchAMD64, "", "")
	if err != nil {
		t.Fatalf("NewPlatform: %v", err)
	}
	if p.Libc() != LibcNone {
		t.Errorf("windows libc = %q, want none", p.Libc())
	}
	if p.ExecutableSuffix() != ".exe" {
		t.Errorf("windows ExecutableSuffix = %q, want .exe", p.ExecutableSuffix())
	}
}

func TestNewPlatform_InvalidVariant(t *testing.T) {
	for _, v := range []string{"a/b", `a\b`, "a\x00b", "a\tb"} {
		if _, err := NewPlatform(OSLinux, ArchAMD64, LibcAny, v); !errors.Is(err, ErrInvalidVariant) {
			t.Errorf("NewPlatform variant=%q は ErrInvalidVariant: %v", v, err)
		}
	}
}

func TestNewPlatform_InvalidOSArch(t *testing.T) {
	if _, err := NewPlatform("plan9", ArchAMD64, LibcNone, ""); !errors.Is(err, ErrInvalidOS) {
		t.Errorf("NewPlatform os=plan9 は ErrInvalidOS: %v", err)
	}
	if _, err := NewPlatform(OSLinux, "riscv", LibcAny, ""); !errors.Is(err, ErrInvalidArch) {
		t.Errorf("NewPlatform arch=riscv は ErrInvalidArch: %v", err)
	}
}

func TestPlatform_String(t *testing.T) {
	p, _ := NewPlatform(OSWindows, ArchARM64, LibcNone, "gnu")
	want := "windows/arm64/none/gnu"
	if p.String() != want {
		t.Errorf("Platform.String() = %q, want %q", p.String(), want)
	}
}
