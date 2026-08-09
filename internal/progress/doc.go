// Package progress は型付き進捗、warning、cancel境界を担う。
//
// docs/02-architecture.md §2 の論理領域「internal/progress」に対応する。
//
// 依存範囲: domainだけに依存する。
//
// P1-04時点で持つのは[Progress]、[Sink]、[CancelToken]、[Reporter]と
// [ResultWarning] である。TTY progress barや非TTY節目表示はCLI adapterの責務で
// あり本packageには置かない（docs/13-progress.md P8-05）。許可するinternal
// importは scripts/ci/check_imports.py の表を正本とし、`policy` jobが検査する。
package progress
