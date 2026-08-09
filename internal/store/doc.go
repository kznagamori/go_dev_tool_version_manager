// Package store はstate、catalog、receiptの永続化とatomic writeを担う。
//
// docs/02-architecture.md §2 の論理領域「internal/store」に対応する。
//
// 依存範囲: domainとportに依存する。書込みはFileSystem portのAtomicWriteだけを使う。
//
// P1-01時点ではpackage骨格だけであり、実体は後続taskで実装する
// （docs/13-progress.md）。許可するinternal importは
// scripts/ci/check_imports.py の表を正本とし、`policy` jobが検査する。
package store
