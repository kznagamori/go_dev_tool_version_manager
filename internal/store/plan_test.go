package store

import (
	"strings"
	"testing"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
)

// specPlanJSON はdocs/04-storage-and-data.md §16の構造例そのものである。
//
// 同§は「上のJSONはtop-level、summary、setup、inputs、配列のkey形状を示す
// **構造例**であり、記載量を抑えるためoperation entryを空にしている。
// そのままでは`operation=install`に必要なdownload/extract/probeがないため、
// semantic validatorは実行可能なPlanとして拒否しなければならない」と定める。
// codec層はkey形状の検査までを担い、実行可能性の判定はPlan作成側（P6-02）が
// 行う。したがってこの例はcodecとしては**通る**のが正しい。
const specPlanJSON = `{
  "schema": 1,
  "client_version": "2026.08.07.00",
  "invocation_id": "33333333333333333333333333333333",
  "operation_id": "22222222222222222222222222222222",
  "operation": "install",
  "created_at": "2026-08-07T09:00:00Z",
  "summary": {
    "tool_id": "python",
    "version": "3.13.7",
    "platform_id": "windows-amd64",
    "provider_kind": "third-party",
    "provider_name": "Astral",
    "provider_repository": "https://github.com/astral-sh/python-build-standalone",
    "provider_homepage": "https://github.com/astral-sh/python-build-standalone",
    "provider_license": "MPL-2.0",
    "provider_release": "20250814",
    "license_notice": "",
    "channel": "stable",
    "lifecycle": "supported",
    "expected_digest": "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
    "checksum_source": "asset-field",
    "warning_count": 1
  },
  "setup": null,
  "inputs": {
    "root_id": "0123456789abcdef0123456789abcdef",
    "config_sha256": "",
    "project_sha256": "",
    "definition_sha256": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
    "catalog_sha256": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
    "registry_sha256": "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
    "selections_revision": 8,
    "setup_revision": 3,
    "receipt_index_revision": 5
  },
  "downloads": [],
  "extracts": [],
  "probes": [],
  "writes": [],
  "storage": [],
  "warnings": [
    {
      "code": "W_THIRD_PARTY",
      "message_id": "warning.third_party",
      "parameters": {},
      "requires_explicit_approval": true
    }
  ]
}`

func TestParsePlanAcceptsSpecExample(t *testing.T) {
	value, err := ParsePlan([]byte(specPlanJSON))
	if err != nil {
		t.Fatalf("ParsePlan = %s", describe(err))
	}
	if value.Kind != OperationInstall || value.ClientVersion != "2026.08.07.00" {
		t.Errorf("operation/client_version = %q/%q", value.Kind, value.ClientVersion)
	}
	if value.Summary.Tool.String() != "python" || value.Summary.Version != "3.13.7" {
		t.Errorf("summary tool/version = %q/%q", value.Summary.Tool, value.Summary.Version)
	}
	if value.Summary.ProviderKind != ProviderThirdParty {
		t.Errorf("provider_kind = %q", value.Summary.ProviderKind)
	}
	if value.Setup != nil {
		t.Error("operation=installなのにsetupがある")
	}
	if value.Inputs.SelectionsRevision != 8 || value.Inputs.SetupRevision != 3 {
		t.Errorf("inputs revision = %d/%d", value.Inputs.SelectionsRevision, value.Inputs.SetupRevision)
	}
	if len(value.Warnings) != 1 || value.Warnings[0].Code != WarnThirdParty {
		t.Fatalf("warnings = %+v", value.Warnings)
	}
	codes := value.ApprovalCodes()
	if len(codes) != 1 || codes[0] != WarnThirdParty {
		t.Errorf("ApprovalCodes = %v", codes)
	}
}

