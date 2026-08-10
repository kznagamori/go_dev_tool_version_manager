package progress

import (
	"fmt"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
)

// Phase は進捗の段階である（docs/02-architecture.md §10）。
type Phase string

// Phase のexactly 10値。§10で閉じており、未定義値を受理しない。
const (
	PhaseResolve  Phase = "resolve"
	PhasePlan     Phase = "plan"
	PhaseDownload Phase = "download"
	PhaseVerify   Phase = "verify"
	PhaseExtract  Phase = "extract"
	PhaseProbe    Phase = "probe"
	PhaseCommit   Phase = "commit"
	PhaseCleanup  Phase = "cleanup"
	PhaseRollback Phase = "rollback"
	PhaseComplete Phase = "complete"
)

// PhaseCount は§10が定めるphase数である。
const PhaseCount = 10

var phases = map[Phase]struct{}{
	PhaseResolve: {}, PhasePlan: {}, PhaseDownload: {}, PhaseVerify: {},
	PhaseExtract: {}, PhaseProbe: {}, PhaseCommit: {}, PhaseCleanup: {},
	PhaseRollback: {}, PhaseComplete: {},
}

// ParsePhase は文字列をPhaseへ変換する。
func ParsePhase(text string) (Phase, error) {
	phase := Phase(text)
	if _, ok := phases[phase]; !ok {
		return "", fmt.Errorf("progress: phase %q は§10の10値に含まれない", text)
	}
	return phase, nil
}

// Unit は進捗数値の単位である（docs/02-architecture.md §10）。
type Unit string

// Unit のexactly 4値。
const (
	UnitNone  Unit = "none"
	UnitBytes Unit = "bytes"
	UnitItems Unit = "items"
	UnitSteps Unit = "steps"
)

// UnitCount は§10が定めるunit数である。
const UnitCount = 4

var units = map[Unit]struct{}{
	UnitNone: {}, UnitBytes: {}, UnitItems: {}, UnitSteps: {},
}

// ParseUnit は文字列をUnitへ変換する。
func ParseUnit(text string) (Unit, error) {
	unit := Unit(text)
	if _, ok := units[unit]; !ok {
		return "", fmt.Errorf("progress: unit %q は§10の4値に含まれない", text)
	}
	return unit, nil
}

// Progress は1件の進捗通知である（docs/02-architecture.md §10）。
//
// TotalとRateはproviderが値を持たない場合があるためpointerとする。0とunknownを
// 同じ表現にすると、CLIが「0 byte中0 byte完了」を100%として描いてしまう。
type Progress struct {
	OperationID domain.OperationID
	Phase       Phase
	Tool        domain.ToolID
	Version     domain.Version
	Current     int64
	Total       *int64
	Unit        Unit
	Rate        *float64
	MessageID   domain.MessageID
	Parameters  domain.Parameters
}

// Validate は1件の進捗として成立するかを検査する。
//
// 単調非減少の検査は1件だけでは行えないため[Reporter]が担当する。
func (p Progress) Validate() error {
	if _, ok := phases[p.Phase]; !ok {
		return fmt.Errorf("progress: phase %q は§10の10値に含まれない", p.Phase)
	}
	if _, ok := units[p.Unit]; !ok {
		return fmt.Errorf("progress: unit %q は§10の4値に含まれない", p.Unit)
	}
	if p.Current < 0 {
		return fmt.Errorf("progress: currentが負である（%d）", p.Current)
	}
	if p.Total != nil {
		if *p.Total < 0 {
			return fmt.Errorf("progress: totalが負である（%d）", *p.Total)
		}
		if p.Current > *p.Total {
			return fmt.Errorf("progress: current %d がtotal %d を超える", p.Current, *p.Total)
		}
	}
	if p.Rate != nil && *p.Rate < 0 {
		return fmt.Errorf("progress: rateが負である（%v）", *p.Rate)
	}
	return p.Parameters.Validate()
}

// Sink は進捗の受け口である（docs/02-architecture.md §10）。
//
// Reportは呼出し元をblockしてはならない。§10は「遅いconsumerでoperationを
// 無期限blockさせない。最新値coalesceまたは有界bufferをadapterで行う」と定める。
// coalesce/bufferの実装はadapter側の責務であり、この境界では要求として示す。
type Sink interface {
	Report(Progress)
}

// CancelToken はcancel通知である（docs/02-architecture.md §10）。
type CancelToken interface {
	Done() <-chan struct{}
}

// IsCancelled はtokenが既にcancel済みかどうかを即座に返す。
//
// tokenがnilならcancelされていないものとして扱う。cancelを渡さない呼出しを
// 「常にcancel済み」と解釈すると、progressを使わない内部利用が全滅するためである。
func IsCancelled(token CancelToken) bool {
	if token == nil {
		return false
	}
	select {
	case <-token.Done():
		return true
	default:
		return false
	}
}

// CancelledError はcancel検出時に返すtyped errorを作る（§10）。
func CancelledError(operation string) *domain.Error {
	messageID, _ := domain.ParseMessageID(MessageIDCancelled)
	return &domain.Error{
		Code:      domain.CodeCancelled,
		MessageID: messageID,
		Operation: operation,
		Retryable: true,
	}
}

// MessageIDCancelled はcancel時のmessage IDである。
//
// docs/02-architecture.md §10が「cancel検出時は`E_CANCELLED`」と定める変換路を
// 実装するために必要な1件だけを固定する。他のIDはmessage catalogを作るtaskで決める。
const MessageIDCancelled = "error.cancelled"

// Reporter はSinkへの通知に§10の不変条件を課すwrapperである。
//
// Currentの単調非減少はoperation全体にまたがる性質であり、Progress 1件では
// 検査できない。通知の直前に前回値と突き合わせ、違反した通知を捨てる。捨てるのは、
// 進捗表示の不整合のために本来の処理を失敗させるのが割に合わないためである。
// 破棄件数は[Reporter.Dropped]で数えられ、testが検出できる。
//
// Sinkがnilの場合は何もしない。progressを要求しない呼出しのために、
// 呼出し側へnil検査を強いない。
type Reporter struct {
	sink    Sink
	last    map[Phase]int64
	dropped int
}

// NewReporter はSinkを包んだReporterを作る。
func NewReporter(sink Sink) *Reporter {
	return &Reporter{sink: sink, last: make(map[Phase]int64)}
}

// Report は不変条件を満たす通知だけをSinkへ渡す。
//
// 単調性はphaseごとに見る。phaseが変わればcurrentは新しい対象の計数へ切り替わり、
// download 100 byteのあとにextract 0 itemが来るのは正常だからである。
func (r *Reporter) Report(p Progress) {
	if r == nil || r.sink == nil {
		return
	}
	if err := p.Validate(); err != nil {
		r.dropped++
		return
	}
	if previous, seen := r.last[p.Phase]; seen && p.Current < previous {
		r.dropped++
		return
	}
	r.last[p.Phase] = p.Current
	r.sink.Report(p)
}

// Dropped は不変条件違反で捨てた通知の件数を返す。
func (r *Reporter) Dropped() int {
	if r == nil {
		return 0
	}
	return r.dropped
}
