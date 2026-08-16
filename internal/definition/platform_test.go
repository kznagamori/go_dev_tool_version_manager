package definition

import (
	"fmt"
	"strings"
	"testing"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
)

// TestParseChecksPlatformTuple は§5のIDとOS/arch/libcの一致を固定する（§13-4）。
//
// IDだけを見るとdefinitionが誤ったOSを書いても気付けず、個別fieldだけを見ると
// 仕様表にない組を受理してしまう。両方を突き合わせる。
func TestParseChecksPlatformTuple(t *testing.T) {
	tests := []struct {
		name       string
		old, value string
		wantReason string
	}{
		{"未知のplatform ID", `id = "windows-amd64"`, `id = "windows-arm64"`, reasonEnum},
		{"osが不一致", `os = "windows"`, `os = "linux"`, reasonPlatformTuple},
		{"archが不一致", `arch = "amd64"`, `arch = "arm64"`, reasonPlatformTuple},
		{"libcが不一致", `libc = "none"`, `libc = "glibc"`, reasonPlatformTuple},
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

	// Linux側のtupleは通る。片方のOSだけを検査していないことの確認。
	_, err := parseSpec(t,
		`id = "windows-amd64"`, `id = "linux-amd64-glibc"`,
		`os = "windows"`, `os = "linux"`,
		`libc = "none"`, `libc = "glibc"`)
	if err != nil {
		t.Errorf("linux platformが落ちた: %s", describe(err))
	}
}

// TestParseRejectsDuplicatePlatform は§5の「同一tupleは1件」を固定する。
func TestParseRejectsDuplicatePlatform(t *testing.T) {
	block := specDefinitionTOML[strings.Index(specDefinitionTOML, "[[platforms]]"):]
	_, err := Parse(specDefinitionPath, []byte(specDefinitionTOML+"\n"+block))
	if err == nil {
		t.Fatal("同じplatform IDが2件でも通った")
	}
	assertReason(t, err, reasonDuplicate)
}

// TestParseRejectsTooManyPlatforms は§21の「definition platform 2」を固定する。
func TestParseRejectsTooManyPlatforms(t *testing.T) {
	linux := strings.NewReplacer(
		`id = "windows-amd64"`, `id = "linux-amd64-glibc"`,
		`os = "windows"`, `os = "linux"`,
		`libc = "none"`, `libc = "glibc"`,
	).Replace(specDefinitionTOML[strings.Index(specDefinitionTOML, "[[platforms]]"):])
	windows := specDefinitionTOML[strings.Index(specDefinitionTOML, "[[platforms]]"):]

	// 2件はちょうど上限で通る。
	if _, err := Parse(specDefinitionPath, []byte(specDefinitionTOML+"\n"+linux)); err != nil {
		t.Fatalf("2 platformが落ちた: %s", describe(err))
	}
	// 3件は上限超過。IDの重複より先に件数で落ちる。
	_, err := Parse(specDefinitionPath, []byte(specDefinitionTOML+"\n"+linux+"\n"+windows))
	if err == nil {
		t.Fatal("3 platformが通った")
	}
	assertReason(t, err, reasonLimit)
}

// TestParseChecksProviderConditionalKeys は§5.1のartifact_kind別契約を固定する。
func TestParseChecksProviderConditionalKeys(t *testing.T) {
	const providerLicense = `license = "MIT"

[platforms.version_source]`

	t.Run("officialにadoption_reason", func(t *testing.T) {
		_, err := parseSpec(t, providerLicense,
			`license = "MIT"
adoption_reason = "不要な理由"

[platforms.version_source]`)
		if err == nil {
			t.Fatal("officialのadoption_reasonが通った")
		}
		assertReason(t, err, reasonProviderKey)
	})

	t.Run("third-partyでrepository欠落", func(t *testing.T) {
		_, err := parseSpec(t,
			`artifact_kind = "official"`, `artifact_kind = "third-party"`,
			providerLicense, `license = "MIT"
adoption_reason = "理由"

[platforms.version_source]`)
		if err == nil {
			t.Fatal("third-partyのrepository欠落が通った")
		}
		assertReason(t, err, reasonMissing)
	})

	t.Run("third-partyでadoption_reason欠落", func(t *testing.T) {
		_, err := parseSpec(t,
			`artifact_kind = "official"`, `artifact_kind = "third-party"`,
			providerLicense, `license = "MIT"
repository = "https://github.com/example/build"

[platforms.version_source]`)
		if err == nil {
			t.Fatal("third-partyのadoption_reason欠落が通った")
		}
		assertReason(t, err, reasonMissing)
	})

	t.Run("officialのrepositoryは任意", func(t *testing.T) {
		value, err := parseSpec(t, providerLicense, `license = "MIT"
repository = "https://github.com/nodejs/node"

[platforms.version_source]`)
		if err != nil {
			t.Fatalf("Parse = %s", describe(err))
		}
		if value.Platforms[0].Provider.Repository == "" {
			t.Error("repositoryが取り込まれていない")
		}
	})
}

// TestParseRejectsInvalidURL は§4・§5.1のURL契約を固定する。
func TestParseRejectsInvalidURL(t *testing.T) {
	tests := []struct{ name, url string }{
		{"HTTP", "http://nodejs.org/"},
		{"scheme無し", "nodejs.org/"},
		{"userinfo付き", "https://user:pass@nodejs.org/"},
		{"host無し", "https:///dist"},
		{"query付き", "https://nodejs.org/?token=x"},
		{"fragment付き", "https://nodejs.org/#x"},
		{"hostに大文字", "https://NodeJS.org/"},
		{"hostがASCIIでない", "https://ノード.example/"},
		{"前後空白", " https://nodejs.org/"},
		{"上限超過", "https://nodejs.org/" + strings.Repeat("a", URLMaxBytes)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := parseSpec(t, `homepage = "https://nodejs.org/"
license = "MIT"
version_scheme`, fmt.Sprintf(`homepage = %q
license = "MIT"
version_scheme`, test.url))
			if err == nil {
				t.Fatalf("URL %q が通った", test.url)
			}
			assertReason(t, err, reasonURL)
		})
	}
}

