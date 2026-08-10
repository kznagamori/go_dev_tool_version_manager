package progress

import (
	"errors"
	"testing"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
)

// recordingSink は通知を保持するtest用Sinkである。
type recordingSink struct {
	got []Progress
}

func (s *recordingSink) Report(p Progress) { s.got = append(s.got, p) }

func mustMessageID(t *testing.T, text string) domain.MessageID {
	t.Helper()
	id, err := domain.ParseMessageID(text)
	if err != nil {
		t.Fatalf("ParseMessageID(%q) = %v", text, err)
	}
	return id
}

func basePhaseProgress(t *testing.T, phase Phase, current int64) Progress {
	t.Helper()
	return Progress{
		Phase:      phase,
		Current:    current,
		Unit:       UnitBytes,
		MessageID:  mustMessageID(t, "install.progress"),
		Parameters: domain.Parameters{"tool_id": domain.StringScalar("node")},
	}
}

func TestPhaseSetIsClosed(t *testing.T) {
	if len(phases) != PhaseCount {
		t.Fatalf("phase = %d件, want %d件", len(phases), PhaseCount)
	}
	for _, text := range []string{
		"resolve", "plan", "download", "verify", "extract",
		"probe", "commit", "cleanup", "rollback", "complete",
	} {
		if _, err := ParsePhase(text); err != nil {
			t.Errorf("ParsePhase(%q) = %v, want nil", text, err)
		}
	}
	for _, text := range []string{"", "Resolve", "install", "done", "resolve "} {
		if _, err := ParsePhase(text); err == nil {
			t.Errorf("ParsePhase(%q) = nil, want error", text)
		}
	}
}

func TestUnitSetIsClosed(t *testing.T) {
	if len(units) != UnitCount {
		t.Fatalf("unit = %d件, want %d件", len(units), UnitCount)
	}
	for _, text := range []string{"none", "bytes", "items", "steps"} {
		if _, err := ParseUnit(text); err != nil {
			t.Errorf("ParseUnit(%q) = %v, want nil", text, err)
		}
	}
	for _, text := range []string{"", "Bytes", "byte", "percent"} {
		if _, err := ParseUnit(text); err == nil {
			t.Errorf("ParseUnit(%q) = nil, want error", text)
		}
	}
}

func TestProgressValidate(t *testing.T) {
	total := int64(100)
	negativeTotal := int64(-1)
	over := int64(5)
	rate := -1.0

	tests := []struct {
		name    string
		mutate  func(*Progress)
		wantErr bool
	}{
		{"最小構成", func(*Progress) {}, false},
		{"totalあり", func(p *Progress) { p.Total = &total }, false},
		{"current=total", func(p *Progress) { p.Current = total; p.Total = &total }, false},
		{"phase未設定", func(p *Progress) { p.Phase = "" }, true},
		{"phase範囲外", func(p *Progress) { p.Phase = "install" }, true},
		{"unit未設定", func(p *Progress) { p.Unit = "" }, true},
		{"currentが負", func(p *Progress) { p.Current = -1 }, true},
		{"totalが負", func(p *Progress) { p.Total = &negativeTotal }, true},
		{"currentがtotal超過", func(p *Progress) { p.Current = 10; p.Total = &over }, true},
		{"rateが負", func(p *Progress) { p.Rate = &rate }, true},
		{
			"parameter key違反",
			func(p *Progress) { p.Parameters = domain.Parameters{"Tool-ID": domain.NullScalar()} },
			true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			p := basePhaseProgress(t, PhaseDownload, 1)
			test.mutate(&p)
			err := p.Validate()
			if test.wantErr != (err != nil) {
				t.Errorf("Validate = %v, wantErr = %v", err, test.wantErr)
			}
		})
	}
}

// TestReporterEnforcesMonotonicCurrent はdocs/02-architecture.md §10の
// 「Currentは単調非減少」をphase単位で固定する。
func TestReporterEnforcesMonotonicCurrent(t *testing.T) {
	sink := &recordingSink{}
	reporter := NewReporter(sink)

	reporter.Report(basePhaseProgress(t, PhaseDownload, 10))
	reporter.Report(basePhaseProgress(t, PhaseDownload, 20))
	reporter.Report(basePhaseProgress(t, PhaseDownload, 15)) // 後退 → 破棄
	reporter.Report(basePhaseProgress(t, PhaseDownload, 20)) // 同値 → 通す
	reporter.Report(basePhaseProgress(t, PhaseExtract, 0))   // phaseが変われば0から

	if len(sink.got) != 4 {
		t.Fatalf("通知件数 = %d, want 4: %+v", len(sink.got), sink.got)
	}
	if reporter.Dropped() != 1 {
		t.Errorf("Dropped = %d, want 1", reporter.Dropped())
	}
	wantCurrents := []int64{10, 20, 20, 0}
	for i, want := range wantCurrents {
		if sink.got[i].Current != want {
			t.Errorf("通知%dのcurrent = %d, want %d", i, sink.got[i].Current, want)
		}
	}
}

