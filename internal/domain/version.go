package domain

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// VersionScheme はversionの比較規則である（docs/06-tool-definition.md §4）。
type VersionScheme string

// VersionScheme の値。組込みで提供するのはこの3件だけである
// （docs/02-architecture.md §3）。
const (
	SchemeSemver VersionScheme = "semver"
	SchemeGo     VersionScheme = "go"
	SchemePython VersionScheme = "python"
)

// ParseVersionScheme は文字列をVersionSchemeへ変換する。
func ParseVersionScheme(text string) (VersionScheme, error) {
	switch VersionScheme(text) {
	case SchemeSemver, SchemeGo, SchemePython:
		return VersionScheme(text), nil
	}
	return "", fmt.Errorf("domain: version_scheme %q は semver|go|python のいずれでもない", text)
}

// stage はprerelease段階の順序値である。数値が小さいほど先行する。
//
// schemeごとに使う値が異なるが、Compareは同一scheme同士でしか行わないため、
// 1つの型で足りる。finalを最大にして「prereleaseはfinalより前」を表す。
type stage int

const (
	stageSemverPre stage = 1 // semverのprerelease有り
	stageGoBeta    stage = 1
	stageGoRC      stage = 2
	stagePythonA   stage = 1
	stagePythonB   stage = 2
	stagePythonRC  stage = 3
	stageFinal     stage = 9
)

// Version は完全versionである（docs/02-architecture.md §3）。
//
// 元文字列と比較用キーの両方を保持する。利用者入力の一致判定はcatalogの正規
// 文字列とのbyte完全一致で行い、comparison keyへ変換した近似一致をしない
// （docs/06-tool-definition.md §4）。並び替えと大小比較にだけkeyを使う。
type Version struct {
	scheme VersionScheme
	text   string

	major    int
	minor    int
	patch    int
	stage    stage
	stageNum int
	// semverPre はsemverのprerelease識別子列である。空はfinalを表す。
	semverPre []string
}

