package shell

import (
	"errors"
	"testing"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/store"
)

// recorder は適用と巻き戻しの順序を記録する。
type recorder struct {
	events []string
}

// action は指定段階のActionを作る。`changed`と`err`で結果を決める。
func (r *recorder) action(step Step, changed bool, err error) Action {
	return Action{
		Step: step,
		Apply: func() (bool, error) {
			r.events = append(r.events, "apply:"+string(step))
			return changed, err
		},
		Undo: func() error {
			r.events = append(r.events, "undo:"+string(step))
			return nil
		},
	}
}

func TestStepOrderMatchesSpec(t *testing.T) {
	t.Parallel()
	// docs/09-platform.md §7「書込み順はroot/state初期化、shim生成、integration
	// backup、integration変更、再読検証、setup state commit」。**順序そのものが
	// 仕様である。**
	want := []Step{
		StepRootInit, StepShim, StepBackup,
		StepIntegration, StepVerify, StepStateCommit,
	}
	if len(StepOrder) != len(want) {
		t.Fatalf("StepOrder = %v, want %v", StepOrder, want)
	}
	for i := range want {
		if StepOrder[i] != want[i] {
			t.Fatalf("StepOrder = %v, want %v", StepOrder, want)
		}
	}
}

func TestRunTransactionAppliesInOrder(t *testing.T) {
	t.Parallel()
	rec := &recorder{}
	actions := make([]Action, 0, len(StepOrder))
	for _, step := range StepOrder {
		actions = append(actions, rec.action(step, true, nil))
	}
	result, err := RunTransaction(actions)
	if err != nil {
		t.Fatalf("RunTransaction: %v", err)
	}
	if result.RolledBack {
		t.Error("成功したのに巻き戻した")
	}
	for i, step := range StepOrder {
		if rec.events[i] != "apply:"+string(step) {
			t.Fatalf("events = %v, §7の順序どおりでない", rec.events)
		}
		if result.Steps[i].Step != step {
			t.Errorf("Steps[%d] = %q, want %q", i, result.Steps[i].Step, step)
		}
	}
	if !result.AnyChanged() {
		t.Error("すべて変更したのにAnyChangedがfalse")
	}
}

func TestRunTransactionRollsBackInReverseOrder(t *testing.T) {
	t.Parallel()
	rec := &recorder{}
	boom := errors.New("integration failed")
	actions := []Action{
		rec.action(StepRootInit, true, nil),
		rec.action(StepShim, true, nil),
		rec.action(StepBackup, true, nil),
		rec.action(StepIntegration, true, boom),
	}
	result, err := RunTransaction(actions)
	if !errors.Is(err, boom) {
		t.Fatalf("error = %v, 元の失敗を包むべき", err)
	}
	if !result.RolledBack {
		t.Error("RolledBack = false")
	}
	// **巻き戻しは適用の逆順である。** 先に作ったものが後のものの前提に
	// なっているため、前から消すと後のものが宙に浮く。
	want := []string{
		"apply:root-init", "apply:shim", "apply:integration-backup", "apply:integration",
		"undo:integration", "undo:integration-backup", "undo:shim", "undo:root-init",
	}
	if len(rec.events) != len(want) {
		t.Fatalf("events = %v, want %v", rec.events, want)
	}
	for i := range want {
		if rec.events[i] != want[i] {
			t.Fatalf("events = %v, want %v", rec.events, want)
		}
	}
}

func TestRunTransactionUndoesOnlyThisRunsChanges(t *testing.T) {
	t.Parallel()
	rec := &recorder{}
	boom := errors.New("verify failed")
	// shimは既に一致しており何もしなかった（冪等な再実行）。
	actions := []Action{
		rec.action(StepRootInit, true, nil),
		rec.action(StepShim, false, nil),
		rec.action(StepIntegration, true, nil),
		rec.action(StepVerify, false, boom),
	}
	result, err := RunTransaction(actions)
	if !errors.Is(err, boom) {
		t.Fatalf("error = %v, want verify failed", err)
	}
	// **docs/09-platform.md §7「今回作成物だけ逆順rollbackし、既存tool/stateを
	// 削除しない」。** 何もしなかった段階を巻き戻すと、既に正しかった実体を消す。
	for _, event := range rec.events {
		if event == "undo:shim" {
			t.Fatalf("変更していない段階を巻き戻した: %v", rec.events)
		}
		if event == "undo:integration-verify" {
			t.Fatalf("何も作っていない検証段階を巻き戻した: %v", rec.events)
		}
	}
	want := []string{
		"apply:root-init", "apply:shim", "apply:integration", "apply:integration-verify",
		"undo:integration", "undo:root-init",
	}
	if len(rec.events) != len(want) {
		t.Fatalf("events = %v, want %v", rec.events, want)
	}
	for i := range want {
		if rec.events[i] != want[i] {
			t.Fatalf("events = %v, want %v", rec.events, want)
		}
	}
	if result.Changed(StepShim) {
		t.Error("no-opの段階がChangedになっている")
	}
}

