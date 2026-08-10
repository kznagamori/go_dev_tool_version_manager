package security

import (
	"net/url"
	"sort"
	"strings"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
)

// Redacted は除去した値の置換文字列である。
//
// 空文字にせず固定文字列にするのは、「値が無かった」と「値を隠した」を
// 診断で区別できるようにするためである。
const Redacted = "<redacted>"

// HomePlaceholder はuser home pathの置換文字列である（docs/10-security.md §9.1）。
const HomePlaceholder = "<HOME>"

// UserPlaceholder はuser名の置換文字列である。
const UserPlaceholder = "<USER>"

// HostPlaceholder はhostnameの置換文字列である。
const HostPlaceholder = "<HOST>"

// secretEnvSuffixes はdocs/10-security.md §9.2が除去対象とする環境変数名のpatternである。
//
// 同§は`*_TOKEN`, `*_PASSWORD`, `*_SECRET`, `*_KEY`と定める。
var secretEnvSuffixes = []string{"_TOKEN", "_PASSWORD", "_SECRET", "_KEY"}

// secretHeaders はdocs/10-security.md §9.2が除去対象とするHTTP headerである。
//
// 同§は「HTTP authorization/cookie/proxy header」と定める。header名はRFC 9110で
// case insensitiveのため小文字で比較する。
var secretHeaders = map[string]struct{}{
	"authorization":       {},
	"proxy-authorization": {},
	"proxy-authenticate":  {},
	"cookie":              {},
	"set-cookie":          {},
}

// IsSecretEnvName は環境変数名が除去対象かを返す（docs/10-security.md §9.2）。
//
// 判定は大文字小文字を無視する。仕様のpatternは大文字表記だが、環境変数名の
// case規則はplatformで異なり、小文字のkeyを素通しするとmaskが抜けるためである。
func IsSecretEnvName(name string) bool {
	upper := strings.ToUpper(name)
	for _, suffix := range secretEnvSuffixes {
		if strings.HasSuffix(upper, suffix) {
			return true
		}
	}
	return false
}

// IsSecretHeader はHTTP header名が除去対象かを返す（docs/10-security.md §9.2）。
func IsSecretHeader(name string) bool {
	_, ok := secretHeaders[strings.ToLower(strings.TrimSpace(name))]
	return ok
}

// MaskURL はURLからuserinfoとquery値を除去する（docs/10-security.md §9.2）。
//
// 同§は「URL userinfo、既知のtoken query key」を除去対象とするが、「既知の
// token query key」の具体的な一覧を定めていない。そこでquery値を種類によらず
// すべて置換する。要求される集合の上位集合であり、§13のfail closed方針に沿う。
// key名は残すので、どのparameterが付いていたかは診断できる。
//
// 解析できない文字列はURLとして扱えないため、まるごと[Redacted]にする。
// 解析失敗をそのまま返すと、壊れたURLに埋まったcredentialが素通りする。
func MaskURL(raw string) string {
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return Redacted
	}
	if parsed.User != nil {
		parsed.User = url.User(Redacted)
	}
	if parsed.RawQuery != "" {
		values := parsed.Query()
		keys := make([]string, 0, len(values))
		for key := range values {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		masked := url.Values{}
		for _, key := range keys {
			masked.Set(key, Redacted)
		}
		parsed.RawQuery = masked.Encode()
	}
	parsed.Fragment = ""
	return parsed.String()
}

// PathMasker は個人を識別しうるpath要素を置換する（docs/10-security.md §9.2）。
//
// 同§は「user home、user名、hostname、SIDを含むabsolute path」を除去対象とする。
// 置換対象を呼出し側から受け取るのは、これらの実値がOS user lookupの結果であり、
// securityがOS APIを直接呼ぶと[02-architecture.md](../../docs/02-architecture.md)§1の
// 依存方向に反するためである。
type PathMasker struct {
	// replacements は長い順に適用する置換対列である。
	replacements [][2]string
}

// NewPathMasker はhome、user名、hostnameを置換するmaskerを作る。
//
// 空の項目は置換しない。空文字を置換対象にすると、全文字境界へplaceholderが
// 挿入されて出力が壊れる。
func NewPathMasker(home, user, host string) *PathMasker {
	masker := &PathMasker{}
	for _, pair := range [][2]string{
		{home, HomePlaceholder},
		{user, UserPlaceholder},
		{host, HostPlaceholder},
	} {
		if pair[0] != "" {
			masker.replacements = append(masker.replacements, pair)
		}
	}
	// 長い文字列から先に置換する。user名がhome pathの一部である場合
	// （`/home/alice`と`alice`）、短い方を先に適用すると`/home/<USER>`になり、
	// home全体の置換規則が効かなくなる。
	sort.SliceStable(masker.replacements, func(i, j int) bool {
		return len(masker.replacements[i][0]) > len(masker.replacements[j][0])
	})
	return masker
}

// Mask は文字列中の個人識別要素をplaceholderへ置換する。
//
// Windowsのpath区切りとcase非依存の比較には対応しない。呼出し側が渡す実値は
// OSから取得した表記であり、同じ表記で現れたものだけを確実に置換する。
// 取りこぼしを許さない箇所ではpath roleだけを公開する
// （[domain.PathValue.WithoutPath]）。
func (m *PathMasker) Mask(text string) string {
	if m == nil || text == "" {
		return text
	}
	for _, pair := range m.replacements {
		text = strings.ReplaceAll(text, pair[0], pair[1])
	}
	return text
}

// MaskParameters はscalar parameterの文字列値へmaskを適用したcopyを返す。
//
// keyが環境変数名として除去対象なら値ごと[Redacted]にする。boolean、integer、
// nullはそのまま残す。docs/10-security.md §9.2が「expected/actual digestは
// secretではないため記録する」と定めるとおり、値の型だけでsecret判定はしない。
func (m *PathMasker) MaskParameters(params domain.Parameters) domain.Parameters {
	if params == nil {
		return nil
	}
	masked := make(domain.Parameters, len(params))
	for _, key := range params.SortedKeys() {
		value := params[key]
		text, isString := value.Str()
		switch {
		case !isString:
			masked[key] = value
		case IsSecretEnvName(key):
			masked[key] = domain.StringScalar(Redacted)
		default:
			masked[key] = domain.StringScalar(m.Mask(text))
		}
	}
	return masked
}
