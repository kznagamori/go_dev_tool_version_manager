// Package doctor は診断規則と修復計画を担う（04章3.12/3.13節、08章14節）。
//
// doctor は読取り専用診断で各項目に ok/warning/error/skipped と修復可否を返す。repair は
// 安全な修復（current link、shim index、state backup 復元、移動後 path、shell marker 重複
// 等）だけを計画し、payload 再 download や未承認 hook を自動実行しない。
package doctor
