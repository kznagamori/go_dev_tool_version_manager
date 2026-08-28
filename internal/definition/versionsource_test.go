package definition

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
)

// goSourceBlock はdocs/06-tool-definition.md §16.2のGoのversion sourceである。
//
// **`url`にquery stringを含む。** endpoint URLでqueryを拒否できないことの根拠に
// なる正規例である。
const goSourceBlock = `[platforms.version_source]
kind = "json"
url = "https://go.dev/dl/?mode=json&include=all"
items_pointer = ""
version_pointer = "/version"
version_regex = "^go(?P<version>[0-9]+[.][0-9]+(?:[.][0-9]+)?(?:(?:beta|rc)[1-9][0-9]*)?)$"
channel_pointer = "/stable"
assets_pointer = "/files"
max_items = 10000
cache_ttl = "24h"

[platforms.version_source.asset_fields]
name = "/filename"
size = "/size"
digest = "/sha256"
os = "/os"
arch = "/arch"
`

// indexSourceBlock はdocs/06-tool-definition.md §16.4の.NET SDKのversion sourceである。
const indexSourceBlock = `[platforms.version_source]
kind = "json-index"
url = "https://builds.dotnet.microsoft.com/dotnet/release-metadata/releases-index.json"
index_items_pointer = "/releases-index"
index_document_pointer = "/releases.json"
max_documents = 32
document_lifecycle_pointer = "/support-phase"
items_pointer = "/releases"
item_flatten_pointer = "/sdks"
item_parent_published_at_pointer = "/release-date"
version_pointer = "/version"
version_regex = "^(?P<version>[0-9]+[.][0-9]+[.][0-9]+(?:-[0-9A-Za-z.-]+)?)$"
assets_pointer = "/files"
max_items = 10000
cache_ttl = "24h"

[platforms.version_source.lifecycle_map]
active = "supported"
maintenance = "supported"
eol = "eol"
`

// staticSourceBlock はdocs/06-tool-definition.md §16.3のPythonのversion sourceである。
const staticSourceBlock = `[platforms.version_source]
kind = "static"
max_items = 10000

[[platforms.version_source.static_versions]]
version = "3.13.7"
channel = "stable"
lifecycle = "supported"
lifecycle_evidence = "https://devguide.python.org/versions/"
lifecycle_assessed_at = 2026-08-07T00:00:00Z
published_at = "2025-08-14T00:00:00Z"

[[platforms.version_source.static_versions.assets]]
name = "cpython-3.13.7-x86_64-pc-windows-msvc-install_only_stripped.tar.gz"
url = "https://github.com/astral-sh/python-build-standalone/releases/download/20250814/cpython.tar.gz"
size = 1
digest = "0000000000000000000000000000000000000000000000000000000000000000"
digest_algorithm = "sha256"
os = "windows"
arch = "amd64"
libc = "none"
release_tag = "20250814"
release_url = "https://github.com/astral-sh/python-build-standalone/releases/tag/20250814"
release_id = "0"
asset_id = "0"
published_at = "2025-08-14T00:00:00Z"
`

// withSource は正規例のversion source blockを差し替える。
func withSource(t *testing.T, block string) (*Definition, *domain.Error) {
	t.Helper()
	return parseSpec(t, specVersionSourceBlock, block)
}

