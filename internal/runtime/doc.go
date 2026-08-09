// Package runtime は実行環境生成、command解決、子process起動を担う。
//
// docs/02-architecture.md §2 の論理領域「internal/runtime」に対応する。
//
// 依存範囲: domain、port、storeに依存する。process起動はProcessRunner port経由で行う。
//
// P1-01時点ではpackage骨格だけであり、実体は後続taskで実装する
// （docs/13-progress.md）。許可するinternal importは
// scripts/ci/check_imports.py の表を正本とし、`policy` jobが検査する。
package runtime
