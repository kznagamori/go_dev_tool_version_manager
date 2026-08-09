// Package catalog は配布元照会、版正規化、channel/lifecycle判定、catalog cacheを担う。
//
// docs/02-architecture.md §2 の論理領域「internal/catalog」に対応する。
//
// 依存範囲: domain、port、definitionに依存する。HTTP接続はHTTPClient port経由で行う。
//
// P1-01時点ではpackage骨格だけであり、実体は後続taskで実装する
// （docs/13-progress.md）。許可するinternal importは
// scripts/ci/check_imports.py の表を正本とし、`policy` jobが検査する。
package catalog
