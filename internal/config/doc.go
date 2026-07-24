// Package config は実行ファイル隣接の gdtvm.toml、project の .gdtvm.toml、許可された
// 環境変数の読込みと統合を担う（05章）。
//
// strict parse により未知 key・型違い・上限超過・範囲外を拒否し、寛容な fallback を
// 追加しない。設定優先順位は CLI option > project 選択 > 隣接 gdtvm.toml > built-in
// 既定値とし、gdtvm 固有環境変数は許可されたものだけを読む。
package config
