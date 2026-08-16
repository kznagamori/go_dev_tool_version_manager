package definition

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// LicenseMaxBytes はSPDX expressionの上限である。
//
// 仕様は個別の上限を定めていないため、§21の「その他definition array」と同じ
// 桁の値を使わず、表示textの上限（[NameMaxBytes]）と揃える。expressionは
// 識別子の連結であり、これを超える式は標準registryに現れない。
const LicenseMaxBytes = NameMaxBytes

// spdxIDStringRe はSPDX license expressionのidstringである。
//
// SPDX 2.3の`idstring = 1*(ALPHA / DIGIT / "-" / "." )`。
var spdxIDStringRe = regexp.MustCompile(`^[A-Za-z0-9.-]+$`)

// requireLicense は必須のSPDX expressionを検査する（§4・§5.1）。
//
// **検査するのは式のsyntaxだけで、SPDX license listへの登録有無は見ない。**
// listをclientへ同梱するとregistryとは別の更新経路ができ、上流のlist更新で
// 既存definitionが読めなくなる。実在する識別子かどうかはregistry reviewの
// 判断であり、docs/07-registry-and-tools.md §5の項目9がOSI承認かどうかを含めて
// 人手で照合する契約になっている。
//
// `LicenseRef-`形式を受けるのは、.NET SDKのWindows配布物のように独自EULAで
// SPDX識別子を持たないlicenseがあるためである（同§、`LicenseRef-dotnet-library`）。
func requireLicense(raw *string, field string, diagnostics *Diagnostics) string {
	if raw == nil {
		diagnostics.Add(field, reason(reasonMissing), fmt.Sprintf("`%s`が無い", field))
		return ""
	}
	if err := checkLicenseExpression(*raw); err != nil {
		diagnostics.Add(field, reason(reasonLicense), fmt.Sprintf("%s: %s", field, err))
		return ""
	}
	return *raw
}

// checkLicenseExpression はSPDX license expressionのsyntaxを検査する。
func checkLicenseExpression(text string) error {
	switch {
	case text == "":
		return errors.New("licenseが空")
	case len(text) > LicenseMaxBytes:
		return fmt.Errorf("licenseが%d byteを超える（%d byte）", LicenseMaxBytes, len(text))
	case strings.TrimSpace(text) != text:
		return errors.New("licenseの前後に空白がある")
	}
	tokens, err := tokenizeLicense(text)
	if err != nil {
		return err
	}
	parser := &licenseParser{tokens: tokens}
	if err := parser.parseExpression(); err != nil {
		return err
	}
	if parser.position != len(parser.tokens) {
		return fmt.Errorf("license expressionに余分な字句 %q がある", parser.tokens[parser.position])
	}
	return nil
}

// tokenizeLicense はexpressionを括弧と語へ分ける。
func tokenizeLicense(text string) ([]string, error) {
	var tokens []string
	var current strings.Builder
	flush := func() {
		if current.Len() > 0 {
			tokens = append(tokens, current.String())
			current.Reset()
		}
	}
	for _, char := range text {
		switch {
		case char == '(' || char == ')':
			flush()
			tokens = append(tokens, string(char))
		case char == ' ':
			flush()
		case char > 0x7F:
			return nil, fmt.Errorf("license expressionにASCII以外の文字 %q がある", string(char))
		default:
			current.WriteRune(char)
		}
	}
	flush()
	if len(tokens) == 0 {
		return nil, errors.New("license expressionが空")
	}
	return tokens, nil
}

// licenseParser はSPDX expressionの再帰下降parserである。
//
//	expression        = compound *( ("AND" / "OR") compound )
//	compound          = "(" expression ")" / simple [ "WITH" idstring ]
//	simple            = license-ref / idstring [ "+" ]
//	license-ref       = [ "DocumentRef-" idstring ":" ] "LicenseRef-" idstring
//
// ANDとORの優先順位を区別しない。syntaxの妥当性だけを見るため、結合の違いが
// 判定を変えないためである。
type licenseParser struct {
	tokens   []string
	position int
}

func (p *licenseParser) peek() (string, bool) {
	if p.position >= len(p.tokens) {
		return "", false
	}
	return p.tokens[p.position], true
}

func (p *licenseParser) parseExpression() error {
	if err := p.parseCompound(); err != nil {
		return err
	}
	for {
		token, ok := p.peek()
		if !ok || (token != "AND" && token != "OR") {
			return nil
		}
		p.position++
		if err := p.parseCompound(); err != nil {
			return err
		}
	}
}

func (p *licenseParser) parseCompound() error {
	token, ok := p.peek()
	if !ok {
		return errors.New("license expressionが演算子で終わっている")
	}
	if token == "(" {
		p.position++
		if err := p.parseExpression(); err != nil {
			return err
		}
		next, ok := p.peek()
		if !ok || next != ")" {
			return errors.New("license expressionの括弧が閉じていない")
		}
		p.position++
		return nil
	}
	if err := p.parseSimple(); err != nil {
		return err
	}
	// WITHの右辺はlicense exception IDで、`+`も括弧も取らない。
	if next, ok := p.peek(); ok && next == "WITH" {
		p.position++
		exception, ok := p.peek()
		if !ok {
			return errors.New("`WITH`の後にexception IDが無い")
		}
		// 演算子と括弧はidstring grammarに一致してしまうため、先に弾く。
		// `WITH OR`のような式を受理すると、右辺が式なのかIDなのか決まらない。
		switch exception {
		case "AND", "OR", "WITH", "(", ")":
			return fmt.Errorf("`WITH`の右辺に演算子 %q がある", exception)
		}
		if !spdxIDStringRe.MatchString(exception) {
			return fmt.Errorf("license exception ID %q がidstring grammarに合わない", exception)
		}
		p.position++
	}
	return nil
}

func (p *licenseParser) parseSimple() error {
	token, ok := p.peek()
	if !ok {
		return errors.New("license IDが無い")
	}
	switch token {
	case "AND", "OR", "WITH", ")":
		return fmt.Errorf("license IDのある位置に演算子 %q がある", token)
	}
	p.position++
	// `LicenseRef-`と`DocumentRef-...:LicenseRef-...`はSPDX listに無いlicenseを
	// 指す正規の書き方である（SPDX 2.3 Annex D）。
	if body, found := strings.CutPrefix(token, "DocumentRef-"); found {
		documentID, licenseRef, hasColon := strings.Cut(body, ":")
		if !hasColon {
			return fmt.Errorf("DocumentRef %q に`:`が無い", token)
		}
		if !spdxIDStringRe.MatchString(documentID) {
			return fmt.Errorf("DocumentRef ID %q がidstring grammarに合わない", documentID)
		}
		return checkLicenseRef(licenseRef)
	}
	if strings.HasPrefix(token, "LicenseRef-") {
		return checkLicenseRef(token)
	}
	// 末尾の`+`は「このversion以降」を表す正規の接尾辞である。
	id := strings.TrimSuffix(token, "+")
	if id == "" || !spdxIDStringRe.MatchString(id) {
		return fmt.Errorf("license ID %q がidstring grammarに合わない", token)
	}
	return nil
}

func checkLicenseRef(token string) error {
	body, found := strings.CutPrefix(token, "LicenseRef-")
	if !found {
		return fmt.Errorf("%q が`LicenseRef-`で始まらない", token)
	}
	if body == "" || !spdxIDStringRe.MatchString(body) {
		return fmt.Errorf("LicenseRef %q がidstring grammarに合わない", token)
	}
	return nil
}
