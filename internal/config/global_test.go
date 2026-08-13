package config

import (
	"strings"
	"testing"
	"time"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain/port"
)

// specSample はdocs/05-configuration.md §3が「全keyを明示した例」として載せる内容である。
const specSample = `schema = 1

[application]
color = "auto"

[paths]
user_data_root = ""

[project]
filename = ".gdtvm.toml"
stop_at_vcs_root = true

[network]
connect_timeout = "30s"
request_timeout = "5m"

[download]
cache_max_bytes = 10737418240

[runtime]
auto_install_on_use = true

[logs]
level = "info"
max_files = 5
max_bytes_per_file = 5242880
`

func userRequest(t *testing.T, body string) GlobalRequest {
	t.Helper()
	return GlobalRequest{Data: []byte(body), Mode: domain.ModeUser, Host: linuxHost(t)}
}

// TestParseGlobalConfigAcceptsSpecSample は仕様の例が既定値と一致することを固定する。
//
// §3は各keyの右コメントを既定値としている。例をそのまま読んだ結果が
// [DefaultGlobalConfig]と一致しなければ、どちらかが仕様とずれている。
func TestParseGlobalConfigAcceptsSpecSample(t *testing.T) {
	got, err := ParseGlobalConfig(userRequest(t, specSample))
	if err != nil {
		t.Fatalf("ParseGlobalConfig = %v, want nil", err)
	}
	if got != DefaultGlobalConfig() {
		t.Errorf("仕様の例 = %+v, want %+v", got, DefaultGlobalConfig())
	}
}

// TestParseGlobalConfigMinimalFileUsesDefaults は`schema`だけのfileを許すことを見る。
//
// §3は「各table/keyは任意で、省略時は右コメントの既定値を使う」と定める。
func TestParseGlobalConfigMinimalFileUsesDefaults(t *testing.T) {
	got, err := ParseGlobalConfig(userRequest(t, "schema = 1\n"))
	if err != nil {
		t.Fatalf("ParseGlobalConfig = %v, want nil", err)
	}
	if got != DefaultGlobalConfig() {
		t.Errorf("最小file = %+v, want %+v", got, DefaultGlobalConfig())
	}
}

// TestParseGlobalConfigDistinguishesFalseFromUnset は明示falseが既定trueを上書きすることを見る。
func TestParseGlobalConfigDistinguishesFalseFromUnset(t *testing.T) {
	body := "schema = 1\n[project]\nstop_at_vcs_root = false\n[runtime]\nauto_install_on_use = false\n"
	got, err := ParseGlobalConfig(userRequest(t, body))
	if err != nil {
		t.Fatalf("ParseGlobalConfig = %v, want nil", err)
	}
	if got.StopAtVCSRoot {
		t.Error("stop_at_vcs_root = true, want false")
	}
	if got.AutoInstallOnUse {
		t.Error("auto_install_on_use = true, want false")
	}
}

func TestParseGlobalConfigAppliesEachKey(t *testing.T) {
	body := `schema = 1
[application]
color = "never"
[project]
stop_at_vcs_root = false
[network]
connect_timeout = "1s"
request_timeout = "1h"
[download]
cache_max_bytes = 1073741824
[logs]
level = "trace"
max_files = 100
max_bytes_per_file = 1073741824
`
	got, err := ParseGlobalConfig(userRequest(t, body))
	if err != nil {
		t.Fatalf("ParseGlobalConfig = %v, want nil", err)
	}
	if got.Color != ColorNever {
		t.Errorf("Color = %q", got.Color)
	}
	if got.ConnectTimeout != time.Second || got.RequestTimeout != time.Hour {
		t.Errorf("timeout = %v/%v", got.ConnectTimeout, got.RequestTimeout)
	}
	if got.CacheMaxBytes != 1<<30 {
		t.Errorf("CacheMaxBytes = %d", got.CacheMaxBytes)
	}
	if got.LogLevel != port.LevelTrace || got.LogMaxFiles != 100 || got.LogMaxBytesPerFile != 1<<30 {
		t.Errorf("logs = %v/%d/%d", got.LogLevel, got.LogMaxFiles, got.LogMaxBytesPerFile)
	}
}

