package store

import (
	"strings"
	"testing"
	"time"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
)

const (
	testRootID   = "0123456789abcdef0123456789abcdef"
	testBackupID = "abcdef0123456789abcdef0123456789"
	testInstall  = "11111111111111111111111111111111"
	testDigestA  = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	testDigestB  = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

// specSchemaTOML はdocs/04-storage-and-data.md §8の例そのものである。
//
// 仕様の例をそのまま読めることをtestで固定する。読めなければ、仕様か実装の
// どちらかがずれている。
const specSchemaTOML = `schema = 1
revision = 1
root_id = "0123456789abcdef0123456789abcdef"
mode = "user"
created_at = "2026-08-07T09:00:00Z"
updated_at = "2026-08-07T09:00:00Z"
client_version = "2026.08.07.00"
state_schema = 1
receipt_schema = 1
catalog_schema = 1
`

// specSetupTOML はdocs/04-storage-and-data.md §9の例そのものである。
const specSetupTOML = `schema = 1
revision = 3
root_id = "0123456789abcdef0123456789abcdef"
mode = "user"
path_integration = "user-path"
shell = ""
shim_path = "shims"
backup_id = "abcdef0123456789abcdef0123456789"
updated_at = "2026-08-07T09:00:00Z"

[integration_identity]
kind = "windows-registry-value"
location = "HKCU\\Environment"
name = "Path"
before_sha256 = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
after_sha256 = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
`

// specSelectionsTOML はdocs/04-storage-and-data.md §11の例そのものである。
const specSelectionsTOML = `schema = 1
revision = 8
root_id = "0123456789abcdef0123456789abcdef"
updated_at = "2026-08-07T09:00:00Z"

[[selections]]
tool_id = "node"
version = "22.18.0"
platform_id = "windows-amd64"
install_id = "11111111111111111111111111111111"
selected_at = "2026-08-07T09:00:00Z"
`

// specShimIndexTOML はdocs/04-storage-and-data.md §12の例そのものである。
const specShimIndexTOML = `schema = 1
revision = 4
root_id = "0123456789abcdef0123456789abcdef"
client_version = "2026.08.07.00"
receipt_index_revision = 5
updated_at = "2026-08-07T09:00:00Z"

[[commands]]
name = "node"
tool_id = "node"

[[commands]]
name = "npm"
tool_id = "node"
`

// specReceiptIndexTOML はdocs/04-storage-and-data.md §13の例そのものである。
const specReceiptIndexTOML = `schema = 1
revision = 5
root_id = "0123456789abcdef0123456789abcdef"
updated_at = "2026-08-07T09:00:00Z"

[[receipts]]
tool_id = "node"
version = "22.18.0"
platform_id = "windows-amd64"
install_id = "11111111111111111111111111111111"
path = "tools/node/versions/22.18.0/windows-amd64/.gdtvm-install.toml"
receipt_sha256 = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
health = "healthy"
`

func TestParseStateSchemaAcceptsSpecExample(t *testing.T) {
	value, err := ParseStateSchema([]byte(specSchemaTOML))
	if err != nil {
		t.Fatalf("ParseStateSchema = %v", err)
	}
	if value.Revision != 1 || value.RootID != testRootID {
		t.Errorf("revision/root_id = %d/%q", value.Revision, value.RootID)
	}
	if value.Mode != domain.ModeUser {
		t.Errorf("mode = %q, want %q", value.Mode, domain.ModeUser)
	}
	if value.ClientVersion != "2026.08.07.00" {
		t.Errorf("client_version = %q", value.ClientVersion)
	}
	want := time.Date(2026, 8, 7, 9, 0, 0, 0, time.UTC)
	if !value.CreatedAt.Equal(want) || !value.UpdatedAt.Equal(want) {
		t.Errorf("timestamps = %v / %v, want %v", value.CreatedAt, value.UpdatedAt, want)
	}
	if value.StateSchema != 1 || value.ReceiptSchema != 1 || value.CatalogSchema != 1 {
		t.Error("schema revisionが1でない")
	}
}

func TestStateSchemaRoundTrip(t *testing.T) {
	value, parseErr := ParseStateSchema([]byte(specSchemaTOML))
	if parseErr != nil {
		t.Fatalf("ParseStateSchema = %v", parseErr)
	}
	data, encodeErr := EncodeStateSchema(value)
	if encodeErr != nil {
		t.Fatalf("EncodeStateSchema = %v", encodeErr)
	}
	again, reparseErr := ParseStateSchema(data)
	if reparseErr != nil {
		t.Fatalf("再parse = %v\n%s", reparseErr, data)
	}
	if again != value {
		t.Errorf("round tripで値が変わった\n%+v\n%+v", again, value)
	}
	assertTrailingLF(t, data)
}

// TestParseStateSchemaRejects は§8の「許可keyは上記だけで全件必須」を固定する。
func TestParseStateSchemaRejects(t *testing.T) {
	tests := []struct {
		name string
		toml string
	}{
		{"schema欠落", strings.Replace(specSchemaTOML, "schema = 1\n", "", 1)},
		{"schemaが2", strings.Replace(specSchemaTOML, "schema = 1\n", "schema = 2\n", 1)},
		{"unknown key", specSchemaTOML + "extra = 1\n"},
		{"重複key", specSchemaTOML + "revision = 2\n"},
		{"revision型違い", strings.Replace(specSchemaTOML, "revision = 1", `revision = "1"`, 1)},
		{"revisionが負", strings.Replace(specSchemaTOML, "revision = 1", "revision = -1", 1)},
		{"root_id短い", strings.Replace(specSchemaTOML, testRootID, "0123", 1)},
		{"root_id大文字", strings.Replace(specSchemaTOML, testRootID, strings.ToUpper(testRootID), 1)},
		{"root_id欠落", strings.Replace(specSchemaTOML, `root_id = "`+testRootID+`"`+"\n", "", 1)},
		{"mode enum外", strings.Replace(specSchemaTOML, `mode = "user"`, `mode = "system"`, 1)},
		{"created_at非UTC", strings.Replace(specSchemaTOML,
			`created_at = "2026-08-07T09:00:00Z"`, `created_at = "2026-08-07T09:00:00+09:00"`, 1)},
		{"created_atがdate", strings.Replace(specSchemaTOML,
			`created_at = "2026-08-07T09:00:00Z"`, `created_at = "2026-08-07"`, 1)},
		{"client_version不正", strings.Replace(specSchemaTOML,
			`client_version = "2026.08.07.00"`, `client_version = "1.0"`, 1)},
		{"state_schemaが2", strings.Replace(specSchemaTOML, "state_schema = 1", "state_schema = 2", 1)},
		{"receipt_schema欠落", strings.Replace(specSchemaTOML, "receipt_schema = 1\n", "", 1)},
		{"catalog_schema欠落", strings.Replace(specSchemaTOML, "catalog_schema = 1\n", "", 1)},
		{"BOM付き", "\ufeff" + specSchemaTOML},
		{"空file", ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := ParseStateSchema([]byte(test.toml)); err == nil {
				t.Error("ParseStateSchema = nil, want error")
			}
		})
	}
}

