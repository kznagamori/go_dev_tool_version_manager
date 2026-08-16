package definition

import (
	"fmt"
	"strings"
	"testing"
)

// replaceSpec は正規例の1か所を差し替えてparseする。
func replaceSpec(t *testing.T, old, value string) (*Definition, error) {
	t.Helper()
	if !strings.Contains(specDefinitionTOML, old) {
		t.Fatalf("差し替え対象 %q が正規例に無い", old)
	}
	source := strings.Replace(specDefinitionTOML, old, value, 1)
	definition, err := Parse(specDefinitionPath, []byte(source))
	if err == nil {
		return definition, nil
	}
	return nil, err
}

// rejectSpec は差し替えが指定のreason codeで拒否されることを確かめる。
func rejectSpec(t *testing.T, old, value, wantReason string) {
	t.Helper()
	if !strings.Contains(specDefinitionTOML, old) {
		t.Fatalf("差し替え対象 %q が正規例に無い", old)
	}
	source := strings.Replace(specDefinitionTOML, old, value, 1)
	_, err := Parse(specDefinitionPath, []byte(source))
	if err == nil {
		t.Fatalf("%q → %q が通った", old, value)
	}
	assertReason(t, err, wantReason)
}

// acceptSpec は差し替えが通ることを確かめる。
func acceptSpec(t *testing.T, old, value string) *Definition {
	t.Helper()
	definition, err := replaceSpec(t, old, value)
	if err != nil {
		t.Fatalf("%q → %q が落ちた: %v", old, value, err)
	}
	return definition
}

// TestArtifactRejects は§7.1のartifact keyを固定する。
func TestArtifactRejects(t *testing.T) {
	tests := []struct{ name, old, value, wantReason string }{
		{"idがprimaryでない", `id = "primary"`, `id = "main"`, reasonEnum},
		{"sourceがenum外", `source = "template"`, `source = "mirror"`, reasonEnum},
		{"formatがenum外", `format = "zip"`, `format = "tar.xz"`, reasonEnum},
		{"sizeが負", "size = 0", "size = -1", reasonLimit},
		{"urlがHTTP", `url = "https://nodejs.org/dist/v{{version}}/node-v{{version}}-win-x64.zip"`,
			`url = "http://nodejs.org/dist/v{{version}}/x.zip"`, reasonURL},
		{"urlのhostにtemplate", `url = "https://nodejs.org/dist/v{{version}}/node-v{{version}}-win-x64.zip"`,
			`url = "https://{{version}}.nodejs.org/x.zip"`, reasonURL},
		{"urlに未知template", `url = "https://nodejs.org/dist/v{{version}}/node-v{{version}}-win-x64.zip"`,
			`url = "https://nodejs.org/{{payload}}/x.zip"`, reasonTemplate},
		{"fileに区切り", `file = "node-v{{version}}-win-x64.zip"`,
			`file = "dist/node-{{version}}.zip"`, reasonText},
		{"fileに未知template", `file = "node-v{{version}}-win-x64.zip"`,
			`file = "node-{{platform.id}}.zip"`, reasonTemplate},
		// 宣言していないmetadata keyは、render時に値が無くinstallできない。
		{"未宣言のmetadata", `file = "node-v{{version}}-win-x64.zip"`,
			`file = "node-{{metadata.tag}}.zip"`, reasonTemplate},
		{"未宣言のasset field", `file = "node-v{{version}}-win-x64.zip"`,
			`file = "{{asset.name}}"`, reasonTemplate},
		{"対応の取れない波括弧", `file = "node-v{{version}}-win-x64.zip"`,
			`file = "node-{version}.zip"`, reasonTemplate},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rejectSpec(t, test.old, test.value, test.wantReason)
		})
	}
}

