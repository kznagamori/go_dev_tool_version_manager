package registry

import (
	"github.com/kznagamori/go_dev_tool_version_manager/internal/definition"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
)

// windowsPlatform とlinuxPlatform はdocs/07-registry-and-tools.md §6が定める
// platform tupleである。§5第5項が「platform tupleが`windows-amd64|
// linux-amd64-glibc`だけ」と定める。
const (
	windowsPlatform = "windows-amd64"
	linuxPlatform   = "linux-amd64-glibc"
)

// osiApprovedLicenses はregistryが宣言してよいOSI承認OSS license識別子である。
//
// §5第9項は「`license_notice`を宣言したplatformの`provider.license`がOSI承認OSS
// license識別子でなく、宣言しないplatformの`provider.license`がOSI承認OSS license
// 識別子である」と定めるが、OSI承認listそのものは仕様に無い。
//
// §6がregistryを標準4 toolへ閉じているため、判定に必要な識別子は有限である。
// 未知の識別子は承認/非承認のどちらとも扱わずerrorにする（fail closed）。SPDXの
// 全listを持ち込むと、承認状態の更新をregistryと無関係に追う必要が出る。tool追加
// 時は[14-maintenance.md](14-maintenance.md)の手順でこの表を更新する。
var osiApprovedLicenses = map[string]bool{
	"BSD-3-Clause": true,
	"MIT":          true,
	"MPL-2.0":      true,
	// .NET SDKのWindows配布物。Microsoft独自EULAでSPDX識別子を持たないため、
	// SPDXのLicenseRef形式で書く（§10.2）。OSI承認ではない。
	"LicenseRef-dotnet-library": false,
}

// standardStorage は§7.3・§8.3・§9.4・§10.4のtyped storage 1件である。
type standardStorage struct {
	id    string
	kind  definition.StorageKind
	scope definition.StorageScope
}

// standardPlatform は§7〜§10がplatformごとに定める契約である。
type standardPlatform struct {
	// providerLicense は配布物のSPDX識別子である。
	providerLicense string
	// licenseNotice は`license_notice`を宣言するかである。
	licenseNotice bool
}

// standardTool は§7〜§10がtoolごとに定める契約である。
//
// 表で持つのは、§5第6項が「required command、typed storage、provider、checksum、
// channel/lifecycleとその根拠が§7〜§10の表と一致」を要求するためである。判定を
// codeへ散らすと、仕様表との対応が読めなくなる。
type standardTool struct {
	id            string
	toolLicense   string
	homepage      string
	versionScheme domain.VersionScheme
	providerKind  definition.ArtifactKind
	// platforms はplatform IDごとの契約である。2件だけを持つ。
	platforms map[string]standardPlatform
	// sourceKind は§6のversion source種別である。
	sourceKind definition.VersionSourceKind
	// artifactSource は§7.1のartifact URL/file決定方式である。
	artifactSource definition.ArtifactSource
	// checksumKind は§7.1・§8.1・§9.1・§10.1のprovider checksum種別である。
	checksumKind definition.ChecksumKind
	// checksumAlgorithm は`[platforms.artifact.checksum]`が宣言するalgorithmで
	// ある。**宣言しないtoolでは空になる。**
	//
	// §6は「algorithmはproviderが公開したもの（`sha256`または`sha512`）をそのまま
	// 使う」と定めるが、その値がdefinitionのどこに現れるかはsourceの形で変わる。
	// Goと.NETはasset側にalgorithm fieldが無いため`checksum.algorithm`で宣言し、
	// Pythonはstatic assetごとの`digest_algorithm`、Node.jsは`line_format`が
	// 決める。宣言の無い場所へ既定値を補うと、providerが公開したalgorithmと
	// definitionの宣言が一致しているかを検査できなくなる。
	checksumAlgorithm definition.DigestAlgorithm
	// checksumLineFormat は`text-file`の行形式である。他のkindでは空。
	checksumLineFormat string
	// staticAssetAlgorithm は`static` sourceのassetが持つdigest algorithmである。
	// static sourceでないtoolでは空。
	staticAssetAlgorithm definition.DigestAlgorithm
	// stripComponents は§9の展開parameterである。
	stripComponents int
	// commands は§7.2・§8.2・§9.3・§10.3のrequired command完全集合である。
	//
	// 両platformで同じ集合になる。shim名がOSで変わると利用者のscriptが
	// platform間で動かなくなる。
	commands []string
	// storage は§7.3・§8.3・§9.4・§10.4のtyped storage完全集合である。
	storage []standardStorage
	// lifecycleMap は§10.1の`support-phase`写像である。宣言しないtoolではnil。
	//
	// §5第10項が「`lifecycle_map`がupstream enumの全値を明示」を要求するため、
	// upstream側の値集合をここで固定する。
	lifecycleMap map[string]definition.Lifecycle
}

