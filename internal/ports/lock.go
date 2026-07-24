package ports

import (
	"context"
	"time"
)

// LockInfo は lock ファイルに記録する所有情報である（02章7節）。PID 不在だけで即時
// 破棄せず、開始時刻・operation ID・hostname と合わせて stale 判定する。
type LockInfo struct {
	PID         int
	StartedAt   time.Time
	OperationID string
	Hostname    string
}

// Lock は取得済みの lock を表す。Release で解放する。
type Lock interface {
	// Info は所有情報を返す。
	Info() LockInfo
	// Release は lock を解放する。冪等であること。
	Release() error
}

// LockManager は process 間の共有/排他ロック、所有情報、timeout を扱う抽象 port である
// （02章5/7節）。lock 順序は上位が固定し、本 port は個々の取得/検査を提供する。
type LockManager interface {
	// Acquire は name の lock を取得する。exclusive=false は共有ロック。timeout 経過で
	// 失敗する。context cancel に応答する。
	Acquire(ctx context.Context, name string, exclusive bool, timeout time.Duration) (Lock, error)
	// Inspect は現在の所有情報を返す（存在しなければ ok=false）。
	Inspect(name string) (info LockInfo, ok bool, err error)
}
