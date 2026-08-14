package store

import (
	"strings"
	"testing"
)

// specReceiptTOML はdocs/04-storage-and-data.md §14の例そのものである。
//
// 仕様の例をそのまま読めることをtestで固定する。読めなければ、仕様か実装の
// どちらかがずれている。
const specReceiptTOML = `schema = 1
install_id = "11111111111111111111111111111111"
root_id = "0123456789abcdef0123456789abcdef"
tool_id = "node"
version = "22.18.0"
platform_id = "windows-amd64"
installed_at = "2026-08-07T09:00:00Z"
client_version = "2026.08.07.00"
client_commit = "0123456789abcdef0123456789abcdef01234567"
definition_path = "tools/node.toml"
definition_sha256 = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
payload_path = "payload"

[artifact]
provider_kind = "official"
provider_name = "Node.js project"
provider_release = "v22.18.0"
url = "https://nodejs.org/dist/v22.18.0/node-v22.18.0-win-x64.zip"
file = "node-v22.18.0-win-x64.zip"
size = 1
digest = "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
checksum_source = "text-file"
third_party_approved = false
license_notice_approved = false

[[storage]]
id = "global-packages"
kind = "global-packages"
scope = "version"
path = "version-data/global-packages"
purge = "with-version"

[[commands]]
name = "node"
target = "{{payload}}/node.exe"
fixed_args = []
environment_profile = "default"
working_directory = "inherit"
passthrough_signals = true

[[environment_profiles]]
id = "default"
path_prepend = ["{{storage.global-packages}}"]
path_append = []
unset = []
override_allowed = []
shell_export = ["NPM_CONFIG_PREFIX"]

[environment_profiles.set]
NPM_CONFIG_PREFIX = "{{storage.global-packages}}"

[[probes]]
id = "version"
runtime_command = "node"
args = ["--version"]
stream = "stdout"
expect = "version"
regex = "^v(?P<version>[0-9]+[.][0-9]+[.][0-9]+)$"
expected_version = "22.18.0"
expected_root = ""
required_paths = []
timeout_ms = 30000
required = true
status = "passed"
reported_version = "22.18.0"
finished_at = "2026-08-07T09:00:00Z"

[[command_targets]]
path = "payload/node.exe"
size = 1
sha256 = "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
`

func TestParseReceiptAcceptsSpecExample(t *testing.T) {
	value, err := ParseReceipt([]byte(specReceiptTOML))
	if err != nil {
		t.Fatalf("ParseReceipt = %v", err)
	}
	if value.InstallID != testInstall || value.PayloadPath != PayloadDirectoryName {
		t.Errorf("install_id/payload_path = %q/%q", value.InstallID, value.PayloadPath)
	}
	if value.Ref.Tool.String() != "node" || value.Ref.Version != "22.18.0" {
		t.Errorf("tool/version = %q/%q", value.Ref.Tool, value.Ref.Version)
	}
	if value.Artifact.ProviderKind != ProviderOfficial {
		t.Errorf("provider_kind = %q", value.Artifact.ProviderKind)
	}
	if value.Artifact.Digest.Upstream() !=
		"sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc" {
		t.Errorf("digest = %q", value.Artifact.Digest.Upstream())
	}
	if len(value.Storage) != 1 || value.Storage[0].Scope != ScopeVersion {
		t.Errorf("storage = %+v", value.Storage)
	}
	if len(value.Commands) != 1 || value.Commands[0].Target != "{{payload}}/node.exe" {
		t.Errorf("commands = %+v", value.Commands)
	}
	if len(value.EnvironmentProfiles) != 1 {
		t.Fatalf("profile件数 = %d", len(value.EnvironmentProfiles))
	}
	if got := value.EnvironmentProfiles[0].Set["NPM_CONFIG_PREFIX"]; got != "{{storage.global-packages}}" {
		t.Errorf("set[NPM_CONFIG_PREFIX] = %q", got)
	}
	if len(value.Probes) != 1 || value.Probes[0].TimeoutMillis != 30000 {
		t.Errorf("probes = %+v", value.Probes)
	}
	if len(value.CommandTargets) != 1 || value.CommandTargets[0].Path != "payload/node.exe" {
		t.Errorf("command_targets = %+v", value.CommandTargets)
	}
}

