package definition

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
)

// registryToolsDir は標準toolのdefinitionを置くdirectoryである
// （docs/07-registry-and-tools.md §2）。
const registryToolsDir = "../../registry/tools"

// TestRegistryToolDefinitionsParse は`registry/tools/*.toml`がschema 1を通ることを
// 固定する。
//
// docs/06-tool-definition.md §15は「実registryと本例が食い違う場合、実装で補完せず
// 仕様/definition/fixtureを同期修正する」と定める。標準toolはregistry TOMLの
// contract testで検証し、tool固有のGo分岐を作らない（CLAUDE.md §7・§11）。
func TestRegistryToolDefinitionsParse(t *testing.T) {
	paths := registryToolPaths(t)
	if len(paths) == 0 {
		t.Fatal("registry/tools にdefinitionが1件も無い")
	}
	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			value := parseRegistryTool(t, path)
			// §4はfile basenameとtool IDの一致を要求する。
			base := filepath.Base(path)
			wantID := base[:len(base)-len(filepath.Ext(base))]
			if value.Tool.ID.String() != wantID {
				t.Errorf("tool id = %q, want %q（file basenameと一致）", value.Tool.ID, wantID)
			}
			// §5はplatform 1件以上・ID一意を要求する。v0.1の対象は2 platformである。
			if len(value.Platforms) != 2 {
				t.Fatalf("platform = %d件, want 2（windows-amd64とlinux-amd64-glibc）", len(value.Platforms))
			}
			ids := []string{value.Platforms[0].Platform.ID(), value.Platforms[1].Platform.ID()}
			sort.Strings(ids)
			if ids[0] != "linux-amd64-glibc" || ids[1] != "windows-amd64" {
				t.Errorf("platform id = %v", ids)
			}
		})
	}
}

// TestRegistryToolDefinitionsMatchSpec は§7〜§16が名指しする契約をtoolごとに
// 固定する。仕様の表と実定義が食い違ったときにtestが落ちるようにする。
func TestRegistryToolDefinitionsMatchSpec(t *testing.T) {
	cases := []struct {
		file     string
		scheme   domain.VersionScheme
		kind     VersionSourceKind
		artifact ArtifactSource
		checksum ChecksumKind
		strip    int
		// windowsCommands は§7.2・§8.2・§10.2のrequired command集合である。
		windowsCommands []string
	}{
		{
			// docs/07-registry-and-tools.md §7。asset listがdownload URLを持たない
			// ため`url`/`file`をtemplateにする（§7.1）。
			"go.toml", domain.SchemeGo, SourceJSON, SourceAsset, ChecksumAssetField, 1,
			[]string{"go", "gofmt"},
		},
		{
			// 同§8。URL templateは作れるがplatform archiveの公開有無を
			// `required_tokens`で示す。
			"node.toml", domain.SchemeSemver, SourceJSON, SourceTemplate, ChecksumTextFile, 1,
			[]string{"node", "npm", "npx"},
		},
		{
			// 同§10。archiveにtop-level directoryが無いため`strip_components=0`。
			"dotnet.toml", domain.SchemeSemver, SourceJSONIndex, SourceAsset, ChecksumAssetField, 0,
			[]string{"dotnet"},
		},
		{
			// 同§9。live releaseから毎回artifactを選ばず`static_versions`へ固定する。
			// top-level `python/`を1 component除去する。
			"python.toml", domain.SchemePython, SourceStatic, SourceAsset, ChecksumAssetField, 1,
			[]string{"python", "python3", "pip", "pip3"},
		},
	}
	for _, c := range cases {
		t.Run(c.file, func(t *testing.T) {
			value := parseRegistryTool(t, filepath.Join(registryToolsDir, c.file))
			if value.Tool.VersionScheme != c.scheme {
				t.Errorf("version_scheme = %q, want %q", value.Tool.VersionScheme, c.scheme)
			}
			windows := platformByID(t, value, "windows-amd64")
			if windows.VersionSource.Kind != c.kind {
				t.Errorf("version_source.kind = %q, want %q", windows.VersionSource.Kind, c.kind)
			}
			if windows.Artifact.Source != c.artifact {
				t.Errorf("artifact.source = %q, want %q", windows.Artifact.Source, c.artifact)
			}
			if windows.Artifact.Checksum.Kind != c.checksum {
				t.Errorf("checksum.kind = %q, want %q", windows.Artifact.Checksum.Kind, c.checksum)
			}
			if windows.Install.StripComponents != c.strip {
				t.Errorf("strip_components = %d, want %d", windows.Install.StripComponents, c.strip)
			}
			// required commandは両platformで同じ集合である。shim名がOSで変わると
			// 利用者のscriptがplatform間で動かなくなる。
			for _, id := range []string{"windows-amd64", "linux-amd64-glibc"} {
				platform := platformByID(t, value, id)
				got := commandNames(platform)
				if len(got) != len(c.windowsCommands) {
					t.Fatalf("%s command = %v, want %v", id, got, c.windowsCommands)
				}
				for index, name := range c.windowsCommands {
					if got[index] != name {
						t.Errorf("%s command[%d] = %q, want %q", id, index, got[index], name)
					}
				}
			}
		})
	}
}

