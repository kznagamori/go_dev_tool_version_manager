package porttest

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/events"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/ports"
)

// FakeClock は注入した時刻を返す決定的な ports.Clock である。
type FakeClock struct {
	mu sync.Mutex
	t  time.Time
}

// NewFakeClock は基準時刻 t の FakeClock を返す。
func NewFakeClock(t time.Time) *FakeClock { return &FakeClock{t: t} }

// Now は現在保持している時刻を返す。
func (c *FakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

// Since は保持時刻から s までの経過を返す。
func (c *FakeClock) Since(s time.Time) time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t.Sub(s)
}

// Advance は保持時刻を d だけ進める。
func (c *FakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

// FakeIDGenerator は連番の ID を返す決定的な ports.IDGenerator である。
type FakeIDGenerator struct {
	mu     sync.Mutex
	n      int
	prefix string
}

// NewFakeIDGenerator は prefix 付き連番を返す generator を作る。prefix 空は "id"。
func NewFakeIDGenerator(prefix string) *FakeIDGenerator {
	if prefix == "" {
		prefix = "id"
	}
	return &FakeIDGenerator{prefix: prefix}
}

// NewID は "<prefix>-<n>" を返す（n は 1 から増加）。
func (g *FakeIDGenerator) NewID() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.n++
	return fmt.Sprintf("%s-%d", g.prefix, g.n)
}

// RecordingSink は emit された event を記録する events.EventSink である。
type RecordingSink struct {
	mu     sync.Mutex
	events []events.Event
}

// Emit は event を記録する。
func (s *RecordingSink) Emit(e events.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, e)
}

// Events は記録した event のコピーを返す。
func (s *RecordingSink) Events() []events.Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]events.Event(nil), s.events...)
}

// DenyApprovals は常に deny を返す events.ApprovalProvider である（既定回答は deny）。
type DenyApprovals struct{}

// Resolve は常に DecisionDeny を返す。
func (DenyApprovals) Resolve(_ context.Context, _ events.ApprovalRequest) (events.ApprovalDecision, error) {
	return events.DecisionDeny, nil
}

// NopLogger は何も出力しない ports.Logger である。
type NopLogger struct{}

// Log は何もしない。
func (NopLogger) Log(ports.LogLevel, string, ...ports.Field) {}

// With は自身を返す。
func (l NopLogger) With(...ports.Field) ports.Logger { return l }

// StubLocaleProvider は固定 Locale を返す ports.LocaleProvider である。
type StubLocaleProvider struct {
	Locale ports.Locale
}

// Detect は保持する Locale を返す。
func (p StubLocaleProvider) Detect() ports.Locale { return p.Locale }
