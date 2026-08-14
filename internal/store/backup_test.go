package store

import (
	"encoding/base64"
	"strings"
	"testing"
)

// specBackupTOML はdocs/04-storage-and-data.md §10の例そのものである。
const specBackupTOML = `schema = 1
backup_id = "abcdef0123456789abcdef0123456789"
root_id = "0123456789abcdef0123456789abcdef"
kind = "windows-user-path"
created_at = "2026-08-07T09:00:00Z"
target = "HKCU\\Environment\\Path"
existed = true
value_type = "REG_EXPAND_SZ"
raw_bytes_base64 = "VABFAFMAVAA="
sha256 = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
`

func TestParseSetupBackupAcceptsSpecExample(t *testing.T) {
	value, err := ParseSetupBackup([]byte(specBackupTOML))
	if err != nil {
		t.Fatalf("ParseSetupBackup = %v", err)
	}
	if value.Kind != BackupWindowsUserPath || !value.Existed {
		t.Errorf("kind/existed = %q/%v", value.Kind, value.Existed)
	}
	if value.Target != `HKCU\Environment\Path` || value.ValueType != "REG_EXPAND_SZ" {
		t.Errorf("target/value_type = %q/%q", value.Target, value.ValueType)
	}
	// `VABFAFMAVAA=`はUTF-16LEの"TEST"である。registry valueのraw bytesを
	// そのまま持つという§10の規定を、decode結果で確かめる。
	want := []byte{'T', 0, 'E', 0, 'S', 0, 'T', 0}
	if string(value.Raw) != string(want) {
		t.Errorf("raw = %v, want %v", value.Raw, want)
	}
}

func TestSetupBackupRoundTrip(t *testing.T) {
	value, parseErr := ParseSetupBackup([]byte(specBackupTOML))
	if parseErr != nil {
		t.Fatalf("ParseSetupBackup = %v", parseErr)
	}
	data, encodeErr := EncodeSetupBackup(value)
	if encodeErr != nil {
		t.Fatalf("EncodeSetupBackup = %v", encodeErr)
	}
	again, reparseErr := ParseSetupBackup(data)
	if reparseErr != nil {
		t.Fatalf("再parse = %v\n%s", reparseErr, data)
	}
	if again.BackupID != value.BackupID || string(again.Raw) != string(value.Raw) {
		t.Errorf("round tripで値が変わった\n%+v\n%+v", again, value)
	}
	assertTrailingLF(t, data)
}

// TestSetupBackupAbsentTarget は§10の「不存在は`existed=false`, raw空, digest 64 zero」
// を1通りの表現に固定する。
func TestSetupBackupAbsentTarget(t *testing.T) {
	zero := strings.Repeat("0", 64)
	absent := strings.NewReplacer(
		"existed = true", "existed = false",
		`value_type = "REG_EXPAND_SZ"`, `value_type = ""`,
		`raw_bytes_base64 = "VABFAFMAVAA="`, `raw_bytes_base64 = ""`,
		testDigestA, zero,
	).Replace(specBackupTOML)
	value, err := ParseSetupBackup([]byte(absent))
	if err != nil {
		t.Fatalf("不存在backupが落ちた: %v", err)
	}
	if value.Existed || len(value.Raw) != 0 || value.SHA256 != zero {
		t.Errorf("不存在の表現がずれている: %+v", value)
	}

	rejects := []struct {
		name string
		toml string
	}{
		{"existed=falseなのにrawがある",
			strings.Replace(absent, `raw_bytes_base64 = ""`, `raw_bytes_base64 = "VABFAFMAVAA="`, 1)},
		{"existed=falseなのにdigestが非zero",
			strings.Replace(absent, `sha256 = "`+zero+`"`, `sha256 = "`+testDigestA+`"`, 1)},
		{"existed=falseなのにvalue_typeがある",
			strings.Replace(absent, `value_type = ""`, `value_type = "REG_SZ"`, 1)},
		{"existed=trueなのにdigestが64 zero",
			strings.Replace(specBackupTOML, `sha256 = "`+testDigestA+`"`, `sha256 = "`+zero+`"`, 1)},
		{"existed=trueのwindows backupにvalue_typeが無い",
			strings.Replace(specBackupTOML, `value_type = "REG_EXPAND_SZ"`, `value_type = ""`, 1)},
	}
	for _, test := range rejects {
		t.Run(test.name, func(t *testing.T) {
			if _, err := ParseSetupBackup([]byte(test.toml)); err == nil {
				t.Error("ParseSetupBackup = nil, want error")
			}
		})
	}
}

