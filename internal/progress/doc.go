// Package progress は型付き進捗、warning、cancel境界を担う。
//
// docs/02-architecture.md §2 の論理領域「internal/progress」に対応する。
//
// 依存範囲: domainだけに依存する。
//
// P1-01時点ではpackage骨格だけであり、実体は後続taskで実装する
// （docs/13-progress.md）。許可するinternal importは
// scripts/ci/check_imports.py の表を正本とし、`policy` jobが検査する。
package progress
