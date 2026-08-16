package catalog

import (
	"fmt"
	"strings"
	"testing"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/definition"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
)

// nodeStyleSource はdocs/06-tool-definition.md §15のNode.js sourceである。
//
// top-levelが配列の文書を`items_pointer = ""`で指す。`channel_pointer`を宣言せず、
// channelは正規versionのprerelease構文から導出する。
func nodeStyleSource() definition.VersionSource {
	return definition.VersionSource{
		Kind:               definition.SourceJSON,
		URL:                "https://nodejs.org/dist/index.json",
		ItemsPointer:       "",
		VersionPointer:     "/version",
		VersionRegex:       `^v(?P<version>[0-9]+[.][0-9]+[.][0-9]+(?:-[0-9A-Za-z.-]+)?)$`,
		PublishedAtPointer: definition.DeclaredPointer("/date"),
		MaxItems:           10000,
	}
}

// goStyleSource はdocs/06-tool-definition.md §16.2のGo sourceである。
//
// `channel_pointer`がbooleanを指す。
func goStyleSource() definition.VersionSource {
	return definition.VersionSource{
		Kind:           definition.SourceJSON,
		URL:            "https://go.dev/dl/?mode=json&include=all",
		ItemsPointer:   "",
		VersionPointer: "/version",
		VersionRegex: `^go(?P<version>[0-9]+[.][0-9]+(?:[.][0-9]+)?` +
			`(?:(?:beta|rc)[1-9][0-9]*)?)$`,
		ChannelPointer: definition.DeclaredPointer("/stable"),
		MaxItems:       10000,
	}
}

// flattenSource はdocs/06-tool-definition.md §16.4の.NET SDK sourceである。
//
// 親releaseを`/sdks`で1段展開し、親の`release-date`を全子itemへ継承する。
func flattenSource() definition.VersionSource {
	return definition.VersionSource{
		Kind:                         definition.SourceJSONIndex,
		ItemsPointer:                 "/releases",
		ItemFlattenPointer:           definition.DeclaredPointer("/sdks"),
		ItemParentPublishedAtPointer: definition.DeclaredPointer("/release-date"),
		VersionPointer:               "/version",
		VersionRegex:                 `^(?P<version>[0-9]+[.][0-9]+[.][0-9]+(?:-[0-9A-Za-z.-]+)?)$`,
		MaxItems:                     10000,
	}
}

// TestBuildItemsReadsJSONSource は§6.1・§6.3の`json` source評価を固定する。
func TestBuildItemsReadsJSONSource(t *testing.T) {
	document := mustDecode(t, `[
		{"version":"v22.18.0","date":"2025-07-01"},
		{"version":"v23.0.0-rc.1","date":"2025-08-14"}
	]`)
	items := mustBuildItems(t, ItemsRequest{
		Source:   nodeStyleSource(),
		Scheme:   domain.SchemeSemver,
		Document: document,
		Origin:   "https://nodejs.org/dist/index.json",
	})
	if len(items) != 2 {
		t.Fatalf("items = %d件, want 2", len(items))
	}

	// `version_regex`のnamed captureが正規versionを取り出す。
	if items[0].Version.String() != "22.18.0" {
		t.Errorf("version = %q, want 22.18.0", items[0].Version.String())
	}
	// regex適用前のraw versionを残す。§6.5のprovider_releaseが使う。
	if items[0].RawVersion != "v22.18.0" {
		t.Errorf("raw = %q, want v22.18.0", items[0].RawVersion)
	}
	// full-dateは`T00:00:00Z`へ正規化する。
	if items[0].PublishedAt != "2025-07-01T00:00:00Z" {
		t.Errorf("published_at = %q", items[0].PublishedAt)
	}
	// `channel_pointer`省略時は正規versionの構文から導出する。
	if items[0].Channel != domain.ChannelStable {
		t.Errorf("channel = %q, want stable", items[0].Channel)
	}
	if items[1].Channel != domain.ChannelPrerelease {
		t.Errorf("channel = %q, want prerelease", items[1].Channel)
	}
	// item本体を残す。asset/tokenの抽出が後段で使う。
	if items[0].Node == nil {
		t.Error("Nodeがnil")
	}
}

