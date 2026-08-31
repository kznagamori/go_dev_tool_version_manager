package app

import (
	"reflect"
	"strings"
	"testing"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/store"
)

// testPlanInputs は全fieldが非zeroの`inputs`を返す。
//
// zero値だと「比較していない」と「一致した」を区別できない。
func testPlanInputs() store.PlanInputs {
	return store.PlanInputs{
		RootID:               strings.Repeat("1", 32),
		ConfigSHA256:         strings.Repeat("2", 64),
		ProjectSHA256:        strings.Repeat("3", 64),
		DefinitionSHA256:     strings.Repeat("4", 64),
		CatalogSHA256:        strings.Repeat("5", 64),
		RegistrySHA256:       strings.Repeat("6", 64),
		SelectionsRevision:   7,
		SetupRevision:        8,
		ReceiptIndexRevision: 9,
	}
}

// TestPlanFreshnessAcceptsUnchangedInputs は一致時にstaleとしないことを固定する。
func TestPlanFreshnessAcceptsUnchangedInputs(t *testing.T) {
	inputs := testPlanInputs()
	plan := store.Plan{Inputs: inputs}
	freshness := CheckPlanFreshness(plan, inputs)
	if freshness.Stale {
		t.Errorf("一致しているのにstale: %v", freshness.ChangedFields)
	}
	if err := StalePlanError(freshness); err != nil {
		t.Errorf("staleでないのにerrorが返った: %v", err)
	}
}

// TestPlanFreshnessComparesEveryInput は§16の`inputs` 9 fieldすべてを比べることを固定する。
//
// **1 fieldずつ変えて、必ずそのfieldが検出されることを見る。** 比較表から
// 1件でも落ちると、その入力が変わったPlanを新鮮と誤判定してExecuteが進む。
func TestPlanFreshnessComparesEveryInput(t *testing.T) {
	tests := []struct {
		field  string
		mutate func(*store.PlanInputs)
	}{
		{"root_id", func(i *store.PlanInputs) { i.RootID = strings.Repeat("a", 32) }},
		{"config_sha256", func(i *store.PlanInputs) { i.ConfigSHA256 = strings.Repeat("a", 64) }},
		{"project_sha256", func(i *store.PlanInputs) { i.ProjectSHA256 = strings.Repeat("a", 64) }},
		{"definition_sha256", func(i *store.PlanInputs) {
			i.DefinitionSHA256 = strings.Repeat("a", 64)
		}},
		{"catalog_sha256", func(i *store.PlanInputs) { i.CatalogSHA256 = strings.Repeat("a", 64) }},
		{"registry_sha256", func(i *store.PlanInputs) { i.RegistrySHA256 = strings.Repeat("a", 64) }},
		{"selections_revision", func(i *store.PlanInputs) { i.SelectionsRevision = 99 }},
		{"setup_revision", func(i *store.PlanInputs) { i.SetupRevision = 99 }},
		{"receipt_index_revision", func(i *store.PlanInputs) { i.ReceiptIndexRevision = 99 }},
	}
	// §16の`inputs`は9 field。structが増えたら比較表とここを同時に増やす。
	if len(tests) != PlanInputFieldCount {
		t.Fatalf("caseが%d件、want %d件", len(tests), PlanInputFieldCount)
	}
	if got := reflect.TypeOf(store.PlanInputs{}).NumField(); got != PlanInputFieldCount {
		t.Fatalf("PlanInputsのfieldが%d件、want %d件。比較表も更新する", got, PlanInputFieldCount)
	}

	for _, test := range tests {
		t.Run(test.field, func(t *testing.T) {
			plan := store.Plan{Inputs: testPlanInputs()}
			current := testPlanInputs()
			test.mutate(&current)

			freshness := CheckPlanFreshness(plan, current)
			if !freshness.Stale {
				t.Fatalf("%s が変わったのにstaleにならない", test.field)
			}
			if len(freshness.ChangedFields) != 1 || freshness.ChangedFields[0] != test.field {
				t.Errorf("ChangedFields = %v, want [%s]", freshness.ChangedFields, test.field)
			}
			err := StalePlanError(freshness)
			if err == nil {
				t.Fatal("staleなのにerrorが返らない")
			}
			if err.Code != domain.CodePlanStale {
				t.Errorf("code = %s, want %s", err.Code, domain.CodePlanStale)
			}
			// docs/03-cli.md §7「8 | 競合 | …`E_PLAN_STALE`…」。
			if err.ExitCode() != 8 {
				t.Errorf("終了code = %d, want 8", err.ExitCode())
			}
			// §14「Causeを公開境界へ出さない」。利用者向けmessageは
			// message IDで引くため、error文字列へfield名を載せない。
			if strings.Contains(err.Error(), test.field) {
				t.Errorf("公開error文字列へ内部詳細が漏れている: %v", err)
			}
			// 詳細はcauseから辿れること。
			if unwrapped := err.Unwrap(); unwrapped == nil ||
				!strings.Contains(unwrapped.Error(), test.field) {
				t.Errorf("causeが変化fieldを持たない: %v", unwrapped)
			}
		})
	}
}

// TestPlanFreshnessReportsEveryChangedField は複数fieldの変化をすべて挙げることを固定する。
func TestPlanFreshnessReportsEveryChangedField(t *testing.T) {
	plan := store.Plan{Inputs: testPlanInputs()}
	current := testPlanInputs()
	current.ConfigSHA256 = strings.Repeat("a", 64)
	current.SetupRevision = 99

	freshness := CheckPlanFreshness(plan, current)
	if !freshness.Stale {
		t.Fatal("2 field変わったのにstaleにならない")
	}
	want := []string{"config_sha256", "setup_revision"}
	if !reflect.DeepEqual(freshness.ChangedFields, want) {
		t.Errorf("ChangedFields = %v, want %v", freshness.ChangedFields, want)
	}
}

// TestPlanFreshnessDetectsEmptyCurrent は現在値が空でも一致扱いしないことを固定する。
//
// 再取得に失敗してzero値が渡ったとき、「一致した」と読むと変化を見逃す。
// 呼出し側が失敗をerrorとして扱うのが本筋だが、ここでも黙って通さない。
func TestPlanFreshnessDetectsEmptyCurrent(t *testing.T) {
	plan := store.Plan{Inputs: testPlanInputs()}
	freshness := CheckPlanFreshness(plan, store.PlanInputs{})
	if !freshness.Stale {
		t.Fatal("現在値が空なのにstaleにならない")
	}
	if len(freshness.ChangedFields) != PlanInputFieldCount {
		t.Errorf("ChangedFields = %d件, want %d件",
			len(freshness.ChangedFields), PlanInputFieldCount)
	}
}