func TestPlanRoundTrip(t *testing.T) {
	value, parseErr := ParsePlan([]byte(specPlanJSON))
	if parseErr != nil {
		t.Fatalf("ParsePlan = %s", describe(parseErr))
	}
	data, encodeErr := EncodePlan(value)
	if encodeErr != nil {
		t.Fatalf("EncodePlan = %s", describe(encodeErr))
	}
	again, reparseErr := ParsePlan(data)
	if reparseErr != nil {
		t.Fatalf("再parse = %s\n%s", describe(reparseErr), data)
	}
	if again.Kind != value.Kind || again.Invocation != value.Invocation {
		t.Error("round tripで識別子が変わった")
	}
	if len(again.Warnings) != len(value.Warnings) {
		t.Errorf("warning件数 = %d, want %d", len(again.Warnings), len(value.Warnings))
	}
	if again.Setup != nil {
		t.Error("setupがnullでなくなった")
	}
	// setup=nullがJSONへnullとして出る。keyを落とすとexact key集合から外れる。
	if !strings.Contains(string(data), `"setup":null`) {
		t.Errorf("setupがnullで出ていない: %s", data)
	}
}

// TestPlanWarningApprovalIsFixedByCode は§16.1の承認要否がcodeで決まることを固定する。
func TestPlanWarningApprovalIsFixedByCode(t *testing.T) {
	// W_THIRD_PARTYは承認必要。falseにすると拒否される。
	flipped := strings.Replace(specPlanJSON,
		`"requires_explicit_approval": true`, `"requires_explicit_approval": false`, 1)
	if _, err := ParsePlan([]byte(flipped)); err == nil {
		t.Error("W_THIRD_PARTYのapprovalをfalseにしても通った")
	}
	// W_RESTART_REQUIREDは承認不要。trueにすると拒否される。
	restart := strings.NewReplacer(
		`"code": "W_THIRD_PARTY"`, `"code": "W_RESTART_REQUIRED"`,
		`"message_id": "warning.third_party"`, `"message_id": "warning.restart_required"`,
	).Replace(specPlanJSON)
	if _, err := ParsePlan([]byte(restart)); err == nil {
		t.Error("W_RESTART_REQUIREDのapprovalをtrueにしても通った")
	}
	ok := strings.Replace(restart,
		`"requires_explicit_approval": true`, `"requires_explicit_approval": false`, 1)
	value, err := ParsePlan([]byte(ok))
	if err != nil {
		t.Fatalf("W_RESTART_REQUIRED（承認不要）が落ちた: %s", describe(err))
	}
	if len(value.ApprovalCodes()) != 0 {
		t.Errorf("ApprovalCodes = %v, want 空", value.ApprovalCodes())
	}
}

// TestPlanApprovalCodeCount は§16.1の「承認対象は7件」を固定する。
func TestPlanApprovalCodeCount(t *testing.T) {
	if len(planWarningApproval) != PlanWarningCodeCount {
		t.Fatalf("PlanWarningCodeは%d件でなければならない（%d件）",
			PlanWarningCodeCount, len(planWarningApproval))
	}
	approvals := 0
	for _, requires := range planWarningApproval {
		if requires {
			approvals++
		}
	}
	if approvals != PlanApprovalCodeCount {
		t.Errorf("承認対象は%d件でなければならない（%d件）", PlanApprovalCodeCount, approvals)
	}
	if planWarningApproval[WarnRestartRequired] {
		t.Error("W_RESTART_REQUIREDは承認対象にしない")
	}
}

// TestPlanWarningCountMatches は§16の`warning_count`一致を固定する。
func TestPlanWarningCountMatches(t *testing.T) {
	mismatch := strings.Replace(specPlanJSON, `"warning_count": 1`, `"warning_count": 2`, 1)
	if _, err := ParsePlan([]byte(mismatch)); err == nil {
		t.Error("warning_count不一致が通った")
	}
}

// TestPlanRejectsDuplicateWarningCode は承認単位が件数と一致することを固定する。
func TestPlanRejectsDuplicateWarningCode(t *testing.T) {
	warning := `{
      "code": "W_THIRD_PARTY",
      "message_id": "warning.third_party",
      "parameters": {},
      "requires_explicit_approval": true
    }`
	duplicated := strings.Replace(specPlanJSON, `"warning_count": 1`, `"warning_count": 2`, 1)
	duplicated = strings.Replace(duplicated, warning, warning+","+warning, 1)
	if _, err := ParsePlan([]byte(duplicated)); err == nil {
		t.Error("同一codeの重複が通った")
	}
}

