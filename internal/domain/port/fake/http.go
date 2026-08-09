package fake

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"sync"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain/port"
)

// HTTPClient操作名。
const (
	OpHTTPGet  = "http.Get"
	OpHTTPHead = "http.Head"
)

// ErrDownloadFailed はdownload失敗のfailure injectionで使う。
// docs/11-quality-and-ci.md §8 のscenario 10が要求する。
var ErrDownloadFailed = errors.New("fake: download failed")

// HTTPStub は1つのURLに対する応答定義である。
type HTTPStub struct {
	StatusCode int
	Header     map[string]string
	Body       []byte
	// RedirectTo が空でなければredirect先とみなす。実装が追跡上限を数える。
	RedirectTo string
}

// HTTPClient は登録した応答だけを返す決定的なport.HTTPClientである。
//
// 未登録URLはerrorにする。networkへ出ないことを型で保証するのではなく、
// 「stubを書き忘れたtestが黙って通らない」ようにするためである。
type HTTPClient struct {
	mu       sync.Mutex
	stubs    map[string]HTTPStub
	injector *Injector
	// Requests は要求されたURLを順に記録する。host allowlist検査に使う。
	Requests []string
}

var _ port.HTTPClient = (*HTTPClient)(nil)

// NewHTTPClient は空のHTTPClientを作る。
func NewHTTPClient(injector *Injector) *HTTPClient {
	if injector == nil {
		injector = NewInjector()
	}
	return &HTTPClient{stubs: make(map[string]HTTPStub), injector: injector}
}

// Injector は失敗注入器を返す。
func (h *HTTPClient) Injector() *Injector { return h.injector }

// Stub はURLへの応答を登録する。
func (h *HTTPClient) Stub(url string, stub HTTPStub) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.stubs[url] = stub
}

func (h *HTTPClient) do(ctx context.Context, op string, req port.HTTPRequest, withBody bool) (*port.HTTPResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := h.injector.Check(op); err != nil {
		return nil, err
	}
	if !strings.HasPrefix(req.URL, "https://") {
		return nil, errors.New("fake: only https is allowed")
	}
	if req.MaxBodyBytes <= 0 {
		return nil, errors.New("fake: MaxBodyBytes must be positive")
	}

	current := req.URL
	for hop := 0; ; hop++ {
		h.mu.Lock()
		h.Requests = append(h.Requests, current)
		stub, ok := h.stubs[current]
		h.mu.Unlock()
		if !ok {
			return nil, errors.New("fake: no stub registered for " + current)
		}
		if stub.RedirectTo == "" {
			if int64(len(stub.Body)) > req.MaxBodyBytes {
				return nil, errors.New("fake: response body exceeds MaxBodyBytes")
			}
			resp := &port.HTTPResponse{
				StatusCode:    stub.StatusCode,
				Header:        stub.Header,
				ContentLength: int64(len(stub.Body)),
				FinalURL:      current,
			}
			if withBody {
				resp.Body = io.NopCloser(bytes.NewReader(append([]byte(nil), stub.Body...)))
			}
			return resp, nil
		}
		if hop >= req.MaxRedirects {
			return nil, errors.New("fake: too many redirects")
		}
		current = stub.RedirectTo
	}
}

// Get はbody付きで取得する。
func (h *HTTPClient) Get(ctx context.Context, req port.HTTPRequest) (*port.HTTPResponse, error) {
	return h.do(ctx, OpHTTPGet, req, true)
}

// Head はmetadataだけを取得する。
func (h *HTTPClient) Head(ctx context.Context, req port.HTTPRequest) (*port.HTTPResponse, error) {
	return h.do(ctx, OpHTTPHead, req, false)
}