// standardTools は§6が定める標準4 toolの契約である。
//
// 「その他のtool ID、unsupported placeholder、helper definitionをregistryへ
// 入れない」（§6）。件数と並びはmanifestのID ASCII byte順に合わせる。
var standardTools = []standardTool{
	{
		// docs/07-registry-and-tools.md §10。
		id:            "dotnet",
		toolLicense:   "MIT",
		homepage:      "https://dotnet.microsoft.com/",
		versionScheme: domain.SchemeSemver,
		providerKind:  definition.KindOfficial,
		platforms: map[string]standardPlatform{
			// Windows配布archiveはMITではなくMicrosoft独自EULAである（§10.2）。
			windowsPlatform: {providerLicense: "LicenseRef-dotnet-library", licenseNotice: true},
			linuxPlatform:   {providerLicense: "MIT", licenseNotice: false},
		},
		sourceKind:        definition.SourceJSONIndex,
		artifactSource:    definition.SourceAsset,
		checksumKind:      definition.ChecksumAssetField,
		checksumAlgorithm: definition.AlgorithmSHA512,
		// archiveにtop-level directoryが無い（§10.1）。
		stripComponents: 0,
		commands:        []string{"dotnet"},
		storage: []standardStorage{
			{"cli-home", definition.StorageRuntimeData, definition.ScopeVersion},
			{"nuget-packages", definition.StorageContentCache, definition.ScopeTool},
			{"nuget-http-cache", definition.StorageContentCache, definition.ScopeTool},
			{"nuget-plugins-cache", definition.StorageContentCache, definition.ScopeTool},
		},
		lifecycleMap: map[string]definition.Lifecycle{
			"preview":     definition.LifecycleSupported,
			"go-live":     definition.LifecycleSupported,
			"active":      definition.LifecycleSupported,
			"maintenance": definition.LifecycleSupported,
			"eol":         definition.LifecycleEOL,
		},
	},
	{
		// 同§7。
		id:            "go",
		toolLicense:   "BSD-3-Clause",
		homepage:      "https://go.dev/",
		versionScheme: domain.SchemeGo,
		providerKind:  definition.KindOfficial,
		platforms: map[string]standardPlatform{
			windowsPlatform: {providerLicense: "BSD-3-Clause", licenseNotice: false},
			linuxPlatform:   {providerLicense: "BSD-3-Clause", licenseNotice: false},
		},
		sourceKind:        definition.SourceJSON,
		artifactSource:    definition.SourceAsset,
		checksumKind:      definition.ChecksumAssetField,
		checksumAlgorithm: definition.AlgorithmSHA256,
		// top-level `go/`を1 component除去する（§7.1）。
		stripComponents: 1,
		commands:        []string{"go", "gofmt"},
		storage: []standardStorage{
			{"config", definition.StorageConfig, definition.ScopeTool},
			{"workspace", definition.StorageRuntimeData, definition.ScopeTool},
			{"module-cache", definition.StorageContentCache, definition.ScopeTool},
			{"build-cache", definition.StorageBuildCache, definition.ScopeTool},
			{"global-tools", definition.StorageGlobalBin, definition.ScopeTool},
		},
	},
	{
		// 同§8。
		id:            "node",
		toolLicense:   "MIT",
		homepage:      "https://nodejs.org/",
		versionScheme: domain.SchemeSemver,
		providerKind:  definition.KindOfficial,
		platforms: map[string]standardPlatform{
			windowsPlatform: {providerLicense: "MIT", licenseNotice: false},
			linuxPlatform:   {providerLicense: "MIT", licenseNotice: false},
		},
		sourceKind: definition.SourceJSON,
		// URL templateは作れるが、checksumは別fileの`SHASUMS256.txt`から引く。
		artifactSource: definition.SourceTemplate,
		checksumKind:   definition.ChecksumTextFile,
		// `SHASUMS256.txt`の行形式がalgorithmを決めるため宣言しない（§8.1）。
		checksumLineFormat: "sha256-space-filename",
		stripComponents:    1,
		commands:           []string{"node", "npm", "npx"},
		storage: []standardStorage{
			{"config", definition.StorageConfig, definition.ScopeTool},
			{"cache", definition.StorageContentCache, definition.ScopeTool},
			{"history", definition.StorageRuntimeData, definition.ScopeTool},
			// native addonがNode/ABI差を持ちうるためversion scope（§8.3）。
			{"global-packages", definition.StorageGlobalPackages, definition.ScopeVersion},
		},
	},
	{
		// 同§9。公式CPythonが両OSで同一の非root portable archive契約を提供
		// しないため、採用理由を明記したthird-party artifactを使う（§6・§9.1）。
		id:            "python",
		toolLicense:   "PSF-2.0",
		homepage:      "https://www.python.org/",
		versionScheme: domain.SchemePython,
		providerKind:  definition.KindThirdParty,
		platforms: map[string]standardPlatform{
			windowsPlatform: {providerLicense: "MPL-2.0", licenseNotice: false},
			linuxPlatform:   {providerLicense: "MPL-2.0", licenseNotice: false},
		},
		// live releaseから毎回artifactを選ばず`static_versions`へ固定する（§9.2）。
		sourceKind:     definition.SourceStatic,
		artifactSource: definition.SourceAsset,
		checksumKind:   definition.ChecksumAssetField,
		// static assetごとの`digest_algorithm`が持つため宣言しない（§9.2）。
		staticAssetAlgorithm: definition.AlgorithmSHA256,
		// top-level `python/`を1 component除去する（§9.1）。
		stripComponents: 1,
		commands:        []string{"python", "python3", "pip", "pip3"},
		storage: []standardStorage{
			{"config", definition.StorageConfig, definition.ScopeTool},
			{"cache", definition.StorageContentCache, definition.ScopeTool},
			{"history", definition.StorageRuntimeData, definition.ScopeTool},
			// Python X.Y/ABI差があるためversion scope（§9.4）。
			{"user-packages", definition.StorageGlobalPackages, definition.ScopeVersion},
		},
	},
}

// standardToolByID はtool IDから契約を引く。
func standardToolByID(id string) (standardTool, bool) {
	for _, tool := range standardTools {
		if tool.id == id {
			return tool, true
		}
	}
	return standardTool{}, false
}
