// Package registry は実行ファイル隣接の同梱 registry の読込み、schema 検証、
// definition revision 構築を担う（07章）。
//
// 起動時に registry tree の canonical SHA-256 を raw file bytes から計算し、binary
// 埋込みの expected hash と照合する。registry 単体の download/update は提供せず、
// 標準定義は client と同じ配布単位として扱う。
package registry
