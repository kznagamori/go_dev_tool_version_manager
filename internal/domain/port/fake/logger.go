package fake

import (
	"sync"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain/port"
)

// Logger は記録を保持するfake Loggerである。
//
// productionのsinkと違いblockしないので、docs/02-architecture.md §10が求める
// 「遅いconsumerでblockさせない」性質の検査には使えない。ここで検査するのは
// 何がどのlevelで記録されたか、maskが効いているかである。
type Logger struct {
	mu       sync.Mutex
	records  []port.LogRecord
	minLevel port.LogLevel
}

// levelOrder はlevelの詳細度である。値が大きいほど詳細を表す。
var levelOrder = map[port.LogLevel]int{
	port.LevelError: 0,
	port.LevelWarn:  1,
	port.LevelInfo:  2,
	port.LevelDebug: 3,
	port.LevelTrace: 4,
}

// NewLogger は既定levelがinfoのfake Loggerを作る。
//
// docs/10-security.md §12が「通常logの既定levelはinfo」と定める。
func NewLogger() *Logger {
	return &Logger{minLevel: port.LevelInfo}
}

// SetLevel は記録するlevelの下限を変える。
func (l *Logger) SetLevel(level port.LogLevel) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.minLevel = level
}

// Log はrecordを保持する。Enabledがfalseのlevelは捨てる。
func (l *Logger) Log(record port.LogRecord) {
	if !l.Enabled(record.Level) {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.records = append(l.records, record)
}

// Enabled は指定levelが記録対象かを返す。
func (l *Logger) Enabled(level port.LogLevel) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return levelOrder[level] <= levelOrder[l.minLevel]
}

// Records は記録したrecordのcopyを時系列で返す。
func (l *Logger) Records() []port.LogRecord {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]port.LogRecord(nil), l.records...)
}

var _ port.Logger = (*Logger)(nil)
