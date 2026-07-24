// Package runtime は実行環境生成、command 解決、子プロセス起動を担う（06章10節、
// 08章12節）。
//
// 親環境を基底に managed 依存・tool profile・command profile・exec 明示の順で環境を
// merge し、PATH を正規化して先勝ちで重複排除する。launcher 種別（native/cmd-script/
// powershell-script/sh-script）に従い argv を構成する。
package runtime
