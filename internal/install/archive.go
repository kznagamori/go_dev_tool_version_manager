package install

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/security"
)

// archive上限（docs/04-storage-and-data.md §21）。
const (
	// ArchiveEntryMax はarchiveが持てるentry数である。
	ArchiveEntryMax = 200_000
	// ArchiveFileMaxBytes は展開後の単一fileの上限である。
	ArchiveFileMaxBytes int64 = 4 << 30
	// ArchiveTotalMaxBytes は展開後の総bytesの上限である。
	ArchiveTotalMaxBytes int64 = 20 << 30
	// ArchiveRatioMax は圧縮比の上限である（entry単位と全体の両方）。
	ArchiveRatioMax = 1_000
)

// EntryKind はarchive entryの種別である。
type EntryKind string

// EntryKind の値。展開してよいのはdirectoryとregular fileだけである。
const (
	// KindDir はdirectoryである。
	KindDir EntryKind = "dir"
	// KindFile はregular fileである。
	KindFile EntryKind = "file"
	// KindSymlink はsymbolic linkである。展開しない。
	KindSymlink EntryKind = "symlink"
	// KindHardlink はhard linkである。展開しない。
	KindHardlink EntryKind = "hardlink"
	// KindOther はdevice、fifo、socketなどである。展開しない。
	KindOther EntryKind = "other"
)

// Entry はarchiveが宣言する1 entryである。
//
// zipとtar.gzのどちらから作っても同じ検査を通せるよう、両方に共通する情報だけを
// 持つ。formatごとの差はこの型へ写す側が吸収する。
type Entry struct {
	// Name はarchiveが宣言するpathである。slash区切りで、正規化していない生の値。
	Name string
	// Kind は種別である。
	Kind EntryKind
	// Size は展開後のbyte数である（宣言値）。
	Size int64
	// CompressedSize は圧縮後のbyte数である。0は不明を表す。
	CompressedSize int64
}

// InspectRequest はentry事前検査の入力である。
type InspectRequest struct {
	// Entries はarchiveの全entryである。
	Entries []Entry
	// StripComponents は先頭から取り除くpath componentの数である（0か1）。
	StripComponents int
	// Host は展開先のplatformである。Windows規則の適用可否を決める。
	Host domain.Platform
}

// InspectResult は検査を通ったentryである。
type InspectResult struct {
	// Paths は`strip_components`適用後の相対path（slash区切り）である。
	//
	// 宣言順を保つ。展開はこの順で行い、directoryが先に来ることを前提にしない。
	Paths []string
	// TotalBytes は展開後の総bytes（宣言値）である。
	TotalBytes int64
}