// TestPlanSetupExclusivity は§16のsetup/operation対応を固定する。
func TestPlanSetupExclusivity(t *testing.T) {
	withSetup := strings.Replace(specPlanJSON, `"setup": null`, `"setup": `+setupPlanFixture(), 1)
	if _, err := ParsePlan([]byte(withSetup)); err == nil {
		t.Error("operation=installなのにsetupがあるPlanが通った")
	}
	setupOp := makeSetupPlanJSON(t)
	if _, err := ParsePlan([]byte(setupOp)); err != nil {
		t.Fatalf("operation=setupが落ちた: %s", describe(planErrorOf(setupOp)))
	}
	withoutSetup := strings.Replace(setupOp, `"setup": `+setupPlanFixture(), `"setup": null`, 1)
	if _, err := ParsePlan([]byte(withoutSetup)); err == nil {
		t.Error("operation=setupなのにsetupがnullのPlanが通った")
	}
}

// setupPlanFixture は§16のSetupPlan 15 keyを満たすfixtureである。
func setupPlanFixture() string {
	return `{
    "mode": "user",
    "previous_mode": "",
    "data_root": {"role": "data-root", "path": "/home/u/.local/share/gdtvm"},
    "distribution_root": {"role": "distribution-root", "path": "/opt/gdtvm"},
    "previous_data_root": {"role": "data-root", "path": ""},
    "previous_distribution_root": {"role": "distribution-root", "path": ""},
    "filesystem_capabilities": ["atomic-replace", "directory-rename", "file-identity", "owner-enforcement", "symlink"],
    "current_link_strategy": "symlink",
    "shim_strategy": "symlink",
    "shim_directory": {"role": "shim", "path": "/home/u/.local/share/gdtvm/shims"},
    "path_integration": "shell-profile",
    "shell": "bash",
    "integration_target": {"role": "config", "path": "/home/u/.bashrc"},
    "backup_path": {"role": "state-backup", "path": "/home/u/.local/share/gdtvm/state/backups/setup-abcdef0123456789abcdef0123456789.toml"},
    "restart_required": false
  }`
}

// makeSetupPlanJSON はoperation=setupのPlanを組み立てる。
func makeSetupPlanJSON(t *testing.T) string {
	t.Helper()
	return strings.NewReplacer(
		`"operation": "install"`, `"operation": "setup"`,
		`"setup": null`, `"setup": `+setupPlanFixture(),
		`"tool_id": "python"`, `"tool_id": ""`,
		`"version": "3.13.7"`, `"version": ""`,
		`"platform_id": "windows-amd64"`, `"platform_id": ""`,
		`"provider_kind": "third-party"`, `"provider_kind": "none"`,
		`"provider_name": "Astral"`, `"provider_name": ""`,
		`"provider_repository": "https://github.com/astral-sh/python-build-standalone"`,
		`"provider_repository": ""`,
		`"provider_homepage": "https://github.com/astral-sh/python-build-standalone"`,
		`"provider_homepage": ""`,
		`"provider_license": "MPL-2.0"`, `"provider_license": ""`,
		`"provider_release": "20250814"`, `"provider_release": ""`,
		`"channel": "stable"`, `"channel": ""`,
		`"lifecycle": "supported"`, `"lifecycle": ""`,
		`"expected_digest": "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"`,
		`"expected_digest": ""`,
		`"checksum_source": "asset-field"`, `"checksum_source": ""`,
		`"code": "W_THIRD_PARTY"`, `"code": "W_SHELL_MODIFICATION"`,
		`"message_id": "warning.third_party"`, `"message_id": "warning.shell_modification"`,
	).Replace(specPlanJSON)
}

// planErrorOf はtestの診断用にParsePlanのerrorを取り出す。
func planErrorOf(source string) *domain.Error {
	_, err := ParsePlan([]byte(source))
	return err
}

