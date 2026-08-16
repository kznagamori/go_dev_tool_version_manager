package registry

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
)

// registryDir はrepositoryのregistry sourceである（docs/07-registry-and-tools.md §2）。
const registryDir = "../../registry"

// specManifest はdocs/07-registry-and-tools.md §3の正規例である。
// digestは実fileの値へ差し替えてある。
const specManifest = `schema = 1
tool_definition_schema = 1
client_min_version = "2026.08.07.00"

[[tools]]
id = "dotnet"
path = "tools/dotnet.toml"
sha256 = "1111111111111111111111111111111111111111111111111111111111111111"

[[tools]]
id = "go"
path = "tools/go.toml"
sha256 = "2222222222222222222222222222222222222222222222222222222222222222"

[[tools]]
id = "node"
path = "tools/node.toml"
sha256 = "3333333333333333333333333333333333333333333333333333333333333333"

[[tools]]
id = "python"
path = "tools/python.toml"
sha256 = "4444444444444444444444444444444444444444444444444444444444444444"
`

// TestParseManifestAcceptsSpecExample は§3の正規例が通ることを固定する。
func TestParseManifestAcceptsSpecExample(t *testing.T) {
	value, err := ParseManifest([]byte(specManifest))
	if err != nil {
		t.Fatalf("ParseManifest = %v", err.Cause)
	}
	if value.Schema != SchemaVersion || value.ToolDefinitionSchema != ToolDefinitionSchema {
		t.Errorf("schema = %d/%d", value.Schema, value.ToolDefinitionSchema)
	}
	if value.ClientMinVersion.String() != "2026.08.07.00" {
		t.Errorf("client_min_version = %q", value.ClientMinVersion)
	}
	// maxだけ任意である（§3）。宣言していなければzeroになる。
	if !value.ClientMaxVersion.IsZero() {
		t.Errorf("client_max_version = %q, want zero", value.ClientMaxVersion)
	}
	if len(value.Tools) != ToolCount {
		t.Fatalf("tools = %d件, want %d", len(value.Tools), ToolCount)
	}
	for index, want := range []string{"dotnet", "go", "node", "python"} {
		if value.Tools[index].ID.String() != want {
			t.Errorf("tools[%d] = %q, want %q", index, value.Tools[index].ID, want)
		}
	}
}

// TestParseManifestRejects は§3のexact key集合と契約を固定する。
//
// registryはclientへ同梱される信頼の根であり、読めない部分を推測で補うと
// digest検証の前提が崩れる。
func TestParseManifestRejects(t *testing.T) {
	cases := []struct{ name, old, value string }{
		{"unknown key", "schema = 1\n", "schema = 1\nextra = true\n"},
		{"tool entryのunknown key", `sha256 = "1111`, "note = \"x\"\nsha256 = \"1111"},
		{"schemaが1でない", "schema = 1\n", "schema = 2\n"},
		{"tool_definition_schemaが1でない", "tool_definition_schema = 1", "tool_definition_schema = 2"},
		{"schemaが無い", "schema = 1\n", ""},
		{"tool_definition_schemaが無い", "tool_definition_schema = 1\n", ""},
		{"client_min_versionが無い", `client_min_version = "2026.08.07.00"`, ""},
		{"client_min_versionがCalVerでない", `client_min_version = "2026.08.07.00"`,
			`client_min_version = "1.2.3"`},
		{"client_min_versionの日付が存在しない", `client_min_version = "2026.08.07.00"`,
			`client_min_version = "2026.02.30.00"`},
		// pathは`tools/<id>.toml`固定である。任意pathを許すと、manifestが
		// registry treeの外のfileを指せる。
		{"pathがIDと合わない", `path = "tools/go.toml"`, `path = "tools/golang.toml"`},
		{"pathがtree外", `path = "tools/go.toml"`, `path = "../go.toml"`},
		{"sha256が短い", `sha256 = "2222222222222222222222222222222222222222222222222222222222222222"`,
			`sha256 = "2222"`},
		{"sha256が大文字", `sha256 = "2222222222222222222222222222222222222222222222222222222222222222"`,
			`sha256 = "2222222222222222222222222222222222222222222222222222222222222AAA"`},
		{"sha256にprefix", `sha256 = "2222222222222222222222222222222222222222222222222222222222222222"`,
			`sha256 = "sha256:22222222222222222222222222222222222222222222222222222222222222"`},
		{"idがgrammar外", `id = "go"`, `id = "Go"`},
		// toolsはID ASCII byte順である。順序が自由だと、同じ内容のregistryが
		// 複数のbyte列を持ちうる。
		{"ID順でない", "id = \"dotnet\"\npath = \"tools/dotnet.toml\"",
			"id = \"zzz\"\npath = \"tools/zzz.toml\""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if !strings.Contains(specManifest, c.old) {
				t.Fatalf("差し替え対象 %q が正規例に無い", c.old)
			}
			source := strings.Replace(specManifest, c.old, c.value, 1)
			if _, err := ParseManifest([]byte(source)); err == nil {
				t.Fatalf("%q → %q が通った", c.old, c.value)
			} else if err.Code != domain.CodeRegistryInvalid {
				t.Fatalf("code = %s, want %s", err.Code, domain.CodeRegistryInvalid)
			}
		})
	}
}

