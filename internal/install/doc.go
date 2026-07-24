// Package install は依存計画、ダウンロード、検証、展開、hook、receipt 生成を担う
// （08章）。
//
// 状態変更は Resolve → Plan → Approve → Execute → Commit → Cleanup の段階を通り、
// staging 領域だけを変更してから完成先へ atomic に公開する。backend 方式（rustup）は
// shared store と版別 receipt を用い、payload を複製しない。
package install
