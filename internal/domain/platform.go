package domain

import "fmt"

// OS は対象OSである（docs/06-tool-definition.md §5）。
type OS string

// OS の値。v0.1の対象はこの2件だけである。
const (
	OSWindows OS = "windows"
	OSLinux   OS = "linux"
)

// Arch は対象architectureである。v0.1は`amd64`だけを対象とする。
type Arch string

// Arch の値。
const ArchAMD64 Arch = "amd64"

// Libc はC標準libraryの識別子である。Windowsは`none`、Linuxは`glibc`とする。
type Libc string

// Libc の値。
const (
	LibcNone  Libc = "none"
	LibcGlibc Libc = "glibc"
)

// Platform は対象platformである（docs/02-architecture.md §3）。
//
// IDとOS/arch/libcの組は docs/06-tool-definition.md §5 の表どおりに固定し、
// 同一tupleは1件だけとする。unsupported placeholderを持たない。
type Platform struct {
	id   string
	os   OS
	arch Arch
	libc Libc
}

// PlatformID の値。schema 1が対応する2件だけを表す。
const (
	PlatformWindowsAMD64    = "windows-amd64"
	PlatformLinuxAMD64Glibc = "linux-amd64-glibc"
)

// platforms はIDからtupleへの固定表である。
// 組合せを実行時に合成せず表で閉じることで、仕様にない組が現れないようにする。
var platforms = map[string]Platform{
	PlatformWindowsAMD64:    {id: PlatformWindowsAMD64, os: OSWindows, arch: ArchAMD64, libc: LibcNone},
	PlatformLinuxAMD64Glibc: {id: PlatformLinuxAMD64Glibc, os: OSLinux, arch: ArchAMD64, libc: LibcGlibc},
}

// ParsePlatform はplatform IDをPlatformへ変換する。
func ParsePlatform(id string) (Platform, error) {
	platform, ok := platforms[id]
	if !ok {
		return Platform{}, fmt.Errorf(
			"domain: platform id %q は %s|%s のいずれでもない",
			id, PlatformWindowsAMD64, PlatformLinuxAMD64Glibc)
	}
	return platform, nil
}

// ID はplatform IDを返す。
func (p Platform) ID() string { return p.id }

// OS は対象OSを返す。
func (p Platform) OS() OS { return p.os }

// Arch は対象architectureを返す。
func (p Platform) Arch() Arch { return p.arch }

// Libc は対象libcを返す。
func (p Platform) Libc() Libc { return p.libc }

// ExecutableSuffix は実行形式のfile名suffixを返す。
//
// Windowsは`.exe`、Linuxは空である。command名からexecutable名を組み立てる箇所で
// OS判定を散らさないよう、domain値として持つ（docs/02-architecture.md §3）。
func (p Platform) ExecutableSuffix() string {
	if p.os == OSWindows {
		return ".exe"
	}
	return ""
}

// IsZero はParsePlatformを通していない値かどうかを返す。
func (p Platform) IsZero() bool { return p.id == "" }
