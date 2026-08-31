package registry

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/pelletier/go-toml/v2"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/security"
)

// MessageCatalogPath はregistry rootからのcatalog相対pathである。
//
// v0.1の言語は日本語だけである（docs/04-storage-and-data.md §20）。
const MessageCatalogPath = "messages/ja.toml"

// MessageFileMaxBytes はcatalog全体の上限である（§20「全体2 MiB」）。
const MessageFileMaxBytes = 2 << 20

// MessageMaxBytes は1 messageの上限である（§20「1 message 8 KiB」）。
const MessageMaxBytes = 8 << 10

// MessageCount はcatalogが持つmessage数である。
//
// production GoとregistryのTOMLが出しうるmessage IDの数と一致する。件数を定数で
// 持つのは、message IDを増減させたときにcatalogの更新漏れへ気付くためである。
// 網羅そのものはscripts/ci/check_messages.pyがsource全体を走査して検査する。
const MessageCount = 87

// MessageCatalog は`messages/ja.toml`のtyped表現である。
type MessageCatalog struct {
	// entries はmessage ID → templateである。
	entries map[string]string
	// placeholders はmessage ID → templateが使うparameter名（出現順）である。
	//
	// parse時に走査済みの結果を持つ。参照のたびに再走査すると、走査が失敗しうる
	// 前提のcodeを書くことになるが、[ParseMessageCatalog]を通った値は必ず走査
	// できるため、その分岐は到達しない。
	placeholders map[string][]string
	// ids は宣言順のmessage IDである。診断を宣言順で出すために持つ。
	ids []string
}

// Len はcatalogが持つmessage数を返す。
func (c MessageCatalog) Len() int { return len(c.ids) }

// IDs は宣言順のmessage IDを返す。
func (c MessageCatalog) IDs() []string {
	return append([]string(nil), c.ids...)
}

// Template はmessage IDに対応するtemplateを返す。
func (c MessageCatalog) Template(id string) (string, bool) {
	template, ok := c.entries[id]
	return template, ok
}

// Placeholders はtemplateが使うparameter名を出現順・重複なしで返す。
//
// catalogが存在しないmessage IDを渡した場合はfalseを返す。
func (c MessageCatalog) Placeholders(id string) ([]string, bool) {
	if _, ok := c.entries[id]; !ok {
		return nil, false
	}
	return append([]string(nil), c.placeholders[id]...), true
}

// ParseMessageCatalog は`messages/ja.toml`を読む（docs/04-storage-and-data.md §20）。
//
// keyは§7のmessage ID grammar、値はUTF-8 template string。placeholderは`{name}`、
// literal braceは`{{`/`}}`。template内ANSI、terminal control、秘密値展開を拒否する。
//
// 表示側がescapeしても、catalogが制御文字を持てばlogやreportを経由して端末へ届く。
// 混入を検出できる唯一の場所がここなので、読み込み時にfail closedで弾く。
func ParseMessageCatalog(data []byte) (MessageCatalog, *domain.Error) {
	if len(data) > MessageFileMaxBytes {
		return MessageCatalog{}, invalidError(fmt.Errorf(
			"%s が%d byteを超える（%d byte）", MessageCatalogPath, MessageFileMaxBytes, len(data)))
	}
	if !utf8.Valid(data) {
		return MessageCatalog{}, invalidError(fmt.Errorf(
			"%s がUTF-8として不正である", MessageCatalogPath))
	}

	// TOMLの`a.b = "x"`はdotted keyであり、decodeすると`a`の下に`b`を持つtableへ
	// なる。§20のmessage IDはdotted keyそのものなので、decode後に平坦化して
	// `a.b`へ戻す。
	var raw map[string]any
	decoder := toml.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&raw); err != nil {
		return MessageCatalog{}, invalidError(describeDecodeError(err))
	}
	entries := make(map[string]string)
	if err := flattenMessages(raw, "", entries); err != nil {
		return MessageCatalog{}, invalidError(err)
	}

	ids, err := declarationOrder(data, entries)
	if err != nil {
		return MessageCatalog{}, invalidError(err)
	}
	placeholders := make(map[string][]string, len(ids))
	for _, id := range ids {
		names, err := checkMessage(id, entries[id])
		if err != nil {
			return MessageCatalog{}, invalidError(err)
		}
		placeholders[id] = names
	}
	return MessageCatalog{entries: entries, placeholders: placeholders, ids: ids}, nil
}

// flattenMessages はdecode結果のnested tableをdotted message IDへ戻す。
//
// leafがstringでなければerrorにする。message catalogの値はtemplate stringだけで
// あり、数値やbooleanを文字列化して受けると表示側の契約が崩れる。
func flattenMessages(node map[string]any, prefix string, out map[string]string) error {
	for key, value := range node {
		id := key
		if prefix != "" {
			id = prefix + "." + key
		}
		switch typed := value.(type) {
		case string:
			out[id] = typed
		case map[string]any:
			if err := flattenMessages(typed, id, out); err != nil {
				return err
			}
		default:
			return fmt.Errorf("message ID %q の値がtemplate stringでない（%T）", id, value)
		}
	}
	return nil
}

