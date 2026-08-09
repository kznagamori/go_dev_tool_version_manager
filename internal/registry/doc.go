// Package registry は同梱registryの読込みとschema検証を担う。
//
// docs/02-architecture.md §2 の論理領域「internal/registry」に対応する。
//
// 依存範囲: domain、port、definitionに依存する。registry単体のdownloadやupdate経路を持たない。
//
// P1-01時点ではpackage骨格だけであり、実体は後続taskで実装する
// （docs/13-progress.md）。許可するinternal importは
// scripts/ci/check_imports.py の表を正本とし、`policy` jobが検査する。
package registry
