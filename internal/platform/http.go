// Package platform のHTTP client adapterである。
package platform

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain/port"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/security"
)

// MaxRedirects はredirect追跡の組込み上限である。
//
// docs/04-storage-and-data.md §21「redirect / network retry 10 / 初回後3回」。
// 要求側が指定する`MaxRedirects`はこの値を超えられない。
const MaxRedirects = 10

// MaxRetries は初回requestの後に行うretry回数である（同§21「初回後3回」）。
const MaxRetries = 3

// retryBackoff は各retry前の待機時間である（同§21「backoff 1/2/4秒」）。
//
// 件数はMaxRetriesと一致させる。ずれるとretryの途中でbackoffが決まらない。
var retryBackoff = [MaxRetries]time.Duration{
	1 * time.Second,
	2 * time.Second,
	4 * time.Second,
}

// MaxRetryAfter は`Retry-After`として受け入れる最大待機時間である
// （同§21「Retry-After最大30秒」）。
//
// 上流が過大な値を返しても、それに従って長時間待たない。
const MaxRetryAfter = 30 * time.Second

// ClientConfig はHTTP clientの構築parameterである。
//
// proxyとCA bundleを受け取らないのは、docs/05-configuration.md §3.4が
// 「proxyはGo toolchain固定versionの`net/http.ProxyFromEnvironment`契約を使用し、
// global config keyを設けない」「OS trust storeを使う。TLS検証無効、HTTP許可、
// 任意CA bundle、credential headerを設定するkeyはschema 1にない」と定めるため
// である。無効化経路を作らないために、構築時にも受け取らない。
type ClientConfig struct {
	// ConnectTimeout はTCP接続の上限である（§3.4「connect 1s～5m」）。
	ConnectTimeout time.Duration
	// RequestTimeout は1 requestの全体上限である（同「request 10s～1h」）。
	//
	// retryを含む全体ではなく、1回の試行に適用する。retryごとに上限を
	// 与え直さないと、1回目が上限近くまで粘ったときに残りが実行されない。
	RequestTimeout time.Duration
	// Clock はretry backoffの待機に使う。
	Clock port.Clock
}

// Client はport.HTTPClientのproduction実装である。
//
// docs/10-security.md §10に従い、HTTPS必須、TLS検証無効化なし、redirectごとの
// scheme/host/credential検査、有限retryを行う。
// structured logを出さないのは、docs/04-storage-and-data.md §18のLogRecordが
// invocation/operation IDを必須とし、それらをadapterが持たないためである。
// 秘密値のmaskはerror messageへ載るURLに対して適用する（docs/10-security.md §9.2）。
// 記録はrequestを発行したApplication Serviceが行う。
type Client struct {
	http    *http.Client
	clock   port.Clock
	timeout time.Duration
}

var _ port.HTTPClient = (*Client)(nil)

// NewClient はHTTP clientを作る。
func NewClient(config ClientConfig) (*Client, error) {
	switch {
	case config.ConnectTimeout <= 0:
		return nil, errors.New("platform: connect timeoutが正でない")
	case config.RequestTimeout <= 0:
		return nil, errors.New("platform: request timeoutが正でない")
	case config.Clock == nil:
		return nil, errors.New("platform: Clockが無い")
	}

	transport := &http.Transport{
		// docs/05-configuration.md §3.4。proxy設定keyを持たず、Go標準の
		// 環境変数契約をそのまま使う。
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout: config.ConnectTimeout,
		}).DialContext,
		// TLSClientConfigはMinVersionだけを指定する。RootCAsを設定しないことが
		// OS trust storeを使う指定であり、InsecureSkipVerifyを書ける経路を
		// 作らない（§3.4・docs/10-security.md §10）。
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
		TLSHandshakeTimeout:   config.ConnectTimeout,
		ResponseHeaderTimeout: config.RequestTimeout,
		ForceAttemptHTTP2:     true,
	}

	return &Client{
		http: &http.Client{
			Transport: transport,
			// redirectは自前で追う。net/httpに任せると、hopごとの
			// scheme/host/credential検査（§10）を挟めない。
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		clock:   config.Clock,
		timeout: config.RequestTimeout,
	}, nil
}