// TestStateSchemaAcceptsDevelClientVersion は§8のdevelopment buildを固定する。
func TestStateSchemaAcceptsDevelClientVersion(t *testing.T) {
	source := strings.Replace(specSchemaTOML,
		`client_version = "2026.08.07.00"`, `client_version = "devel"`, 1)
	value, err := ParseStateSchema([]byte(source))
	if err != nil {
		t.Fatalf("ParseStateSchema = %v", err)
	}
	if value.ClientVersion != DevelopmentClientVersion {
		t.Errorf("client_version = %q, want %q", value.ClientVersion, DevelopmentClientVersion)
	}
}

func TestParseSetupStateAcceptsSpecExample(t *testing.T) {
	value, err := ParseSetupState([]byte(specSetupTOML))
	if err != nil {
		t.Fatalf("ParseSetupState = %v", err)
	}
	if value.PathIntegration != PathIntegrationUserPath || value.Shell != "" {
		t.Errorf("integration/shell = %q/%q", value.PathIntegration, value.Shell)
	}
	if value.ShimPath != ShimDirectoryName || value.BackupID != testBackupID {
		t.Errorf("shim_path/backup_id = %q/%q", value.ShimPath, value.BackupID)
	}
	if value.Identity.Kind != IntegrationWindowsRegistryValue {
		t.Errorf("identity.kind = %q", value.Identity.Kind)
	}
	if value.Identity.Location != `HKCU\Environment` || value.Identity.Name != "Path" {
		t.Errorf("identity location/name = %q/%q", value.Identity.Location, value.Identity.Name)
	}
}

