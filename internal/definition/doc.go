// Package definition はtool定義の解析、schema検証、template評価を担う。
//
// docs/02-architecture.md §2 の論理領域「internal/definition」に対応する。
//
// 依存範囲: domainとportに依存する。tool固有の分岐を持たず、定義の内容だけで挙動を決める（`CLAUDE.md`§7）。
//
// P1-01時点ではpackage骨格だけであり、実体は後続taskで実装する
// （docs/13-progress.md）。許可するinternal importは
// scripts/ci/check_imports.py の表を正本とし、`policy` jobが検査する。
package definition
