// Package security はupstream SHA-256/SHA-512、内部SHA-256、path検査、maskを担う。
//
// docs/02-architecture.md §2 の論理領域「internal/security」に対応する。
//
// 依存範囲: domainとportに依存する。検査はfail closedとし、迂回optionを持たない（§8）。
//
// P2-03時点で持つのは docs/10-security.md §9.2 のmask規則と、
// docs/04-storage-and-data.md §6 のpath containment（[Join]、[ValidateComponent]、
// [IsContained]）である。upstream/内部digestの計算は後続taskで実装する
// （docs/13-progress.md）。許可するinternal importは
// scripts/ci/check_imports.py の表を正本とし、`policy` jobが検査する。
package security
