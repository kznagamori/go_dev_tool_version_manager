package registry

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/definition"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
)

// repositorySource はrepositoryのregistryをSourceとして読む。
func repositorySource(t *testing.T) Source {
	t.Helper()
	source := make(Source)
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
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		source[filepath.ToSlash(relative)] = data
		return nil
	})
	if walkErr != nil {
		t.Fatalf("Walk: %v", walkErr)
	}
	return source
}

// TestValidateSourceAcceptsRepositoryRegistry はrepositoryのregistryが§5の
// 10項目をすべて満たすことを固定する。
//
// これがreleaseへ入るregistryそのものである。仕様表と食い違ったまま公開される
// のを防ぐのが§5の目的なので、CIで常に検査する。
func TestValidateSourceAcceptsRepositoryRegistry(t *testing.T) {
	report := ValidateSource(repositorySource(t))
	for _, finding := range report.Findings {
		t.Errorf("不適合: %s", finding)
	}
	if err := report.Err(); err != nil {
		t.Fatalf("Err = %v", err.Cause)
	}
}

// TestValidateSourceReportsEveryCheck は§5の10項目それぞれが実際に不適合を
// 検出することを固定する。
//
// 検査を書いたつもりで実装が抜けていても、正常なregistryだけを通す testでは
// 気付けない。項番ごとに壊した入力を作り、その項番の findingが出ることを見る。
func TestValidateSourceReportsEveryCheck(t *testing.T) {
	cases := []struct {
		check  int
		name   string
		mutate func(t *testing.T, source Source)
	}{
		{
			1, "§2に無いentryを足す",
			func(_ *testing.T, source Source) {
				// 「helper、key、script、local bundle directoryは存在しない」（§2）。
				source["tools/helper.ps1"] = []byte("x")
			},
		},
		{
			2, "definitionが上限を超える",
			func(t *testing.T, source Source) {
				data := source["tools/go.toml"]
				padded := append([]byte(nil), data...)
				padding := make([]byte, definition.FileMaxBytes)
				for index := range padding {
					padding[index] = '\n'
				}
				source["tools/go.toml"] = append(padded, padding...)
				syncDigest(t, source, "go")
			},
		},
		{
			3, "definitionのdigestが一致しない",
			func(_ *testing.T, source Source) {
				source["tools/go.toml"] = append(append([]byte(nil), source["tools/go.toml"]...), '\n')
			},
		},
		{
			4, "aliasが別toolのIDと衝突する",
			func(t *testing.T, source Source) {
				replaceInDefinition(t, source, "node",
					`aliases = ["nodejs"]`, `aliases = ["go"]`)
			},
		},
		{
			5, "platformを1件へ減らす",
			func(t *testing.T, source Source) {
				data := string(source["tools/go.toml"])
				index := strings.Index(data, "\n[[platforms]]\nid = \""+linuxPlatform+"\"")
				if index < 0 {
					t.Fatal("linux platformが見つからない")
				}
				setDefinition(t, source, "go", data[:index]+"\n")
			},
		},
		{
			6, "§7.2に無いrequired commandを足す",
			func(t *testing.T, source Source) {
				// commandを減らす方向はprobeが参照しているためdefinition自体が
				// 不正になり、§5-3で先に落ちる。§7.2の「完全な集合」は過剰側でも
				// 破れるので、そちらで検査する。
				extra := "\n[[platforms.runtime.commands]]\n" +
					"name = \"goimports\"\n" +
					"target = \"{{payload}}/bin/goimports.exe\"\n" +
					"args = []\n" +
					"environment_profile = \"default\"\n" +
					"required = true\n" +
					"working_directory = \"inherit\"\n" +
					"passthrough_signals = true\n"
				data := string(source["tools/go.toml"])
				marker := "\n[[platforms.runtime.commands]]\nname = \"gofmt\"\n" +
					"target = \"{{payload}}/bin/gofmt.exe\"\n"
				index := strings.Index(data, marker)
				if index < 0 {
					t.Fatal("Windows gofmt commandが見つからない")
				}
				setDefinition(t, source, "go", data[:index]+extra+data[index:])
			},
		},
		{
			7, "license textを取り除く",
			func(_ *testing.T, source Source) {
				delete(source, PythonLicensePath)
			},
		},
		{
			9, "license_notice無しで非OSI licenseを宣言する",
			func(t *testing.T, source Source) {
				// Windows platformの`license_notice`だけを消す。
				replaceInDefinition(t, source, "dotnet",
					"license_notice = \"license.dotnet.windows_library_license\"\n", "")
			},
		},
		{
			10, "lifecycle_mapからupstream enumを1件落とす",
			func(t *testing.T, source Source) {
				replaceInDefinition(t, source, "dotnet", "maintenance = \"supported\"\n", "")
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			source := repositorySource(t)
			c.mutate(t, source)
			report := ValidateSource(source)
			if len(report.Findings) == 0 {
				t.Fatal("不適合が1件も出なかった")
			}
			found := false
			for _, finding := range report.Findings {
				if finding.Check == c.check {
					found = true
				}
			}
			if !found {
				t.Fatalf("§5-%d の findingが出なかった: %v", c.check, report.Findings)
			}
			if err := report.Err(); err == nil {
				t.Fatal("Errがnilを返した")
			} else if err.Code != domain.CodeRegistryInvalid {
				t.Fatalf("code = %s, want %s", err.Code, domain.CodeRegistryInvalid)
			}
		})
	}
}