// TestBuildItemsMapsChannelPointer は§6.1のchannel写像を固定する。
//
// Goの`https://go.dev/dl/`は`"stable": true`で正式版を示す。真偽の向きを逆に
// 取ると全versionのchannelが反転する。
func TestBuildItemsMapsChannelPointer(t *testing.T) {
	document := mustDecode(t, `[
		{"version":"go1.25.0","stable":true},
		{"version":"go1.26rc1","stable":false}
	]`)
	items := mustBuildItems(t, ItemsRequest{
		Source:   goStyleSource(),
		Scheme:   domain.SchemeGo,
		Document: document,
		Origin:   "https://go.dev/dl/",
	})
	if len(items) != 2 {
		t.Fatalf("items = %d件, want 2", len(items))
	}
	if items[0].Version.String() != "1.25.0" || items[0].Channel != domain.ChannelStable {
		t.Errorf("item[0] = %q/%q", items[0].Version.String(), items[0].Channel)
	}
	if items[1].Version.String() != "1.26rc1" || items[1].Channel != domain.ChannelPrerelease {
		t.Errorf("item[1] = %q/%q", items[1].Version.String(), items[1].Channel)
	}
}

// TestBuildItemsChannelPointerOverridesSyntax は宣言したpointerが構文導出より
// 優先することを固定する。
//
// §6.1は「pointer省略時だけversion schemeのprerelease構文から導出する」と定める。
// 構文がstableでもpointerがprereleaseと言えばprereleaseである。
func TestBuildItemsChannelPointerOverridesSyntax(t *testing.T) {
	document := mustDecode(t, `[{"version":"go1.25.0","stable":false}]`)
	items := mustBuildItems(t, ItemsRequest{
		Source:   goStyleSource(),
		Scheme:   domain.SchemeGo,
		Document: document,
		Origin:   "https://go.dev/dl/",
	})
	if items[0].Channel != domain.ChannelPrerelease {
		t.Fatalf("channel = %q, want prerelease（pointerが優先する）", items[0].Channel)
	}
	// 構文だけで見ればstableである。導出とpointerが別物であることの確認。
	if items[0].Version.IsPrerelease() {
		t.Fatal("1.25.0がprerelease構文と判定された")
	}
}

// TestBuildItemsFlattensOneLevel は§6.1の`item_flatten_pointer`を固定する。
func TestBuildItemsFlattensOneLevel(t *testing.T) {
	document := mustDecode(t, `{"releases":[
		{"release-date":"2026-01-14","sdks":[
			{"version":"9.0.101"},
			{"version":"9.0.102"}
		]},
		{"release-date":"2026-02-11","sdks":[
			{"version":"9.0.103"}
		]}
	]}`)
	items := mustBuildItems(t, ItemsRequest{
		Source:   flattenSource(),
		Scheme:   domain.SchemeSemver,
		Document: document,
		Origin:   "https://example.invalid/releases.json",
	})
	// 得られた配列を1段だけ連結する。親の並び順を保つ。
	want := []string{"9.0.101", "9.0.102", "9.0.103"}
	if len(items) != len(want) {
		t.Fatalf("items = %d件, want %d", len(items), len(want))
	}
	for index, version := range want {
		if items[index].Version.String() != version {
			t.Errorf("items[%d] = %q, want %q", index, items[index].Version.String(), version)
		}
	}
	// 親の公開日時をその親から展開した全itemへ継承する。
	stamps := []string{"2026-01-14T00:00:00Z", "2026-01-14T00:00:00Z", "2026-02-11T00:00:00Z"}
	for index, stamp := range stamps {
		if items[index].PublishedAt != stamp {
			t.Errorf("items[%d].PublishedAt = %q, want %q", index, items[index].PublishedAt, stamp)
		}
	}
}