func TestRunTransactionUndoesFailedStepItself(t *testing.T) {
	t.Parallel()
	rec := &recorder{}
	boom := errors.New("partial write")
	// 途中まで作ってから失敗した段階（changed=true かつ err非nil）。
	actions := []Action{
		rec.action(StepRootInit, true, nil),
		rec.action(StepShim, true, boom),
	}
	if _, err := RunTransaction(actions); !errors.Is(err, boom) {
		t.Fatalf("error = %v, want partial write", err)
	}
	// **失敗した段階自身も巻き戻す。** 途中まで作っている可能性があり、
	// 残すと次回の実行が既存entryで止まる。
	found := false
	for _, event := range rec.events {
		if event == "undo:shim" {
			found = true
		}
	}
	if !found {
		t.Errorf("失敗した段階を巻き戻していない: %v", rec.events)
	}
}

func TestRunTransactionKeepsOriginalErrorOverRollbackFailure(t *testing.T) {
	t.Parallel()
	boom := errors.New("integration failed")
	undoBoom := errors.New("undo failed")
	actions := []Action{
		{
			Step:  StepRootInit,
			Apply: func() (bool, error) { return true, nil },
			Undo:  func() error { return undoBoom },
		},
		{
			Step:  StepIntegration,
			Apply: func() (bool, error) { return true, boom },
			Undo:  func() error { return undoBoom },
		},
	}
	result, err := RunTransaction(actions)
	// **元の失敗を巻き戻し失敗で上書きしない。** 利用者がまず知りたいのは
	// 「なぜsetupが止まったか」である。
	if !errors.Is(err, boom) {
		t.Fatalf("error = %v, 元の失敗であるべき", err)
	}
	if len(result.RollbackErrors) != 2 {
		t.Fatalf("RollbackErrors = %v, 2件であるべき", result.RollbackErrors)
	}
	for _, failure := range result.RollbackErrors {
		if !errors.Is(failure, ErrRollback) {
			t.Errorf("巻き戻し失敗 = %v, want ErrRollback", failure)
		}
	}
}

func TestRunTransactionContinuesRollbackAfterFailure(t *testing.T) {
	t.Parallel()
	var undone []Step
	makeAction := func(step Step, undoErr error) Action {
		return Action{
			Step:  step,
			Apply: func() (bool, error) { return true, nil },
			Undo:  func() error { undone = append(undone, step); return undoErr },
		}
	}
	actions := []Action{
		makeAction(StepRootInit, nil),
		makeAction(StepShim, errors.New("undo shim failed")),
		{
			Step:  StepIntegration,
			Apply: func() (bool, error) { return true, errors.New("boom") },
		},
	}
	if _, err := RunTransaction(actions); err == nil {
		t.Fatal("失敗したのにerrorが返らない")
	}
	// **1件失敗しても残りを続ける。** 途中で止めると、後続の作成物が
	// 残ったままになる。
	if len(undone) != 2 || undone[0] != StepShim || undone[1] != StepRootInit {
		t.Errorf("undone = %v, shim→root-initの順で全件巻き戻すべき", undone)
	}
}

func TestRunTransactionAllowsSkippedSteps(t *testing.T) {
	t.Parallel()
	rec := &recorder{}
	// `path_integration=none`ではbackup／変更／検証の3段階が無い。
	actions := []Action{
		rec.action(StepRootInit, true, nil),
		rec.action(StepShim, true, nil),
		rec.action(StepStateCommit, true, nil),
	}
	if _, err := RunTransaction(actions); err != nil {
		t.Fatalf("段階を飛ばしたら落ちた: %v", err)
	}
}

func TestRunTransactionRejectsOutOfOrderSteps(t *testing.T) {
	t.Parallel()
	rec := &recorder{}
	cases := map[string][]Action{
		"逆順": {
			rec.action(StepIntegration, true, nil),
			rec.action(StepShim, true, nil),
		},
		"同じ段階を2回": {
			rec.action(StepShim, true, nil),
			rec.action(StepShim, true, nil),
		},
		"未知の段階": {
			{Step: "unknown", Apply: func() (bool, error) { return true, nil }},
		},
		"空": {},
	}
	for name, actions := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			// 並べ替えを許すと、§7の順序が実装のたびに変わりうる。
			if _, err := RunTransaction(actions); !errors.Is(err, ErrStepOrder) {
				t.Fatalf("error = %v, want ErrStepOrder", err)
			}
		})
	}
}

