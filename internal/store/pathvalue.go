package store

import (
	"fmt"
	"strings"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
)

// WindowsUserPathLocator はWindows user PATHのregistry value位置である。
//
// docs/04-storage-and-data.md §17.2が「Windows user PATHのregistry valueは
// filesystem pathではないが変更対象の識別が必要なため、`SetupPlan.
// integration_target`と`writes[]`ではrole=`config`、`action=registry-value`とし、
// `path`はexact locator `HKCU\Environment\Path`とする。これはPlan
// `PathValue.path`をabsolute filesystem pathとしない唯一の例外である」と定める。
const WindowsUserPathLocator = `HKCU\Environment\Path`

// pathValueJSON は§17.2の`PathValue`のexact key集合である。
type pathValueJSON struct {
	Role *string `json:"role"`
	Path *string `json:"path"`
}

// pathValueOf はtyped pathをJSON構造へ変換する。
func pathValueOf(value domain.PathValue) *pathValueJSON {
	role := string(value.Role())
	path := value.Path()
	return &pathValueJSON{Role: &role, Path: &path}
}

// pathMode はPlan/CLI JSONのpathへ課す制約である。
//
// 「絶対path必須」を既定とし、空とlocatorをそれぞれ独立に許す。§17.2の
// `integration_target`のように両方を許すfieldがあるため、択一のenumでは表せない。
type pathMode struct {
	// allowEmpty は空pathを許す。
	//
	// §17.2が「`SetupPlan`のprevious root/integration target/backup pathと
	// §17のCLI resultで明示したoptional fieldだけ空を許す」と定める。
	allowEmpty bool
	// allowLocator はWindows user PATHのregistry locatorを許す。
	//
	// §17.2が「これはPlan `PathValue.path`をabsolute filesystem pathとしない
	// 唯一の例外である」と定める。
	allowLocator bool
}

var (
	// pathAbsolute はabsolute pathだけを許す。
	//
	// §17.2が「PlanとCLI JSONの`path`はOS nativeの正規absolute pathとする」と
	// 定める。相対pathを公開境界へ出すと、読み手の作業directoryによって
	// 指す先が変わる。
	pathAbsolute = pathMode{}
	// pathOptional は空pathも許す。
	pathOptional = pathMode{allowEmpty: true}
	// pathLocatorOrAbsolute はregistry locatorも許す。
	pathLocatorOrAbsolute = pathMode{allowLocator: true}
	// pathOptionalLocator は空とregistry locatorの両方を許す。
	pathOptionalLocator = pathMode{allowEmpty: true, allowLocator: true}
)

// buildPathValue は§17.2の`PathValue`を読む。
func buildPathValue(
	field string, raw *pathValueJSON, want domain.PathRole, mode pathMode,
) (domain.PathValue, error) {
	if raw == nil {
		return domain.PathValue{}, fmt.Errorf("%sが無い", field)
	}
	roleText, err := requirePresent(field+".role", raw.Role)
	if err != nil {
		return domain.PathValue{}, err
	}
	role, err := domain.ParsePathRole(roleText)
	if err != nil {
		return domain.PathValue{}, fmt.Errorf("%s.role: %w", field, err)
	}
	// roleは契約ごとに固定である。§17.2が「同じabsolute pathへ複数roleを恣意的に
	// 付けず、最も具体的なroleを使う」と定めており、fieldごとのroleを検査しないと
	// role単位のmaskと書込み範囲検査（§9.1・11-quality-and-ci.md §7.2）が効かない。
	if want != "" && role != want {
		return domain.PathValue{}, fmt.Errorf("%s.roleは%qでなければならない（%q）", field, want, role)
	}
	path, err := requirePresent(field+".path", raw.Path)
	if err != nil {
		return domain.PathValue{}, err
	}
	if err := checkPathMode(field, path, mode); err != nil {
		return domain.PathValue{}, err
	}
	value, err := domain.NewPathValue(role, path)
	if err != nil {
		return domain.PathValue{}, fmt.Errorf("%s: %w", field, err)
	}
	return value, nil
}

func checkPathMode(field, path string, mode pathMode) error {
	if path == "" {
		if mode.allowEmpty {
			return nil
		}
		return fmt.Errorf("%s.pathが空", field)
	}
	if mode.allowLocator && path == WindowsUserPathLocator {
		return nil
	}
	if !isAbsolutePath(path) {
		return fmt.Errorf("%s.pathがabsolute pathでない（%q）", field, path)
	}
	return nil
}

// isAbsolutePath はPOSIXとWindowsの両方のabsolute pathを判定する。
//
// `filepath.IsAbs`は動作中OSの規則だけで判定するため使えない。Plan JSONは
// 生成したOSと別のOSで読むこと（両OSのCI runnerでの検査を含む）があり、
// runtime.GOOSに依存すると片方のOSのpathを常に相対と誤判定する。
func isAbsolutePath(path string) bool {
	if strings.HasPrefix(path, "/") {
		return true
	}
	// UNC pathも絶対である。ただしrootとしての採用は別途拒否する（P2-03）。
	if strings.HasPrefix(path, `\\`) {
		return true
	}
	// `C:\...`形式。drive letterの後ろは`\`でなければならない。`C:x`は
	// drive相対pathであり、process ごとのcurrent directoryに依存する。
	if len(path) >= 3 && path[1] == ':' && path[2] == '\\' && isASCIILetter(path[0]) {
		return true
	}
	return false
}

func isASCIILetter(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}
