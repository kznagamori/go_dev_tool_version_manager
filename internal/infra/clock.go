package infra

import (
	"time"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/ports"
)

// systemClock は実時間を返す ports.Clock の既定実装である。
type systemClock struct{}

// NewSystemClock は実時間を用いる Clock を返す。
func NewSystemClock() ports.Clock { return systemClock{} }

// Now は現在の wall-clock 時刻を返す（単調成分を含む）。
func (systemClock) Now() time.Time { return time.Now() }

// Since は t からの経過時間を返す。
func (systemClock) Since(t time.Time) time.Duration { return time.Since(t) }
