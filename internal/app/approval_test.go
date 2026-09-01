package app

import (
	"strings"
	"testing"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/store"
)

// planWithWarnings は指定codeのwarningを持つPlanを返す。
func planWithWarnings(t *testing.T, codes ...store.PlanWarningCode) store.Plan {
	t.Helper()
	messageID, err := domain.ParseMessageID("plan.third_party")
	if err != nil {
		t.Fatalf("ParseMessageID: %v", err)
	}
	warnings := make([]store.PlanWarning, 0, len(codes))
	for _, code := range codes {
		warnings = append(warnings, store.NewPlanWarning(code, messageID, nil))
	}
	return store.Plan{Kind: store.OperationInstall, Warnings: warnings}
}

// TestAssumeYesApprovalCoversExactlySevenCodes は`--yes`の承認範囲を固定する。
//
// docs/08-install-runtime.md §4「`--yes`は§16.1で明示承認が必要な7件すべてを
// 承認できる」。§16.1「`W_RESTART_REQUIRED`は情報提供であり承認の対象にしない」。
//
// **`W_RESTART_REQUIRED`を含めない。** 含めると承認対象が8件あるように見え、
// 承認単位の件数が§16.1と食い違う。
func TestAssumeYesApprovalCoversExactlySevenCodes(t *testing.T) {
	approval := AssumeYesApproval()
	if approval.Mode != ApprovalAssumed {
		t.Errorf("mode = %s, want %s", approval.Mode, ApprovalAssumed)
	}
	if len(approval.Codes) != store.PlanApprovalCodeCount {
		t.Fatalf("承認code = %d件, want %d件", len(approval.Codes), store.PlanApprovalCodeCount)
	}
	for _, code := range approval.Codes {
		if code == store.WarnRestartRequired {
			t.Error("W_RESTART_REQUIREDが承認集合へ入っている")
		}
	}
	// 承認が必要な7件すべてを持つPlanを満たせること。
	plan := planWithWarnings(t, approval.Codes...)
	if !approval.Covers(plan) {
		t.Error("--yesが7件すべてを承認できていない")
	}
	if err := CheckApproval(plan, approval); err != nil {
		t.Errorf("CheckApproval = %v", err)
	}
}

// TestCheckApprovalRejectsMissingCode は未承認codeがあれば拒否することを固定する。
//
// §16.1「Approvalが満たさないcodeが1件でもあれば`E_APPROVAL_REQUIRED`とする」。
func TestCheckApprovalRejectsMissingCode(t *testing.T) {
	plan := planWithWarnings(t, store.WarnThirdParty, store.WarnEOL)

	// 片方だけ承認する。
	approval, err := NewApproval(ApprovalInteractive,
		[]store.PlanWarningCode{store.WarnThirdParty})
	if err != nil {
		t.Fatalf("NewApproval: %v", err)
	}
	if approval.Covers(plan) {
		t.Error("1件不足でもCoversがtrueになった")
	}
	appErr := CheckApproval(plan, approval)
	if appErr == nil {
		t.Fatal("未承認codeがあるのに通った")
	}
	if appErr.Code != domain.CodeApprovalRequired {
		t.Errorf("code = %s, want %s", appErr.Code, domain.CodeApprovalRequired)
	}
	// docs/03-cli.md §7の終了codeは11である。
	if appErr.ExitCode() != 11 {
		t.Errorf("終了code = %d, want 11", appErr.ExitCode())
	}
	// 不足codeはcauseから辿れること。公開error文字列には出さない（§14）。
	unwrapped := appErr.Unwrap()
	if unwrapped == nil || !strings.Contains(unwrapped.Error(), string(store.WarnEOL)) {
		t.Errorf("causeが不足codeを持たない: %v", unwrapped)
	}
}

// TestCheckApprovalIgnoresRestartRequired は承認不要codeを要求しないことを固定する。
//
// `W_RESTART_REQUIRED`だけを持つPlanは、空のApprovalで通らなければならない。
// 要求してしまうと、情報提供のwarningが承認を強いることになる。
func TestCheckApprovalIgnoresRestartRequired(t *testing.T) {
	plan := planWithWarnings(t, store.WarnRestartRequired)
	approval, err := NewApproval(ApprovalInteractive, nil)
	if err != nil {
		t.Fatalf("NewApproval: %v", err)
	}
	if !approval.Covers(plan) {
		t.Error("W_RESTART_REQUIREDが承認を要求している")
	}
	if appErr := CheckApproval(plan, approval); appErr != nil {
		t.Errorf("CheckApproval = %v", appErr)
	}
}

