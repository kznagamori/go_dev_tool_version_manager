package catalog

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/definition"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain/port/fake"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/store"
)

const (
	goSourceURL   = "https://go.dev/dl/?mode=json&include=all"
	nodeSourceURL = "https://nodejs.org/dist/index.json"
	digest64      = "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	otherDigest64 = "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
)

var fetchedAt = time.Date(2026, 8, 7, 9, 0, 0, 0, time.UTC)

// goAssetSource はdocs/06-tool-definition.md §16.2のGo sourceにasset fieldを足す。
func goAssetSource() definition.VersionSource {
	source := goStyleSource()
	source.AssetsPointer = definition.DeclaredPointer("/files")
	source.AssetFields = map[definition.AssetField]string{
		definition.AssetName: "/filename", definition.AssetURL: "/url",
		definition.AssetSize: "/size", definition.AssetDigest: "/sha256",
		definition.AssetOS: "/os", definition.AssetArch: "/arch",
	}
	source.CacheTTL = 24 * time.Hour
	return source
}

// goArtifact はdocs/06-tool-definition.md §16.2のGo artifactである。
func goArtifact() definition.Artifact {
	return definition.Artifact{
		Source: definition.SourceAsset,
		Format: definition.FormatZip,
		Selector: &definition.ArtifactSelector{
			NameRegex: `^go(?P<version>[0-9][0-9A-Za-z.]*)[.]windows-amd64[.]zip$`,
			OS:        "windows", Arch: "amd64",
		},
		Checksum: definition.ArtifactChecksum{
			Kind: definition.ChecksumAssetField, Algorithm: definition.AlgorithmSHA256,
		},
		RedirectHosts: []string{"dl.google.com"},
	}
}

func goDocument(t *testing.T) any {
	t.Helper()
	return mustDecode(t, `[
	  {"version":"go1.25.0","stable":true,"files":[
	    {"filename":"go1.25.0.windows-amd64.zip","size":1,"sha256":"`+digest64+`",
	     "url":"https://go.dev/dl/go1.25.0.windows-amd64.zip","os":"windows","arch":"amd64"},
	    {"filename":"go1.25.0.windows-amd64.msi","size":2,"sha256":"`+otherDigest64+`",
	     "url":"https://go.dev/dl/go1.25.0.windows-amd64.msi","os":"windows","arch":"amd64"},
	    {"filename":"go1.25.0.linux-amd64.tar.gz","size":3,"sha256":"`+otherDigest64+`",
	     "url":"https://go.dev/dl/go1.25.0.linux-amd64.tar.gz","os":"linux","arch":"amd64"}
	  ]},
	  {"version":"go1.24.9","stable":true,"files":[
	    {"filename":"go1.24.9.windows-amd64.zip","size":4,"sha256":"`+digest64+`",
	     "url":"https://go.dev/dl/go1.24.9.windows-amd64.zip","os":"windows","arch":"amd64"}
	  ]}
	]`)
}

func goRequest(t *testing.T, document any) BuildRequest {
	t.Helper()
	source := goAssetSource()
	items := mustBuildItems(t, ItemsRequest{
		Source: source, Scheme: domain.SchemeGo, Document: document, Origin: goSourceURL,
	})
	return BuildRequest{
		Tool:             mustToolID(t, "go"),
		Platform:         mustPlatform(t, "windows-amd64"),
		Source:           source,
		Artifact:         goArtifact(),
		ArtifactKind:     definition.KindOfficial,
		DefinitionSHA256: strings.Repeat("a", 64),
		SourceIdentity:   goSourceURL,
		FetchedAt:        fetchedAt,
		Items:            items,
	}
}

