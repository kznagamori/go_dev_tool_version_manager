package catalog

import (
	"context"
	"strings"
	"testing"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/definition"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain/port/fake"
)

const (
	dotnetIndexURL = "https://builds.dotnet.microsoft.com/dotnet/release-metadata/releases-index.json"
	dotnetChild90  = "https://builds.dotnet.microsoft.com/dotnet/release-metadata/9.0/releases.json"
	dotnetChild80  = "https://builds.dotnet.microsoft.com/dotnet/release-metadata/8.0/releases.json"
)

// indexSource はdocs/06-tool-definition.md §16.4の.NET SDK sourceである。
func indexSource() definition.VersionSource {
	source := flattenSource()
	source.URL = dotnetIndexURL
	source.IndexItemsPointer = "/releases-index"
	// pointer tokenに`.`を含むkeyを指す。Microsoftのindexは`releases.json`という
	// 名前のfieldへ子文書URLを入れる。
	source.IndexDocumentPointer = "/releases.json"
	source.MaxDocuments = 32
	source.DocumentLifecyclePointer = definition.DeclaredPointer("/support-phase")
	source.LifecycleMap = map[string]definition.Lifecycle{
		"preview": definition.LifecycleSupported, "go-live": definition.LifecycleSupported,
		"active": definition.LifecycleSupported, "maintenance": definition.LifecycleSupported,
		"eol": definition.LifecycleEOL,
	}
	return source
}

func indexDocument(t *testing.T, urls ...string) any {
	t.Helper()
	entries := make([]string, len(urls))
	for index, value := range urls {
		entries[index] = `{"channel-version":"x","releases.json":"` + value + `"}`
	}
	return mustDecode(t, `{"releases-index":[`+strings.Join(entries, ",")+`]}`)
}

// TestChildDocumentURLsKeepsDeclarationOrder は§6.2の「子文書は宣言順に処理」を
// 固定する。
func TestChildDocumentURLsKeepsDeclarationOrder(t *testing.T) {
	urls, err := ChildDocumentURLs(IndexRequest{
		Source:   indexSource(),
		Document: indexDocument(t, dotnetChild90, dotnetChild80),
		IndexURL: dotnetIndexURL,
	})
	if err != nil {
		t.Fatalf("ChildDocumentURLs = %s", describeErr(err))
	}
	want := []string{dotnetChild90, dotnetChild80}
	if len(urls) != len(want) {
		t.Fatalf("urls = %v", urls)
	}
	for index, value := range want {
		if urls[index] != value {
			t.Errorf("urls[%d] = %q, want %q", index, urls[index], value)
		}
	}
}

// TestChildDocumentURLsDeduplicates は§6.2の「重複URLは1回だけ取得する」を固定する。
func TestChildDocumentURLsDeduplicates(t *testing.T) {
	urls, err := ChildDocumentURLs(IndexRequest{
		Source:   indexSource(),
		Document: indexDocument(t, dotnetChild90, dotnetChild80, dotnetChild90),
		IndexURL: dotnetIndexURL,
	})
	if err != nil {
		t.Fatalf("ChildDocumentURLs = %s", describeErr(err))
	}
	if len(urls) != 2 || urls[0] != dotnetChild90 || urls[1] != dotnetChild80 {
		t.Fatalf("urls = %v（最初の出現順で1回だけ）", urls)
	}
}

// TestChildDocumentURLsRestrictsHost は§6.2のhost規則を固定する。
//
// 「子文書URLのhostはindex文書のhost、または`artifact.redirect_hosts`と同じ規則で
// 宣言した完全hostだけを許す。index応答から任意hostを動的に信頼しない。」
func TestChildDocumentURLsRestrictsHost(t *testing.T) {
	const otherHost = "https://cdn.example.invalid/9.0/releases.json"

	t.Run("index文書のhostは許可", func(t *testing.T) {
		if _, err := ChildDocumentURLs(IndexRequest{
			Source:   indexSource(),
			Document: indexDocument(t, dotnetChild90),
			IndexURL: dotnetIndexURL,
		}); err != nil {
			t.Fatalf("= %s", describeErr(err))
		}
	})

	t.Run("宣言していないhostは拒否", func(t *testing.T) {
		_, err := ChildDocumentURLs(IndexRequest{
			Source:   indexSource(),
			Document: indexDocument(t, otherHost),
			IndexURL: dotnetIndexURL,
		})
		if err == nil {
			t.Fatal("index応答のhostをそのまま信頼した")
		}
		if err.Code != domain.CodeDefinitionInvalid {
			t.Fatalf("code = %s", err.Code)
		}
	})

	t.Run("redirect_hostsで宣言したhostは許可", func(t *testing.T) {
		if _, err := ChildDocumentURLs(IndexRequest{
			Source:        indexSource(),
			RedirectHosts: []string{"cdn.example.invalid"},
			Document:      indexDocument(t, otherHost),
			IndexURL:      dotnetIndexURL,
		}); err != nil {
			t.Fatalf("= %s", describeErr(err))
		}
	})
}

