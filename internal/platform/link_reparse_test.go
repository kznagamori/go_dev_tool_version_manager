package platform

import (
	"encoding/binary"
	"errors"
	"strings"
	"testing"
	"unicode/utf16"
)

// reparse bufferの組立てと解釈はOS APIを呼ばない純粋な変換であり、両OSのtest
// 実行で確かめる。Windowsでしか動かないtestにすると、境界検査の退行がWindows
// jobまで見つからない。

func TestMountPointReparseDataRoundTrip(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"通常のpath":     `C:\Users\dev\gdtvm\installs\go\1.25.0`,
		"volume root": `D:\`,
		"空白入り":        `C:\Program Files\gdtvm`,
		"非ASCII":      `C:\ユーザー\gdtvm`,
	}
	for name, target := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			buffer, err := mountPointReparseData(target)
			if err != nil {
				t.Fatalf("mountPointReparseData: %v", err)
			}
			tag, got, err := parseReparsePoint(buffer)
			if err != nil {
				t.Fatalf("parseReparsePoint: %v", err)
			}
			if tag != reparseTagMountPoint {
				t.Errorf("tag = 0x%08X, want 0x%08X", tag, reparseTagMountPoint)
			}
			// 組立てた値がそのまま戻ること。戻らないとjunctionが別のdirectoryを
			// 指し、`current`が誤ったversionを解決する。
			if got != target {
				t.Errorf("target = %q, want %q", got, target)
			}
		})
	}
}

func TestMountPointReparseDataRejectsUnusableTarget(t *testing.T) {
	t.Parallel()
	t.Run("NUL入り", func(t *testing.T) {
		t.Parallel()
		// NULはUTF-16列の終端である。含んだまま書くとSubstituteNameがそこで
		// 切れ、切れた先のdirectoryを指すjunctionができる。
		if _, err := mountPointReparseData("C:\\a\x00b"); err == nil {
			t.Fatal("NULを含むtargetが通った")
		}
	})
	t.Run("上限超過", func(t *testing.T) {
		t.Parallel()
		// 黙って切り詰めると別のdirectoryを指す。
		if _, err := mountPointReparseData(`C:\` + strings.Repeat("a", reparseMaxDataSize)); err == nil {
			t.Fatal("上限を超えるtargetが通った")
		}
	})
}

func TestParseReparsePointRejectsMalformedBuffer(t *testing.T) {
	t.Parallel()

	// 正しいbufferを起点に、fieldを1つずつ壊す。
	valid, err := mountPointReparseData(`C:\target`)
	if err != nil {
		t.Fatalf("mountPointReparseData: %v", err)
	}

	corrupt := func(mutate func([]byte)) []byte {
		buffer := make([]byte, len(valid))
		copy(buffer, valid)
		mutate(buffer)
		return buffer
	}

	cases := map[string][]byte{
		"header未満": valid[:reparseHeaderSize],
		"未知のtag": corrupt(func(b []byte) {
			binary.LittleEndian.PutUint32(b[0:4], 0xA000001B)
		}),
		"offsetが奇数": corrupt(func(b []byte) {
			binary.LittleEndian.PutUint16(b[8:10], 1)
		}),
		"lengthが奇数": corrupt(func(b []byte) {
			binary.LittleEndian.PutUint16(b[10:12], 3)
		}),
		"lengthが範囲外": corrupt(func(b []byte) {
			binary.LittleEndian.PutUint16(b[10:12], 0xFFFE)
		}),
		"offsetが範囲外": corrupt(func(b []byte) {
			binary.LittleEndian.PutUint16(b[8:10], 0xFFFE)
		}),
	}
	for name, buffer := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			// 範囲を確かめずに切り出すとここでpanicする。errorで返ること自体が
			// 検査対象である。
			_, _, err := parseReparsePoint(buffer)
			if !errors.Is(err, ErrUnknownReparse) {
				t.Fatalf("error = %v, want ErrUnknownReparse", err)
			}
		})
	}
}

func TestParseReparsePointReadsSymlinkLayout(t *testing.T) {
	t.Parallel()
	// symlinkだけがname fieldの直後にFlagsを持つ。junctionと同じoffsetで読むと
	// 4 byteずれたpathを返す。
	target := `C:\target`
	substitute := utf16NulTerminated(substitutePrefix + target)
	printName := utf16NulTerminated(target)
	pathBytes := (len(substitute) + len(printName)) * 2

	buffer := make([]byte,
		reparseHeaderSize+reparseNameFieldsSize+reparseSymlinkFlagsSize+pathBytes)
	binary.LittleEndian.PutUint32(buffer[0:4], reparseTagSymlink)
	binary.LittleEndian.PutUint16(buffer[4:6],
		uint16(reparseNameFieldsSize+reparseSymlinkFlagsSize+pathBytes))
	binary.LittleEndian.PutUint16(buffer[8:10], 0)
	binary.LittleEndian.PutUint16(buffer[10:12], uint16((len(substitute)-1)*2))
	binary.LittleEndian.PutUint16(buffer[12:14], uint16(len(substitute)*2))
	binary.LittleEndian.PutUint16(buffer[14:16], uint16((len(printName)-1)*2))
	offset := reparseHeaderSize + reparseNameFieldsSize + reparseSymlinkFlagsSize
	for _, unit := range append(substitute, printName...) {
		binary.LittleEndian.PutUint16(buffer[offset:offset+2], unit)
		offset += 2
	}

	tag, got, err := parseReparsePoint(buffer)
	if err != nil {
		t.Fatalf("parseReparsePoint: %v", err)
	}
	if tag != reparseTagSymlink {
		t.Errorf("tag = 0x%08X, want 0x%08X", tag, reparseTagSymlink)
	}
	if got != target {
		t.Errorf("target = %q, want %q", got, target)
	}
}

func TestParseReparsePointKeepsVolumeRoot(t *testing.T) {
	t.Parallel()
	// `C:\`から末尾separatorを削ると`C:`になり、drive相対pathという別の意味へ
	// 変わる。volume mount point規約の末尾separatorだけを削る。
	buffer, err := mountPointReparseData(`E:\`)
	if err != nil {
		t.Fatalf("mountPointReparseData: %v", err)
	}
	_, got, err := parseReparsePoint(buffer)
	if err != nil {
		t.Fatalf("parseReparsePoint: %v", err)
	}
	if got != `E:\` {
		t.Errorf("target = %q, want %q", got, `E:\`)
	}
}

func TestParseReparsePointTrimsTrailingSeparator(t *testing.T) {
	t.Parallel()
	// mklinkが作るjunctionはSubstituteNameを`\??\C:\dir\`の形で保存する。
	// 末尾separatorを残すと、保存値との比較が形式差で外れる。
	target := `C:\dir\`
	substitute := utf16NulTerminated(substitutePrefix + target)
	printName := utf16NulTerminated(target)
	pathBytes := (len(substitute) + len(printName)) * 2
	buffer := make([]byte, reparseHeaderSize+reparseNameFieldsSize+pathBytes)
	binary.LittleEndian.PutUint32(buffer[0:4], reparseTagMountPoint)
	binary.LittleEndian.PutUint16(buffer[4:6], uint16(reparseNameFieldsSize+pathBytes))
	binary.LittleEndian.PutUint16(buffer[8:10], 0)
	binary.LittleEndian.PutUint16(buffer[10:12], uint16((len(substitute)-1)*2))
	binary.LittleEndian.PutUint16(buffer[12:14], uint16(len(substitute)*2))
	binary.LittleEndian.PutUint16(buffer[14:16], uint16((len(printName)-1)*2))
	offset := reparseHeaderSize + reparseNameFieldsSize
	for _, unit := range append(substitute, printName...) {
		binary.LittleEndian.PutUint16(buffer[offset:offset+2], unit)
		offset += 2
	}

	_, got, err := parseReparsePoint(buffer)
	if err != nil {
		t.Fatalf("parseReparsePoint: %v", err)
	}
	if got != `C:\dir` {
		t.Errorf("target = %q, want %q", got, `C:\dir`)
	}
}

func TestUTF16NulTerminated(t *testing.T) {
	t.Parallel()
	got := utf16NulTerminated("ab")
	want := append(utf16.Encode([]rune("ab")), 0)
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unit[%d] = %d, want %d", i, got[i], want[i])
		}
	}
}
