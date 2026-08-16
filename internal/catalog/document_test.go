package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain/port"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain/port/fake"
)

const testDocumentURL = "https://example.invalid/versions.json"

// TestFetchDocumentReadsJSON は§6.1の「HTTPS GETで1文書だけを読む」を固定する。
func TestFetchDocumentReadsJSON(t *testing.T) {
	client := fake.NewHTTPClient(nil)
	client.Stub(testDocumentURL, fake.HTTPStub{
		StatusCode: 200,
		Body:       []byte(`[{"version":"v22.18.0"}]`),
	})

	document, err := FetchDocument(context.Background(), client, testDocumentURL)
	if err != nil {
		t.Fatalf("FetchDocument = %s", describeErr(err))
	}
	if document.FinalURL != testDocumentURL {
		t.Errorf("FinalURL = %q", document.FinalURL)
	}
	if describeNode(document.Root) != "array" {
		t.Fatalf("Root = %s, want array", describeNode(document.Root))
	}

	// 上限とredirect上限を要求へ載せる（docs/04-storage-and-data.md §21）。
	if len(client.Requests) != 1 || client.Requests[0] != testDocumentURL {
		t.Errorf("Requests = %v", client.Requests)
	}
}

// TestFetchDocumentKeepsNumberPrecision は数値をjson.Numberで保持することを固定する。
//
// §6.5が「IDもprecision lossを避けるためdecimal stringとして扱う」と定める。
// float64へ落とすと大きなrelease ID/asset IDが桁落ちし、receiptの再現性が壊れる。
func TestFetchDocumentKeepsNumberPrecision(t *testing.T) {
	const bigID = "9007199254740993" // 2^53+1。float64では表現できない
	client := fake.NewHTTPClient(nil)
	client.Stub(testDocumentURL, fake.HTTPStub{
		StatusCode: 200,
		Body:       []byte(`{"release_id":` + bigID + `}`),
	})

	document, err := FetchDocument(context.Background(), client, testDocumentURL)
	if err != nil {
		t.Fatalf("FetchDocument = %s", describeErr(err))
	}
	node, resolveErr := resolvePointer(document.Root, "/release_id")
	if resolveErr != nil {
		t.Fatal(resolveErr)
	}
	number, ok := node.(json.Number)
	if !ok {
		t.Fatalf("node = %T, want json.Number", node)
	}
	if number.String() != bigID {
		t.Fatalf("release_id = %s, want %s", number.String(), bigID)
	}
}

// TestFetchDocumentRejectsBadResponses は取得側の失敗を固定する。
func TestFetchDocumentRejectsBadResponses(t *testing.T) {
	cases := []struct {
		name string
		stub fake.HTTPStub
		code domain.ErrorCode
	}{
		{"404", fake.HTTPStub{StatusCode: 404, Body: []byte(`{}`)}, domain.CodeNetwork},
		{"500", fake.HTTPStub{StatusCode: 500, Body: []byte(`{}`)}, domain.CodeNetwork},
		{"204", fake.HTTPStub{StatusCode: 204}, domain.CodeNetwork},
		// definitionが参照する形になっていない文書はsource errorである。
		{"壊れたJSON", fake.HTTPStub{StatusCode: 200, Body: []byte(`{`)}, domain.CodeDefinitionInvalid},
		{"空body", fake.HTTPStub{StatusCode: 200, Body: []byte(``)}, domain.CodeDefinitionInvalid},
		{
			"文書の後に余分な値",
			fake.HTTPStub{StatusCode: 200, Body: []byte(`{"a":1} {"b":2}`)},
			domain.CodeDefinitionInvalid,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			client := fake.NewHTTPClient(nil)
			client.Stub(testDocumentURL, c.stub)
			_, err := FetchDocument(context.Background(), client, testDocumentURL)
			if err == nil {
				t.Fatal("FetchDocumentが成功した")
			}
			if err.Code != c.code {
				t.Fatalf("code = %s, want %s", err.Code, c.code)
			}
		})
	}
}

