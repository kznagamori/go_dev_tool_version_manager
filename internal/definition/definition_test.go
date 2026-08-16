package definition

import (
	"fmt"
	"strings"
	"testing"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
)

// specDefinitionTOML はdocs/06-tool-definition.md §15の正規例（Node.js）から、
// 本PRが検証する§2・§4・§5・§5.1をそのまま取ったものである。
//
// §6以降のtableは本PRでは存在検査だけを行うため、最小の中身で置く。内容の検証は
// P3-01の2本目・3本目で足す。
const specDefinitionTOML = `schema = 1
schema_id = "https://github.com/kznagamori/go_dev_tool_version_manager/schemas/tool-definition/v1"

[tool]
id = "node"
name = "Node.js"
aliases = ["nodejs"]
description = "Node.js JavaScript runtime"
homepage = "https://nodejs.org/"
license = "MIT"
version_scheme = "semver"

[[platforms]]
id = "windows-amd64"
os = "windows"
arch = "amd64"
libc = "none"
artifact_kind = "official"
storage = []

[platforms.provider]
name = "Node.js project"
homepage = "https://nodejs.org/"
license = "MIT"

[platforms.version_source]
kind = "json"

[platforms.artifact]
id = "primary"

[platforms.install]
strip_components = 1

[platforms.runtime]

[platforms.validation]
`

const specDefinitionPath = "tools/node.toml"

// describe はtyped errorのCauseまで出す。Error()はCauseを含めない。
func describe(err *domain.Error) string {
	if err == nil {
		return "<nil>"
	}
	if err.Cause != nil {
		return fmt.Sprintf("%s / cause=%v", err.Error(), err.Cause)
	}
	return err.Error()
}

// parseSpec は正規例をbaseに1か所だけ差し替えたdefinitionをparseする。
func parseSpec(t *testing.T, replacements ...string) (*Definition, *domain.Error) {
	t.Helper()
	source := specDefinitionTOML
	for index := 0; index+1 < len(replacements); index += 2 {
		old, replacement := replacements[index], replacements[index+1]
		if !strings.Contains(source, old) {
			t.Fatalf("差し替え対象 %q が正規例に無い", old)
		}
		source = strings.Replace(source, old, replacement, 1)
	}
	return Parse(specDefinitionPath, []byte(source))
}

// TestParseAcceptsSpecExample は§15の正規例が通ることを固定する。
func TestParseAcceptsSpecExample(t *testing.T) {
	value, err := parseSpec(t)
	if err != nil {
		t.Fatalf("Parse = %s", describe(err))
	}
	if value.Tool.ID.String() != "node" || value.Tool.Name != "Node.js" {
		t.Errorf("tool = %q/%q", value.Tool.ID, value.Tool.Name)
	}
	if len(value.Tool.Aliases) != 1 || value.Tool.Aliases[0] != "nodejs" {
		t.Errorf("aliases = %v", value.Tool.Aliases)
	}
	if value.Tool.VersionScheme != domain.SchemeSemver {
		t.Errorf("version_scheme = %q", value.Tool.VersionScheme)
	}
	if len(value.Platforms) != 1 {
		t.Fatalf("platforms = %d件", len(value.Platforms))
	}
	platform := value.Platforms[0]
	if platform.Platform.ID() != domain.PlatformWindowsAMD64 {
		t.Errorf("platform id = %q", platform.Platform.ID())
	}
	if platform.ArtifactKind != KindOfficial {
		t.Errorf("artifact_kind = %q", platform.ArtifactKind)
	}
	// officialではlicense_noticeとadoption_reasonが空になる。
	if !platform.LicenseNotice.IsZero() {
		t.Errorf("license_notice = %q", platform.LicenseNotice)
	}
	if platform.Provider.AdoptionReason != "" {
		t.Errorf("adoption_reason = %q", platform.Provider.AdoptionReason)
	}
	if platform.Provider.Name != "Node.js project" || platform.Provider.Repository != "" {
		t.Errorf("provider = %+v", platform.Provider)
	}
	// §6以降のtableは中身を解釈せず保持する。
	if platform.VersionSource == nil || platform.Artifact == nil ||
		platform.Install == nil || platform.Runtime == nil || platform.Validation == nil {
		t.Errorf("raw tableが保持されていない: %+v", platform)
	}
	if platform.Storage == nil || len(platform.Storage) != 0 {
		t.Errorf("storage = %v, want 空配列", platform.Storage)
	}
}

