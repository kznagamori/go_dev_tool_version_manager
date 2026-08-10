package domain

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
)

// ScalarKind はScalarが保持する値の種別である。
type ScalarKind uint8

// ScalarKind の値。docs/04-storage-and-data.md §16.1が
// 「string/bool/integer/nullだけのmap」と定める4種に対応する。
const (
	ScalarNull ScalarKind = iota
	ScalarString
	ScalarBool
	ScalarInt
)

// Scalar は公開境界へ出せる単純値である（docs/02-architecture.md §10・§14）。
//
// typed error、progress、warning、structured logのparameterはこの4種だけを取る。
// 任意のGo値を許すと、error message、Plan、JSON envelopeへstruct、slice、pointerが
// 混ざり、message catalogのplaceholder展開とJSON schemaが壊れる。
//
// zero値はnullである。nullは「値が無い」ことを明示するための正当な状態のため、
// 他のdomain値と違ってzero値を無効にしない。
type Scalar struct {
	kind ScalarKind
	str  string
	b    bool
	i    int64
}

// NullScalar はnull値を返す。
func NullScalar() Scalar { return Scalar{kind: ScalarNull} }

// StringScalar は文字列値を作る。
func StringScalar(value string) Scalar { return Scalar{kind: ScalarString, str: value} }

// BoolScalar は真偽値を作る。
func BoolScalar(value bool) Scalar { return Scalar{kind: ScalarBool, b: value} }

// IntScalar は整数値を作る。
//
// JSON側の安全な整数範囲はdocs/04-storage-and-data.md §7が2^53-1以下と定めるが、
// その制限はbyte count/revisionの表現に対するものである。ここでは種別だけを保持し、
// 範囲検査は値をJSONへ出すcodecの責務とする。
func IntScalar(value int64) Scalar { return Scalar{kind: ScalarInt, i: value} }

// Kind は値の種別を返す。
func (s Scalar) Kind() ScalarKind { return s.kind }

// IsNull はnullかどうかを返す。
func (s Scalar) IsNull() bool { return s.kind == ScalarNull }

// Str は文字列値と、種別が文字列かどうかを返す。
func (s Scalar) Str() (string, bool) { return s.str, s.kind == ScalarString }

// Bool は真偽値と、種別が真偽値かどうかを返す。
func (s Scalar) Bool() (bool, bool) { return s.b, s.kind == ScalarBool }

// Int は整数値と、種別が整数かどうかを返す。
func (s Scalar) Int() (int64, bool) { return s.i, s.kind == ScalarInt }

// String は表示・debug用の文字列表現を返す。
//
// 機械可読形式ではない。JSON化はcodecが種別ごとに行う。
func (s Scalar) String() string {
	switch s.kind {
	case ScalarString:
		return s.str
	case ScalarBool:
		return strconv.FormatBool(s.b)
	case ScalarInt:
		return strconv.FormatInt(s.i, 10)
	default:
		return "null"
	}
}

// parameterKeyRe はdocs/04-storage-and-data.md §7のscalar parameter key grammarである。
var parameterKeyRe = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// ParameterKeyMaxLength は§7が定めるkeyの最大長である。
const ParameterKeyMaxLength = 64

// ValidateParameterKey はscalar parameter keyのgrammarを検査する。
//
// typed error、progress、warning、structured logで同じgrammarを使う。同じ値を指す
// keyが箇所ごとに`tool_id`と`toolId`へ分かれると、message templateのplaceholderと
// CLI JSONの突き合わせができなくなる（§7）。
func ValidateParameterKey(key string) error {
	switch {
	case key == "":
		return fmt.Errorf("domain: parameter keyが空")
	case len(key) > ParameterKeyMaxLength:
		return fmt.Errorf(
			"domain: parameter key %q は%d文字を超える", key, ParameterKeyMaxLength)
	case !parameterKeyRe.MatchString(key):
		return fmt.Errorf(
			"domain: parameter key %q が `^[a-z][a-z0-9_]*$` に一致しない", key)
	}
	return nil
}

// Parameters はmessage templateへ渡すscalarの集合である。
//
// 件数の上限は用途ごとに違う（structured logのfieldsだけが最大64件、
// docs/04-storage-and-data.md §18）。ここでは上限を持たず、必要な箇所が
// [Parameters.ValidateWithLimit] で指定する。
type Parameters map[string]Scalar

// Validate は全keyがgrammarに合うことを検査する。
func (p Parameters) Validate() error {
	for _, key := range p.SortedKeys() {
		if err := ValidateParameterKey(key); err != nil {
			return err
		}
	}
	return nil
}

// ValidateWithLimit はgrammarに加えて件数上限を検査する。
func (p Parameters) ValidateWithLimit(max int) error {
	if len(p) > max {
		return fmt.Errorf("domain: parameterは最大%d件だが %d件が渡された", max, len(p))
	}
	return p.Validate()
}

// SortedKeys はkeyを昇順で返す。
//
// 診断、log、golden testがmapのiteration順に依存しないようにする
// （docs/02-architecture.md §7「internal map iteration順を露出しない」）。
func (p Parameters) SortedKeys() []string {
	keys := make([]string, 0, len(p))
	for key := range p {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// Clone は独立したcopyを返す。
//
// requestとresultは境界通過後にimmutableとして扱う（docs/02-architecture.md §5）が、
// Goのmapは参照であるため、境界を越える際に共有を切る手段を型として用意する。
func (p Parameters) Clone() Parameters {
	if p == nil {
		return nil
	}
	clone := make(Parameters, len(p))
	for key, value := range p {
		clone[key] = value
	}
	return clone
}