func TestSetupStateRoundTrip(t *testing.T) {
	value, parseErr := ParseSetupState([]byte(specSetupTOML))
	if parseErr != nil {
		t.Fatalf("ParseSetupState = %v", parseErr)
	}
	data, encodeErr := EncodeSetupState(value)
	if encodeErr != nil {
		t.Fatalf("EncodeSetupState = %v", encodeErr)
	}
	again, reparseErr := ParseSetupState(data)
	if reparseErr != nil {
		t.Fatalf("再parse = %v\n%s", reparseErr, data)
	}
	if again != value {
		t.Errorf("round tripで値が変わった\n%+v\n%+v", again, value)
	}
	assertTrailingLF(t, data)
}

// TestSetupStateShellMatchesIntegration は§9のshell整合を固定する。
func TestSetupStateShellMatchesIntegration(t *testing.T) {
	profile := strings.NewReplacer(
		`path_integration = "user-path"`, `path_integration = "shell-profile"`,
		`shell = ""`, `shell = "bash"`,
		`kind = "windows-registry-value"`, `kind = "shell-profile-file"`,
	).Replace(specSetupTOML)
	value, err := ParseSetupState([]byte(profile))
	if err != nil {
		t.Fatalf("shell-profile構成が落ちた: %v", err)
	}
	if value.Shell != ShellBash {
		t.Errorf("shell = %q, want %q", value.Shell, ShellBash)
	}

	rejects := []struct {
		name string
		toml string
	}{
		{"user-pathなのにshellがある",
			strings.Replace(specSetupTOML, `shell = ""`, `shell = "bash"`, 1)},
		{"shell-profileなのにshellが空",
			strings.Replace(specSetupTOML,
				`path_integration = "user-path"`, `path_integration = "shell-profile"`, 1)},
		{"shellがenum外",
			strings.NewReplacer(
				`path_integration = "user-path"`, `path_integration = "shell-profile"`,
				`shell = ""`, `shell = "pwsh"`).Replace(specSetupTOML)},
	}
	for _, test := range rejects {
		t.Run(test.name, func(t *testing.T) {
			if _, err := ParseSetupState([]byte(test.toml)); err == nil {
				t.Error("ParseSetupState = nil, want error")
			}
		})
	}
}

// TestSetupStateIntegrationNone は§9のkind=noneの表現が1通りであることを固定する。
func TestSetupStateIntegrationNone(t *testing.T) {
	zero := strings.Repeat("0", 64)
	none := strings.NewReplacer(
		`path_integration = "user-path"`, `path_integration = "none"`,
		`kind = "windows-registry-value"`, `kind = "none"`,
		`location = "HKCU\\Environment"`, `location = ""`,
		`name = "Path"`, `name = ""`,
		testDigestA, zero, testDigestB, zero,
	).Replace(specSetupTOML)
	if _, err := ParseSetupState([]byte(none)); err != nil {
		t.Fatalf("kind=noneが落ちた: %v", err)
	}

	rejects := []struct {
		name string
		toml string
	}{
		{"noneなのにlocationがある",
			strings.Replace(none, `location = ""`, `location = "HKCU\\Environment"`, 1)},
		{"noneなのにnameがある", strings.Replace(none, `name = ""`, `name = "Path"`, 1)},
		{"noneなのにdigestが非zero",
			strings.Replace(none, `before_sha256 = "`+zero+`"`, `before_sha256 = "`+testDigestA+`"`, 1)},
	}
	for _, test := range rejects {
		t.Run(test.name, func(t *testing.T) {
			if _, err := ParseSetupState([]byte(test.toml)); err == nil {
				t.Error("ParseSetupState = nil, want error")
			}
		})
	}
}

