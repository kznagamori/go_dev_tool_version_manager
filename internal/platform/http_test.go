package platform

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain/port"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain/port/fake"
)

// testStart は決定的なfake clockの起点である。
var testStart = time.Date(2026, 8, 17, 0, 0, 0, 0, time.UTC)

// newTestClient はTLS serverの証明書を信頼するclientを作る。
//
// `httptest.NewTLSServer`のself-signed証明書はOS trust storeに無い。production
// 実装がTLS検証を無効化する経路を持たないことが本taskの契約なので、client側の
// 設定を緩めるのではなく、serverの証明書をtransportへ与える。
func newTestClient(t *testing.T, server *httptest.Server) (*Client, *fake.Clock) {
	t.Helper()
	clock := fake.NewClock(testStart)
	client, err := NewClient(ClientConfig{
		ConnectTimeout: 30 * time.Second,
		RequestTimeout: 5 * time.Second,
		Clock:          clock,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	transport, ok := client.http.Transport.(*http.Transport)
	if !ok {
		t.Fatal("transportがhttp.Transportでない")
	}
	transport.TLSClientConfig = server.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
	return client, clock
}

func testRequest(url string) port.HTTPRequest {
	return port.HTTPRequest{URL: url, MaxRedirects: 5, MaxBodyBytes: 1 << 20}
}

// TestClientGetReadsBody は成功応答を読めることを固定する。
func TestClientGetReadsBody(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %q", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer server.Close()

	client, _ := newTestClient(t, server)
	response, err := client.Get(context.Background(), testRequest(server.URL))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		t.Errorf("StatusCode = %d", response.StatusCode)
	}
	if response.FinalURL != server.URL {
		t.Errorf("FinalURL = %q, want %q", response.FinalURL, server.URL)
	}
	if response.Header["Content-Type"] != "application/json" {
		t.Errorf("Content-Type = %q", response.Header["Content-Type"])
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(body) != `{"ok":true}` {
		t.Errorf("body = %q", body)
	}
}

// TestClientHeadDiscardsBody はHEADがbodyを返さないことを固定する。
func TestClientHeadDiscardsBody(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodHead {
			t.Errorf("method = %q", r.Method)
		}
		w.Header().Set("Content-Length", "11")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, _ := newTestClient(t, server)
	response, err := client.Head(context.Background(), testRequest(server.URL))
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	defer response.Body.Close()
	if response.ContentLength != 11 {
		t.Errorf("ContentLength = %d, want 11", response.ContentLength)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(body) != 0 {
		t.Errorf("HEADがbodyを返した: %q", body)
	}
}

// TestClientRejectsInsecureRequest はdocs/10-security.md §10の入力契約を固定する。
//
// HTTPS必須、URL userinfo禁止、credential headerを要求側から受けない。
func TestClientRejectsInsecureRequest(t *testing.T) {
	client, _ := newTestClient(t, httptest.NewTLSServer(http.NotFoundHandler()))

	cases := []struct {
		name string
		req  port.HTTPRequest
	}{
		{"http", port.HTTPRequest{URL: "http://example.com/", MaxBodyBytes: 1}},
		{"scheme無し", port.HTTPRequest{URL: "example.com/", MaxBodyBytes: 1}},
		{"file", port.HTTPRequest{URL: "file:///etc/passwd", MaxBodyBytes: 1}},
		{"userinfo", port.HTTPRequest{URL: "https://user:pass@example.com/", MaxBodyBytes: 1}},
		{"host無し", port.HTTPRequest{URL: "https:///path", MaxBodyBytes: 1}},
		{"MaxBodyBytesが0", port.HTTPRequest{URL: "https://example.com/"}},
		{"MaxBodyBytesが負", port.HTTPRequest{URL: "https://example.com/", MaxBodyBytes: -1}},
		{"MaxRedirectsが負", port.HTTPRequest{
			URL: "https://example.com/", MaxRedirects: -1, MaxBodyBytes: 1}},
		{"Authorization header", port.HTTPRequest{
			URL: "https://example.com/", MaxBodyBytes: 1,
			Header: map[string]string{"Authorization": "Bearer x"}}},
		{"Cookie header", port.HTTPRequest{
			URL: "https://example.com/", MaxBodyBytes: 1,
			Header: map[string]string{"Cookie": "a=b"}}},
		{"Proxy-Authorization header", port.HTTPRequest{
			URL: "https://example.com/", MaxBodyBytes: 1,
			Header: map[string]string{"Proxy-Authorization": "Basic x"}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := client.Get(context.Background(), c.req); err == nil {
				t.Fatal("不正な要求が通った")
			}
		})
	}
}

// TestClientMasksCredentialInError はerror messageへcredentialを載せないことを
// 固定する（docs/10-security.md §9.2）。
func TestClientMasksCredentialInError(t *testing.T) {
	client, _ := newTestClient(t, httptest.NewTLSServer(http.NotFoundHandler()))
	// 到達しないhostで失敗させ、token付きqueryがmaskされることを見る。
	target := "https://127.0.0.1:1/path?access_token=SECRETVALUE"
	_, err := client.Get(context.Background(), testRequest(target))
	if err == nil {
		t.Fatal("到達しないhostで成功した")
	}
	if strings.Contains(err.Error(), "SECRETVALUE") {
		t.Fatalf("error messageへsecretが出た: %v", err)
	}
}

// TestClientFollowsRedirect はredirectを追い、FinalURLへ最終URLを返すことを
// 固定する。
func TestClientFollowsRedirect(t *testing.T) {
	var hops int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/start":
			atomic.AddInt32(&hops, 1)
			http.Redirect(w, r, "/next", http.StatusFound)
		case "/next":
			atomic.AddInt32(&hops, 1)
			_, _ = io.WriteString(w, "arrived")
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client, _ := newTestClient(t, server)
	response, err := client.Get(context.Background(), testRequest(server.URL+"/start"))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer response.Body.Close()
	if response.FinalURL != server.URL+"/next" {
		t.Errorf("FinalURL = %q", response.FinalURL)
	}
	if got := atomic.LoadInt32(&hops); got != 2 {
		t.Errorf("hop = %d, want 2", got)
	}
}

// TestClientRejectsRedirectToInsecureScheme はhttpsからhttpへ落とすredirectを
// 追わないことを固定する（§10「redirectごとにscheme/host/credentialを検査」）。
func TestClientRejectsRedirectToInsecureScheme(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://example.com/downgraded", http.StatusFound)
	}))
	defer server.Close()

	client, _ := newTestClient(t, server)
	if _, err := client.Get(context.Background(), testRequest(server.URL)); err == nil {
		t.Fatal("httpへのredirectを追った")
	}
}