// TestVersionSourceAcceptsSpecExamples は§16の4 toolのsourceが通ることを固定する。
func TestVersionSourceAcceptsSpecExamples(t *testing.T) {
	t.Run("json（Node.js）", func(t *testing.T) {
		value, err := parseSpec(t)
		if err != nil {
			t.Fatalf("Parse = %s", describe(err))
		}
		source := value.Platforms[0].VersionSource
		if source.Kind != SourceJSON || source.PublishedAtPointer != DeclaredPointer("/date") {
			t.Errorf("source = %+v", source)
		}
	})

	t.Run("json（Go、query付きURL）", func(t *testing.T) {
		value, err := withSource(t, goSourceBlock)
		if err != nil {
			t.Fatalf("Parse = %s", describe(err))
		}
		source := value.Platforms[0].VersionSource
		if source.URL != "https://go.dev/dl/?mode=json&include=all" {
			t.Errorf("url = %q", source.URL)
		}
		if source.ChannelPointer != DeclaredPointer("/stable") {
			t.Errorf("channel_pointer = %+v", source.ChannelPointer)
		}
		want := map[AssetField]string{
			AssetName: "/filename", AssetSize: "/size", AssetDigest: "/sha256",
			AssetOS: "/os", AssetArch: "/arch",
		}
		if len(source.AssetFields) != len(want) {
			t.Fatalf("asset_fields = %v", source.AssetFields)
		}
		for field, pointer := range want {
			if source.AssetFields[field] != pointer {
				t.Errorf("asset_fields[%s] = %q, want %q", field, source.AssetFields[field], pointer)
			}
		}
	})

	t.Run("json-index（.NET SDK）", func(t *testing.T) {
		value, err := withSource(t, indexSourceBlock)
		if err != nil {
			t.Fatalf("Parse = %s", describe(err))
		}
		source := value.Platforms[0].VersionSource
		if source.Kind != SourceJSONIndex || source.MaxDocuments != 32 {
			t.Errorf("source = %q/%d", source.Kind, source.MaxDocuments)
		}
		if source.ItemFlattenPointer != DeclaredPointer("/sdks") ||
			source.ItemParentPublishedAtPointer != DeclaredPointer("/release-date") {
			t.Errorf("flatten = %+v/%+v",
				source.ItemFlattenPointer, source.ItemParentPublishedAtPointer)
		}
		want := map[string]Lifecycle{
			"active": LifecycleSupported, "maintenance": LifecycleSupported, "eol": LifecycleEOL,
		}
		if len(source.LifecycleMap) != len(want) {
			t.Fatalf("lifecycle_map = %v", source.LifecycleMap)
		}
		for key, lifecycle := range want {
			if source.LifecycleMap[key] != lifecycle {
				t.Errorf("lifecycle_map[%q] = %q, want %q", key, source.LifecycleMap[key], lifecycle)
			}
		}
	})

	t.Run("static（Python）", func(t *testing.T) {
		value, err := parseSpec(t,
			`version_scheme = "semver"`, `version_scheme = "python"`,
			specVersionSourceBlock, staticSourceBlock)
		if err != nil {
			t.Fatalf("Parse = %s", describe(err))
		}
		source := value.Platforms[0].VersionSource
		if source.Kind != SourceStatic || len(source.StaticVersions) != 1 {
			t.Fatalf("source = %q/%d件", source.Kind, len(source.StaticVersions))
		}
		static := source.StaticVersions[0]
		if static.Version.String() != "3.13.7" || static.Channel != ChannelStable ||
			static.Lifecycle != LifecycleSupported {
			t.Errorf("static = %+v", static)
		}
		if len(static.Assets) != 1 {
			t.Fatalf("assets = %d件", len(static.Assets))
		}
		asset := static.Assets[0]
		if asset.DigestAlgorithm != AlgorithmSHA256 || len(asset.Digest) != 64 {
			t.Errorf("digest = %q/%q", asset.DigestAlgorithm, asset.Digest)
		}
		if asset.OS != domain.OSWindows || asset.Arch != domain.ArchAMD64 ||
			asset.Libc != domain.LibcNone {
			t.Errorf("asset platform = %q/%q/%q", asset.OS, asset.Arch, asset.Libc)
		}
		// full-dateとRFC 3339のどちらもUTCへ正規化する。
		if !static.PublishedAt.Equal(time.Date(2025, 8, 14, 0, 0, 0, 0, time.UTC)) {
			t.Errorf("published_at = %v", static.PublishedAt)
		}
		// staticはnetworkを使わないためcache_ttlを持たない。
		if source.CacheTTL != 0 || source.URL != "" {
			t.Errorf("staticがnetwork fieldを持っている: %v/%q", source.CacheTTL, source.URL)
		}
	})
}