// 数値要素は不要なleading zeroを禁止する（docs/06-tool-definition.md §4）。
var (
	numRe       = regexp.MustCompile(`^(0|[1-9][0-9]*)$`)
	semverRe    = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-([0-9A-Za-z.\-]+))?$`)
	goRe        = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:\.(0|[1-9][0-9]*)|(beta|rc)([1-9][0-9]*))?$`)
	pythonRe    = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:(a|b|rc)([1-9][0-9]*))?$`)
	semverIdent = regexp.MustCompile(`^[0-9A-Za-z\-]+$`)
)

// ParseVersion は正規version文字列をVersionへ変換する。
//
// schemeごとのgrammarに合わない入力、leading `v`、build metadata、leading zeroを
// すべて拒否する。寛容な正規化を行わないのは、catalogの正規文字列だけを唯一の
// 入力形式とする契約（docs/06-tool-definition.md §4）を型で守るためである。
func ParseVersion(scheme VersionScheme, text string) (Version, error) {
	if text == "" {
		return Version{}, fmt.Errorf("domain: versionが空")
	}
	switch scheme {
	case SchemeSemver:
		return parseSemver(text)
	case SchemeGo:
		return parseGo(text)
	case SchemePython:
		return parsePython(text)
	}
	return Version{}, fmt.Errorf("domain: version_scheme %q は semver|go|python のいずれでもない", scheme)
}

func parseSemver(text string) (Version, error) {
	match := semverRe.FindStringSubmatch(text)
	if match == nil {
		return Version{}, fmt.Errorf(
			"domain: semver version %q が MAJOR.MINOR.PATCH[-prerelease] に合わない。"+
				"leading `v` と build metadata は含めない", text)
	}
	version := Version{
		scheme: SchemeSemver,
		text:   text,
		major:  mustAtoi(match[1]),
		minor:  mustAtoi(match[2]),
		patch:  mustAtoi(match[3]),
		stage:  stageFinal,
	}
	if match[4] != "" {
		idents := strings.Split(match[4], ".")
		for _, ident := range idents {
			if ident == "" || !semverIdent.MatchString(ident) {
				return Version{}, fmt.Errorf("domain: semver version %q のprerelease識別子 %q が不正", text, ident)
			}
			// 数値識別子はleading zeroを禁止する（SemVer 2.0.0）。
			if isAllDigits(ident) && !numRe.MatchString(ident) {
				return Version{}, fmt.Errorf("domain: semver version %q のprerelease識別子 %q にleading zeroがある", text, ident)
			}
		}
		version.semverPre = idents
		version.stage = stageSemverPre
	}
	return version, nil
}

func parseGo(text string) (Version, error) {
	match := goRe.FindStringSubmatch(text)
	if match == nil {
		return Version{}, fmt.Errorf(
			"domain: go version %q が MAJOR.MINOR / MAJOR.MINOR.PATCH / MAJOR.MINORbetaN|rcN に合わない", text)
	}
	version := Version{
		scheme: SchemeGo,
		text:   text,
		major:  mustAtoi(match[1]),
		minor:  mustAtoi(match[2]),
		stage:  stageFinal,
	}
	switch {
	case match[3] != "":
		version.patch = mustAtoi(match[3])
	case match[4] != "":
		// prereleaseにpatchは無い。比較はstageで先に決着する。
		version.stageNum = mustAtoi(match[5])
		if match[4] == "beta" {
			version.stage = stageGoBeta
		} else {
			version.stage = stageGoRC
		}
	}
	return version, nil
}

func parsePython(text string) (Version, error) {
	match := pythonRe.FindStringSubmatch(text)
	if match == nil {
		return Version{}, fmt.Errorf(
			"domain: python version %q が MAJOR.MINOR.PATCH[aN|bN|rcN] に合わない", text)
	}
	version := Version{
		scheme: SchemePython,
		text:   text,
		major:  mustAtoi(match[1]),
		minor:  mustAtoi(match[2]),
		patch:  mustAtoi(match[3]),
		stage:  stageFinal,
	}
	if match[4] != "" {
		version.stageNum = mustAtoi(match[5])
		switch match[4] {
		case "a":
			version.stage = stagePythonA
		case "b":
			version.stage = stagePythonB
		default:
			version.stage = stagePythonRC
		}
	}
	return version, nil
}

// Scheme は比較規則を返す。
func (v Version) Scheme() VersionScheme { return v.scheme }

// String はcatalogの正規version文字列を返す。入力一致はこの値のbyte完全一致で行う。
func (v Version) String() string { return v.text }

// IsZero はParseVersionを通していない値かどうかを返す。
func (v Version) IsZero() bool { return v.text == "" }

// Compare は同じschemeのversion同士を比較する。
//
// v < other なら負、等しければ0、v > other なら正を返す。schemeが異なる場合は
// errorにする。異なるschemeのversionは順序を定義できず、暗黙に片方の規則で
// 比較すると誤った「最新」を選ぶためである。
func (v Version) Compare(other Version) (int, error) {
	if v.IsZero() || other.IsZero() {
		return 0, fmt.Errorf("domain: 未初期化のversionは比較できない")
	}
	if v.scheme != other.scheme {
		return 0, fmt.Errorf("domain: scheme %q と %q のversionは比較できない", v.scheme, other.scheme)
	}

	switch v.scheme {
	case SchemeGo:
		// 比較順は major/minor、beta<rc<final、prerelease番号、finalのpatch
		// （docs/06-tool-definition.md §4）。
		if c := compareInts(v.major, other.major); c != 0 {
			return c, nil
		}
		if c := compareInts(v.minor, other.minor); c != 0 {
			return c, nil
		}
		if c := compareInts(int(v.stage), int(other.stage)); c != 0 {
			return c, nil
		}
		if c := compareInts(v.stageNum, other.stageNum); c != 0 {
			return c, nil
		}
		return compareInts(v.patch, other.patch), nil

	case SchemePython:
		// 比較順は数値3要素、a<b<rc<final、prerelease番号。
		if c := compareInts(v.major, other.major); c != 0 {
			return c, nil
		}
		if c := compareInts(v.minor, other.minor); c != 0 {
			return c, nil
		}
		if c := compareInts(v.patch, other.patch); c != 0 {
			return c, nil
		}
		if c := compareInts(int(v.stage), int(other.stage)); c != 0 {
			return c, nil
		}
		return compareInts(v.stageNum, other.stageNum), nil

	default:
		// SemVer 2.0.0のprecedence。
		if c := compareInts(v.major, other.major); c != 0 {
			return c, nil
		}
		if c := compareInts(v.minor, other.minor); c != 0 {
			return c, nil
		}
		if c := compareInts(v.patch, other.patch); c != 0 {
			return c, nil
		}
		return compareSemverPre(v.semverPre, other.semverPre), nil
	}
}

// compareSemverPre はSemVer 2.0.0のprerelease precedenceを実装する。
//
// prereleaseを持たない側が大きい。識別子は左から比較し、数値同士は数値順、
// 文字列同士はASCII順、数値は文字列より小さい。すべて等しければ識別子数が
// 多い側が大きい。
func compareSemverPre(left, right []string) int {
	if len(left) == 0 && len(right) == 0 {
		return 0
	}
	if len(left) == 0 {
		return 1
	}
	if len(right) == 0 {
		return -1
	}
	for i := 0; i < len(left) && i < len(right); i++ {
		l, r := left[i], right[i]
		lNum, rNum := isAllDigits(l), isAllDigits(r)
		switch {
		case lNum && rNum:
			if c := compareInts(mustAtoi(l), mustAtoi(r)); c != 0 {
				return c
			}
		case lNum:
			return -1
		case rNum:
			return 1
		default:
			if c := strings.Compare(l, r); c != 0 {
				return c
			}
		}
	}
	return compareInts(len(left), len(right))
}

func compareInts(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// mustAtoi は正規表現で数値と確定した部分文字列を整数へ変換する。
// 正規表現を通った値だけを渡すため、変換失敗は起こらない。
func mustAtoi(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		// 呼出側が正規表現でmatchさせた部分だけを渡す契約であり、ここへ来るのは
		// 実装の不整合である。production pathのerror処理としてpanicを使うのではなく、
		// 到達不能を示す。
		panic("domain: 数値化できない部分文字列を渡した: " + s)
	}
	return n
}