// TestArtifactSourceConditionalKeys は§7.1の`source`別契約を固定する。
func TestArtifactSourceConditionalKeys(t *testing.T) {
	t.Run("source=templateでselector", func(t *testing.T) {
		rejectSpec(t, "[platforms.artifact.checksum]",
			"[platforms.artifact.selector]\nname_regex = \"^x$\"\n\n[platforms.artifact.checksum]",
			reasonConditional)
	})

	// asset sourceの正規例（§16.3のPython）へ組み替える。
	assetArtifact := `[platforms.artifact]
id = "primary"
source = "asset"
url = ""
file = ""
format = "zip"
size = 0
redirect_hosts = ["release-assets.githubusercontent.com"]

[platforms.artifact.selector]
name_regex = "^node-v(?P<version>[0-9.]+)-win-x64[.]zip$"
os = "windows"
arch = "amd64"

[platforms.artifact.checksum]
kind = "asset-field"
algorithm = "sha256"
`
	currentArtifact := specDefinitionTOML[strings.Index(specDefinitionTOML, "[platforms.artifact]"):strings.Index(specDefinitionTOML, "[platforms.install]")]

	t.Run("source=assetが通る", func(t *testing.T) {
		value := acceptSpec(t, currentArtifact, assetArtifact+"\n")
		artifact := value.Platforms[0].Artifact
		if artifact.Source != SourceAsset || artifact.Selector == nil {
			t.Fatalf("artifact = %+v", artifact)
		}
		if artifact.Checksum.Kind != ChecksumAssetField ||
			artifact.Checksum.Algorithm != AlgorithmSHA256 {
			t.Errorf("checksum = %+v", artifact.Checksum)
		}
		if len(artifact.RedirectHosts) != 1 {
			t.Errorf("redirect_hosts = %v", artifact.RedirectHosts)
		}
	})

	// §7.1は「`source=asset`の`url`/`file`は空なら選択assetの`url`/`name`を使い、
	// 非空なら選択assetを`{{asset.<field>}}`で参照できるtemplateとしてrenderする」
	// と定める。Goのようにasset listがdownload URLを持たない配布元に使う。
	// `{{asset.<field>}}`は`asset_fields`の宣言と突き合わせるため、asset field
	// を宣言する§16.2のGo sourceへ差し替えて確かめる。
	templateArtifact := strings.Replace(assetArtifact,
		`url = ""`+"\n"+`file = ""`,
		`url = "https://go.dev/dl/{{asset.name}}"`+"\n"+`file = "{{asset.name}}"`, 1)

	t.Run("source=assetでurl/file templateが通る", func(t *testing.T) {
		value, err := parseSpec(t,
			specVersionSourceBlock, goSourceBlock,
			currentArtifact, templateArtifact+"\n")
		if err != nil {
			t.Fatalf("Parse = %s", describe(err))
		}
		artifact := value.Platforms[0].Artifact
		if artifact.URL != "https://go.dev/dl/{{asset.name}}" || artifact.File != "{{asset.name}}" {
			t.Fatalf("artifact = %+v", artifact)
		}
		if artifact.Selector == nil {
			t.Fatal("selectorが必要である")
		}
	})

	// URLとfile名の出所が食い違う定義を受理しない。
	rejectAssetTemplate := func(t *testing.T, artifactBlock string) {
		t.Helper()
		_, err := parseSpec(t,
			specVersionSourceBlock, goSourceBlock,
			currentArtifact, artifactBlock+"\n")
		if err == nil {
			t.Fatal("片方だけのtemplateが通った")
		}
	}

	t.Run("source=assetでurlだけ非空", func(t *testing.T) {
		rejectAssetTemplate(t, strings.Replace(assetArtifact,
			`url = ""`, `url = "https://go.dev/dl/{{asset.name}}"`, 1))
	})

	t.Run("source=assetでfileだけ非空", func(t *testing.T) {
		rejectAssetTemplate(t, strings.Replace(assetArtifact,
			`file = ""`, `file = "{{asset.name}}"`, 1))
	})

	t.Run("source=assetでselectorが無い", func(t *testing.T) {
		reduced := assetArtifact[:strings.Index(assetArtifact, "[platforms.artifact.selector]")] +
			assetArtifact[strings.Index(assetArtifact, "[platforms.artifact.checksum]"):]
		rejectSpec(t, currentArtifact, reduced+"\n", reasonMissing)
	})

	t.Run("selectorに条件が無い", func(t *testing.T) {
		empty := strings.Replace(assetArtifact,
			"name_regex = \"^node-v(?P<version>[0-9.]+)-win-x64[.]zip$\"\nos = \"windows\"\narch = \"amd64\"\n",
			"", 1)
		rejectSpec(t, currentArtifact, empty+"\n", reasonConditional)
	})

	t.Run("source=templateでkind=asset-field", func(t *testing.T) {
		// assetのdigestを使うため、artifact自体もassetから選ぶ必要がある。
		rejectSpec(t, "kind = \"text-file\"\nurl = \"https://nodejs.org/dist/v{{version}}/SHASUMS256.txt\"\nline_format = \"sha256-space-filename\"",
			"kind = \"asset-field\"\nalgorithm = \"sha256\"", reasonConditional)
	})
}

