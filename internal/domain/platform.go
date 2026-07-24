package domain

import (
	"fmt"
	"strings"
	"unicode"
)

// OS は対象オペレーティングシステムの enum である（06章4節）。将来値の追加は
// schema 更新を伴う。
type OS string

// Arch は対象アーキテクチャの enum である（06章4節）。
type Arch string

// Libc は Linux tool artifact 選択に用いる libc 種別の enum である（06章4節）。
type Libc string

// OS enum の値。
const (
	OSWindows OS = "windows"
	OSLinux   OS = "linux"
)

// Arch enum の値。
const (
	ArchAMD64 Arch = "amd64"
	ArchARM64 Arch = "arm64"
)

// Libc enum の値。
const (
	LibcAny   Libc = "any"
	LibcGlibc Libc = "glibc"
	LibcMusl  Libc = "musl"
	LibcNone  Libc = "none"
)

// NewOS は文字列を検証して OS を返す。未対応値は ErrInvalidOS を包んで返す。
func NewOS(s string) (OS, error) {
	switch OS(s) {
	case OSWindows, OSLinux:
		return OS(s), nil
	default:
		return "", fmt.Errorf("%w: %q", ErrInvalidOS, s)
	}
}

// NewArch は文字列を検証して Arch を返す。未対応値は ErrInvalidArch を包んで返す。
func NewArch(s string) (Arch, error) {
	switch Arch(s) {
	case ArchAMD64, ArchARM64:
		return Arch(s), nil
	default:
		return "", fmt.Errorf("%w: %q", ErrInvalidArch, s)
	}
}

// NewLibc は文字列を検証して Libc を返す。未対応値は ErrInvalidLibc を包んで返す。
func NewLibc(s string) (Libc, error) {
	switch Libc(s) {
	case LibcAny, LibcGlibc, LibcMusl, LibcNone:
		return Libc(s), nil
	default:
		return "", fmt.Errorf("%w: %q", ErrInvalidLibc, s)
	}
}

// DefaultLibcFor は OS ごとの既定 libc を返す（06章4節: Windows=none, Linux=any）。
func DefaultLibcFor(os OS) Libc {
	if os == OSWindows {
		return LibcNone
	}
	return LibcAny
}

// DefaultVariant は variant 省略時の既定 directory 名である（06章4節, 03章4節）。
const DefaultVariant = "default"

// Platform は OS、arch、libc、variant を束ねた artifact 選択の対象を表す。
// 実行形式 suffix は OS から導出する。
type Platform struct {
	os      OS
	arch    Arch
	libc    Libc
	variant string
}

// NewPlatform は各要素を検証して Platform を生成する。libc がゼロ値なら OS 既定を
// 用い、variant が空なら "default" を用いる。variant に path separator・制御文字・
// NUL を含む場合は ErrInvalidVariant を返す。
func NewPlatform(os OS, arch Arch, libc Libc, variant string) (Platform, error) {
	if _, err := NewOS(string(os)); err != nil {
		return Platform{}, err
	}
	if _, err := NewArch(string(arch)); err != nil {
		return Platform{}, err
	}
	if libc == "" {
		libc = DefaultLibcFor(os)
	} else if _, err := NewLibc(string(libc)); err != nil {
		return Platform{}, err
	}
	v, err := normalizeVariant(variant)
	if err != nil {
		return Platform{}, err
	}
	return Platform{os: os, arch: arch, libc: libc, variant: v}, nil
}

// normalizeVariant は variant を検査し、空なら既定値へ正規化する。
func normalizeVariant(s string) (string, error) {
	if s == "" {
		return DefaultVariant, nil
	}
	if strings.ContainsAny(s, `/\`) || strings.IndexByte(s, 0) >= 0 {
		return "", fmt.Errorf("%w: %q: path separator/NUL を含む", ErrInvalidVariant, s)
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return "", fmt.Errorf("%w: %q: 制御文字を含む", ErrInvalidVariant, s)
		}
	}
	return s, nil
}

// OS は対象 OS を返す。
func (p Platform) OS() OS { return p.os }

// Arch は対象アーキテクチャを返す。
func (p Platform) Arch() Arch { return p.arch }

// Libc は対象 libc を返す。
func (p Platform) Libc() Libc { return p.libc }

// Variant は variant 名を返す（既定は "default"）。
func (p Platform) Variant() string { return p.variant }

// ExecutableSuffix は OS に応じた実行ファイル拡張子を返す（Windows は ".exe"、
// それ以外は空文字）。
func (p Platform) ExecutableSuffix() string {
	if p.os == OSWindows {
		return ".exe"
	}
	return ""
}

// String は "os/arch/libc/variant" 形式の識別文字列を返す。
func (p Platform) String() string {
	return fmt.Sprintf("%s/%s/%s/%s", p.os, p.arch, p.libc, p.variant)
}
