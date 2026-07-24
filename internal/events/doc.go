// Package events は進捗・警告・確認要求・監査イベントの schema と sink を担う
// （10章7節）。
//
// event は schema・operation_id・sequence・type・message_id・非秘密 args を持ち、
// operation ごとに sequence を直列化する。progress の中間 event は coalesce できるが、
// 開始・終了・warning・approval を落とさない。
package events