// TestRegistryDotnetDeclaresWindowsLicenseNotice は§5の`license_notice`契約を
// 固定する。
//
// 「公式配布物でもOSI承認OSS licenseでないplatformには`license_notice`を宣言し、
// Planの重要要約で明示承認を求める。.NET SDKのWindows配布物が該当する」
// （CLAUDE.md §8）。Linux配布物はMITのため宣言しない。
func TestRegistryDotnetDeclaresWindowsLicenseNotice(t *testing.T) {
	value := parseRegistryTool(t, filepath.Join(registryToolsDir, "dotnet.toml"))
	windows := platformByID(t, value, "windows-amd64")
	if windows.LicenseNotice.IsZero() {
		t.Error("Windows配布物に`license_notice`が無い")
	}
	linux := platformByID(t, value, "linux-amd64-glibc")
	if !linux.LicenseNotice.IsZero() {
		t.Errorf("Linux配布物へ`license_notice`が宣言されている（%s）", linux.LicenseNotice)
	}
}

// TestRegistryDefinitionsDeclareOfficialProvider は標準3 toolがofficial artifact
// であることを固定する。third-partyはPlanでprovider/repository/license/
// adoption_reasonを常に表示する必要があり、宣言の有無が承認要件を変える。
func TestRegistryDefinitionsDeclareOfficialProvider(t *testing.T) {
	for _, file := range []string{"go.toml", "node.toml", "dotnet.toml"} {
		value := parseRegistryTool(t, filepath.Join(registryToolsDir, file))
		for _, platform := range value.Platforms {
			if platform.ArtifactKind != KindOfficial {
				t.Errorf("%s %s artifact_kind = %q, want official",
					file, platform.Platform.ID(), platform.ArtifactKind)
			}
			// officialはadoption_reasonを書けない（§5.1）。
			if platform.Provider.AdoptionReason != "" {
				t.Errorf("%s %s にadoption_reasonがある", file, platform.Platform.ID())
			}
		}
	}
}

// TestRegistryPythonDeclaresThirdPartyProvider は§5.1のthird-party契約を固定する。
//
// 「third-partyは全件必須。Planでprovider、repository、license、adoption_reasonを
// 常に表示する」。[07-registry-and-tools.md](../../docs/07-registry-and-tools.md)
// §9.1はPythonのprovider licenseをMPL-2.0とし、**`license_notice`は宣言しない**
// （archive内のPython/dependency license bundleも保持する）。
func TestRegistryPythonDeclaresThirdPartyProvider(t *testing.T) {
	value := parseRegistryTool(t, filepath.Join(registryToolsDir, "python.toml"))
	for index := range value.Platforms {
		platform := &value.Platforms[index]
		id := platform.Platform.ID()
		if platform.ArtifactKind != KindThirdParty {
			t.Errorf("%s artifact_kind = %q, want third-party", id, platform.ArtifactKind)
		}
		if platform.Provider.Repository == "" || platform.Provider.AdoptionReason == "" {
			t.Errorf("%s third-partyのrepository/adoption_reasonが空: %+v", id, platform.Provider)
		}
		if !platform.LicenseNotice.IsZero() {
			t.Errorf("%s へlicense_noticeが宣言されている（MPL-2.0はOSI承認）", id)
		}
	}
}

