// Package i18n は locale-neutral な message ID と ja/en カタログを担う（14章15節、
// 01章7.8節）。
//
// client 本体の基本 message は binary 同梱とし、registry message は tool 固有 notes
// だけで基本 error を上書きできない。message ID は ASCII lower dot path、placeholder は
// ASCII identifier で、ja/en 間の placeholder 集合が完全一致しなければ CI 失敗とする。
package i18n
