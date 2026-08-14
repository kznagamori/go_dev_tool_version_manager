package store

import (
	"strings"
	"testing"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
)

// specSuccessEnvelope はdocs/04-storage-and-data.md §17の成功例そのものである。
//
// 例の`data`は`{}`だが、§17は`installed`のexact keyを`installs`と定める。
// codecはexact keyを検査するため、例の`{}`は拒否される。§17自身が
// 「成功時`data`のexact keyとvalue typeを次に固定する」と定めており、
// 例はtop-levelの形だけを示している。
const specSuccessEnvelope = `{
  "schema": 1,
  "ok": true,
  "command": "installed",
  "invocation_id": "33333333333333333333333333333333",
  "data": {"installs": []},
  "warnings": []
}`

// specFailureEnvelope はdocs/04-storage-and-data.md §17の失敗例そのものである。
const specFailureEnvelope = `{
  "schema": 1,
  "ok": false,
  "command": "installed",
  "invocation_id": "33333333333333333333333333333333",
  "error": {
    "code": "E_STATE_CORRUPT",
    "message_id": "error.state_corrupt",
    "parameters": {},
    "retryable": false
  },
  "warnings": []
}`

func envelopeErrorOf(source string) *domain.Error {
	_, err := DecodeEnvelope([]byte(source))
	return err
}

func TestDecodeEnvelopeAcceptsSpecExamples(t *testing.T) {
	success, err := DecodeEnvelope([]byte(specSuccessEnvelope))
	if err != nil {
		t.Fatalf("成功envelope = %s", describe(err))
	}
	if !success.OK || success.Command != CommandInstalled {
		t.Errorf("ok/command = %v/%q", success.OK, success.Command)
	}
	if success.Error != nil {
		t.Error("成功なのにerrorがある")
	}

	failure, err := DecodeEnvelope([]byte(specFailureEnvelope))
	if err != nil {
		t.Fatalf("失敗envelope = %s", describe(err))
	}
	if failure.OK || failure.Error == nil {
		t.Fatalf("ok/error = %v/%+v", failure.OK, failure.Error)
	}
	if failure.Error.Code != "E_STATE_CORRUPT" || failure.Error.Retryable {
		t.Errorf("error = %+v", failure.Error)
	}
}

func TestEnvelopeRoundTrip(t *testing.T) {
	for _, source := range []string{specSuccessEnvelope, specFailureEnvelope} {
		value, parseErr := DecodeEnvelope([]byte(source))
		if parseErr != nil {
			t.Fatalf("DecodeEnvelope = %s", describe(parseErr))
		}
		data, encodeErr := EncodeEnvelope(value)
		if encodeErr != nil {
			t.Fatalf("EncodeEnvelope = %s", describe(encodeErr))
		}
		again, reparseErr := DecodeEnvelope(data)
		if reparseErr != nil {
			t.Fatalf("再parse = %s\n%s", describe(reparseErr), data)
		}
		if again.OK != value.OK || again.Command != value.Command {
			t.Errorf("round tripで変わった: %+v", again)
		}
		assertTrailingLF(t, data)
	}
}

// TestEnvelopeDataErrorExclusivity は§17の「data/errorを排他にする」を固定する。
func TestEnvelopeDataErrorExclusivity(t *testing.T) {
	rejects := []struct {
		name string
		json string
	}{
		{"ok=trueでerrorがある",
			strings.Replace(specSuccessEnvelope, `"data": {"installs": []},`,
				`"data": {"installs": []}, "error": {"code": "E_INTERNAL", "message_id": "error.internal", "parameters": {}, "retryable": false},`, 1)},
		{"ok=trueでdataが無い",
			strings.Replace(specSuccessEnvelope, `"data": {"installs": []},`, "", 1)},
		{"ok=falseでdataがある",
			strings.Replace(specFailureEnvelope, `"error": {`,
				`"data": {"installs": []}, "error": {`, 1)},
		{"ok=falseでerrorが無い",
			`{"schema":1,"ok":false,"command":"installed","invocation_id":"33333333333333333333333333333333","warnings":[]}`},
	}
	for _, test := range rejects {
		t.Run(test.name, func(t *testing.T) {
			if _, err := DecodeEnvelope([]byte(test.json)); err == nil {
				t.Error("DecodeEnvelope = nil, want error")
			}
		})
	}
}

