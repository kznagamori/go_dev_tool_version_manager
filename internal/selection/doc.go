// Package selection はuser/project選択、current link、優先順位を担う。
//
// docs/02-architecture.md §2 の論理領域「internal/selection」に対応する。
//
// 依存範囲: domain、port、storeに依存する。
//
// P1-01時点ではpackage骨格だけであり、実体は後続taskで実装する
// （docs/13-progress.md）。許可するinternal importは
// scripts/ci/check_imports.py の表を正本とし、`policy` jobが検査する。
package selection