// TestRegistryPythonPinsUpstreamDigests は§6.6・§7.2の固定catalog契約を固定する。
//
// [07-registry-and-tools.md](../../docs/07-registry-and-tools.md)§9.2が「Windows/
// Linux exact asset name/URL/ID/size/SHA-256」をversionごとに固定すると定め、
// §7.2が「v0.1はchecksumを公開しないartifactを扱わない」と定める。digestは
// providerが公開した値であり、sizeは正整数である。
func TestRegistryPythonPinsUpstreamDigests(t *testing.T) {
	value := parseRegistryTool(t, filepath.Join(registryToolsDir, "python.toml"))
	for index := range value.Platforms {
		platform := &value.Platforms[index]
		id := platform.Platform.ID()
		versions := platform.VersionSource.StaticVersions
		if len(versions) == 0 {
			t.Fatalf("%s のstatic_versionsが空", id)
		}
		for _, version := range versions {
			// §9.2は新旧各1件の固定を求める。asset数は1 platformにつき1件である。
			if len(version.Assets) != 1 {
				t.Errorf("%s %s asset = %d件, want 1", id, version.Version, len(version.Assets))
				continue
			}
			asset := version.Assets[0]
			if asset.DigestAlgorithm != AlgorithmSHA256 {
				t.Errorf("%s %s digest_algorithm = %q", id, version.Version, asset.DigestAlgorithm)
			}
			if len(asset.Digest) != DigestHexLength(AlgorithmSHA256) {
				t.Errorf("%s %s digestのhex長 = %d", id, version.Version, len(asset.Digest))
			}
			if asset.Size <= 0 {
				t.Errorf("%s %s size = %d, want 正整数", id, version.Version, asset.Size)
			}
			if asset.ReleaseTag == "" {
				t.Errorf("%s %s release_tagが空（provider releaseに使う）", id, version.Version)
			}
			// `unknown`でも「不明と判断した調査根拠」をevidenceへ残す（§6.6）。
			if version.LifecycleEvidence == "" || version.LifecycleAssessedAt.IsZero() {
				t.Errorf("%s %s のlifecycle根拠か評価日が空", id, version.Version)
			}
		}
	}
}

// TestRegistryPythonVersionSetsMatchAcrossPlatforms は§6.6の
// 「registry validatorは両platformの正規version集合が完全一致することを検査し、
// 片方だけの更新漏れを拒否する」を固定する。
func TestRegistryPythonVersionSetsMatchAcrossPlatforms(t *testing.T) {
	value := parseRegistryTool(t, filepath.Join(registryToolsDir, "python.toml"))
	sets := make([][]string, 0, len(value.Platforms))
	for index := range value.Platforms {
		versions := make([]string, 0)
		for _, entry := range value.Platforms[index].VersionSource.StaticVersions {
			versions = append(versions, entry.Version.String())
		}
		sort.Strings(versions)
		sets = append(sets, versions)
	}
	if len(sets) != 2 {
		t.Fatalf("platform = %d件", len(sets))
	}
	if strings.Join(sets[0], ",") != strings.Join(sets[1], ",") {
		t.Fatalf("version集合が一致しない: %v / %v", sets[0], sets[1])
	}
}

// --- helper ---

func registryToolPaths(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(registryToolsDir)
	if err != nil {
		t.Fatalf("ReadDir(%s): %v", registryToolsDir, err)
	}
	var paths []string
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".toml" {
			continue
		}
		paths = append(paths, filepath.Join(registryToolsDir, entry.Name()))
	}
	sort.Strings(paths)
	return paths
}

func parseRegistryTool(t *testing.T, path string) *Definition {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	value, parseErr := Parse("tools/"+filepath.Base(path), data)
	if parseErr != nil {
		t.Fatalf("Parse(%s) = %s", path, describe(parseErr))
	}
	return value
}

func platformByID(t *testing.T, value *Definition, id string) *Platform {
	t.Helper()
	for index := range value.Platforms {
		if value.Platforms[index].Platform.ID() == id {
			return &value.Platforms[index]
		}
	}
	t.Fatalf("platform %q が無い", id)
	return nil
}

func commandNames(platform *Platform) []string {
	names := make([]string, 0, len(platform.Runtime.Commands))
	for _, command := range platform.Runtime.Commands {
		names = append(names, command.Name)
	}
	return names
}