// TestEndpointURLAllowsQueryButReferenceDoesNot は§16.2のGo URLを根拠に、
// URLのkindごとのquery可否を固定する。
//
// part 1（§4・§5.1）では参照URLのqueryを一律拒否した。§6のendpoint URLは
// `https://go.dev/dl/?mode=json&include=all`が正規例であり、同じ規則を適用
// できない。両者が別の規則であることをここで固定する。
func TestEndpointURLAllowsQueryButReferenceDoesNot(t *testing.T) {
	const goURL = "https://go.dev/dl/?mode=json&include=all"
	if err := checkHTTPSURL(goURL, "url", urlEndpoint); err != nil {
		t.Errorf("endpoint URLがqueryで落ちた: %v", err)
	}
	if err := checkHTTPSURL(goURL, "homepage", urlReference); err == nil {
		t.Error("参照URLがqueryを受理した")
	}
	// userinfoとfragmentはkindによらず拒否する。
	for _, kind := range []urlKind{urlReference, urlEndpoint} {
		if err := checkHTTPSURL("https://u:p@go.dev/dl", "url", kind); err == nil {
			t.Errorf("kind %d がuserinfoを受理した", kind)
		}
		if err := checkHTTPSURL("https://go.dev/dl#x", "url", kind); err == nil {
			t.Errorf("kind %d がfragmentを受理した", kind)
		}
	}
}

// TestVersionSourceRequiresKeysPerKind はkindごとの必須keyを固定する。
//
// §6.1は許可keyだけを列挙し必須keyを明示しないため、契約から導ける必須keyを
// ここで固定する。`cache_ttl`はdocs/04-storage-and-data.md §15が「static
// sourceは`expires_at=null`を許す」と定めることから、network sourceでは必須と
// 読む。
func TestVersionSourceRequiresKeysPerKind(t *testing.T) {
	tests := []struct {
		name  string
		block string
		keys  []string
	}{
		{"json", specVersionSourceBlock, []string{
			"kind", "url", "items_pointer", "version_pointer", "version_regex",
			"max_items", "cache_ttl",
		}},
		{"json-index", indexSourceBlock, []string{
			"kind", "url", "index_items_pointer", "index_document_pointer", "max_documents",
			"items_pointer", "version_pointer", "version_regex", "max_items", "cache_ttl",
		}},
		{"static", staticSourceBlock, []string{"kind", "max_items", "static_versions"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, key := range test.keys {
				t.Run(key, func(t *testing.T) {
					reduced := removeSourceKey(t, test.block, key)
					_, err := parseSpec(t,
						`version_scheme = "semver"`, `version_scheme = "python"`,
						specVersionSourceBlock, reduced)
					if err == nil {
						t.Errorf("kind=%s で `%s` が無くても通った", test.name, key)
					}
				})
			}
		})
	}
}

// sourceTableHead はversion source blockのtop-level table部分だけを返す。
//
// `[platforms.version_source]`の次のtable headerより前が対象である。
func sourceTableHead(block string) string {
	lines := strings.Split(block, "\n")
	for index, line := range lines {
		trimmed := strings.TrimSpace(line)
		if index > 0 && strings.HasPrefix(trimmed, "[") {
			return strings.Join(lines[:index], "\n")
		}
	}
	return block
}

// removeSourceKey はversion source blockから1 keyの行（またはtable）を落とす。
func removeSourceKey(t *testing.T, block, key string) string {
	t.Helper()
	if key == "static_versions" {
		index := strings.Index(block, "[[platforms.version_source.static_versions]]")
		if index < 0 {
			t.Fatalf("`static_versions`がblockに無い")
		}
		return block[:index]
	}
	lines := strings.Split(block, "\n")
	for index, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), key+" = ") {
			return strings.Join(append(append([]string{}, lines[:index]...), lines[index+1:]...), "\n")
		}
	}
	t.Fatalf("key %q がblockに無い", key)
	return ""
}