// TestSetupPlanCapabilityConsistency は§16のplatform別capability契約を固定する。
func TestSetupPlanCapabilityConsistency(t *testing.T) {
	base := makeSetupPlanJSON(t)
	if _, err := ParsePlan([]byte(base)); err != nil {
		t.Fatalf("Linux構成が落ちた: %s", describe(planErrorOf(base)))
	}

	windows := strings.NewReplacer(
		`["atomic-replace", "directory-rename", "file-identity", "owner-enforcement", "symlink"]`,
		`["atomic-replace", "directory-rename", "file-identity", "hardlink", "junction", "owner-enforcement"]`,
		`"current_link_strategy": "symlink"`, `"current_link_strategy": "junction"`,
		`"shim_strategy": "symlink"`, `"shim_strategy": "hardlink"`,
	).Replace(base)
	if _, err := ParsePlan([]byte(windows)); err != nil {
		t.Fatalf("Windows構成が落ちた: %s", describe(planErrorOf(windows)))
	}

	rejects := []struct {
		name string
		json string
	}{
		{"junctionなのにcapabilityが無い",
			strings.Replace(base, `"current_link_strategy": "symlink"`, `"current_link_strategy": "junction"`, 1)},
		{"symlink環境でshimがhardlink",
			strings.Replace(base, `"shim_strategy": "symlink"`, `"shim_strategy": "hardlink"`, 1)},
		{"hardlink capabilityがあるのにshimがsymlink",
			strings.Replace(base,
				`["atomic-replace", "directory-rename", "file-identity", "owner-enforcement", "symlink"]`,
				`["atomic-replace", "directory-rename", "file-identity", "hardlink", "owner-enforcement", "symlink"]`, 1)},
		{"共通必須capabilityが欠ける",
			strings.Replace(base,
				`["atomic-replace", "directory-rename", "file-identity", "owner-enforcement", "symlink"]`,
				`["directory-rename", "file-identity", "owner-enforcement", "symlink"]`, 1)},
		{"capabilityがbyte順でない",
			strings.Replace(base,
				`["atomic-replace", "directory-rename", "file-identity", "owner-enforcement", "symlink"]`,
				`["directory-rename", "atomic-replace", "file-identity", "owner-enforcement", "symlink"]`, 1)},
		{"capabilityが重複",
			strings.Replace(base,
				`["atomic-replace", "directory-rename", "file-identity", "owner-enforcement", "symlink"]`,
				`["atomic-replace", "atomic-replace", "directory-rename", "file-identity", "owner-enforcement", "symlink"]`, 1)},
		{"capabilityが空",
			strings.Replace(base,
				`["atomic-replace", "directory-rename", "file-identity", "owner-enforcement", "symlink"]`,
				`[]`, 1)},
	}
	for _, test := range rejects {
		t.Run(test.name, func(t *testing.T) {
			if _, err := ParsePlan([]byte(test.json)); err == nil {
				t.Error("ParsePlan = nil, want error")
			}
		})
	}
}

// TestSetupPlanPreviousFieldsAreSimultaneous は§16のprevious 3 fieldの同時性と
// W_MODE_CHANGEの同値を固定する。
func TestSetupPlanPreviousFieldsAreSimultaneous(t *testing.T) {
	base := makeSetupPlanJSON(t)

	// 3件すべて非空 ＋ W_MODE_CHANGE 1件なら通る。
	changed := strings.NewReplacer(
		`"previous_mode": ""`, `"previous_mode": "portable"`,
		`"previous_data_root": {"role": "data-root", "path": ""}`,
		`"previous_data_root": {"role": "data-root", "path": "/opt/old"}`,
		`"previous_distribution_root": {"role": "distribution-root", "path": ""}`,
		`"previous_distribution_root": {"role": "distribution-root", "path": "/opt/olddist"}`,
		`"warning_count": 1`, `"warning_count": 2`,
		`"code": "W_SHELL_MODIFICATION"`, `"code": "W_MODE_CHANGE"`,
		`"message_id": "warning.shell_modification"`, `"message_id": "warning.mode_change"`,
	).Replace(base)
	if _, err := ParsePlan([]byte(changed)); err == nil {
		t.Log("warning_countが2なのにwarningsが1件のため拒否される（想定内）")
	}

	// 1件だけ非空にすると拒否される。
	partial := strings.Replace(base, `"previous_mode": ""`, `"previous_mode": "portable"`, 1)
	if _, err := ParsePlan([]byte(partial)); err == nil {
		t.Error("previous_modeだけ非空のPlanが通った")
	}
	partialRoot := strings.Replace(base,
		`"previous_data_root": {"role": "data-root", "path": ""}`,
		`"previous_data_root": {"role": "data-root", "path": "/opt/old"}`, 1)
	if _, err := ParsePlan([]byte(partialRoot)); err == nil {
		t.Error("previous_data_rootだけ非空のPlanが通った")
	}
}