// TestBuildItemsDoesNotRecurseFlatten は展開が1段までであることを固定する。
//
// §6.1は「展開は1段までで、入れ子の再帰展開を行わない」と定める。子itemが
// さらに配列を持っていても、その中身をversion itemにしない。
func TestBuildItemsDoesNotRecurseFlatten(t *testing.T) {
	document := mustDecode(t, `{"releases":[
		{"release-date":"2026-01-14","sdks":[
			{"version":"9.0.101","sdks":[{"version":"9.0.999"}]}
		]}
	]}`)
	items := mustBuildItems(t, ItemsRequest{
		Source:   flattenSource(),
		Scheme:   domain.SchemeSemver,
		Document: document,
		Origin:   "https://example.invalid/releases.json",
	})
	if len(items) != 1 {
		t.Fatalf("items = %d件, want 1（再帰展開しない）", len(items))
	}
	if items[0].Version.String() != "9.0.101" {
		t.Fatalf("version = %q, want 9.0.101", items[0].Version.String())
	}
}

// TestBuildItemsRejectsSourceLayoutViolations は§6.1・§6.3のsource errorを固定する。
//
// **1件でも壊れていれば全体を失敗させる。** 読めたitemだけでcatalogを作ると、
// 上流のlayout変更が「versionが減っただけ」に見えて気付けない。
func TestBuildItemsRejectsSourceLayoutViolations(t *testing.T) {
	cases := []struct {
		name     string
		source   definition.VersionSource
		scheme   domain.VersionScheme
		document string
		why      string
	}{
		{
			"regexに一致しない", nodeStyleSource(), domain.SchemeSemver,
			`[{"version":"v22.18.0","date":"2025-07-01"},{"version":"nightly","date":"2025-07-02"}]`,
			"§6.3のsource layout違反",
		},
		{
			"version_pointerの欠落", nodeStyleSource(), domain.SchemeSemver,
			`[{"date":"2025-07-01"}]`, "参照fieldの欠落",
		},
		{
			"version_pointerの型違い", nodeStyleSource(), domain.SchemeSemver,
			`[{"version":22,"date":"2025-07-01"}]`, "参照fieldの型違い",
		},
		{
			"items_pointerが配列でない", nodeStyleSource(), domain.SchemeSemver,
			`{"version":"v22.18.0"}`, "items_pointerの先が配列でない",
		},
		{
			"published_atの型違い", nodeStyleSource(), domain.SchemeSemver,
			`[{"version":"v22.18.0","date":20250701}]`, "参照fieldの型違い",
		},
		{
			"published_atが日時でない", nodeStyleSource(), domain.SchemeSemver,
			`[{"version":"v22.18.0","date":"July 2025"}]`, "UTC RFC 3339でもfull-dateでもない",
		},
		{
			"channel_pointerの欠落", goStyleSource(), domain.SchemeGo,
			`[{"version":"go1.25.0"}]`, "参照fieldの欠落",
		},
		{
			"channel値が数値", goStyleSource(), domain.SchemeGo,
			`[{"version":"go1.25.0","stable":1}]`, "stringでもbooleanでもない",
		},
		{
			"channel値が未知string", goStyleSource(), domain.SchemeGo,
			`[{"version":"go1.25.0","stable":"lts"}]`, "未知stringのfallbackをしない",
		},
		{
			"flatten先が配列でない", flattenSource(), domain.SchemeSemver,
			`{"releases":[{"release-date":"2026-01-14","sdks":{"version":"9.0.101"}}]}`,
			"flatten先が配列でない",
		},
		{
			"flatten先が無い", flattenSource(), domain.SchemeSemver,
			`{"releases":[{"release-date":"2026-01-14"}]}`, "flatten先が存在しない",
		},
		{
			"親公開日時の欠落", flattenSource(), domain.SchemeSemver,
			`{"releases":[{"sdks":[{"version":"9.0.101"}]}]}`, "参照fieldの欠落",
		},
		{
			"親公開日時が日時でない", flattenSource(), domain.SchemeSemver,
			`{"releases":[{"release-date":"soon","sdks":[{"version":"9.0.101"}]}]}`,
			"UTC RFC 3339でもfull-dateでもない",
		},
		{
			"schemeに合わないversion", nodeStyleSource(), domain.SchemeSemver,
			`[{"version":"v22.18","date":"2025-07-01"}]`, "semverはpatch必須",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := BuildItems(ItemsRequest{
				Source:   c.source,
				Scheme:   c.scheme,
				Document: mustDecode(t, c.document),
				Origin:   "https://example.invalid/doc.json",
			})
			if err == nil {
				t.Fatalf("BuildItemsが成功した（%s）", c.why)
			}
			if err.Code != domain.CodeDefinitionInvalid {
				t.Fatalf("code = %s, want %s", err.Code, domain.CodeDefinitionInvalid)
			}
		})
	}
}

