// Package ports は 02章5節の抽象ポート（Application Service と Infrastructure の境界
// インターフェース）を定義する。
//
// 本パッケージは interface と、その入出力に必要な最小の value 型だけを持ち、具体的な
// OS API・HTTP クライアント・TOML ライブラリ・CLI/Wails 型を参照しない（domain のみ
// 依存）。02章5節の「名称は概念名でありGoの宣言そのものではない」に従い、port の
// Go 表現を本パッケージへ集約して import 循環を避ける。テストでは全 port をメモリまたは
// 一時ディレクトリ実装へ差し替える。
package ports
