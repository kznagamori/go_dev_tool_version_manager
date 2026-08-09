// Package config は実行file隣接設定、project設定、許可された環境変数の読込みと統合を担う。
//
// docs/02-architecture.md §2 の論理領域「internal/config」に対応する。
//
// 依存範囲: domainとportに依存する。設定fileの読取りはFileSystem port経由で行う。
//
// P1-01時点ではpackage骨格だけであり、実体は後続taskで実装する
// （docs/13-progress.md）。許可するinternal importは
// scripts/ci/check_imports.py の表を正本とし、`policy` jobが検査する。
package config