// TestChecksumConditionalKeys は§7.2のkind別契約を固定する。
func TestChecksumConditionalKeys(t *testing.T) {
	const textFile = "kind = \"text-file\"\n" +
		"url = \"https://nodejs.org/dist/v{{version}}/SHASUMS256.txt\"\n" +
		"line_format = \"sha256-space-filename\""
	tests := []struct{ name, value, wantReason string }{
		{"kindがenum外", "kind = \"inline\"", reasonEnum},
		{"line_formatがenum外",
			"kind = \"text-file\"\nurl = \"https://x.invalid/S.txt\"\nline_format = \"md5-space-filename\"",
			reasonEnum},
		{"text-fileでurlが無い", "kind = \"text-file\"\nline_format = \"sha256-space-filename\"",
			reasonMissing},
		{"text-fileでline_formatが無い", "kind = \"text-file\"\nurl = \"https://x.invalid/S.txt\"",
			reasonMissing},
		// 「`text-file`は`line_format`がalgorithmを含むため`algorithm`を書かない」
		{"text-fileでalgorithm", textFile + "\nalgorithm = \"sha256\"", reasonConditional},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rejectSpec(t, textFile, test.value, test.wantReason)
		})
	}

	t.Run("checksum tableが無い", func(t *testing.T) {
		source := removeSections(t, "platforms.artifact.checksum")
		_, err := Parse(specDefinitionPath, []byte(source))
		if err == nil {
			t.Fatal("checksumが無くても通った")
		}
		assertReason(t, err, reasonMissing)
	})
}

// TestRedirectHostsRejects は§7.1の`redirect_hosts`を固定する。
func TestRedirectHostsRejects(t *testing.T) {
	const anchor = `format = "zip"`
	tests := []struct{ name, hosts, wantReason string }{
		{"wildcard", `["*.example.invalid"]`, reasonText},
		{"大文字", `["Example.Invalid"]`, reasonText},
		{"scheme付き", `["https://example.invalid"]`, reasonText},
		{"port付き", `["example.invalid:443"]`, reasonText},
		{"空", `[""]`, reasonText},
		{"重複", `["a.invalid", "a.invalid"]`, reasonDuplicate},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rejectSpec(t, anchor, anchor+"\nredirect_hosts = "+test.hosts, test.wantReason)
		})
	}
	// 正当なhostは通る。過剰な拒否をしていないことの確認。
	acceptSpec(t, anchor, anchor+"\nredirect_hosts = [\"release-assets.githubusercontent.com\"]")
}

// TestInstallStripComponents は§9を固定する。
func TestInstallStripComponents(t *testing.T) {
	for _, value := range []string{"0", "1"} {
		acceptSpec(t, "strip_components = 1", "strip_components = "+value)
	}
	// 「2階層以上の除去が必要なartifactはv0.1の標準registryへ採用しない」
	for _, value := range []string{"2", "-1", "10"} {
		t.Run(value, func(t *testing.T) {
			rejectSpec(t, "strip_components = 1", "strip_components = "+value, reasonEnum)
		})
	}
	t.Run("型違い", func(t *testing.T) {
		rejectSpec(t, "strip_components = 1", `strip_components = "1"`, reasonDecode)
	})
}

// TestArtifactRequiresEveryKey は§7.1のtemplate source必須keyを固定する。
func TestArtifactRequiresEveryKey(t *testing.T) {
	keys := []string{"id", "source", "url", "file", "format", "size"}
	for _, key := range keys {
		t.Run(key, func(t *testing.T) {
			lines := strings.Split(specDefinitionTOML, "\n")
			start := -1
			for index, line := range lines {
				if strings.TrimSpace(line) == "[platforms.artifact]" {
					start = index
					break
				}
			}
			if start < 0 {
				t.Fatal("`[platforms.artifact]`が正規例に無い")
			}
			for index := start + 1; index < len(lines); index++ {
				if strings.HasPrefix(strings.TrimSpace(lines[index]), "[") {
					t.Fatalf("key %q がartifact tableに無い", key)
				}
				if strings.HasPrefix(strings.TrimSpace(lines[index]), key+" = ") {
					reduced := strings.Join(
						append(append([]string{}, lines[:index]...), lines[index+1:]...), "\n")
					if _, err := Parse(specDefinitionPath, []byte(reduced)); err == nil {
						t.Errorf("artifact key %q が無くても通った", key)
					}
					return
				}
			}
		})
	}
	_ = fmt.Sprint()
}