// TestClientRejectsRedirectWithCredential はuserinfo付きredirect先を拒否する
// ことを固定する。
func TestClientRejectsRedirectWithCredential(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://user:pass@example.com/", http.StatusFound)
	}))
	defer server.Close()

	client, _ := newTestClient(t, server)
	if _, err := client.Get(context.Background(), testRequest(server.URL)); err == nil {
		t.Fatal("userinfo付きredirectを追った")
	}
}

// TestClientEnforcesRedirectLimit はredirect上限を固定する。
//
// 要求側の指定と§21の組込み上限10のうち小さい方を使う。
func TestClientEnforcesRedirectLimit(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 無限にredirectし続ける。
		http.Redirect(w, r, "/loop", http.StatusFound)
	}))
	defer server.Close()
	client, _ := newTestClient(t, server)

	t.Run("要求側の上限", func(t *testing.T) {
		req := testRequest(server.URL)
		req.MaxRedirects = 2
		if _, err := client.Get(context.Background(), req); err == nil {
			t.Fatal("上限を超えても成功した")
		} else if !strings.Contains(err.Error(), "redirectが上限2回") {
			t.Fatalf("上限違反として報告されない: %v", err)
		}
	})

	t.Run("組込み上限を超える指定", func(t *testing.T) {
		req := testRequest(server.URL)
		// §21の10を超える指定は10へ丸める。要求側が上限を緩められない。
		req.MaxRedirects = 1000
		if _, err := client.Get(context.Background(), req); err == nil {
			t.Fatal("上限を超えても成功した")
		} else if !strings.Contains(err.Error(), "redirectが上限10回") {
			t.Fatalf("組込み上限へ丸められていない: %v", err)
		}
	})

	t.Run("redirect禁止", func(t *testing.T) {
		req := testRequest(server.URL)
		req.MaxRedirects = 0
		if _, err := client.Get(context.Background(), req); err == nil {
			t.Fatal("MaxRedirects=0でredirectを追った")
		}
	})
}

