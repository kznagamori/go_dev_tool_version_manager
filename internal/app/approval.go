package app

import (
	"errors"
	"fmt"
	"sort"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/store"
)

// ApprovalMode は承認の与えられ方である（docs/02-architecture.md §8）。
//
// 「Approvalは`InteractiveYes|AssumeYes`と承認categoryの集合を持つ一時値で、
// 永続approval databaseを作らない」。
type ApprovalMode string

// ApprovalMode のexactly 2値。
const (
	// ApprovalInteractive は利用者がPlan表示後に対話で答えた承認である。
	ApprovalInteractive ApprovalMode = "interactive-yes"
	// ApprovalAssumed は`--yes`による事前承認である。
	ApprovalAssumed ApprovalMode = "assume-yes"
)

// ApprovalModeCount は§8が定めるmode数である。
const ApprovalModeCount = 2

var approvalModes = map[ApprovalMode]struct{}{
	ApprovalInteractive: {}, ApprovalAssumed: {},
}

// Approval は1 operationぶんの承認である。
//
// **一時値であり永続化しない**（§8）。processをまたいで再利用できる形にすると、
// 過去の承認が別のPlanへ効いてしまう。
type Approval struct {
	// Mode は承認の与えられ方である。
	Mode ApprovalMode
	// Codes は利用者が承認した`PlanWarningCode`の集合である。
	Codes []store.PlanWarningCode
}

// NewApproval はApprovalを組み立てる。
//
// 重複codeを畳み込み、未知codeを拒否する。§16.1の8件に無いcodeを受けると、
// 承認集合の件数が実際に承認した内容と食い違う。
func NewApproval(mode ApprovalMode, codes []store.PlanWarningCode) (Approval, error) {
	if _, ok := approvalModes[mode]; !ok {
		return Approval{}, fmt.Errorf("app: 未知のapproval mode %q", mode)
	}
	seen := make(map[store.PlanWarningCode]struct{}, len(codes))
	unique := make([]store.PlanWarningCode, 0, len(codes))
	for _, code := range codes {
		if !store.IsPlanWarningCode(code) {
			return Approval{}, fmt.Errorf("app: 未知のwarning code %q", code)
		}
		if _, duplicate := seen[code]; duplicate {
			continue
		}
		seen[code] = struct{}{}
		unique = append(unique, code)
	}
	sort.Slice(unique, func(i, j int) bool { return unique[i] < unique[j] })
	return Approval{Mode: mode, Codes: unique}, nil
}

// AssumeYesApproval は`--yes`のApprovalを返す（docs/08-install-runtime.md §4）。
//
// 「`--yes`は§16.1で明示承認が必要な7件すべてを承認できるが、警告表示・結果記録を
// 消さない」。表示と記録は呼出し側の責務であり、ここは承認集合だけを作る。
//
// **`W_RESTART_REQUIRED`を含めない。** §16.1が「情報提供であり承認の対象にしない」
// と定めており、承認集合へ入れると承認対象が8件あるように見える。
func AssumeYesApproval() Approval {
	codes := store.ApprovalRequiredCodes()
	return Approval{Mode: ApprovalAssumed, Codes: codes}
}

// Covers はPlanが要求する承認をこのApprovalが満たすかを返す。
func (a Approval) Covers(plan store.Plan) bool {
	return len(a.missing(plan)) == 0
}

// missing はPlanが要求するのに承認されていないcodeを返す。
func (a Approval) missing(plan store.Plan) []store.PlanWarningCode {
	granted := make(map[store.PlanWarningCode]struct{}, len(a.Codes))
	for _, code := range a.Codes {
		granted[code] = struct{}{}
	}
	var missing []store.PlanWarningCode
	for _, code := range plan.ApprovalCodes() {
		if _, ok := granted[code]; !ok {
			missing = append(missing, code)
		}
	}
	return missing
}

// CheckApproval はPlanの承認要求が満たされているかを検査する（§8手順2）。
//
// docs/04-storage-and-data.md §16「Approvalは`requires_explicit_approval=true`の
// warning `code`集合そのものであり、Executeは同じPlan objectのcode集合を
// Approvalが満たすことを検査する」。§16.1「Approvalが満たさないcodeが1件でも
// あれば`E_APPROVAL_REQUIRED`とする」。
//
// **security failureを承認で回避できない**（§4）。ここが見るのはwarning codeだけで
// あり、checksum mismatch、archive違反、path逸脱はそもそもwarningにならない。
func CheckApproval(plan store.Plan, approval Approval) *domain.Error {
	if _, ok := approvalModes[approval.Mode]; !ok {
		return &domain.Error{
			Code:      domain.CodeApprovalRequired,
			MessageID: approvalMessageID(messageApprovalRequired),
			Cause:     fmt.Errorf("app: approval modeが未設定"),
		}
	}
	missing := approval.missing(plan)
	if len(missing) == 0 {
		return nil
	}
	return &domain.Error{
		Code:      domain.CodeApprovalRequired,
		MessageID: approvalMessageID(messageApprovalRequired),
		// 利用者は承認して再実行できる。
		Retryable: true,
		Cause:     fmt.Errorf("app: 未承認のwarning code: %v", missing),
	}
}

// CheckPlanIdentity はPlanが今のclientとinvocationのものかを検査する（§8手順1）。
//
// 「Plan schema/client/invocationの一致」。schemaは[store.ParsePlan]が読込み時に
// 検査済みのため、ここではclientとinvocationを見る。
//
// **別invocationのPlanを実行しない。** Planは承認と対で意味を持つ一時値であり、
// 前回実行のPlanを今回のapprovalで実行できると、利用者が見た内容と実際の対象が
// 食い違う。
func CheckPlanIdentity(plan store.Plan, build BuildInfo, invocation domain.InvocationID) *domain.Error {
	switch {
	case plan.ClientVersion != build.ClientVersion:
		return planIdentityError(fmt.Errorf(
			"app: Planのclient versionが今のclientと違う（plan=%q client=%q）",
			plan.ClientVersion, build.ClientVersion))
	case invocation.IsZero():
		return planIdentityError(errors.New("app: 現在のinvocation IDが未設定"))
	case plan.Invocation != invocation:
		return planIdentityError(errors.New("app: Planが別のinvocationで作られている"))
	}
	return nil
}

// planIdentityError は§8手順1の不一致を表す。
//
// 利用者から見ると「Planが古い」であり、stale判定と同じ行動（作り直して再実行）を
// 取るため`E_PLAN_STALE`を使う。
func planIdentityError(cause error) *domain.Error {
	return &domain.Error{
		Code:      domain.CodePlanStale,
		MessageID: approvalMessageID(messagePlanStale),
		Retryable: true,
		Cause:     cause,
	}
}

// messageApprovalRequired は`E_APPROVAL_REQUIRED`の利用者向けmessage IDである。
const messageApprovalRequired = "plan.approval_required"

// approvalMessageID は定数のmessage IDをMessageIDへ変換する。
//
// 引数は本package内のconstantだけでparseは失敗しない。失敗した場合もzero値の
// まま返し、error処理の途中でpanicさせない（CLAUDE.md §9）。
func approvalMessageID(id string) domain.MessageID {
	value, _ := domain.ParseMessageID(id)
	return value
}
