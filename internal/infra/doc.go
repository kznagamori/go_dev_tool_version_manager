// Package infra は OS 非依存の抽象 port（Clock、IDGenerator、Logger）の既定実装を
// 提供する。
//
// composition root（Application Service 生成時）がこれらを注入する。OS 固有の port
// （FileSystem、LinkManager、ProcessRunner 等）は internal/platform が担い、本 package は
// 時刻・ID 生成・構造化ログのような platform 中立な基盤に限る。テストでは fake へ差し替える。
package infra