// TestValidateSourceCoversEveryCheckNumber は§5の10項目すべてに検査があることを
// 固定する。
//
// [TestValidateSourceReportsEveryCheck]は項番8を壊す入力を作れない。両platformの
// version集合はdefinition parserが検査しており、片方だけを壊すと先に§5-3の
// definition parse errorになるためである。項番の網羅はここで別に見る。
func TestValidateSourceCoversEveryCheckNumber(t *testing.T) {
	// §5-8はdefinition.Parseの`definition.platform_version_set_mismatch`が
	// 担保する。§5-3として報告されるため、独立の項番を持たない。
	handledElsewhere := map[int]string{
		8: "definition.Parseがplatform_version_set_mismatchで検査し、§5-3として報告する",
	}
	implemented := map[int]bool{
		1: true, 2: true, 3: true, 4: true, 5: true,
		6: true, 7: true, 9: true, 10: true,
	}
	for check := 1; check <= SourceCheckCount; check++ {
		if implemented[check] {
			continue
		}
		if _, ok := handledElsewhere[check]; ok {
			continue
		}
		t.Errorf("§5-%d の検査が無い", check)
	}
	if len(implemented)+len(handledElsewhere) != SourceCheckCount {
		t.Fatalf("項目数 = %d, want %d",
			len(implemented)+len(handledElsewhere), SourceCheckCount)
	}
}

