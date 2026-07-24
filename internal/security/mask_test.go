package security

import (
	"strings"
	"testing"
)

func TestIsSensitiveHeader(t *testing.T) {
	for _, h := range []string{"Authorization", "authorization", "Proxy-Authorization", "Cookie", "Set-Cookie"} {
		if !IsSensitiveHeader(h) {
			t.Errorf("IsSensitiveHeader(%q) = false", h)
		}
	}
	for _, h := range []string{"Content-Type", "Accept", "User-Agent"} {
		if IsSensitiveHeader(h) {
			t.Errorf("IsSensitiveHeader(%q) = true", h)
		}
	}
}

func TestIsSensitiveEnvKey(t *testing.T) {
	for _, k := range []string{"GITHUB_TOKEN", "MY_PASSWORD", "X_SECRET", "API_KEY", "TOKEN", "password"} {
		if !IsSensitiveEnvKey(k) {
			t.Errorf("IsSensitiveEnvKey(%q) = false", k)
		}
	}
	for _, k := range []string{"PATH", "HOME", "GOROOT", "JAVA_HOME"} {
		if IsSensitiveEnvKey(k) {
			t.Errorf("IsSensitiveEnvKey(%q) = true", k)
		}
	}
}

func TestMaskHeaderValue(t *testing.T) {
	if got := MaskHeaderValue("Authorization", "Bearer abc"); got != Redacted {
		t.Errorf("Authorization = %q, want %q", got, Redacted)
	}
	if got := MaskHeaderValue("Accept", "application/json"); got != "application/json" {
		t.Errorf("Accept は素通しすべき: %q", got)
	}
	if got := MaskHeaderValue("X-Custom-Token", "sekret", "X-Custom-Token"); got != Redacted {
		t.Errorf("追加指定 header = %q, want %q", got, Redacted)
	}
}

func TestMaskEnvValue(t *testing.T) {
	if got := MaskEnvValue("GITHUB_TOKEN", "ghp_xxx"); got != Redacted {
		t.Errorf("GITHUB_TOKEN = %q", got)
	}
	if got := MaskEnvValue("GOROOT", "/opt/go"); got != "/opt/go" {
		t.Errorf("GOROOT は素通しすべき: %q", got)
	}
}

func TestMaskURL(t *testing.T) {
	masked := MaskURL("https://user:pass@example.com/path?token=abc123&x=1")
	if strings.Contains(masked, "pass") {
		t.Errorf("userinfo が残っている: %q", masked)
	}
	if strings.Contains(masked, "abc123") {
		t.Errorf("token 値が残っている: %q", masked)
	}
	if !strings.Contains(masked, "x=1") {
		t.Errorf("非 secret query が失われた: %q", masked)
	}

	plain := "https://go.dev/dl/go1.26.5.linux-amd64.tar.gz"
	if MaskURL(plain) != plain {
		t.Errorf("secret を含まない URL は不変であるべき: %q", MaskURL(plain))
	}
}