// TestBuildCatalogSelectsExactlyOneAsset は§7.1のselectorを固定する。
//
// 「指定条件すべてに一致するassetをexactly 1件要求する。**source順で選ばない。**」
// 同じentryにinstallerやsource archiveが並ぶため、`name_regex`で目的のarchiveだけ
// へ絞る。先頭一致で選ぶと別の配布物をinstallしうる。
func TestBuildCatalogSelectsExactlyOneAsset(t *testing.T) {
	catalog, warnings, err := BuildCatalog(context.Background(), nil, goRequest(t, goDocument(t)))
	if err != nil {
		t.Fatalf("BuildCatalog = %s", describeErr(err))
	}
	if len(warnings) != 0 {
		t.Errorf("warnings = %d件", len(warnings))
	}
	if len(catalog.Items) != 2 {
		t.Fatalf("items = %d件, want 2", len(catalog.Items))
	}
	first := catalog.Items[0]
	if first.ArtifactFile != "go1.25.0.windows-amd64.zip" {
		t.Errorf("artifact_file = %q", first.ArtifactFile)
	}
	if first.ArtifactSize != 1 {
		t.Errorf("artifact_size = %d, want 1", first.ArtifactSize)
	}
	if first.ArtifactDigest.Upstream() != "sha256:"+digest64 {
		t.Errorf("artifact_digest = %q", first.ArtifactDigest.Upstream())
	}
	if first.ChecksumSource != store.ChecksumAssetField {
		t.Errorf("checksum_source = %q", first.ChecksumSource)
	}
	// §6.5のprovider releaseはregex適用前のraw versionである。
	if first.ProviderRelease != "go1.25.0" {
		t.Errorf("provider_release = %q, want go1.25.0", first.ProviderRelease)
	}
	if !first.Installable || first.UnavailableReason != "" {
		t.Errorf("installable = %v / reason = %q", first.Installable, first.UnavailableReason)
	}
}

// TestBuildCatalogOrdersItems は§15の「version comparison降順、同値なら
// version byte順」を固定する。
func TestBuildCatalogOrdersItems(t *testing.T) {
	catalog, _, err := BuildCatalog(context.Background(), nil, goRequest(t, goDocument(t)))
	if err != nil {
		t.Fatalf("BuildCatalog = %s", describeErr(err))
	}
	want := []string{"1.25.0", "1.24.9"}
	for index, version := range want {
		if catalog.Items[index].VersionText != version {
			t.Errorf("items[%d] = %q, want %q", index, catalog.Items[index].VersionText, version)
		}
	}
}

// TestSortCatalogItemsBreaksTiesByVersionBytes は同値時の並びを固定する。
//
// goの`1.20`と`1.20.0`はcomparison keyが同値で正規文字列が違う。並びが入力順に
// 依存すると、同じsourceから作ったcatalogのdiffが安定しない。
func TestSortCatalogItemsBreaksTiesByVersionBytes(t *testing.T) {
	long := store.CatalogItem{
		Version: mustParse(t, domain.SchemeGo, "1.20.0"), VersionText: "1.20.0",
	}
	short := store.CatalogItem{
		Version: mustParse(t, domain.SchemeGo, "1.20"), VersionText: "1.20",
	}
	items := []store.CatalogItem{long, short}
	if err := sortCatalogItems(items); err != nil {
		t.Fatal(err)
	}
	if items[0].VersionText != "1.20" || items[1].VersionText != "1.20.0" {
		t.Fatalf("並び = %q, %q（byte順）", items[0].VersionText, items[1].VersionText)
	}
}

// TestBuildCatalogMarksUnavailableWhenNoAssetMatches は§7.1の0件一致を固定する。
//
// 「0件はそのversionを`installable=false/artifact-not-found`」。source errorに
// せず、そのplatformで導入できないitemとしてcatalogへ残す。
func TestBuildCatalogMarksUnavailableWhenNoAssetMatches(t *testing.T) {
	document := mustDecode(t, `[
	  {"version":"go1.25.0","stable":true,"files":[
	    {"filename":"go1.25.0.linux-amd64.tar.gz","size":3,"sha256":"`+digest64+`",
	     "url":"https://go.dev/dl/go1.25.0.linux-amd64.tar.gz","os":"linux","arch":"amd64"}
	  ]}
	]`)
	catalog, _, err := BuildCatalog(context.Background(), nil, goRequest(t, document))
	if err != nil {
		t.Fatalf("BuildCatalog = %s", describeErr(err))
	}
	item := catalog.Items[0]
	if item.Installable {
		t.Fatal("0件一致がinstallableになった")
	}
	if item.UnavailableReason != messageArtifactNotFound {
		t.Errorf("unavailable_reason = %q", item.UnavailableReason)
	}
	if item.ArtifactFile != "" || item.ArtifactURL != "" || !item.ArtifactDigest.IsZero() {
		t.Errorf("unavailable itemがartifactを持っている: %+v", item)
	}
	// provider releaseはassetが無くても決まる（§6.5のraw version）。
	if item.ProviderRelease != "go1.25.0" {
		t.Errorf("provider_release = %q", item.ProviderRelease)
	}
	// unavailableでもcatalogとして正しく書き出せる。
	if _, encodeErr := store.EncodeCatalog(catalog); encodeErr != nil {
		t.Fatalf("EncodeCatalog = %s", describeErr(encodeErr))
	}
}