// TestChildDocumentURLsRejectsBadURLs は子文書URLの形を固定する。
func TestChildDocumentURLsRejectsBadURLs(t *testing.T) {
	cases := []struct{ url, why string }{
		{"http://builds.dotnet.microsoft.com/9.0/releases.json", "HTTPSでない"},
		{"/9.0/releases.json", "絶対URLでない"},
		{"https://user:pass@builds.dotnet.microsoft.com/r.json", "credentialを含む"},
		{"https://builds.dotnet.microsoft.com:8443/r.json", "portを含む"},
		{"https://BUILDS.dotnet.microsoft.com/r.json", "hostがlowercaseでない"},
		{"https:///r.json", "hostが無い"},
		{"ftp://builds.dotnet.microsoft.com/r.json", "HTTPSでない"},
	}
	for _, c := range cases {
		t.Run(c.why, func(t *testing.T) {
			_, err := ChildDocumentURLs(IndexRequest{
				Source:   indexSource(),
				Document: indexDocument(t, c.url),
				IndexURL: dotnetIndexURL,
			})
			if err == nil {
				t.Fatalf("ChildDocumentURLs(%q) が成功した", c.url)
			}
		})
	}
}

// TestChildDocumentURLsAppliesDocumentLimit は§6.2の`max_documents`を固定する。
//
// 上限は**除去後の取得対象数**へ効く。`max_documents`は取得する文書数の上限
// だからである。超過は切り捨てずerrorにする。
func TestChildDocumentURLsAppliesDocumentLimit(t *testing.T) {
	base := "https://builds.dotnet.microsoft.com/dotnet/release-metadata/"

	t.Run("重複を除けば上限内", func(t *testing.T) {
		source := indexSource()
		source.MaxDocuments = 2
		urls, err := ChildDocumentURLs(IndexRequest{
			Source:   source,
			Document: indexDocument(t, dotnetChild90, dotnetChild80, dotnetChild90, dotnetChild80),
			IndexURL: dotnetIndexURL,
		})
		if err != nil {
			t.Fatalf("= %s", describeErr(err))
		}
		if len(urls) != 2 {
			t.Fatalf("urls = %v", urls)
		}
	})

	t.Run("max_documents超過", func(t *testing.T) {
		source := indexSource()
		source.MaxDocuments = 2
		_, err := ChildDocumentURLs(IndexRequest{
			Source: source,
			Document: indexDocument(t,
				base+"9.0/releases.json", base+"8.0/releases.json", base+"7.0/releases.json"),
			IndexURL: dotnetIndexURL,
		})
		if err == nil {
			t.Fatal("上限超過が成功した")
		}
	})

	t.Run("組込み上限32を拡大できない", func(t *testing.T) {
		source := indexSource()
		source.MaxDocuments = definition.MaxDocumentsLimit * 2
		urls := make([]string, 0, definition.MaxDocumentsLimit+1)
		for index := 0; index <= definition.MaxDocumentsLimit; index++ {
			urls = append(urls, base+string(rune('a'+index%26))+string(rune('0'+index/26))+"/r.json")
		}
		_, err := ChildDocumentURLs(IndexRequest{
			Source:   source,
			Document: indexDocument(t, urls...),
			IndexURL: dotnetIndexURL,
		})
		if err == nil {
			t.Fatal("組込み上限を超えて成功した")
		}
	})
}

