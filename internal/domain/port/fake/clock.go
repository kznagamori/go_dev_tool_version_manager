package fake

import (
	"sync"
	"time"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain/port"
)

// Clock は手動で進める決定的なport.Clockである。
//
// wall clockと単調時間を別々に保持し、Advanceで両方を、SetNowでwall clockだけを
// 動かせる。NTP補正やsummer timeでwall clockが巻き戻ってもtimeout判定が壊れない
// ことを検査するために、2つを独立に動かせる必要がある。
type Clock struct {
	mu    sync.Mutex
	now   time.Time
	nanos int64
}

var _ port.Clock = (*Clock)(nil)

// NewClock は指定時刻から始まるClockを作る。
func NewClock(start time.Time) *Clock {
	return &Clock{now: start}
}

// Now はwall clock時刻を返す。
func (c *Clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// Monotonic は現在の単調時間点を返す。
func (c *Clock) Monotonic() port.Monotonic {
	c.mu.Lock()
	defer c.mu.Unlock()
	return port.NewMonotonic(c.nanos)
}

// Since は単調時間で経過時間を返す。
func (c *Clock) Since(start port.Monotonic) time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	return port.NewMonotonic(c.nanos).Sub(start)
}

// Advance はwall clockと単調時間の両方を進める。
func (c *Clock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
	c.nanos += int64(d)
}

// AdvanceMonotonic は単調時間だけを進める。wall clockは動かさない。
func (c *Clock) AdvanceMonotonic(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.nanos += int64(d)
}

// SetNow はwall clockだけを設定する。単調時間は動かさない。
// 時計の巻き戻しを再現するために使う。
func (c *Clock) SetNow(t time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = t
}