// TestValidateSourceChecksEveryContractField は§7〜§10の表が定めるfieldごとに
// 不一致を検出することを固定する。
//
// 契約表を持つだけでは、どのfieldを実際に突き合わせているかが分からない。field
// 単位で1件ずつ壊し、その field名を含む findingが出ることを見る。
//
// definitionとして内部整合を保てる変更だけを使う。`version_scheme`や storage ID
// のように、変えるとdefinition parseが先に落ちるfieldは§5-3として報告され、
// §5-6の検査に到達しない。
func TestValidateSourceChecksEveryContractField(t *testing.T) {
	cases := []struct {
		field    string
		tool     string
		check    int
		old, new string
		// want は findingのReasonに含まれる文字列である。
		want string
	}{
		{
			field: "tool license", tool: "go", check: 6,
			want: "tool license",
		},
		{
			field: "homepage", tool: "go", check: 6,
			want: "homepage",
		},
		{
			field: "artifact_kind", tool: "go", check: 6,
			// third-partyは`repository`と`adoption_reason`が必須（§5.1）。
			// 揃えないとdefinition parseが先に落ちる。
			old: "name = \"Go project\"\nhomepage = \"https://go.dev/\"\n" +
				"license = \"BSD-3-Clause\"",
			new: "name = \"Go project\"\nrepository = \"https://github.com/golang/go\"\n" +
				"homepage = \"https://go.dev/\"\nlicense = \"BSD-3-Clause\"\n" +
				"adoption_reason = \"provider.python.standalone_reason\"",
			want: "artifact_kind",
		},
		{
			field: "checksum.algorithm", tool: "go", check: 6,
			old: "algorithm = \"sha256\"", new: "algorithm = \"sha512\"",
			want: "checksum.algorithm",
		},
		{
			field: "strip_components", tool: "go", check: 6,
			old: "strip_components = 1", new: "strip_components = 0",
			want: "strip_components",
		},
		{
			field: "storage kind", tool: "go", check: 6,
			old:  "id = \"workspace\"\nkind = \"runtime-data\"",
			new:  "id = \"workspace\"\nkind = \"build-cache\"",
			want: "storage \"workspace\" のkind",
		},
		{
			field: "storage scope", tool: "go", check: 6,
			// scopeを変えるとpurgeの許容値も変わる（§8）。両方を整合させる。
			old: "id = \"workspace\"\nkind = \"runtime-data\"\nscope = \"tool\"\n" +
				"path = \"workspace\"\npurge = \"explicit\"",
			new: "id = \"workspace\"\nkind = \"runtime-data\"\nscope = \"version\"\n" +
				"path = \"workspace\"\npurge = \"with-version\"",
			want: "storage \"workspace\" のscope",
		},
		{
			field: "provider.license", tool: "go", check: 6,
			old: "name = \"Go project\"\nhomepage = \"https://go.dev/\"\n" +
				"license = \"BSD-3-Clause\"",
			new:  "name = \"Go project\"\nhomepage = \"https://go.dev/\"\nlicense = \"MIT\"",
			want: "provider.license",
		},
		{
			field: "lifecycle_mapの写像先", tool: "dotnet", check: 10,
			old: "active = \"supported\"", new: "active = \"eol\"",
			want: "lifecycle_map[\"active\"]",
		},
	}

	for _, c := range cases {
		t.Run(c.field, func(t *testing.T) {
			source := repositorySource(t)
			switch c.field {
			case "tool license":
				// `[tool]`のlicenseだけを差し替える。provider側と同じ文字列を
				// 持つtoolがあるため、table単位で位置を特定する。
				replaceToolValue(t, source, c.tool, "license", "MIT")
			case "homepage":
				replaceToolValue(t, source, c.tool, "homepage", "https://example.com/")
			case "artifact_kind":
				replaceInDefinition(t, source, c.tool, c.old, c.new)
				replaceInDefinition(t, source, c.tool,
					"artifact_kind = \"official\"", "artifact_kind = \"third-party\"")
			default:
				replaceInDefinition(t, source, c.tool, c.old, c.new)
			}

			report := ValidateSource(source)
			var found bool
			for _, finding := range report.Findings {
				if finding.Check == c.check && strings.Contains(finding.Reason, c.want) {
					found = true
				}
			}
			if !found {
				t.Fatalf("%q を含む§5-%d の findingが出なかった: %v",
					c.want, c.check, report.Findings)
			}
		})
	}
}

// checksum.line_formatとversion_scheme、storage IDは契約表に持つが、値を変えると
// definition parseが先に落ちるため単独の不一致を作れない。
//   - `line_format`はdefinitionが1値のenumとして拒否する。
//   - `version_scheme`を変えると固定versionがそのschemeで解釈できなくなる。
//   - storage IDを変えるとenvironment profileの`{{storage.<id>}}`が解決できない。
//
// いずれも§5-3として報告されるため、契約表の値が実定義と一致することは
// [TestValidateSourceAcceptsRepositoryRegistry]が担保する。

// TestValidateSourceRejectsStorageSetMismatch はstorage集合の過不足を検出する
// ことを固定する。
func TestValidateSourceRejectsStorageSetMismatch(t *testing.T) {
	source := repositorySource(t)
	// §7.3に無いstorageを1件足す。environment profileから参照しないため
	// definition単体では整合する。
	extra := "\n[[platforms.storage]]\n" +
		"id = \"scratch\"\n" +
		"kind = \"runtime-data\"\n" +
		"scope = \"tool\"\n" +
		"path = \"scratch\"\n" +
		"purge = \"explicit\"\n"
	data := string(source["tools/go.toml"])
	marker := "\n[[platforms.storage]]\nid = \"workspace\"\n"
	index := strings.Index(data, marker)
	if index < 0 {
		t.Fatal("workspace storageが見つからない")
	}
	setDefinition(t, source, "go", data[:index]+extra+data[index:])

	report := ValidateSource(source)
	var found bool
	for _, finding := range report.Findings {
		if finding.Check == 6 && strings.Contains(finding.Reason, "storage = [") {
			found = true
		}
	}
	if !found {
		t.Fatalf("storage集合の不一致が報告されない: %v", report.Findings)
	}
}