// TestBuildItemsRejectsOversizeDocument は1文書だけで組込み上限を超える入力を
// per-item処理の前に止めることを固定する。
func TestBuildItemsRejectsOversizeDocument(t *testing.T) {
	var builder strings.Builder
	builder.WriteString("[")
	for index := 0; index <= definition.MaxItemsLimit; index++ {
		if index > 0 {
			builder.WriteString(",")
		}
		fmt.Fprintf(&builder, `{"version":"v1.0.%d","date":"2025-07-01"}`, index)
	}
	builder.WriteString("]")

	_, err := BuildItems(ItemsRequest{
		Source:   nodeStyleSource(),
		Scheme:   domain.SchemeSemver,
		Document: mustDecode(t, builder.String()),
		Origin:   "https://example.invalid/doc.json",
	})
	if err == nil {
		t.Fatal("組込み上限を超える文書が成功した")
	}
	if err.Code != domain.CodeDefinitionInvalid {
		t.Fatalf("code = %s", err.Code)
	}
}

// TestBuildItemsRequiresScheme はversion schemeなしで評価しないことを固定する。
func TestBuildItemsRequiresScheme(t *testing.T) {
	_, err := BuildItems(ItemsRequest{
		Source:   nodeStyleSource(),
		Document: mustDecode(t, `[]`),
		Origin:   "https://example.invalid/doc.json",
	})
	if err == nil {
		t.Fatal("schemeなしで成功した")
	}
	if err.Code != domain.CodeInternal {
		t.Fatalf("code = %s, want %s", err.Code, domain.CodeInternal)
	}
}

// TestCheckItemLimit は§6.1のitem数上限を固定する。
//
// `max_items`は組込み上限を**縮小する方向にだけ**働く。超過は切り捨てず
// errorにする。黙って打ち切ると、上限に達した以降のversionが存在しないことと
// 区別できなくなる。
func TestCheckItemLimit(t *testing.T) {
	cases := []struct {
		name     string
		count    int
		maxItems int
		wantErr  bool
	}{
		{"上限内", 10, 100, false},
		{"max_itemsちょうど", 100, 100, false},
		{"max_items超過", 101, 100, true},
		{"組込み上限ちょうど", definition.MaxItemsLimit, 0, false},
		{"組込み上限超過", definition.MaxItemsLimit + 1, 0, true},
		// max_itemsが組込み上限より大きくても拡大しない。
		{"max_itemsで拡大できない", definition.MaxItemsLimit + 1, definition.MaxItemsLimit * 2, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			source := nodeStyleSource()
			source.MaxItems = c.maxItems
			err := CheckItemLimit(c.count, source, "https://example.invalid/doc.json")
			if c.wantErr && err == nil {
				t.Fatal("上限超過が成功した")
			}
			if !c.wantErr && err != nil {
				t.Fatalf("CheckItemLimit = %s", describeErr(err))
			}
		})
	}
}

// TestBuildItemsAcceptsEmptyDocument は空の配列を正当な結果として扱うことを固定する。
// 上流が一時的に0件を返した場合と、layoutが壊れた場合を混同しない。
func TestBuildItemsAcceptsEmptyDocument(t *testing.T) {
	items := mustBuildItems(t, ItemsRequest{
		Source:   nodeStyleSource(),
		Scheme:   domain.SchemeSemver,
		Document: mustDecode(t, `[]`),
		Origin:   "https://example.invalid/doc.json",
	})
	if len(items) != 0 {
		t.Fatalf("items = %d件, want 0", len(items))
	}
}

// --- helper ---

func mustBuildItems(t *testing.T, req ItemsRequest) []VersionItem {
	t.Helper()
	items, err := BuildItems(req)
	if err != nil {
		t.Fatalf("BuildItems = %s", describeErr(err))
	}
	return items
}
