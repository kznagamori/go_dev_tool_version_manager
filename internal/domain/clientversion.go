package domain

import (
	"fmt"
	"regexp"
	"strconv"
	"time"
)

// clientVersionRe はdocs/11-quality-and-ci.md §2が明示したCalVer grammarである。
var clientVersionRe = regexp.MustCompile(
	`^([0-9]{4})[.](0[1-9]|1[0-2])[.](0[1-9]|[12][0-9]|3[01])[.]([0-9]{2})$`)

// ClientVersion はclientのCalVer `YYYY.MM.DD.XX`である。
//
// docs/11-quality-and-ci.md §2が「比較は4個の10進整数tuple。SemVerではなく、
// SemVerへ変換せずprerelease/build suffixを付けない」と定める。registryの
// `client_min_version`/`client_max_version`との照合（docs/07-registry-and-tools.md
// §3）が大小比較を要するため、domain値として持つ。
//
// zero値は未設定である。development buildの`devel`はこの型で表さない。CalVerでは
// なく、releaseの範囲判定に使えないためである。
type ClientVersion struct {
	text     string
	year     int
	month    int
	day      int
	sequence int
}

// ParseClientVersion はCalVer文字列をClientVersionへ変換する。
//
// grammarに加えて実在日付を検査する。`2026.02.30.00`はgrammarを満たすが存在せず、
// 日付として比較する意味を持たない。
func ParseClientVersion(text string) (ClientVersion, error) {
	match := clientVersionRe.FindStringSubmatch(text)
	if match == nil {
		return ClientVersion{}, fmt.Errorf(
			"domain: client version %q が `YYYY.MM.DD.XX` に合わない", text)
	}
	year, _ := strconv.Atoi(match[1])
	month, _ := strconv.Atoi(match[2])
	day, _ := strconv.Atoi(match[3])
	sequence, _ := strconv.Atoi(match[4])

	// time.Dateは範囲外の日をnormalizeするため、正規化後と一致するかで実在を見る。
	stamp := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
	if stamp.Year() != year || stamp.Month() != time.Month(month) || stamp.Day() != day {
		return ClientVersion{}, fmt.Errorf("domain: client version %q の日付が存在しない", text)
	}
	return ClientVersion{
		text: text, year: year, month: month, day: day, sequence: sequence,
	}, nil
}

// String は元のCalVer文字列を返す。
func (v ClientVersion) String() string { return v.text }

// IsZero はParseClientVersionを通していない値かどうかを返す。
func (v ClientVersion) IsZero() bool { return v.text == "" }

// Compare は4個の10進整数tupleとして比較する。
//
// v < other なら負、等しければ0、v > other なら正を返す。文字列比較にしないのは、
// `XX`が2桁固定でもyear/month/dayの区切りを含む文字列順が整数順と一致する保証を
// 実装の外へ出さないためである。
func (v ClientVersion) Compare(other ClientVersion) int {
	pairs := [][2]int{
		{v.year, other.year},
		{v.month, other.month},
		{v.day, other.day},
		{v.sequence, other.sequence},
	}
	for _, pair := range pairs {
		if c := compareInts(pair[0], pair[1]); c != 0 {
			return c
		}
	}
	return 0
}
