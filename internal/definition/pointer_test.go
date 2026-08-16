package definition

import (
	"strings"
	"testing"
)

// TestValidatePointerFollowsRFC6901 は§6.1の「pointerはすべてRFC 6901」を固定する。
//
//	json-pointer    = *( "/" reference-token )
//	reference-token = *( unescaped / escaped )
//	escaped         = "~" ( "0" / "1" )
func TestValidatePointerFollowsRFC6901(t *testing.T) {
	valid := []string{
		// 空は文書全体を指す。RFC 6901が明示的に許す。
		"",
		"/",
		"/version",
		"/releases-index",
		"/a/b/c",
		"/0",
		// escaped: `~0`は`~`、`~1`は`/`を表す。
		"/a~0b",
		"/a~1b",
		"/~0~1",
		// 空のreference tokenは合法である（keyが空文字のmember）。
		"//",
		"/a//b",
		// 非ASCIIもUTF-8として正しければ通る。
		"/バージョン",
	}
	for _, pointer := range valid {
		if err := validatePointer(pointer, "pointer"); err != nil {
			t.Errorf("%q が落ちた: %v", pointer, err)
		}
	}

	invalid := []struct{ name, pointer string }{
		{"`/`で始まらない", "version"},
		{"`~`で終わる", "/a~"},
		{"`~`の後が2", "/a~2b"},
		{"`~`の後が文字", "/a~xb"},
		{"上限超過", "/" + strings.Repeat("a", PointerMaxBytes)},
		{"UTF-8でない", "/\xff\xfe"},
	}
	for _, test := range invalid {
		t.Run(test.name, func(t *testing.T) {
			if err := validatePointer(test.pointer, "pointer"); err == nil {
				t.Errorf("%q が通った", test.pointer)
			}
		})
	}
}

// TestParseRejectsInvalidPointer はdefinition経由でpointer検査が効くことを固定する。
func TestParseRejectsInvalidPointer(t *testing.T) {
	// 必須pointerと任意pointerの両方を確かめる。
	tests := []struct{ name, old, value string }{
		{"items_pointer", `items_pointer = ""`, `items_pointer = "version"`},
		{"version_pointer", `version_pointer = "/version"`, `version_pointer = "/a~"`},
		{"published_at_pointer", `published_at_pointer = "/date"`, `published_at_pointer = "date"`},
		{"required_tokens_pointer", `required_tokens_pointer = "/files"`,
			`required_tokens_pointer = "files"`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			block := strings.Replace(specVersionSourceBlock, test.old, test.value, 1)
			_, err := withSource(t, block)
			if err == nil {
				t.Fatalf("%s が通った", test.value)
			}
			assertReason(t, err, reasonPointer)
		})
	}
}

// TestAssetFieldsContract は§6.5の`asset_fields`を固定する。
func TestAssetFieldsContract(t *testing.T) {
	if len(assetFieldOrder) != AssetFieldCount {
		t.Fatalf("assetFieldOrder = %d件, want %d件", len(assetFieldOrder), AssetFieldCount)
	}
	// §6.5の13値がすべてparseできる。
	for _, field := range assetFieldOrder {
		if _, err := parseAssetField(string(field)); err != nil {
			t.Errorf("asset field %q が落ちた: %v", field, err)
		}
	}
	if _, err := parseAssetField("kind"); err == nil {
		t.Error("集合外のasset fieldが通った")
	}

	tests := []struct {
		name       string
		table      string
		wantReason string
	}{
		{"集合外のkey", "[platforms.version_source.asset_fields]\nkind = \"/kind\"\n", reasonEnum},
		{"値がpointerでない", "[platforms.version_source.asset_fields]\nname = \"filename\"\n", reasonPointer},
		{"空table", "[platforms.version_source.asset_fields]\n", reasonConditional},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := withSource(t, specVersionSourceBlock+"\n"+test.table)
			if err == nil {
				t.Fatal("Parse = nil, want error")
			}
			assertReason(t, err, test.wantReason)
		})
	}
}

// TestMetadataFieldsContract は§6.5の`metadata_fields`を固定する。
func TestMetadataFieldsContract(t *testing.T) {
	t.Run("正当なkeyとpointer", func(t *testing.T) {
		table := "[platforms.version_source.metadata_fields]\nrelease_date = \"/date\"\n"
		value, err := withSource(t, specVersionSourceBlock+"\n"+table)
		if err != nil {
			t.Fatalf("Parse = %s", describe(err))
		}
		if value.Platforms[0].VersionSource.MetadataFields["release_date"] != "/date" {
			t.Errorf("metadata_fields = %v", value.Platforms[0].VersionSource.MetadataFields)
		}
	})

	tests := []struct {
		name       string
		table      string
		wantReason string
	}{
		{"keyがgrammar外", "[platforms.version_source.metadata_fields]\nReleaseDate = \"/date\"\n",
			reasonIdentifier},
		{"keyにhyphen", "[platforms.version_source.metadata_fields]\nrelease-date = \"/date\"\n",
			reasonIdentifier},
		{"値がpointerでない", "[platforms.version_source.metadata_fields]\nrelease_date = \"date\"\n",
			reasonPointer},
		{"空table", "[platforms.version_source.metadata_fields]\n", reasonConditional},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := withSource(t, specVersionSourceBlock+"\n"+test.table)
			if err == nil {
				t.Fatal("Parse = nil, want error")
			}
			assertReason(t, err, test.wantReason)
		})
	}
}

// TestStaticKindRejectsTableKeys はstaticのtable形式禁止keyを固定する。
//
// scalar keyは[TestVersionSourceRejectsForbiddenKeysPerKind]が扱う。table形式は
// 行の差し替えでは表現できないため別に置く。
func TestStaticKindRejectsTableKeys(t *testing.T) {
	tables := map[string]string{
		"lifecycle_map":   "[platforms.version_source.lifecycle_map]\nactive = \"supported\"\n",
		"asset_fields":    "[platforms.version_source.asset_fields]\nname = \"/filename\"\n",
		"metadata_fields": "[platforms.version_source.metadata_fields]\ntag = \"/tag\"\n",
		"lifecycle_overrides": "[[platforms.version_source.lifecycle_overrides]]\n" +
			"version = \"3.13.7\"\nstatus = \"eol\"\n" +
			"evidence = \"https://example.invalid/lifecycle\"\n" +
			"assessed_at = 2026-08-07T00:00:00Z\n",
	}
	for key, table := range tables {
		t.Run(key, func(t *testing.T) {
			// staticのtop-level tableとassetsの間へ挿入する。
			marker := "[[platforms.version_source.static_versions]]"
			index := strings.Index(staticSourceBlock, marker)
			block := staticSourceBlock[:index] + table + "\n" + staticSourceBlock[index:]
			_, err := parseSpec(t,
				`version_scheme = "semver"`, `version_scheme = "python"`,
				specVersionSourceBlock, block)
			if err == nil {
				t.Fatalf("kind=staticで `%s` が通った", key)
			}
			assertReason(t, err, reasonKindKey)
		})
	}
}