// declarationOrder はsourceの行順でmessage IDを返す。
//
// decode済みのkey集合と突き合わせ、両者が一致しない場合はerrorにする。TOMLの
// table記法やdotted keyで書かれるとこの行走査と食い違うため、一致検査で捕まえる。
func declarationOrder(data []byte, entries map[string]string) ([]string, error) {
	var ids []string
	seen := make(map[string]struct{}, len(entries))
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		index := strings.Index(line, "=")
		if index < 0 {
			continue
		}
		key := strings.TrimSpace(line[:index])
		if _, ok := entries[key]; !ok {
			continue
		}
		if _, duplicated := seen[key]; duplicated {
			continue
		}
		seen[key] = struct{}{}
		ids = append(ids, key)
	}
	if len(ids) != len(entries) {
		missing := make([]string, 0)
		for key := range entries {
			if _, ok := seen[key]; !ok {
				missing = append(missing, key)
			}
		}
		sort.Strings(missing)
		return nil, fmt.Errorf(
			"message IDは1行1件の`<id> = \"<template>\"`で書く（宣言順を読めないkey: %v）", missing)
	}
	return ids, nil
}

// checkMessage は1 entryの契約を検査し、templateが使うplaceholder名を返す。
func checkMessage(id, template string) ([]string, error) {
	if _, err := domain.ParseMessageID(id); err != nil {
		return nil, fmt.Errorf("message ID %q: %w", id, err)
	}
	if template == "" {
		return nil, fmt.Errorf("message ID %q のtemplateが空", id)
	}
	if len(template) > MessageMaxBytes {
		return nil, fmt.Errorf(
			"message ID %q のtemplateが%d byteを超える（%d byte）",
			id, MessageMaxBytes, len(template))
	}
	if err := checkControlChars(id, template); err != nil {
		return nil, err
	}
	names, err := scanTemplate(template)
	if err != nil {
		return nil, fmt.Errorf("message ID %q: %w", id, err)
	}
	for _, name := range names {
		if err := domain.ValidateParameterKey(name); err != nil {
			return nil, fmt.Errorf("message ID %q のplaceholder: %w", id, err)
		}
		// 秘密値展開を禁じる（§20）。docs/10-security.md §9.2がmask対象とする
		// 名前をtemplateが展開すると、maskを通さない経路で表示される。
		if security.IsSecretEnvName(name) || security.IsSecretHeader(name) {
			return nil, fmt.Errorf(
				"message ID %q のplaceholder `{%s}` は秘密値として除去される名前である", id, name)
		}
	}
	return names, nil
}

// checkControlChars はANSI escapeとterminal control文字を拒否する（§20）。
//
// 改行とタブも拒否する。message catalogの値は1行のtemplateであり、表示側が
// 幅計算と折返しを行う。catalogが改行を持つと、table表示やJSON envelopeで
// 同じmessageが別のlayoutになる。
func checkControlChars(id, template string) error {
	for index, r := range template {
		switch {
		case r == 0x1B:
			return fmt.Errorf(
				"message ID %q のtemplate %d byte目にANSI escape（ESC）がある", id, index)
		case r == '\n' || r == '\r' || r == '\t':
			return fmt.Errorf(
				"message ID %q のtemplate %d byte目に改行またはtabがある。templateは1行にする",
				id, index)
		case r < 0x20 || r == 0x7F:
			return fmt.Errorf(
				"message ID %q のtemplate %d byte目にC0制御文字 U+%04X がある", id, index, r)
		case unicode.Is(unicode.Cf, r):
			// U+200E LEFT-TO-RIGHT MARKのようなformat文字は、表示上の文字順を
			// 変えてtextの意味を偽装できる。
			return fmt.Errorf(
				"message ID %q のtemplate %d byte目にformat制御文字 U+%04X がある", id, index, r)
		}
	}
	return nil
}

// scanTemplate はtemplateを走査してplaceholder名を出現順・重複なしで返す。
//
// placeholderは`{name}`、literal braceは`{{`/`}}`（§20）。閉じない`{`、空の
// `{}`、対応しない`}`を拒否する。壊れたtemplateを黙って素通しすると、render時に
// placeholderがそのまま利用者へ出る。
func scanTemplate(template string) ([]string, error) {
	var names []string
	seen := make(map[string]struct{})
	for index := 0; index < len(template); {
		switch template[index] {
		case '{':
			if index+1 < len(template) && template[index+1] == '{' {
				index += 2
				continue
			}
			end := strings.IndexByte(template[index+1:], '}')
			if end < 0 {
				return nil, fmt.Errorf("%d byte目の `{` が閉じていない", index)
			}
			name := template[index+1 : index+1+end]
			if name == "" {
				return nil, fmt.Errorf("%d byte目の placeholder が空である", index)
			}
			if strings.ContainsAny(name, "{") {
				return nil, fmt.Errorf("%d byte目の placeholder が入れ子になっている", index)
			}
			if _, ok := seen[name]; !ok {
				seen[name] = struct{}{}
				names = append(names, name)
			}
			index += end + 2
		case '}':
			if index+1 < len(template) && template[index+1] == '}' {
				index += 2
				continue
			}
			return nil, fmt.Errorf("%d byte目に対応する `{` の無い `}` がある", index)
		default:
			index++
		}
	}
	return names, nil
}