// `lifecycle_map`を宣言しないtoolがmapを持つ場合は、definition側が
// `kind=json-index`以外での宣言を`definition.kind_key_forbidden`として拒否する
// （§5-3として報告される）。registry側で重ねて検査すると到達しない分岐になるため、
// [checkLifecycleMap]はcontractがmapを持つtoolだけを見る。

// TestValidateSourceReportsMissingDefinition はmanifestが指すdefinitionが無い
// 場合を報告することを固定する。
func TestValidateSourceReportsMissingDefinition(t *testing.T) {
	source := repositorySource(t)
	delete(source, "tools/go.toml")

	report := ValidateSource(source)
	var found bool
	for _, finding := range report.Findings {
		if finding.Check == 3 && strings.Contains(finding.Reason, "manifestが指すdefinitionが無い") {
			found = true
		}
	}
	if !found {
		t.Fatalf("definition欠落が報告されない: %v", report.Findings)
	}
}

// TestValidateSourceReportsBrokenMessageCatalog はcatalogの内容契約違反を
// 報告することを固定する。
//
// §5はcatalogの内容を項目として挙げていないが、検査せずに通すとcatalogだけが
// 未検証でrelease archiveへ入る。
func TestValidateSourceReportsBrokenMessageCatalog(t *testing.T) {
	source := repositorySource(t)
	source[MessageCatalogPath] = []byte("error.internal = \"{api_key}\"\n")

	report := ValidateSource(source)
	var found bool
	for _, finding := range report.Findings {
		if finding.Check == 2 && finding.Path == MessageCatalogPath {
			found = true
		}
	}
	if !found {
		t.Fatalf("catalogの契約違反が報告されない: %v", report.Findings)
	}
}

// TestValidateSourceRejectsUnknownLicenseIdentifier は判定表に無いlicense識別子を
// 承認/非承認のどちらとも扱わないことを固定する。
//
// 未知の識別子をOSI承認とみなすと、制限的なlicenseの配布物が`license_notice`
// なしで通る。非承認とみなすと、正当なOSS licenseへ不要な承認要求が出る。
func TestValidateSourceRejectsUnknownLicenseIdentifier(t *testing.T) {
	source := repositorySource(t)
	// `[tool].license`ではなく`[platforms.provider].license`を差し替える。
	// §5第9項が見るのは配布物のlicenseである。
	replaceInDefinition(t, source, "go",
		"name = \"Go project\"\nhomepage = \"https://go.dev/\"\nlicense = \"BSD-3-Clause\"",
		"name = \"Go project\"\nhomepage = \"https://go.dev/\"\nlicense = \"Zlib\"")

	report := ValidateSource(source)
	var found bool
	for _, finding := range report.Findings {
		if finding.Check == 9 && strings.Contains(finding.Reason, "OSI承認判定表に無い") {
			found = true
		}
	}
	if !found {
		t.Fatalf("未知のlicense識別子が判定表の欠落として報告されない: %v", report.Findings)
	}
}

// TestValidateSourceRejectsLicenseNoticeOnOSSLicense はOSI承認licenseへ
// `license_notice`を宣言することを拒否することを固定する（§5第9項の逆向き）。
//
// 「OSS licenseのplatformへ宣言しない」（docs/06-tool-definition.md §5）。
// 不要な承認要求は、本当に承認が要る場面の重みを下げる。
func TestValidateSourceRejectsLicenseNoticeOnOSSLicense(t *testing.T) {
	source := repositorySource(t)
	replaceInDefinition(t, source, "go",
		"artifact_kind = \"official\"",
		"artifact_kind = \"official\"\nlicense_notice = \"license.dotnet.windows_library_license\"")

	report := ValidateSource(source)
	var found bool
	for _, finding := range report.Findings {
		if finding.Check == 9 && strings.Contains(finding.Reason, "OSI承認OSS licenseである") {
			found = true
		}
	}
	if !found {
		t.Fatalf("OSS licenseへのlicense_notice宣言が報告されない: %v", report.Findings)
	}
}

