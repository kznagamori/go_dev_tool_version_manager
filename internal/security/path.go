package security

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
)

// PathComponentMaxBytes は1 path componentの上限である。
//
// docs/04-storage-and-data.md §21の「path component / logical path 255 byte / 32 KiB」。
const PathComponentMaxBytes = 255

// LogicalPathMaxBytes は組み立てたlogical path全体の上限である（同§21）。
const LogicalPathMaxBytes = 32 << 10

// windowsReservedNames はWindowsの予約device名である（docs/04-storage-and-data.md §6）。
//
// 拡張子を付けても予約は解除されないため、最初の`.`より前を比較する。case非依存で
// 判定するのは、Windowsがこれらをcase非依存に予約するためである。
var windowsReservedNames = map[string]struct{}{
	"con": {}, "prn": {}, "aux": {}, "nul": {},
	"com1": {}, "com2": {}, "com3": {}, "com4": {}, "com5": {},
	"com6": {}, "com7": {}, "com8": {}, "com9": {},
	"lpt1": {}, "lpt2": {}, "lpt3": {}, "lpt4": {}, "lpt5": {},
	"lpt6": {}, "lpt7": {}, "lpt8": {}, "lpt9": {},
}

// JoinRequest はlogical rootからのpath組み立て入力である。
type JoinRequest struct {
	// Root はlogical rootのabsolute pathである。role付きで受け取り、
	// 戻り値へ同じroleを引き継ぐ。
	Root domain.PathValue
	// Components はrootからの相対component列である。separatorを含めない。
	Components []string
	// Host はpath規則を決めるplatformである。
	Host domain.Platform
}

// Join はlogical rootからabsolute pathを組み立てる（docs/04-storage-and-data.md §6）。
//
// 同§は「すべての書込み前にlogical rootからabsolute pathを組み立て、canonical
// parent containmentを検査する。absolute component、`..`、空component、予約名、
// ADS、NUL、separator混在、symlink/reparse point経由の逸脱を拒否する」と定める。
// このうちfilesystemを触らずに判定できるものをここで拒否し、canonical parent
// containmentとsymlink/reparse経由の逸脱は[VerifyContainment]が扱う。
//
// 文字列連結ではなくcomponent列を受け取るのは、呼出し側が`filepath.Join`で先に
// 潰した`..`を検出できなくするためである。componentのまま受ければ、`..`が
// 混ざった時点で拒否できる。
func Join(req JoinRequest) (domain.PathValue, error) {
	if req.Root.IsZero() {
		return domain.PathValue{}, fmt.Errorf("security: logical rootが未設定")
	}
	if req.Root.Path() == "" {
		return domain.PathValue{}, fmt.Errorf("security: logical root %s のpathが空", req.Root.Role())
	}
	if req.Host.IsZero() {
		return domain.PathValue{}, fmt.Errorf("security: host platformが未設定")
	}

	separator := PathSeparator(req.Host)
	windows := req.Host.OS() == "windows"

	for _, component := range req.Components {
		if err := ValidateComponent(component, windows); err != nil {
			return domain.PathValue{}, err
		}
	}

	joined := strings.TrimRight(req.Root.Path(), separator)
	for _, component := range req.Components {
		joined += separator + component
	}
	if len(joined) > LogicalPathMaxBytes {
		return domain.PathValue{}, fmt.Errorf(
			"security: logical pathが%d byteを超える（%d byte）", LogicalPathMaxBytes, len(joined))
	}

	return domain.NewPathValue(req.Root.Role(), joined)
}

// ValidateComponent は1 path componentの安全性を検査する（§6）。
//
// windowsがtrueのときだけ、ADSの`:`とWindows予約名を拒否する。Linuxではどちらも
// 通常のfile名として有効であり、拒否すると正当なpathを扱えなくなる。逆に区切りと
// NULは両OSで拒否する。
func ValidateComponent(component string, windows bool) error {
	switch {
	case component == "":
		return fmt.Errorf("security: path componentが空")
	case component == "." || component == "..":
		return fmt.Errorf("security: path component %q は相対参照である", component)
	case len(component) > PathComponentMaxBytes:
		return fmt.Errorf(
			"security: path componentが%d byteを超える（%d byte）", PathComponentMaxBytes, len(component))
	case strings.ContainsRune(component, 0):
		return fmt.Errorf("security: path componentにNULが含まれる")
	case strings.ContainsAny(component, `/\`):
		// separator混在の検出でもある。componentへ区切りを埋めると、
		// 上位のcomponent検査を回避して階層を1段抜けられる。
		return fmt.Errorf("security: path component %q に区切りが含まれる", component)
	case !utf8.ValidString(component):
		return fmt.Errorf("security: path componentが正しいUTF-8でない")
	}
	if !windows {
		return nil
	}
	if strings.Contains(component, ":") {
		// NTFSのalternate data streamは`name:stream`で開ける。componentへ`:`を
		// 許すと、検査した名前とは別のstreamへ書ける。
		return fmt.Errorf("security: path component %q にADS区切り `:` が含まれる", component)
	}
	base := component
	if index := strings.Index(base, "."); index >= 0 {
		base = base[:index]
	}
	if _, reserved := windowsReservedNames[strings.ToLower(base)]; reserved {
		return fmt.Errorf("security: path component %q はWindows予約device名である", component)
	}
	// Windowsは末尾の空白とdotを暗黙に落とすため、検査した名前と実際に作られる
	// 名前がずれる。ずれる入力は受け付けない。
	if strings.HasSuffix(component, " ") || strings.HasSuffix(component, ".") {
		return fmt.Errorf("security: path component %q が空白またはdotで終わる", component)
	}
	return nil
}

// PathSeparator はplatformのpath区切りを返す。
//
// runtime.GOOSではなく引数のplatformで決める。どちらのrunnerからでも両OSの規則を
// testできるようにするためである（CLAUDE.md §5）。
func PathSeparator(host domain.Platform) string {
	if host.OS() == "windows" {
		return `\`
	}
	return "/"
}

// IsContained はcanonical childがcanonical root配下かを判定する（§6）。
//
// 両方とも[RealPath]相当で解決済みのcanonical pathを渡す。解決前のpathで比べると、
// symlink/reparse pointを経由した逸脱を見逃す。
//
// rootそのものは配下として扱う。root自身への書込み（rootのmkdirやrename）を
// containment違反にすると、setupがrootを作れなくなるためである。
//
// 判定はcomponent単位で行う。文字列prefixで比べると`/data/gdtvm`に対して
// `/data/gdtvm-evil`が配下と誤判定される。
func IsContained(root, child string, host domain.Platform) bool {
	separator := PathSeparator(host)
	rootParts := splitPath(root, separator)
	childParts := splitPath(child, separator)

	if len(childParts) < len(rootParts) {
		return false
	}
	windows := host.OS() == "windows"
	for i, part := range rootParts {
		if !sameComponent(part, childParts[i], windows) {
			return false
		}
	}
	return true
}

func splitPath(path, separator string) []string {
	trimmed := strings.Trim(path, separator)
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, separator)
}

// sameComponent はplatformのcase規則でcomponentを比較する。
//
// Windowsのfilesystemはcase非依存のため、`C:\Data`と`c:\data`は同じ位置を指す。
// case sensitiveに比べると、同じrootへの書込みをcontainment違反と誤判定する。
func sameComponent(left, right string, windows bool) bool {
	if windows {
		return strings.EqualFold(left, right)
	}
	return left == right
}