func TestReceiptRoundTrip(t *testing.T) {
	value, parseErr := ParseReceipt([]byte(specReceiptTOML))
	if parseErr != nil {
		t.Fatalf("ParseReceipt = %v", parseErr)
	}
	data, encodeErr := EncodeReceipt(value)
	if encodeErr != nil {
		t.Fatalf("EncodeReceipt = %v", encodeErr)
	}
	again, reparseErr := ParseReceipt(data)
	if reparseErr != nil {
		t.Fatalf("再parse = %v\n%s", reparseErr, data)
	}
	if again.InstallID != value.InstallID || again.Ref != value.Ref {
		t.Error("round tripで識別子が変わった")
	}
	if len(again.Storage) != len(value.Storage) ||
		len(again.Commands) != len(value.Commands) ||
		len(again.Probes) != len(value.Probes) ||
		len(again.CommandTargets) != len(value.CommandTargets) {
		t.Errorf("round tripで件数が変わった\n%+v", again)
	}
	if again.Probes[0].ReportedVersion != value.Probes[0].ReportedVersion {
		t.Error("probeのreported versionが変わった")
	}
	assertTrailingLF(t, data)
	// 同じ値から同じbyte列が出る。receipt_sha256が内容だけで決まるために必要。
	second, secondErr := EncodeReceipt(value)
	if secondErr != nil {
		t.Fatalf("EncodeReceipt = %v", secondErr)
	}
	if string(second) != string(data) {
		t.Error("出力が決定的でない")
	}
}

// TestReceiptStorageScopePurge はdocs/06-tool-definition.md §8の組合せを固定する。
func TestReceiptStorageScopePurge(t *testing.T) {
	toolScope := strings.NewReplacer(
		`scope = "version"`, `scope = "tool"`,
		`path = "version-data/global-packages"`, `path = "shared/global-packages"`,
		`purge = "with-version"`, `purge = "retain"`,
	).Replace(specReceiptTOML)
	if _, err := ParseReceipt([]byte(toolScope)); err != nil {
		t.Fatalf("tool scope/retainが落ちた: %v", err)
	}

	rejects := []struct {
		name string
		toml string
	}{
		{"tool scopeにwith-version",
			strings.Replace(toolScope, `purge = "retain"`, `purge = "with-version"`, 1)},
		{"version scopeにretain",
			strings.Replace(specReceiptTOML, `purge = "with-version"`, `purge = "retain"`, 1)},
		{"version scopeにexplicit",
			strings.Replace(specReceiptTOML, `purge = "with-version"`, `purge = "explicit"`, 1)},
	}
	for _, test := range rejects {
		t.Run(test.name, func(t *testing.T) {
			if _, err := ParseReceipt([]byte(test.toml)); err == nil {
				t.Error("ParseReceipt = nil, want error")
			}
		})
	}
}

