package catalog

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/definition"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain/port"
)

// IndexRequest はindex文書から子文書URLを取り出す要求である（§6.2）。
type IndexRequest struct {
	// Source はdefinitionのversion sourceである。kindは`json-index`である。
	Source definition.VersionSource
	// RedirectHosts は同じplatformの`artifact.redirect_hosts`である。
	//
	// §6.2は子文書URLのhostを「index文書のhost、または`artifact.redirect_hosts`と
	// 同じ規則で宣言した完全host」に限る。§6.1の許可key 24件に host を宣言する
	// keyが無いため、**同じplatformのartifactが宣言したhost集合**を指す。
	// wildcardなしのASCII lowercase完全hostという規則もそこから引き継ぐ。
	RedirectHosts []string
	// Document はindex文書のrootである。
	Document any
	// IndexURL はindex文書のURLである。catalogの`source_identity`にもなる。
	IndexURL string
}

// ChildDocumentURLs はindex文書から子文書URLを宣言順で返す（§6.2）。
//
// 重複URLは1回だけ取得するため、最初の出現順を保って除去する。件数は除去後の
// 取得対象数で判定する。`max_documents`は取得する文書数の上限だからである。
//
// **index応答から任意hostを動的に信頼しない。** 許可hostはindex文書のhostと
// definitionが宣言した完全hostだけで、redirect先の最終URLからallowlistを
// 広げない（§6.2・docs/10-security.md §132）。
func ChildDocumentURLs(req IndexRequest) ([]string, *domain.Error) {
	indexHost, err := documentHost(req.IndexURL)
	if err != nil {
		return nil, sourceError(req.IndexURL, fmt.Errorf("index URL: %w", err))
	}
	allowed := allowedChildHosts(indexHost, req.RedirectHosts)

	entries, err := pointerArray(req.Document, req.Source.IndexItemsPointer)
	if err != nil {
		return nil, sourceError(req.IndexURL, fmt.Errorf("index_items_pointer: %w", err))
	}

	seen := make(map[string]struct{}, len(entries))
	urls := make([]string, 0, len(entries))
	for index, entry := range entries {
		child, childErr := pointerString(entry, req.Source.IndexDocumentPointer)
		if childErr != nil {
			return nil, sourceError(req.IndexURL,
				fmt.Errorf("index_document_pointer index[%d]: %w", index, childErr))
		}
		if hostErr := checkChildHost(child, allowed); hostErr != nil {
			return nil, sourceError(req.IndexURL, fmt.Errorf("index[%d]: %w", index, hostErr))
		}
		if _, duplicate := seen[child]; duplicate {
			continue
		}
		seen[child] = struct{}{}
		urls = append(urls, child)
	}

	if limitErr := checkDocumentLimit(len(urls), req.Source, req.IndexURL); limitErr != nil {
		return nil, limitErr
	}
	return urls, nil
}

// checkDocumentLimit は§6.2の`max_documents`と組込み上限を適用する。
//
// 超過を切り捨てない。読む文書を黙って打ち切ると、残りの子文書に載っていた
// versionが存在しないことと区別できなくなる。
func checkDocumentLimit(count int, source definition.VersionSource, origin string) *domain.Error {
	limit := definition.MaxDocumentsLimit
	if source.MaxDocuments > 0 && source.MaxDocuments < limit {
		limit = source.MaxDocuments
	}
	if count > limit {
		return sourceError(origin, fmt.Errorf("子文書が上限%d件を超える（%d件）", limit, count))
	}
	return nil
}

// allowedChildHosts は許可する子文書hostの集合を作る。
func allowedChildHosts(indexHost string, redirectHosts []string) map[string]struct{} {
	allowed := make(map[string]struct{}, len(redirectHosts)+1)
	allowed[indexHost] = struct{}{}
	for _, host := range redirectHosts {
		allowed[host] = struct{}{}
	}
	return allowed
}

// checkChildHost は子文書URLが許可hostのHTTPS絶対URLであることを検査する。
func checkChildHost(child string, allowed map[string]struct{}) error {
	host, err := documentHost(child)
	if err != nil {
		return err
	}
	if _, ok := allowed[host]; !ok {
		return fmt.Errorf("子文書URLのhost %q が許可されていない（%q）", host, child)
	}
	return nil
}

// documentHost はHTTPS絶対URLのhostを返す。
//
// portを含む形は受けない。`redirect_hosts`は「ASCII lowercase完全host」であり
// （§7.1）、portの有無で同じhostが2通りに書けると照合が一致しなくなる。
// credentialを含むURLも拒否する（docs/10-security.md）。
func documentHost(raw string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("URLとして読めない（%q）", raw)
	}
	switch {
	case parsed.Scheme != "https":
		return "", fmt.Errorf("HTTPS絶対URLでない（%q）", raw)
	case parsed.User != nil:
		return "", fmt.Errorf("URLがcredentialを含む")
	case parsed.Host == "":
		return "", fmt.Errorf("URLにhostが無い（%q）", raw)
	case parsed.Port() != "":
		return "", fmt.Errorf("URLがportを含む（%q）", raw)
	}
	host := parsed.Hostname()
	if host != strings.ToLower(host) {
		return "", fmt.Errorf("hostがlowercaseでない（%q）", host)
	}
	return host, nil
}

// FetchChildDocuments は子文書を宣言順に取得する（§6.2）。
//
// **1件でも取得・parseに失敗したらcatalog全体を失敗させる。** 部分catalogを
// 公開しない。読めた子文書だけでcatalogを作ると、取得に失敗した子文書のversion
// が「上流から消えた」ように見える。
func FetchChildDocuments(
	ctx context.Context, client port.HTTPClient, urls []string,
) ([]*Document, *domain.Error) {
	documents := make([]*Document, 0, len(urls))
	for _, childURL := range urls {
		document, err := FetchDocument(ctx, client, childURL)
		if err != nil {
			return nil, err
		}
		documents = append(documents, document)
	}
	return documents, nil
}