// TestClientEnforcesBodyLimit は上限超過のbodyをerrorにすることを固定する。
//
// 静かに切り詰めると、途中までのarchiveを完全なものとして扱ってしまう。
func TestClientEnforcesBodyLimit(t *testing.T) {
	payload := strings.Repeat("a", 1000)
	t.Run("Content-Lengthで事前に拒否", func(t *testing.T) {
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(w, payload)
		}))
		defer server.Close()
		client, _ := newTestClient(t, server)
		req := testRequest(server.URL)
		req.MaxBodyBytes = 100
		if _, err := client.Get(context.Background(), req); err == nil {
			t.Fatal("上限超過が通った")
		} else if !strings.Contains(err.Error(), "上限") {
			t.Fatalf("上限違反として報告されない: %v", err)
		}
	})

	t.Run("読取り中に超過", func(t *testing.T) {
		// Content-Lengthを出さないchunked応答で、読取り中に超えさせる。
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			flusher, _ := w.(http.Flusher)
			for index := 0; index < 10; index++ {
				_, _ = io.WriteString(w, payload)
				if flusher != nil {
					flusher.Flush()
				}
			}
		}))
		defer server.Close()
		client, _ := newTestClient(t, server)
		req := testRequest(server.URL)
		req.MaxBodyBytes = 100
		response, err := client.Get(context.Background(), req)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		defer response.Body.Close()
		if _, err := io.ReadAll(response.Body); err == nil {
			t.Fatal("上限を超えて読めた")
		}
	})

	t.Run("上限ちょうどは読める", func(t *testing.T) {
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(w, payload)
		}))
		defer server.Close()
		client, _ := newTestClient(t, server)
		req := testRequest(server.URL)
		req.MaxBodyBytes = int64(len(payload))
		response, err := client.Get(context.Background(), req)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		defer response.Body.Close()
		body, err := io.ReadAll(response.Body)
		if err != nil {
			t.Fatalf("ReadAll: %v", err)
		}
		if len(body) != len(payload) {
			t.Errorf("読めた長さ = %d, want %d", len(body), len(payload))
		}
	})
}

// TestClientRetriesTransientStatus は429と5xxだけを有限retryすることを固定する
// （§10「429/5xx/一時networkだけ有限retry」）。
//
// backoffはfake Clockが記録するため、実時間を消費しない。
func TestClientRetriesTransientStatus(t *testing.T) {
	cases := []struct {
		name       string
		status     int
		wantTries  int
		wantSleeps []time.Duration
	}{
		// §21「初回後3回。backoff 1/2/4秒」。
		{"503", http.StatusServiceUnavailable, 1 + MaxRetries,
			[]time.Duration{time.Second, 2 * time.Second, 4 * time.Second}},
		{"429", http.StatusTooManyRequests, 1 + MaxRetries,
			[]time.Duration{time.Second, 2 * time.Second, 4 * time.Second}},
		{"500", http.StatusInternalServerError, 1 + MaxRetries,
			[]time.Duration{time.Second, 2 * time.Second, 4 * time.Second}},
		// 4xxはretryしない。再実行しても同じ結果になる。
		{"404", http.StatusNotFound, 1, nil},
		{"403", http.StatusForbidden, 1, nil},
		{"400", http.StatusBadRequest, 1, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var tries int32
			server := httptest.NewTLSServer(http.HandlerFunc(
				func(w http.ResponseWriter, _ *http.Request) {
					atomic.AddInt32(&tries, 1)
					w.WriteHeader(c.status)
				}))
			defer server.Close()

			client, clock := newTestClient(t, server)
			if _, err := client.Get(context.Background(), testRequest(server.URL)); err == nil {
				t.Fatal("失敗statusで成功した")
			}
			if got := int(atomic.LoadInt32(&tries)); got != c.wantTries {
				t.Errorf("試行回数 = %d, want %d", got, c.wantTries)
			}
			sleeps := clock.Sleeps()
			if len(sleeps) != len(c.wantSleeps) {
				t.Fatalf("backoff = %v, want %v", sleeps, c.wantSleeps)
			}
			for index := range c.wantSleeps {
				if sleeps[index] != c.wantSleeps[index] {
					t.Errorf("backoff[%d] = %v, want %v",
						index, sleeps[index], c.wantSleeps[index])
				}
			}
		})
	}
}