// TestParseAcceptsThirdPartyPlatform は§5.1のthird-party契約を固定する。
func TestParseAcceptsThirdPartyPlatform(t *testing.T) {
	value, err := parseSpec(t,
		`artifact_kind = "official"`, `artifact_kind = "third-party"
license_notice = "license.python.build_standalone"`,
		`license = "MIT"

[platforms.version_source]`, `license = "LicenseRef-python-build-standalone"
repository = "https://github.com/astral-sh/python-build-standalone"
adoption_reason = "CPythonが再配置可能な公式archiveを配布していないため"

[platforms.version_source]`)
	if err != nil {
		t.Fatalf("Parse = %s", describe(err))
	}
	platform := value.Platforms[0]
	if platform.ArtifactKind != KindThirdParty {
		t.Fatalf("artifact_kind = %q", platform.ArtifactKind)
	}
	if platform.LicenseNotice.String() != "license.python.build_standalone" {
		t.Errorf("license_notice = %q", platform.LicenseNotice)
	}
	if platform.Provider.Repository == "" || platform.Provider.AdoptionReason == "" {
		t.Errorf("third-partyの必須fieldが空: %+v", platform.Provider)
	}
	// SPDX listに無いlicenseもLicenseRef-形式なら通る。
	if platform.Provider.License != "LicenseRef-python-build-standalone" {
		t.Errorf("provider.license = %q", platform.Provider.License)
	}
}

// TestParseRejectsSchemaMismatch は§13-2を固定する。
func TestParseRejectsSchemaMismatch(t *testing.T) {
	tests := []struct {
		name       string
		old, value string
		wantReason string
	}{
		{"schemaが2", "schema = 1", "schema = 2", reasonSchema},
		{"schema_idが別値", SchemaID, SchemaID + "x", reasonSchemaID},
		{"schemaが欠落", "schema = 1\n", "", reasonMissing},
		{"schema_idが欠落", `schema_id = "` + SchemaID + `"` + "\n", "", reasonMissing},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := parseSpec(t, test.old, test.value)
			if err == nil {
				t.Fatal("Parse = nil, want error")
			}
			assertReason(t, err, test.wantReason)
		})
	}
}

// TestParseRejectsUnknownAndDuplicate は§1・§13-1を固定する。
func TestParseRejectsUnknownAndDuplicate(t *testing.T) {
	tests := []struct{ name, old, value string }{
		{"top-levelのunknown key", "schema = 1", "schema = 1\nextra = 1"},
		{"tool内のunknown key", `id = "node"`, `id = "node"` + "\nextra = 1"},
		{"platform内のunknown key", `id = "windows-amd64"`, `id = "windows-amd64"` + "\nextra = 1"},
		{"provider内のunknown key", `name = "Node.js project"`, `name = "Node.js project"` + "\nextra = 1"},
		{"重複key", "schema = 1", "schema = 1\nschema = 1"},
		{"重複table", "[tool]", "[tool]\n[tool]"},
		{"型違い", "schema = 1", `schema = "1"`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := parseSpec(t, test.old, test.value)
			if err == nil {
				t.Fatal("Parse = nil, want error")
			}
			assertReason(t, err, reasonDecode)
		})
	}
}

// TestParseRejectsMalformedInput は§1のbyte制約を固定する。
func TestParseRejectsMalformedInput(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{"空", nil},
		{"BOM付き", append([]byte{0xEF, 0xBB, 0xBF}, specDefinitionTOML...)},
		{"UTF-8でない", []byte("schema = \"\xff\xfe\"\n")},
		{"上限超過", []byte(strings.Repeat("#", FileMaxBytes+1))},
		{"TOMLとして壊れている", []byte("schema = \n")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := Parse(specDefinitionPath, test.data); err == nil {
				t.Error("Parse = nil, want error")
			}
		})
	}
}