// TestVersionSourceRejectsForbiddenKeysPerKind は§6.1の表を1組ずつ固定する。
//
// kindごとの禁止keyは表で持つ。条件分岐で書くとkindが増減したときにどのkeyが
// 漏れたかを読み取れない。表の各行がテストされていることをここで担保する。
func TestVersionSourceRejectsForbiddenKeysPerKind(t *testing.T) {
	// 値はgrammarとして正しいものにする。禁止判定がgrammar違反で置き換わらない
	// ようにするためである。
	sample := map[string]string{
		"url":                              `url = "https://example.invalid/v.json"`,
		"index_items_pointer":              `index_items_pointer = "/index"`,
		"index_document_pointer":           `index_document_pointer = "/document-url"`,
		"max_documents":                    `max_documents = 16`,
		"document_lifecycle_pointer":       `document_lifecycle_pointer = "/support-phase"`,
		"lifecycle_pointer":                `lifecycle_pointer = "/phase"`,
		"items_pointer":                    `items_pointer = ""`,
		"item_flatten_pointer":             `item_flatten_pointer = "/sdks"`,
		"item_parent_published_at_pointer": `item_parent_published_at_pointer = "/date"`,
		"version_pointer":                  `version_pointer = "/version"`,
		"version_regex":                    `version_regex = "^(?P<version>.+)$"`,
		"channel_pointer":                  `channel_pointer = "/channel"`,
		"published_at_pointer":             `published_at_pointer = "/date"`,
		"assets_pointer":                   `assets_pointer = "/files"`,
		"required_tokens_pointer":          `required_tokens_pointer = "/files"`,
		"required_tokens":                  `required_tokens = ["a"]`,
		"cache_ttl":                        `cache_ttl = "24h"`,
	}
	for _, kind := range []VersionSourceKind{SourceJSON, SourceJSONIndex, SourceStatic} {
		base := map[VersionSourceKind]string{
			SourceJSON: specVersionSourceBlock, SourceJSONIndex: indexSourceBlock,
			SourceStatic: staticSourceBlock,
		}[kind]
		forbidden := forbiddenSourceKeys(kind)
		checked := 0
		for _, key := range sourceKeyOrder {
			if _, banned := forbidden[key]; !banned {
				continue
			}
			line, ok := sample[key]
			if !ok {
				// table形式のkeyは別testで扱う。
				continue
			}
			// 判定はversion source tableのtop-level部分だけで行う。
			// static assetのようなsub tableにも`url`や`size`があり、含めると
			// 別tableのkeyを禁止keyの宣言と誤認する。
			if strings.Contains(sourceTableHead(base), key+" = ") {
				t.Fatalf("kind=%s のbaseに禁止key `%s` が含まれている", kind, key)
			}
			checked++
			t.Run(fmt.Sprintf("%s/%s", kind, key), func(t *testing.T) {
				block := strings.Replace(base, "kind = \""+string(kind)+"\"",
					"kind = \""+string(kind)+"\"\n"+line, 1)
				_, err := parseSpec(t,
					`version_scheme = "semver"`, `version_scheme = "python"`,
					specVersionSourceBlock, block)
				if err == nil {
					t.Fatalf("kind=%s で禁止key `%s` が通った", kind, key)
				}
				assertReason(t, err, reasonKindKey)
			})
		}
		if checked == 0 && kind != SourceJSONIndex {
			t.Errorf("kind=%s の禁止keyが1件も検査されていない", kind)
		}
	}
}

// TestVersionSourceRejectsStaticVersionsOnNetworkKind は`static_versions`の
// 禁止をtable形式で固定する。
func TestVersionSourceRejectsStaticVersionsOnNetworkKind(t *testing.T) {
	staticEntry := staticSourceBlock[strings.Index(
		staticSourceBlock, "[[platforms.version_source.static_versions]]"):]
	for _, base := range []string{specVersionSourceBlock, indexSourceBlock} {
		_, err := parseSpec(t, specVersionSourceBlock, base+"\n"+staticEntry)
		if err == nil {
			t.Fatal("network kindで`static_versions`が通った")
		}
		assertReason(t, err, reasonKindKey)
	}
}

// TestJSONKindRejectsLifecyclePointer は利用者判断で確定した契約を固定する。
//
// §6.1の表は`json`へ`lifecycle_map`を禁じるが、§6.3は`lifecycle_pointer`が
// 読んだ値を`lifecycle_map`で写像すると定め、mapに無い値をsource errorにする。
// `json`が`lifecycle_pointer`だけを許すと、その組合せは必ずsource errorになる
// 定義がschema検証を通ってしまう。**利用者判断により`json`は両方を禁止する。**
func TestJSONKindRejectsLifecyclePointer(t *testing.T) {
	for _, line := range []string{
		`lifecycle_pointer = "/phase"`,
		"[platforms.version_source.lifecycle_map]\nactive = \"supported\"",
	} {
		_, err := parseSpec(t, specVersionSourceBlock, specVersionSourceBlock+"\n"+line+"\n")
		if err == nil {
			t.Fatalf("kind=jsonで %q が通った", line)
		}
		assertReason(t, err, reasonKindKey)
	}
}

