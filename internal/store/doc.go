// Package store は状態・catalog・receipt・承認・journal の永続化と atomic write を
// 担う（03章、14章）。
//
// 書換えは一時ファイル + flush + 同一ボリューム rename を基本とし、.bak・revision・
// 楽観的競合検出・corruption recovery を提供する。TOML state は unknown key を拒否し、
// journal は JSON Lines、catalog は正規化 JSON とする。schema migration は N→N+1 の
// 逐次関数だけを実装する。
package store