// TestParseChecksToolIDBasename は§4の「file basenameと一致」を固定する。
//
// fileを別名でcopyしただけの重複定義がregistryへ入るのを防ぐ。
func TestParseChecksToolIDBasename(t *testing.T) {
	source := []byte(specDefinitionTOML)
	if _, err := Parse("tools/node.toml", source); err != nil {
		t.Fatalf("一致するpathが落ちた: %s", describe(err))
	}
	_, err := Parse("tools/nodejs.toml", source)
	if err == nil {
		t.Fatal("basenameが違っても通った")
	}
	assertReason(t, err, reasonBasename)
}

// TestParseChecksAliases は§4のalias契約を固定する。
func TestParseChecksAliases(t *testing.T) {
	t.Run("空配列は通る", func(t *testing.T) {
		value, err := parseSpec(t, `aliases = ["nodejs"]`, `aliases = []`)
		if err != nil {
			t.Fatalf("Parse = %s", describe(err))
		}
		if len(value.Tool.Aliases) != 0 {
			t.Errorf("aliases = %v", value.Tool.Aliases)
		}
	})
	t.Run("grammar違反", func(t *testing.T) {
		_, err := parseSpec(t, `aliases = ["nodejs"]`, `aliases = ["NodeJS"]`)
		if err == nil {
			t.Fatal("uppercase aliasが通った")
		}
		assertReason(t, err, reasonIdentifier)
	})
	t.Run("alias同士の重複", func(t *testing.T) {
		_, err := parseSpec(t, `aliases = ["nodejs"]`, `aliases = ["nodejs", "nodejs"]`)
		if err == nil {
			t.Fatal("重複aliasが通った")
		}
		assertReason(t, err, reasonDuplicate)
	})
	t.Run("tool IDと同じalias", func(t *testing.T) {
		// IDと同じaliasは解決の起点が2つになり、どちらが正規かを型で区別できない。
		_, err := parseSpec(t, `aliases = ["nodejs"]`, `aliases = ["node"]`)
		if err == nil {
			t.Fatal("tool IDと同じaliasが通った")
		}
		assertReason(t, err, reasonDuplicate)
	})
	t.Run("上限超過", func(t *testing.T) {
		entries := make([]string, AliasMax+1)
		for index := range entries {
			entries[index] = fmt.Sprintf("%q", fmt.Sprintf("alias-%d", index))
		}
		_, err := parseSpec(t, `aliases = ["nodejs"]`,
			"aliases = ["+strings.Join(entries, ", ")+"]")
		if err == nil {
			t.Fatal("alias上限超過が通った")
		}
		assertReason(t, err, reasonLimit)
	})
}

