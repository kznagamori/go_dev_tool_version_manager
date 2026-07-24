package ports

import "time"

// Clock は現在時刻と単調時間を提供する抽象 port である（02章5節）。テストでは fake
// clock に差し替えて TTL・interval・タイムスタンプを決定化する。
type Clock interface {
	// Now は wall-clock 時刻を返す（Go の time.Time は単調成分も保持する）。
	Now() time.Time
	// Since は t からの経過時間を返す。
	Since(t time.Time) time.Duration
}