// TestBuildCatalogRejectsAmbiguousSelector は2件以上一致をsource errorにすることを
// 固定する（§7.1「2件以上はdefinition/source error」）。
func TestBuildCatalogRejectsAmbiguousSelector(t *testing.T) {
	document := mustDecode(t, `[
	  {"version":"go1.25.0","stable":true,"files":[
	    {"filename":"go1.25.0.windows-amd64.zip","size":1,"sha256":"`+digest64+`",
	     "url":"https://go.dev/dl/a.zip","os":"windows","arch":"amd64"},
	    {"filename":"go1.25.0.windows-amd64.zip","size":2,"sha256":"`+otherDigest64+`",
	     "url":"https://go.dev/dl/b.zip","os":"windows","arch":"amd64"}
	  ]}
	]`)
	_, _, err := BuildCatalog(context.Background(), nil, goRequest(t, document))
	if err == nil {
		t.Fatal("2件一致が成功した")
	}
	if err.Code != domain.CodeDefinitionInvalid {
		t.Fatalf("code = %s", err.Code)
	}
}

// TestBuildCatalogUsesRequiredTokens は§6.2のtoken判定を固定する。
//
// 「required tokenが1件でもないversion itemは**source errorではなく**現在
// platformで`installable=false/artifact-not-found`」。
func TestBuildCatalogUsesRequiredTokens(t *testing.T) {
	source := nodeStyleSource()
	source.CacheTTL = 24 * time.Hour
	source.RequiredTokensPointer = definition.DeclaredPointer("/files")
	source.RequiredTokens = []string{"win-x64-zip"}
	document := mustDecode(t, `[
	  {"version":"v22.18.0","date":"2025-07-01","files":["win-x64-zip","linux-x64"]},
	  {"version":"v22.17.0","date":"2025-06-01","files":["linux-x64"]}
	]`)
	items := mustBuildItems(t, ItemsRequest{
		Source: source, Scheme: domain.SchemeSemver, Document: document, Origin: nodeSourceURL,
	})
	request := BuildRequest{
		Tool: mustToolID(t, "node"), Platform: mustPlatform(t, "windows-amd64"),
		Source: source,
		Artifact: definition.Artifact{
			Source: definition.SourceTemplate,
			URL:    "https://nodejs.org/dist/v{{version}}/node-v{{version}}-win-x64.zip",
			File:   "node-v{{version}}-win-x64.zip",
			Format: definition.FormatZip,
			Checksum: definition.ArtifactChecksum{
				Kind:       definition.ChecksumTextFile,
				URL:        "https://nodejs.org/dist/v{{version}}/SHASUMS256.txt",
				LineFormat: definition.LineFormatSHA256,
			},
		},
		ArtifactKind: definition.KindOfficial, DefinitionSHA256: strings.Repeat("b", 64),
		SourceIdentity: nodeSourceURL, FetchedAt: fetchedAt, Items: items,
	}

	client := fake.NewHTTPClient(nil)
	client.Stub("https://nodejs.org/dist/v22.18.0/SHASUMS256.txt", fake.HTTPStub{
		StatusCode: 200,
		Body: []byte(digest64 + "  node-v22.18.0-win-x64.zip\n" +
			otherDigest64 + "  node-v22.18.0-linux-x64.tar.gz\n"),
	})

	catalog, _, err := BuildCatalog(context.Background(), client, request)
	if err != nil {
		t.Fatalf("BuildCatalog = %s", describeErr(err))
	}
	if len(catalog.Items) != 2 {
		t.Fatalf("items = %d件", len(catalog.Items))
	}
	available, missing := catalog.Items[0], catalog.Items[1]
	if !available.Installable {
		t.Errorf("22.18.0がinstallableでない: %q", available.UnavailableReason)
	}
	// template renderの結果を使う。
	if available.ArtifactFile != "node-v22.18.0-win-x64.zip" {
		t.Errorf("artifact_file = %q", available.ArtifactFile)
	}
	if available.ArtifactDigest.Upstream() != "sha256:"+digest64 {
		t.Errorf("artifact_digest = %q", available.ArtifactDigest.Upstream())
	}
	if available.ChecksumSource != store.ChecksumTextFile {
		t.Errorf("checksum_source = %q", available.ChecksumSource)
	}
	// tokenが無いversionはunavailableになる。checksumも取りに行かない。
	if missing.Installable || missing.UnavailableReason != messageArtifactNotFound {
		t.Errorf("22.17.0 = %v / %q", missing.Installable, missing.UnavailableReason)
	}
	if len(client.Requests) != 1 {
		t.Errorf("checksum取得が%d回（unavailable itemでは取得しない）", len(client.Requests))
	}
}

