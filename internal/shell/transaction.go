package shell

import (
	"errors"
	"fmt"
)

// Step はsetupの書込み段階である。
//
// docs/09-platform.md §7「書込み順はroot/state初期化、shim生成、integration
// backup、integration変更、再読検証、setup state commit」。**順序そのものが
// 仕様である**ため、値と並びを表で固定する。
type Step string

// Step の値。§7の書込み順に並べる。
const (
	// StepRootInit はroot/stateの初期化である。
	StepRootInit Step = "root-init"
	// StepShim はshim生成である。
	StepShim Step = "shim"
	// StepBackup はintegration backupの取得である。
	StepBackup Step = "integration-backup"
	// StepIntegration はintegration（user PATH／shell profile）の変更である。
	StepIntegration Step = "integration"
	// StepVerify は変更後の再読検証である。
	StepVerify Step = "integration-verify"
	// StepStateCommit はsetup stateのcommitである。
	StepStateCommit Step = "state-commit"
)

// StepOrder は§7が定める書込み順である。
//
// 実行側はこの並びに一致することをtestで固定する。並びを実行時に組み立てると、
// 段階を足したときに順序の意図が失われる。
var StepOrder = []Step{
	StepRootInit,
	StepShim,
	StepBackup,
	StepIntegration,
	StepVerify,
	StepStateCommit,
}

// transactionのsentinel error。
var (
	// ErrStepOrder は段階が§7の順序に従っていないことを表す。
	ErrStepOrder = errors.New("shell: 書込み段階が仕様の順序に従っていない")
	// ErrRollback は巻き戻し自体が失敗したことを表す。
	ErrRollback = errors.New("shell: 巻き戻しに失敗した")
)

// Action は1段階の適用と巻き戻しである。
//
// **巻き戻しの対象は「今回作成したもの」だけである**（docs/09-platform.md §7
// 「途中失敗は今回作成物だけ逆順rollbackし、既存tool/stateを削除しない」）。
// `Apply`が`changed=false`を返した段階は何も作っていないため巻き戻さない。
type Action struct {
	// Step はこの操作が属する書込み段階である。
	Step Step
	// Apply は段階を適用する。
	//
	// `changed`は**この実行で実際に変更したか**である。既に一致していて何も
	// しなかった場合はfalseを返す（§7「既に一致する項目をno-opとして報告する」）。
	Apply func() (changed bool, err error)
	// Undo は`Apply`が作ったものを取り除く。nilなら巻き戻し不要である。
	//
	// 既存の実体を消す処理をここへ書かない。**巻き戻しで既存tool/stateを
	// 削除しない**（§7）。
	Undo func() error
}

// StepResult は1段階の結果である。
type StepResult struct {
	// Step は書込み段階である。
	Step Step
	// Changed はこの実行で実際に変更したかである。
	Changed bool
}

// TransactionResult はtransaction全体の結果である。
type TransactionResult struct {
	// Steps は実行した段階の結果である。§7の順序で並ぶ。
	Steps []StepResult
	// RolledBack は失敗して巻き戻したかである。
	RolledBack bool
	// RollbackErrors は巻き戻し中に起きた失敗である。
	//
	// **元の失敗を上書きしない。** 巻き戻しが失敗しても、利用者が知りたいのは
	// まず「なぜsetupが止まったか」である。
	RollbackErrors []error
}

// Changed は指定段階がこの実行で変更を行ったかを返す。
func (r TransactionResult) Changed(step Step) bool {
	for _, result := range r.Steps {
		if result.Step == step {
			return result.Changed
		}
	}
	return false
}

// AnyChanged はいずれかの段階が変更を行ったかを返す。
//
// §7「setupは冪等とする。setup済みrootへの再実行は差分だけを適用し、既に
// 一致する項目をno-opとして報告する」。すべてno-opなら再実行は何も変えていない。
func (r TransactionResult) AnyChanged() bool {
	for _, result := range r.Steps {
		if result.Changed {
			return true
		}
	}
	return false
}

// RunTransaction は段階を順に適用し、失敗したら今回作成物だけを逆順で巻き戻す。
//
// docs/09-platform.md §7の書込み順と巻き戻し規則をここで満たす。
//
// **journalを書かない。** [15-deferred.md](../../docs/15-deferred.md) D-08が
// operation journalを延期し、v0.1の代替を「staging＋atomic renameとindex
// 再構築」「operation tmpのdirectory単位削除」と定める。巻き戻しは同一process
// 内のundo listで行い、processごと落ちた場合の回復は§7の**冪等な再実行**が担う。
//
// 巻き戻しの順序は適用の逆である。先に作ったものが後のものの前提になっている
// ため、前から消すと後のものが宙に浮く。
func RunTransaction(actions []Action) (TransactionResult, error) {
	if err := checkStepOrder(actions); err != nil {
		return TransactionResult{}, err
	}
	result := TransactionResult{Steps: make([]StepResult, 0, len(actions))}
	// 巻き戻し対象は**この実行で変更した段階だけ**である。
	undo := make([]Action, 0, len(actions))

	for _, action := range actions {
		if action.Apply == nil {
			return result, fmt.Errorf("shell: 段階%sにApplyが無い", action.Step)
		}
		changed, err := action.Apply()
		if changed {
			// 変更したものだけをundo listへ積む。失敗した段階自身も、
			// 途中まで作っている可能性があるため積む。
			undo = append(undo, action)
		}
		result.Steps = append(result.Steps, StepResult{Step: action.Step, Changed: changed})
		if err != nil {
			result.RolledBack = true
			result.RollbackErrors = rollback(undo)
			return result, fmt.Errorf("shell: 段階%sで失敗した: %w", action.Step, err)
		}
	}
	return result, nil
}

// rollback はundo listを逆順に実行し、起きた失敗を集める。
//
// **1件失敗しても残りを続ける。** 途中で止めると、後続の作成物が残ったままに
// なる。集めた失敗は呼出し側が報告する。
func rollback(undo []Action) []error {
	var failures []error
	for i := len(undo) - 1; i >= 0; i-- {
		action := undo[i]
		if action.Undo == nil {
			continue
		}
		if err := action.Undo(); err != nil {
			failures = append(failures, fmt.Errorf(
				"%w: 段階%s: %w", ErrRollback, action.Step, err))
		}
	}
	return failures
}

// checkStepOrder は段階が§7の順序に従っていることを確かめる。
//
// 段階を飛ばすことは許すが、**並べ替えは許さない**。integrationを行わない
// setup（`path_integration=none`）ではbackup／変更／検証の3段階が無いためである。
func checkStepOrder(actions []Action) error {
	if len(actions) == 0 {
		return fmt.Errorf("%w: 段階が1つも無い", ErrStepOrder)
	}
	position := make(map[Step]int, len(StepOrder))
	for i, step := range StepOrder {
		position[step] = i
	}
	previous := -1
	for _, action := range actions {
		index, known := position[action.Step]
		if !known {
			return fmt.Errorf("%w: 未知の段階%s", ErrStepOrder, action.Step)
		}
		if index <= previous {
			return fmt.Errorf(
				"%w: %sが順序%vに反する", ErrStepOrder, action.Step, StepOrder)
		}
		previous = index
	}
	return nil
}
