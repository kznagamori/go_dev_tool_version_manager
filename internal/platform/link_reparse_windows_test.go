//go:build windows

package platform

import (
	"testing"

	"golang.org/x/sys/windows"
)

// TestReparseConstantsMatchSystemHeaders はlink_reparse.goの定数がOS定義と一致する
// ことを固定する。
//
// 定数を自前で持つのは、reparse bufferの組立てと解釈をplatform非依存のfileへ置き、
// 境界検査を両OSのtestで確かめるためである。**値がずれると、Linux側のtestは通った
// ままWindowsで別種のreparse pointを作る。** その食い違いをここで止める。
func TestReparseConstantsMatchSystemHeaders(t *testing.T) {
	t.Parallel()
	if reparseTagMountPoint != windows.IO_REPARSE_TAG_MOUNT_POINT {
		t.Errorf("mount point tag = 0x%08X, want 0x%08X",
			reparseTagMountPoint, uint32(windows.IO_REPARSE_TAG_MOUNT_POINT))
	}
	if reparseTagSymlink != windows.IO_REPARSE_TAG_SYMLINK {
		t.Errorf("symlink tag = 0x%08X, want 0x%08X",
			reparseTagSymlink, uint32(windows.IO_REPARSE_TAG_SYMLINK))
	}
	if reparseMaxDataSize != windows.MAXIMUM_REPARSE_DATA_BUFFER_SIZE {
		t.Errorf("max size = %d, want %d",
			reparseMaxDataSize, windows.MAXIMUM_REPARSE_DATA_BUFFER_SIZE)
	}
}
