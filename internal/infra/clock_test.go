package infra

import (
	"testing"
	"time"
)

func TestSystemClock(t *testing.T) {
	c := NewSystemClock()
	now := c.Now()
	if now.IsZero() {
		t.Fatal("Now() がゼロ値")
	}
	past := now.Add(-time.Second)
	if d := c.Since(past); d < 0 {
		t.Errorf("Since(past) が負: %v", d)
	}
}
