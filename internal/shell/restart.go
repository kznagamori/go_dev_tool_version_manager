package shell

import "github.com/kznagamori/go_dev_tool_version_manager/internal/store"

// RestartRequired はこの実行が既存processへ反映されない変更を行ったかを返す。
//
// docs/04-storage-and-data.md §16.2は`W_RESTART_REQUIRED`を「既存terminal/GUI
// processへ変更が反映されない」と定め、§16は`restart_required`を「trueと
// `W_RESTART_REQUIRED` exactly 1件を同値にする」と定めるが、**どの変更がtrueを
// 導くかは書いていない**。次の3点から一意に決まる。
//
//  1. docs/09-platform.md §6「gdtvm shim directoryはuser PATH/profileの先頭へ
//     1回だけ追加する」——利用者から見える経路はPATHだけである。
//  2. 同§8「VS Code等は起動時環境を保持する。setup/use後は新terminalまたは
//     Reload Windowが必要な場合を表示する」——反映されないのは起動時に環境を
//     読み込むprocessである。
//  3. shimとtool本体はPATH経由で解決され、既存processも次回起動時に新しい実体を
//     見る。PATHそのものが変わらなければ、既存processに不足は生じない。
//
// したがって**integrationを実際に変更したときだけtrue**とする。
//
// `path_integration=none`では変更する経路が無い。冪等な再実行で何も変えなかった
// 場合も、既存processが見ている状態と食い違わないためfalseである（§7「既に
// 一致する項目をno-opとして報告する」）。
func RestartRequired(result TransactionResult) bool {
	return result.Changed(StepIntegration)
}

// IntegrationWarnings はintegration変更に伴うPlan warningを返す。
//
// docs/04-storage-and-data.md §16.1は`W_SHELL_MODIFICATION`を「user PATHまたは
// shell profileの変更・除去」と定め、承認を**必要**とする。§16.2の
// `W_RESTART_REQUIRED`は同じ変更を情報として伝えるもので承認を**要さない**。
//
// **2つは同じ条件で立つが、役割が違う。** 片方だけを出すと、利用者は承認を
// 求められずにPATHを変えられるか、変わったことを知らされないままになる。
func IntegrationWarnings(
	integration store.PathIntegration, result TransactionResult,
) []store.PlanWarningCode {
	if integration == store.PathIntegrationNone || !result.Changed(StepIntegration) {
		return nil
	}
	return []store.PlanWarningCode{
		store.WarnShellModification,
		store.WarnRestartRequired,
	}
}