// TestEnvelopeDataKeysMatchCommand は§17のcommandごとのexact keyを固定する。
func TestEnvelopeDataKeysMatchCommand(t *testing.T) {
	accepts := []struct {
		command string
		data    string
	}{
		{"installed", `{"installs": []}`},
		{"current", `{"selections": []}`},
		{"version", `{"build": ` + buildFixture() + `}`},
		{"available", `{"tool_id": "node", "platform_id": "windows-amd64", "items": []}`},
		{"doctor", `{"status": "healthy", "diagnostics": ` + diagnosticsFixture(SeverityInfo) +
			`, "report_path": {"role": "report", "path": ""}}`},
	}
	for _, test := range accepts {
		t.Run("受理/"+test.command, func(t *testing.T) {
			source := envelopeWith(test.command, test.data)
			if _, err := DecodeEnvelope([]byte(source)); err != nil {
				t.Errorf("command %q が落ちた: %s", test.command, describe(envelopeErrorOf(source)))
			}
		})
	}

	rejects := []struct {
		name    string
		command string
		data    string
	}{
		{"installedにselections", "installed", `{"selections": []}`},
		{"installedにinstallsとselections", "installed", `{"installs": [], "selections": []}`},
		{"currentにinstalls", "current", `{"installs": []}`},
		{"availableにitemsだけ", "available", `{"items": []}`},
		{"versionにbuildが無い", "version", `{}`},
		{"doctorにreport_pathが無い", "doctor",
			`{"status": "healthy", "diagnostics": ` + diagnosticsFixture(SeverityInfo) + `}`},
	}
	for _, test := range rejects {
		t.Run("拒否/"+test.name, func(t *testing.T) {
			if _, err := DecodeEnvelope([]byte(envelopeWith(test.command, test.data))); err == nil {
				t.Error("DecodeEnvelope = nil, want error")
			}
		})
	}
}

func envelopeWith(command, data string) string {
	return `{"schema":1,"ok":true,"command":"` + command +
		`","invocation_id":"33333333333333333333333333333333","data":` + data + `,"warnings":[]}`
}

func buildFixture() string {
	return `{"version": "2026.08.07.00", "commit": "0123456789abcdef0123456789abcdef01234567",
		"build_time": "2026-08-07T09:00:00Z", "go_version": "go1.26.5",
		"platform_id": "linux-amd64-glibc", "state_schema": 1, "definition_schema": 1,
		"registry_schema": 1, "development": false}`
}

// diagnosticsFixture は§17.1の10件をcode順で作る。
func diagnosticsFixture(severity Severity) string {
	entries := make([]string, 0, DiagnosticCodeCount)
	for _, code := range diagnosticCodeOrder {
		entries = append(entries, `{"code": "`+string(code)+`", "severity": "`+string(severity)+
			`", "message_id": "doctor.ok", "parameters": {}, "paths": []}`)
	}
	return "[" + strings.Join(entries, ",") + "]"
}

// TestDoctorDiagnosticsAreExactlyTen は§17.1の「10件をcode順にexactly 1件ずつ」を
// 固定する。
func TestDoctorDiagnosticsAreExactlyTen(t *testing.T) {
	full := diagnosticsFixture(SeverityInfo)
	ok := envelopeWith("doctor", `{"status": "healthy", "diagnostics": `+full+
		`, "report_path": {"role": "report", "path": ""}}`)
	if _, err := DecodeEnvelope([]byte(ok)); err != nil {
		t.Fatalf("10件のdiagnosticsが落ちた: %s", describe(envelopeErrorOf(ok)))
	}

	// 1件減らす。検査していない項目を「問題なし」に見せないため拒否する。
	entries := strings.Split(strings.Trim(full, "[]"), "},{")
	short := "[" + strings.Join(entries[:len(entries)-1], "},{") + "}]"
	nine := envelopeWith("doctor", `{"status": "healthy", "diagnostics": `+short+
		`, "report_path": {"role": "report", "path": ""}}`)
	if _, err := DecodeEnvelope([]byte(nine)); err == nil {
		t.Error("9件のdiagnosticsが通った")
	}

	// code順でないと拒否する。
	swapped := strings.Replace(full, `"code": "D_PATH"`, `"code": "D_TMP"`, 1)
	swapped = strings.Replace(swapped, `{"code": "D_TMP", "severity": "info", "message_id": "doctor.ok", "parameters": {}, "paths": []}]`,
		`{"code": "D_PATH", "severity": "info", "message_id": "doctor.ok", "parameters": {}, "paths": []}]`, 1)
	outOfOrder := envelopeWith("doctor", `{"status": "healthy", "diagnostics": `+swapped+
		`, "report_path": {"role": "report", "path": ""}}`)
	if _, err := DecodeEnvelope([]byte(outOfOrder)); err == nil {
		t.Error("code順でないdiagnosticsが通った")
	}
}

