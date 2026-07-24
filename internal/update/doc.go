// Package update は公式 GitHub Release の探索、checksum 照合、self-update
// transaction を担う（08章16節）。
//
// 公式 repository identity → published Release metadata → canonical checksums.txt →
// 対象 archive の SHA-256 → 安全展開 → package 検証 → commit → rollback を fail closed
// で行う。client、既定 gdtvm.toml、registry、文書を 1 transaction で更新し、registry
// 単体更新を提供しない。
package update