// TestFetchDocumentTransportFailureIsRetryable は取得失敗がretryable networkに
// なることを固定する。retry自体はHTTPClient adapterの責務である（P5-01）。
func TestFetchDocumentTransportFailureIsRetryable(t *testing.T) {
	client := fake.NewHTTPClient(nil)
	client.Injector().FailOnce(fake.OpHTTPGet, fake.ErrDownloadFailed)
	client.Stub(testDocumentURL, fake.HTTPStub{StatusCode: 200, Body: []byte(`[]`)})

	_, err := FetchDocument(context.Background(), client, testDocumentURL)
	if err == nil {
		t.Fatal("FetchDocumentが成功した")
	}
	if err.Code != domain.CodeNetwork || !err.Retryable {
		t.Fatalf("err = %s / retryable=%v", err.Code, err.Retryable)
	}
}

// TestFetchDocumentRejectsNonHTTPSFinalURL はredirect後のschemeを固定する。
//
// §6.1の「redirect後もHTTPS」。adapterが守る契約だが、平文経路の内容を取り込むと
// 以降のdigest照合まで信頼の根が無くなるため、呼出し側でも確かめる。
func TestFetchDocumentRejectsNonHTTPSFinalURL(t *testing.T) {
	_, err := FetchDocument(context.Background(), plainFinalURLClient{}, testDocumentURL)
	if err == nil {
		t.Fatal("FetchDocumentが成功した")
	}
	if err.Code != domain.CodeNetwork {
		t.Fatalf("code = %s, want %s", err.Code, domain.CodeNetwork)
	}
}

// TestFetchDocumentRejectsOversizeBody は16 MiB上限を呼出し側でも切ることを固定する。
//
// docs/04-storage-and-data.md §21「upstream metadata response各文書 16 MiB」。
// adapterが上限を守らなかった場合でも、ここで止める。
func TestFetchDocumentRejectsOversizeBody(t *testing.T) {
	_, err := FetchDocument(context.Background(), endlessBodyClient{}, testDocumentURL)
	if err == nil {
		t.Fatal("FetchDocumentが成功した")
	}
	if err.Code != domain.CodeNetwork {
		t.Fatalf("code = %s, want %s", err.Code, domain.CodeNetwork)
	}
	if !strings.Contains(err.Cause.Error(), "16777216") {
		t.Fatalf("cause = %v", err.Cause)
	}
}

// TestFetchDocumentRequiresClient はport未注入をinternal errorにすることを固定する。
func TestFetchDocumentRequiresClient(t *testing.T) {
	if _, err := FetchDocument(context.Background(), nil, testDocumentURL); err == nil {
		t.Fatal("HTTPClientなしで成功した")
	}
}

// TestToScalarAcceptsOnlyScalars は公開境界のscalarへ落とせる値を固定する。
//
// docs/02-architecture.md §10のScalarは string/bool/integer/null だけを取る。
// 表現できない値を黙ってnullへ倒すと、§6.1の「未知値をfallbackしない」契約が崩れる。
func TestToScalarAcceptsOnlyScalars(t *testing.T) {
	accepted := map[string]domain.ScalarKind{
		`"stable"`: domain.ScalarString,
		`true`:     domain.ScalarBool,
		`12`:       domain.ScalarInt,
		`null`:     domain.ScalarNull,
	}
	for text, kind := range accepted {
		scalar, err := toScalar(mustDecode(t, text))
		if err != nil {
			t.Errorf("toScalar(%s) = %v, want nil", text, err)
			continue
		}
		if scalar.Kind() != kind {
			t.Errorf("toScalar(%s).Kind() = %d, want %d", text, scalar.Kind(), kind)
		}
	}
	for _, text := range []string{`[]`, `{}`, `1.5`, `1e400`} {
		if _, err := toScalar(mustDecode(t, text)); err == nil {
			t.Errorf("toScalar(%s) が成功した", text)
		}
	}
}