// TestLifecycleMapPairing は§6.1の「pointerとmapは組」を固定する。
func TestLifecycleMapPairing(t *testing.T) {
	mapBlock := "\n[platforms.version_source.lifecycle_map]\nactive = \"supported\"\neol = \"eol\"\n"

	t.Run("pointerだけ", func(t *testing.T) {
		// indexSourceBlockからlifecycle_map tableを落とす。
		block := indexSourceBlock[:strings.Index(indexSourceBlock, "[platforms.version_source.lifecycle_map]")]
		_, err := withSource(t, block)
		if err == nil {
			t.Fatal("`lifecycle_map`が無いpointerが通った")
		}
		assertReason(t, err, reasonConditional)
	})

	t.Run("mapだけ", func(t *testing.T) {
		block := strings.Replace(indexSourceBlock, `document_lifecycle_pointer = "/support-phase"`+"\n", "", 1)
		_, err := withSource(t, block)
		if err == nil {
			t.Fatal("pointerが無い`lifecycle_map`が通った")
		}
		assertReason(t, err, reasonConditional)
	})

	t.Run("両方のpointerを同時指定", func(t *testing.T) {
		block := strings.Replace(indexSourceBlock,
			`document_lifecycle_pointer = "/support-phase"`,
			`document_lifecycle_pointer = "/support-phase"`+"\n"+`lifecycle_pointer = "/phase"`, 1)
		_, err := withSource(t, block)
		if err == nil {
			t.Fatal("document/item両方のlifecycle pointerが通った")
		}
		assertReason(t, err, reasonConditional)
	})

	t.Run("item側pointerとmapの組は通る", func(t *testing.T) {
		block := strings.Replace(indexSourceBlock,
			`document_lifecycle_pointer = "/support-phase"`, `lifecycle_pointer = "/phase"`, 1)
		if _, err := withSource(t, block); err != nil {
			t.Errorf("Parse = %s", describe(err))
		}
	})

	t.Run("mapの値がenum外", func(t *testing.T) {
		block := strings.Replace(indexSourceBlock, `eol = "eol"`, `eol = "retired"`, 1)
		_, err := withSource(t, block)
		if err == nil {
			t.Fatal("lifecycle enum外が通った")
		}
		assertReason(t, err, reasonEnum)
	})

	t.Run("mapが空table", func(t *testing.T) {
		block := indexSourceBlock[:strings.Index(
			indexSourceBlock, "[platforms.version_source.lifecycle_map]")] +
			"[platforms.version_source.lifecycle_map]\n"
		_, err := withSource(t, block)
		if err == nil {
			t.Fatal("空の`lifecycle_map`が通った")
		}
		assertReason(t, err, reasonConditional)
	})
	_ = mapBlock
}

// TestFlattenContract は§6.1の`item_flatten_pointer`の組契約を固定する。
func TestFlattenContract(t *testing.T) {
	t.Run("親pointerだけでflattenが無い", func(t *testing.T) {
		block := strings.Replace(indexSourceBlock, `item_flatten_pointer = "/sdks"`+"\n", "", 1)
		_, err := withSource(t, block)
		if err == nil {
			t.Fatal("`item_flatten_pointer`が無い親pointerが通った")
		}
		assertReason(t, err, reasonConditional)
	})

	t.Run("親pointerと子published_at_pointerの同時指定", func(t *testing.T) {
		block := strings.Replace(indexSourceBlock,
			`item_parent_published_at_pointer = "/release-date"`,
			`item_parent_published_at_pointer = "/release-date"`+"\n"+`published_at_pointer = "/date"`, 1)
		_, err := withSource(t, block)
		if err == nil {
			t.Fatal("親と子の公開日時pointerの同時指定が通った")
		}
		assertReason(t, err, reasonConditional)
	})

	t.Run("flattenだけなら通る", func(t *testing.T) {
		block := strings.Replace(indexSourceBlock,
			`item_parent_published_at_pointer = "/release-date"`+"\n", "", 1)
		if _, err := withSource(t, block); err != nil {
			t.Errorf("Parse = %s", describe(err))
		}
	})
}