// TestParseSetupStateRejects は§9のexact keyと固定値を固定する。
func TestParseSetupStateRejects(t *testing.T) {
	tests := []struct {
		name string
		toml string
	}{
		{"unknown top-level key", specSetupTOML + "extra = 1\n"},
		{"unknown identity key", specSetupTOML + "unexpected = \"x\"\n"},
		{"integration_identity欠落",
			strings.Split(specSetupTOML, "[integration_identity]")[0]},
		{"path_integration enum外",
			strings.Replace(specSetupTOML, `path_integration = "user-path"`, `path_integration = "auto"`, 1)},
		{"shim_pathが他値",
			strings.Replace(specSetupTOML, `shim_path = "shims"`, `shim_path = "bin"`, 1)},
		{"backup_idが不正",
			strings.Replace(specSetupTOML, testBackupID, "zz", 1)},
		{"identity kind enum外",
			strings.Replace(specSetupTOML, `kind = "windows-registry-value"`, `kind = "registry"`, 1)},
		{"before_sha256が短い",
			strings.Replace(specSetupTOML, testDigestA, "aaaa", 1)},
		{"before_sha256が大文字",
			strings.Replace(specSetupTOML, testDigestA, strings.ToUpper(testDigestA), 1)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := ParseSetupState([]byte(test.toml)); err == nil {
				t.Error("ParseSetupState = nil, want error")
			}
		})
	}
}

func TestParseSelectionsAcceptsSpecExample(t *testing.T) {
	value, err := ParseSelections([]byte(specSelectionsTOML))
	if err != nil {
		t.Fatalf("ParseSelections = %v", err)
	}
	if value.Revision != 8 || len(value.Entries) != 1 {
		t.Fatalf("revision/件数 = %d/%d", value.Revision, len(value.Entries))
	}
	entry := value.Entries[0]
	if entry.Ref.Tool.String() != "node" || entry.Ref.Version != "22.18.0" {
		t.Errorf("tool/version = %q/%q", entry.Ref.Tool, entry.Ref.Version)
	}
	if entry.Ref.Platform.ID() != "windows-amd64" || entry.InstallID != testInstall {
		t.Errorf("platform/install_id = %q/%q", entry.Ref.Platform.ID(), entry.InstallID)
	}
}

func TestSelectionsRoundTrip(t *testing.T) {
	value, parseErr := ParseSelections([]byte(specSelectionsTOML))
	if parseErr != nil {
		t.Fatalf("ParseSelections = %v", parseErr)
	}
	data, encodeErr := EncodeSelections(value)
	if encodeErr != nil {
		t.Fatalf("EncodeSelections = %v", encodeErr)
	}
	again, reparseErr := ParseSelections(data)
	if reparseErr != nil {
		t.Fatalf("再parse = %v\n%s", reparseErr, data)
	}
	if len(again.Entries) != 1 || again.Entries[0] != value.Entries[0] {
		t.Errorf("round tripでentryが変わった\n%+v\n%+v", again.Entries, value.Entries)
	}
	assertTrailingLF(t, data)
}