// TestValidateSourceRejectsExtraLifecycleMapEntry はupstream enumに無い値を
// `lifecycle_map`へ残すことを拒否することを固定する。
//
// upstreamが廃止したphaseの写像が残っていることに気付けなくなる。
func TestValidateSourceRejectsExtraLifecycleMapEntry(t *testing.T) {
	source := repositorySource(t)
	replaceInDefinition(t, source, "dotnet",
		"eol = \"eol\"\n", "eol = \"eol\"\nretired = \"eol\"\n")

	report := ValidateSource(source)
	var found bool
	for _, finding := range report.Findings {
		if finding.Check == 10 && strings.Contains(finding.Reason, "upstream enumに無い") {
			found = true
		}
	}
	if !found {
		t.Fatalf("余分なlifecycle_map entryが報告されない: %v", report.Findings)
	}
}

// TestValidateSourceStopsWhenManifestUnreadable はmanifestが読めない場合に
// definitionを要する項目を実行しないことを固定する。
//
// manifestが無いとdefinitionとの対応付けができない。無関係な findingを並べると、
// 直すべき1件が埋もれる。
func TestValidateSourceStopsWhenManifestUnreadable(t *testing.T) {
	source := repositorySource(t)
	delete(source, ManifestPath)

	report := ValidateSource(source)
	if len(report.Findings) == 0 {
		t.Fatal("manifest欠落が報告されない")
	}
	for _, finding := range report.Findings {
		switch finding.Check {
		case 1, 3:
			// treeの欠落とmanifest欠落は報告される。
		case 7:
			t.Errorf("license textはあるのに報告された: %s", finding)
		default:
			if finding.Check >= 4 {
				t.Errorf("manifestが無いのに§5-%d を実行した: %s", finding.Check, finding)
			}
		}
	}
}

// TestSourceFindingString は診断の表記を固定する。
func TestSourceFindingString(t *testing.T) {
	withPath := SourceFinding{Check: 6, Path: "tools/go.toml", Reason: "x"}
	if got := withPath.String(); got != "§5-6 tools/go.toml: x" {
		t.Errorf("String = %q", got)
	}
	treeWide := SourceFinding{Check: 1, Reason: "y"}
	if got := treeWide.String(); got != "§5-1: y" {
		t.Errorf("String = %q", got)
	}
}

// TestStandardToolsCoverManifest は契約表が標準4 toolを漏れなく持つことを
// 固定する。
func TestStandardToolsCoverManifest(t *testing.T) {
	if len(standardTools) != ToolCount {
		t.Fatalf("契約表 = %d件, want %d", len(standardTools), ToolCount)
	}
	source := repositorySource(t)
	manifest, err := ParseManifest(source[ManifestPath])
	if err != nil {
		t.Fatalf("ParseManifest = %v", err.Cause)
	}
	for index, entry := range manifest.Tools {
		if standardTools[index].id != entry.ID.String() {
			t.Errorf("契約表[%d] = %q, manifest = %q",
				index, standardTools[index].id, entry.ID)
		}
		if _, ok := standardToolByID(entry.ID.String()); !ok {
			t.Errorf("契約表に %q が無い", entry.ID)
		}
	}
	if _, ok := standardToolByID("zig"); ok {
		t.Error("標準4 tool以外が契約表から引けた")
	}

	// 各toolはexactly 2 platformの契約を持つ（§5第5項）。
	for _, tool := range standardTools {
		if len(tool.platforms) != 2 {
			t.Errorf("%s のplatform契約 = %d件, want 2", tool.id, len(tool.platforms))
		}
		for _, id := range []string{windowsPlatform, linuxPlatform} {
			if _, ok := tool.platforms[id]; !ok {
				t.Errorf("%s に %s の契約が無い", tool.id, id)
			}
		}
	}
}

// --- helper ---

// replaceToolValue は`[tool]` tableの1 keyだけを差し替える。
//
// providerと同じkey名・同じ値を持つtoolがあるため、table単位で位置を特定する。
func replaceToolValue(t *testing.T, source Source, id, key, value string) {
	t.Helper()
	path := "tools/" + id + ".toml"
	lines := strings.Split(string(source[path]), "\n")
	inTable := false
	for index, raw := range lines {
		trimmed := strings.TrimSpace(raw)
		if strings.HasPrefix(trimmed, "[") {
			inTable = trimmed == "[tool]"
			continue
		}
		if inTable && strings.HasPrefix(trimmed, key+" = ") {
			lines[index] = key + " = \"" + value + "\""
			setDefinition(t, source, id, strings.Join(lines, "\n"))
			return
		}
	}
	t.Fatalf("%s の [tool] に %q が無い", path, key)
}