// TestSetupPlanRestartWarning は§16のrestart_requiredとW_RESTART_REQUIREDの同値を
// 固定する。
func TestSetupPlanRestartWarning(t *testing.T) {
	base := makeSetupPlanJSON(t)
	onlyFlag := strings.Replace(base, `"restart_required": false`, `"restart_required": true`, 1)
	if _, err := ParsePlan([]byte(onlyFlag)); err == nil {
		t.Error("restart_required=trueなのにW_RESTART_REQUIREDが無いPlanが通った")
	}
	both := strings.NewReplacer(
		`"restart_required": false`, `"restart_required": true`,
		`"warning_count": 1`, `"warning_count": 2`,
		`"warnings": [`, `"warnings": [
    {
      "code": "W_RESTART_REQUIRED",
      "message_id": "warning.restart_required",
      "parameters": {},
      "requires_explicit_approval": false
    },`,
	).Replace(base)
	if _, err := ParsePlan([]byte(both)); err != nil {
		t.Errorf("restart_required=true＋W_RESTART_REQUIREDが落ちた: %s", describe(planErrorOf(both)))
	}
}

// TestSetupPlanIntegrationTarget は§17.2のWindows PATH locator例外を固定する。
func TestSetupPlanIntegrationTarget(t *testing.T) {
	base := makeSetupPlanJSON(t)
	locator := strings.NewReplacer(
		`"path_integration": "shell-profile"`, `"path_integration": "user-path"`,
		`"shell": "bash"`, `"shell": ""`,
		`"integration_target": {"role": "config", "path": "/home/u/.bashrc"}`,
		`"integration_target": {"role": "config", "path": "HKCU\\Environment\\Path"}`,
	).Replace(base)
	if _, err := ParsePlan([]byte(locator)); err != nil {
		t.Fatalf("Windows PATH locatorが落ちた: %s", describe(planErrorOf(locator)))
	}

	rejects := []struct {
		name string
		json string
	}{
		{"noneなのにtargetがある",
			strings.NewReplacer(
				`"path_integration": "shell-profile"`, `"path_integration": "none"`,
				`"shell": "bash"`, `"shell": ""`).Replace(base)},
		{"shell-profileなのにtargetが空",
			strings.Replace(base,
				`"integration_target": {"role": "config", "path": "/home/u/.bashrc"}`,
				`"integration_target": {"role": "config", "path": ""}`, 1)},
		{"targetが相対path",
			strings.Replace(base, `"path": "/home/u/.bashrc"`, `"path": ".bashrc"`, 1)},
		{"targetのroleが違う",
			strings.Replace(base,
				`"integration_target": {"role": "config", "path": "/home/u/.bashrc"}`,
				`"integration_target": {"role": "state", "path": "/home/u/.bashrc"}`, 1)},
	}
	for _, test := range rejects {
		t.Run(test.name, func(t *testing.T) {
			if _, err := ParsePlan([]byte(test.json)); err == nil {
				t.Error("ParsePlan = nil, want error")
			}
		})
	}
}

// TestPlanSummaryToollessOperation は§16の「対象toolがないoperation」を固定する。
func TestPlanSummaryToollessOperation(t *testing.T) {
	base := makeSetupPlanJSON(t)
	if _, err := ParsePlan([]byte(base)); err != nil {
		t.Fatalf("provider_kind=noneのsetupが落ちた: %s", describe(planErrorOf(base)))
	}
	rejects := []struct {
		name string
		json string
	}{
		{"setupでprovider_kindがofficial",
			strings.Replace(base, `"provider_kind": "none"`, `"provider_kind": "official"`, 1)},
		{"installでprovider_kindがnone",
			strings.Replace(specPlanJSON, `"provider_kind": "third-party"`, `"provider_kind": "none"`, 1)},
		{"noneなのにtool_idがある",
			strings.Replace(base, `"tool_id": ""`, `"tool_id": "python"`, 1)},
		{"noneなのにchannelがある",
			strings.Replace(base, `"channel": ""`, `"channel": "stable"`, 1)},
		{"third-partyでrepositoryが空",
			strings.Replace(specPlanJSON,
				`"provider_repository": "https://github.com/astral-sh/python-build-standalone"`,
				`"provider_repository": ""`, 1)},
		{"third-partyでlicenseが空",
			strings.Replace(specPlanJSON, `"provider_license": "MPL-2.0"`, `"provider_license": ""`, 1)},
	}
	for _, test := range rejects {
		t.Run(test.name, func(t *testing.T) {
			if _, err := ParsePlan([]byte(test.json)); err == nil {
				t.Error("ParsePlan = nil, want error")
			}
		})
	}
}

