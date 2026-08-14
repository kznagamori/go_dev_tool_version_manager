package store

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
)

// TestEnvelopeRoundTripsEveryCommand は5 commandすべてのdataが往復することを固定する。
//
// §17がcommandごとに別の`data`形を定めるため、1 commandだけの往復では
// 残り4つのencode経路が検査されない。表示だけに使うfieldでも、書けない値を
// 書けてしまうと利用者が誤った情報を見る。
func TestEnvelopeRoundTripsEveryCommand(t *testing.T) {
	tests := []struct {
		command string
		data    string
		check   func(*testing.T, Envelope)
	}{
		{
			command: "available",
			data: `{"tool_id": "node", "platform_id": "windows-amd64", "items": [` +
				availableItemFixture() + `]}`,
			check: func(t *testing.T, value Envelope) {
				if value.Available == nil || len(value.Available.Items) != 1 {
					t.Fatalf("available = %+v", value.Available)
				}
				item := value.Available.Items[0]
				// CLI JSONはschemeを持たないためVersionはzero、textだけが入る。
				if !item.Version.IsZero() {
					t.Error("CLI JSON由来のitemにVersionが入っている")
				}
				if item.VersionText != "22.18.0" {
					t.Errorf("VersionText = %q", item.VersionText)
				}
				if item.Lifecycle != "supported" || !item.Installable {
					t.Errorf("item = %+v", item)
				}
			},
		},
		{
			command: "installed",
			data:    `{"installs": [` + installFixture() + `]}`,
			check: func(t *testing.T, value Envelope) {
				if len(value.Installs) != 1 {
					t.Fatalf("installs = %+v", value.Installs)
				}
				install := value.Installs[0]
				if install.Ref.Tool.String() != "node" || install.DiskSize != 12345 {
					t.Errorf("install = %+v", install)
				}
				if !install.Selected || install.Provider != ProviderOfficial {
					t.Errorf("selected/provider = %v/%q", install.Selected, install.Provider)
				}
			},
		},
		{
			command: "current",
			data: `{"selections": [{
				"source": "project",
				"project_file": {"role": "project-file", "path": "/work/.gdtvm.toml"},
				"tool_id": "node", "version": "22.18.0", "install_id": "` + testInstall + `",
				"payload_path": {"role": "payload", "path": "/data/gdtvm/tools/node/payload"},
				"health": "healthy"}]}`,
			check: func(t *testing.T, value Envelope) {
				if len(value.Selections) != 1 {
					t.Fatalf("selections = %+v", value.Selections)
				}
				if value.Selections[0].Source != "project" {
					t.Errorf("source = %q", value.Selections[0].Source)
				}
				if value.Selections[0].ProjectFile.Path() != "/work/.gdtvm.toml" {
					t.Errorf("project_file = %q", value.Selections[0].ProjectFile.Path())
				}
			},
		},
		{
			command: "doctor",
			data: `{"status": "degraded", "diagnostics": ` + diagnosticsWithPaths() +
				`, "report_path": {"role": "report", "path": "/data/gdtvm/report.md"}}`,
			check: func(t *testing.T, value Envelope) {
				if value.Doctor == nil || value.Doctor.Status != DoctorDegraded {
					t.Fatalf("doctor = %+v", value.Doctor)
				}
				if len(value.Doctor.Diagnostics) != DiagnosticCodeCount {
					t.Errorf("diagnostics件数 = %d", len(value.Doctor.Diagnostics))
				}
				if len(value.Doctor.Diagnostics[0].Paths) != 1 {
					t.Errorf("paths = %+v", value.Doctor.Diagnostics[0].Paths)
				}
				if value.Doctor.ReportPath.Path() != "/data/gdtvm/report.md" {
					t.Errorf("report_path = %q", value.Doctor.ReportPath.Path())
				}
			},
		},
		{
			command: "version",
			data:    `{"build": ` + buildFixture() + `}`,
			check: func(t *testing.T, value Envelope) {
				if value.Build == nil || value.Build.GoVersion != "go1.26.5" {
					t.Fatalf("build = %+v", value.Build)
				}
				if value.Build.Development {
					t.Error("release versionなのにdevelopment=true")
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.command, func(t *testing.T) {
			source := envelopeWith(test.command, test.data)
			value, parseErr := DecodeEnvelope([]byte(source))
			if parseErr != nil {
				t.Fatalf("DecodeEnvelope = %s", describe(parseErr))
			}
			test.check(t, value)

			data, encodeErr := EncodeEnvelope(value)
			if encodeErr != nil {
				t.Fatalf("EncodeEnvelope = %s", describe(encodeErr))
			}
			again, reparseErr := DecodeEnvelope(data)
			if reparseErr != nil {
				t.Fatalf("再parse = %s\n%s", describe(reparseErr), data)
			}
			test.check(t, again)
			assertTrailingLF(t, data)

			// 同じ値から同じbyte列が出る。
			second, secondErr := EncodeEnvelope(value)
			if secondErr != nil {
				t.Fatalf("EncodeEnvelope = %s", describe(secondErr))
			}
			if string(second) != string(data) {
				t.Error("出力が決定的でない")
			}
			// commandに対応しないkeyが`data`直下へ出ていない（§17）。
			// 部分文字列ではなく`data`の直下keyだけを見る。`tool_id`のような名前は
			// `installs[]`のentry内には正当に現れる。
			assertDataKeys(t, data, test.command)
		})
	}
}

// assertDataKeys は`data`直下のkeyがcommandの契約と一致することを確かめる（§17）。
func assertDataKeys(t *testing.T, document []byte, command string) {
	t.Helper()
	var envelope struct {
		Data map[string]json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(document, &envelope); err != nil {
		t.Fatalf("出力をJSONとして読めない: %v", err)
	}
	want := map[string][]string{
		"available": {"tool_id", "platform_id", "items"},
		"installed": {"installs"},
		"current":   {"selections"},
		"doctor":    {"status", "diagnostics", "report_path"},
		"version":   {"build"},
	}[command]

	expected := make(map[string]struct{}, len(want))
	for _, key := range want {
		expected[key] = struct{}{}
		if _, ok := envelope.Data[key]; !ok {
			t.Errorf("command %q のdataに %q が無い\n%s", command, key, document)
		}
	}
	for key := range envelope.Data {
		if _, ok := expected[key]; !ok {
			t.Errorf("command %q のdataに %q が現れた\n%s", command, key, document)
		}
	}
}

func availableItemFixture() string {
	return `{
		"version": "22.18.0",
		"channel": "stable",
		"lifecycle": "supported",
		"lifecycle_evidence": "https://github.com/nodejs/Release",
		"lifecycle_assessed_at": "2026-08-07T00:00:00Z",
		"published_at": "2026-07-01T00:00:00Z",
		"installable": true,
		"unavailable_reason": "",
		"provider_kind": "official",
		"provider_release": "v22.18.0",
		"artifact_file": "node-v22.18.0-win-x64.zip",
		"artifact_url": "https://nodejs.org/dist/v22.18.0/node.zip",
		"artifact_size": 1,
		"artifact_digest": "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
		"checksum_source": "text-file"
	}`
}

func installFixture() string {
	return `{
		"tool_id": "node", "version": "22.18.0", "platform_id": "windows-amd64",
		"install_id": "` + testInstall + `", "installed_at": "2026-08-07T09:00:00Z",
		"health": "healthy",
		"receipt_path": {"role": "receipt", "path": "D:\\gdtvm\\.gdtvm-install.toml"},
		"disk_size": 12345, "provider_kind": "official", "selected": true
	}`
}

// diagnosticsWithPaths はwarnを1件含み、pathを持つdiagnosticsを作る。
func diagnosticsWithPaths() string {
	entries := make([]string, 0, DiagnosticCodeCount)
	for index, code := range diagnosticCodeOrder {
		severity := "info"
		paths := "[]"
		if index == 0 {
			severity = "warn"
			paths = `[{"role": "shim", "path": "/data/gdtvm/shims"}]`
		}
		entries = append(entries, `{"code": "`+string(code)+`", "severity": "`+severity+
			`", "message_id": "doctor.checked", "parameters": {"count": 3}, "paths": `+paths+`}`)
	}
	return "[" + strings.Join(entries, ",") + "]"
}

// TestAvailableItemsUseCatalogKeys は§17の「`CatalogItem`は§15 itemのexact key集合」を
// 固定する。
func TestAvailableItemsUseCatalogKeys(t *testing.T) {
	source := envelopeWith("available",
		`{"tool_id": "node", "platform_id": "windows-amd64", "items": [`+availableItemFixture()+`]}`)
	rejects := []struct{ name, json string }{
		{"unknown item key",
			strings.Replace(source, `"version": "22.18.0",`, `"version": "22.18.0", "extra": 1,`, 1)},
		{"lifecycle_evidenceが非HTTPS",
			strings.Replace(source, `"https://github.com/nodejs/Release"`, `"http://github.com/nodejs/Release"`, 1)},
		{"artifact_fileに区切り",
			strings.Replace(source, `"node-v22.18.0-win-x64.zip"`, `"dist/node.zip"`, 1)},
		{"versionが範囲指定",
			strings.Replace(source, `"version": "22.18.0",`, `"version": "^22.18.0",`, 1)},
		{"installableなのにreasonがある",
			strings.Replace(source, `"unavailable_reason": ""`, `"unavailable_reason": "catalog.missing"`, 1)},
	}
	for _, test := range rejects {
		t.Run(test.name, func(t *testing.T) {
			if _, err := DecodeEnvelope([]byte(test.json)); err == nil {
				t.Error("DecodeEnvelope = nil, want error")
			}
		})
	}
}

// TestEncodeEnvelopeRejectsInvalidValue はencode経路の検査を固定する。
func TestEncodeEnvelopeRejectsInvalidValue(t *testing.T) {
	base, parseErr := DecodeEnvelope([]byte(envelopeWith("installed",
		`{"installs": [`+installFixture()+`]}`)))
	if parseErr != nil {
		t.Fatalf("DecodeEnvelope = %s", describe(parseErr))
	}

	tests := []struct {
		name   string
		mutate func(*Envelope)
	}{
		{"commandとdataが不一致", func(e *Envelope) { e.Command = CommandCurrent }},
		{"ok=falseなのにErrorが無い", func(e *Envelope) { e.OK = false }},
		{"install_idが不正", func(e *Envelope) { e.Installs[0].InstallID = "x" }},
		{"receipt_pathが相対", func(e *Envelope) {
			path, err := domain.NewPathValue(domain.RoleReceipt, "tools/x")
			if err != nil {
				t.Fatalf("path作成に失敗した: %v", err)
			}
			e.Installs[0].ReceiptPath = path
		}},
		{"healthがenum外", func(e *Envelope) { e.Installs[0].Health = "broken" }},
		{"installsがtuple順でない", func(e *Envelope) {
			second := e.Installs[0]
			second.Ref.Version = "20.0.0"
			e.Installs = []InstallSummary{e.Installs[0], second}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := base
			value.Installs = append([]InstallSummary(nil), base.Installs...)
			test.mutate(&value)
			if _, err := EncodeEnvelope(value); err == nil {
				t.Error("EncodeEnvelope = nil, want error")
			}
		})
	}
}
