// Package catalog は公式配布元の照会、版正規化、stable 判定、catalog キャッシュを
// 担う（06章5節、08章2節、14章10節）。
//
// version source adapter の結果を正規化 JSON の catalog として保存し、artifact/
// checksum の遅延解決（pending）と TTL/ETag を扱う。HTML/JSON の構造変化は refresh
// 失敗として扱い、空一覧を成功保存しない。
package catalog