// InspectEntries はarchiveの全entryを展開前に検査する。
//
// docs/10-security.md §5「archiveは展開前に全entryを検査し、absolute/drive/UNC、
// `..`、NUL/control、ADS、invalid/non-NFC Unicode、Windows予約名/case衝突、
// duplicate、symlink/hardlink/reparse、特殊file、size/count/ratio超過を拒否する」。
//
// **1件でも違反があれば展開しない。** 安全なentryだけを選んで展開すると、
// archiveが意図した構成と違うものが出来上がり、probeが何を検査しているか
// 分からなくなる。
//
// 宣言sizeでの検査は上限の第一段である。zip bombは宣言を偽れるため、
// 実展開bytesでの打ち切りは展開側が別に行う。
func InspectEntries(req InspectRequest) *domain.Error {
	if len(req.Entries) == 0 {
		return archiveUnsafe(errors.New("install: archiveにentryが無い"))
	}
	if len(req.Entries) > ArchiveEntryMax {
		return archiveUnsafe(fmt.Errorf(
			"install: entry数が上限%dを超える（%d件）", ArchiveEntryMax, len(req.Entries)))
	}
	if req.StripComponents < 0 || req.StripComponents > 1 {
		return archiveUnsafe(fmt.Errorf(
			"install: strip_componentsは0か1だけ（%d）", req.StripComponents))
	}

	windows := req.Host.OS() == domain.OSWindows
	var total int64
	// ratioExpanded とratioCompressed は圧縮比を出せるentryだけの合計である。
	//
	// 圧縮後sizeを持たないentry（tar）を分子だけへ足すと、比が実体と無関係に
	// 膨らむ。同じ母集団どうしで比べる。
	var ratioExpanded, ratioCompressed int64
	// entrySeen は完全一致の重複検出に使う。keyは正規化したpathである。
	entrySeen := make(map[string]struct{}, len(req.Entries))
	// prefixSeen はcase衝突の検出に使う。keyは正規化したpath prefix、値は
	// 最初に現れた実際の表記である。**file名だけでなくdirectory componentも
	// 見る。** `bin/go`と`BIN/other`はfile名が違ってもWindowsでは同じ
	// directoryを指し、展開後の構成が一意にならない。
	prefixSeen := make(map[string]string, len(req.Entries))
	paths := make([]string, 0, len(req.Entries))

	for index, entry := range req.Entries {
		name, err := checkEntry(entry, windows)
		if err != nil {
			return archiveUnsafe(fmt.Errorf("install: entry[%d] %q: %w", index, entry.Name, err))
		}

		stripped, keep, err := stripComponents(name, req.StripComponents)
		if err != nil {
			return archiveUnsafe(fmt.Errorf("install: entry[%d] %q: %w", index, entry.Name, err))
		}
		if !keep {
			// strip対象のtop-level directory自身は展開先を持たない。
			continue
		}

		// 完全一致の重複を拒否する。
		key := foldKey(stripped)
		if _, taken := entrySeen[key]; taken {
			return archiveUnsafe(fmt.Errorf(
				"install: entry[%d] %q が重複またはcase衝突している", index, entry.Name))
		}
		entrySeen[key] = struct{}{}

		// case衝突をpath prefix単位で見る。Windowsはcase insensitiveなので
		// `Bin/go.exe`と`bin/go.exe`が同じfileを指す。Linuxでも衝突させないのは、
		// 同じarchiveを両OSで同じ構成へ展開するためである。
		if previous, conflict := registerPrefixes(prefixSeen, stripped); conflict {
			return archiveUnsafe(fmt.Errorf(
				"install: entry[%d] %q が %q とcase衝突する", index, entry.Name, previous))
		}

		if entry.Kind == KindFile {
			if entry.Size > ArchiveFileMaxBytes {
				return archiveUnsafe(fmt.Errorf(
					"install: entry[%d] %q の展開後sizeが上限%d byteを超える（%d byte）",
					index, entry.Name, ArchiveFileMaxBytes, entry.Size))
			}
			// 加算overflowをfail closedで扱う（§21）。
			if total > ArchiveTotalMaxBytes-entry.Size {
				return archiveUnsafe(fmt.Errorf(
					"install: 展開後の総sizeが上限%d byteを超える", ArchiveTotalMaxBytes))
			}
			total += entry.Size
			if entry.CompressedSize > 0 {
				ratioExpanded += entry.Size
				ratioCompressed += entry.CompressedSize
			}

			if err := checkRatio(entry.Size, entry.CompressedSize); err != nil {
				return archiveUnsafe(fmt.Errorf(
					"install: entry[%d] %q: %w", index, entry.Name, err))
			}
		}
		paths = append(paths, stripped)
	}

	if len(paths) == 0 {
		return archiveUnsafe(errors.New("install: strip_components適用後にentryが残らない"))
	}
	// 全体の圧縮比。entryごとに閾値内でも、合計で異常な比になるarchiveを拒否する。
	if err := checkRatio(ratioExpanded, ratioCompressed); err != nil {
		return archiveUnsafe(fmt.Errorf("install: archive全体の%w", err))
	}
	return nil
}

// InspectEntriesResult は検査を通ったpathを返す。
//
// [InspectEntries]と同じ検査を行い、成功時に展開対象を返す。検査だけを行いたい
// 呼出しと、展開へ進む呼出しの両方があるため分けている。
func InspectEntriesResult(req InspectRequest) (InspectResult, *domain.Error) {
	if err := InspectEntries(req); err != nil {
		return InspectResult{}, err
	}
	// 検査を通ったので再走査は失敗しない。
	var result InspectResult
	for _, entry := range req.Entries {
		name, _ := checkEntry(entry, req.Host.OS() == domain.OSWindows)
		stripped, keep, _ := stripComponents(name, req.StripComponents)
		if !keep {
			continue
		}
		result.Paths = append(result.Paths, stripped)
		if entry.Kind == KindFile {
			result.TotalBytes += entry.Size
		}
	}
	return result, nil
}

