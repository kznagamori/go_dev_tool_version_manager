package fake

import (
	"errors"
	"testing"
	"time"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain/port"
)

func testRecord(t *testing.T, level port.LogLevel) port.LogRecord {
	t.Helper()
	invocation, err := domain.ParseInvocationID("33333333333333333333333333333333")
	if err != nil {
		t.Fatalf("ParseInvocationID = %v", err)
	}
	messageID, err := domain.ParseMessageID("install.started")
	if err != nil {
		t.Fatalf("ParseMessageID = %v", err)
	}
	return port.LogRecord{
		Time:       DefaultNow(),
		Level:      level,
		Invocation: invocation,
		Component:  "installer",
		MessageID:  messageID,
	}
}

// TestFakeLoggerDefaultLevelIsInfo はdocs/10-security.md §12の
// 「通常logの既定levelはinfo」を固定する。
func TestFakeLoggerDefaultLevelIsInfo(t *testing.T) {
	logger := NewLogger()

	for _, level := range []port.LogLevel{port.LevelError, port.LevelWarn, port.LevelInfo} {
		if !logger.Enabled(level) {
			t.Errorf("既定levelで %s がEnabled=false", level)
		}
	}
	for _, level := range []port.LogLevel{port.LevelDebug, port.LevelTrace} {
		if logger.Enabled(level) {
			t.Errorf("既定levelで %s がEnabled=true", level)
		}
	}
}

func TestFakeLoggerDropsDisabledLevels(t *testing.T) {
	logger := NewLogger()

	logger.Log(testRecord(t, port.LevelInfo))
	logger.Log(testRecord(t, port.LevelDebug))
	if got := logger.Records(); len(got) != 1 || got[0].Level != port.LevelInfo {
		t.Fatalf("Records = %+v, want infoの1件", got)
	}

	logger.SetLevel(port.LevelTrace)
	logger.Log(testRecord(t, port.LevelDebug))
	logger.Log(testRecord(t, port.LevelTrace))
	if got := logger.Records(); len(got) != 3 {
		t.Fatalf("Records = %d件, want 3件", len(got))
	}
}

// TestFakeLoggerRecordsAreCopies は取得後の書換えが内部へ伝わらないことを見る。
func TestFakeLoggerRecordsAreCopies(t *testing.T) {
	logger := NewLogger()
	logger.Log(testRecord(t, port.LevelInfo))

	got := logger.Records()
	got[0].Component = "書き換えた"
	if again := logger.Records(); again[0].Component != "installer" {
		t.Errorf("Records()の書換えが内部へ伝わった: %q", again[0].Component)
	}
}

// TestFakeRandomIsDeterministic はgolden比較のための連番を固定する。
func TestFakeRandomIsDeterministic(t *testing.T) {
	random := NewRandom(NewInjector())

	want := []string{
		"00000000000000000000000000000001",
		"00000000000000000000000000000002",
		"00000000000000000000000000000003",
	}
	for i, expected := range want {
		raw, err := random.NewIDBytes()
		if err != nil {
			t.Fatalf("%d回目のNewIDBytes = %v", i+1, err)
		}
		if got := domain.NewOperationID(raw).String(); got != expected {
			t.Errorf("%d回目 = %q, want %q", i+1, got, expected)
		}
	}

	// 別instanceは独立した連番を持つ。package global stateを持たないことの確認。
	other := NewRandom(NewInjector())
	raw, err := other.NewIDBytes()
	if err != nil {
		t.Fatalf("NewIDBytes = %v", err)
	}
	if got := domain.NewInvocationID(raw).String(); got != want[0] {
		t.Errorf("別instanceの1回目 = %q, want %q", got, want[0])
	}
}

func TestFakeRandomFailureInjection(t *testing.T) {
	injector := NewInjector()
	random := NewRandom(injector)
	sentinel := errors.New("entropy source unavailable")
	injector.FailOnce(OpNewIDBytes, sentinel)

	if _, err := random.NewIDBytes(); !errors.Is(err, sentinel) {
		t.Fatalf("1回目 = %v, want %v", err, sentinel)
	}
	raw, err := random.NewIDBytes()
	if err != nil {
		t.Fatalf("2回目 = %v, want nil", err)
	}
	// 失敗した呼出しは連番を進めない。IDの飛びをgolden比較で検出できるようにする。
	if got := domain.NewOperationID(raw).String(); got != "00000000000000000000000000000001" {
		t.Errorf("2回目のID = %q, want 1", got)
	}
}

// TestSetProvidesEveryPort はNewSetが8 portすべてを埋めることを固定する。
func TestSetProvidesEveryPort(t *testing.T) {
	ports := NewSet().Ports()
	if missing := ports.Missing(); len(missing) != 0 {
		t.Fatalf("NewSet().Ports()に欠落がある: %v", missing)
	}
	if got := NewSet().Clock.Now(); !got.Equal(DefaultNow()) {
		t.Errorf("Clock起点 = %v, want %v", got, DefaultNow())
	}
	if DefaultNow().Location() != time.UTC {
		t.Error("DefaultNowがUTCでない")
	}
}