// TestClientSucceedsAfterRetry はretry後に成功すればbodyを返すことを固定する。
func TestClientSucceedsAfterRetry(t *testing.T) {
	var tries int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if atomic.AddInt32(&tries, 1) < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = io.WriteString(w, "recovered")
	}))
	defer server.Close()

	client, clock := newTestClient(t, server)
	response, err := client.Get(context.Background(), testRequest(server.URL))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(body) != "recovered" {
		t.Errorf("body = %q", body)
	}
	// 失敗2回のあと成功したので、backoffは2回だけである。
	if got := clock.Sleeps(); len(got) != 2 {
		t.Errorf("backoff = %v, want 2件", got)
	}
}

// TestClientHonorsRetryAfter は`Retry-After`を上限付きで尊重することを固定する
// （§21「Retry-After最大30秒」）。
func TestClientHonorsRetryAfter(t *testing.T) {
	cases := []struct {
		name   string
		header string
		want   time.Duration
	}{
		{"秒数", "5", 5 * time.Second},
		// 上限を超える指定は30秒へ丸める。上流の過大な値へ従わない。
		{"上限超過", "600", MaxRetryAfter},
		{"上限ちょうど", "30", MaxRetryAfter},
		// 解釈できない値と非正値はbackoffの既定へ戻す。
		{"解釈不能", "soon", time.Second},
		{"0", "0", time.Second},
		{"負", "-5", time.Second},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(
				func(w http.ResponseWriter, _ *http.Request) {
					w.Header().Set("Retry-After", c.header)
					w.WriteHeader(http.StatusServiceUnavailable)
				}))
			defer server.Close()

			client, clock := newTestClient(t, server)
			if _, err := client.Get(context.Background(), testRequest(server.URL)); err == nil {
				t.Fatal("503で成功した")
			}
			sleeps := clock.Sleeps()
			if len(sleeps) == 0 {
				t.Fatal("backoffが記録されていない")
			}
			if sleeps[0] != c.want {
				t.Errorf("1回目のbackoff = %v, want %v", sleeps[0], c.want)
			}
		})
	}
}

// TestClientHonorsRetryAfterHTTPDate はHTTP-date形式の`Retry-After`を扱うことを
// 固定する。
//
// 過去日時は指定なしとして扱い、backoffの既定へ戻す。
func TestClientHonorsRetryAfterHTTPDate(t *testing.T) {
	cases := []struct {
		name  string
		stamp time.Time
		want  time.Duration
	}{
		{"未来", testStart.Add(10 * time.Second), 10 * time.Second},
		{"過去", testStart.Add(-time.Hour), time.Second},
		{"上限超過", testStart.Add(time.Hour), MaxRetryAfter},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(
				func(w http.ResponseWriter, _ *http.Request) {
					w.Header().Set("Retry-After", c.stamp.Format(http.TimeFormat))
					w.WriteHeader(http.StatusServiceUnavailable)
				}))
			defer server.Close()

			client, clock := newTestClient(t, server)
			if _, err := client.Get(context.Background(), testRequest(server.URL)); err == nil {
				t.Fatal("503で成功した")
			}
			sleeps := clock.Sleeps()
			if len(sleeps) == 0 {
				t.Fatal("backoffが記録されていない")
			}
			if sleeps[0] != c.want {
				t.Errorf("1回目のbackoff = %v, want %v", sleeps[0], c.want)
			}
		})
	}
}

