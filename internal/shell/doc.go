// Package shell は shell 統合の setup、profile marker、completion、undo を担う
// （09章、04章3.1/3.15節）。
//
// cmd AutoRun（HKCU のみ）、PowerShell 5.1/7 profile、bash/zsh/fish startup を marker
// 範囲に限定し、事前表示・個別確認・backup・冪等・undo を必須とする。execution policy を
// 変更せず、生成 startup file は network/hook を実行しない。
package shell
