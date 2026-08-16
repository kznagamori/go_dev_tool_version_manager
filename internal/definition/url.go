package definition

import (
	"fmt"
	"net/url"
	"strings"
)

// requireHTTPSURL は必須のHTTPS URLを検査する（§4・§5.1）。
//
// 同§が「URLはHTTPS、credential/query secretなし」と定める。
//
// **query stringをkey種別によらず拒否する。** docs/10-security.md §9.2は
// 「既知のtoken query key」を除去対象とするがその一覧を定めていない。列挙できない
// 以上、通す判断も落とす判断も根拠を持てないため、§13のfail closedに従って
// query全体を拒否する。[security.MaskURL]がmask側で同じ理由からquery値を種類に
// よらず伏せるのと対になる判断である。標準4 toolのdefinitionはいずれもqueryを
// 使わない。
//
// fragmentも拒否する。取得に影響せず、mask時に落ちるため、残っていても診断と
// 実際の取得先がずれるだけである。
func requireHTTPSURL(raw *string, field string, diagnostics *Diagnostics) string {
	if raw == nil {
		diagnostics.Add(field, reason(reasonMissing), fmt.Sprintf("`%s`が無い", field))
		return ""
	}
	if err := checkHTTPSURL(*raw, field); err != nil {
		diagnostics.Add(field, reason(reasonURL), err.Error())
		return ""
	}
	return *raw
}

// optionalHTTPSURL は任意のHTTPS URLを検査する。未設定は空文字を返す。
func optionalHTTPSURL(raw *string, field string, diagnostics *Diagnostics) string {
	if raw == nil {
		return ""
	}
	if err := checkHTTPSURL(*raw, field); err != nil {
		diagnostics.Add(field, reason(reasonURL), err.Error())
		return ""
	}
	return *raw
}

func checkHTTPSURL(text, field string) error {
	switch {
	case text == "":
		return fmt.Errorf("%sが空", field)
	case len(text) > URLMaxBytes:
		return fmt.Errorf("%sが%d byteを超える（%d byte）", field, URLMaxBytes, len(text))
	case strings.TrimSpace(text) != text:
		return fmt.Errorf("%sの前後に空白がある", field)
	}
	parsed, err := url.Parse(text)
	if err != nil {
		return fmt.Errorf("%sがURLとして解釈できない（%q）", field, text)
	}
	switch {
	case parsed.Scheme != "https":
		return fmt.Errorf("%sはHTTPSでなければならない（%q）", field, text)
	case parsed.User != nil:
		return fmt.Errorf("%sにuserinfoを含められない", field)
	case parsed.Host == "":
		return fmt.Errorf("%sにhostが無い（%q）", field, text)
	case parsed.RawQuery != "" || strings.Contains(text, "?"):
		return fmt.Errorf("%sにquery stringを含められない", field)
	case parsed.Fragment != "" || strings.Contains(text, "#"):
		return fmt.Errorf("%sにfragmentを含められない", field)
	}
	// hostはASCII lowercaseへ限る。大文字やIDNのpunycode前表記を許すと、
	// 同じhostが別の文字列としてredirect許可listの比較を抜ける（§7.1）。
	if parsed.Host != strings.ToLower(parsed.Host) {
		return fmt.Errorf("%sのhostに大文字が含まれる（%q）", field, parsed.Host)
	}
	for _, char := range parsed.Host {
		if char > 0x7F {
			return fmt.Errorf("%sのhostがASCIIでない（%q）", field, parsed.Host)
		}
	}
	return nil
}