// TestClientStopsOnCancel はcancelでretryを打ち切ることを固定する。
//
// cancelは「状態を変えれば再実行できる」失敗ではないため、backoffを待たずに抜ける。
func TestClientStopsOnCancel(t *testing.T) {
	var tries int32
	ctx, cancel := context.WithCancel(context.Background())
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if atomic.AddInt32(&tries, 1) == 1 {
			// 1回目の応答後にcancelする。2回目のbackoffで抜けるはずである。
			cancel()
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client, clock := newTestClient(t, server)
	_, err := client.Get(ctx, testRequest(server.URL))
	if err == nil {
		t.Fatal("cancelしたのに成功した")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if got := clock.Sleeps(); len(got) != 0 {
		t.Errorf("cancel後に待機した: %v", got)
	}
	if got := atomic.LoadInt32(&tries); got != 1 {
		t.Errorf("試行回数 = %d, want 1", got)
	}
}

// TestNewClientRejectsInvalidConfig は構築parameterの検査を固定する。
func TestNewClientRejectsInvalidConfig(t *testing.T) {
	clock := fake.NewClock(testStart)
	cases := []struct {
		name   string
		config ClientConfig
	}{
		{"connect timeoutが0", ClientConfig{RequestTimeout: time.Second, Clock: clock}},
		{"connect timeoutが負", ClientConfig{
			ConnectTimeout: -time.Second, RequestTimeout: time.Second, Clock: clock}},
		{"request timeoutが0", ClientConfig{ConnectTimeout: time.Second, Clock: clock}},
		{"Clockが無い", ClientConfig{
			ConnectTimeout: time.Second, RequestTimeout: time.Second}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := NewClient(c.config); err == nil {
				t.Fatal("不正なconfigが通った")
			}
		})
	}
}

// TestNewClientUsesStandardProxyAndTrustStore はproxyとTLSの契約を固定する。
//
// docs/05-configuration.md §3.4は「proxyはGo標準の`ProxyFromEnvironment`契約」
// 「OS trust storeを使う。TLS検証無効を設定するkeyはschema 1にない」と定める。
// RootCAsをnilのままにすることがOS trust storeを使う指定である。
func TestNewClientUsesStandardProxyAndTrustStore(t *testing.T) {
	client, err := NewClient(ClientConfig{
		ConnectTimeout: time.Second,
		RequestTimeout: time.Second,
		Clock:          fake.NewClock(testStart),
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	transport, ok := client.http.Transport.(*http.Transport)
	if !ok {
		t.Fatal("transportがhttp.Transportでない")
	}
	if transport.Proxy == nil {
		t.Error("Proxyが未設定である")
	}
	if transport.TLSClientConfig == nil {
		t.Fatal("TLSClientConfigが無い")
	}
	if transport.TLSClientConfig.InsecureSkipVerify {
		t.Error("InsecureSkipVerifyが有効である")
	}
	if transport.TLSClientConfig.RootCAs != nil {
		t.Error("RootCAsが設定されている（OS trust storeを使わない）")
	}
	if transport.TLSClientConfig.MinVersion < 0x0303 {
		t.Errorf("MinVersion = %#x, want TLS 1.2以上", transport.TLSClientConfig.MinVersion)
	}
}

// TestRetryBackoffMatchesRetryCount はbackoff表とretry回数が一致することを
// 固定する。
//
// ずれるとretryの途中でbackoffが決まらない。
func TestRetryBackoffMatchesRetryCount(t *testing.T) {
	if len(retryBackoff) != MaxRetries {
		t.Fatalf("backoff = %d件, want %d", len(retryBackoff), MaxRetries)
	}
	// §21が定める1/2/4秒である。
	want := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second}
	for index := range want {
		if retryBackoff[index] != want[index] {
			t.Errorf("backoff[%d] = %v, want %v", index, retryBackoff[index], want[index])
		}
	}
}

// TestClientSendsCustomHeader は秘密でないheaderを送ることを固定する。
//
// credential headerは[validateRequest]が拒否するが、`Accept`のような通常の
// headerは要求側が指定できる。
func TestClientSendsCustomHeader(t *testing.T) {
	var got string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("Accept")
		_, _ = io.WriteString(w, "ok")
	}))
	defer server.Close()

	client, _ := newTestClient(t, server)
	req := testRequest(server.URL)
	req.Header = map[string]string{"Accept": "application/json"}
	response, err := client.Get(context.Background(), req)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer response.Body.Close()
	if got != "application/json" {
		t.Errorf("Accept = %q", got)
	}
}

