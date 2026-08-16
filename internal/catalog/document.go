package catalog

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain/port"
)

// DocumentMaxBytes は上流metadata 1文書の上限である。
//
// docs/04-storage-and-data.md §21「upstream metadata response各文書 16 MiB」。
const DocumentMaxBytes = 16 << 20

// RedirectMax は追跡するredirectの上限である（同§21「redirect 10」）。
const RedirectMax = 10

// Document は取得済みのJSON文書である。
type Document struct {
	// FinalURL はredirect後の最終URLである。
	FinalURL string
	// Root は復号したJSON値である。object、配列、scalarのいずれかになる。
	Root any
}

// FetchDocument は上流metadata文書を1件取得する。
//
// docs/06-tool-definition.md §6.1の「HTTPS GETで1文書だけを読む」「1文書あたり
// 16 MiB」「redirect後もHTTPS」を守る。
//
// **retryとbackoffは行わない。** docs/08-install-runtime.md §70の
// 「network timeout/5xx/429だけ初回後に最大3回retry」はHTTPClient adapterの
// 責務であり（docs/13-progress.md P5-01）、ここで二重に再送すると上限が積算する。
func FetchDocument(
	ctx context.Context, client port.HTTPClient, url string,
) (*Document, *domain.Error) {
	if client == nil {
		return nil, domain.Internal(fmt.Errorf("catalog: HTTPClientが未注入"))
	}
	response, err := client.Get(ctx, port.HTTPRequest{
		URL:          url,
		MaxRedirects: RedirectMax,
		MaxBodyBytes: DocumentMaxBytes,
	})
	if err != nil {
		return nil, fetchError(url, err)
	}
	defer func() {
		if response.Body != nil {
			_ = response.Body.Close()
		}
	}()

	if response.StatusCode != 200 {
		return nil, fetchError(url, fmt.Errorf("status %d", response.StatusCode))
	}
	// redirect後のschemeを呼出し側でも確かめる。adapterが守る契約だが、上流の
	// redirectでHTTPSから外れたcatalogを取り込むと、以降のdigest照合まで平文
	// 経路の内容を信頼することになる（§6.1「redirect後もHTTPS」）。
	if !strings.HasPrefix(response.FinalURL, "https://") {
		return nil, fetchError(url, fmt.Errorf("redirect後のURLがHTTPSでない（%q）", response.FinalURL))
	}
	if response.Body == nil {
		return nil, fetchError(url, fmt.Errorf("応答bodyが無い"))
	}

	// 上限+1 byteまで読み、超過を検出する。MaxBodyBytesはadapterへ渡す要求値で
	// あり、こちらでも切らないとadapterの実装差でarchive bomb相当の入力を受ける。
	body, readErr := io.ReadAll(io.LimitReader(response.Body, DocumentMaxBytes+1))
	if readErr != nil {
		return nil, fetchError(url, readErr)
	}
	if len(body) > DocumentMaxBytes {
		return nil, fetchError(url, fmt.Errorf("応答bodyが%d byteを超える", DocumentMaxBytes))
	}

	root, decodeErr := decodeJSON(body)
	if decodeErr != nil {
		return nil, sourceError(url, decodeErr)
	}
	return &Document{FinalURL: response.FinalURL, Root: root}, nil
}

// decodeJSON はJSON文書をexactly 1値として復号する。
//
// 数値は[json.Number]で保持する。float64へ落とすと、§6.5が「IDもprecision loss
// を避けるためdecimal stringとして扱う」と定めるrelease ID/asset IDが桁落ちし、
// receiptの再現性が壊れる。sizeも同じ理由でstringのまま扱う。
func decodeJSON(data []byte) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var root any
	if err := decoder.Decode(&root); err != nil {
		return nil, fmt.Errorf("JSONとして読めない: %w", err)
	}
	// 1文書は1値である。後続に値が続く入力を受けると、どちらを文書とみなすかが
	// 決まらない。
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, fmt.Errorf("JSON文書の後に余分な値がある")
	}
	return root, nil
}

// jsonKind は診断用のJSON型名を返す。
func jsonKind(node any) string {
	switch node.(type) {
	case nil:
		return "null"
	case bool:
		return "boolean"
	case json.Number:
		return "number"
	case string:
		return "string"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	default:
		return "unknown"
	}
}

// toScalar はJSON nodeを公開境界のscalarへ変換する。
//
// docs/02-architecture.md §10の[domain.Scalar]は string/bool/integer/null だけを
// 取る。配列・object・非整数の数値は表現できないため、写像側でenumへ落とす前に
// ここで拒否する。channel/lifecycleの写像は「未知値をfallbackしない」契約であり
// （§6.1）、表現できない値を黙ってnullへ倒すとその契約が崩れる。
func toScalar(node any) (domain.Scalar, error) {
	switch value := node.(type) {
	case nil:
		return domain.NullScalar(), nil
	case string:
		return domain.StringScalar(value), nil
	case bool:
		return domain.BoolScalar(value), nil
	case json.Number:
		number, err := value.Int64()
		if err != nil {
			return domain.Scalar{}, fmt.Errorf("数値 %s が64 bit整数でない", value.String())
		}
		return domain.IntScalar(number), nil
	default:
		return domain.Scalar{}, fmt.Errorf("%sはscalarでない", jsonKind(node))
	}
}

// fullDateLayout はISO 8601 full-dateである（§6.1）。
const fullDateLayout = "2006-01-02"

// normalizeTimestamp は公開日時を正規化する。
//
// §6.1が「UTC RFC 3339またはISO 8601 full-date（`YYYY-MM-DD`）のstringだけを受け、
// full-dateは`T00:00:00Z`へ正規化する」と定める。
//
// offsetが0でない値を拒否するのは、同じ時刻が別表記でcatalogへ入るとdiffと
// 突き合わせが読めなくなるためである。0 offsetの`+00:00`表記は`Z`へ揃える。
func normalizeTimestamp(text, field string) (string, error) {
	if date, err := time.Parse(fullDateLayout, text); err == nil {
		return date.UTC().Format(time.RFC3339), nil
	}
	stamp, err := time.Parse(time.RFC3339, text)
	if err != nil {
		return "", fmt.Errorf("%sがUTC RFC 3339でもISO 8601 full-dateでもない（%q）", field, text)
	}
	if _, offset := stamp.Zone(); offset != 0 {
		return "", fmt.Errorf("%sのoffsetがUTCでない（%q）", field, text)
	}
	return stamp.UTC().Format(time.RFC3339), nil
}
