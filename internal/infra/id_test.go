package infra

import (
	"regexp"
	"testing"
)

var uuidV4Pattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestUUIDGenerator_Format(t *testing.T) {
	g := NewUUIDGenerator()
	for i := 0; i < 100; i++ {
		id := g.NewID()
		if !uuidV4Pattern.MatchString(id) {
			t.Fatalf("生成 ID が UUIDv4 形式でない: %q", id)
		}
	}
}

func TestUUIDGenerator_Unique(t *testing.T) {
	g := NewUUIDGenerator()
	seen := make(map[string]struct{}, 2000)
	for i := 0; i < 2000; i++ {
		id := g.NewID()
		if _, dup := seen[id]; dup {
			t.Fatalf("ID が重複: %q", id)
		}
		seen[id] = struct{}{}
	}
}