// TestBuildCatalogRecordsLifecycleEvidence は§15のlifecycle記録元を固定する。
//
// 「source fieldならsource URL/fetch時刻、override/staticならdefinition記録を
// 使う。上流がlifecycleを示さず既定の`unknown`になったitemもsource URL/fetch
// 時刻を使う。」（利用者判断で§15へ明記した規定）
func TestBuildCatalogRecordsLifecycleEvidence(t *testing.T) {
	t.Run("override", func(t *testing.T) {
		request := goRequest(t, goDocument(t))
		request.Source.LifecycleOverrides = []definition.LifecycleOverride{
			newOverride(t, domain.SchemeGo, "1.24.9", definition.LifecycleEOL),
		}
		request.Items = mustBuildItems(t, ItemsRequest{
			Source: request.Source, Scheme: domain.SchemeGo,
			Document: goDocument(t), Origin: goSourceURL,
		})
		catalog, _, err := BuildCatalog(context.Background(), nil, request)
		if err != nil {
			t.Fatalf("BuildCatalog = %s", describeErr(err))
		}
		eol := catalog.Items[1]
		if eol.Lifecycle != domain.LifecycleEOL {
			t.Fatalf("lifecycle = %q", eol.Lifecycle)
		}
		// definition記録を使う。
		if eol.LifecycleEvidence != "https://example.invalid/official-lifecycle" {
			t.Errorf("evidence = %q", eol.LifecycleEvidence)
		}
		if !eol.LifecycleAssessedAt.Equal(time.Date(2026, 8, 7, 0, 0, 0, 0, time.UTC)) {
			t.Errorf("assessed_at = %s", eol.LifecycleAssessedAt)
		}
	})

	t.Run("既定unknown", func(t *testing.T) {
		catalog, _, err := BuildCatalog(context.Background(), nil, goRequest(t, goDocument(t)))
		if err != nil {
			t.Fatalf("BuildCatalog = %s", describeErr(err))
		}
		item := catalog.Items[0]
		if item.Lifecycle != domain.LifecycleUnknown {
			t.Fatalf("lifecycle = %q", item.Lifecycle)
		}
		// source URLとfetch時刻を記録する。根拠不明のitemを作らない。
		if item.LifecycleEvidence != goSourceURL {
			t.Errorf("evidence = %q, want %q", item.LifecycleEvidence, goSourceURL)
		}
		if !item.LifecycleAssessedAt.Equal(fetchedAt) {
			t.Errorf("assessed_at = %s, want %s", item.LifecycleAssessedAt, fetchedAt)
		}
	})
}

// TestBuildCatalogMergesPublishedAt は§6.1の公開日時の優先順位を固定する。
func TestBuildCatalogMergesPublishedAt(t *testing.T) {
	cases := []struct {
		name        string
		itemValue   string
		assetValue  string
		want        string
		wantFailure bool
	}{
		{"itemだけ", "2026-07-01T00:00:00Z", "", "2026-07-01T00:00:00Z", false},
		{"assetだけ", "", "2026-07-02T00:00:00Z", "2026-07-02T00:00:00Z", false},
		{"同じ値", "2026-07-01T00:00:00Z", "2026-07-01T00:00:00Z", "2026-07-01T00:00:00Z", false},
		{"どちらも無い", "", "", "", false},
		// 複数の非空値が異なればsource errorにする。片方を黙って優先すると、
		// 上流のどちらが正しいかを人が確認する機会が消える。
		{"食い違い", "2026-07-01T00:00:00Z", "2026-07-02T00:00:00Z", "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			asset := &Asset{PublishedAt: c.assetValue}
			got, err := mergePublishedAt(c.itemValue, asset)
			if c.wantFailure {
				if err == nil {
					t.Fatal("食い違いが成功した")
				}
				return
			}
			if err != nil {
				t.Fatalf("mergePublishedAt = %v", err)
			}
			if got != c.want {
				t.Fatalf("= %q, want %q", got, c.want)
			}
		})
	}
}

