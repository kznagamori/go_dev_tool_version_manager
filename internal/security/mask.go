package security

import (
	"net/url"
	"strings"
)

// Redacted は秘密値を伏せた表示である。log/表示前に secret を置換する（11章12節）。
const Redacted = "[REDACTED]"

// sensitiveHeaders は常に mask する header 名（小文字）である（11章12節、06章5節）。
var sensitiveHeaders = map[string]struct{}{
	"authorization":       {},
	"proxy-authorization": {},
	"cookie":              {},
	"set-cookie":          {},
}

// sensitiveQueryKeys は URL query で mask する既知の secret key（小文字）である。
var sensitiveQueryKeys = map[string]struct{}{
	"token":        {},
	"access_token": {},
	"api_key":      {},
	"apikey":       {},
	"key":          {},
	"sig":          {},
	"signature":    {},
	"password":     {},
	"secret":       {},
}

// envSecretSuffixes は secret とみなす環境変数名の接尾辞（大文字）である（11章12節、05章7節）。
var envSecretSuffixes = []string{"_TOKEN", "_PASSWORD", "_SECRET", "_KEY"}

// IsSensitiveHeader は header 名が既定の mask 対象かを返す（case-insensitive）。
func IsSensitiveHeader(name string) bool {
	_, ok := sensitiveHeaders[strings.ToLower(name)]
	return ok
}

// IsSensitiveEnvKey は環境変数名が secret 値を保持するとみなすかを返す（case-insensitive）。
func IsSensitiveEnvKey(name string) bool {
	upper := strings.ToUpper(name)
	switch upper {
	case "TOKEN", "PASSWORD", "SECRET", "KEY":
		return true
	}
	for _, suf := range envSecretSuffixes {
		if strings.HasSuffix(upper, suf) {
			return true
		}
	}
	return false
}

// MaskHeaderValue は header 名が mask 対象、または extraSecretHeaders に含まれる場合に
// 値を Redacted へ置換する。それ以外は値をそのまま返す。
func MaskHeaderValue(name, value string, extraSecretHeaders ...string) string {
	if IsSensitiveHeader(name) {
		return Redacted
	}
	lower := strings.ToLower(name)
	for _, h := range extraSecretHeaders {
		if strings.ToLower(h) == lower {
			return Redacted
		}
	}
	return value
}

// MaskEnvValue は環境変数名が secret とみなされる場合に値を Redacted へ置換する。
func MaskEnvValue(name, value string) string {
	if IsSensitiveEnvKey(name) {
		return Redacted
	}
	return value
}

// MaskURL は URL の userinfo を除去し、既知の secret query key の値を Redacted へ置換した
// 表示用 URL を返す。parse できない場合は安全側で userinfo を含みうる元文字列を返さず、
// scheme://host/path 相当へ落とせないため固定の伏字を返す。
func MaskURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return Redacted
	}
	if u.User != nil {
		u.User = url.User(Redacted)
	}
	if q := u.Query(); len(q) > 0 {
		changed := false
		for key := range q {
			if _, ok := sensitiveQueryKeys[strings.ToLower(key)]; ok {
				q.Set(key, Redacted)
				changed = true
			}
		}
		if changed {
			u.RawQuery = q.Encode()
		}
	}
	return u.String()
}