// setDefinition はdefinitionを差し替え、manifestのdigestを追随させる。
func setDefinition(t *testing.T, source Source, id, data string) {
	t.Helper()
	source["tools/"+id+".toml"] = []byte(data)
	syncDigest(t, source, id)
}

// replaceInDefinition はdefinition内の1件を置換し、digestを追随させる。
func replaceInDefinition(t *testing.T, source Source, id, old, new string) {
	t.Helper()
	path := "tools/" + id + ".toml"
	data := string(source[path])
	if !strings.Contains(data, old) {
		t.Fatalf("%s に %q が無い", path, old)
	}
	setDefinition(t, source, id, strings.Replace(data, old, new, 1))
}

// syncDigest はmanifestのdigestをdefinitionの実体へ合わせる。
//
// digest不一致（§5-3）を意図しない testで巻き込まないためである。
func syncDigest(t *testing.T, source Source, id string) {
	t.Helper()
	digest := DefinitionDigest(source["tools/"+id+".toml"])
	manifest := string(source[ManifestPath])
	marker := "id = \"" + id + "\"\npath = \"tools/" + id + ".toml\"\nsha256 = \""
	index := strings.Index(manifest, marker)
	if index < 0 {
		t.Fatalf("manifestに %q のentryが無い", id)
	}
	start := index + len(marker)
	end := strings.IndexByte(manifest[start:], '"')
	if end < 0 {
		t.Fatal("manifestのsha256が閉じていない")
	}
	source[ManifestPath] = []byte(manifest[:start] + digest + manifest[start+end:])
}

// TestCheckPlatformContractReportsEveryMismatch は契約表の各fieldの不一致を
// 検出することを固定する。
//
// [TestValidateSourceChecksEveryContractField]はdefinitionとして内部整合を
// 保てる変更しか作れず、`version_scheme`やversion source種別のように変えると
// definition parseが先に落ちるfieldへ到達できない。ここでは合成した
// [definition.Platform]を直接渡し、突き合わせそのものを検査する。
func TestCheckPlatformContractReportsEveryMismatch(t *testing.T) {
	contract, ok := standardToolByID("go")
	if !ok {
		t.Fatal("go の契約が無い")
	}
	platform, err := domain.ParsePlatform(windowsPlatform)
	if err != nil {
		t.Fatal(err)
	}

	// baseline は契約どおりのplatformである。ここから1 fieldずつ壊す。
	baseline := func() definition.Platform {
		storage := make([]definition.Storage, 0, len(contract.storage))
		for _, entry := range contract.storage {
			storage = append(storage, definition.Storage{
				ID: entry.id, Kind: entry.kind, Scope: entry.scope,
			})
		}
		commands := make([]definition.Command, 0, len(contract.commands))
		for _, name := range contract.commands {
			commands = append(commands, definition.Command{Name: name, Required: true})
		}
		return definition.Platform{
			Platform:      platform,
			ArtifactKind:  contract.providerKind,
			Provider:      definition.Provider{License: contract.platforms[windowsPlatform].providerLicense},
			VersionSource: definition.VersionSource{Kind: contract.sourceKind},
			Artifact: definition.Artifact{
				Source: contract.artifactSource,
				Checksum: definition.ArtifactChecksum{
					Kind:       contract.checksumKind,
					Algorithm:  contract.checksumAlgorithm,
					LineFormat: contract.checksumLineFormat,
				},
			},
			Install: definition.Install{StripComponents: contract.stripComponents},
			Storage: storage,
			Runtime: definition.Runtime{Commands: commands},
		}
	}

	// 壊していないbaselineは通る。壊した側の findingが本当にその変更由来だと
	// 言えるようにする。
	var clean SourceReport
	checkPlatformContract("tools/go.toml", baseline(), contract, &clean)
	for _, finding := range clean.Findings {
		t.Errorf("契約どおりのplatformで不適合: %s", finding)
	}

	cases := []struct {
		field  string
		mutate func(*definition.Platform)
		want   string
	}{
		{"version_source.kind", func(p *definition.Platform) {
			p.VersionSource.Kind = definition.SourceStatic
		}, "version_source.kind"},
		{"artifact.source", func(p *definition.Platform) {
			p.Artifact.Source = definition.SourceTemplate
		}, "artifact.source"},
		{"checksum.kind", func(p *definition.Platform) {
			p.Artifact.Checksum.Kind = definition.ChecksumTextFile
		}, "checksum.kind"},
		{"checksum.line_format", func(p *definition.Platform) {
			p.Artifact.Checksum.LineFormat = "sha256-space-filename"
		}, "checksum.line_format"},
		{"storage欠落", func(p *definition.Platform) {
			p.Storage[0].ID = "renamed"
		}, "storage \"config\" が無い"},
		{"required command不足", func(p *definition.Platform) {
			p.Runtime.Commands = p.Runtime.Commands[:1]
		}, "required command"},
		{"required commandがrequiredでない", func(p *definition.Platform) {
			p.Runtime.Commands[1].Required = false
		}, "required command"},
	}
	for _, c := range cases {
		t.Run(c.field, func(t *testing.T) {
			value := baseline()
			c.mutate(&value)
			var report SourceReport
			checkPlatformContract("tools/go.toml", value, contract, &report)
			var found bool
			for _, finding := range report.Findings {
				if strings.Contains(finding.Reason, c.want) {
					found = true
				}
			}
			if !found {
				t.Fatalf("%q を含む findingが出なかった: %v", c.want, report.Findings)
			}
		})
	}
}

