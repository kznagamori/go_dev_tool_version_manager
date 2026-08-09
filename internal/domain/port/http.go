package port

import (
	"context"
	"io"
)

// HTTPRequest はGET/HEADの要求である。
//
// method、body、TLS optionを利用者入力から受けないのは、docs/10-security.md が
// TLS検証の無効化と任意hostへの接続を禁止するためである。許可するのはURLと
// header、上限だけとする。
type HTTPRequest struct {
	// URL はHTTPSの完全URLである。実装はhttpを拒否する。
	URL string
	// Header は追加request headerである。credentialを入れない。
	Header map[string]string
	// MaxRedirects はredirectの追跡上限である。0はredirect禁止を意味する。
	// 上限を要求側が持つのは、docs/12-public-docs.md のbootstrapがhostを
	// redirectごとに再検査する契約と揃えるためである。
	MaxRedirects int
	// MaxBodyBytes はresponse bodyの読取り上限である。0以下は拒否する。
	// archive bomb と無制限応答を境界で止める。
	MaxBodyBytes int64
}

// HTTPResponse は応答である。BodyはCloseするまで有効である。
type HTTPResponse struct {
	StatusCode int
	Header     map[string]string
	// ContentLength は不明なとき-1である。
	ContentLength int64
	// FinalURL はredirect後の最終URLである。host allowlist検査に使う。
	FinalURL string
	Body     io.ReadCloser
}

// HTTPClient はHTTPSのGET/HEADを抽象化する（docs/02-architecture.md §4.1）。
//
// cancelはcontextで伝える。実装はTLS検証を無効化する経路を持たない。
type HTTPClient interface {
	// Get はbody付きで取得する。呼出側がResponse.Bodyをcloseする。
	Get(ctx context.Context, req HTTPRequest) (*HTTPResponse, error)

	// Head はmetadataだけを取得する。Bodyはnilである。
	Head(ctx context.Context, req HTTPRequest) (*HTTPResponse, error)
}