// Get はbody付きで取得する。呼出側がResponse.Bodyをcloseする。
func (c *Client) Get(ctx context.Context, req port.HTTPRequest) (*port.HTTPResponse, error) {
	return c.do(ctx, http.MethodGet, req)
}

// Head はmetadataだけを取得する。Bodyはnilである。
func (c *Client) Head(ctx context.Context, req port.HTTPRequest) (*port.HTTPResponse, error) {
	return c.do(ctx, http.MethodHead, req)
}

// do はretryを含む1回の論理requestを実行する。
//
// retryするのは429/5xx/一時networkだけである（§10）。404やTLS検証失敗は
// 再実行しても同じ結果になるか、再実行そのものが危険である。
func (c *Client) do(
	ctx context.Context, method string, req port.HTTPRequest,
) (*port.HTTPResponse, error) {
	if err := validateRequest(req); err != nil {
		return nil, err
	}

	var lastErr error
	for attempt := 0; ; attempt++ {
		response, retryAfter, err := c.attempt(ctx, method, req)
		if err == nil {
			return response, nil
		}
		lastErr = err

		var retryable *retryableError
		if !errors.As(err, &retryable) || attempt >= MaxRetries {
			return nil, lastErr
		}

		wait := retryBackoff[attempt]
		// `Retry-After`は上流の指示だが、過大な値へ無条件に従わない。
		if retryAfter > 0 {
			wait = retryAfter
			if wait > MaxRetryAfter {
				wait = MaxRetryAfter
			}
		}
		if err := c.clock.Sleep(ctx, wait); err != nil {
			// cancelはretryせず、待機の途中でも即座に抜ける。
			return nil, err
		}
	}
}

// attempt は1回の試行を行う。retry可能な失敗はretryableErrorで返す。
//
// 2番目の戻り値は`Retry-After`から読み取った待機時間である。指定が無ければ0を返す。
func (c *Client) attempt(
	ctx context.Context, method string, req port.HTTPRequest,
) (*port.HTTPResponse, time.Duration, error) {
	current := req.URL
	limit := req.MaxRedirects
	if limit > MaxRedirects {
		limit = MaxRedirects
	}

	for hop := 0; ; hop++ {
		// requestごとにtimeoutを与える。redirect chain全体で1つにすると、
		// hopが増えるほど各hopの実効時間が縮む。
		attemptCtx, cancel := context.WithTimeout(ctx, c.timeout)
		response, err := c.send(attemptCtx, method, current, req.Header)
		if err != nil {
			cancel()
			return nil, 0, err
		}

		location := response.Header.Get("Location")
		if !isRedirect(response.StatusCode) || location == "" {
			body, err := c.readBody(method, response, req.MaxBodyBytes, cancel)
			if err != nil {
				return nil, 0, err
			}
			if status := checkStatus(response); status != nil {
				body.Close()
				return nil, retryAfterOf(c.clock, response), status
			}
			return &port.HTTPResponse{
				StatusCode:    response.StatusCode,
				Header:        headerMap(response.Header),
				ContentLength: response.ContentLength,
				FinalURL:      current,
				Body:          body,
			}, 0, nil
		}

		// redirectは追わずにbodyを捨てる。
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
		response.Body.Close()
		cancel()

		if hop >= limit {
			return nil, 0, fmt.Errorf(
				"platform: redirectが上限%d回を超えた（%s）", limit, security.MaskURL(req.URL))
		}
		next, err := resolveRedirect(current, location)
		if err != nil {
			return nil, 0, err
		}
		current = next
	}
}