// TestSetupBackupShellProfileHasNoValueType はvalue typeがregistry専用であることを固定する。
func TestSetupBackupShellProfileHasNoValueType(t *testing.T) {
	profile := strings.NewReplacer(
		`kind = "windows-user-path"`, `kind = "shell-profile"`,
		`target = "HKCU\\Environment\\Path"`, `target = "/home/u/.bashrc"`,
		`value_type = "REG_EXPAND_SZ"`, `value_type = ""`,
	).Replace(specBackupTOML)
	if _, err := ParseSetupBackup([]byte(profile)); err != nil {
		t.Fatalf("shell-profile backupが落ちた: %v", err)
	}
	withType := strings.Replace(profile, `value_type = ""`, `value_type = "REG_SZ"`, 1)
	if _, err := ParseSetupBackup([]byte(withType)); err == nil {
		t.Error("shell-profileにvalue_typeがあるのに通った")
	}
}

// TestSetupBackupRejects は§10のexact keyとbase64制約を固定する。
func TestSetupBackupRejects(t *testing.T) {
	tests := []struct {
		name string
		toml string
	}{
		{"unknown key", specBackupTOML + "extra = 1\n"},
		{"kind enum外", strings.Replace(specBackupTOML, `kind = "windows-user-path"`, `kind = "registry"`, 1)},
		{"target空", strings.Replace(specBackupTOML, `target = "HKCU\\Environment\\Path"`, `target = ""`, 1)},
		{"base64でない", strings.Replace(specBackupTOML, `raw_bytes_base64 = "VABFAFMAVAA="`,
			`raw_bytes_base64 = "not base64!"`, 1)},
		{"padding無しbase64", strings.Replace(specBackupTOML, `raw_bytes_base64 = "VABFAFMAVAA="`,
			`raw_bytes_base64 = "VABFAFMAVAA"`, 1)},
		{"backup_id不正", strings.Replace(specBackupTOML, testBackupID, "xyz", 1)},
		{"existed欠落", strings.Replace(specBackupTOML, "existed = true\n", "", 1)},
		{"existed型違い", strings.Replace(specBackupTOML, "existed = true", `existed = "true"`, 1)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := ParseSetupBackup([]byte(test.toml)); err == nil {
				t.Error("ParseSetupBackup = nil, want error")
			}
		})
	}
}

// TestSetupBackupRawSizeLimit はdecode後のsize上限を固定する（§10）。
//
// encode後ではなくdecode後で見ることを確かめる。base64は4/3へ膨らむため、
// encode後のみを見ていると上限を実質1.33倍に緩めてしまう。
func TestSetupBackupRawSizeLimit(t *testing.T) {
	oversized := base64.StdEncoding.EncodeToString(make([]byte, BackupRawMaxBytes+1))
	source := strings.Replace(specBackupTOML,
		`raw_bytes_base64 = "VABFAFMAVAA="`, `raw_bytes_base64 = "`+oversized+`"`, 1)
	if _, err := ParseSetupBackup([]byte(source)); err == nil {
		t.Error("decode後の上限超過が通った")
	}

	exact := base64.StdEncoding.EncodeToString(make([]byte, BackupRawMaxBytes))
	atLimit := strings.Replace(specBackupTOML,
		`raw_bytes_base64 = "VABFAFMAVAA="`, `raw_bytes_base64 = "`+exact+`"`, 1)
	if _, err := ParseSetupBackup([]byte(atLimit)); err != nil {
		t.Errorf("上限ちょうどが落ちた: %v", err)
	}
}