// TestRequiredTokensPairing は§6.2の`required_tokens*`の組契約を固定する。
func TestRequiredTokensPairing(t *testing.T) {
	tests := []struct {
		name       string
		block      string
		wantReason string
	}{
		{"pointerだけ", strings.Replace(specVersionSourceBlock,
			`required_tokens = ["win-x64-zip"]`+"\n", "", 1), reasonConditional},
		{"tokensだけ", strings.Replace(specVersionSourceBlock,
			`required_tokens_pointer = "/files"`+"\n", "", 1), reasonConditional},
		{"空配列", strings.Replace(specVersionSourceBlock,
			`required_tokens = ["win-x64-zip"]`, `required_tokens = []`, 1), reasonConditional},
		{"重複", strings.Replace(specVersionSourceBlock,
			`required_tokens = ["win-x64-zip"]`, `required_tokens = ["a", "a"]`, 1), reasonDuplicate},
		{"空文字", strings.Replace(specVersionSourceBlock,
			`required_tokens = ["win-x64-zip"]`, `required_tokens = [""]`, 1), reasonText},
		{"ASCII以外", strings.Replace(specVersionSourceBlock,
			`required_tokens = ["win-x64-zip"]`, `required_tokens = ["ウィン"]`, 1), reasonText},
		{"空白を含む", strings.Replace(specVersionSourceBlock,
			`required_tokens = ["win-x64-zip"]`, `required_tokens = ["a b"]`, 1), reasonText},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := withSource(t, test.block)
			if err == nil {
				t.Fatal("Parse = nil, want error")
			}
			assertReason(t, err, test.wantReason)
		})
	}

	// 両方欠落は正当である（Go/.NET SDKの正規例が該当）。
	block := strings.Replace(specVersionSourceBlock, `required_tokens_pointer = "/files"`+"\n", "", 1)
	block = strings.Replace(block, `required_tokens = ["win-x64-zip"]`+"\n", "", 1)
	if _, err := withSource(t, block); err != nil {
		t.Errorf("required_tokens両方欠落が落ちた: %s", describe(err))
	}
}

// TestVersionRegexRequiresSingleNamedCapture は§6.3の`version_regex`を固定する。
func TestVersionRegexRequiresSingleNamedCapture(t *testing.T) {
	tests := []struct{ name, pattern string }{
		{"named captureが無い", `^v(.+)$`},
		{"名前が違う", `^v(?P<ver>.+)$`},
		{"2件ある", `^(?P<version>[0-9]+)[.](?P<version>[0-9]+)$`},
		{"RE2として不正", `^v(?P<version>[a-z$`},
		{"空", ``},
		{"上限超過", "^(?P<version>" + strings.Repeat("a", RegexMaxBytes) + ")$"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			block := strings.Replace(specVersionSourceBlock,
				`version_regex = "^v(?P<version>[0-9]+[.][0-9]+[.][0-9]+(?:-[0-9A-Za-z.-]+)?)$"`,
				fmt.Sprintf("version_regex = %q", test.pattern), 1)
			_, err := withSource(t, block)
			if err == nil {
				t.Fatalf("version_regex %q が通った", test.pattern)
			}
			assertReason(t, err, reasonRegex)
		})
	}
}

// TestCacheTTLRange は`cache_ttl`の解釈と範囲を固定する。
func TestCacheTTLRange(t *testing.T) {
	valid := []string{"1m", "24h", "720h"}
	for _, text := range valid {
		block := strings.Replace(specVersionSourceBlock, `cache_ttl = "24h"`,
			fmt.Sprintf("cache_ttl = %q", text), 1)
		if _, err := withSource(t, block); err != nil {
			t.Errorf("cache_ttl %q が落ちた: %s", text, describe(err))
		}
	}
	invalid := []string{"0s", "-1h", "59s", "721h", "24", "1 h", "24hours"}
	for _, text := range invalid {
		t.Run(text, func(t *testing.T) {
			block := strings.Replace(specVersionSourceBlock, `cache_ttl = "24h"`,
				fmt.Sprintf("cache_ttl = %q", text), 1)
			_, err := withSource(t, block)
			if err == nil {
				t.Fatalf("cache_ttl %q が通った", text)
			}
			assertReason(t, err, reasonDuration)
		})
	}
}