// checkEntry は1 entryのnameと種別を検査し、正規化したnameを返す。
func checkEntry(entry Entry, windows bool) (string, error) {
	// 展開してよいのはdirectoryとregular fileだけである（§5）。symlink/hardlinkは
	// 展開先の外を指せるため、archiveの中に限っても許さない。
	switch entry.Kind {
	case KindDir, KindFile:
	case KindSymlink, KindHardlink:
		return "", fmt.Errorf("%s entryは展開しない", entry.Kind)
	case KindOther:
		return "", errors.New("device/fifo/socketなどの特殊entryは展開しない")
	default:
		return "", fmt.Errorf("未知のentry種別 %q", entry.Kind)
	}
	if entry.Size < 0 {
		return "", fmt.Errorf("sizeが負（%d）", entry.Size)
	}
	if entry.CompressedSize < 0 {
		return "", fmt.Errorf("圧縮後sizeが負（%d）", entry.CompressedSize)
	}

	name := entry.Name
	if name == "" {
		return "", errors.New("nameが空")
	}
	if !utf8.ValidString(name) {
		return "", errors.New("nameがUTF-8として不正")
	}
	// non-NFCを拒否する（§5）。正規化して受けると、archiveが宣言したnameと
	// 展開先のnameが違う状態になり、probeのrequired pathと突き合わせられない。
	if !norm.NFC.IsNormalString(name) {
		return "", errors.New("nameがNFC正規化されていない")
	}
	if strings.ContainsRune(name, 0) {
		return "", errors.New("nameにNULを含む")
	}
	for _, r := range name {
		if r < 0x20 || r == 0x7F {
			return "", fmt.Errorf("nameに制御文字 U+%04X を含む", r)
		}
		if unicode.Is(unicode.Cf, r) {
			// 表示上の文字順を変えてpathの意味を偽装できる。
			return "", fmt.Errorf("nameにformat制御文字 U+%04X を含む", r)
		}
	}
	// backslashはWindowsのseparatorであり、archive内では通常のname文字である。
	// 区別せず受けると、Linuxで作った`a\b`がWindowsで`a/b`になる。
	if strings.ContainsRune(name, '\\') {
		return "", errors.New("nameにbackslashを含む")
	}
	if strings.HasPrefix(name, "/") {
		return "", errors.New("absolute pathである")
	}
	if isDriveOrUNC(name) {
		return "", errors.New("drive letterまたはUNC pathである")
	}

	trimmed := strings.TrimSuffix(name, "/")
	if trimmed == "" {
		return "", errors.New("nameがrootを指す")
	}
	for _, component := range strings.Split(trimmed, "/") {
		// component単位の安全検査はinternal/securityへ委ねる。空、`.`/`..`、NUL、
		// 区切り混在、長さ、Windows予約名、ADS区切り、末尾空白/dotを見る。
		// 予約名listをここへ複製すると、規則を変えたときに片方だけが古いまま
		// になる（docs/02-architecture.md §2が「path検査」をinternal/securityの
		// 責務とする）。
		if err := security.ValidateComponent(component, windows); err != nil {
			return "", err
		}
	}
	return trimmed, nil
}

// isDriveOrUNC はWindowsのdrive相対pathとUNC pathを判定する。
func isDriveOrUNC(name string) bool {
	if strings.HasPrefix(name, "//") {
		return true
	}
	if len(name) >= 2 && name[1] == ':' {
		c := name[0]
		return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
	}
	return false
}

// stripComponents は先頭componentを取り除く。
//
// 2番目の戻り値がfalseなら、そのentry自身は展開先を持たない（取り除く対象の
// top-level directoryそのもの）。
func stripComponents(name string, strip int) (string, bool, error) {
	if strip == 0 {
		return name, true, nil
	}
	index := strings.IndexByte(name, '/')
	if index < 0 {
		// top-level entry自身。directoryなら展開先が無く、fileなら
		// strip後にnameが残らない。
		return "", false, nil
	}
	rest := name[index+1:]
	if rest == "" {
		return "", false, nil
	}
	return rest, true, nil
}

// checkRatio は圧縮比の上限を検査する（§21「圧縮比（entry/全体）1,000」）。
//
// 圧縮後sizeが不明（0）の場合は判定しない。tarのentryは個別の圧縮後sizeを
// 持たないためで、その場合は全体の比と実展開bytesの打ち切りで守る。
func checkRatio(expanded, compressed int64) error {
	if compressed <= 0 || expanded <= 0 {
		return nil
	}
	if expanded/compressed > ArchiveRatioMax {
		return fmt.Errorf(
			"圧縮比が上限%dを超える（%d / %d）", ArchiveRatioMax, expanded, compressed)
	}
	return nil
}

// archiveUnsafe はarchive安全検査の失敗をtyped errorにする。
func archiveUnsafe(cause error) *domain.Error {
	return &domain.Error{
		Code: domain.CodeArchiveUnsafe,
		// 同じarchiveを展開し直しても同じ結果になる（docs/02-architecture.md §14）。
		Retryable: false,
		PathRole:  domain.RoleStaging,
		Cause:     cause,
	}
}

// foldKey はcase衝突の比較keyを作る。
//
// NFC正規化はcheckEntryが済ませているが、比較側でも通すことで、将来nameの
// 検査を緩めたときに比較だけが古い前提のまま残るのを防ぐ。
func foldKey(path string) string {
	return strings.ToLower(norm.NFC.String(path))
}

// registerPrefixes はpathの各prefixを登録し、case衝突があれば衝突相手を返す。
//
// `bin/go`は`bin`と`bin/go`を登録する。同じ正規化keyに違う表記が既にあれば衝突
// である。同じ表記なら、directory entryとその下のfileが同じprefixを共有する
// 正常な状態なので通す。
func registerPrefixes(seen map[string]string, path string) (string, bool) {
	components := strings.Split(path, "/")
	for index := range components {
		prefix := strings.Join(components[:index+1], "/")
		key := foldKey(prefix)
		previous, taken := seen[key]
		if taken && previous != prefix {
			return previous, true
		}
		if !taken {
			seen[key] = prefix
		}
	}
	return "", false
}
