package platform

import (
	"encoding/binary"
	"fmt"
	"strings"
	"unicode/utf16"
)

// reparse pointのwire形式（winnt.hのREPARSE_DATA_BUFFER）。
//
// **byte組立てと解釈をplatform非依存のfileへ置く。** ここはOS APIを呼ばない純粋な
// 変換であり、Windowsでしかbuildしない場所へ置くと、境界検査をLinux側のtestで
// 確かめられない。数値はwinnt.hが公開する固定値である（Windows側でx/sysの定義と
// 一致することをtestで固定する）。
const (
	// reparseTagMountPoint はdirectory junctionのtagである。
	reparseTagMountPoint uint32 = 0xA0000003
	// reparseTagSymlink はsymbolic linkのtagである。
	reparseTagSymlink uint32 = 0xA000000C

	// reparseHeaderSize は`ReparseTag`／`ReparseDataLength`／`Reserved`の計である。
	reparseHeaderSize = 8
	// reparseNameFieldsSize は4つのname offset/length fieldの計である。
	reparseNameFieldsSize = 8
	// reparseSymlinkFlagsSize はsymlinkだけが持つ`Flags`の大きさである。
	reparseSymlinkFlagsSize = 4
	// reparseMaxDataSize はreparse bufferの上限である（MAXIMUM_REPARSE_DATA_BUFFER_SIZE）。
	reparseMaxDataSize = 16 * 1024

	// substitutePrefix はSubstituteNameが持つNT object manager prefixである。
	substitutePrefix = `\??\`
	// volumeRootLength は`C:\`の長さである。末尾separatorを削ってよいかの判定に使う。
	volumeRootLength = 3
)

// mountPointReparseData はjunction用のREPARSE_DATA_BUFFERを組み立てる。
//
// path bufferはSubstituteNameとPrintNameをそれぞれNUL終端で連結し、offset/lengthは
// **NULを含めない** byte数で表す（winnt.hのMountPointReparseBuffer）。
// SubstituteNameはNT object manager名なので`\??\`を前置する。
func mountPointReparseData(targetDir string) ([]byte, error) {
	if strings.ContainsRune(targetDir, 0) {
		// NULはUTF-16列の終端であり、path中にあるとSubstituteNameがそこで切れる。
		// 切れた先を指すjunctionができる。
		return nil, fmt.Errorf("platform: junction targetにNULが含まれる")
	}
	substitute := utf16NulTerminated(substitutePrefix + targetDir)
	printName := utf16NulTerminated(targetDir)

	pathBytes := (len(substitute) + len(printName)) * 2
	total := reparseHeaderSize + reparseNameFieldsSize + pathBytes
	if total > reparseMaxDataSize {
		// 上限を超えたbufferはDeviceIoControlが受け取らない。作れないことを
		// 呼出し側へ返す。長いpathを黙って切り詰めると別のdirectoryを指す。
		return nil, fmt.Errorf(
			"platform: junction targetが長すぎる（reparse buffer %d byte > 上限%d byte）",
			total, reparseMaxDataSize)
	}

	buffer := make([]byte, total)
	binary.LittleEndian.PutUint32(buffer[0:4], reparseTagMountPoint)
	// ReparseDataLengthはheaderより後ろのbyte数である。
	binary.LittleEndian.PutUint16(buffer[4:6], uint16(reparseNameFieldsSize+pathBytes))
	// buffer[6:8]はReservedで0のままにする。
	binary.LittleEndian.PutUint16(buffer[8:10], 0)
	binary.LittleEndian.PutUint16(buffer[10:12], uint16((len(substitute)-1)*2))
	binary.LittleEndian.PutUint16(buffer[12:14], uint16(len(substitute)*2))
	binary.LittleEndian.PutUint16(buffer[14:16], uint16((len(printName)-1)*2))

	offset := reparseHeaderSize + reparseNameFieldsSize
	for _, unit := range append(substitute, printName...) {
		binary.LittleEndian.PutUint16(buffer[offset:offset+2], unit)
		offset += 2
	}
	return buffer, nil
}

// parseReparsePoint はreparse bufferからtagとSubstituteNameを取り出す。
//
// **PrintNameではなくSubstituteNameを読む。** PrintNameは表示用で空でもよく、
// 実際に解決へ使われるのはSubstituteNameだからである。junctionの`\??\`前置は
// 保存形式であって利用者が指定したtargetの一部ではないため取り除く。
//
// offsetとlengthはfilesystemが返した値である。**範囲を確かめずに切り出さない**
// —— 壊れたreparse pointがあった場合に範囲外を読む。
func parseReparsePoint(buffer []byte) (uint32, string, error) {
	if len(buffer) < reparseHeaderSize+reparseNameFieldsSize {
		return 0, "", fmt.Errorf("%w: bufferが%d byteしかない", ErrUnknownReparse, len(buffer))
	}
	tag := binary.LittleEndian.Uint32(buffer[0:4])
	pathStart := reparseHeaderSize + reparseNameFieldsSize
	switch tag {
	case reparseTagMountPoint:
	case reparseTagSymlink:
		// symlinkだけがname fieldの直後にFlagsを持つ。
		pathStart += reparseSymlinkFlagsSize
	default:
		return tag, "", fmt.Errorf("%w: tag=0x%08X", ErrUnknownReparse, tag)
	}
	offset := int(binary.LittleEndian.Uint16(buffer[8:10]))
	length := int(binary.LittleEndian.Uint16(buffer[10:12]))
	if offset%2 != 0 || length%2 != 0 {
		return tag, "", fmt.Errorf(
			"%w: SubstituteNameのoffset/lengthがUTF-16単位でない", ErrUnknownReparse)
	}
	start := pathStart + offset
	end := start + length
	if end > len(buffer) {
		return tag, "", fmt.Errorf(
			"%w: SubstituteNameがbufferの範囲外である", ErrUnknownReparse)
	}
	units := make([]uint16, 0, length/2)
	for i := start; i < end; i += 2 {
		units = append(units, binary.LittleEndian.Uint16(buffer[i:i+2]))
	}
	target := strings.TrimPrefix(string(utf16.Decode(units)), substitutePrefix)
	// volume mount point規約の末尾separatorを取り除く。`C:\`のようなvolume root
	// までは削らない —— 削ると別のpath（drive相対）になる。
	if len(target) > volumeRootLength {
		target = strings.TrimSuffix(target, `\`)
	}
	return tag, target, nil
}

// utf16NulTerminated はNUL終端のUTF-16列を返す。
func utf16NulTerminated(s string) []uint16 {
	return append(utf16.Encode([]rune(s)), 0)
}
