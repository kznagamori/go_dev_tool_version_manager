package store

import (
	"strings"
	"testing"
	"time"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain/port"
)

// specLogLine はdocs/04-storage-and-data.md §18の例そのものである。
const specLogLine = `{"schema":1,"time":"2026-08-07T09:00:00Z","level":"info",` +
	`"invocation_id":"33333333333333333333333333333333",` +
	`"operation_id":"22222222222222222222222222222222","component":"installer",` +
	`"message_id":"install.started","fields":{"tool_id":"node","version":"22.18.0"}}`

func mustInvocationID(t *testing.T, hex string) domain.InvocationID {
	t.Helper()
	value, err := domain.ParseInvocationID(hex)
	if err != nil {
		t.Fatalf("ParseInvocationID(%q) = %v", hex, err)
	}
	return value
}

func mustMessageID(t *testing.T, text string) domain.MessageID {
	t.Helper()
	value, err := domain.ParseMessageID(text)
	if err != nil {
		t.Fatalf("ParseMessageID(%q) = %v", text, err)
	}
	return value
}

func sampleRecord(t *testing.T) port.LogRecord {
	t.Helper()
	operation, err := domain.ParseOperationID("22222222222222222222222222222222")
	if err != nil {
		t.Fatalf("ParseOperationID = %v", err)
	}
	return port.LogRecord{
		Time:       time.Date(2026, 8, 7, 9, 0, 0, 0, time.UTC),
		Level:      port.LevelInfo,
		Invocation: mustInvocationID(t, "33333333333333333333333333333333"),
		Operation:  operation,
		Component:  "installer",
		MessageID:  mustMessageID(t, "install.started"),
		Fields: domain.Parameters{
			"tool_id": domain.StringScalar("node"),
			"version": domain.StringScalar("22.18.0"),
		},
	}
}

func TestDecodeLogLineAcceptsSpecExample(t *testing.T) {
	record, err := DecodeLogLine([]byte(specLogLine))
	if err != nil {
		t.Fatalf("DecodeLogLine = %v", err)
	}
	if record.Level != port.LevelInfo || record.Component != "installer" {
		t.Errorf("level/component = %q/%q", record.Level, record.Component)
	}
	if record.MessageID.String() != "install.started" {
		t.Errorf("message_id = %q", record.MessageID)
	}
	if len(record.Fields) != 2 {
		t.Fatalf("fields件数 = %d", len(record.Fields))
	}
	tool, ok := record.Fields["tool_id"].Str()
	if !ok || tool != "node" {
		t.Errorf("fields[tool_id] = %q", tool)
	}
}

func TestLogLineRoundTrip(t *testing.T) {
	record := sampleRecord(t)
	data, encodeErr := EncodeLogLine(record)
	if encodeErr != nil {
		t.Fatalf("EncodeLogLine = %v", encodeErr)
	}
	again, decodeErr := DecodeLogLine(data)
	if decodeErr != nil {
		t.Fatalf("DecodeLogLine = %v\n%s", decodeErr, data)
	}
	if !again.Time.Equal(record.Time) || again.Level != record.Level {
		t.Errorf("time/level = %v/%q", again.Time, again.Level)
	}
	if again.Invocation != record.Invocation || again.Operation != record.Operation {
		t.Error("IDがround tripで変わった")
	}
	if len(again.Fields) != len(record.Fields) {
		t.Errorf("fields件数 = %d, want %d", len(again.Fields), len(record.Fields))
	}
	// JSON Linesは1行である。行内にLFがあると1 recordが2行に割れる。
	if strings.Count(string(data), "\n") != 1 || data[len(data)-1] != '\n' {
		t.Errorf("1行でない: %q", data)
	}
}

// TestEncodeLogLineOmitsOperationIDWhenAbsent は読取りcommandのlogを固定する。
//
// operation_idは変更transactionだけが持つ。keyを落とすとexact key集合から
// 外れるため、空文字列で出す。
func TestEncodeLogLineOmitsOperationIDWhenAbsent(t *testing.T) {
	record := sampleRecord(t)
	record.Operation = domain.OperationID{}
	data, err := EncodeLogLine(record)
	if err != nil {
		t.Fatalf("EncodeLogLine = %v", err)
	}
	if !strings.Contains(string(data), `"operation_id":""`) {
		t.Errorf("operation_idが空文字列で出ていない: %s", data)
	}
	again, decodeErr := DecodeLogLine(data)
	if decodeErr != nil {
		t.Fatalf("DecodeLogLine = %v", decodeErr)
	}
	if !again.Operation.IsZero() {
		t.Error("operation_idが復元で非zeroになった")
	}
}

// TestEncodeLogLineRejects は§18のkey集合とscalar制約を固定する。
func TestEncodeLogLineRejects(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*port.LogRecord)
	}{
		{"level未設定", func(r *port.LogRecord) { r.Level = "" }},
		{"level enum外", func(r *port.LogRecord) { r.Level = "fatal" }},
		{"time未設定", func(r *port.LogRecord) { r.Time = time.Time{} }},
		{"timeが非UTC", func(r *port.LogRecord) {
			r.Time = r.Time.In(time.FixedZone("JST", 9*3600))
		}},
		{"invocation未設定", func(r *port.LogRecord) { r.Invocation = domain.InvocationID{} }},
		{"component空", func(r *port.LogRecord) { r.Component = "" }},
		{"message_id未設定", func(r *port.LogRecord) { r.MessageID = domain.MessageID{} }},
		{"field keyがgrammar外", func(r *port.LogRecord) {
			r.Fields = domain.Parameters{"toolId": domain.StringScalar("node")}
		}},
		{"fields件数超過", func(r *port.LogRecord) {
			fields := make(domain.Parameters, port.LogFieldsMax+1)
			for index := 0; index <= port.LogFieldsMax; index++ {
				fields[fieldKey(index)] = domain.StringScalar("x")
			}
			r.Fields = fields
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			record := sampleRecord(t)
			test.mutate(&record)
			if _, err := EncodeLogLine(record); err == nil {
				t.Error("EncodeLogLine = nil, want error")
			}
		})
	}
}