// TestDoctorStatusMatchesSeverity は§17.1のstatus導出規則を固定する。
func TestDoctorStatusMatchesSeverity(t *testing.T) {
	doctor := func(status string, diagnostics string) string {
		return envelopeWith("doctor", `{"status": "`+status+`", "diagnostics": `+diagnostics+
			`, "report_path": {"role": "report", "path": ""}}`)
	}
	withWarn := strings.Replace(diagnosticsFixture(SeverityInfo),
		`"code": "D_PATH", "severity": "info"`, `"code": "D_PATH", "severity": "warn"`, 1)
	withError := strings.Replace(diagnosticsFixture(SeverityInfo),
		`"code": "D_PATH", "severity": "info"`, `"code": "D_PATH", "severity": "error"`, 1)

	accepts := []struct{ status, diagnostics string }{
		{"healthy", diagnosticsFixture(SeverityInfo)},
		{"degraded", withWarn},
		{"unhealthy", withError},
	}
	for _, test := range accepts {
		t.Run("受理/"+test.status, func(t *testing.T) {
			source := doctor(test.status, test.diagnostics)
			if _, err := DecodeEnvelope([]byte(source)); err != nil {
				t.Errorf("status %q が落ちた: %s", test.status, describe(envelopeErrorOf(source)))
			}
		})
	}
	rejects := []struct{ name, status, diagnostics string }{
		{"warnがあるのにhealthy", "healthy", withWarn},
		{"errorがあるのにdegraded", "degraded", withError},
		{"errorが無いのにunhealthy", "unhealthy", diagnosticsFixture(SeverityInfo)},
	}
	for _, test := range rejects {
		t.Run("拒否/"+test.name, func(t *testing.T) {
			if _, err := DecodeEnvelope([]byte(doctor(test.status, test.diagnostics))); err == nil {
				t.Error("DecodeEnvelope = nil, want error")
			}
		})
	}
}

// TestSelectionSummarySourceContract は§17のsource別契約を固定する。
func TestSelectionSummarySourceContract(t *testing.T) {
	selection := func(source, projectFile, version, installID, payloadPath string) string {
		return envelopeWith("current", `{"selections": [{
			"source": "`+source+`",
			"project_file": {"role": "project-file", "path": "`+projectFile+`"},
			"tool_id": "node", "version": "`+version+`", "install_id": "`+installID+`",
			"payload_path": {"role": "payload", "path": "`+payloadPath+`"},
			"health": "healthy"}]}`)
	}
	accepts := []struct{ name, json string }{
		{"project", selection("project", "/work/.gdtvm.toml", "22.18.0", testInstall, "/data/payload")},
		{"user", selection("user", "", "22.18.0", testInstall, "/data/payload")},
		{"none", selection("none", "", "", "", "")},
	}
	for _, test := range accepts {
		t.Run("受理/"+test.name, func(t *testing.T) {
			if _, err := DecodeEnvelope([]byte(test.json)); err != nil {
				t.Errorf("source %q が落ちた: %s", test.name, describe(envelopeErrorOf(test.json)))
			}
		})
	}
	rejects := []struct{ name, json string }{
		{"projectなのにproject_fileが空", selection("project", "", "22.18.0", testInstall, "/data/payload")},
		{"userなのにproject_fileがある", selection("user", "/work/.gdtvm.toml", "22.18.0", testInstall, "/data/payload")},
		{"noneなのにversionがある", selection("none", "", "22.18.0", "", "")},
		{"noneなのにinstall_idがある", selection("none", "", "", testInstall, "")},
		{"noneなのにpayload_pathがある", selection("none", "", "", "", "/data/payload")},
		{"userなのにversionが空", selection("user", "", "", testInstall, "/data/payload")},
		{"userなのにinstall_idが不正", selection("user", "", "22.18.0", "xyz", "/data/payload")},
	}
	for _, test := range rejects {
		t.Run("拒否/"+test.name, func(t *testing.T) {
			if _, err := DecodeEnvelope([]byte(test.json)); err == nil {
				t.Error("DecodeEnvelope = nil, want error")
			}
		})
	}
}

