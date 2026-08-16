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

	major    uint64
	minor    uint64
	patch    uint64
	stage    stage
	stageNum uint64
	// semverPre はsemverのprerelease識別子列である。空はfinalを表す。
	semverPre []semverIdent
}

// semverIdent はsemverのprerelease識別子1件である。
//
// 数値かどうかと数値としての値をParseVersionの時点で確定させる。Compareが比較の
// たびに文字列を数値化すると、範囲外の識別子で失敗しうる変換が比較側に残り、
// error を返せない[Version.Compare]の途中で扱えない値が出る。範囲検査はparseで
// 済ませ、比較は大小判定だけにする。
type semverIdent struct {
	text    string
	num     uint64
	numeric bool
}

// 数値要素は不要なleading zeroを禁止する（docs/06-tool-definition.md §4）。
var (
	numRe         = regexp.MustCompile(`^(0|[1-9][0-9]*)$`)
	semverRe      = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-([0-9A-Za-z.\-]+))?$`)
	goRe          = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:\.(0|[1-9][0-9]*)|(beta|rc)([1-9][0-9]*))?$`)
	pythonRe      = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:(a|b|rc)([1-9][0-9]*))?$`)
	semverIdentRe = regexp.MustCompile(`^[0-9A-Za-z\-]+$`)
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
	numbers, err := parseNumbers(text, match[1], match[2], match[3])
	if err != nil {
		return Version{}, err
	}
	version := Version{
		scheme: SchemeSemver,
		text:   text,
		major:  numbers[0],
		minor:  numbers[1],
		patch:  numbers[2],
		stage:  stageFinal,
	}
	if match[4] != "" {
		idents, err := parseSemverPre(text, match[4])
		if err != nil {
			return Version{}, err
		}
		version.semverPre = idents
		version.stage = stageSemverPre
	}
	return version, nil
}

// parseSemverPre はprerelease部を識別子列へ分解する。
func parseSemverPre(text, raw string) ([]semverIdent, error) {
	parts := strings.Split(raw, ".")
	idents := make([]semverIdent, len(parts))
	for i, part := range parts {
		if part == "" || !semverIdentRe.MatchString(part) {
			return nil, fmt.Errorf("domain: semver version %q のprerelease識別子 %q が不正", text, part)
		}
		if !isAllDigits(part) {
			idents[i] = semverIdent{text: part}
			continue
		}
		// 数値識別子はleading zeroを禁止する（SemVer 2.0.0）。
		if !numRe.MatchString(part) {
			return nil, fmt.Errorf("domain: semver version %q のprerelease識別子 %q にleading zeroがある", text, part)
		}
		numbers, err := parseNumbers(text, part)
		if err != nil {
			return nil, err
		}
		idents[i] = semverIdent{text: part, num: numbers[0], numeric: true}
	}
	return idents, nil
}

func parseGo(text string) (Version, error) {
	match := goRe.FindStringSubmatch(text)
	if match == nil {
		return Version{}, fmt.Errorf(
			"domain: go version %q が MAJOR.MINOR / MAJOR.MINOR.PATCH / MAJOR.MINORbetaN|rcN に合わない", text)
	}
	numbers, err := parseNumbers(text, match[1], match[2])
	if err != nil {
		return Version{}, err
	}
	version := Version{
		scheme: SchemeGo,
		text:   text,
		major:  numbers[0],
		minor:  numbers[1],
		stage:  stageFinal,
	}
	switch {
	case match[3] != "":
		patch, err := parseNumbers(text, match[3])
		if err != nil {
			return Version{}, err
		}
		version.patch = patch[0]
	case match[4] != "":
		// prereleaseにpatchは無い。比較はstageで先に決着する。
		stageNum, err := parseNumbers(text, match[5])
		if err != nil {
			return Version{}, err
		}
		version.stageNum = stageNum[0]
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
	numbers, err := parseNumbers(text, match[1], match[2], match[3])
	if err != nil {
		return Version{}, err
	}
	version := Version{
		scheme: SchemePython,
		text:   text,
		major:  numbers[0],
		minor:  numbers[1],
		patch:  numbers[2],
		stage:  stageFinal,
	}
	if match[4] != "" {
		stageNum, err := parseNumbers(text, match[5])
		if err != nil {
			return Version{}, err
		}
		version.stageNum = stageNum[0]
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

// IsPrerelease は正規versionがschemeのprerelease構文を持つかを返す。
//
// docs/06-tool-definition.md §6.1が「`channel_pointer`を省略した場合、正規
// versionが各schemeのprerelease構文を持てば`prerelease`、それ以外は`stable`と
// する」と定める。その判定に使う。
//
// 判定は**構文だけ**で行う。semverは`-`以降のprerelease識別子の有無、goは
// `betaN`/`rcN`、pythonは`aN`/`bN`/`rcN`である。version番号の大小や公開日から
// prereleaseを推測しない。未初期化のversionはfalseを返す。
func (v Version) IsPrerelease() bool {
	if v.IsZero() {
		return false
	}
	return v.stage != stageFinal
}

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
		// （docs/06-tool-definition.md §4）。stageをpatchより先に見るため、
		// prereleaseは同じminorのどのfinal patchよりも小さくなる。
		if c := compareUint64(v.major, other.major); c != 0 {
			return c, nil
		}
		if c := compareUint64(v.minor, other.minor); c != 0 {
			return c, nil
		}
		if c := compareInts(int(v.stage), int(other.stage)); c != 0 {
			return c, nil
		}
		if c := compareUint64(v.stageNum, other.stageNum); c != 0 {
			return c, nil
		}
		return compareUint64(v.patch, other.patch), nil

	case SchemePython:
		// 比較順は数値3要素、a<b<rc<final、prerelease番号。goと違いpatchを
		// stageより先に見るため、上のpatchのprereleaseが下のpatchのfinalより大きい。
		if c := compareUint64(v.major, other.major); c != 0 {
			return c, nil
		}
		if c := compareUint64(v.minor, other.minor); c != 0 {
			return c, nil
		}
		if c := compareUint64(v.patch, other.patch); c != 0 {
			return c, nil
		}
		if c := compareInts(int(v.stage), int(other.stage)); c != 0 {
			return c, nil
		}
		return compareUint64(v.stageNum, other.stageNum), nil

	default:
		// SemVer 2.0.0のprecedence。
		if c := compareUint64(v.major, other.major); c != 0 {
			return c, nil
		}
		if c := compareUint64(v.minor, other.minor); c != 0 {
			return c, nil
		}
		if c := compareUint64(v.patch, other.patch); c != 0 {
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
func compareSemverPre(left, right []semverIdent) int {
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
		switch {
		case l.numeric && r.numeric:
			if c := compareUint64(l.num, r.num); c != 0 {
				return c
			}
		case l.numeric:
			return -1
		case r.numeric:
			return 1
		default:
			if c := strings.Compare(l.text, r.text); c != 0 {
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

func compareUint64(a, b uint64) int {
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

// parseNumbers は正規表現が数値と確定させた部分文字列群を数値へ変換する。
//
// 正規表現はleading zeroと非数字を既に排除しているため、ここで起こりうる失敗は
// 桁あふれだけである。version文字列は上流catalogのJSONに由来する外部入力であり
// （docs/06-tool-definition.md §6.3）、桁数に上限が無い。**桁あふれをparse errorに
// して閉じる**。以前のようにpanicへ倒すと、上流が異常なversionを1件返しただけで
// processが落ちる。
//
// 表現の範囲は64 bit符号なし整数とする。Goの`int`のままだと32 bit platformと
// 64 bit platformで受理するversionが変わり、同じregistryがplatformごとに違う
// 結果になる。
func parseNumbers(text string, parts ...string) ([]uint64, error) {
	values := make([]uint64, len(parts))
	for i, part := range parts {
		value, err := strconv.ParseUint(part, 10, 64)
		if err != nil {
			return nil, fmt.Errorf(
				"domain: version %q の数値要素 %q が64 bit符号なし整数の範囲を超える", text, part)
		}
		values[i] = value
	}
	return values, nil
}