// TestParseManifestRequiresExactlyFourTools は§3の「exactly 4件」を固定する。
func TestParseManifestRequiresExactlyFourTools(t *testing.T) {
	t.Run("3件", func(t *testing.T) {
		index := strings.Index(specManifest, `[[tools]]
id = "python"`)
		if index < 0 {
			t.Fatal("python entryが正規例に無い")
		}
		if _, err := ParseManifest([]byte(specManifest[:index])); err == nil {
			t.Fatal("3件でも通った")
		}
	})
	t.Run("5件", func(t *testing.T) {
		extra := specManifest + `
[[tools]]
id = "zig"
path = "tools/zig.toml"
sha256 = "5555555555555555555555555555555555555555555555555555555555555555"
`
		if _, err := ParseManifest([]byte(extra)); err == nil {
			t.Fatal("5件でも通った")
		}
	})
	t.Run("ID重複", func(t *testing.T) {
		duplicated := strings.Replace(specManifest,
			"id = \"python\"\npath = \"tools/python.toml\"",
			"id = \"node\"\npath = \"tools/node.toml\"", 1)
		if _, err := ParseManifest([]byte(duplicated)); err == nil {
			t.Fatal("ID重複でも通った")
		}
	})
}

// TestCheckClientVersion は§3のclient version範囲を固定する。
func TestCheckClientVersion(t *testing.T) {
	withMax := strings.Replace(specManifest,
		`client_min_version = "2026.08.07.00"`,
		"client_min_version = \"2026.08.07.00\"\nclient_max_version = \"2026.12.31.99\"", 1)
	value, err := ParseManifest([]byte(withMax))
	if err != nil {
		t.Fatalf("ParseManifest = %v", err.Cause)
	}

	cases := []struct {
		version string
		ok      bool
	}{
		{"2026.08.07.00", true},  // 下限ちょうど
		{"2026.08.06.99", false}, // 下限未満
		{"2026.10.01.00", true},
		{"2026.12.31.99", true},  // 上限ちょうど
		{"2027.01.01.00", false}, // 上限超過
	}
	for _, c := range cases {
		t.Run(c.version, func(t *testing.T) {
			client, parseErr := domain.ParseClientVersion(c.version)
			if parseErr != nil {
				t.Fatal(parseErr)
			}
			checkErr := value.CheckClientVersion(client)
			if c.ok && checkErr != nil {
				t.Fatalf("範囲内が拒否された: %v", checkErr.Cause)
			}
			if !c.ok {
				if checkErr == nil {
					t.Fatal("範囲外が通った")
				}
				if checkErr.Code != domain.CodeRegistryInvalid {
					t.Fatalf("code = %s", checkErr.Code)
				}
			}
		})
	}

	t.Run("未設定のclient version", func(t *testing.T) {
		if err := value.CheckClientVersion(domain.ClientVersion{}); err == nil {
			t.Fatal("未設定でも通った")
		}
	})
}

// TestParseManifestRejectsInvertedRange はmaxがminより小さい宣言を拒否することを
// 固定する。どのclientも読めないregistryになる。
func TestParseManifestRejectsInvertedRange(t *testing.T) {
	inverted := strings.Replace(specManifest,
		`client_min_version = "2026.08.07.00"`,
		"client_min_version = \"2026.08.07.00\"\nclient_max_version = \"2026.08.06.00\"", 1)
	if _, err := ParseManifest([]byte(inverted)); err == nil {
		t.Fatal("max < min が通った")
	}
}

