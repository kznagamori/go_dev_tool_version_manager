package infra

import (
	"encoding/json"
	"io"
	"sync"
	"time"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/ports"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/security"
)

// levelRank は log level の重大度順位である（小さいほど重大）。
var levelRank = map[ports.LogLevel]int{
	ports.LevelError: 0,
	ports.LevelWarn:  1,
	ports.LevelInfo:  2,
	ports.LevelDebug: 3,
	ports.LevelTrace: 4,
}

// structuredLogger は UTC ISO 8601 時刻・level・structured fields を 1 行 JSON で出力する
// ports.Logger の既定実装である（13章13節）。field key が secret を示す場合は値を伏せる。
type structuredLogger struct {
	w      io.Writer
	level  ports.LogLevel
	fields []ports.Field
	clock  ports.Clock
	mu     *sync.Mutex
}

// NewLogger は w へ構造化ログを出力する Logger を返す。level 未満（より詳細）のログは
// 抑制する。writer は複数の子 logger 間で mutex により直列化する。
func NewLogger(w io.Writer, level ports.LogLevel, clock ports.Clock) ports.Logger {
	return &structuredLogger{w: w, level: level, clock: clock, mu: &sync.Mutex{}}
}

// With は fields を引き継いだ子 logger を返す。writer・level・clock・mutex を共有する。
func (l *structuredLogger) With(fields ...ports.Field) ports.Logger {
	merged := make([]ports.Field, 0, len(l.fields)+len(fields))
	merged = append(merged, l.fields...)
	merged = append(merged, fields...)
	return &structuredLogger{w: l.w, level: l.level, fields: merged, clock: l.clock, mu: l.mu}
}

// enabled は指定 level が現在の出力閾値で有効かを返す。
func (l *structuredLogger) enabled(level ports.LogLevel) bool {
	lr, ok := levelRank[level]
	if !ok {
		return false
	}
	cr, ok := levelRank[l.level]
	if !ok {
		cr = levelRank[ports.LevelInfo]
	}
	return lr <= cr
}

// Log は level が有効なら 1 行 JSON を出力する。secret を示す field key の値は伏せる。
func (l *structuredLogger) Log(level ports.LogLevel, msg string, fields ...ports.Field) {
	if !l.enabled(level) {
		return
	}
	record := make(map[string]any, len(l.fields)+len(fields)+3)
	for _, f := range l.fields {
		record[f.Key] = maskFieldValue(f.Key, f.Value)
	}
	for _, f := range fields {
		record[f.Key] = maskFieldValue(f.Key, f.Value)
	}
	// 予約 key は field で上書きされないよう最後に設定する。
	record["time"] = l.clock.Now().UTC().Format(time.RFC3339Nano)
	record["level"] = string(level)
	record["msg"] = msg

	data, err := json.Marshal(record)
	if err != nil {
		data = []byte(`{"level":"` + string(level) + `","msg":"<unmarshalable log record>"}`)
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	_, _ = l.w.Write(append(data, '\n'))
}

// maskFieldValue は field key が secret を示す場合に値を Redacted へ置換する（11章12節）。
// URL 等 value 内の secret は呼び出し側が security.MaskURL で処理する。
func maskFieldValue(key string, value any) any {
	if security.IsSensitiveEnvKey(key) || security.IsSensitiveHeader(key) {
		return security.Redacted
	}
	return value
}