// TestParseGlobalConfigRejectsStrictViolations はdocs/05-configuration.md §1の
// 「unknown key、重複key/table、型違い、enum外、上限外」の拒否を固定する。
func TestParseGlobalConfigRejectsStrictViolations(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"schemaが無い", "[application]\ncolor = \"auto\"\n"},
		{"schemaが1以外", "schema = 2\n"},
		{"schemaが文字列", "schema = \"1\"\n"},
		{"空file", ""},
		{"unknown top-level key", "schema = 1\nlanguage = \"ja\"\n"},
		{"unknown table", "schema = 1\n[telemetry]\nenabled = true\n"},
		{"unknown table内key", "schema = 1\n[application]\ntheme = \"dark\"\n"},
		{"重複key", "schema = 1\n[application]\ncolor = \"auto\"\ncolor = \"never\"\n"},
		{"重複table", "schema = 1\n[network]\n[network]\n"},
		{"色のenum外", "schema = 1\n[application]\ncolor = \"rainbow\"\n"},
		{"色が大文字", "schema = 1\n[application]\ncolor = \"AUTO\"\n"},
		{"色が真偽値", "schema = 1\n[application]\ncolor = true\n"},
		{"project filenameが他値", "schema = 1\n[project]\nfilename = \"gdtvm.toml\"\n"},
		{"stop_at_vcs_rootが文字列", "schema = 1\n[project]\nstop_at_vcs_root = \"true\"\n"},
		{"connect_timeoutがduration以外", "schema = 1\n[network]\nconnect_timeout = \"soon\"\n"},
		{"connect_timeoutが下限未満", "schema = 1\n[network]\nconnect_timeout = \"999ms\"\n"},
		{"connect_timeoutが上限超過", "schema = 1\n[network]\nconnect_timeout = \"5m1s\"\n"},
		{"connect_timeoutが負", "schema = 1\n[network]\nconnect_timeout = \"-1s\"\n"},
		{"request_timeoutが下限未満", "schema = 1\n[network]\nrequest_timeout = \"9s\"\n"},
		{"request_timeoutが上限超過", "schema = 1\n[network]\nrequest_timeout = \"1h1s\"\n"},
		{"cache_max_bytesが下限未満", "schema = 1\n[download]\ncache_max_bytes = 1073741823\n"},
		{"cache_max_bytesが上限超過", "schema = 1\n[download]\ncache_max_bytes = 1099511627777\n"},
		{"cache_max_bytesが文字列", "schema = 1\n[download]\ncache_max_bytes = \"10GiB\"\n"},
		{"log levelのenum外", "schema = 1\n[logs]\nlevel = \"fatal\"\n"},
		{"max_filesが0", "schema = 1\n[logs]\nmax_files = 0\n"},
		{"max_filesが101", "schema = 1\n[logs]\nmax_files = 101\n"},
		{"max_bytes_per_fileが下限未満", "schema = 1\n[logs]\nmax_bytes_per_file = 1048575\n"},
		{"max_bytes_per_fileが上限超過", "schema = 1\n[logs]\nmax_bytes_per_file = 1073741825\n"},
		{"TOMLとして壊れている", "schema = 1\n[application\n"},
		{"末尾に余分なdata", "schema = 1\n= broken\n"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := ParseGlobalConfig(userRequest(t, test.body))
			if err == nil {
				t.Fatal("ParseGlobalConfig = nil, want error")
			}
			if err.Code != domain.CodeConfigInvalid {
				t.Errorf("Code = %s, want %s", err.Code, domain.CodeConfigInvalid)
			}
			if validateErr := err.Validate(); validateErr != nil {
				t.Errorf("typed errorがValidateで落ちた: %v", validateErr)
			}
		})
	}
}

