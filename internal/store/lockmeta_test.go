package store

import (
	"strings"
	"testing"
	"time"
)

// specLockJSON はdocs/04-storage-and-data.md §19の例そのものである。
const specLockJSON = `{"schema":1,"lock_id":"44444444444444444444444444444444",` +
	`"role":"state","operation_id":"22222222222222222222222222222222",` +
	`"pid":1234,"created_at":"2026-08-07T09:00:00Z"}`

func TestParseLockMetadataAcceptsSpecExample(t *testing.T) {
	value, err := ParseLockMetadata([]byte(specLockJSON))
	if err != nil {
		t.Fatalf("ParseLockMetadata = %s", describe(err))
	}
	if value.LockID != "44444444444444444444444444444444" || value.Role != "state" {
		t.Errorf("lock_id/role = %q/%q", value.LockID, value.Role)
	}
	if value.PID != 1234 {
		t.Errorf("pid = %d", value.PID)
	}
	if value.Operation.String() != "22222222222222222222222222222222" {
		t.Errorf("operation_id = %q", value.Operation)
	}
}

func TestLockMetadataRoundTrip(t *testing.T) {
	value, parseErr := ParseLockMetadata([]byte(specLockJSON))
	if parseErr != nil {
		t.Fatalf("ParseLockMetadata = %s", describe(parseErr))
	}
	data, encodeErr := EncodeLockMetadata(value)
	if encodeErr != nil {
		t.Fatalf("EncodeLockMetadata = %s", describe(encodeErr))
	}
	again, reparseErr := ParseLockMetadata(data)
	if reparseErr != nil {
		t.Fatalf("再parse = %s\n%s", describe(reparseErr), data)
	}
	if again != value {
		t.Errorf("round tripで値が変わった\n%+v\n%+v", again, value)
	}
	assertTrailingLF(t, data)
}

// TestLockMetadataAcceptsEveryRole は§12の6分類すべてのroleを固定する。
func TestLockMetadataAcceptsEveryRole(t *testing.T) {
	roles := []string{
		"state", "setup", "shim",
		"catalog~node",
		"install~node~22.18.0~linux-amd64-glibc",
		"storage~node~global-packages",
	}
	for _, role := range roles {
		t.Run(role, func(t *testing.T) {
			source := strings.Replace(specLockJSON, `"role":"state"`, `"role":"`+role+`"`, 1)
			value, err := ParseLockMetadata([]byte(source))
			if err != nil {
				t.Fatalf("role %q が落ちた: %s", role, describe(err))
			}
			if value.Role != role {
				t.Errorf("role = %q, want %q", value.Role, role)
			}
		})
	}
}

// TestLockMetadataRejectsMalformedRole は§19の「role grammarは§12」を固定する。
//
// 未知classや正規形でないroleのlock fileを診断へそのまま出すと、存在しない
// lockを保持しているように見える。
func TestLockMetadataRejectsMalformedRole(t *testing.T) {
	rejects := []struct{ name, role string }{
		{"未定義class", "payload"},
		{"空", ""},
		{"qualifier不要classにqualifier", "state~node"},
		{"qualifier必須classにqualifier無し", "catalog"},
		{"qualifierが空", "catalog~"},
		{"path区切りを含む", "catalog~a/b"},
		{"相対参照", "catalog~.."},
		{"class名が大文字", "State"},
	}
	for _, test := range rejects {
		t.Run(test.name, func(t *testing.T) {
			source := strings.Replace(specLockJSON, `"role":"state"`, `"role":"`+test.role+`"`, 1)
			if _, err := ParseLockMetadata([]byte(source)); err == nil {
				t.Errorf("role %q が通った", test.role)
			}
		})
	}
}