// TestReceiptTemplateAllowlist は§14のtemplate制限を固定する。
//
// 同§は「target/fixed args/path/setで許すtemplateは`{{payload}}`とreceipt内に
// 存在する`{{storage.<id>}}`およびその子pathだけで、metadata/version/staging/
// outputや再帰展開は禁止する」と定める。
func TestReceiptTemplateAllowlist(t *testing.T) {
	accepts := []struct {
		name   string
		target string
	}{
		{"payload直下", `{{payload}}/node.exe`},
		{"payload root", `{{payload}}`},
		{"storage子path", `{{storage.global-packages}}/bin/node.exe`},
		{"storage root", `{{storage.global-packages}}`},
		{"template無しliteral", `node.exe`},
	}
	for _, test := range accepts {
		t.Run("受理/"+test.name, func(t *testing.T) {
			source := strings.Replace(specReceiptTOML,
				`target = "{{payload}}/node.exe"`, `target = "`+test.target+`"`, 1)
			if _, err := ParseReceipt([]byte(source)); err != nil {
				t.Errorf("target %q が落ちた: %v", test.target, err)
			}
		})
	}

	rejects := []struct {
		name   string
		target string
	}{
		{"version変数", `{{version}}/node.exe`},
		{"platform変数", `{{platform.id}}/node.exe`},
		{"probe_temp（commandでは不可）", `{{probe_temp}}/node.exe`},
		{"未定義storage", `{{storage.unknown}}/node.exe`},
		{"未知変数", `{{staging}}/node.exe`},
		{"literal prefix連結", `bin{{payload}}/node.exe`},
		{"root直後にliteral連結", `{{payload}}bin/node.exe`},
		{"子pathの中で再帰展開", `{{payload}}/{{payload}}/node.exe`},
		{"子pathに相対参照", `{{payload}}/../escape`},
		{"子pathが絶対", `{{payload}}//etc`},
		{"空", ``},
	}
	for _, test := range rejects {
		t.Run("拒否/"+test.name, func(t *testing.T) {
			source := strings.Replace(specReceiptTOML,
				`target = "{{payload}}/node.exe"`, `target = "`+test.target+`"`, 1)
			if _, err := ParseReceipt([]byte(source)); err == nil {
				t.Errorf("target %q が通った", test.target)
			}
		})
	}
}

// TestReceiptProbeAllowsProbeTemp はprobeだけが`{{probe_temp}}`を使えることを固定する。
//
// docs/06-tool-definition.md §12が「`{{probe_temp}}`はvalidation probe内だけ」と
// 定める。commandやstorage pathで使うと、削除される一時directoryを恒久的な
// 参照先にしてしまう。
func TestReceiptProbeAllowsProbeTemp(t *testing.T) {
	source := strings.Replace(specReceiptTOML,
		`args = ["--version"]`, `args = ["--version", "{{probe_temp}}/out.txt"]`, 1)
	if _, err := ParseReceipt([]byte(source)); err != nil {
		t.Fatalf("probe argsの{{probe_temp}}が落ちた: %v", err)
	}
	// 同じtemplateがcommandのfixed_argsでは拒否される。
	command := strings.Replace(specReceiptTOML,
		`fixed_args = []`, `fixed_args = ["{{probe_temp}}/out.txt"]`, 1)
	if _, err := ParseReceipt([]byte(command)); err == nil {
		t.Error("command fixed_argsの{{probe_temp}}が通った")
	}
}

// TestReceiptRequiredPathPrefix はdocs/06-tool-definition.md §11のprefixを固定する。
func TestReceiptRequiredPathPrefix(t *testing.T) {
	accepts := []string{
		`["file:{{payload}}/node.exe"]`,
		`["directory:{{payload}}/lib"]`,
		`["file:{{probe_temp}}/out.txt"]`,
		`["directory:{{storage.global-packages}}"]`,
	}
	for _, entry := range accepts {
		source := strings.Replace(specReceiptTOML, `required_paths = []`,
			`required_paths = `+entry, 1)
		if _, err := ParseReceipt([]byte(source)); err != nil {
			t.Errorf("required_paths %s が落ちた: %v", entry, err)
		}
	}
	rejects := []string{
		`["{{payload}}/node.exe"]`,
		`["dir:{{payload}}/lib"]`,
		`["FILE:{{payload}}/node.exe"]`,
		`["file:{{version}}/node.exe"]`,
		`["file:{{storage.unknown}}/x"]`,
	}
	for _, entry := range rejects {
		source := strings.Replace(specReceiptTOML, `required_paths = []`,
			`required_paths = `+entry, 1)
		if _, err := ParseReceipt([]byte(source)); err == nil {
			t.Errorf("required_paths %s が通った", entry)
		}
	}
}

