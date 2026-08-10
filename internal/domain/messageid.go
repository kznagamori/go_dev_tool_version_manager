package domain

import (
	"fmt"
	"regexp"
	"strings"
)

// messageIDRe はdocs/04-storage-and-data.md §7のmessage ID grammarである。
var messageIDRe = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)*$`)

// MessageIDMaxLength は§7が定めるmessage IDの最大長である。
const MessageIDMaxLength = 128

// MessageIDMinSegments は§7が定めるsegmentの最小数である。
const MessageIDMinSegments = 2

// MessageID は表示文を引くためのstable keyである。
//
// docs/02-architecture.md §14・§10に従い、typed error、progress、warningは
// 表示文そのものではなくこのIDとscalar parameterを返し、文はpresentation層が
// message catalogから生成する。表示文を値へ埋め込むと、CLI human表示とJSON
// envelopeで同じ失敗が別の文言になり、言語追加時にcatalogの網羅性も検査できない。
//
// 先頭segmentは`error`、`warning`、`install`のような分類として使う（§7）。
type MessageID struct {
	id string
}

// ParseMessageID は文字列をMessageIDへ変換する。
//
// grammar外、segment不足、長さ超過をすべて拒否する。`..`や`.start`のような壊れた
// keyを受理するとcatalogのlookupが静かに失敗するためである。
func ParseMessageID(text string) (MessageID, error) {
	switch {
	case text == "":
		return MessageID{}, fmt.Errorf("domain: message IDが空")
	case len(text) > MessageIDMaxLength:
		return MessageID{}, fmt.Errorf(
			"domain: message ID %q は%d文字を超える", text, MessageIDMaxLength)
	case !messageIDRe.MatchString(text):
		return MessageID{}, fmt.Errorf(
			"domain: message ID %q が `^[a-z][a-z0-9_]*(\\.[a-z][a-z0-9_]*)*$` に一致しない", text)
	}
	if segments := strings.Count(text, ".") + 1; segments < MessageIDMinSegments {
		return MessageID{}, fmt.Errorf(
			"domain: message ID %q のsegmentが%d件しかない（%d件以上が必要）",
			text, segments, MessageIDMinSegments)
	}
	return MessageID{id: text}, nil
}

// String はmessage ID文字列を返す。
func (m MessageID) String() string { return m.id }

// Segments はdotで区切ったsegment列を返す。
//
// 先頭segmentによる分類（`error.*`、`warning.*`）をcatalog検査が使う。
func (m MessageID) Segments() []string {
	if m.id == "" {
		return nil
	}
	return strings.Split(m.id, ".")
}

// IsZero はParseを通していない値かどうかを返す。
func (m MessageID) IsZero() bool { return m.id == "" }
