package registry

import (
	"fmt"
	"sort"
	"strings"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/definition"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
)

// LicenseFileMaxBytes はregistryが同梱するlicense textの上限である。
//
// docs/07-registry-and-tools.md §5第2項が「registry TOML、definition TOML、
// message/licenseがsize上限内」を要求するが、docs/04-storage-and-data.md §21の
// 表にlicense fileの行が無い。同表の「registry manifest各file 2 MiB」を
// registry treeのfileへ適用する読みを採る。
const LicenseFileMaxBytes = 2 << 20

// PythonLicensePath はPython third-party licenseのregistry内pathである（§2）。
const PythonLicensePath = "licenses/python-build-standalone-MPL-2.0.txt"

// SourceCheckCount は§5が定める検査項目数である。
//
// 件数を定数で持つのは、項目を足したときに実装漏れへ気付くためである。
const SourceCheckCount = 10

// Source はvalidate対象のregistry sourceである。
//
// keyはregistry rootからの相対path（slash区切り）、値はfileのraw bytesである。
// fileの読取り自体はApplication Serviceが`port.FileSystem`経由で行い、本packageは
// 読み込み済みのbytesだけを扱う。
type Source map[string][]byte

// SourceFinding は§5の検査で見つかった不適合1件である。
type SourceFinding struct {
	// Check は§5の項番（1〜10）である。
	Check int
	// Path はregistry rootからの相対pathである。tree全体に関わる指摘では空。
	Path string
	// Reason は不適合の内容である。
	Reason string
}

// String は`<項番> <path>: <reason>`形式で返す。
func (f SourceFinding) String() string {
	if f.Path == "" {
		return fmt.Sprintf("§5-%d: %s", f.Check, f.Reason)
	}
	return fmt.Sprintf("§5-%d %s: %s", f.Check, f.Path, f.Reason)
}

// SourceReport は§5の検査結果である。
type SourceReport struct {
	// Findings は見つかった不適合である。項番順、同項番内は検出順である。
	Findings []SourceFinding
}

// Err は不適合があれば`E_REGISTRY_INVALID`を返す。
//
// 1件目で止めず全件を集約するのは、release前検査で1件ずつしか直せないと
// 修正の往復が実用にならないためである（docs/06-tool-definition.md §13と同じ理由）。
func (r SourceReport) Err() *domain.Error {
	if len(r.Findings) == 0 {
		return nil
	}
	return invalidError(fmt.Errorf(
		"source validationで%d件の不適合: %s", len(r.Findings), r.Findings[0]))
}

// add は不適合を記録する。
func (r *SourceReport) add(check int, path, format string, args ...any) {
	r.Findings = append(r.Findings, SourceFinding{
		Check: check, Path: path, Reason: fmt.Sprintf(format, args...),
	})
}