// TestCheckPlatformContractRejectsUnknownPlatform は契約表に無いplatform IDの
// license検査を飛ばすことを固定する。
//
// platform tuple自体の不正は§5-5が報告する。ここで重ねて報告すると、同じ原因の
// findingが2件出て直すべき箇所が読みにくくなる。
func TestCheckPlatformContractRejectsUnknownPlatform(t *testing.T) {
	contract, _ := standardToolByID("go")
	linux, err := domain.ParsePlatform(linuxPlatform)
	if err != nil {
		t.Fatal(err)
	}
	// linuxの契約だけを取り除いた表を作る。
	trimmed := contract
	trimmed.platforms = map[string]standardPlatform{
		windowsPlatform: contract.platforms[windowsPlatform],
	}
	value := definition.Platform{Platform: linux, Provider: definition.Provider{License: "MIT"}}

	var report SourceReport
	checkLicense("tools/go.toml", linux.ID(), value, trimmed, &report)
	if len(report.Findings) != 0 {
		t.Fatalf("契約表に無いplatformでlicense findingが出た: %v", report.Findings)
	}
}

// TestCheckLifecycleMapReportsEmptyMap は宣言が必要なtoolでmapが空の場合を
// 報告することを固定する。
func TestCheckLifecycleMapReportsEmptyMap(t *testing.T) {
	contract, _ := standardToolByID("dotnet")
	platform, err := domain.ParsePlatform(windowsPlatform)
	if err != nil {
		t.Fatal(err)
	}
	value := definition.Platform{Platform: platform}

	var report SourceReport
	checkLifecycleMap("tools/dotnet.toml", platform.ID(), value, contract, &report)
	var found bool
	for _, finding := range report.Findings {
		if finding.Check == 10 && strings.Contains(finding.Reason, "lifecycle_mapが無い") {
			found = true
		}
	}
	if !found {
		t.Fatalf("空のlifecycle_mapが報告されない: %v", report.Findings)
	}
}

// TestCheckToolContractRejectsUnknownTool は§6の標準4 tool以外を拒否することを
// 固定する。
//
// 「その他のtool ID、unsupported placeholder、helper definitionをregistryへ
// 入れない」（§6）。
func TestCheckToolContractRejectsUnknownTool(t *testing.T) {
	id, err := domain.ParseToolID("zig")
	if err != nil {
		t.Fatal(err)
	}
	value := &definition.Definition{Path: "tools/zig.toml", Tool: definition.Tool{ID: id}}

	var report SourceReport
	checkToolContract(value, &report)
	var found bool
	for _, finding := range report.Findings {
		if finding.Check == 6 && strings.Contains(finding.Reason, "§6の標準4 toolに無い") {
			found = true
		}
	}
	if !found {
		t.Fatalf("標準4 tool以外が報告されない: %v", report.Findings)
	}
}