// TestBuildCatalogExpiry は§15の期限を固定する。
//
// 「static sourceは`expires_at=null`を許す」。networkのcatalogは必ず期限を持つ。
func TestBuildCatalogExpiry(t *testing.T) {
	t.Run("network", func(t *testing.T) {
		catalog, _, err := BuildCatalog(context.Background(), nil, goRequest(t, goDocument(t)))
		if err != nil {
			t.Fatalf("BuildCatalog = %s", describeErr(err))
		}
		if !catalog.HasExpiry() {
			t.Fatal("network sourceのcatalogが期限を持たない")
		}
		if !catalog.ExpiresAt.Equal(fetchedAt.Add(24 * time.Hour)) {
			t.Fatalf("expires_at = %s", catalog.ExpiresAt)
		}
	})

	t.Run("static", func(t *testing.T) {
		entry := staticEntry(t, "3.13.7", definition.ChannelStable, definition.LifecycleSupported)
		entry.Assets = []definition.StaticAsset{{
			Name: "cpython-3.13.7-x86_64-pc-windows-msvc.tar.gz",
			URL:  "https://github.com/example/releases/download/20250814/example.tar.gz",
			Size: 1, Digest: digest64, DigestAlgorithm: definition.AlgorithmSHA256,
			OS: "windows", Arch: "amd64", Libc: "none", ReleaseTag: "20250814",
			ReleaseURL:  "https://github.com/example/releases/tag/20250814",
			ReleaseID:   "123",
			AssetID:     "456",
			PublishedAt: time.Date(2025, 8, 14, 0, 0, 0, 0, time.UTC),
		}}
		source := staticSource(t, entry)
		items, buildErr := BuildStaticItems(source)
		if buildErr != nil {
			t.Fatalf("BuildStaticItems = %s", describeErr(buildErr))
		}
		catalog, _, err := BuildCatalog(context.Background(), nil, BuildRequest{
			Tool: mustToolID(t, "python"), Platform: mustPlatform(t, "windows-amd64"),
			Source: source,
			Artifact: definition.Artifact{
				Source: definition.SourceAsset, Format: definition.FormatTarGz,
				Selector: &definition.ArtifactSelector{OS: "windows", Arch: "amd64"},
				Checksum: definition.ArtifactChecksum{Kind: definition.ChecksumAssetField},
			},
			ArtifactKind: definition.KindThirdParty, DefinitionSHA256: strings.Repeat("c", 64),
			SourceIdentity: "registry/tools/python.toml", FetchedAt: fetchedAt, Items: items,
		})
		if err != nil {
			t.Fatalf("BuildCatalog = %s", describeErr(err))
		}
		if catalog.HasExpiry() {
			t.Fatal("static sourceのcatalogが期限を持っている")
		}
		item := catalog.Items[0]
		if item.ProviderKind != store.ProviderThirdParty {
			t.Errorf("provider_kind = %q", item.ProviderKind)
		}
		// §6.5「static sourceはassetの必須`release_tag`を使う」。
		if item.ProviderRelease != "20250814" {
			t.Errorf("provider_release = %q, want 20250814", item.ProviderRelease)
		}
		// static itemはalgorithm fieldを持つため、definitionの宣言なしで解決する。
		if item.ArtifactDigest.Upstream() != "sha256:"+digest64 {
			t.Errorf("artifact_digest = %q", item.ArtifactDigest.Upstream())
		}
		// lifecycle記録はdefinition記録である。
		if item.LifecycleEvidence != "https://devguide.python.org/versions/" {
			t.Errorf("evidence = %q", item.LifecycleEvidence)
		}
	})
}