// TestReceiptProbeExpectContract はdocs/06-tool-definition.md §11のexpect別契約を固定する。
func TestReceiptProbeExpectContract(t *testing.T) {
	success := strings.NewReplacer(
		`expect = "version"`, `expect = "success"`,
		`expected_version = "22.18.0"`, `expected_version = ""`,
		`reported_version = "22.18.0"`, `reported_version = ""`,
	).Replace(specReceiptTOML)
	if _, err := ParseReceipt([]byte(success)); err != nil {
		t.Fatalf("expect=successが落ちた: %v", err)
	}

	pathWithin := strings.NewReplacer(
		`expect = "version"`, `expect = "path-within"`,
		`expected_version = "22.18.0"`, `expected_version = ""`,
		`expected_root = ""`, `expected_root = "{{payload}}"`,
		`reported_version = "22.18.0"`, `reported_version = ""`,
	).Replace(specReceiptTOML)
	if _, err := ParseReceipt([]byte(pathWithin)); err != nil {
		t.Fatalf("expect=path-withinが落ちた: %v", err)
	}

	rejects := []struct {
		name string
		toml string
	}{
		{"versionでregexが空",
			strings.Replace(specReceiptTOML, `regex = "^v(?P<version>[0-9]+[.][0-9]+[.][0-9]+)$"`, `regex = ""`, 1)},
		{"versionでexpected_versionが空",
			strings.Replace(specReceiptTOML, `expected_version = "22.18.0"`, `expected_version = ""`, 1)},
		{"versionでexpected_rootがある",
			strings.Replace(specReceiptTOML, `expected_root = ""`, `expected_root = "{{payload}}"`, 1)},
		{"successでexpected_versionがある",
			strings.Replace(success, `expected_version = ""`, `expected_version = "22.18.0"`, 1)},
		{"successでreported_versionがある",
			strings.Replace(success, `reported_version = ""`, `reported_version = "22.18.0"`, 1)},
		{"path-withinでexpected_rootが空",
			strings.Replace(pathWithin, `expected_root = "{{payload}}"`, `expected_root = ""`, 1)},
		{"reported_versionがexpected_versionと不一致",
			strings.Replace(specReceiptTOML, `reported_version = "22.18.0"`, `reported_version = "22.0.0"`, 1)},
	}
	for _, test := range rejects {
		t.Run(test.name, func(t *testing.T) {
			if _, err := ParseReceipt([]byte(test.toml)); err == nil {
				t.Error("ParseReceipt = nil, want error")
			}
		})
	}
}

// TestReceiptRequiredProbeMustPass は§14の「required=trueはpassed必須」を固定する。
func TestReceiptRequiredProbeMustPass(t *testing.T) {
	skipped := strings.Replace(specReceiptTOML, `status = "passed"`, `status = "skipped"`, 1)
	if _, err := ParseReceipt([]byte(skipped)); err == nil {
		t.Error("required=trueのskipped probeが通った")
	}
	optional := strings.NewReplacer(
		`required = true`, `required = false`,
		`status = "passed"`, `status = "skipped"`,
		`reported_version = "22.18.0"`, `reported_version = ""`,
	).Replace(specReceiptTOML)
	if _, err := ParseReceipt([]byte(optional)); err != nil {
		t.Errorf("required=falseのskipped probeが落ちた: %v", err)
	}
}

// TestReceiptThirdPartyApproval は§14のprovider承認整合を固定する。
func TestReceiptThirdPartyApproval(t *testing.T) {
	thirdParty := strings.NewReplacer(
		`provider_kind = "official"`, `provider_kind = "third-party"`,
		`third_party_approved = false`, `third_party_approved = true`,
	).Replace(specReceiptTOML)
	if _, err := ParseReceipt([]byte(thirdParty)); err != nil {
		t.Fatalf("承認済みthird-partyが落ちた: %v", err)
	}
	unapproved := strings.Replace(specReceiptTOML,
		`provider_kind = "official"`, `provider_kind = "third-party"`, 1)
	if _, err := ParseReceipt([]byte(unapproved)); err == nil {
		t.Error("承認なしthird-partyが通った")
	}
	officialApproved := strings.Replace(specReceiptTOML,
		`third_party_approved = false`, `third_party_approved = true`, 1)
	if _, err := ParseReceipt([]byte(officialApproved)); err == nil {
		t.Error("officialにthird_party_approved=trueが通った")
	}
}

