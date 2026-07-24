// Package domain は gdtvm の中核となる値型、不変条件、エラーモデルを提供する。
//
// 本パッケージは 02章の依存方向に従い、CLI、Wails、具体的な OS API、HTTP
// クライアント、TOML ライブラリを参照しない。値型は境界を通過した後は
// immutable として扱い、package global mutable state を持たない。
//
// 各値型のコンストラクタは不変条件を検査し、違反時は errors.go の sentinel
// error を包んで返す。CoreError（coreerror.go）は Application Service 境界の
// 型付きエラーであり、value 検査エラーを cause として包める。
package domain
