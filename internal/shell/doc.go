// Package shell はsetup、profile marker、undoを担う。
//
// docs/02-architecture.md §2 の論理領域「internal/shell」に対応する。
//
// 依存範囲: domain、port、platformに依存する。system環境変数とHKLMを変更しない（§8）。
//
// P1-01時点ではpackage骨格だけであり、実体は後続taskで実装する
// （docs/13-progress.md）。許可するinternal importは
// scripts/ci/check_imports.py の表を正本とし、`policy` jobが検査する。
package shell
