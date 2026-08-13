// Package config は実行file隣接設定、project設定、許可された環境変数の読込みと統合を担う。
//
// docs/02-architecture.md §2 の論理領域「internal/config」に対応する。
//
// 依存範囲: domainとportに依存する。設定fileの読取りはFileSystem port経由で行う。
//
// P2-01時点で持つのは docs/04-storage-and-data.md §1のmode決定・root決定と、
// docs/05-configuration.md §2のglobal config locatorである。TOML schema検証と
// project探索はP2-02、rootのfilesystem安全検査はP2-03で実装する
// （docs/13-progress.md）。許可するinternal importは
// scripts/ci/check_imports.py の表を正本とし、`policy` jobが検査する。
package config