// TestCheckStaticAssetAlgorithmReportsMismatch はstatic assetのdigest algorithm
// 不一致を報告することを固定する。
func TestCheckStaticAssetAlgorithmReportsMismatch(t *testing.T) {
	contract, _ := standardToolByID("python")
	platform, err := domain.ParsePlatform(windowsPlatform)
	if err != nil {
		t.Fatal(err)
	}
	value := definition.Platform{
		Platform: platform,
		VersionSource: definition.VersionSource{
			StaticVersions: []definition.StaticVersion{{
				Assets: []definition.StaticAsset{{
					Name:            "cpython.tar.gz",
					DigestAlgorithm: definition.AlgorithmSHA512,
				}},
			}},
		},
	}

	var report SourceReport
	checkStaticAssetAlgorithm("tools/python.toml", platform.ID(), value, contract, &report)
	var found bool
	for _, finding := range report.Findings {
		if strings.Contains(finding.Reason, "digest_algorithm") {
			found = true
		}
	}
	if !found {
		t.Fatalf("static assetのalgorithm不一致が報告されない: %v", report.Findings)
	}
}

// TestValidateSourceStopsWhenManifestBroken はmanifestが壊れている場合に
// definitionを要する項目を実行しないことを固定する。
//
// 欠落（[TestValidateSourceStopsWhenManifestUnreadable]）とparse失敗は別経路
// である。どちらでもdefinitionとの対応付けができない。
func TestValidateSourceStopsWhenManifestBroken(t *testing.T) {
	source := repositorySource(t)
	source[ManifestPath] = []byte("schema = 2\n")

	report := ValidateSource(source)
	var manifestReported bool
	for _, finding := range report.Findings {
		if finding.Check == 3 && finding.Path == ManifestPath {
			manifestReported = true
		}
		if finding.Check >= 4 {
			t.Errorf("manifestが壊れているのに§5-%d を実行した: %s", finding.Check, finding)
		}
	}
	if !manifestReported {
		t.Fatalf("manifestのparse失敗が報告されない: %v", report.Findings)
	}
}

// TestCheckToolContractReportsToolLevelMismatch は`[tool]`側のfield不一致を
// 報告することを固定する。
//
// `version_scheme`は実定義で変えるとversionが解釈できずdefinition parseが先に
// 落ちるため、合成した定義で突き合わせを検査する。
func TestCheckToolContractReportsToolLevelMismatch(t *testing.T) {
	contract, _ := standardToolByID("go")
	id, err := domain.ParseToolID("go")
	if err != nil {
		t.Fatal(err)
	}
	value := &definition.Definition{
		Path: "tools/go.toml",
		Tool: definition.Tool{
			ID:            id,
			License:       contract.toolLicense,
			Homepage:      contract.homepage,
			VersionScheme: domain.SchemeSemver, // 契約はgo scheme。
		},
	}

	var report SourceReport
	checkToolContract(value, &report)
	var found bool
	for _, finding := range report.Findings {
		if finding.Check == 6 && strings.Contains(finding.Reason, "version_scheme") {
			found = true
		}
	}
	if !found {
		t.Fatalf("version_schemeの不一致が報告されない: %v", report.Findings)
	}
}

// TestCheckCommandsRejectsRenamedCommand は件数が同じでも名前が違えば拒否する
// ことを固定する。
//
// 「required command集合は本章とdefinition/fixture/contract testで完全一致
// させる」（§6）。件数だけの一致で通すと、shim名の取り違えを見逃す。
func TestCheckCommandsRejectsRenamedCommand(t *testing.T) {
	contract, _ := standardToolByID("go")
	platform, err := domain.ParsePlatform(windowsPlatform)
	if err != nil {
		t.Fatal(err)
	}
	value := definition.Platform{
		Platform: platform,
		Runtime: definition.Runtime{Commands: []definition.Command{
			{Name: "go", Required: true},
			// `gofmt`ではなく`goimports`。件数は同じである。
			{Name: "goimports", Required: true},
		}},
	}

	var report SourceReport
	checkCommands("tools/go.toml", platform.ID(), value, contract, &report)
	var found bool
	for _, finding := range report.Findings {
		if strings.Contains(finding.Reason, "required command") {
			found = true
		}
	}
	if !found {
		t.Fatalf("command名の取り違えが報告されない: %v", report.Findings)
	}
}
