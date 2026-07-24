package i18n

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// catalogSchema は message catalog の対応 schema major である。
const catalogSchema = 1

// catalogFile は messages/<lang>.toml の decode 先である。DisallowUnknownFields で
// top-level の未知 key を拒否しつつ、[messages] 配下の message ID は map で受ける。
type catalogFile struct {
	Schema   int               `toml:"schema"`
	Language string            `toml:"language"`
	Messages map[string]string `toml:"messages"`
}

// Catalog は 1 言語の message ID → template を保持する（14章15節）。
type Catalog struct {
	language string
	messages map[string]string
}

// Load は message catalog TOML を strict parse して Catalog を生成する。未知 top-level
// key、未対応 schema/language、不正な message ID、不正な template は拒否する。
func Load(data []byte) (*Catalog, error) {
	var cf catalogFile
	dec := toml.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cf); err != nil {
		return nil, fmt.Errorf("message catalog の parse に失敗: %w", err)
	}
	if cf.Schema != catalogSchema {
		return nil, fmt.Errorf("message catalog schema %d は非対応（想定 %d）", cf.Schema, catalogSchema)
	}
	if cf.Language != "ja" && cf.Language != "en" {
		return nil, fmt.Errorf("message catalog language %q は非対応", cf.Language)
	}
	for id, tmpl := range cf.Messages {
		if !validMessageID(id) {
			return nil, fmt.Errorf("不正な message ID: %q", id)
		}
		if err := validateTemplate(tmpl); err != nil {
			return nil, fmt.Errorf("message %q: %w", id, err)
		}
	}
	return &Catalog{language: cf.Language, messages: cf.Messages}, nil
}

// Language は catalog の言語（"ja" または "en"）を返す。
func (c *Catalog) Language() string { return c.language }

// Message は message ID の template を返す。存在しなければ ok=false。
func (c *Catalog) Message(id string) (string, bool) {
	v, ok := c.messages[id]
	return v, ok
}

// Render は message ID の template を args で置換した文字列を返す。存在しなければ ok=false。
func (c *Catalog) Render(id string, args map[string]string) (string, bool) {
	tmpl, ok := c.messages[id]
	if !ok {
		return "", false
	}
	return substitute(tmpl, args), true
}

// IDs は保持する message ID を昇順で返す。
func (c *Catalog) IDs() []string {
	ids := make([]string, 0, len(c.messages))
	for id := range c.messages {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// ValidateParity は 2 つの catalog の message ID 集合と、各 message の placeholder 集合が
// 完全一致することを検査する（14章15節, 13章12節）。不一致があれば理由を列挙したエラーを返す。
func ValidateParity(a, b *Catalog) error {
	var problems []string
	for id := range a.messages {
		if _, ok := b.messages[id]; !ok {
			problems = append(problems, fmt.Sprintf("message ID %q は %s にのみ存在", id, a.language))
		}
	}
	for id := range b.messages {
		if _, ok := a.messages[id]; !ok {
			problems = append(problems, fmt.Sprintf("message ID %q は %s にのみ存在", id, b.language))
		}
	}
	for id, ta := range a.messages {
		tb, ok := b.messages[id]
		if !ok {
			continue
		}
		if !sameStringSet(placeholderSet(ta), placeholderSet(tb)) {
			problems = append(problems, fmt.Sprintf("message ID %q の placeholder が %s と %s で不一致", id, a.language, b.language))
		}
	}
	if len(problems) > 0 {
		sort.Strings(problems)
		return fmt.Errorf("message catalog の placeholder/ID 不一致: %s", strings.Join(problems, "; "))
	}
	return nil
}
