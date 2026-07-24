package ports

import (
	"context"
	"io"
)

// HTTPRequest は HTTPClient への要求である。具体的な net/http 型を境界へ漏らさない。
// header の secret 値は Logger の mask 対象であり、本型には呼び出し側が sanitized 前提で
// 渡す。
type HTTPRequest struct {
	Method  string // "GET" または "HEAD"
	URL     string // absolute HTTPS URL
	Headers map[string]string
	// RangeFrom が非 nil のとき Range: bytes=<from>- を付与し、再開 download に用いる。
	RangeFrom *int64
	// MaxBodyBytes は body の受信上限（0 は既定上限を呼び出し側が別途強制）。
	MaxBodyBytes int64
}

// HTTPResponse は HTTPClient の応答である。Body は呼び出し側が Close する。
type HTTPResponse struct {
	StatusCode    int
	Headers       map[string]string
	Body          io.ReadCloser
	ContentLength int64 // 不明は -1
	FinalURL      string
}

// HTTPClient は GET/HEAD、range 再開、redirect 再検査、proxy、TLS、response 上限を扱う
// 抽象 port である（02章5節, 08章5節, 11章11節）。TLS 検証を無効化する手段を提供せず、
// redirect ごとに scheme/host policy を呼び出し側が再検査できるよう FinalURL を返す。
type HTTPClient interface {
	// Do は要求を実行する。context の cancel/deadline に必ず応答する。
	Do(ctx context.Context, req HTTPRequest) (*HTTPResponse, error)
}