// fieldKey は§7のscalar parameter key grammarに合うkeyを作る。
func fieldKey(index int) string {
	letters := "abcdefghijklmnopqrstuvwxyz"
	return "k" + string(letters[index/26%26]) + string(letters[index%26]) + "_" +
		string(letters[index/676%26])
}

// TestDecodeLogLineRejects は§18のstrict decodeを固定する。
func TestDecodeLogLineRejects(t *testing.T) {
	tests := []struct {
		name string
		line string
	}{
		{"unknown key", strings.Replace(specLogLine, `"schema":1`, `"schema":1,"extra":1`, 1)},
		{"重複key", strings.Replace(specLogLine, `"level":"info"`, `"level":"info","level":"warn"`, 1)},
		{"schemaが2", strings.Replace(specLogLine, `"schema":1`, `"schema":2`, 1)},
		{"level enum外", strings.Replace(specLogLine, `"level":"info"`, `"level":"fatal"`, 1)},
		{"time非UTC", strings.Replace(specLogLine,
			`"time":"2026-08-07T09:00:00Z"`, `"time":"2026-08-07T09:00:00+09:00"`, 1)},
		{"invocation_id不正", strings.Replace(specLogLine,
			"33333333333333333333333333333333", "3333", 1)},
		{"message_idがsegment 1件", strings.Replace(specLogLine,
			`"message_id":"install.started"`, `"message_id":"started"`, 1)},
		{"component空", strings.Replace(specLogLine, `"component":"installer"`, `"component":""`, 1)},
		{"fieldsがnested object", strings.Replace(specLogLine,
			`"fields":{"tool_id":"node","version":"22.18.0"}`, `"fields":{"tool_id":{"a":1}}`, 1)},
		{"fieldsがarray値", strings.Replace(specLogLine,
			`"fields":{"tool_id":"node","version":"22.18.0"}`, `"fields":{"tool_id":[1,2]}`, 1)},
		{"fieldsが小数", strings.Replace(specLogLine,
			`"fields":{"tool_id":"node","version":"22.18.0"}`, `"fields":{"size":1.5}`, 1)},
		{"field keyがgrammar外", strings.Replace(specLogLine,
			`"tool_id":"node"`, `"toolId":"node"`, 1)},
		{"trailing data", specLogLine + specLogLine},
		{"BOM付き", "\ufeff" + specLogLine},
		{"不正UTF-8", specLogLine[:20] + "\xff" + specLogLine[20:]},
		{"空", ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := DecodeLogLine([]byte(test.line)); err == nil {
				t.Error("DecodeLogLine = nil, want error")
			}
		})
	}
}

// TestLogLineScalarKinds は§7のscalar 4種が往復することを固定する。
func TestLogLineScalarKinds(t *testing.T) {
	record := sampleRecord(t)
	record.Fields = domain.Parameters{
		"text":   domain.StringScalar("value"),
		"flag":   domain.BoolScalar(true),
		"count":  domain.IntScalar(42),
		"absent": domain.NullScalar(),
	}
	data, encodeErr := EncodeLogLine(record)
	if encodeErr != nil {
		t.Fatalf("EncodeLogLine = %v", encodeErr)
	}
	again, decodeErr := DecodeLogLine(data)
	if decodeErr != nil {
		t.Fatalf("DecodeLogLine = %v\n%s", decodeErr, data)
	}
	if value, ok := again.Fields["count"].Int(); !ok || value != 42 {
		t.Errorf("count = %d ok=%v", value, ok)
	}
	if value, ok := again.Fields["flag"].Bool(); !ok || !value {
		t.Errorf("flag = %v ok=%v", value, ok)
	}
	if !again.Fields["absent"].IsNull() {
		t.Error("nullが復元されていない")
	}
}

// TestLogLineRejectsUnsafeInteger は§7の2^53-1境界を固定する。
func TestLogLineRejectsUnsafeInteger(t *testing.T) {
	record := sampleRecord(t)
	record.Fields = domain.Parameters{"count": domain.IntScalar(JSONMaxSafeInteger + 1)}
	if _, err := EncodeLogLine(record); err == nil {
		t.Error("2^53を超えるintegerが通った")
	}
	record.Fields = domain.Parameters{"count": domain.IntScalar(JSONMaxSafeInteger)}
	if _, err := EncodeLogLine(record); err != nil {
		t.Errorf("2^53-1が落ちた: %v", err)
	}
}

// TestLogLineSizeLimit は§21のlog 1行256 KiB上限を固定する。
func TestLogLineSizeLimit(t *testing.T) {
	record := sampleRecord(t)
	record.Fields = domain.Parameters{"payload": domain.StringScalar(strings.Repeat("x", LogLineMaxBytes))}
	if _, err := EncodeLogLine(record); err == nil {
		t.Error("256 KiB超過が通った")
	}
	if _, err := DecodeLogLine([]byte(strings.Repeat("x", LogLineMaxBytes+1))); err == nil {
		t.Error("読込み側の上限超過が通った")
	}
}