// TestSelectionsRequireSortedUnique は§11の「toolごとに最大1件、tool ID byte順で
// 一意」を固定する。
func TestSelectionsRequireSortedUnique(t *testing.T) {
	entry := func(tool string) string {
		return "\n[[selections]]\ntool_id = \"" + tool + "\"\nversion = \"1.0.0\"\n" +
			"platform_id = \"linux-amd64-glibc\"\ninstall_id = \"" + testInstall + "\"\n" +
			"selected_at = \"2026-08-07T09:00:00Z\"\n"
	}
	header := strings.Split(specSelectionsTOML, "[[selections]]")[0]

	if _, err := ParseSelections([]byte(header + strings.TrimPrefix(entry("go"), "\n") + entry("node"))); err != nil {
		t.Fatalf("昇順2件が落ちた: %v", err)
	}
	if _, err := ParseSelections([]byte(header + strings.TrimPrefix(entry("node"), "\n") + entry("go"))); err == nil {
		t.Error("降順が通った")
	}
	if _, err := ParseSelections([]byte(header + strings.TrimPrefix(entry("go"), "\n") + entry("go"))); err == nil {
		t.Error("同一tool 2件が通った")
	}
}

// TestSelectionsRejectsInexactVersion は完全versionだけを保存する制約を固定する。
//
// schemeごとのgrammarはdefinitionが決めるため、ここではscheme非依存に判定できる
// 拒否だけを見る（[InstallRef]の分担）。P2-02の`ParseProjectConfig`と同じ範囲である。
func TestSelectionsRejectsInexactVersion(t *testing.T) {
	for _, version := range []string{"", "latest", "^22.0.0", ">=22", "~22.1", "22.*", "22.18.0 ", "22 , 23"} {
		source := strings.Replace(specSelectionsTOML, `version = "22.18.0"`,
			`version = "`+version+`"`, 1)
		if _, err := ParseSelections([]byte(source)); err == nil {
			t.Errorf("version %q が通った", version)
		}
	}
}

// TestSelectionsAcceptsSchemeDependentVersions はcodec層が判定しない範囲を固定する。
//
// `22.x`のようなrange記号を含まない部分versionは、schemeを知らなければ完全version
// と区別できない。scheme検証はtool definitionが入るP3の責務であり、ここで
// 「`x`を含めば拒否」のような規則を足すと、schemeによっては正当な値を拒否する。
// この境界が動いたら気付けるよう、通ることを明示的に固定する。
func TestSelectionsAcceptsSchemeDependentVersions(t *testing.T) {
	for _, version := range []string{"22.x", "22", "1.22", "3.13.7rc1"} {
		source := strings.Replace(specSelectionsTOML, `version = "22.18.0"`,
			`version = "`+version+`"`, 1)
		if _, err := ParseSelections([]byte(source)); err != nil {
			t.Errorf("version %q がcodec層で落ちた: %v", version, err)
		}
	}
}

func TestParseShimIndexAcceptsSpecExample(t *testing.T) {
	value, err := ParseShimIndex([]byte(specShimIndexTOML))
	if err != nil {
		t.Fatalf("ParseShimIndex = %v", err)
	}
	if value.ReceiptIndexRevision != 5 || len(value.Commands) != 2 {
		t.Fatalf("receipt revision/件数 = %d/%d", value.ReceiptIndexRevision, len(value.Commands))
	}
	if value.Commands[0].Name != "node" || value.Commands[1].Name != "npm" {
		t.Errorf("command名 = %q/%q", value.Commands[0].Name, value.Commands[1].Name)
	}
	// §12は「`tool_id`は正規IDで、commandごとにexactly 1 toolへ対応する」と定める。
	if value.Commands[1].ToolID.String() != "node" {
		t.Errorf("npmのtool_id = %q, want node", value.Commands[1].ToolID)
	}
}

func TestShimIndexRoundTrip(t *testing.T) {
	value, parseErr := ParseShimIndex([]byte(specShimIndexTOML))
	if parseErr != nil {
		t.Fatalf("ParseShimIndex = %v", parseErr)
	}
	data, encodeErr := EncodeShimIndex(value)
	if encodeErr != nil {
		t.Fatalf("EncodeShimIndex = %v", encodeErr)
	}
	again, reparseErr := ParseShimIndex(data)
	if reparseErr != nil {
		t.Fatalf("再parse = %v\n%s", reparseErr, data)
	}
	if len(again.Commands) != len(value.Commands) {
		t.Fatalf("command件数 = %d, want %d", len(again.Commands), len(value.Commands))
	}
	for index, command := range again.Commands {
		if command != value.Commands[index] {
			t.Errorf("commands[%d] = %+v, want %+v", index, command, value.Commands[index])
		}
	}
	assertTrailingLF(t, data)
}