// TestBuildCatalogRoundTripsThroughCodec は組み立てたcatalogがそのままcodecを
// 通ることを固定する。§15の契約を2か所で別々に解釈しないための検査である。
func TestBuildCatalogRoundTripsThroughCodec(t *testing.T) {
	catalog, _, err := BuildCatalog(context.Background(), nil, goRequest(t, goDocument(t)))
	if err != nil {
		t.Fatalf("BuildCatalog = %s", describeErr(err))
	}
	data, encodeErr := store.EncodeCatalog(catalog)
	if encodeErr != nil {
		t.Fatalf("EncodeCatalog = %s", describeErr(encodeErr))
	}
	parsed, parseErr := store.ParseCatalog(store.CatalogRequest{Data: data, Scheme: domain.SchemeGo})
	if parseErr != nil {
		t.Fatalf("ParseCatalog = %s", describeErr(parseErr))
	}
	if len(parsed.Items) != len(catalog.Items) {
		t.Fatalf("items = %d件, want %d", len(parsed.Items), len(catalog.Items))
	}
	if parsed.SourceIdentity != goSourceURL {
		t.Errorf("source_identity = %q", parsed.SourceIdentity)
	}
	if parsed.Items[0].ArtifactDigest.Upstream() != catalog.Items[0].ArtifactDigest.Upstream() {
		t.Errorf("digestが往復しない")
	}
}

// --- helper ---

func mustToolID(t *testing.T, text string) domain.ToolID {
	t.Helper()
	id, err := domain.ParseToolID(text)
	if err != nil {
		t.Fatalf("ParseToolID(%q): %v", text, err)
	}
	return id
}

func mustPlatform(t *testing.T, text string) domain.Platform {
	t.Helper()
	platform, err := domain.ParsePlatform(text)
	if err != nil {
		t.Fatalf("ParsePlatform(%q): %v", text, err)
	}
	return platform
}

// TestBuildCatalogRendersAssetURLTemplate は§7.1の
// 「`source=asset`の`url`/`file`は非空なら選択assetを`{{asset.<field>}}`で
// 参照できるtemplateとしてrenderする」を固定する（利用者判断で§7.1へ明記）。
//
// Goの`https://go.dev/dl/?mode=json`の`files[]`はdownload URLを持たずfile名だけ
// を載せる。空のままではdownload先を決められない。
func TestBuildCatalogRendersAssetURLTemplate(t *testing.T) {
	// upstreamがURLを持たない形を再現する。
	document := mustDecode(t, `[
	  {"version":"go1.25.0","stable":true,"files":[
	    {"filename":"go1.25.0.windows-amd64.zip","size":1,"sha256":"`+digest64+`",
	     "os":"windows","arch":"amd64"}
	  ]}
	]`)
	source := goAssetSource()
	delete(source.AssetFields, definition.AssetURL)

	artifact := goArtifact()
	artifact.URL = "https://go.dev/dl/{{asset.name}}"
	artifact.File = "{{asset.name}}"

	items := mustBuildItems(t, ItemsRequest{
		Source: source, Scheme: domain.SchemeGo, Document: document, Origin: goSourceURL,
	})
	catalog, _, err := BuildCatalog(context.Background(), nil, BuildRequest{
		Tool: mustToolID(t, "go"), Platform: mustPlatform(t, "windows-amd64"),
		Source: source, Artifact: artifact, ArtifactKind: definition.KindOfficial,
		DefinitionSHA256: strings.Repeat("a", 64), SourceIdentity: goSourceURL,
		FetchedAt: fetchedAt, Items: items,
	})
	if err != nil {
		t.Fatalf("BuildCatalog = %s", describeErr(err))
	}
	item := catalog.Items[0]
	if item.ArtifactURL != "https://go.dev/dl/go1.25.0.windows-amd64.zip" {
		t.Errorf("artifact_url = %q", item.ArtifactURL)
	}
	if item.ArtifactFile != "go1.25.0.windows-amd64.zip" {
		t.Errorf("artifact_file = %q", item.ArtifactFile)
	}
	// digestはassetのfieldから決まる（`checksum.kind=asset-field`）。
	if item.ArtifactDigest.Upstream() != "sha256:"+digest64 {
		t.Errorf("artifact_digest = %q", item.ArtifactDigest.Upstream())
	}
}

// TestBuildCatalogUsesAssetURLWhenTemplateEmpty は空のときに選択assetの
// `url`/`name`を使うことを固定する（Python static sourceが該当する）。
func TestBuildCatalogUsesAssetURLWhenTemplateEmpty(t *testing.T) {
	catalog, _, err := BuildCatalog(context.Background(), nil, goRequest(t, goDocument(t)))
	if err != nil {
		t.Fatalf("BuildCatalog = %s", describeErr(err))
	}
	if catalog.Items[0].ArtifactURL != "https://go.dev/dl/go1.25.0.windows-amd64.zip" {
		t.Fatalf("artifact_url = %q", catalog.Items[0].ArtifactURL)
	}
}