// TestReceiptEnvironmentNameUniqueness は§14の「env map keyはplatform規則で一意」を
// 固定する。
func TestReceiptEnvironmentNameUniqueness(t *testing.T) {
	// Windowsは環境変数名をcase非依存に扱う。
	windows := strings.Replace(specReceiptTOML,
		`shell_export = ["NPM_CONFIG_PREFIX"]`,
		`shell_export = ["NPM_CONFIG_PREFIX", "npm_config_prefix"]`, 1)
	if _, err := ParseReceipt([]byte(windows)); err == nil {
		t.Error("Windowsでcase違いの重複が通った")
	}
	// Linuxはcase sensitiveのため別変数である。
	linux := strings.NewReplacer(
		`platform_id = "windows-amd64"`, `platform_id = "linux-amd64-glibc"`,
		`target = "{{payload}}/node.exe"`, `target = "{{payload}}/bin/node"`,
		`path = "payload/node.exe"`, `path = "payload/bin/node"`,
		`shell_export = ["NPM_CONFIG_PREFIX"]`,
		`shell_export = ["NPM_CONFIG_PREFIX", "npm_config_prefix"]`,
	).Replace(specReceiptTOML)
	if _, err := ParseReceipt([]byte(linux)); err != nil {
		t.Errorf("Linuxでcase違いが落ちた: %v", err)
	}
}

// TestParseReceiptRejects は§14のexact keyと必須制約を固定する。
func TestParseReceiptRejects(t *testing.T) {
	tests := []struct {
		name string
		toml string
	}{
		{"unknown top-level key", specReceiptTOML + "extra = 1\n"},
		{"schemaが2", strings.Replace(specReceiptTOML, "schema = 1\n", "schema = 2\n", 1)},
		{"payload_pathが他値",
			strings.Replace(specReceiptTOML, `payload_path = "payload"`, `payload_path = "files"`, 1)},
		{"client_commitが短い",
			strings.Replace(specReceiptTOML, "0123456789abcdef0123456789abcdef01234567", "0123", 1)},
		{"client_commitが大文字",
			strings.Replace(specReceiptTOML, "0123456789abcdef0123456789abcdef01234567",
				"0123456789ABCDEF0123456789abcdef01234567", 1)},
		{"definition_pathが絶対",
			strings.Replace(specReceiptTOML, `definition_path = "tools/node.toml"`,
				`definition_path = "/tools/node.toml"`, 1)},
		{"artifact欠落", strings.Split(specReceiptTOML, "[artifact]")[0]},
		{"artifact.urlがHTTP",
			strings.Replace(specReceiptTOML, "https://nodejs.org", "http://nodejs.org", 1)},
		{"artifact.urlにuserinfo",
			strings.Replace(specReceiptTOML, "https://nodejs.org", "https://user:token@nodejs.org", 1)},
		{"artifact.digestが内部形式",
			strings.Replace(specReceiptTOML,
				"sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
				"cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc", 1)},
		{"artifact.digestのhex長が不一致",
			strings.Replace(specReceiptTOML,
				"sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
				"sha512:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc", 1)},
		{"artifact.fileに区切り",
			strings.Replace(specReceiptTOML, `file = "node-v22.18.0-win-x64.zip"`,
				`file = "dist/node-v22.18.0-win-x64.zip"`, 1)},
		{"artifact.sizeが負",
			strings.Replace(specReceiptTOML, "size = 1\ndigest", "size = -1\ndigest", 1)},
		{"checksum_source enum外",
			strings.Replace(specReceiptTOML, `checksum_source = "text-file"`, `checksum_source = "asset"`, 1)},
		{"commands空", strings.Replace(specReceiptTOML,
			"[[commands]]\nname = \"node\"\ntarget = \"{{payload}}/node.exe\"\nfixed_args = []\n"+
				"environment_profile = \"default\"\nworking_directory = \"inherit\"\n"+
				"passthrough_signals = true\n", "", 1)},
		{"未定義profile参照",
			strings.Replace(specReceiptTOML, `environment_profile = "default"`,
				`environment_profile = "missing"`, 1)},
		{"working_directory enum外",
			strings.Replace(specReceiptTOML, `working_directory = "inherit"`, `working_directory = "cwd"`, 1)},
		{"storage id重複", specReceiptTOML + "\n[[storage]]\nid = \"global-packages\"\n" +
			"kind = \"config\"\nscope = \"version\"\npath = \"version-data/config\"\npurge = \"with-version\"\n"},
		{"storage kind enum外",
			strings.Replace(specReceiptTOML, `kind = "global-packages"`, `kind = "misc"`, 1)},
		{"probe timeout下限未満",
			strings.Replace(specReceiptTOML, "timeout_ms = 30000", "timeout_ms = 0", 1)},
		{"probe timeout上限超過",
			strings.Replace(specReceiptTOML, "timeout_ms = 30000", "timeout_ms = 120001", 1)},
		{"probe stream enum外",
			strings.Replace(specReceiptTOML, `stream = "stdout"`, `stream = "both"`, 1)},
		{"command_targetsがpayload外",
			strings.Replace(specReceiptTOML, `path = "payload/node.exe"`, `path = "tools/node.exe"`, 1)},
		{"command_targets sha256が上流形式",
			strings.Replace(specReceiptTOML,
				"dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
				"sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd", 1)},
		{"environment変数名がgrammar外",
			strings.Replace(specReceiptTOML, `shell_export = ["NPM_CONFIG_PREFIX"]`,
				`shell_export = ["NPM-CONFIG"]`, 1)},
		{"BOM付き", "\ufeff" + specReceiptTOML},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := ParseReceipt([]byte(test.toml)); err == nil {
				t.Error("ParseReceipt = nil, want error")
			}
		})
	}
}

