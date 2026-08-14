package store

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
)

// idHexRe は128 bit randomの32 lowercase hexである（docs/04-storage-and-data.md §7）。
var idHexRe = regexp.MustCompile(`^[0-9a-f]{32}$`)

// clientVersionRe はrelease clientの完全versionである。
//
// docs/11-quality-and-ci.md の`YYYY.MM.DD.XX`。development buildだけが
// [DevelopmentClientVersion]を使える（docs/04-storage-and-data.md §8）。
var clientVersionRe = regexp.MustCompile(`^[0-9]{4}\.[0-9]{2}\.[0-9]{2}\.[0-9]{2}$`)

// commandNameRe は公開commandの名前である。
//
// docs/06-tool-definition.md のruntime command名に合わせ、ASCII lowercase英数と
// `-`、`_`、`.`だけを許す。shim fileの名前になるため、path componentとして
// 安全な文字だけへ限る（docs/04-storage-and-data.md §6）。
var commandNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

// LatestKeyword は完全versionとして拒否する予約語である。
//
// docs/05-configuration.md §4が「値はcatalog正規完全versionだけ。latest、
// channel、range、配列を保存しない」と定める。state fileも同じ制約を持つ。
const LatestKeyword = "latest"

// InstallRef は永続fileが持つtool/version/platformの組である。
//
// [domain.InstallKey]ではなくversionをtextで持つ。versionのschemeはtool
// definitionが決めるため（docs/06-tool-definition.md §4）、definitionを持たない
// codec層ではschemeを一意に決められない。ここではscheme非依存に判定できる
// 「完全versionであること」までを検査し、比較を要する処理はdefinitionが入る
// P3以降が[domain.ParseVersion]で行う。
//
// P2-02の`ParseProjectConfig`と同じ分担であり、判定規則も同じである。
type InstallRef struct {
	// Tool は正規tool IDである。aliasはregistryが解決済みである前提。
	Tool domain.ToolID
	// Version はcatalogが正規化した完全versionのtextである。
	Version string
	// Platform は対象platformである。
	Platform domain.Platform
}

// SortKey は永続fileの整列に使うkeyを返す。
//
// NUL区切りにするのは、`tool`+`version`の連結が別のtupleと衝突しないように
// するためである。tool ID/version/platform IDのいずれもNULを含まない。
func (r InstallRef) SortKey() string {
	return r.Tool.String() + "\x00" + r.Version + "\x00" + r.Platform.ID()
}

// requireInstallRef はtool/version/platformの組を読む。
func requireInstallRef(prefix string, toolID, version, platformID *string) (InstallRef, error) {
	tool, err := requireToolID(prefix+".tool_id", toolID)
	if err != nil {
		return InstallRef{}, err
	}
	versionText, err := requirePresent(prefix+".version", version)
	if err != nil {
		return InstallRef{}, err
	}
	if err := requireExactVersion(prefix+".version", versionText); err != nil {
		return InstallRef{}, err
	}
	platformText, err := requirePresent(prefix+".platform_id", platformID)
	if err != nil {
		return InstallRef{}, err
	}
	platform, err := domain.ParsePlatform(platformText)
	if err != nil {
		return InstallRef{}, fmt.Errorf("%s.platform_id: %w", prefix, err)
	}
	return InstallRef{Tool: tool, Version: versionText, Platform: platform}, nil
}

// requireExactVersion はscheme非依存に完全versionであることを確かめる。
//
// range記号、wildcard、`latest`、前後空白を拒否する。schemeごとのgrammarは
// definitionが決めるためここでは見ない（[InstallRef]の分担）。
func requireExactVersion(field, version string) error {
	switch {
	case version == "":
		return fmt.Errorf("%sが空", field)
	case strings.TrimSpace(version) != version:
		return fmt.Errorf("%sに前後空白がある（%q）", field, version)
	case version == LatestKeyword:
		return fmt.Errorf("%sに%qは保存できない", field, LatestKeyword)
	case strings.ContainsAny(version, "*^~<>= ,|"):
		return fmt.Errorf("%sが完全versionでない（%q）", field, version)
	}
	return nil
}

// requireCommandName は公開command名を読む。
func requireCommandName(field string, raw *string) (string, error) {
	text, err := requirePresent(field, raw)
	if err != nil {
		return "", err
	}
	if !commandNameRe.MatchString(text) {
		return "", fmt.Errorf("%sがcommand名のgrammarに合わない（%q）", field, text)
	}
	return text, nil
}

// requireEnum は閉じた集合の値を読む（docs/04-storage-and-data.md §17.1）。
//
// 同§が「`unknown`という値を持つenumだけが『不明』を表現でき、それ以外のenumで
// 未定義値を受理しない」と定める。permissiveなfallbackを作らない。
func requireEnum[T ~string](field string, raw *string, allowed map[T]struct{}) (T, error) {
	var zero T
	text, err := requirePresent(field, raw)
	if err != nil {
		return zero, err
	}
	if _, ok := allowed[T(text)]; !ok {
		return zero, fmt.Errorf("%sが許可された値でない（%q）", field, text)
	}
	return T(text), nil
}