func TestRunTransactionRejectsMissingApply(t *testing.T) {
	t.Parallel()
	actions := []Action{{Step: StepRootInit}}
	if _, err := RunTransaction(actions); err == nil {
		t.Fatal("Applyが無い段階が通った")
	}
}

func TestRunTransactionReportsIdempotentRerun(t *testing.T) {
	t.Parallel()
	rec := &recorder{}
	// docs/09-platform.md §7「setupは冪等とする。setup済みrootへの再実行は
	// 差分だけを適用し、既に一致する項目をno-opとして報告する」。
	actions := make([]Action, 0, len(StepOrder))
	for _, step := range StepOrder {
		actions = append(actions, rec.action(step, false, nil))
	}
	result, err := RunTransaction(actions)
	if err != nil {
		t.Fatalf("RunTransaction: %v", err)
	}
	if result.AnyChanged() {
		t.Error("すべてno-opなのにAnyChangedがtrue")
	}
	for _, event := range rec.events {
		if len(event) > 5 && event[:5] == "undo:" {
			t.Fatalf("成功したのに巻き戻した: %v", rec.events)
		}
	}
}

func TestRestartRequiredFollowsIntegrationChange(t *testing.T) {
	t.Parallel()
	// §16.2「既存terminal/GUI processへ変更が反映されない」。§6は利用者から
	// 見える経路をPATHだけと定め、§8は起動時環境を保持するprocessを挙げる。
	// **integrationを実際に変更したときだけtrue。**
	changed := TransactionResult{Steps: []StepResult{
		{Step: StepShim, Changed: true},
		{Step: StepIntegration, Changed: true},
	}}
	if !RestartRequired(changed) {
		t.Error("integrationを変更したのにfalse")
	}

	// shimだけ作り直してもPATHは変わらない。既存processに不足は生じない。
	shimOnly := TransactionResult{Steps: []StepResult{
		{Step: StepShim, Changed: true},
		{Step: StepIntegration, Changed: false},
	}}
	if RestartRequired(shimOnly) {
		t.Error("PATHが変わらないのにtrue")
	}

	// integration段階自体が無い（path_integration=none）。
	none := TransactionResult{Steps: []StepResult{{Step: StepShim, Changed: true}}}
	if RestartRequired(none) {
		t.Error("integration段階が無いのにtrue")
	}
}

func TestIntegrationWarningsPairApprovalAndNotice(t *testing.T) {
	t.Parallel()
	changed := TransactionResult{Steps: []StepResult{{Step: StepIntegration, Changed: true}}}

	got := IntegrationWarnings(store.PathIntegrationUserPath, changed)
	// **2つは同じ条件で立つが役割が違う。** 片方だけだと、承認を求められずに
	// PATHを変えられるか、変わったことを知らされないままになる。
	if len(got) != 2 {
		t.Fatalf("warnings = %v, 2件であるべき", got)
	}
	var hasModification, hasRestart bool
	for _, code := range got {
		switch code {
		case store.WarnShellModification:
			hasModification = true
		case store.WarnRestartRequired:
			hasRestart = true
		}
	}
	if !hasModification || !hasRestart {
		t.Errorf("warnings = %v, W_SHELL_MODIFICATIONとW_RESTART_REQUIREDの両方が要る", got)
	}

	// §16.2「`W_RESTART_REQUIRED`は情報提供であり承認の対象にしない」。
	// 承認要否の表はinternal/storeが持つ。ここで7件を並べ直すと、表を変えた
	// ときに片方だけが古いままになる。
	approval := store.ApprovalRequiredCodes()
	inApproval := func(want store.PlanWarningCode) bool {
		for _, code := range approval {
			if code == want {
				return true
			}
		}
		return false
	}
	if inApproval(store.WarnRestartRequired) {
		t.Error("W_RESTART_REQUIREDが承認対象になっている")
	}
	if !inApproval(store.WarnShellModification) {
		t.Error("W_SHELL_MODIFICATIONが承認対象になっていない")
	}

	if got := IntegrationWarnings(store.PathIntegrationNone, changed); got != nil {
		t.Errorf("none = %v, 変更する経路が無いのでwarningは出ない", got)
	}
	unchanged := TransactionResult{Steps: []StepResult{{Step: StepIntegration, Changed: false}}}
	if got := IntegrationWarnings(store.PathIntegrationUserPath, unchanged); got != nil {
		t.Errorf("no-op = %v, 何も変えていないのでwarningは出ない", got)
	}
}