// TestEnvelopeRetryableMatchesCode はdocs/02-architecture.md §14の非retryable codeを
// 固定する。
func TestEnvelopeRetryableMatchesCode(t *testing.T) {
	nonRetryable := strings.Replace(specFailureEnvelope, `"retryable": false`, `"retryable": true`, 1)
	if _, err := DecodeEnvelope([]byte(nonRetryable)); err == nil {
		t.Error("E_STATE_CORRUPTにretryable=trueが通った")
	}
	retryable := strings.NewReplacer(
		`"code": "E_STATE_CORRUPT"`, `"code": "E_NETWORK"`,
		`"message_id": "error.state_corrupt"`, `"message_id": "error.network"`,
		`"retryable": false`, `"retryable": true`,
	).Replace(specFailureEnvelope)
	if _, err := DecodeEnvelope([]byte(retryable)); err != nil {
		t.Errorf("E_NETWORKのretryable=trueが落ちた: %s", describe(envelopeErrorOf(retryable)))
	}
}

// TestEnvelopeWarningsUseResultCodes は§17の「codeは§16.2の`ResultWarningCode`だけ」を
// 固定する。
func TestEnvelopeWarningsUseResultCodes(t *testing.T) {
	withWarning := strings.Replace(specSuccessEnvelope, `"warnings": []`,
		`"warnings": [{"code": "W_CACHE_STALE", "message_id": "warning.cache_stale", "parameters": {}}]`, 1)
	if _, err := DecodeEnvelope([]byte(withWarning)); err != nil {
		t.Fatalf("W_CACHE_STALEが落ちた: %s", describe(envelopeErrorOf(withWarning)))
	}
	// Plan warning codeはresult warningへ使えない（§16.1・§16.2）。
	planCode := strings.Replace(withWarning, `"code": "W_CACHE_STALE"`, `"code": "W_THIRD_PARTY"`, 1)
	if _, err := DecodeEnvelope([]byte(planCode)); err == nil {
		t.Error("Plan warning codeがresult warningとして通った")
	}
	// result warningはapproval fieldを持たない。
	withApproval := strings.Replace(withWarning, `"parameters": {}}]`,
		`"parameters": {}, "requires_explicit_approval": true}]`, 1)
	if _, err := DecodeEnvelope([]byte(withApproval)); err == nil {
		t.Error("result warningにapproval fieldがあるのに通った")
	}
}

// TestEnvelopePathValueContract は§17.2のrole固定とabsolute path要求を固定する。
func TestEnvelopePathValueContract(t *testing.T) {
	install := func(receiptPath string) string {
		return envelopeWith("installed", `{"installs": [{
			"tool_id": "node", "version": "22.18.0", "platform_id": "windows-amd64",
			"install_id": "`+testInstall+`", "installed_at": "2026-08-07T09:00:00Z",
			"health": "healthy", "receipt_path": `+receiptPath+`,
			"disk_size": 1, "provider_kind": "official", "selected": true}]}`)
	}
	if _, err := DecodeEnvelope([]byte(install(
		`{"role": "receipt", "path": "D:\\gdtvm\\.gdtvm-install.toml"}`))); err != nil {
		t.Fatalf("Windows absolute pathが落ちた: %s",
			describe(envelopeErrorOf(install(`{"role": "receipt", "path": "D:\\gdtvm\\.gdtvm-install.toml"}`))))
	}
	if _, err := DecodeEnvelope([]byte(install(
		`{"role": "receipt", "path": "/data/gdtvm/.gdtvm-install.toml"}`))); err != nil {
		t.Error("POSIX absolute pathが落ちた")
	}
	rejects := []struct{ name, path string }{
		{"roleが違う", `{"role": "state", "path": "/data/x"}`},
		{"roleがenum外", `{"role": "unknown", "path": "/data/x"}`},
		{"pathが相対", `{"role": "receipt", "path": "tools/node/.gdtvm-install.toml"}`},
		{"pathが空", `{"role": "receipt", "path": ""}`},
		{"drive相対path", `{"role": "receipt", "path": "C:x"}`},
		{"pathが無い", `{"role": "receipt"}`},
	}
	for _, test := range rejects {
		t.Run(test.name, func(t *testing.T) {
			if _, err := DecodeEnvelope([]byte(install(test.path))); err == nil {
				t.Error("DecodeEnvelope = nil, want error")
			}
		})
	}
}

// TestBuildResultDevelopmentMatchesVersion はversionとdevelopmentの同値を固定する。
func TestBuildResultDevelopmentMatchesVersion(t *testing.T) {
	devel := strings.NewReplacer(
		`"version": "2026.08.07.00"`, `"version": "devel"`,
		`"development": false`, `"development": true`,
	).Replace(buildFixture())
	source := envelopeWith("version", `{"build": `+devel+`}`)
	if _, err := DecodeEnvelope([]byte(source)); err != nil {
		t.Fatalf("devel buildが落ちた: %s", describe(envelopeErrorOf(source)))
	}
	mismatch := envelopeWith("version",
		`{"build": `+strings.Replace(buildFixture(), `"version": "2026.08.07.00"`, `"version": "devel"`, 1)+`}`)
	if _, err := DecodeEnvelope([]byte(mismatch)); err == nil {
		t.Error("version=develなのにdevelopment=falseが通った")
	}
}

