package registry

import "fmt"

// LoadScope はcommandごとに検証するregistryの範囲である
// （docs/07-registry-and-tools.md §4）。
type LoadScope struct {
	// Header はregistry headerを必須検証するかである。
	Header bool
	// AllDefinitions は4 definitionすべてを必須検証するかである。
	AllDefinitions bool
	// TargetDefinition は対象definitionのdigestを必須検証するかである。
	TargetDefinition bool
	// ContinueOnError は読めるfileを継続して読むかである。`doctor`だけが真になる。
	ContinueOnError bool
}

// ReadsRegistry はregistryを読むscopeかどうかを返す。
func (s LoadScope) ReadsRegistry() bool {
	return s.Header || s.AllDefinitions || s.TargetDefinition || s.ContinueOnError
}

// scopes は§4のcommand別load範囲である。
//
// 表で持つのは、commandが増減したときにどのcommandの範囲が未定義かを読み取れる
// ようにするためである。条件分岐に散らすと「registryを読まない」ことの根拠が
// 追えなくなる。
var scopes = map[string]LoadScope{
	// 「registryを読まず、binaryへ埋め込んだbuild/schema情報だけを返す」。
	"version": {},
	// 「破損箇所を診断するため読めるfileを継続する」。
	"doctor": {ContinueOnError: true},
	// 「registry headerと4 definitionを必須検証し、required command集合から
	// shim indexを作る」。
	"setup": {Header: true, AllDefinitions: true},
	// 「registry headerと対象definition digestを必須検証する」。
	"available": {Header: true, TargetDefinition: true},
	"install":   {Header: true, TargetDefinition: true},
	"use":       {Header: true, TargetDefinition: true},
	// 「state、receipt、indexを正本とする。正規tool IDならregistryなしで扱い、
	// alias入力を正規化するときだけ対象definitionを要求する」。
	//
	// aliasかどうかは入力を見るまで決まらないため、scopeとしてはregistryを
	// 読まない側に置き、alias正規化が必要になった時点で呼出し側が
	// [ScopeFor]("use")相当の検証を行う。
	"installed": {},
	"current":   {},
	"uninstall": {},
	// 「registry/networkを読まず、strict検証済みshim index、selection、receipt
	// だけを使う」。shim起動中にregistryを読んでindexを書き換えない。
	"shim": {},
}

// CommandCount は§4が範囲を定めるcommand数である。
//
// v0.1のCLIは9 commandで、`shim runtime`を加えた10件になる。件数を定数で持つ
// のは、commandを足したときに範囲の定義漏れへ気付くためである。
const CommandCount = 10

// ScopeFor はcommandのload範囲を返す。
//
// 未知のcommandはerrorにする。既定値で「registryを読まない」を返すと、範囲を
// 決め忘れたcommandが検証なしで動く。
func ScopeFor(command string) (LoadScope, error) {
	scope, ok := scopes[command]
	if !ok {
		return LoadScope{}, fmt.Errorf("registry: command %q のload範囲が未定義", command)
	}
	return scope, nil
}

// Commands は範囲を定義済みのcommand名を返す。
func Commands() []string {
	names := make([]string, 0, len(scopes))
	for name := range scopes {
		names = append(names, name)
	}
	return names
}
