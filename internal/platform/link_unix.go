//go:build !windows

package platform

import (
	"errors"
	"fmt"
	"os"
	"syscall"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain/port"
)

// createJunction はLinuxでは常に失敗する。
//
// docs/09-platform.md §5.1はLinuxのcurrentをrelative symlinkと定めており、
// junctionに相当する実体が無い。**symlinkで代替しない** —— 呼出し側が
// junctionを要求したのは、そのplatformの規則がjunctionだからである。
func createJunction(linkPath, targetDir string) error {
	_, _ = linkPath, targetDir
	return fmt.Errorf("%w: junctionはWindows専用である", ErrLinkUnsupported)
}

// linkKind はpath自体のlink種別を返す。
//
// symlinkを先に判定する。symlinkは[os.FileMode]で分かり、link countを見るまでも
// ないためである。regular fileはlink countが2以上ならhardlinkとする
// （docs/09-platform.md §3.3の公開command shimと同じ判定をLinuxでも行う）。
func linkKind(path string) (port.LinkKind, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return port.LinkNone, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return port.LinkSymlink, nil
	}
	if !info.Mode().IsRegular() {
		// directory、device、socket等。linkではない。
		return port.LinkNone, nil
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		// 判定できないまま「linkではない」と答えると、RemoveLinkが実体を消す
		// 経路を開く。分からないことをerrorにする。
		return port.LinkNone, errors.New("platform: stat結果からlink countを取得できない")
	}
	if stat.Nlink > 1 {
		return port.LinkHardlink, nil
	}
	return port.LinkNone, nil
}

// readLinkTarget はsymlinkが保存しているtargetを返す。
func readLinkTarget(path string) (string, error) {
	kind, err := linkKind(path)
	if err != nil {
		return "", err
	}
	if kind != port.LinkSymlink {
		// hardlinkは「targetを保存するlink」ではなく、同じ実体への別名である。
		// 読み出せるtargetが存在しない。
		return "", fmt.Errorf("%w: %s", ErrNotLink, path)
	}
	return os.Readlink(path)
}
