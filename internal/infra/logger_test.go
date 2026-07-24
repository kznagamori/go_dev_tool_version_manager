package infra

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/ports"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/security"
)

func decodeLast(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	var rec map[string]any
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &rec); err != nil {
		t.Fatalf("ログ行の JSON parse に失敗: %v (%q)", err, buf.String())
	}
	return rec
}

func TestLogger_LevelFiltering(t *testing.T) {
	var buf bytes.Buffer
	log := NewLogger(&buf, ports.LevelInfo, NewSystemClock())
	log.Log(ports.LevelDebug, "debug msg")
	if buf.Len() != 0 {
		t.Errorf("Info 閾値で Debug は抑制すべき: %q", buf.String())
	}
	log.Log(ports.LevelInfo, "info msg")
	log.Log(ports.LevelError, "error msg")
	if got := strings.Count(buf.String(), "\n"); got != 2 {
		t.Errorf("Info/Error の 2 行を期待、実際 %d 行", got)
	}
}

func TestLogger_StructureAndFields(t *testing.T) {
	var buf bytes.Buffer
	log := NewLogger(&buf, ports.LevelInfo, NewSystemClock())
	log.Log(ports.LevelInfo, "hello", ports.Field{Key: "tool_id", Value: "node"})
	rec := decodeLast(t, &buf)
	if rec["level"] != "info" || rec["msg"] != "hello" {
		t.Errorf("level/msg 不一致: %+v", rec)
	}
	if rec["tool_id"] != "node" {
		t.Errorf("field 不一致: %+v", rec)
	}
	if _, ok := rec["time"]; !ok {
		t.Error("time field がない")
	}
}

func TestLogger_MasksSecrets(t *testing.T) {
	var buf bytes.Buffer
	log := NewLogger(&buf, ports.LevelInfo, NewSystemClock())
	log.Log(ports.LevelInfo, "req",
		ports.Field{Key: "Authorization", Value: "Bearer sekret"},
		ports.Field{Key: "GITHUB_TOKEN", Value: "ghp_zzz"},
		ports.Field{Key: "host", Value: "example.com"},
	)
	out := buf.String()
	if strings.Contains(out, "sekret") || strings.Contains(out, "ghp_zzz") {
		t.Errorf("secret がログに残っている: %q", out)
	}
	if !strings.Contains(out, security.Redacted) {
		t.Errorf("Redacted 置換がない: %q", out)
	}
	if !strings.Contains(out, "example.com") {
		t.Errorf("非 secret field が失われた: %q", out)
	}
}

func TestLogger_WithInheritsFields(t *testing.T) {
	var buf bytes.Buffer
	base := NewLogger(&buf, ports.LevelInfo, NewSystemClock())
	child := base.With(ports.Field{Key: "operation_id", Value: "op-1"})
	child.Log(ports.LevelInfo, "step")
	rec := decodeLast(t, &buf)
	if rec["operation_id"] != "op-1" {
		t.Errorf("With の field が継承されていない: %+v", rec)
	}
}