// ValidateSource はdocs/07-registry-and-tools.md §5のrelease前検査を実行する。
//
// 10項目をすべて実行し、見つかった不適合を集約して返す。release archiveへ入る
// registryが仕様表と食い違ったまま公開されるのを防ぐのが目的であり、途中で
// 打ち切らない。
func ValidateSource(source Source) SourceReport {
	var report SourceReport

	// §5-1: directory/entry集合が§2と一致。
	paths := make([]string, 0, len(source))
	for path := range source {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	if err := CheckTree(paths); err != nil {
		report.add(1, "", "%s", err.Cause)
	}

	// §5-2: size上限。treeが欠けていてもある分は検査する。
	checkSizes(source, &report)

	// §5-3: manifestのID/file/path/digest/schema一致。
	manifest, ok := checkManifest(source, &report)

	// §5-7: Python third-party license textが存在する。
	if _, exists := source[PythonLicensePath]; !exists {
		report.add(7, PythonLicensePath, "Python third-party license textが無い")
	}

	// message catalogは§20のstrict parserで検査する。§5は項目として挙げて
	// いないが、第2項がsize、第1項がtree上の存在を要求しており、内容契約を
	// 検査せずに通すとcatalogだけが未検証で release archiveへ入る。
	if data, exists := source[MessageCatalogPath]; exists {
		if err := checkMessageCatalog(data); err != nil {
			report.add(2, MessageCatalogPath, "%s", err.Cause)
		}
	}

	if !ok {
		// manifestが読めないとdefinitionの対応付けができない。残りの項目は
		// definitionの解析結果を必要とするため、ここで返す。
		return report
	}
	checkDefinitions(source, manifest, &report)
	return report
}

// checkMessageCatalog は§20のstrict parserを通す。
func checkMessageCatalog(data []byte) *domain.Error {
	_, err := ParseMessageCatalog(data)
	return err
}

// checkSizes は§5第2項のsize上限を検査する。
func checkSizes(source Source, report *SourceReport) {
	limits := map[string]int{
		ManifestPath:       ManifestFileMaxBytes,
		MessageCatalogPath: MessageFileMaxBytes,
		PythonLicensePath:  LicenseFileMaxBytes,
	}
	for path, data := range source {
		limit, ok := limits[path]
		if !ok {
			switch {
			case strings.HasPrefix(path, "tools/"):
				limit = definition.FileMaxBytes
			case strings.HasPrefix(path, "schemas/"):
				// schema JSONは§5が名指ししていないが、release archiveへ入る
				// registry treeのfileである。manifestと同じ上限を適用する。
				limit = ManifestFileMaxBytes
			default:
				continue
			}
		}
		if len(data) > limit {
			report.add(2, path, "%d byteが上限%d byteを超える", len(data), limit)
		}
	}
}

// checkManifest は§5第3項のうちmanifest側を検査する。
func checkManifest(source Source, report *SourceReport) (Manifest, bool) {
	data, exists := source[ManifestPath]
	if !exists {
		report.add(3, ManifestPath, "manifestが無い")
		return Manifest{}, false
	}
	manifest, err := ParseManifest(data)
	if err != nil {
		report.add(3, ManifestPath, "%s", err.Cause)
		return Manifest{}, false
	}
	return manifest, true
}

// checkDefinitions はdefinitionを要する§5の項目を検査する。
func checkDefinitions(source Source, manifest Manifest, report *SourceReport) {
	parsed := make([]*definition.Definition, 0, len(manifest.Tools))
	for _, entry := range manifest.Tools {
		data, exists := source[entry.Path]
		if !exists {
			report.add(3, entry.Path, "manifestが指すdefinitionが無い")
			continue
		}
		// §5-3: digestはraw file bytesと一致する。
		if err := VerifyDefinitionDigest(entry, data); err != nil {
			report.add(3, entry.Path, "%s", err.Cause)
		}
		// definition.Parseがschema、schema_id、ID/basename一致、§13-1〜10の
		// 全検査を行う。§5-3の「schemaが一致」と§5-8の両platform version集合
		// 一致はここで担保される。
		value, parseErr := definition.Parse(entry.Path, data)
		if parseErr != nil {
			report.add(3, entry.Path, "%s", parseErr.Cause)
			continue
		}
		if value.Tool.ID.String() != entry.ID.String() {
			report.add(3, entry.Path, "定義のtool ID %q がmanifestの %q と一致しない",
				value.Tool.ID, entry.ID)
		}
		parsed = append(parsed, value)
	}

	checkAliasCollisions(parsed, report)
	for _, value := range parsed {
		checkPlatformTuple(value, report)
		checkToolContract(value, report)
	}
}

// checkAliasCollisions は§5第4項「aliasが4 tool全体で衝突しない」を検査する。
//
// tool IDとaliasは同じ名前空間である。`use go`のような入力を解決するとき、
// あるtoolのaliasが別のtoolのIDと同じだと、どちらを指すかが決まらない。
// definition単体の検査では他のdefinitionを見られないため、registry全体を
// 見るここで行う（docs/06-tool-definition.md §13-11）。
func checkAliasCollisions(definitions []*definition.Definition, report *SourceReport) {
	// name → 宣言元の説明。宣言順で最初のものを残す。
	owner := make(map[string]string)
	claim := func(name, origin, path string) {
		if previous, taken := owner[name]; taken {
			report.add(4, path, "名前 %q が %s と %s で衝突している", name, previous, origin)
			return
		}
		owner[name] = origin
	}
	for _, value := range definitions {
		id := value.Tool.ID.String()
		claim(id, fmt.Sprintf("%s のtool ID", id), value.Path)
	}
	for _, value := range definitions {
		id := value.Tool.ID.String()
		for _, alias := range value.Tool.Aliases {
			claim(alias, fmt.Sprintf("%s のalias", id), value.Path)
		}
	}
}

// checkPlatformTuple は§5第5項「platform tupleが`windows-amd64|
// linux-amd64-glibc`だけ」を検査する。
func checkPlatformTuple(value *definition.Definition, report *SourceReport) {
	want := []string{windowsPlatform, linuxPlatform}
	got := make([]string, 0, len(value.Platforms))
	for _, platform := range value.Platforms {
		got = append(got, platform.Platform.ID())
	}
	if len(got) != len(want) {
		report.add(5, value.Path, "platformが%v、want %v", got, want)
		return
	}
	sorted := append([]string(nil), got...)
	sort.Strings(sorted)
	wantSorted := append([]string(nil), want...)
	sort.Strings(wantSorted)
	for index := range wantSorted {
		if sorted[index] != wantSorted[index] {
			report.add(5, value.Path, "platformが%v、want %v", got, want)
			return
		}
	}
}

// checkToolContract は§5第6項・第9項・第10項を検査する。
//
// 第6項「required command、typed storage、provider、checksum、channel/lifecycle
// とその根拠が§7〜§10の表と一致」、第9項の`license_notice`とOSI承認識別子の
// 対応、第10項の`lifecycle_map`網羅である。
func checkToolContract(value *definition.Definition, report *SourceReport) {
	id := value.Tool.ID.String()
	contract, known := standardToolByID(id)
	if !known {
		// §6は「その他のtool ID、unsupported placeholder、helper definitionを
		// registryへ入れない」と定める。
		report.add(6, value.Path, "tool ID %q は§6の標準4 toolに無い", id)
		return
	}

	if value.Tool.License != contract.toolLicense {
		report.add(6, value.Path, "tool license = %q, want %q",
			value.Tool.License, contract.toolLicense)
	}
	if value.Tool.Homepage != contract.homepage {
		report.add(6, value.Path, "homepage = %q, want %q",
			value.Tool.Homepage, contract.homepage)
	}
	if value.Tool.VersionScheme != contract.versionScheme {
		report.add(6, value.Path, "version_scheme = %q, want %q",
			value.Tool.VersionScheme, contract.versionScheme)
	}

	for _, platform := range value.Platforms {
		checkPlatformContract(value.Path, platform, contract, report)
	}
}

func checkPlatformContract(
	path string, platform definition.Platform, contract standardTool, report *SourceReport,
) {
	id := platform.Platform.ID()
	label := func(format string, args ...any) string {
		return id + ": " + fmt.Sprintf(format, args...)
	}

	if platform.ArtifactKind != contract.providerKind {
		report.add(6, path, "%s", label("artifact_kind = %q, want %q",
			platform.ArtifactKind, contract.providerKind))
	}
	if platform.VersionSource.Kind != contract.sourceKind {
		report.add(6, path, "%s", label("version_source.kind = %q, want %q",
			platform.VersionSource.Kind, contract.sourceKind))
	}
	if platform.Artifact.Source != contract.artifactSource {
		report.add(6, path, "%s", label("artifact.source = %q, want %q",
			platform.Artifact.Source, contract.artifactSource))
	}
	if platform.Artifact.Checksum.Kind != contract.checksumKind {
		report.add(6, path, "%s", label("checksum.kind = %q, want %q",
			platform.Artifact.Checksum.Kind, contract.checksumKind))
	}
	// providerが公開したalgorithmがdefinitionのどこに現れるかはsourceの形で
	// 変わる（§6・[standardTool]のcheckusmAlgorithm参照）。宣言する場所ごとに
	// 突き合わせ、宣言しない場所へ既定値を補わない。
	if platform.Artifact.Checksum.Algorithm != contract.checksumAlgorithm {
		report.add(6, path, "%s", label("checksum.algorithm = %q, want %q",
			platform.Artifact.Checksum.Algorithm, contract.checksumAlgorithm))
	}
	if platform.Artifact.Checksum.LineFormat != contract.checksumLineFormat {
		report.add(6, path, "%s", label("checksum.line_format = %q, want %q",
			platform.Artifact.Checksum.LineFormat, contract.checksumLineFormat))
	}
	checkStaticAssetAlgorithm(path, id, platform, contract, report)
	if platform.Install.StripComponents != contract.stripComponents {
		report.add(6, path, "%s", label("strip_components = %d, want %d",
			platform.Install.StripComponents, contract.stripComponents))
	}

	checkCommands(path, id, platform, contract, report)
	checkStorage(path, id, platform, contract, report)
	checkLicense(path, id, platform, contract, report)
	checkLifecycleMap(path, id, platform, contract, report)
}

// checkStaticAssetAlgorithm は`static` sourceのassetが持つdigest algorithmを
// 検査する。
//
// §6は「providerが公開したalgorithム（`sha256`または`sha512`）をそのまま使う」と
// 定める。Pythonは`static_versions`のassetごとに`digest_algorithm`を持つため、
// artifact側の`checksum.algorithm`ではなくここを見る（§9.2）。
func checkStaticAssetAlgorithm(
	path, id string, platform definition.Platform, contract standardTool, report *SourceReport,
) {
	if contract.staticAssetAlgorithm == "" {
		return
	}
	for _, version := range platform.VersionSource.StaticVersions {
		for _, asset := range version.Assets {
			if asset.DigestAlgorithm != contract.staticAssetAlgorithm {
				report.add(6, path, "%s: static asset %q のdigest_algorithm = %q, want %q",
					id, asset.Name, asset.DigestAlgorithm, contract.staticAssetAlgorithm)
			}
		}
	}
}

// checkCommands は§7.2・§8.2・§9.3・§10.3のrequired command完全集合を検査する。
//
// 「required command集合は本章とdefinition/fixture/contract testで完全一致
// させる」（§6）。過不足の両方を見る。
func checkCommands(
	path, id string, platform definition.Platform, contract standardTool, report *SourceReport,
) {
	got := make([]string, 0, len(platform.Runtime.Commands))
	for _, command := range platform.Runtime.Commands {
		if command.Required {
			got = append(got, command.Name)
		}
	}
	if !sameStrings(got, contract.commands) {
		report.add(6, path, "%s: required command = %v, want %v", id, got, contract.commands)
	}
}

// checkStorage は§7.3・§8.3・§9.4・§10.4のtyped storageを検査する。
func checkStorage(
	path, id string, platform definition.Platform, contract standardTool, report *SourceReport,
) {
	if len(platform.Storage) != len(contract.storage) {
		got := make([]string, 0, len(platform.Storage))
		for _, storage := range platform.Storage {
			got = append(got, storage.ID)
		}
		want := make([]string, 0, len(contract.storage))
		for _, storage := range contract.storage {
			want = append(want, storage.id)
		}
		report.add(6, path, "%s: storage = %v, want %v", id, got, want)
		return
	}
	byID := make(map[string]definition.Storage, len(platform.Storage))
	for _, storage := range platform.Storage {
		byID[storage.ID] = storage
	}
	for _, want := range contract.storage {
		got, exists := byID[want.id]
		if !exists {
			report.add(6, path, "%s: storage %q が無い", id, want.id)
			continue
		}
		if got.Kind != want.kind {
			report.add(6, path, "%s: storage %q のkind = %q, want %q",
				id, want.id, got.Kind, want.kind)
		}
		if got.Scope != want.scope {
			report.add(6, path, "%s: storage %q のscope = %q, want %q",
				id, want.id, got.Scope, want.scope)
		}
	}
}

// checkLicense は§5第9項を検査する。
//
// 「`license_notice`を宣言したplatformの`provider.license`がOSI承認OSS license
// 識別子でなく、宣言しないplatformの`provider.license`がOSI承認OSS license
// 識別子である」。`official`区分であることが「利用条件が緩い」ことを意味しない
// ため、区分表示だけに委ねない（docs/10-security.md §9）。
func checkLicense(
	path, id string, platform definition.Platform, contract standardTool, report *SourceReport,
) {
	want, known := contract.platforms[id]
	if !known {
		// platform tupleの検査（§5-5）が別途報告する。
		return
	}
	license := platform.Provider.License
	if license != want.providerLicense {
		report.add(6, path, "%s: provider.license = %q, want %q",
			id, license, want.providerLicense)
	}

	declared := !platform.LicenseNotice.IsZero()
	if declared != want.licenseNotice {
		report.add(9, path, "%s: license_notice宣言 = %t, want %t",
			id, declared, want.licenseNotice)
	}

	approved, listed := osiApprovedLicenses[license]
	if !listed {
		// 未知の識別子を承認/非承認のどちらとも扱わない。
		report.add(9, path, "%s: provider.license %q がOSI承認判定表に無い。"+
			"tool追加時はdocs/14-maintenance.mdの手順で表を更新する", id, license)
		return
	}
	switch {
	case declared && approved:
		report.add(9, path, "%s: license_notice を宣言しているが provider.license %q は"+
			"OSI承認OSS licenseである", id, license)
	case !declared && !approved:
		report.add(9, path, "%s: provider.license %q はOSI承認OSS licenseでないため"+
			"license_notice の宣言が必要である", id, license)
	}
}

// checkLifecycleMap は§5第10項「`lifecycle_map`がupstream enumの全値を明示」を
// 検査する。
//
// 「mapに無い値はsource errorとし、黙って`unknown`へ倒さない」（§10.1）。
// 過不足の両方を見る。余分な値を許すと、upstreamが廃止したphaseの写像が残って
// いることに気付けない。
func checkLifecycleMap(
	path, id string, platform definition.Platform, contract standardTool, report *SourceReport,
) {
	got := platform.VersionSource.LifecycleMap
	if contract.lifecycleMap == nil {
		// 宣言しないtoolがmapを持つ場合はdefinition側が先に拒否する。
		// `lifecycle_map`は`kind=json-index`でだけ書けるkeyであり、標準4 toolで
		// json-indexなのは.NET SDKだけである（§10.1）。ここで重ねて検査すると
		// 到達しない分岐になる。
		return
	}
	if len(got) == 0 {
		report.add(10, path, "%s: lifecycle_mapが無い。upstream enumの全値を明示する", id)
		return
	}
	var missing, extra []string
	for key := range contract.lifecycleMap {
		if _, exists := got[key]; !exists {
			missing = append(missing, key)
		}
	}
	for key, value := range got {
		want, exists := contract.lifecycleMap[key]
		if !exists {
			extra = append(extra, key)
			continue
		}
		if value != want {
			report.add(10, path, "%s: lifecycle_map[%q] = %q, want %q", id, key, value, want)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)
	if len(missing) > 0 {
		report.add(10, path, "%s: lifecycle_mapにupstream enumの %v が無い", id, missing)
	}
	if len(extra) > 0 {
		report.add(10, path, "%s: lifecycle_mapにupstream enumに無い %v がある", id, extra)
	}
}

// sameStrings は順序込みで一致するかを返す。
func sameStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for index := range want {
		if got[index] != want[index] {
			return false
		}
	}
	return true
}
