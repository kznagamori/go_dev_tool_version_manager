//go:build windows

package platform

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain/port"
)

// createJunction はdirectory junctionを作る。
//
// **Go標準libraryにjunctionを作るAPIが無い。** [os.Symlink]はsymbolic linkを作り、
// docs/09-platform.md §3.2が求めるdirectory junctionにはならない。両者はWindows上
// で別のreparse tagを持ち、標準userがsymlinkを作るには特権かDeveloper Modeが要る
// のに対しjunctionは要らない。したがって`FSCTL_SET_REPARSE_POINT`を直接呼ぶ。
//
// junctionは「空のdirectoryへreparse pointを付ける」形で作る。途中で失敗したら
// 作ったdirectoryを消す —— 残すと、次回の作成が「既に存在する」で失敗し、
// 中身のない`current`が居座る。
func createJunction(linkPath, targetDir string) error {
	buffer, err := mountPointReparseData(targetDir)
	if err != nil {
		return err
	}
	if err := os.Mkdir(linkPath, 0o700); err != nil {
		return fmt.Errorf("platform: junctionのdirectoryを作れない: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = os.Remove(linkPath)
		}
	}()

	pathPtr, err := windows.UTF16PtrFromString(linkPath)
	if err != nil {
		return fmt.Errorf("platform: junction pathをUTF-16へ変換できない: %w", err)
	}
	// FILE_FLAG_OPEN_REPARSE_POINTが無いとreparse pointを辿った先が開く。
	// FILE_FLAG_BACKUP_SEMANTICSが無いとdirectory handleを取得できない。
	handle, err := windows.CreateFile(
		pathPtr,
		windows.GENERIC_WRITE,
		0,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_BACKUP_SEMANTICS,
		0,
	)
	if err != nil {
		return &os.PathError{Op: "CreateFile", Path: linkPath, Err: err}
	}
	defer func() { _ = windows.CloseHandle(handle) }()

	var returned uint32
	if err := windows.DeviceIoControl(
		handle, windows.FSCTL_SET_REPARSE_POINT,
		&buffer[0], uint32(len(buffer)), nil, 0, &returned, nil,
	); err != nil {
		return &os.PathError{Op: "FSCTL_SET_REPARSE_POINT", Path: linkPath, Err: err}
	}
	committed = true
	return nil
}

// linkKind はpath自体のlink種別を返す。
//
// reparse point属性を先に見る。junctionはdirectory属性も持つため、directoryかどうか
// で先に振り分けるとjunctionを通常のdirectoryとして扱ってしまう。
func linkKind(path string) (port.LinkKind, error) {
	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return port.LinkNone, fmt.Errorf("platform: pathをUTF-16へ変換できない: %w", err)
	}
	attrs, err := windows.GetFileAttributes(pathPtr)
	if err != nil {
		return port.LinkNone, &os.PathError{Op: "GetFileAttributes", Path: path, Err: err}
	}
	if attrs&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		tag, _, err := readReparsePoint(path)
		if err != nil {
			return port.LinkNone, err
		}
		switch tag {
		case reparseTagMountPoint:
			return port.LinkJunction, nil
		case reparseTagSymlink:
			return port.LinkSymlink, nil
		default:
			return port.LinkNone, fmt.Errorf("%w: tag=0x%08X path=%s", ErrUnknownReparse, tag, path)
		}
	}
	if attrs&windows.FILE_ATTRIBUTE_DIRECTORY != 0 {
		return port.LinkNone, nil
	}
	links, err := fileLinkCount(path)
	if err != nil {
		return port.LinkNone, err
	}
	if links > 1 {
		return port.LinkHardlink, nil
	}
	return port.LinkNone, nil
}

// readLinkTarget はjunctionまたはsymlinkが保存しているtargetを返す。
func readLinkTarget(path string) (string, error) {
	kind, err := linkKind(path)
	if err != nil {
		return "", err
	}
	if kind != port.LinkJunction && kind != port.LinkSymlink {
		return "", fmt.Errorf("%w: %s", ErrNotLink, path)
	}
	_, target, err := readReparsePoint(path)
	if err != nil {
		return "", err
	}
	return target, nil
}

// readReparsePoint はreparse tagとSubstituteNameを返す。
func readReparsePoint(path string) (uint32, string, error) {
	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, "", fmt.Errorf("platform: pathをUTF-16へ変換できない: %w", err)
	}
	handle, err := windows.CreateFile(
		pathPtr,
		windows.GENERIC_READ,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_BACKUP_SEMANTICS,
		0,
	)
	if err != nil {
		return 0, "", &os.PathError{Op: "CreateFile", Path: path, Err: err}
	}
	defer func() { _ = windows.CloseHandle(handle) }()

	buffer := make([]byte, reparseMaxDataSize)
	var returned uint32
	if err := windows.DeviceIoControl(
		handle, windows.FSCTL_GET_REPARSE_POINT,
		nil, 0, &buffer[0], uint32(len(buffer)), &returned, nil,
	); err != nil {
		return 0, "", &os.PathError{Op: "FSCTL_GET_REPARSE_POINT", Path: path, Err: err}
	}
	// filesystemが埋めた範囲だけを渡す。buffer全体を渡すと、末尾の未初期化領域を
	// name fieldの範囲内と誤認しうる。
	return parseReparsePoint(buffer[:returned])
}

// fileLinkCount はfileを指す名前の数を返す。
func fileLinkCount(path string) (uint32, error) {
	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, fmt.Errorf("platform: pathをUTF-16へ変換できない: %w", err)
	}
	handle, err := windows.CreateFile(
		pathPtr,
		0,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_FLAG_BACKUP_SEMANTICS,
		0,
	)
	if err != nil {
		return 0, &os.PathError{Op: "CreateFile", Path: path, Err: err}
	}
	defer func() { _ = windows.CloseHandle(handle) }()

	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil {
		return 0, &os.PathError{Op: "GetFileInformationByHandle", Path: path, Err: err}
	}
	return info.NumberOfLinks, nil
}
