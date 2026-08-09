package port

import (
	"errors"
	"fmt"
	"time"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
)

// LogLevel はstructured logのlevelである（docs/04-storage-and-data.md §18）。
type LogLevel string

// LogLevel のexactly 5値。§18で閉じている。
const (
	LevelError LogLevel = "error"
	LevelWarn  LogLevel = "warn"
	LevelInfo  LogLevel = "info"
	LevelDebug LogLevel = "debug"
	LevelTrace LogLevel = "trace"
)

// LogLevelCount は§18が定めるlevel数である。
const LogLevelCount = 5

var logLevels = map[LogLevel]struct{}{
	LevelError: {}, LevelWarn: {}, LevelInfo: {}, LevelDebug: {}, LevelTrace: {},
}

// ParseLogLevel は文字列をLogLevelへ変換する。
func ParseLogLevel(text string) (LogLevel, error) {
	level := LogLevel(text)
	if _, ok := logLevels[level]; !ok {
		return "", fmt.Errorf("port: log level %q は error|warn|info|debug|trace のいずれでもない", text)
	}
	return level, nil
}

// LogSchemaVersion はstructured logのschema revisionである。
//
// docs/04-storage-and-data.md §7が「schema revisionはすべて`1`」と定める。
const LogSchemaVersion = 1

// LogFieldsMax は1 recordのfields上限である（docs/04-storage-and-data.md §18）。
const LogFieldsMax = 64

// LogRecord は1行のstructured logである（docs/04-storage-and-data.md §18）。
//
// 表示文を持たずmessage IDとtyped fieldだけを持つ。docs/02-architecture.md §15が
// 求めるmaskは、この型を作る呼出し側と、実際に書き出すsinkの両方で行う。型として
// maskを強制できないのは、何がsecretかがfieldの意味に依存するためであり、
// 代わりにmask漏れのnegative testを必須とする（docs/10-security.md §12）。
type LogRecord struct {
	// Time はUTCのlog時刻である。
	Time time.Time
	// Level はlogのlevelである。
	Level LogLevel
	// Invocation はCLI呼出しの識別子である。全recordで必須とする。
	Invocation domain.InvocationID
	// Operation は変更transactionの識別子である。transaction外のlogでは未設定を許す。
	Operation domain.OperationID
	// Component はlogを出したcomponent名である（例 `installer`）。
	// docs/04-storage-and-data.md §18は値のgrammarを定めていないため、
	// 空でないことだけを要求する。
	Component string
	// MessageID は表示文を引くkeyである。
	MessageID domain.MessageID
	// Fields はmask済みのscalar fieldである。最大[LogFieldsMax]件。
	Fields domain.Parameters
}

// Validate はsinkへ渡す前の不変条件を検査する。
//
// 誤りは全件返す。log 1行の組立て誤りは同種のfieldで同時に起きるためである。
func (r LogRecord) Validate() error {
	var errs []error

	if r.Time.IsZero() {
		errs = append(errs, errors.New("port: log recordのtimeが未設定"))
	} else if zone, offset := r.Time.Zone(); offset != 0 || zone != "UTC" {
		errs = append(errs, fmt.Errorf("port: log recordのtimeはUTCで持つ（%s%+d が渡された）", zone, offset))
	}
	if _, ok := logLevels[r.Level]; !ok {
		errs = append(errs, fmt.Errorf(
			"port: log level %q は error|warn|info|debug|trace のいずれでもない", r.Level))
	}
	if r.Invocation.IsZero() {
		errs = append(errs, errors.New("port: log recordのinvocation IDが未設定"))
	}
	if r.Component == "" {
		errs = append(errs, errors.New("port: log recordのcomponentが未設定"))
	}
	if r.MessageID.IsZero() {
		errs = append(errs, errors.New("port: log recordのmessage IDが未設定"))
	}
	if err := r.Fields.ValidateWithLimit(LogFieldsMax); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

// Logger は構造化logのportである（docs/02-architecture.md §4.1・§15）。
//
// 専用audit logは持たない。install/use/uninstallの事実は通常log、receipt、
// resultから追跡する（§15）。
type Logger interface {
	// Log は1 recordを記録する。呼出し元をblockしない実装とする。
	Log(LogRecord)
	// Enabled は指定levelが記録対象かを返す。
	//
	// 記録されないlevelのためにfieldの組立てとmaskを行うのは無駄であり、
	// 呼出し側が先に判定できるようにする。
	Enabled(LogLevel) bool
}
