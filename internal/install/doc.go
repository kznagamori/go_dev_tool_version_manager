// Package install はdownload、検証、安全展開、probe、receipt、transactionを担う。
//
// docs/02-architecture.md §2 の論理領域「internal/install」に対応する。
//
// 依存範囲: domain、port、security、definitionに依存する。外部作用はすべてport経由にする。
//
// P1-01時点ではpackage骨格だけであり、実体は後続taskで実装する
// （docs/13-progress.md）。許可するinternal importは
// scripts/ci/check_imports.py の表を正本とし、`policy` jobが検査する。
package install
