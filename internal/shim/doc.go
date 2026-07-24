// Package shim は shim metadata 生成、呼出名解決、実体委譲を担う（08章11節、10章6節）。
//
// 起動 basename から候補 tool/runtime command を引き、有効選択・receipt から所有 tool を
// 決めて実体を起動する。network/install/prompt/state write を行わず、target は receipt の
// payload/shared 相対 path から解決して一般 PATH 検索を使わない。再帰は depth 8 で防ぐ。
package shim
