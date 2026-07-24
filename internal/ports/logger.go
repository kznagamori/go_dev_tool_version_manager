package ports

// LogLevel はログ出力レベルである（05章3.6節）。
type LogLevel string

// LogLevel の値（重大度の高い順）。
const (
	LevelError LogLevel = "error"
	LevelWarn  LogLevel = "warn"
	LevelInfo  LogLevel = "info"
	LevelDebug LogLevel = "debug"
	LevelTrace LogLevel = "trace"
)

// Field は構造化ログの key-value である。Value に秘密値が渡っても Logger 実装が
// mask 対象 key/値を伏せる（11章12節）。
type Field struct {
	Key   string
	Value any
}

// Logger は構造化レベル、operation ID、秘密値マスクを扱う抽象 port である（02章5節,
// 11章12節）。With で文脈フィールド（operation_id 等）を固定した子 logger を得る。
type Logger interface {
	// Log は指定レベルで message と fields を出力する。
	Log(level LogLevel, msg string, fields ...Field)
	// With は fields を引き継いだ子 logger を返す。
	With(fields ...Field) Logger
}
