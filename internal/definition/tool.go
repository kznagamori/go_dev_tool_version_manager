package definition

import (
	"fmt"
	"path"
	"strings"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
)

// buildTool は§4の`[tool]`を検証してmodelへ入れる。
//
// 7 key全件必須である。1件でも欠けていれば残りの検査は続けるが、値は入れない。
func buildTool(table *toolTable, filePath string, value *Definition, diagnostics *Diagnostics) {
	if table == nil {
		diagnostics.Add("tool", reason(reasonMissing), "top-level `tool` tableが無い")
		return
	}
	value.Tool.ID = buildToolID(table.ID, filePath, diagnostics)
	value.Tool.Name = requireText(table.Name, "tool.name", 1, NameMaxBytes, diagnostics)
	value.Tool.Aliases = buildAliases(table.Aliases, value.Tool.ID, diagnostics)
	value.Tool.Description = requireText(
		table.Description, "tool.description", 1, DescriptionMaxBytes, diagnostics)
	value.Tool.Homepage = requireHTTPSURL(table.Homepage, "tool.homepage", urlReference, diagnostics)
	value.Tool.License = requireLicense(table.License, "tool.license", diagnostics)
	value.Tool.VersionScheme = buildVersionScheme(table.VersionScheme, diagnostics)
}

// buildToolID は`id`のgrammarとfile basenameとの一致を検査する（§3・§4）。
//
// basename一致を検査するのは、fileを別名でcopyしただけの重複定義がregistryへ
// 入るのを防ぐためである。
func buildToolID(raw *string, filePath string, diagnostics *Diagnostics) domain.ToolID {
	if raw == nil {
		diagnostics.Add("tool.id", reason(reasonMissing), "`tool.id`が無い")
		return domain.ToolID{}
	}
	if err := ValidateToolID(*raw); err != nil {
		diagnostics.Add("tool.id", reason(reasonIdentifier), err.Error())
		return domain.ToolID{}
	}
	toolID, err := domain.ParseToolID(*raw)
	if err != nil {
		diagnostics.Add("tool.id", reason(reasonIdentifier), err.Error())
		return domain.ToolID{}
	}
	if base := strings.TrimSuffix(path.Base(filePath), ".toml"); base != *raw {
		diagnostics.Add("tool.id", reason(reasonBasename),
			fmt.Sprintf("`tool.id` %q がfile basename %q と一致しない", *raw, base))
	}
	return toolID
}

// buildAliases は§4の`aliases`を検査する。
//
// 空配列は許すがkey自体は必須である。registry全体でのtool ID/aliasとの衝突検査
// （§3）は全definitionを見る必要があるためP4-01の範囲であり、ここでは同一
// definition内の衝突と自分自身のIDとの衝突までを見る。
func buildAliases(raw *[]string, toolID domain.ToolID, diagnostics *Diagnostics) []string {
	if raw == nil {
		diagnostics.Add("tool.aliases", reason(reasonMissing), "`tool.aliases`が無い")
		return nil
	}
	aliases := *raw
	if len(aliases) > AliasMax {
		diagnostics.Add("tool.aliases", reason(reasonLimit),
			fmt.Sprintf("aliasが%d件を超える（%d件）", AliasMax, len(aliases)))
		return nil
	}
	for index, alias := range aliases {
		if err := ValidateToolID(alias); err != nil {
			diagnostics.Add(fmt.Sprintf("tool.aliases[%d]", index), reason(reasonIdentifier), err.Error())
		}
	}
	// 自分自身のIDもcase非依存の一意検査に含める。IDと同じaliasは解決の起点が
	// 2つになり、どちらが正規かをtypeで区別できなくなる。
	scope := append([]string{}, aliases...)
	if !toolID.IsZero() {
		scope = append(scope, toolID.String())
	}
	if err := requireUniqueIdentifiers("tool ID/alias", scope); err != nil {
		diagnostics.Add("tool.aliases", reason(reasonDuplicate), err.Error())
		return nil
	}
	return append([]string{}, aliases...)
}

// buildVersionScheme は§4の`version_scheme`を検査する。
//
// grammarと比較規則の実装は[domain.ParseVersion]が持つ。ここではenumの3値だけを
// 見る。scheme依存のversion検査（§6.4のoverride、§6.6のstatic version）は
// P3-01の2本目の範囲である。
func buildVersionScheme(raw *string, diagnostics *Diagnostics) domain.VersionScheme {
	if raw == nil {
		diagnostics.Add("tool.version_scheme", reason(reasonMissing), "`tool.version_scheme`が無い")
		return ""
	}
	scheme, err := domain.ParseVersionScheme(*raw)
	if err != nil {
		diagnostics.Add("tool.version_scheme", reason(reasonEnum), err.Error())
		return ""
	}
	return scheme
}

// requireText は必須のUTF-8 textをbyte長で検査する。
func requireText(raw *string, field string, min, max int, diagnostics *Diagnostics) string {
	if raw == nil {
		diagnostics.Add(field, reason(reasonMissing), fmt.Sprintf("`%s`が無い", field))
		return ""
	}
	text := *raw
	switch {
	case len(text) < min:
		diagnostics.Add(field, reason(reasonText),
			fmt.Sprintf("%sが%d byteに満たない（%d byte）", field, min, len(text)))
		return ""
	case len(text) > max:
		diagnostics.Add(field, reason(reasonText),
			fmt.Sprintf("%sが%d byteを超える（%d byte）", field, max, len(text)))
		return ""
	// 前後空白は§3がidentifierへ禁じるが、表示textでも受け付けない。表示幅と
	// 一致検査が入力の空白で変わると、同じ値が別物として扱われる。
	case strings.TrimSpace(text) != text:
		diagnostics.Add(field, reason(reasonText), fmt.Sprintf("%sの前後に空白がある", field))
		return ""
	}
	return text
}