// requiredKeyCount は本PRが「全件必須」を検査するscalar keyの件数である。
//
// §2の2件、§4の7件、§5の6件（`storage`含む）、§5.1のofficial必須3件。
// 件数を定数で持つのは、正規例が縮んで検査が空振りしたときに気付くためである。
const requiredKeyCount = 18

// TestParseRequiresEveryKey は§2・§4・§5・§5.1の「全件必須」を1行ずつ落として確かめる。
//
// 個別testでは網羅できないため、正規例からkeyを1件ずつ削除して全件が拒否される
// ことを機械的に確かめる。
//
// 走査対象は`[platforms.version_source]`より前の行だけとする。それ以降のtableは
// 本PRでは存在検査だけを行い、中のkeyの必須性はP3-01の2本目・3本目が決める。
func TestParseRequiresEveryKey(t *testing.T) {
	lines := strings.Split(strings.TrimRight(specDefinitionTOML, "\n"), "\n")
	limit := len(lines)
	for index, line := range lines {
		if strings.TrimSpace(line) == "[platforms.version_source]" {
			limit = index
			break
		}
	}
	if limit == len(lines) {
		t.Fatal("正規例に `[platforms.version_source]` が無い")
	}

	removed := 0
	for index := 0; index < limit; index++ {
		trimmed := strings.TrimSpace(lines[index])
		if trimmed == "" || strings.HasPrefix(trimmed, "[") {
			continue
		}
		key, _, found := strings.Cut(trimmed, " =")
		if !found {
			continue
		}
		removed++
		t.Run(fmt.Sprintf("%d行目の%s", index+1, key), func(t *testing.T) {
			source := strings.Join(append(append([]string{}, lines[:index]...), lines[index+1:]...), "\n")
			if _, err := Parse(specDefinitionPath, []byte(source+"\n")); err == nil {
				t.Errorf("key %q が無くても通った", key)
			}
		})
	}
	if removed != requiredKeyCount {
		t.Fatalf("削除対象のkeyが%d件、want %d件。正規例が縮んでいないか確認する",
			removed, requiredKeyCount)
	}
}

// TestParseRejectsTableRemoval は必須tableの欠落を固定する。
func TestParseRejectsTableRemoval(t *testing.T) {
	tables := []string{
		"[platforms.version_source]\nkind = \"json\"\n",
		"[platforms.artifact]\nid = \"primary\"\n",
		"[platforms.install]\nstrip_components = 1\n",
		"[platforms.runtime]\n",
		"[platforms.validation]\n",
		"[platforms.provider]\nname = \"Node.js project\"\nhomepage = \"https://nodejs.org/\"\nlicense = \"MIT\"\n",
	}
	for _, table := range tables {
		name, _, _ := strings.Cut(table, "\n")
		t.Run(name, func(t *testing.T) {
			source := strings.Replace(specDefinitionTOML, table, "", 1)
			if source == specDefinitionTOML {
				t.Fatalf("table %q が正規例に無い", name)
			}
			_, err := Parse(specDefinitionPath, []byte(source))
			if err == nil {
				t.Fatal("必須tableが無くても通った")
			}
			assertReason(t, err, reasonMissing)
		})
	}
	// `[[platforms]]`自体が無い場合も拒否する。
	if _, err := Parse(specDefinitionPath, []byte(specDefinitionTOML[:strings.Index(
		specDefinitionTOML, "[[platforms]]")])); err == nil {
		t.Error("platformが0件でも通った")
	}
}

// assertReason は1件目の診断のreason codeを確かめる。
func assertReason(t *testing.T, err *domain.Error, want string) {
	t.Helper()
	if err.Code != domain.CodeDefinitionInvalid {
		t.Errorf("code = %q, want %q", err.Code, domain.CodeDefinitionInvalid)
	}
	if err.PathRole != domain.RoleToolDefinition {
		t.Errorf("path role = %q, want %q", err.PathRole, domain.RoleToolDefinition)
	}
	first, ok := err.Parameters["first"]
	if !ok {
		t.Fatalf("parametersにfirstが無い: %v", err.Parameters)
	}
	got, _ := first.Str()
	if got != want {
		t.Errorf("先頭のreason = %q, want %q（cause=%v）", got, want, err.Cause)
	}
}