// TestShimIndexRejects は§12のname一意・昇順とgrammarを固定する。
func TestShimIndexRejects(t *testing.T) {
	tests := []struct {
		name string
		toml string
	}{
		{"name降順", strings.Replace(specShimIndexTOML,
			"name = \"node\"\ntool_id = \"node\"\n\n[[commands]]\nname = \"npm\"",
			"name = \"npm\"\ntool_id = \"node\"\n\n[[commands]]\nname = \"node\"", 1)},
		{"name重複", strings.Replace(specShimIndexTOML, `name = "npm"`, `name = "node"`, 1)},
		{"name大文字", strings.Replace(specShimIndexTOML, `name = "node"`, `name = "Node"`, 1)},
		{"nameに区切り", strings.Replace(specShimIndexTOML, `name = "node"`, `name = "a/b"`, 1)},
		{"name空", strings.Replace(specShimIndexTOML, `name = "node"`, `name = ""`, 1)},
		{"tool_idがalias形式", strings.Replace(specShimIndexTOML, `tool_id = "node"`, `tool_id = "Node_JS"`, 1)},
		{"receipt_index_revision欠落",
			strings.Replace(specShimIndexTOML, "receipt_index_revision = 5\n", "", 1)},
		{"client_version欠落",
			strings.Replace(specShimIndexTOML, "client_version = \"2026.08.07.00\"\n", "", 1)},
		{"unknown command key", specShimIndexTOML + "extra = 1\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := ParseShimIndex([]byte(test.toml)); err == nil {
				t.Error("ParseShimIndex = nil, want error")
			}
		})
	}
}

func TestParseReceiptIndexAcceptsSpecExample(t *testing.T) {
	value, err := ParseReceiptIndex([]byte(specReceiptIndexTOML))
	if err != nil {
		t.Fatalf("ParseReceiptIndex = %v", err)
	}
	if len(value.Entries) != 1 {
		t.Fatalf("件数 = %d", len(value.Entries))
	}
	entry := value.Entries[0]
	if entry.Health != domain.HealthHealthy {
		t.Errorf("health = %q", entry.Health)
	}
	if entry.ReceiptSHA256 != testDigestB {
		t.Errorf("receipt_sha256 = %q", entry.ReceiptSHA256)
	}
	want := "tools/node/versions/22.18.0/windows-amd64/.gdtvm-install.toml"
	if entry.Path != want {
		t.Errorf("path = %q, want %q", entry.Path, want)
	}
}

func TestReceiptIndexRoundTrip(t *testing.T) {
	value, parseErr := ParseReceiptIndex([]byte(specReceiptIndexTOML))
	if parseErr != nil {
		t.Fatalf("ParseReceiptIndex = %v", parseErr)
	}
	data, encodeErr := EncodeReceiptIndex(value)
	if encodeErr != nil {
		t.Fatalf("EncodeReceiptIndex = %v", encodeErr)
	}
	again, reparseErr := ParseReceiptIndex(data)
	if reparseErr != nil {
		t.Fatalf("再parse = %v\n%s", reparseErr, data)
	}
	if len(again.Entries) != 1 || again.Entries[0] != value.Entries[0] {
		t.Errorf("round tripでentryが変わった\n%+v\n%+v", again.Entries, value.Entries)
	}
	assertTrailingLF(t, data)
}