// TestParseGlobalConfigReportsPosition は§1の「位置付き」診断を確かめる。
func TestParseGlobalConfigReportsPosition(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"unknown key", "schema = 1\n[application]\ntheme = \"dark\"\n"},
		{"型違い", "schema = 1\n[application]\ncolor = true\n"},
		{"重複key", "schema = 1\n[application]\ncolor = \"auto\"\ncolor = \"never\"\n"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := ParseGlobalConfig(userRequest(t, test.body))
			if err == nil {
				t.Fatal("ParseGlobalConfig = nil, want error")
			}
			detail, ok := err.Parameters["detail"].Str()
			if !ok {
				t.Fatalf("detail parameterが無い: %+v", err.Parameters)
			}
			if !strings.Contains(detail, "行") || !strings.Contains(detail, "列") {
				t.Errorf("detail = %q に行・列が含まれない", detail)
			}
		})
	}
}

// TestParseGlobalConfigUserDataRoot は§3.2の受入条件を固定する。
func TestParseGlobalConfigUserDataRoot(t *testing.T) {
	body := "schema = 1\n[paths]\nuser_data_root = \"/srv/managed\"\n"

	got, err := ParseGlobalConfig(userRequest(t, body))
	if err != nil {
		t.Fatalf("ParseGlobalConfig = %v, want nil", err)
	}
	if got.UserDataRoot != "/srv/managed" {
		t.Errorf("UserDataRoot = %q", got.UserDataRoot)
	}

	// §3.2「user modeだけに許し、portableでは非空を拒否する」
	_, portableErr := ParseGlobalConfig(GlobalRequest{
		Data: []byte(body), Mode: domain.ModePortable, Host: linuxHost(t),
	})
	if portableErr == nil {
		t.Fatal("portableで非空のuser_data_rootが通った")
	}
	if portableErr.Code != domain.CodeConfigInvalid {
		t.Errorf("Code = %s", portableErr.Code)
	}

	// portableでも空なら通る。既定値のまま書いたfileを拒否しないためである。
	if _, emptyErr := ParseGlobalConfig(GlobalRequest{
		Data: []byte("schema = 1\n[paths]\nuser_data_root = \"\"\n"),
		Mode: domain.ModePortable,
		Host: linuxHost(t),
	}); emptyErr != nil {
		t.Errorf("portableで空のuser_data_rootが落ちた: %v", emptyErr)
	}

	// 相対pathは拒否する。
	if _, relErr := ParseGlobalConfig(userRequest(t,
		"schema = 1\n[paths]\nuser_data_root = \"relative/dir\"\n")); relErr == nil {
		t.Error("相対pathのuser_data_rootが通った")
	}

	// Windowsのdrive付きpathはWindows hostで通る。
	if _, winErr := ParseGlobalConfig(GlobalRequest{
		Data: []byte("schema = 1\n[paths]\nuser_data_root = \"D:\\\\managed\"\n"),
		Mode: domain.ModeUser,
		Host: windowsHost(t),
	}); winErr != nil {
		t.Errorf("Windowsのabsolute pathが落ちた: %v", winErr)
	}
}

// TestParseGlobalConfigRejectsOversizedFile はdocs/04-storage-and-data.md §21の
// 「global/project TOML各file 1 MiB」を固定する。
func TestParseGlobalConfigRejectsOversizedFile(t *testing.T) {
	// 上限ちょうどは通す。commentで埋めてschemaだけ有効にする。
	head := "schema = 1\n"
	filler := strings.Repeat("#", ConfigFileMaxBytes-len(head)-1) + "\n"
	if _, atLimit := ParseGlobalConfig(userRequest(t, head+filler)); atLimit != nil {
		t.Fatalf("上限ちょうどが落ちた: %v", atLimit)
	}

	over := head + strings.Repeat("#", ConfigFileMaxBytes-len(head)) + "\n"
	_, err := ParseGlobalConfig(userRequest(t, over))
	if err == nil {
		t.Fatal("上限超過が通った")
	}
	if err.Code != domain.CodeConfigInvalid {
		t.Errorf("Code = %s", err.Code)
	}
}