// TestParseLockMetadataRejects は§19のexact keyと値制約を固定する。
func TestParseLockMetadataRejects(t *testing.T) {
	tests := []struct{ name, json string }{
		{"unknown key", strings.Replace(specLockJSON, `"schema":1,`, `"schema":1,"extra":1,`, 1)},
		{"重複key", strings.Replace(specLockJSON, `"schema":1,`, `"schema":1,"schema":2,`, 1)},
		{"schemaが2", strings.Replace(specLockJSON, `"schema":1`, `"schema":2`, 1)},
		{"lock_idが不正", strings.Replace(specLockJSON, "44444444444444444444444444444444", "4444", 1)},
		{"lock_idが大文字", strings.Replace(specLockJSON,
			"44444444444444444444444444444444", "4444444444444444444444444444444A", 1)},
		{"operation_idが不正", strings.Replace(specLockJSON, "22222222222222222222222222222222", "2222", 1)},
		{"pidが0", strings.Replace(specLockJSON, `"pid":1234`, `"pid":0`, 1)},
		{"pidが負", strings.Replace(specLockJSON, `"pid":1234`, `"pid":-1`, 1)},
		{"pidが小数", strings.Replace(specLockJSON, `"pid":1234`, `"pid":12.5`, 1)},
		{"pidが欠落", strings.Replace(specLockJSON, `"pid":1234,`, "", 1)},
		{"created_at非UTC", strings.Replace(specLockJSON,
			`"created_at":"2026-08-07T09:00:00Z"`, `"created_at":"2026-08-07T09:00:00+09:00"`, 1)},
		{"created_atがzero time", strings.Replace(specLockJSON,
			`"created_at":"2026-08-07T09:00:00Z"`, `"created_at":"0001-01-01T00:00:00Z"`, 1)},
		{"trailing data", specLockJSON + specLockJSON},
		{"BOM付き", "\ufeff" + specLockJSON},
		{"空", ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := ParseLockMetadata([]byte(test.json)); err == nil {
				t.Error("ParseLockMetadata = nil, want error")
			}
		})
	}
}

// TestLockMetadataErrorIsInternal は破損時のerror codeを固定する。
//
// lock fileは診断metadataであり、読めなくてもOS lockによる排他性は失われない。
// `E_STATE_CORRUPT`にして操作全体を止める扱いにしない。
func TestLockMetadataErrorIsInternal(t *testing.T) {
	_, err := ParseLockMetadata([]byte(`{"schema":2}`))
	if err == nil {
		t.Fatal("schema 2が通った")
	}
	if err.Code != "E_INTERNAL" {
		t.Errorf("code = %q, want E_INTERNAL", err.Code)
	}
	if err.PathRole != "state" {
		t.Errorf("path role = %q, want state", err.PathRole)
	}
	if len(err.Parameters) != 0 {
		t.Errorf("parametersが空でない: %v", err.Parameters)
	}
}

// TestEncodeLockMetadataRejectsInvalidValue はencode経路の検査を固定する。
func TestEncodeLockMetadataRejectsInvalidValue(t *testing.T) {
	base, parseErr := ParseLockMetadata([]byte(specLockJSON))
	if parseErr != nil {
		t.Fatalf("ParseLockMetadata = %s", describe(parseErr))
	}
	if _, err := EncodeLockMetadata(base); err != nil {
		t.Fatalf("正当な値が落ちた: %s", describe(err))
	}
	tests := []struct {
		name   string
		mutate func(*LockMetadata)
	}{
		{"lock_idが空", func(m *LockMetadata) { m.LockID = "" }},
		{"roleが未定義class", func(m *LockMetadata) { m.Role = "payload" }},
		{"roleが正規形でない", func(m *LockMetadata) { m.Role = "state~x" }},
		{"pidが0", func(m *LockMetadata) { m.PID = 0 }},
		{"created_atがzero", func(m *LockMetadata) { m.CreatedAt = time.Time{} }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := base
			test.mutate(&value)
			if _, err := EncodeLockMetadata(value); err == nil {
				t.Error("EncodeLockMetadata = nil, want error")
			}
		})
	}
}
