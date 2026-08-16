package definition

import (
	"fmt"
	"net/url"
	"strings"
)

// urlKind はURLの用途である。query stringの可否だけが違う。
//
// **参照URLはqueryを拒否し、endpoint URLは許す。**
//
// §5.1は「URLはHTTPS、credential/query secretなし」と定めるが、
// docs/10-security.md §9.2は「既知のtoken query key」の一覧を定めていない。
// 列挙できない以上、参照URLでは§13のfail closedに従ってquery全体を拒否する。
// [security.MaskURL]がmask側で同じ理由からquery値を種類によらず伏せるのと
// 対になる判断である。
//
// 一方でendpoint URLはqueryを拒否できない。§16.2のGoが
// `https://go.dev/dl/?mode=json&include=all`を正規例に持ち、queryがAPI契約の
// 一部だからである。
type urlKind int

const (
	// urlReference は表示・参照のためのURLである（`homepage`, `repository`）。
	urlReference urlKind = iota
	// urlEndpoint は取得先のURLである（`version_source.url`、子文書、artifact）。
	urlEndpoint
)

// requireHTTPSURL は必須のHTTPS URLを検査する（§4・§5.1・§6）。
//
// fragmentはどちらのkindでも拒否する。取得に影響せず、mask時に落ちるため、
// 残っていても診断と実際の取得先がずれるだけである。userinfoも
// docs/10-security.md §11.1が「URL userinfoのdefinition指定を禁止する」と
// 定めるため、kindによらず拒否する。
func requireHTTPSURL(raw *string, field string, kind urlKind, diagnostics *Diagnostics) string {
	if raw == nil {
		diagnostics.Add(field, reason(reasonMissing), fmt.Sprintf("`%s`が無い", field))
		return ""
	}
	if err := checkHTTPSURL(*raw, field, kind); err != nil {
		diagnostics.Add(field, reason(reasonURL), err.Error())
		return ""
	}
	return *raw
}

// optionalHTTPSURL は任意のHTTPS URLを検査する。未設定は空文字を返す。
func optionalHTTPSURL(raw *string, field string, kind urlKind, diagnostics *Diagnostics) string {
	if raw == nil {
		return ""
	}
	if err := checkHTTPSURL(*raw, field, kind); err != nil {
		diagnostics.Add(field, reason(reasonURL), err.Error())
		return ""
	}
	return *raw
}

func checkHTTPSURL(text, field string, kind urlKind) error {
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
	case parsed.Fragment != "" || strings.Contains(text, "#"):
		return fmt.Errorf("%sにfragmentを含められない", field)
	}
	if kind == urlReference && (parsed.RawQuery != "" || strings.Contains(text, "?")) {
		return fmt.Errorf("%sにquery stringを含められない", field)
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