// TestDecodeEnvelopeRejects は§17のstrict decodeを固定する。
func TestDecodeEnvelopeRejects(t *testing.T) {
	tests := []struct{ name, json string }{
		{"unknown top-level key",
			strings.Replace(specSuccessEnvelope, `"schema": 1,`, `"schema": 1, "extra": 1,`, 1)},
		{"重複key",
			strings.Replace(specSuccessEnvelope, `"schema": 1,`, `"schema": 1, "schema": 2,`, 1)},
		{"schemaが2", strings.Replace(specSuccessEnvelope, `"schema": 1`, `"schema": 2`, 1)},
		{"commandが書込み系",
			strings.Replace(specSuccessEnvelope, `"command": "installed"`, `"command": "install"`, 1)},
		{"commandがenum外",
			strings.Replace(specSuccessEnvelope, `"command": "installed"`, `"command": "list"`, 1)},
		{"invocation_idが不正",
			strings.Replace(specSuccessEnvelope, "33333333333333333333333333333333", "3333", 1)},
		{"warningsが無い",
			strings.Replace(specSuccessEnvelope, `"warnings": []`, `"unused": []`, 1)},
		{"error codeがenum外",
			strings.Replace(specFailureEnvelope, `"code": "E_STATE_CORRUPT"`, `"code": "E_UNKNOWN"`, 1)},
		{"error message_idがsegment 1件",
			strings.Replace(specFailureEnvelope, `"message_id": "error.state_corrupt"`, `"message_id": "corrupt"`, 1)},
		{"parametersにnested object",
			strings.Replace(specFailureEnvelope, `"parameters": {}`, `"parameters": {"a": {"b": 1}}`, 1)},
		{"parameters keyがgrammar外",
			strings.Replace(specFailureEnvelope, `"parameters": {}`, `"parameters": {"toolId": "node"}`, 1)},
		{"trailing data", specSuccessEnvelope + specSuccessEnvelope},
		{"BOM付き", "\ufeff" + specSuccessEnvelope},
		{"空", ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := DecodeEnvelope([]byte(test.json)); err == nil {
				t.Error("DecodeEnvelope = nil, want error")
			}
		})
	}
}

// TestEnvelopeErrorIsInternal はenvelopeの組立て失敗がinternal errorであることを固定する。
func TestEnvelopeErrorIsInternal(t *testing.T) {
	_, err := DecodeEnvelope([]byte(`{"schema":2}`))
	if err == nil {
		t.Fatal("schema 2が通った")
	}
	if err.Code != "E_INTERNAL" {
		t.Errorf("code = %q, want E_INTERNAL", err.Code)
	}
	if len(err.Parameters) != 0 {
		t.Errorf("parametersが空でない: %v", err.Parameters)
	}
}

// TestEnvelopeInstallsSortedUnique は§17のlist決定性を固定する。
func TestEnvelopeInstallsSortedUnique(t *testing.T) {
	install := func(version string) string {
		return `{"tool_id": "node", "version": "` + version + `", "platform_id": "windows-amd64",
			"install_id": "` + testInstall + `", "installed_at": "2026-08-07T09:00:00Z",
			"health": "healthy", "receipt_path": {"role": "receipt", "path": "/data/` + version + `"},
			"disk_size": 1, "provider_kind": "official", "selected": false}`
	}
	sorted := envelopeWith("installed", `{"installs": [`+install("20.0.0")+`,`+install("22.18.0")+`]}`)
	if _, err := DecodeEnvelope([]byte(sorted)); err != nil {
		t.Fatalf("byte昇順が落ちた: %s", describe(envelopeErrorOf(sorted)))
	}
	reversed := envelopeWith("installed", `{"installs": [`+install("22.18.0")+`,`+install("20.0.0")+`]}`)
	if _, err := DecodeEnvelope([]byte(reversed)); err == nil {
		t.Error("byte降順が通った")
	}
	duplicate := envelopeWith("installed", `{"installs": [`+install("22.18.0")+`,`+install("22.18.0")+`]}`)
	if _, err := DecodeEnvelope([]byte(duplicate)); err == nil {
		t.Error("同一tupleの重複が通った")
	}
}