// TestCheckTree は§2のexact treeを固定する。
//
// 「上記以外のentryをrelease registryへ含めない」。過不足のどちらもerrorにする。
func TestCheckTree(t *testing.T) {
	complete := ExactTree()
	if err := CheckTree(complete); err != nil {
		t.Fatalf("exact treeが拒否された: %v", err.Cause)
	}

	t.Run("欠落", func(t *testing.T) {
		if err := CheckTree(complete[:len(complete)-1]); err == nil {
			t.Fatal("欠落が通った")
		}
	})
	t.Run("余分", func(t *testing.T) {
		// helper、key、script、local bundleは存在しない（§2）。
		extra := append(append([]string(nil), complete...), "tools/helper.ps1")
		if err := CheckTree(extra); err == nil {
			t.Fatal("余分なentryが通った")
		}
	})
	t.Run("両方", func(t *testing.T) {
		mixed := append(append([]string(nil), complete[:len(complete)-1]...), "keys/signing.pub")
		if err := CheckTree(mixed); err == nil {
			t.Fatal("欠落と余分が通った")
		}
	})
}

// TestVerifyDefinitionDigest は§3のdigest照合を固定する。
func TestVerifyDefinitionDigest(t *testing.T) {
	data := []byte("schema = 1\n")
	entry := ToolEntry{Path: "tools/go.toml", SHA256: DefinitionDigest(data)}
	if err := VerifyDefinitionDigest(entry, data); err != nil {
		t.Fatalf("一致するdigestが拒否された: %v", err.Cause)
	}
	if err := VerifyDefinitionDigest(entry, append(data, ' ')); err == nil {
		t.Fatal("1 byte違いが通った")
	} else if err.Code != domain.CodeRegistryInvalid {
		t.Fatalf("code = %s", err.Code)
	}
}

// TestScopeFor は§4のcommand別load範囲を固定する。
func TestScopeFor(t *testing.T) {
	cases := []struct {
		command string
		want    LoadScope
	}{
		// registryを読まず、binaryへ埋め込んだbuild/schema情報だけを返す。
		{"version", LoadScope{}},
		// 破損箇所を診断するため読めるfileを継続する。
		{"doctor", LoadScope{ContinueOnError: true}},
		// registry headerと4 definitionを必須検証する。
		{"setup", LoadScope{Header: true, AllDefinitions: true}},
		// registry headerと対象definition digestを必須検証する。
		{"available", LoadScope{Header: true, TargetDefinition: true}},
		{"install", LoadScope{Header: true, TargetDefinition: true}},
		{"use", LoadScope{Header: true, TargetDefinition: true}},
		// state、receipt、indexを正本とする。
		{"installed", LoadScope{}},
		{"current", LoadScope{}},
		{"uninstall", LoadScope{}},
		// registry/networkを読まない。
		{"shim", LoadScope{}},
	}
	if len(cases) != CommandCount {
		t.Fatalf("case = %d件, want %d", len(cases), CommandCount)
	}
	for _, c := range cases {
		t.Run(c.command, func(t *testing.T) {
			got, err := ScopeFor(c.command)
			if err != nil {
				t.Fatalf("ScopeFor = %v", err)
			}
			if got != c.want {
				t.Fatalf("= %+v, want %+v", got, c.want)
			}
		})
	}

	// 未知commandを既定値で通さない。範囲を決め忘れたcommandが検証なしで動く。
	if _, err := ScopeFor("refresh"); err == nil {
		t.Fatal("未知commandが通った")
	}
	if len(Commands()) != CommandCount {
		t.Fatalf("Commands = %d件, want %d", len(Commands()), CommandCount)
	}
}

// TestRepositoryManifestMatchesDefinitions はrepositoryの`registry.toml`が
// `registry/tools/*.toml`の実digestと一致することを固定する。
//
// definitionを変更してmanifestを更新し忘れると、release registryのdigest検証が
// 失敗する。CIで先に気付けるようにする（§3「digestはraw file bytes」）。
func TestRepositoryManifestMatchesDefinitions(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(registryDir, ManifestPath))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	manifest, parseErr := ParseManifest(data)
	if parseErr != nil {
		t.Fatalf("ParseManifest = %v", parseErr.Cause)
	}
	for _, entry := range manifest.Tools {
		t.Run(entry.ID.String(), func(t *testing.T) {
			definition, readErr := os.ReadFile(filepath.Join(registryDir, entry.Path))
			if readErr != nil {
				t.Fatalf("ReadFile(%s): %v", entry.Path, readErr)
			}
			if verifyErr := VerifyDefinitionDigest(entry, definition); verifyErr != nil {
				t.Fatalf("%s: %v", entry.Path, verifyErr.Cause)
			}
		})
	}
}