// TestCheckApprovalAcceptsPlanWithoutWarnings はwarning無しのPlanを固定する。
func TestCheckApprovalAcceptsPlanWithoutWarnings(t *testing.T) {
	plan := planWithWarnings(t)
	approval, err := NewApproval(ApprovalInteractive, nil)
	if err != nil {
		t.Fatalf("NewApproval: %v", err)
	}
	if appErr := CheckApproval(plan, approval); appErr != nil {
		t.Errorf("CheckApproval = %v", appErr)
	}
}

// TestCheckApprovalRejectsUnsetMode はmode未設定を拒否することを固定する。
//
// zero値のApprovalは「承認していない」であり、warning無しのPlanでも通さない。
// 通すと、Approvalを渡し忘れた呼出しがそのまま実行へ進む。
func TestCheckApprovalRejectsUnsetMode(t *testing.T) {
	plan := planWithWarnings(t)
	if appErr := CheckApproval(plan, Approval{}); appErr == nil {
		t.Fatal("mode未設定のApprovalが通った")
	}
}

// TestNewApprovalRejectsUnknownValue は未知mode/codeを拒否することを固定する。
func TestNewApprovalRejectsUnknownValue(t *testing.T) {
	if _, err := NewApproval(ApprovalMode("force"), nil); err == nil {
		t.Error("未知のmodeが通った")
	}
	if _, err := NewApproval(ApprovalInteractive,
		[]store.PlanWarningCode{"W_ANYTHING"}); err == nil {
		t.Error("未知のwarning codeが通った")
	}
	// §16.1の8件はすべて受け付ける。承認不要のcodeを明示的に渡すのは無害である。
	all := append(store.ApprovalRequiredCodes(), store.WarnRestartRequired)
	if _, err := NewApproval(ApprovalAssumed, all); err != nil {
		t.Errorf("§16.1の8件が拒否された: %v", err)
	}
}

// TestNewApprovalFoldsDuplicates は重複codeを畳み込むことを固定する。
func TestNewApprovalFoldsDuplicates(t *testing.T) {
	approval, err := NewApproval(ApprovalInteractive, []store.PlanWarningCode{
		store.WarnEOL, store.WarnThirdParty, store.WarnEOL,
	})
	if err != nil {
		t.Fatalf("NewApproval: %v", err)
	}
	if len(approval.Codes) != 2 {
		t.Fatalf("codes = %v, want 2件", approval.Codes)
	}
	// code順に並ぶ。順序が安定しないと診断とlogが実行ごとに変わる。
	if approval.Codes[0] != store.WarnEOL || approval.Codes[1] != store.WarnThirdParty {
		t.Errorf("codes = %v, want [W_EOL W_THIRD_PARTY]", approval.Codes)
	}
}

// TestApprovalModeCountMatchesSpec はmode数を固定する。
func TestApprovalModeCountMatchesSpec(t *testing.T) {
	if len(approvalModes) != ApprovalModeCount {
		t.Errorf("mode = %d件, want %d件", len(approvalModes), ApprovalModeCount)
	}
}

// TestCheckPlanIdentityRejectsForeignPlan は別client/invocationのPlanを拒否することを固定する。
//
// docs/02-architecture.md §8手順1「Plan schema/client/invocationの一致」。
// **Planは承認と対で意味を持つ一時値である。** 前回実行のPlanを今回のapprovalで
// 実行できると、利用者が見た内容と実際の対象が食い違う。
func TestCheckPlanIdentityRejectsForeignPlan(t *testing.T) {
	invocation, err := domain.ParseInvocationID(strings.Repeat("a", 32))
	if err != nil {
		t.Fatalf("ParseInvocationID: %v", err)
	}
	other, err := domain.ParseInvocationID(strings.Repeat("b", 32))
	if err != nil {
		t.Fatalf("ParseInvocationID: %v", err)
	}
	build := BuildInfo{ClientVersion: "2026.08.31.01"}
	plan := store.Plan{ClientVersion: build.ClientVersion, Invocation: invocation}

	if idErr := CheckPlanIdentity(plan, build, invocation); idErr != nil {
		t.Fatalf("一致しているのに拒否された: %v", idErr)
	}

	tests := []struct {
		name       string
		plan       store.Plan
		build      BuildInfo
		invocation domain.InvocationID
	}{
		{"client versionが違う",
			store.Plan{ClientVersion: "2026.01.01.01", Invocation: invocation},
			build, invocation},
		{"invocationが違う", plan, build, other},
		{"現在のinvocationが未設定", plan, build, domain.InvocationID{}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			idErr := CheckPlanIdentity(test.plan, test.build, test.invocation)
			if idErr == nil {
				t.Fatal("不一致が通った")
			}
			// 利用者の行動はstaleと同じ（作り直して再実行）。
			if idErr.Code != domain.CodePlanStale {
				t.Errorf("code = %s, want %s", idErr.Code, domain.CodePlanStale)
			}
		})
	}
}
