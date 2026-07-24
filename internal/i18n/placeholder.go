package i18n

import (
	"fmt"
	"regexp"
	"strings"
)

// messageIDPattern は message ID の形式（ASCII lower dot path）である（14章15節）。
// 各 segment は小文字・数字・underscore で、dot で連結する。
var messageIDPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z0-9_]+)*$`)

// placeholderPattern は `{ident}` 形式の placeholder を表す。ident は ASCII identifier。
var placeholderPattern = regexp.MustCompile(`\{([a-z][a-z0-9_]*)\}`)

// validMessageID は message ID が形式規則を満たすかを返す。
func validMessageID(id string) bool { return messageIDPattern.MatchString(id) }

// placeholderSet は template 内の placeholder 名の集合を返す。
func placeholderSet(tmpl string) map[string]struct{} {
	set := make(map[string]struct{})
	for _, m := range placeholderPattern.FindAllStringSubmatch(tmpl, -1) {
		set[m[1]] = struct{}{}
	}
	return set
}

// validateTemplate は template に不正な波括弧がないことを検査する。format specifier や
// 任意 template 実行を許さないため（14章15節）、有効な placeholder 以外の `{`/`}` を拒否する。
func validateTemplate(tmpl string) error {
	stripped := placeholderPattern.ReplaceAllString(tmpl, "")
	if strings.ContainsAny(stripped, "{}") {
		return fmt.Errorf("不正な波括弧または placeholder を含む")
	}
	return nil
}

// substitute は template の placeholder を args の値へ置換する。値は文字列としてそのまま
// 挿入し、format specifier を解釈しない。対応する arg がない placeholder は literal のまま
// 残し、欠落を可視化する。
func substitute(tmpl string, args map[string]string) string {
	return placeholderPattern.ReplaceAllStringFunc(tmpl, func(match string) string {
		name := match[1 : len(match)-1] // `{` と `}` を除く
		if v, ok := args[name]; ok {
			return v
		}
		return match
	})
}

// sameStringSet は 2 つの集合が一致するかを返す。
func sameStringSet(a, b map[string]struct{}) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if _, ok := b[k]; !ok {
			return false
		}
	}
	return true
}