// TestReceiptStorageMayBeEmpty は§14の「arrayはstorageだけ空可」を固定する。
func TestReceiptStorageMayBeEmpty(t *testing.T) {
	withoutStorage := strings.NewReplacer(
		"[[storage]]\nid = \"global-packages\"\nkind = \"global-packages\"\n"+
			"scope = \"version\"\npath = \"version-data/global-packages\"\n"+
			"purge = \"with-version\"\n\n", "",
		`path_prepend = ["{{storage.global-packages}}"]`, `path_prepend = []`,
		`shell_export = ["NPM_CONFIG_PREFIX"]`, `shell_export = []`,
		// setはkeyとして必須である（§14「全件必須」）。storageが無い場合は
		// 空tableにする。table自体を消すとkey欠落として正しく拒否される。
		"[environment_profiles.set]\nNPM_CONFIG_PREFIX = \"{{storage.global-packages}}\"\n",
		"[environment_profiles.set]\n",
	).Replace(specReceiptTOML)
	if _, err := ParseReceipt([]byte(withoutStorage)); err != nil {
		t.Fatalf("storage無しreceiptが落ちた: %s", describe(err))
	}

	// 他のarrayは空にできない。
	withoutProbes := strings.Split(specReceiptTOML, "[[probes]]")[0] +
		strings.SplitN(specReceiptTOML, "[[command_targets]]", 2)[1]
	if _, err := ParseReceipt([]byte("[[command_targets]]" + withoutProbes)); err == nil {
		t.Error("probes無しreceiptが通った")
	}
}

// TestReceiptErrorCarriesReceiptRole はerror codeとroleを固定する。
func TestReceiptErrorCarriesReceiptRole(t *testing.T) {
	_, err := ParseReceipt([]byte("schema = 2\n"))
	if err == nil {
		t.Fatal("schema 2が通った")
	}
	if err.Code != "E_RECEIPT_INVALID" {
		t.Errorf("code = %q, want E_RECEIPT_INVALID", err.Code)
	}
	if err.PathRole != "receipt" {
		t.Errorf("path role = %q, want receipt", err.PathRole)
	}
	if len(err.Parameters) != 0 {
		t.Errorf("parametersが空でない: %v", err.Parameters)
	}
}

// TestReceiptFileSizeLimit は§21のreceipt 1 MiB上限を固定する。
func TestReceiptFileSizeLimit(t *testing.T) {
	padding := "\n# " + strings.Repeat("x", ReceiptFileMaxBytes)
	if _, err := ParseReceipt([]byte(specReceiptTOML + padding)); err == nil {
		t.Error("1 MiB超過が通った")
	}
}