// TestChildDocumentURLsRejectsBadIndexLayout はindex文書のlayout違反を固定する。
func TestChildDocumentURLsRejectsBadIndexLayout(t *testing.T) {
	cases := []struct{ name, document string }{
		{"index_items_pointerが配列でない", `{"releases-index":{"releases.json":"x"}}`},
		{"index_items_pointerが無い", `{"other":[]}`},
		{"index_document_pointerが無い", `{"releases-index":[{"channel-version":"9.0"}]}`},
		{"index_document_pointerが文字列でない", `{"releases-index":[{"releases.json":42}]}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := ChildDocumentURLs(IndexRequest{
				Source:   indexSource(),
				Document: mustDecode(t, c.document),
				IndexURL: dotnetIndexURL,
			})
			if err == nil {
				t.Fatal("ChildDocumentURLsが成功した")
			}
			if err.Code != domain.CodeDefinitionInvalid {
				t.Fatalf("code = %s", err.Code)
			}
		})
	}
}

// TestChildDocumentURLsRejectsBadIndexURL はindex URL自体の形を固定する。
func TestChildDocumentURLsRejectsBadIndexURL(t *testing.T) {
	_, err := ChildDocumentURLs(IndexRequest{
		Source:   indexSource(),
		Document: indexDocument(t, dotnetChild90),
		IndexURL: "http://builds.dotnet.microsoft.com/index.json",
	})
	if err == nil {
		t.Fatal("HTTPSでないindex URLが成功した")
	}
}

// TestFetchChildDocumentsFailsWholeCatalog は§6.2の部分catalog禁止を固定する。
//
// 「1件でも取得・parseに失敗したらcatalog全体をsource errorにする。部分catalogを
// 公開しない。」読めた子文書だけでcatalogを作ると、取得に失敗した子文書のversion
// が「上流から消えた」ように見える。
func TestFetchChildDocumentsFailsWholeCatalog(t *testing.T) {
	t.Run("全件成功", func(t *testing.T) {
		client := fake.NewHTTPClient(nil)
		client.Stub(dotnetChild90, fake.HTTPStub{StatusCode: 200, Body: []byte(`{"releases":[]}`)})
		client.Stub(dotnetChild80, fake.HTTPStub{StatusCode: 200, Body: []byte(`{"releases":[]}`)})

		documents, err := FetchChildDocuments(
			context.Background(), client, []string{dotnetChild90, dotnetChild80})
		if err != nil {
			t.Fatalf("= %s", describeErr(err))
		}
		if len(documents) != 2 {
			t.Fatalf("documents = %d件", len(documents))
		}
		// 宣言順に取得する。
		if client.Requests[0] != dotnetChild90 || client.Requests[1] != dotnetChild80 {
			t.Fatalf("Requests = %v", client.Requests)
		}
	})

	t.Run("2件目が失敗すれば全体が失敗", func(t *testing.T) {
		client := fake.NewHTTPClient(nil)
		client.Stub(dotnetChild90, fake.HTTPStub{StatusCode: 200, Body: []byte(`{"releases":[]}`)})
		client.Stub(dotnetChild80, fake.HTTPStub{StatusCode: 503, Body: []byte(``)})

		documents, err := FetchChildDocuments(
			context.Background(), client, []string{dotnetChild90, dotnetChild80})
		if err == nil {
			t.Fatal("部分catalogが返った")
		}
		if documents != nil {
			t.Fatalf("失敗時に%d件が返った", len(documents))
		}
	})

	t.Run("2件目のparse失敗も全体が失敗", func(t *testing.T) {
		client := fake.NewHTTPClient(nil)
		client.Stub(dotnetChild90, fake.HTTPStub{StatusCode: 200, Body: []byte(`{"releases":[]}`)})
		client.Stub(dotnetChild80, fake.HTTPStub{StatusCode: 200, Body: []byte(`{broken`)})

		_, err := FetchChildDocuments(
			context.Background(), client, []string{dotnetChild90, dotnetChild80})
		if err == nil {
			t.Fatal("parse失敗が無視された")
		}
		if err.Code != domain.CodeDefinitionInvalid {
			t.Fatalf("code = %s", err.Code)
		}
	})
}