// TestRepositoryRegistryHasNoExtraEntries はrepositoryのregistryへ§2に無い
// entryが混ざっていないことを固定する。
//
// **完全一致は検査しない。** `messages/ja.toml`と`licenses/*.txt`は§5のsource
// validatorを作るP4-02で追加するため、この時点では欠落する。余分の検出だけ先に
// 効かせ、完全一致はtree全体が揃った時点で[CheckTree]へ切り替える。
func TestRepositoryRegistryHasNoExtraEntries(t *testing.T) {
	allowed := make(map[string]struct{}, len(ExactTree()))
	for _, path := range ExactTree() {
		allowed[path] = struct{}{}
	}
	var extra []string
	walkErr := filepath.Walk(registryDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		relative, relErr := filepath.Rel(registryDir, path)
		if relErr != nil {
			return relErr
		}
		slashed := filepath.ToSlash(relative)
		if _, ok := allowed[slashed]; !ok {
			extra = append(extra, slashed)
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("Walk: %v", walkErr)
	}
	sort.Strings(extra)
	if len(extra) != 0 {
		t.Fatalf("§2に無いentryがregistryにある: %v", extra)
	}
}

// TestParseManifestRejectsEmptyValues は必須keyを空文字で宣言することを拒否する
// ことを固定する。
//
// keyが無い場合と空の値は別の失敗だが、どちらも「値が決まっていない」ため通さない。
// pointer型のfieldで区別しているので、両方に対してtestを置く。
func TestParseManifestRejectsEmptyValues(t *testing.T) {
	cases := []struct{ name, old, value string }{
		{"client_min_versionが空", `client_min_version = "2026.08.07.00"`, `client_min_version = ""`},
		{"client_max_versionが空", `client_min_version = "2026.08.07.00"`,
			"client_min_version = \"2026.08.07.00\"\nclient_max_version = \"\""},
		{"idが空", `id = "go"`, `id = ""`},
		{"pathが空", `path = "tools/go.toml"`, `path = ""`},
		{"sha256が空",
			`sha256 = "2222222222222222222222222222222222222222222222222222222222222222"`,
			`sha256 = ""`},
		{"idが無い", "id = \"go\"\n", ""},
		{"pathが無い", "path = \"tools/go.toml\"\n", ""},
		{"sha256が無い",
			"sha256 = \"2222222222222222222222222222222222222222222222222222222222222222\"\n", ""},
		{"toolsが無い", "\n[[tools]]\nid = \"dotnet\"", "\n[[tools_x]]\nid_x = \"dotnet\""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if !strings.Contains(specManifest, c.old) {
				t.Fatalf("差し替え対象 %q が正規例に無い", c.old)
			}
			source := strings.Replace(specManifest, c.old, c.value, 1)
			if _, err := ParseManifest([]byte(source)); err == nil {
				t.Fatalf("%q → %q が通った", c.old, c.value)
			} else if err.Code != domain.CodeRegistryInvalid {
				t.Fatalf("code = %s, want %s", err.Code, domain.CodeRegistryInvalid)
			}
		})
	}
}

// TestParseManifestRejectsMalformedTOML は型違いとsyntax errorを拒否し、位置を
// 添えることを固定する。
//
// registryを直す人が最初に見る情報は「どこが壊れているか」であり、汎用の
// decode error文言だけでは足りない。
func TestParseManifestRejectsMalformedTOML(t *testing.T) {
	cases := []struct{ name, old, value string }{
		{"schemaが文字列", "schema = 1\n", "schema = \"1\"\n"},
		{"tool_definition_schemaが真偽値", "tool_definition_schema = 1",
			"tool_definition_schema = true"},
		{"client_min_versionが整数", `client_min_version = "2026.08.07.00"`,
			"client_min_version = 20260807"},
		{"toolsがarray of tableでない", "\n[[tools]]\nid = \"dotnet\"",
			"\ntools = 1\n[[ignored]]\nid = \"dotnet\""},
		{"文字列が閉じていない", `id = "go"`, `id = "go`},
		{"key重複", "id = \"go\"\n", "id = \"go\"\nid = \"go\"\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if !strings.Contains(specManifest, c.old) {
				t.Fatalf("差し替え対象 %q が正規例に無い", c.old)
			}
			source := strings.Replace(specManifest, c.old, c.value, 1)
			err := requireParseFailure(t, source)
			if err.Code != domain.CodeRegistryInvalid {
				t.Fatalf("code = %s, want %s", err.Code, domain.CodeRegistryInvalid)
			}
		})
	}
}