func TestReporterDropsInvalidProgress(t *testing.T) {
	sink := &recordingSink{}
	reporter := NewReporter(sink)

	invalid := basePhaseProgress(t, PhaseDownload, 1)
	invalid.Unit = "percent"
	reporter.Report(invalid)

	if len(sink.got) != 0 {
		t.Errorf("不正な通知がSinkへ届いた: %+v", sink.got)
	}
	if reporter.Dropped() != 1 {
		t.Errorf("Dropped = %d, want 1", reporter.Dropped())
	}
}

// TestReporterToleratesNil はprogressを要求しない呼出しがnil検査なしで使えることを見る。
func TestReporterToleratesNil(t *testing.T) {
	NewReporter(nil).Report(basePhaseProgress(t, PhaseCommit, 1))

	var nilReporter *Reporter
	nilReporter.Report(basePhaseProgress(t, PhaseCommit, 1))
	if nilReporter.Dropped() != 0 {
		t.Errorf("nil ReporterのDropped = %d, want 0", nilReporter.Dropped())
	}
}

func TestCancelToken(t *testing.T) {
	if IsCancelled(nil) {
		t.Error("nil tokenがcancel済みと判定された")
	}

	done := make(chan struct{})
	token := chanToken(done)
	if IsCancelled(token) {
		t.Error("open tokenがcancel済みと判定された")
	}
	close(done)
	if !IsCancelled(token) {
		t.Error("closed tokenがcancel済みと判定されない")
	}
}

// chanToken はchannelをCancelTokenとして使うtest用の実装である。
type chanToken <-chan struct{}

func (c chanToken) Done() <-chan struct{} { return c }

// TestCancelledError はdocs/02-architecture.md §10の
// 「cancel検出時は`E_CANCELLED`」を固定する。
func TestCancelledError(t *testing.T) {
	err := CancelledError("install")

	if err.Code != domain.CodeCancelled {
		t.Errorf("Code = %s, want %s", err.Code, domain.CodeCancelled)
	}
	if err.ExitCode() != 10 {
		t.Errorf("ExitCode = %d, want 10", err.ExitCode())
	}
	if err.Operation != "install" {
		t.Errorf("Operation = %q, want install", err.Operation)
	}
	if err.MessageID.String() != MessageIDCancelled {
		t.Errorf("MessageID = %q, want %q", err.MessageID, MessageIDCancelled)
	}
	if validateErr := err.Validate(); validateErr != nil {
		t.Errorf("Validate = %v, want nil", validateErr)
	}
	var typed *domain.Error
	if !errors.As(error(err), &typed) {
		t.Error("errors.Asでtyped errorとして取り出せない")
	}
}

func TestResultWarningCodeSetIsClosed(t *testing.T) {
	all := AllResultWarningCodes()
	if len(all) != ResultWarningCodeCount {
		t.Fatalf("AllResultWarningCodes = %d件, want %d件", len(all), ResultWarningCodeCount)
	}
	if len(resultWarningCodes) != ResultWarningCodeCount {
		t.Fatalf("resultWarningCodes = %d件, want %d件", len(resultWarningCodes), ResultWarningCodeCount)
	}
	for _, code := range all {
		if _, err := ParseResultWarningCode(string(code)); err != nil {
			t.Errorf("ParseResultWarningCode(%q) = %v, want nil", code, err)
		}
	}
	// §16.1のPlan warning codeはresult warningではない。両者を混ぜない。
	for _, text := range []string{"", "W_THIRD_PARTY", "W_RESTART_REQUIRED", "w_cache_stale"} {
		if _, err := ParseResultWarningCode(text); err == nil {
			t.Errorf("ParseResultWarningCode(%q) = nil, want error", text)
		}
	}
}

func TestResultWarningValidate(t *testing.T) {
	messageID := mustMessageID(t, "warning.cache_stale")

	good := ResultWarning{Code: WarnCacheStale, MessageID: messageID}
	if err := good.Validate(); err != nil {
		t.Errorf("Validate = %v, want nil", err)
	}

	tests := []struct {
		name    string
		warning ResultWarning
	}{
		{"code未設定", ResultWarning{MessageID: messageID}},
		{"code範囲外", ResultWarning{Code: "W_THIRD_PARTY", MessageID: messageID}},
		{"message ID未設定", ResultWarning{Code: WarnCacheStale}},
		{
			"parameter key違反",
			ResultWarning{
				Code:       WarnCacheStale,
				MessageID:  messageID,
				Parameters: domain.Parameters{"Tool ID": domain.NullScalar()},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.warning.Validate(); err == nil {
				t.Error("Validate = nil, want error")
			}
		})
	}
}
