package security

import (
	"net/url"
	"strings"
	"testing"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
)

func TestIsSecretEnvName(t *testing.T) {
	secret := []string{
		"GITHUB_TOKEN", "NPM_PASSWORD", "MY_SECRET", "SIGNING_KEY",
		"github_token", "Npm_Password", "a_key",
	}
	for _, name := range secret {
		if !IsSecretEnvName(name) {
			t.Errorf("IsSecretEnvName(%q) = false, want true", name)
		}
	}

	// docs/10-security.md §9.2は`*_TOKEN`等のsuffixだけを対象とする。
	// digestやversionはsecretではないため記録する。
	public := []string{"GOPATH", "TOKENIZER", "KEYBOARD", "PATH", "tool_id", "expected_digest", ""}
	for _, name := range public {
		if IsSecretEnvName(name) {
			t.Errorf("IsSecretEnvName(%q) = true, want false", name)
		}
	}
}

func TestIsSecretHeader(t *testing.T) {
	secret := []string{
		"Authorization", "authorization", "AUTHORIZATION", " Cookie ",
		"Set-Cookie", "Proxy-Authorization", "proxy-authenticate",
	}
	for _, name := range secret {
		if !IsSecretHeader(name) {
			t.Errorf("IsSecretHeader(%q) = false, want true", name)
		}
	}
	for _, name := range []string{"Content-Type", "User-Agent", "ETag", ""} {
		if IsSecretHeader(name) {
			t.Errorf("IsSecretHeader(%q) = true, want false", name)
		}
	}
}

// TestMaskURLRemovesUserinfoAndQueryValues はdocs/10-security.md §9.2の
// 「URL userinfo、既知のtoken query key」の除去を確かめる。
//
// 「既知のtoken query key」の一覧は仕様に無いため、実装はquery値を種類に
// よらず置換する。ここではその上位集合の動作を固定する。
//
// query値の置換結果はpercent encodingされ`%3Credacted%3E`として現れる。
// RFC 3986がquery値へ`<`/`>`をそのまま置けないためであり、これは正しい挙動である。
func TestMaskURLRemovesUserinfoAndQueryValues(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		leaked  []string
		wantSub []string
	}{
		{
			"userinfo",
			"https://alice:hunter2@example.com/dl/go.tar.gz",
			[]string{"hunter2", "alice"},
			[]string{"example.com", "/dl/go.tar.gz"},
		},
		{
			"token query",
			"https://example.com/a?access_token=abc123&x=1",
			[]string{"abc123"},
			[]string{"access_token", "x", url.QueryEscape(Redacted)},
		},
		{
			"未知のquery key",
			"https://example.com/a?sig=zzz",
			[]string{"zzz"},
			[]string{"sig", url.QueryEscape(Redacted)},
		},
		{
			"fragment",
			"https://example.com/a#hunter2",
			[]string{"hunter2"},
			[]string{"example.com"},
		},
		{
			"queryなし",
			"https://example.com/dl/go1.26.5.linux-amd64.tar.gz",
			nil,
			[]string{"go1.26.5.linux-amd64.tar.gz"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := MaskURL(test.raw)
			for _, leaked := range test.leaked {
				if strings.Contains(got, leaked) {
					t.Errorf("MaskURL(%q) = %q に %q が残った", test.raw, got, leaked)
				}
			}
			for _, want := range test.wantSub {
				if !strings.Contains(got, want) {
					t.Errorf("MaskURL(%q) = %q に %q が含まれない", test.raw, got, want)
				}
			}
		})
	}

	if got := MaskURL(""); got != "" {
		t.Errorf("MaskURL(\"\") = %q, want \"\"", got)
	}
	// 解析できない文字列は丸ごと置換する。壊れたURLのcredentialを素通しさせない。
	if got := MaskURL("://alice:hunter2@%%%"); got != Redacted {
		t.Errorf("解析不能URL = %q, want %q", got, Redacted)
	}
}

func TestPathMaskerReplacesLongestFirst(t *testing.T) {
	masker := NewPathMasker("/home/alice", "alice", "build-host")

	got := masker.Mask("/home/alice/.local/share/gdtvm on build-host by alice")
	for _, leaked := range []string{"/home/alice", "build-host"} {
		if strings.Contains(got, leaked) {
			t.Errorf("Mask結果 %q に %q が残った", got, leaked)
		}
	}
	if !strings.HasPrefix(got, HomePlaceholder+"/.local/share/gdtvm") {
		t.Errorf("home置換が効いていない: %q", got)
	}
	if !strings.Contains(got, HostPlaceholder) || !strings.Contains(got, UserPlaceholder) {
		t.Errorf("host/user置換が効いていない: %q", got)
	}
}

// TestPathMaskerIgnoresEmptyReplacements は空の置換対象で出力が壊れないことを見る。
func TestPathMaskerIgnoresEmptyReplacements(t *testing.T) {
	masker := NewPathMasker("", "", "")
	const text = "/opt/gdtvm/bin"
	if got := masker.Mask(text); got != text {
		t.Errorf("Mask = %q, want %q", got, text)
	}

	var nilMasker *PathMasker
	if got := nilMasker.Mask(text); got != text {
		t.Errorf("nil maskerのMask = %q, want %q", got, text)
	}
}

func TestMaskParameters(t *testing.T) {
	masker := NewPathMasker("/home/alice", "alice", "build-host")
	params := domain.Parameters{
		"data_root":       domain.StringScalar("/home/alice/.local/share/gdtvm"),
		"github_token":    domain.StringScalar("ghp_realsecret"),
		"expected_digest": domain.StringScalar("a1b2c3"),
		"count":           domain.IntScalar(3),
		"offline":         domain.BoolScalar(true),
		"missing":         domain.NullScalar(),
	}

	masked := masker.MaskParameters(params)

	if value, _ := masked["data_root"].Str(); !strings.HasPrefix(value, HomePlaceholder) {
		t.Errorf("data_root = %q, home置換が効いていない", value)
	}
	if value, _ := masked["github_token"].Str(); value != Redacted {
		t.Errorf("github_token = %q, want %q", value, Redacted)
	}
	// digestはsecretではないため記録する（docs/10-security.md §9.2）。
	if value, _ := masked["expected_digest"].Str(); value != "a1b2c3" {
		t.Errorf("expected_digest = %q, want a1b2c3", value)
	}
	if value, ok := masked["count"].Int(); !ok || value != 3 {
		t.Errorf("count = %d, %v", value, ok)
	}
	if value, ok := masked["offline"].Bool(); !ok || !value {
		t.Errorf("offline = %v, %v", value, ok)
	}
	if !masked["missing"].IsNull() {
		t.Error("null値が変わった")
	}

	// 元のmapは変えない。境界通過後の値をimmutableとして扱うため。
	if value, _ := params["github_token"].Str(); value != "ghp_realsecret" {
		t.Errorf("元のparametersが書き換わった: %q", value)
	}
	if masker.MaskParameters(nil) != nil {
		t.Error("nilのMaskParametersがnilでない")
	}
}
