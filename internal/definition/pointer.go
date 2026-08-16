package definition

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// PointerMaxBytes はJSON pointer 1件の上限である。
//
// 仕様は個別の上限を定めていない。pointerはJSON文書の階層を辿るだけの短い値で
// あり、path componentの上限（255 byte、docs/04-storage-and-data.md §21）と同じ
// 桁で足りる。上限を設けないと、definitionが巨大な文字列でparserを膨らませられる。
const PointerMaxBytes = 255

// validatePointer はRFC 6901のjson-pointer grammarを検査する（§6.1）。
//
//	json-pointer    = *( "/" reference-token )
//	reference-token = *( unescaped / escaped )
//	escaped         = "~" ( "0" / "1" )
//
// 空文字は文書全体を指す正当なpointerである。空でなければ`/`で始まらなければ
// ならず、`~`の直後は`0`か`1`だけを許す。
//
// **解決の意味づけは検査しない。** どのnodeを指すか、配列かどうかはP3-03の
// 取得経路が扱う。schema層でgrammarだけを固定するのは、pointerの評価が上流
// 文書の形に依存し、definitionだけでは決まらないためである。
func validatePointer(text, field string) error {
	switch {
	case len(text) > PointerMaxBytes:
		return fmt.Errorf("%sが%d byteを超える（%d byte）", field, PointerMaxBytes, len(text))
	case !utf8.ValidString(text):
		return fmt.Errorf("%sが正しいUTF-8でない", field)
	case text == "":
		// 文書全体を指すpointer。RFC 6901が明示的に許す。
		return nil
	case !strings.HasPrefix(text, "/"):
		return fmt.Errorf("%sは空か`/`で始まらなければならない（%q）", field, text)
	}
	for index := 0; index < len(text); index++ {
		if text[index] != '~' {
			continue
		}
		if index+1 >= len(text) {
			return fmt.Errorf("%sが`~`で終わっている（%q）", field, text)
		}
		if next := text[index+1]; next != '0' && next != '1' {
			return fmt.Errorf("%sの`~`の後は`0`か`1`だけを許す（%q）", field, text)
		}
		index++
	}
	return nil
}

// OptionalPointer は宣言の有無を保持する任意JSON pointerである。
//
// 空文字は「文書全体」または「item全体」を指す正当なRFC 6901 pointerであるため、
// **宣言していないことを空文字で表せない**。§6.1は`channel_pointer`を「省略した
// 場合」に正規versionの構文からchannelを導出すると定めており、省略と空文字宣言で
// 動作が変わる。`published_at_pointer`の「どれも宣言しないsourceは空文字とし」、
// `item_flatten_pointer`の「指定した場合」も同じである。
//
// zero値は未宣言を表す。
type OptionalPointer struct {
	value    string
	declared bool
}

// DeclaredPointer は宣言済みのpointerを作る。
func DeclaredPointer(value string) OptionalPointer {
	return OptionalPointer{value: value, declared: true}
}

// Declared はdefinitionがこのkeyを書いたかどうかを返す。
func (p OptionalPointer) Declared() bool { return p.declared }

// Value はpointer文字列を返す。未宣言では空文字である。
func (p OptionalPointer) Value() string { return p.value }

// requirePointer は必須のJSON pointerを検査する。
func requirePointer(raw *string, field string, diagnostics *Diagnostics) string {
	if raw == nil {
		diagnostics.Add(field, reason(reasonMissing), fmt.Sprintf("`%s`が無い", field))
		return ""
	}
	if err := validatePointer(*raw, field); err != nil {
		diagnostics.Add(field, reason(reasonPointer), err.Error())
		return ""
	}
	return *raw
}

// optionalPointer は任意のJSON pointerを検査する。
//
// 戻り値が宣言の有無を持つのは、空文字が「文書全体」を指す正当な値であり、
// 未宣言と区別できないためである（[OptionalPointer]）。grammar違反は宣言済み
// 扱いにしない。診断を出した時点でdefinitionは拒否され、値は使われない。
func optionalPointer(raw *string, field string, diagnostics *Diagnostics) OptionalPointer {
	if raw == nil {
		return OptionalPointer{}
	}
	if err := validatePointer(*raw, field); err != nil {
		diagnostics.Add(field, reason(reasonPointer), err.Error())
		return OptionalPointer{}
	}
	return DeclaredPointer(*raw)
}