// send は1 hopを送る。
func (c *Client) send(
	ctx context.Context, method, target string, header map[string]string,
) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, method, target, nil)
	if err != nil {
		return nil, fmt.Errorf("platform: requestを作れない: %w", err)
	}
	for key, value := range header {
		request.Header.Set(key, value)
	}

	response, err := c.http.Do(request)
	if err != nil {
		if ctxErr := ctx.Err(); errors.Is(ctxErr, context.Canceled) {
			// 呼出し側のcancelはretryしない。
			return nil, ctxErr
		}
		wrapped := fmt.Errorf(
			"platform: %s の取得に失敗した: %w", security.MaskURL(target), unwrapURLError(err))
		if isTemporaryNetwork(err) {
			return nil, &retryableError{cause: wrapped}
		}
		return nil, wrapped
	}
	return response, nil
}

// unwrapURLError は`*url.Error`から内側のerrorを取り出す。
//
// `net/http`はrequest URLを**maskせずに**`*url.Error`へ含める。そのまま`%w`で
// 包むと、URL側をmaskしてもcause経由でtokenやuserinfoがerror文字列へ出る。
// 取り出すのは失敗の理由だけとし、URLはmask済みの側から与える
// （docs/10-security.md §9.2）。
func unwrapURLError(err error) error {
	var urlErr *url.Error
	if errors.As(err, &urlErr) && urlErr.Err != nil {
		return urlErr.Err
	}
	return err
}

// readBody は上限付きのbodyを返す。HEADではbodyを捨ててnilを返す。
func (c *Client) readBody(
	method string, response *http.Response, maxBytes int64, cancel context.CancelFunc,
) (io.ReadCloser, error) {
	if maxBytes <= 0 {
		response.Body.Close()
		cancel()
		return nil, errors.New("platform: MaxBodyBytesが正でない")
	}
	if method == http.MethodHead {
		response.Body.Close()
		cancel()
		return io.NopCloser(strings.NewReader("")), nil
	}
	// Content-Lengthが上限を超えると分かっている応答は読まずに拒否する。
	if response.ContentLength > maxBytes {
		response.Body.Close()
		cancel()
		return nil, fmt.Errorf(
			"platform: response bodyが上限%d byteを超える（Content-Length %d）",
			maxBytes, response.ContentLength)
	}
	return &limitedBody{
		reader: io.LimitReader(response.Body, maxBytes+1),
		closer: response.Body,
		cancel: cancel,
		limit:  maxBytes,
	}, nil
}

// limitedBody は上限を超えた読取りをerrorにするbodyである。
//
// `io.LimitReader`だけではEOFとして静かに切り詰められ、途中までのarchiveを
// 完全なものとして扱ってしまう。
type limitedBody struct {
	reader io.Reader
	closer io.Closer
	cancel context.CancelFunc
	limit  int64
	read   int64
}

func (b *limitedBody) Read(p []byte) (int, error) {
	n, err := b.reader.Read(p)
	b.read += int64(n)
	if b.read > b.limit {
		return n, fmt.Errorf("platform: response bodyが上限%d byteを超えた", b.limit)
	}
	return n, err
}

func (b *limitedBody) Close() error {
	err := b.closer.Close()
	b.cancel()
	return err
}

// retryableError はretryしてよい失敗である。
type retryableError struct {
	cause error
}

func (e *retryableError) Error() string { return e.cause.Error() }
func (e *retryableError) Unwrap() error { return e.cause }

// validateRequest は要求側の指定を検査する。
func validateRequest(req port.HTTPRequest) error {
	parsed, err := url.Parse(req.URL)
	if err != nil {
		return fmt.Errorf("platform: URLを解釈できない: %w", err)
	}
	if err := checkURL(parsed); err != nil {
		return err
	}
	if req.MaxRedirects < 0 {
		return errors.New("platform: MaxRedirectsが負である")
	}
	if req.MaxBodyBytes <= 0 {
		return errors.New("platform: MaxBodyBytesが正でない")
	}
	// credential headerを要求側から受けない（docs/10-security.md §10）。
	for key := range req.Header {
		if security.IsSecretHeader(key) {
			return fmt.Errorf("platform: header %q は指定できない", key)
		}
	}
	return nil
}

