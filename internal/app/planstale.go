package app

import (
	"fmt"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/store"
)

// PlanFreshness はPlanの入力identityが今も一致するかの判定結果である。
type PlanFreshness struct {
	// Stale は不一致があったかである。
	Stale bool
	// ChangedFields は不一致だった`inputs`のfield名である。
	//
	// 診断へ出すために持つ。**値そのものは持たない。** digestとrevisionは
	// 秘密ではないが、片方だけを見せても利用者の判断材料にならず、
	// log/reportへ流れる情報を増やすだけである。
	ChangedFields []string
}

// CheckPlanFreshness はPlanの`inputs`と再取得した実体を突き合わせる。
//
// docs/04-storage-and-data.md §16「Executeは`inputs`の各値を実体から再取得して
// 一致を確認する。lock取得後にも同じ確認を行い、不一致なら`E_PLAN_STALE`とする」。
// docs/03-cli.md §7も「Plan作成後、承認直前とlock取得後に入力のrevision/digestを
// 再検査し、変化時は`E_PLAN_STALE`」と定める。
//
// **現在値は呼出し側が集める。** それぞれのfileを所有するpackage（config／
// registry／catalog／store）が計算した値を受け取る。この関数が自分で集めると、
// Plan作成時と同じ経路を2回通るだけになり、判定が「同じ関数を2度呼んだ結果の
// 比較」に退化する。
//
// 純関数であり、外部作用を持たない。
func CheckPlanFreshness(plan store.Plan, current store.PlanInputs) PlanFreshness {
	planned := plan.Inputs
	// §16の`inputs` 9 fieldをすべて比べる。1つでも落とすと、その入力が変わった
	// Planを新鮮と誤判定する。件数はTestPlanFreshnessComparesEveryInputが固定する。
	comparisons := []struct {
		field string
		equal bool
	}{
		{"root_id", planned.RootID == current.RootID},
		{"config_sha256", planned.ConfigSHA256 == current.ConfigSHA256},
		{"project_sha256", planned.ProjectSHA256 == current.ProjectSHA256},
		{"definition_sha256", planned.DefinitionSHA256 == current.DefinitionSHA256},
		{"catalog_sha256", planned.CatalogSHA256 == current.CatalogSHA256},
		{"registry_sha256", planned.RegistrySHA256 == current.RegistrySHA256},
		{"selections_revision", planned.SelectionsRevision == current.SelectionsRevision},
		{"setup_revision", planned.SetupRevision == current.SetupRevision},
		{"receipt_index_revision", planned.ReceiptIndexRevision == current.ReceiptIndexRevision},
	}

	var changed []string
	for _, comparison := range comparisons {
		if !comparison.equal {
			changed = append(changed, comparison.field)
		}
	}
	return PlanFreshness{Stale: len(changed) > 0, ChangedFields: changed}
}

// PlanInputFieldCount は§16の`inputs`が持つfield数である。
//
// 比較漏れを検出するために持つ。structのfieldが増えたとき、
// [CheckPlanFreshness]の比較表も同時に増やさなければならない。
const PlanInputFieldCount = 9

// StalePlanError は`E_PLAN_STALE`を作る。
//
// docs/03-cli.md §7の終了codeは8である。どのfieldが変わったかをcauseへ入れ、
// 利用者向けmessageには載せない——変わった値の詳細は診断であり、
// 利用者が取る行動（作り直して再実行）は同じである。
func StalePlanError(freshness PlanFreshness) *domain.Error {
	if !freshness.Stale {
		return nil
	}
	return &domain.Error{
		Code:      domain.CodePlanStale,
		MessageID: staleMessageID(),
		// 利用者は同じ行動（作り直して再実行）を取るため、retry可能である。
		Retryable: true,
		Cause:     fmt.Errorf("app: Planの入力が変化した: %v", freshness.ChangedFields),
	}
}

// messagePlanStale は`E_PLAN_STALE`の利用者向けmessage IDである。
const messagePlanStale = "plan.stale"

// staleMessageID は定数のmessage IDをMessageIDへ変換する。
//
// 引数は本package内のconstantだけでparseは失敗しない。失敗した場合もzero値の
// まま返し、error処理の途中でpanicさせない（CLAUDE.md §9）。
func staleMessageID() domain.MessageID {
	value, _ := domain.ParseMessageID(messagePlanStale)
	return value
}