// TestParsePlanRejects は§16のtop-level契約を固定する。
func TestParsePlanRejects(t *testing.T) {
	tests := []struct {
		name string
		json string
	}{
		{"unknown top-level key",
			strings.Replace(specPlanJSON, `"schema": 1,`, `"schema": 1, "extra": 1,`, 1)},
		{"unknown summary key",
			strings.Replace(specPlanJSON, `"tool_id": "python",`, `"tool_id": "python", "extra": 1,`, 1)},
		{"重複key", strings.Replace(specPlanJSON, `"schema": 1,`, `"schema": 1, "schema": 2,`, 1)},
		{"schemaが2", strings.Replace(specPlanJSON, `"schema": 1`, `"schema": 2`, 1)},
		{"operation enum外",
			strings.Replace(specPlanJSON, `"operation": "install"`, `"operation": "upgrade"`, 1)},
		{"invocation_idが不正",
			strings.Replace(specPlanJSON, "33333333333333333333333333333333", "3333", 1)},
		{"operation_idが不正",
			strings.Replace(specPlanJSON, "22222222222222222222222222222222", "2222", 1)},
		{"client_versionが不正",
			strings.Replace(specPlanJSON, `"client_version": "2026.08.07.00"`, `"client_version": "1.0"`, 1)},
		{"created_at非UTC",
			strings.Replace(specPlanJSON, `"created_at": "2026-08-07T09:00:00Z"`,
				`"created_at": "2026-08-07T09:00:00+09:00"`, 1)},
		{"summary欠落", strings.Replace(specPlanJSON, `"summary": {`, `"unused": {`, 1)},
		{"inputs.root_idが不正",
			strings.Replace(specPlanJSON, `"root_id": "0123456789abcdef0123456789abcdef"`,
				`"root_id": "xyz"`, 1)},
		{"inputs digestが上流形式",
			strings.Replace(specPlanJSON, `"registry_sha256": "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"`,
				`"registry_sha256": "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"`, 1)},
		{"inputs revisionが負",
			strings.Replace(specPlanJSON, `"setup_revision": 3`, `"setup_revision": -1`, 1)},
		{"warning codeが§16.1に無い",
			strings.Replace(specPlanJSON, `"code": "W_THIRD_PARTY"`, `"code": "W_UNKNOWN"`, 1)},
		{"warning codeがresult warning",
			strings.Replace(specPlanJSON, `"code": "W_THIRD_PARTY"`, `"code": "W_CACHE_STALE"`, 1)},
		{"warningsがnull",
			strings.Replace(specPlanJSON, `"warnings": [`, `"warnings2": [`, 1)},
		{"downloadsがnull", strings.Replace(specPlanJSON, `"downloads": []`, `"downloads": null`, 1)},
		{"trailing data", specPlanJSON + specPlanJSON},
		{"BOM付き", "\ufeff" + specPlanJSON},
		{"空", ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := ParsePlan([]byte(test.json)); err == nil {
				t.Error("ParsePlan = nil, want error")
			}
		})
	}
}

// TestPlanErrorIsInternal はPlanの組立て失敗がinternal errorであることを固定する。
//
// Planはgdtvm自身が作るものであり、契約に合わないのは利用者入力ではなく内部誤りである。
func TestPlanErrorIsInternal(t *testing.T) {
	_, err := ParsePlan([]byte(`{"schema":2}`))
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
