//go:build !windows

package platform

import (
	"fmt"

	"golang.org/x/sys/unix"
)

// collectHostFacts はuname(2)とloader観測でhost情報を集める。
//
// docs/09-platform.md §5.3「OS/archはkernel API…で判定し、環境変数や`ldd`表示
// だけで決めない」。**`runtime.GOOS`／`runtime.GOARCH`を使わない** —— どちらも
// buildした対象を表す定数であり、hostの実体ではない。qemuやWindows on ARMの
// x64 emulationでは、amd64 binaryがarm64 kernel上で動く。
func collectHostFacts() (HostFacts, error) {
	var utsname unix.Utsname
	if err := unix.Uname(&utsname); err != nil {
		return HostFacts{}, fmt.Errorf("platform: uname(2)に失敗した: %w", err)
	}
	return HostFacts{
		KernelOS:      unix.ByteSliceToString(utsname.Sysname[:]),
		KernelArch:    unix.ByteSliceToString(utsname.Machine[:]),
		LibcEvidences: probeLibc(linuxLibcProbes),
		// Linuxに`MAX_PATH`相当の切替は無い。pathの長さはfilesystemの上限だけで
		// 決まるため、利用可否という概念が無い。
		LongPathsEnabled: true,
	}, nil
}
