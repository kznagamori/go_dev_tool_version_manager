// Package app はユースケースの公開窓口、要求検証、transaction境界を担う。
//
// docs/02-architecture.md §2 の論理領域「internal/app」に対応する。
//
// 依存範囲: domainとport、および各内部domain serviceを組み合わせる。CLI、具体的OS API、
// 具体的HTTP client、CLI framework/TOML libraryの型を参照しない（§1、§2）。
//
// P1-01時点ではpackage骨格だけであり、実体は後続taskで実装する
// （docs/13-progress.md）。許可するinternal importは
// scripts/ci/check_imports.py の表を正本とし、`policy` jobが検査する。
package app
