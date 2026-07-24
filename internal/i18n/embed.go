package i18n

import (
	"embed"
	"fmt"
)

// baseMessagesFS は client 本体同梱の基本 message catalog である（14章15節）。registry
// の message は tool 固有 notes だけで、この基本 error を上書きできない。
//
//go:embed messages/ja.toml messages/en.toml
var baseMessagesFS embed.FS

// BaseCatalog は同梱の基本 catalog を language（"ja" または "en"）で読み込む。
func BaseCatalog(language string) (*Catalog, error) {
	if language != "ja" && language != "en" {
		return nil, fmt.Errorf("未対応の language: %q", language)
	}
	data, err := baseMessagesFS.ReadFile("messages/" + language + ".toml")
	if err != nil {
		return nil, fmt.Errorf("同梱 catalog %q の読込みに失敗: %w", language, err)
	}
	return Load(data)
}