// TestBoundedIntegers は`max_items`と`max_documents`の範囲を固定する。
func TestBoundedIntegers(t *testing.T) {
	t.Run("max_items", func(t *testing.T) {
		for _, value := range []int{0, -1, MaxItemsLimit + 1} {
			block := strings.Replace(specVersionSourceBlock, "max_items = 10000",
				fmt.Sprintf("max_items = %d", value), 1)
			_, err := withSource(t, block)
			if err == nil {
				t.Fatalf("max_items %d が通った", value)
			}
			assertReason(t, err, reasonLimit)
		}
		// 上限を縮小する値は通る（§6.1）。
		block := strings.Replace(specVersionSourceBlock, "max_items = 10000", "max_items = 1", 1)
		if _, err := withSource(t, block); err != nil {
			t.Errorf("max_items = 1 が落ちた: %s", describe(err))
		}
	})
	t.Run("max_documents", func(t *testing.T) {
		for _, value := range []int{0, -1, MaxDocumentsLimit + 1} {
			block := strings.Replace(indexSourceBlock, "max_documents = 32",
				fmt.Sprintf("max_documents = %d", value), 1)
			_, err := withSource(t, block)
			if err == nil {
				t.Fatalf("max_documents %d が通った", value)
			}
			assertReason(t, err, reasonLimit)
		}
	})
}

// TestOptionalPointerDistinguishesOmissionFromEmpty は任意pointerの「未宣言」と
// 「空文字宣言」が区別できることを固定する（P3-03の1本目で導入）。
//
// 空文字はRFC 6901で文書全体を指す正当なpointerであり、未宣言を空文字で表せない。
// §6.1は`channel_pointer`を**省略した場合**に正規versionのprerelease構文から
// channelを導出すると定めるため、両者を混同すると`channel_pointer = ""`の
// sourceが黙って構文導出へ落ちる。
func TestOptionalPointerDistinguishesOmissionFromEmpty(t *testing.T) {
	// §16.2のGoは`channel_pointer`を宣言し、`published_at_pointer`を宣言しない。
	value, err := withSource(t, goSourceBlock)
	if err != nil {
		t.Fatalf("Parse = %s", describe(err))
	}
	source := value.Platforms[0].VersionSource
	if !source.ChannelPointer.Declared() {
		t.Error("宣言済みの`channel_pointer`がDeclared()=false")
	}
	if source.PublishedAtPointer.Declared() {
		t.Error("未宣言の`published_at_pointer`がDeclared()=true")
	}
	if source.PublishedAtPointer.Value() != "" {
		t.Errorf("未宣言のValue() = %q, want \"\"", source.PublishedAtPointer.Value())
	}

	// 空文字を宣言した場合は「宣言済みで値が空」になる。
	empty := strings.Replace(goSourceBlock, `channel_pointer = "/stable"`, `channel_pointer = ""`, 1)
	value, err = withSource(t, empty)
	if err != nil {
		t.Fatalf("Parse = %s", describe(err))
	}
	source = value.Platforms[0].VersionSource
	if !source.ChannelPointer.Declared() || source.ChannelPointer.Value() != "" {
		t.Fatalf("空文字宣言 = %+v, want {declared, \"\"}", source.ChannelPointer)
	}
}

// TestSpecifiedLimitsMatchSpec はdocs/04-storage-and-data.md §21へ昇格した上限が
// Go側の定数と一致することを固定する。
//
// この5件はP3-01が仕様に無いまま導入し、P6-01の利用者判断で§21の表へ昇格した。
// 定数だけを変えると仕様と実装がずれるため、期待値を§21の値そのままで持つ。
// **この検査を変えるときは§21の表も同じ変更で直す。**
func TestSpecifiedLimitsMatchSpec(t *testing.T) {
	tests := []struct {
		name string
		got  int
		want int
	}{
		{"JSON pointer 255 byte", PointerMaxBytes, 255},
		{"version_regex 1024 byte", RegexMaxBytes, 1024},
		{"SPDX expression 128 byte", LicenseMaxBytes, 128},
		{"URL hostname 253 byte", hostMaxBytes, 253},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.got != test.want {
				t.Errorf("%d, want %d（§21の表と食い違う）", test.got, test.want)
			}
		})
	}
	if CacheTTLMin != time.Minute {
		t.Errorf("CacheTTLMin = %v, want 1分（§21の表と食い違う）", CacheTTLMin)
	}
	if CacheTTLMax != 30*24*time.Hour {
		t.Errorf("CacheTTLMax = %v, want 30日（§21の表と食い違う）", CacheTTLMax)
	}
}
