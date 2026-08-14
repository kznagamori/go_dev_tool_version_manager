package definition

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/security"
)

// docs/06-tool-definition.md §3のidentifier grammar。
//
// tool ID/aliasとscoped ID（platform/storage/probe/profile）は同じkebab grammarで
// 長さ上限だけが違う。commandは両OS共通のbasenameとして`-+._`を区切りに許す。
var (
	kebabIDRe     = regexp.MustCompile(`^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$`)
	commandNameRe = regexp.MustCompile(`^[a-z][a-z0-9]*(?:[-+._][a-z0-9]+)*$`)
	metadataKeyRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)
)

// §3の長さ上限。byte数で数える。grammarがASCIIへ限るためrune数と一致する。
const (
	// ToolIDMaxBytes はtool IDとaliasの上限である。
	ToolIDMaxBytes = 64
	// ScopedIDMaxBytes はplatform/storage/probe/profile IDの上限である。
	ScopedIDMaxBytes = 96
	// CommandNameMaxBytes は公開command名の上限である。
	CommandNameMaxBytes = 64
)

// ValidateToolID はtool IDとaliasのgrammarを検査する（§3）。
func ValidateToolID(text string) error {
	return checkIdentifier(text, kebabIDRe, ToolIDMaxBytes, "tool ID/alias")
}

// ValidateScopedID はplatform/storage/probe/profile IDのgrammarを検査する（§3）。
func ValidateScopedID(text string) error {
	return checkIdentifier(text, kebabIDRe, ScopedIDMaxBytes, "platform/storage/probe/profile ID")
}

// ValidateCommandName は公開command名のgrammarを検査する（§3）。
//
// Windows/Linux共通のbasenameであり、実際にfilesystemへ現れる名前である。
// grammarに加えてWindows予約device名を拒否するのはこのためである。
func ValidateCommandName(text string) error {
	return checkIdentifier(text, commandNameRe, CommandNameMaxBytes, "command")
}

// ValidateMetadataKey はmetadata keyのgrammarを検査する（§3）。
//
// metadata keyはtemplateの`{{metadata.<key>}}`にだけ現れpath componentにならない
// ため、予約device名の検査対象にしない。長さはgrammar自身が64文字へ制限する。
func ValidateMetadataKey(text string) error {
	if !metadataKeyRe.MatchString(text) {
		return fmt.Errorf("metadata key %q が `%s` に一致しない", text, metadataKeyRe)
	}
	return nil
}

// checkIdentifier はgrammar、長さ、Windows予約名を検査する。
//
// §3は「ASCII以外、uppercase、前後空白、連続separator、`.`/`..`、Windows予約名」の
// 拒否を求める。前5つはgrammarが表現しているため正規表現で足り、予約名だけは
// grammarで表せないので[security.ValidateComponent]へ委ねる。予約名listを
// ここへ複製すると、片方だけが更新される余地ができる。
func checkIdentifier(text string, pattern *regexp.Regexp, max int, kind string) error {
	switch {
	case text == "":
		return fmt.Errorf("%sが空", kind)
	case len(text) > max:
		return fmt.Errorf("%s %q が%d byteを超える（%d byte）", kind, text, max, len(text))
	case !pattern.MatchString(text):
		return fmt.Errorf("%s %q が `%s` に一致しない", kind, text, pattern)
	}
	// identifierはWindowsでもfilesystemへ現れうるため、両OSの規則で検査する。
	if err := security.ValidateComponent(text, true); err != nil {
		return fmt.Errorf("%s %q: %w", kind, text, err)
	}
	return nil
}

// requireUniqueIdentifiers は同一tool内のcase-insensitive衝突を拒否する（§3）。
//
// §3のgrammarは小文字だけを許すため今日はexact一致の検査と同じ結果になるが、
// 仕様が求めているのはcase-insensitiveの一意性である。仕様どおりの比較を書き、
// grammarとの関係はtestで固定する。
func requireUniqueIdentifiers(kind string, values []string) error {
	seen := make(map[string]string, len(values))
	for _, value := range values {
		folded := strings.ToLower(value)
		if previous, duplicate := seen[folded]; duplicate {
			return fmt.Errorf("%s %q と %q がcase非依存で衝突する", kind, previous, value)
		}
		seen[folded] = value
	}
	return nil
}