// checkURL はscheme/credential/hostを検査する。
//
// HTTPS必須、URL userinfo禁止（§10）。redirectのhopごとにも同じ検査を通す。
func checkURL(parsed *url.URL) error {
	switch {
	case parsed.Scheme != "https":
		return fmt.Errorf("platform: schemeがhttpsでない（%q）", parsed.Scheme)
	case parsed.User != nil:
		return errors.New("platform: URLにuserinfoを含められない")
	case parsed.Host == "":
		return errors.New("platform: URLにhostが無い")
	}
	return nil
}

// resolveRedirect はLocationを解決して次のURLを返す。
func resolveRedirect(current, location string) (string, error) {
	base, err := url.Parse(current)
	if err != nil {
		return "", fmt.Errorf("platform: redirect元URLを解釈できない: %w", err)
	}
	next, err := base.Parse(location)
	if err != nil {
		return "", fmt.Errorf("platform: redirect先を解釈できない: %w", err)
	}
	// hopごとにscheme/host/credentialを検査する（§10）。httpsからhttpへ
	// 落とすredirectを追わない。
	if err := checkURL(next); err != nil {
		return "", fmt.Errorf("platform: redirect先が不正である: %w", err)
	}
	return next.String(), nil
}

func isRedirect(status int) bool {
	switch status {
	case http.StatusMovedPermanently, http.StatusFound, http.StatusSeeOther,
		http.StatusTemporaryRedirect, http.StatusPermanentRedirect:
		return true
	}
	return false
}

// checkStatus は成功以外のstatusをerrorにする。
//
// 429と5xxだけをretry可能にする（§10）。404やほかの4xxは再実行しても同じ結果に
// なるため、retryせず即座に返す。
func checkStatus(response *http.Response) error {
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		return nil
	}
	err := fmt.Errorf("platform: HTTP %d", response.StatusCode)
	if response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= 500 {
		return &retryableError{cause: err}
	}
	return err
}

// retryAfterOf は`Retry-After`headerを待機時間として読む。
//
// RFC 9110は秒数とHTTP-dateの両方を認める。dateはClockの現在時刻との差を使う。
// 過去日時や解釈できない値は0（指定なし）として扱い、backoffの既定値へ戻す。
func retryAfterOf(clock port.Clock, response *http.Response) time.Duration {
	raw := strings.TrimSpace(response.Header.Get("Retry-After"))
	if raw == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(raw); err == nil {
		if seconds <= 0 {
			return 0
		}
		return time.Duration(seconds) * time.Second
	}
	stamp, err := http.ParseTime(raw)
	if err != nil {
		return 0
	}
	wait := stamp.Sub(clock.Now())
	if wait <= 0 {
		return 0
	}
	return wait
}

// isTemporaryNetwork は一時的なnetwork失敗かを返す。
//
// TLS検証失敗はretryしない（§10「checksum/schema/404/security errorをretryしない」）。
// 同じ相手へ同じ検証を繰り返しても結果は変わらず、失敗を隠すだけになる。
func isTemporaryNetwork(err error) bool {
	var certErr *tls.CertificateVerificationError
	if errors.As(err, &certErr) {
		return false
	}
	var recordErr tls.RecordHeaderError
	if errors.As(err, &recordErr) {
		return false
	}
	// dial/read/writeの失敗（connection refused、reset、host unreachable）は
	// 相手側の再起動や一時的な経路障害で起きるため、有限回だけ再試行する。
	//
	// `*net.OpError`は`net.Error`でもあるため、先に判定する。順序を逆にすると
	// `Timeout()`がfalseなOpErrorがすべて非retryとして落ちる。
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return netErr.Timeout()
	}
	return false
}

// headerMap はresponse headerを1 key 1 valueへ写す。
//
// 同じkeyが複数ある場合は最初の値を使う。port.HTTPResponseが単一値のmapを
// 契約にしているためで、複数値を連結して1つに見せない。
func headerMap(header http.Header) map[string]string {
	values := make(map[string]string, len(header))
	for key := range header {
		values[key] = header.Get(key)
	}
	return values
}
