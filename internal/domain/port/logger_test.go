package port

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
)

func mustMessageID(t *testing.T, text string) domain.MessageID {
	t.Helper()
	id, err := domain.ParseMessageID(text)
	if err != nil {
		t.Fatalf("ParseMessageID(%q) = %v", text, err)
	}
	return id
}

func validRecord(t *testing.T) LogRecord {
	t.Helper()
	invocation, err := domain.ParseInvocationID("33333333333333333333333333333333")
	if err != nil {
		t.Fatalf("ParseInvocationID = %v", err)
	}
	operation, err := domain.ParseOperationID("22222222222222222222222222222222")
	if err != nil {
		t.Fatalf("ParseOperationID = %v", err)
	}
	return LogRecord{
		Time:       time.Date(2026, time.August, 7, 9, 0, 0, 0, time.UTC),
		Level:      LevelInfo,
		Invocation: invocation,
		Operation:  operation,
		Component:  "installer",
		MessageID:  mustMessageID(t, "install.started"),
		Fields: domain.Parameters{
			"tool_id": domain.StringScalar("node"),
			"version": domain.StringScalar("22.18.0"),
		},
	}
}

func TestLogLevelSetIsClosed(t *testing.T) {
	if len(logLevels) != LogLevelCount {
		t.Fatalf("log level = %d件, want %d件", len(logLevels), LogLevelCount)
	}
	for _, text := range []string{"error", "warn", "info", "debug", "trace"} {
		if _, err := ParseLogLevel(text); err != nil {
			t.Errorf("ParseLogLevel(%q) = %v, want nil", text, err)
		}
	}
	for _, text := range []string{"", "INFO", "warning", "fatal", "info "} {
		if _, err := ParseLogLevel(text); err == nil {
			t.Errorf("ParseLogLevel(%q) = nil, want error", text)
		}
	}
}

func TestLogSchemaAndLimits(t *testing.T) {
	if LogSchemaVersion != 1 {
		t.Errorf("LogSchemaVersion = %d, want 1", LogSchemaVersion)
	}
	if LogFieldsMax != 64 {
		t.Errorf("LogFieldsMax = %d, want 64", LogFieldsMax)
	}
}

func TestLogRecordValidate(t *testing.T) {
	if err := validRecord(t).Validate(); err != nil {
		t.Fatalf("正常なrecordがValidateで落ちた: %v", err)
	}

	// operation IDはtransaction外のlogで未設定を許す。
	noOperation := validRecord(t)
	noOperation.Operation = domain.OperationID{}
	if err := noOperation.Validate(); err != nil {
		t.Errorf("operation ID未設定がValidateで落ちた: %v", err)
	}

	tests := []struct {
		name    string
		mutate  func(*LogRecord)
		wantSub string
	}{
		{"time未設定", func(r *LogRecord) { r.Time = time.Time{} }, "time"},
		{
			"timeがUTCでない",
			func(r *LogRecord) { r.Time = r.Time.In(time.FixedZone("JST", 9*60*60)) },
			"UTC",
		},
		{"level未設定", func(r *LogRecord) { r.Level = "" }, "log level"},
		{"level範囲外", func(r *LogRecord) { r.Level = "fatal" }, "log level"},
		{"invocation未設定", func(r *LogRecord) { r.Invocation = domain.InvocationID{} }, "invocation"},
		{"component未設定", func(r *LogRecord) { r.Component = "" }, "component"},
		{"message ID未設定", func(r *LogRecord) { r.MessageID = domain.MessageID{} }, "message ID"},
		{
			"field key違反",
			func(r *LogRecord) { r.Fields = domain.Parameters{"Tool-ID": domain.NullScalar()} },
			"parameter key",
		},
		{"field件数超過", func(r *LogRecord) { r.Fields = tooManyFields() }, "最大64件"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			record := validRecord(t)
			test.mutate(&record)
			err := record.Validate()
			if err == nil {
				t.Fatal("Validate = nil, want error")
			}
			if !strings.Contains(err.Error(), test.wantSub) {
				t.Errorf("error %q に %q が含まれない", err, test.wantSub)
			}
		})
	}
}

// TestLogRecordValidateReportsEveryDefect は誤りを1件目で打ち切らないことを見る。
func TestLogRecordValidateReportsEveryDefect(t *testing.T) {
	err := LogRecord{}.Validate()
	if err == nil {
		t.Fatal("Validate = nil, want error")
	}
	for _, want := range []string{"time", "log level", "invocation", "component", "message ID"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error に %q が含まれない:\n%v", want, err)
		}
	}
}

// TestLogFieldsLimitBoundary は上限ちょうどを許し、1件超過を拒むことを見る。
func TestLogFieldsLimitBoundary(t *testing.T) {
	record := validRecord(t)
	record.Fields = fieldsOfSize(LogFieldsMax)
	if err := record.Validate(); err != nil {
		t.Errorf("%d件がValidateで落ちた: %v", LogFieldsMax, err)
	}

	record.Fields = fieldsOfSize(LogFieldsMax + 1)
	if err := record.Validate(); err == nil {
		t.Errorf("%d件がValidateを通った", LogFieldsMax+1)
	}
}

func fieldsOfSize(n int) domain.Parameters {
	fields := make(domain.Parameters, n)
	for i := 0; i < n; i++ {
		fields[fmt.Sprintf("f%d", i)] = domain.IntScalar(int64(i))
	}
	return fields
}

func tooManyFields() domain.Parameters { return fieldsOfSize(LogFieldsMax + 1) }

// TestPortsMissingListsEveryPort は9 portの欠落検出と宣言順を固定する。
func TestPortsMissingListsEveryPort(t *testing.T) {
	missing := Ports{}.Missing()
	want := []string{
		"Clock", "FileSystem", "HTTPClient", "LinkManager", "LockManager",
		"Logger", "ProcessRunner", "Random", "UserLookup",
	}
	if len(missing) != len(want) {
		t.Fatalf("Missing = %v, want %v", missing, want)
	}
	for i, name := range want {
		if missing[i] != name {
			t.Errorf("Missing[%d] = %q, want %q", i, missing[i], name)
		}
	}
}
