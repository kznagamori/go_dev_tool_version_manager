// Package platform はWindows/Linux固有のlink、process、権限、pathを担う。
//
// docs/02-architecture.md §2 の論理領域「internal/platform」に対応する。
//
// 依存範囲: domainとportに依存するInfrastructure adapterである。ここだけが具体的OS APIを扱う（§1）。
//
// 実装済みはHTTP client（P5-01）、ProcessRunner（P5-04）、LinkManager（P7-01）で
// ある。残りは後続taskで実装する（docs/13-progress.md）。許可するinternal importは
// scripts/ci/check_imports.py の表を正本とし、`policy` jobが検査する。
package platform