// TestNormalizeTimestamp は§6.1の公開日時の受理形式を固定する。
func TestNormalizeTimestamp(t *testing.T) {
	accepted := map[string]string{
		// full-dateは`T00:00:00Z`へ正規化する。
		"2025-08-14":            "2025-08-14T00:00:00Z",
		"2026-07-01T00:00:00Z":  "2026-07-01T00:00:00Z",
		"2026-07-01T09:30:00Z":  "2026-07-01T09:30:00Z",
		"2026-07-01T00:00:00Z ": "",
		// 0 offsetの`+00:00`表記は`Z`へ揃える。同じ時刻が別表記でcatalogへ入ると
		// diffと突き合わせが読めなくなる。
		"2026-07-01T00:00:00+00:00": "2026-07-01T00:00:00Z",
		// 秒未満は正規形へ落とす。
		"2026-07-01T00:00:00.500Z": "2026-07-01T00:00:00Z",
	}
	for input, want := range accepted {
		got, err := normalizeTimestamp(input, "published_at_pointer")
		if want == "" {
			if err == nil {
				t.Errorf("normalizeTimestamp(%q) が成功した", input)
			}
			continue
		}
		if err != nil {
			t.Errorf("normalizeTimestamp(%q) = %v, want nil", input, err)
			continue
		}
		if got != want {
			t.Errorf("normalizeTimestamp(%q) = %q, want %q", input, got, want)
		}
	}
	rejected := []struct{ input, why string }{
		{"", "空"},
		{"2026-07-01T00:00:00+09:00", "UTCでないoffset"},
		{"2026-07-01T00:00:00", "offsetが無い"},
		{"2026-07-01 00:00:00Z", "spaceで区切っている"},
		{"20260701", "区切りが無い"},
		{"2026-13-01", "月が範囲外"},
		{"yesterday", "日時でない"},
	}
	for _, c := range rejected {
		if _, err := normalizeTimestamp(c.input, "published_at_pointer"); err == nil {
			t.Errorf("normalizeTimestamp(%q) が成功した（%s）", c.input, c.why)
		}
	}
}

// --- helper ---

func mustDecode(t *testing.T, text string) any {
	t.Helper()
	root, err := decodeJSON([]byte(text))
	if err != nil {
		t.Fatalf("decodeJSON(%s): %v", text, err)
	}
	return root
}

// describeNode はJSON nodeを比較しやすい文字列にする。
func describeNode(node any) string {
	switch value := node.(type) {
	case string:
		return value
	case json.Number:
		return value.String()
	default:
		return jsonKind(node)
	}
}

func describeErr(err *domain.Error) string {
	if err == nil {
		return "<nil>"
	}
	if err.Cause != nil {
		return string(err.Code) + ": " + err.Cause.Error()
	}
	return string(err.Code)
}

// plainFinalURLClient はredirect先がHTTPSでない応答を返す。
//
// fake.HTTPClientはHTTPS以外のstubを登録できないため、この経路専用のstubを置く。
type plainFinalURLClient struct{}

func (plainFinalURLClient) Get(_ context.Context, req port.HTTPRequest) (*port.HTTPResponse, error) {
	return &port.HTTPResponse{
		StatusCode: 200,
		FinalURL:   "http://example.invalid/versions.json",
		Body:       io.NopCloser(strings.NewReader(`[]`)),
	}, nil
}

func (plainFinalURLClient) Head(context.Context, port.HTTPRequest) (*port.HTTPResponse, error) {
	return nil, errors.New("未使用")
}

// endlessBodyClient は上限を無視して無限のbodyを返す。
//
// adapterが`MaxBodyBytes`を守らなかった場合を再現する。
type endlessBodyClient struct{}

func (endlessBodyClient) Get(context.Context, port.HTTPRequest) (*port.HTTPResponse, error) {
	return &port.HTTPResponse{
		StatusCode: 200,
		FinalURL:   testDocumentURL,
		Body:       io.NopCloser(endlessReader{}),
	}, nil
}

func (endlessBodyClient) Head(context.Context, port.HTTPRequest) (*port.HTTPResponse, error) {
	return nil, errors.New("未使用")
}

type endlessReader struct{}

func (endlessReader) Read(buffer []byte) (int, error) {
	for i := range buffer {
		buffer[i] = ' '
	}
	return len(buffer), nil
}
