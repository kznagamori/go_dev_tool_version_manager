// Package domain はToolID、Version、Platform、Plan、Selection、Receipt、Errorのdomain値と規則を担う。
//
// docs/02-architecture.md §2 の論理領域「internal/domain」に対応する。
//
// 依存範囲: 標準libraryと同じdomain配下だけに依存する。抽象portもcore側であるここが所有する（§1）。
//
// P1-01時点ではpackage骨格だけであり、実体は後続taskで実装する
// （docs/13-progress.md）。許可するinternal importは
// scripts/ci/check_imports.py の表を正本とし、`policy` jobが検査する。
package domain