// TestParseChecksLicenseNotice は§5の`license_notice`を固定する。
func TestParseChecksLicenseNotice(t *testing.T) {
	t.Run("message IDとして読む", func(t *testing.T) {
		value, err := parseSpec(t, `artifact_kind = "official"`,
			`artifact_kind = "official"
license_notice = "license.dotnet.windows_library_license"`)
		if err != nil {
			t.Fatalf("Parse = %s", describe(err))
		}
		if value.Platforms[0].LicenseNotice.String() != "license.dotnet.windows_library_license" {
			t.Errorf("license_notice = %q", value.Platforms[0].LicenseNotice)
		}
	})
	t.Run("message ID grammar違反", func(t *testing.T) {
		_, err := parseSpec(t, `artifact_kind = "official"`,
			`artifact_kind = "official"
license_notice = "Restrictive License"`)
		if err == nil {
			t.Fatal("message IDでない値が通った")
		}
		assertReason(t, err, reasonMessageID)
	})
}

// TestParseRejectsInvalidEnums は§4・§5のenumを固定する。
func TestParseRejectsInvalidEnums(t *testing.T) {
	tests := []struct{ name, old, value string }{
		{"version_scheme", `version_scheme = "semver"`, `version_scheme = "calver"`},
		{"artifact_kind", `artifact_kind = "official"`, `artifact_kind = "vendor"`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := parseSpec(t, test.old, test.value)
			if err == nil {
				t.Fatal("Parse = nil, want error")
			}
			assertReason(t, err, reasonEnum)
		})
	}
}

// TestParseRejectsInvalidText は§4の長さ・空白契約を固定する。
func TestParseRejectsInvalidText(t *testing.T) {
	tests := []struct{ name, old, value string }{
		{"nameが空", `name = "Node.js"`, `name = ""`},
		{"nameが上限超過", `name = "Node.js"`,
			fmt.Sprintf("name = %q", strings.Repeat("a", NameMaxBytes+1))},
		{"nameの前後に空白", `name = "Node.js"`, `name = " Node.js"`},
		{"descriptionが空", `description = "Node.js JavaScript runtime"`, `description = ""`},
		{"descriptionが上限超過", `description = "Node.js JavaScript runtime"`,
			fmt.Sprintf("description = %q", strings.Repeat("a", DescriptionMaxBytes+1))},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := parseSpec(t, test.old, test.value)
			if err == nil {
				t.Fatal("Parse = nil, want error")
			}
			assertReason(t, err, reasonText)
		})
	}
}

// TestParseKeepsPathRoleWithoutUserPath はdocs/10-security.md §9.2を固定する。
//
// 診断へ載せるpathはregistry rootからのrelativeであり、利用者のhome pathを
// 含まない。roleだけでなくpathも出せるのはこのためである。
func TestParseKeepsPathRoleWithoutUserPath(t *testing.T) {
	_, err := parseSpec(t, "schema = 1", "schema = 2")
	if err == nil {
		t.Fatal("Parse = nil, want error")
	}
	if err.PathRole != domain.RoleToolDefinition {
		t.Errorf("path role = %q", err.PathRole)
	}
	path, ok := err.Parameters["path"]
	if !ok {
		t.Fatalf("parametersにpathが無い: %v", err.Parameters)
	}
	value, _ := path.Str()
	if value != specDefinitionPath {
		t.Errorf("path = %q, want %q", value, specDefinitionPath)
	}
	if strings.HasPrefix(value, "/") || strings.Contains(value, ":\\") {
		t.Errorf("絶対pathが診断へ出ている: %q", value)
	}
}