// TestIsTemporaryNetworkExcludesTLSFailures はTLS検証失敗をretry対象にしない
// ことを固定する。
//
// docs/10-security.md §10「checksum/schema/404/security errorをretryしない」。
// 同じ相手へ同じ検証を繰り返しても結果は変わらず、失敗を隠すだけになる。
func TestIsTemporaryNetworkExcludesTLSFailures(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"証明書検証失敗", &tls.CertificateVerificationError{}, false},
		{"TLS record header", tls.RecordHeaderError{Msg: "bad"}, false},
		// timeoutは一時的な失敗である。
		{"timeout", &net.OpError{Err: timeoutError{}}, true},
		{"接続失敗", &net.OpError{Op: "dial", Err: errors.New("refused")}, true},
		{"無関係なerror", errors.New("plain"), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isTemporaryNetwork(c.err); got != c.want {
				t.Errorf("isTemporaryNetwork = %t, want %t", got, c.want)
			}
		})
	}
}

// timeoutError はnet.Errorとしてtimeoutを名乗る。
type timeoutError struct{}

func (timeoutError) Error() string   { return "timeout" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return true }

// TestRetryableErrorWrapsCause はretryableErrorがcauseを保持することを固定する。
func TestRetryableErrorWrapsCause(t *testing.T) {
	cause := errors.New("HTTP 503")
	err := &retryableError{cause: cause}
	if err.Error() != cause.Error() {
		t.Errorf("Error = %q, want %q", err.Error(), cause.Error())
	}
	if !errors.Is(err, cause) {
		t.Error("Unwrapでcauseへ辿れない")
	}
}

// TestIsOfflineDistinguishesUnreachableNetwork は接続そのものが無い状態と
// 一時障害を区別することを固定する。
//
// 利用者が取るべき行動が違う。`E_NETWORK`は再実行で直りうる一時障害、
// `E_OFFLINE`は接続が無い状態を指す（docs/03-cli.md §7）。判定はport境界の
// このadapterだけが行い、呼出し側は[port.ErrOffline]で見分ける。
func TestIsOfflineDistinguishesUnreachableNetwork(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"DNS解決失敗", &net.DNSError{Err: "no such host", IsNotFound: true}, true},
		{"network unreachable", &net.OpError{
			Op: "dial", Err: os.NewSyscallError("connect", syscall.ENETUNREACH)}, true},
		{"host unreachable", &net.OpError{
			Op: "dial", Err: os.NewSyscallError("connect", syscall.EHOSTUNREACH)}, true},
		// 到達できたうえでの失敗は一時障害である。
		{"connection refused", &net.OpError{
			Op: "dial", Err: os.NewSyscallError("connect", syscall.ECONNREFUSED)}, false},
		{"HTTP 503", errors.New("platform: HTTP 503"), false},
		{"無関係なerror", errors.New("plain"), false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := isOffline(test.err); got != test.want {
				t.Errorf("isOffline = %t, want %t", got, test.want)
			}
		})
	}
}

// TestWrapNetworkErrorMarksOffline は到達できない相手のerrorをport.ErrOfflineで
// wrapすることを固定する。
//
// 呼出し側がsyscall errnoを見ずに判定できることが、この正規化の目的である。
// 実networkを使わずに固定するため、wrap部分だけを直接呼ぶ。
func TestWrapNetworkErrorMarksOffline(t *testing.T) {
	offline := wrapNetworkError("https://example.invalid/a",
		&net.DNSError{Err: "no such host", IsNotFound: true})
	if !errors.Is(offline, port.ErrOffline) {
		t.Errorf("offline = %v, want port.ErrOffline でwrapされていること", offline)
	}

	transient := wrapNetworkError("https://example.invalid/a",
		&net.OpError{Op: "dial", Err: os.NewSyscallError("connect", syscall.ECONNREFUSED)})
	if errors.Is(transient, port.ErrOffline) {
		t.Errorf("一時障害がofflineとしてwrapされた: %v", transient)
	}
}

// TestWrapNetworkErrorMasksURL はwrapしたerrorへcredentialを載せないことを固定する。
func TestWrapNetworkErrorMasksURL(t *testing.T) {
	wrapped := wrapNetworkError(
		"https://user:pw@example.invalid/a?access_token=SECRETVALUE", errors.New("boom"))
	if strings.Contains(wrapped.Error(), "SECRETVALUE") ||
		strings.Contains(wrapped.Error(), "user:pw@") {
		t.Errorf("errorへcredentialが載った: %v", wrapped)
	}
}