// TestParseManifestRejectsMissingToolsTable は`[[tools]]`が1件も無いmanifestを
// 拒否することを固定する。
//
// headerだけのmanifestは「tool 0件のregistry」ではなく、書きかけである。
func TestParseManifestRejectsMissingToolsTable(t *testing.T) {
	index := strings.Index(specManifest, "[[tools]]")
	if index < 0 {
		t.Fatal("tools entryが正規例に無い")
	}
	err := requireParseFailure(t, specManifest[:index])
	if !strings.Contains(err.Cause.Error(), "tools") {
		t.Errorf("`tools`が無いことを指していない: %v", err.Cause)
	}
}

// TestParseManifestRejectsOversizedFile は上限超過を読む前に拒否することを
// 固定する（docs/04-storage-and-data.md §21「registry manifest各file 2 MiB」）。
func TestParseManifestRejectsOversizedFile(t *testing.T) {
	// 上限ちょうどはparse段階まで進む。ここでは内容が不正なのでparseで落ちるが、
	// 「size検査で落ちたのではない」ことをmessageで確かめる。
	atLimit := make([]byte, ManifestFileMaxBytes)
	for index := range atLimit {
		atLimit[index] = '\n'
	}
	if _, err := ParseManifest(atLimit); err == nil {
		t.Fatal("空manifestが通った")
	} else if strings.Contains(err.Cause.Error(), "byteを超える") {
		t.Fatalf("上限ちょうどがsize超過とされた: %v", err.Cause)
	}

	oversized := append(atLimit, '\n')
	if _, err := ParseManifest(oversized); err == nil {
		t.Fatal("上限超過が通った")
	} else if !strings.Contains(err.Cause.Error(), "byteを超える") {
		t.Fatalf("size超過として拒否されていない: %v", err.Cause)
	}
}

// TestManifestEntry はtool IDからentryを引けることを固定する。
func TestManifestEntry(t *testing.T) {
	manifest, err := ParseManifest([]byte(specManifest))
	if err != nil {
		t.Fatalf("ParseManifest = %v", err.Cause)
	}
	id, parseErr := domain.ParseToolID("node")
	if parseErr != nil {
		t.Fatal(parseErr)
	}
	entry, ok := manifest.Entry(id)
	if !ok {
		t.Fatal("node entryが見つからない")
	}
	if entry.Path != "tools/node.toml" {
		t.Errorf("path = %q", entry.Path)
	}

	// 標準4 tool以外は引けない。registryに無いtoolをdigest検証なしで扱わない。
	other, parseErr := domain.ParseToolID("zig")
	if parseErr != nil {
		t.Fatal(parseErr)
	}
	if _, ok := manifest.Entry(other); ok {
		t.Error("registryに無いtoolのentryが返った")
	}
}

// TestLoadScopeReadsRegistry はregistryを読むscopeかどうかの判定を固定する。
func TestLoadScopeReadsRegistry(t *testing.T) {
	cases := []struct {
		command string
		want    bool
	}{
		// registryを読まないのは§4が明示した4 commandとshimである。
		{"version", false},
		{"installed", false},
		{"current", false},
		{"uninstall", false},
		{"shim", false},
		// doctorは読めるfileを継続して読むため、読む側である。
		{"doctor", true},
		{"setup", true},
		{"available", true},
		{"install", true},
		{"use", true},
	}
	for _, c := range cases {
		t.Run(c.command, func(t *testing.T) {
			scope, err := ScopeFor(c.command)
			if err != nil {
				t.Fatalf("ScopeFor = %v", err)
			}
			if got := scope.ReadsRegistry(); got != c.want {
				t.Fatalf("ReadsRegistry = %t, want %t", got, c.want)
			}
		})
	}
}

// --- helper ---

// requireParseFailure はmanifestがE_REGISTRY_INVALIDで拒否されることを確かめ、
// その[domain.Error]を返す。
func requireParseFailure(t *testing.T, source string) *domain.Error {
	t.Helper()
	value, err := ParseManifest([]byte(source))
	if err == nil {
		t.Fatalf("不正なmanifestが %d件のtoolとして通った", len(value.Tools))
	}
	return err
}
