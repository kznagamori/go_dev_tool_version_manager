// Package doctor は診断規則とreport生成を担う。
//
// docs/02-architecture.md §2 の論理領域「internal/doctor」に対応する。
//
// 依存範囲: domain、port、store、platformに依存する。reportはsecretと個人pathを除去して出す。
//
// P1-01時点ではpackage骨格だけであり、実体は後続taskで実装する
// （docs/13-progress.md）。許可するinternal importは
// scripts/ci/check_imports.py の表を正本とし、`policy` jobが検査する。
package doctor
