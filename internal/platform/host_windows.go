//go:build windows

package platform

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// PROCESSOR_ARCHITECTURE値（winnt.h）。
//
// `GetNativeSystemInfo`が返すのは**native**のarchitectureであり、WOW64や
// x64 emulationで動いていてもhost本来の値になる。
const (
	processorArchitectureAMD64 = 9
	processorArchitectureARM64 = 12
	processorArchitectureIntel = 0
	processorArchitectureARM   = 5
	processorArchitectureIA64  = 6
)

// processorArchitectureNames はPROCESSOR_ARCHITECTURE値をkernel machine名へ直す表
// である。
//
// [ClassifyHost]がOSに依らず同じ語彙で判定できるよう、uname(2)のmachine名へ
// 揃える。対象外のarchも名前を持たせるのは、**「未知の値」ではなく「arm64は対象外」
// と述べられるようにする**ためである。
var processorArchitectureNames = map[uint16]string{
	processorArchitectureAMD64: "x86_64",
	processorArchitectureARM64: "arm64",
	processorArchitectureIntel: "x86",
	processorArchitectureARM:   "arm",
	processorArchitectureIA64:  "ia64",
}

// systemInfo はGetNativeSystemInfoが埋めるSYSTEM_INFOである。
//
// golang.org/x/sys/windows はこの構造体を公開していないため自前で宣言する。
// 使うのは先頭のProcessorArchitectureだけだが、APIが全体を書き込むため
// 省略せず全fieldを持つ。
type systemInfo struct {
	ProcessorArchitecture     uint16
	Reserved                  uint16
	PageSize                  uint32
	MinimumApplicationAddress uintptr
	MaximumApplicationAddress uintptr
	ActiveProcessorMask       uintptr
	NumberOfProcessors        uint32
	ProcessorType             uint32
	AllocationGranularity     uint32
	ProcessorLevel            uint16
	ProcessorRevision         uint16
}

// collectHostFacts はGetNativeSystemInfoでhost情報を集める。
//
// docs/09-platform.md §5.3「OS/archはkernel APIで判定」。**`GetSystemInfo`では
// なく`GetNativeSystemInfo`を使う** —— 前者はemulation下のprocessから見た値を
// 返すため、arm64 host上のx64 emulationでamd64と報告する。それでは§1が非対応と
// 定めるarm64を対象と誤判定する。
func collectHostFacts() (HostFacts, error) {
	kernel32 := windows.NewLazySystemDLL("kernel32.dll")
	getNativeSystemInfo := kernel32.NewProc("GetNativeSystemInfo")
	if err := getNativeSystemInfo.Find(); err != nil {
		return HostFacts{}, fmt.Errorf("platform: GetNativeSystemInfoが見つからない: %w", err)
	}
	var info systemInfo
	// GetNativeSystemInfoは戻り値を持たない（VOID）。エラーはFindで検出済みである。
	_, _, _ = getNativeSystemInfo.Call(uintptr(unsafe.Pointer(&info)))

	arch, known := processorArchitectureNames[info.ProcessorArchitecture]
	if !known {
		// 表に無い値もそのまま載せる。ClassifyHostが対象外と判定し、利用者へ
		// 何が返ってきたかを示せる。
		arch = fmt.Sprintf("unknown-0x%04X", info.ProcessorArchitecture)
	}
	return HostFacts{
		KernelOS:   "windows",
		KernelArch: arch,
		// WindowsにC標準libraryの選択は無い。観測しない。
		LibcEvidences:    nil,
		LongPathsEnabled: longPathsEnabled(),
	}, nil
}

// longPathsEnabled はMAX_PATHを超えるpathを扱えるかを返す。
//
// docs/09-platform.md §3.1「long path利用可否を**API**で扱い、`MAX_PATH`へ暗黙
// truncateしない」。`RtlAreLongPathsEnabled`はmanifestとregistryの両方を反映した
// 実効値を返す。**registry値を自分で読まない** —— 読んだ値とprocessの実効値は
// manifest次第で食い違い、扱えるつもりで扱えないpathが生まれる。
//
// 関数が存在しない古いWindowsではlong pathを扱えないため、falseとする。
func longPathsEnabled() bool {
	ntdll := windows.NewLazySystemDLL("ntdll.dll")
	areEnabled := ntdll.NewProc("RtlAreLongPathsEnabled")
	if err := areEnabled.Find(); err != nil {
		return false
	}
	result, _, _ := areEnabled.Call()
	return result != 0
}