// TestReceiptIndexRejects は§13のtuple一意・sortとpath規則を固定する。
func TestReceiptIndexRejects(t *testing.T) {
	tests := []struct {
		name string
		toml string
	}{
		{"health enum外", strings.Replace(specReceiptIndexTOML, `health = "healthy"`, `health = "broken"`, 1)},
		{"path絶対", strings.Replace(specReceiptIndexTOML,
			`path = "tools/`, `path = "/tools/`, 1)},
		{"pathにbackslash", strings.Replace(specReceiptIndexTOML,
			"tools/node/versions", `tools\node\versions`, 1)},
		{"pathに相対参照", strings.Replace(specReceiptIndexTOML,
			"tools/node/versions", "tools/../node/versions", 1)},
		{"path空", strings.Replace(specReceiptIndexTOML,
			`path = "tools/node/versions/22.18.0/windows-amd64/.gdtvm-install.toml"`, `path = ""`, 1)},
		{"receipt_sha256が上流形式",
			strings.Replace(specReceiptIndexTOML, testDigestB, "sha256:"+testDigestB, 1)},
		{"platform_id未対応",
			strings.Replace(specReceiptIndexTOML, `platform_id = "windows-amd64"`, `platform_id = "darwin-arm64"`, 1)},
		{"install_id欠落",
			strings.Replace(specReceiptIndexTOML, "install_id = \""+testInstall+"\"\n", "", 1)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := ParseReceiptIndex([]byte(test.toml)); err == nil {
				t.Error("ParseReceiptIndex = nil, want error")
			}
		})
	}
}

// TestReceiptIndexRequiresSortedTuple はtupleでの一意・sortを固定する。
func TestReceiptIndexRequiresSortedTuple(t *testing.T) {
	entry := func(version string) string {
		return "[[receipts]]\ntool_id = \"node\"\nversion = \"" + version + "\"\n" +
			"platform_id = \"windows-amd64\"\ninstall_id = \"" + testInstall + "\"\n" +
			"path = \"tools/node/" + version + "/.gdtvm-install.toml\"\n" +
			"receipt_sha256 = \"" + testDigestB + "\"\nhealth = \"healthy\"\n\n"
	}
	header := strings.Split(specReceiptIndexTOML, "[[receipts]]")[0]

	// tupleのbyte順で比べる。versionの数値としての大小ではない（§7）。
	if _, err := ParseReceiptIndex([]byte(header + entry("20.0.0") + entry("22.18.0"))); err != nil {
		t.Fatalf("byte昇順が落ちた: %v", err)
	}
	if _, err := ParseReceiptIndex([]byte(header + entry("22.18.0") + entry("20.0.0"))); err == nil {
		t.Error("byte降順が通った")
	}
	if _, err := ParseReceiptIndex([]byte(header + entry("22.18.0") + entry("22.18.0"))); err == nil {
		t.Error("同一tupleの重複が通った")
	}
}

// TestStateFileSizeLimit は§21の1 MiB上限を固定する。
func TestStateFileSizeLimit(t *testing.T) {
	// commentで嵩増しする。TOMLとしては有効なまま上限だけを超えさせる。
	padding := "\n# " + strings.Repeat("x", StateFileMaxBytes)
	if _, err := ParseStateSchema([]byte(specSchemaTOML + padding)); err == nil {
		t.Error("1 MiB超過が通った")
	}
	// 上限ちょうどは通す。境界の内側を拒否していないことの確認。
	fill := StateFileMaxBytes - len(specSchemaTOML) - 3
	exact := specSchemaTOML + "\n# " + strings.Repeat("x", fill)
	if len(exact) != StateFileMaxBytes {
		t.Fatalf("test dataのsizeが%d byte（want %d）", len(exact), StateFileMaxBytes)
	}
	if _, err := ParseStateSchema([]byte(exact)); err != nil {
		t.Errorf("上限ちょうどが落ちた: %v", err)
	}
}

// assertTrailingLF は§7の「永続fileは末尾LFちょうど1つ」を確かめる。
func assertTrailingLF(t *testing.T, data []byte) {
	t.Helper()
	if len(data) == 0 {
		t.Fatal("出力が空")
	}
	if data[len(data)-1] != '\n' {
		t.Error("末尾がLFでない")
	}
	if len(data) >= 2 && data[len(data)-2] == '\n' {
		t.Error("末尾LFが2つ以上ある")
	}
}
