package store

import (
	"fmt"
	"strings"
	"time"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
)

// ReceiptFileName はinstall receiptのfile名である（docs/04-storage-and-data.md §14）。
const ReceiptFileName = ".gdtvm-install.toml"

// PayloadDirectoryName はreceipt directory相対のpayload pathである（§14）。
//
// 同§が「payload path=`payload`固定」と定める。
const PayloadDirectoryName = "payload"

// ProbeTimeoutMinMillis とProbeTimeoutMaxMillis はprobe timeoutの範囲である。
//
// §14が「timeoutは1～120,000 ms」と定める。docs/06-tool-definition.md §11の
// 「1s～2m」と同じ範囲をmillisecondで表したものである。
const (
	ProbeTimeoutMinMillis = 1
	ProbeTimeoutMaxMillis = 120_000
)

// ProviderKind はartifact providerの種別である（§17.1）。
type ProviderKind string

// ProviderKind の値。`none`は対象toolがないPlan operationだけが使う（§17.1）。
const (
	ProviderOfficial   ProviderKind = "official"
	ProviderThirdParty ProviderKind = "third-party"
)

// receiptProviderKinds はreceiptとcatalogが許すprovider kindである。
//
// §17.1が「`official|third-party`。対象toolがないPlan operationだけ`none`」と
// 定める。receiptとcatalogは必ず対象toolを持つため`none`を含めない。
var receiptProviderKinds = map[ProviderKind]struct{}{
	ProviderOfficial: {}, ProviderThirdParty: {},
}

// ChecksumSource はdigestの取得元である（§17.1）。
type ChecksumSource string

// ChecksumSource のexactly 2値。docs/06-tool-definition.md §7.2のkindと一致する。
const (
	ChecksumAssetField ChecksumSource = "asset-field"
	ChecksumTextFile   ChecksumSource = "text-file"
)

var checksumSources = map[ChecksumSource]struct{}{
	ChecksumAssetField: {}, ChecksumTextFile: {},
}

// StorageKind はtyped storageの種別である（docs/06-tool-definition.md §8）。
type StorageKind string

// StorageKind のexactly 6値。
const (
	StorageConfig         StorageKind = "config"
	StorageContentCache   StorageKind = "content-cache"
	StorageBuildCache     StorageKind = "build-cache"
	StorageGlobalBin      StorageKind = "global-bin"
	StorageGlobalPackages StorageKind = "global-packages"
	StorageRuntimeData    StorageKind = "runtime-data"
)

var storageKinds = map[StorageKind]struct{}{
	StorageConfig: {}, StorageContentCache: {}, StorageBuildCache: {},
	StorageGlobalBin: {}, StorageGlobalPackages: {}, StorageRuntimeData: {},
}

// StorageScope はstorageの寿命の単位である（§17.1）。
type StorageScope string

// StorageScope のexactly 2値。
const (
	ScopeTool    StorageScope = "tool"
	ScopeVersion StorageScope = "version"
)

var storageScopes = map[StorageScope]struct{}{ScopeTool: {}, ScopeVersion: {}}

// StoragePurge はuninstall時の扱いである（§17.1）。
type StoragePurge string

// StoragePurge のexactly 3値。
const (
	PurgeRetain      StoragePurge = "retain"
	PurgeExplicit    StoragePurge = "explicit"
	PurgeWithVersion StoragePurge = "with-version"
)

var storagePurges = map[StoragePurge]struct{}{
	PurgeRetain: {}, PurgeExplicit: {}, PurgeWithVersion: {},
}

// WorkingDirectory はcommand実行時のcurrent directoryである（§17.1）。
type WorkingDirectory string

// WorkingDirectory のexactly 2値。
const (
	WorkingInherit WorkingDirectory = "inherit"
	WorkingPayload WorkingDirectory = "payload"
)

var workingDirectories = map[WorkingDirectory]struct{}{
	WorkingInherit: {}, WorkingPayload: {},
}

// ProbeStream はprobeが読む出力streamである（§17.1）。
type ProbeStream string

// ProbeStream のexactly 3値。
const (
	StreamStdout   ProbeStream = "stdout"
	StreamStderr   ProbeStream = "stderr"
	StreamCombined ProbeStream = "combined"
)

var probeStreams = map[ProbeStream]struct{}{
	StreamStdout: {}, StreamStderr: {}, StreamCombined: {},
}

// ProbeExpect はprobeの期待条件である（§17.1）。
type ProbeExpect string

// ProbeExpect のexactly 3値。
const (
	ExpectVersion    ProbeExpect = "version"
	ExpectSuccess    ProbeExpect = "success"
	ExpectPathWithin ProbeExpect = "path-within"
)

var probeExpects = map[ProbeExpect]struct{}{
	ExpectVersion: {}, ExpectSuccess: {}, ExpectPathWithin: {},
}

// ProbeStatus はprobeの実行結果である（§17.1）。
type ProbeStatus string

// ProbeStatus のexactly 2値。receiptだけが持つ。
const (
	ProbePassed  ProbeStatus = "passed"
	ProbeSkipped ProbeStatus = "skipped"
)

var probeStatuses = map[ProbeStatus]struct{}{ProbePassed: {}, ProbeSkipped: {}}

// requiredPathPrefixes はrequired path entryの種別prefixである。
//
// docs/06-tool-definition.md §11が「entryは`file:<template>|directory:<template>`の
// 文字列として記述し、unknown prefixを拒否する」と定める。
var requiredPathPrefixes = map[string]struct{}{"file": {}, "directory": {}}

// ReceiptArtifact は導入元artifactの記録である（§14）。
//
// v0.1は全artifactでupstream checksumとの一致を必須とするため、検証状態fieldを
// 持たない。receiptに存在すること自体が検証済みを意味する。
type ReceiptArtifact struct {
	// ProviderKind はofficialかthird-partyかである。
	ProviderKind ProviderKind
	// ProviderName は配布元の名称である。
	ProviderName string
	// ProviderRelease は配布元のrelease識別子である。
	ProviderRelease string
	// URL はdownload元のHTTPS URLである。
	URL string
	// File はartifactのfile名である。
	File string
	// Size はartifactのbyte数である。
	Size int64
	// Digest はproviderが公開したupstream digestである。
	Digest domain.Digest
	// ChecksumSource はdigestの取得元である。
	ChecksumSource ChecksumSource
	// ThirdPartyApproved はthird-party providerの明示承認である。
	ThirdPartyApproved bool
	// LicenseNoticeApproved はlicense noticeの明示承認である。
	LicenseNoticeApproved bool
}

// ReceiptStorage はtyped storageの記録である（§14）。
//
// receiptにabsolute rootを書かない。runtimeがreceiptの現在位置とscopeから
// 絶対化する。
type ReceiptStorage struct {
	// ID はplatform内で一意なstorage IDである。
	ID string
	// Kind はstorageの種別である。
	Kind StorageKind
	// Scope はtoolかversionかである。
	Scope StorageScope
	// Path はstorage root内のPOSIX relative pathである。
	Path string
	// Purge はuninstall時の扱いである。
	Purge StoragePurge
}

// ReceiptCommand は公開runtime commandの記録である（§14）。
type ReceiptCommand struct {
	// Name はPATHへ出るcommand名である。
	Name string
	// Target は実体を指すtemplateである。
	Target string
	// FixedArgs はcommandへ常に渡すargvである。
	FixedArgs []string
	// EnvironmentProfile は適用するprofile IDである。
	EnvironmentProfile string
	// WorkingDirectory はcurrent directoryの決め方である。
	WorkingDirectory WorkingDirectory
	// PassthroughSignals はsignalを子processへ透過するかである。
	PassthroughSignals bool
}

// ReceiptEnvironmentProfile は実行環境の記録である（§14）。
type ReceiptEnvironmentProfile struct {
	// ID はreceipt内で一意なprofile IDである。
	ID string
	// PathPrepend はPATHの先頭へ足すentryである。
	PathPrepend []string
	// PathAppend はPATHの末尾へ足すentryである。
	PathAppend []string
	// Set は設定する環境変数である。
	Set map[string]string
	// Unset は削除する環境変数名である。
	Unset []string
	// OverrideAllowed は利用者の既存値を優先してよい変数名である。
	OverrideAllowed []string
	// ShellExport はshellへexportする変数名である。
	ShellExport []string
}

// ReceiptProbe は実行したvalidation probeの記録である（§14）。
type ReceiptProbe struct {
	// ID はreceipt内で一意なprobe IDである。
	ID string
	// RuntimeCommand は実行したcommand名である。
	RuntimeCommand string
	// Args はcommandへ渡したargvである。
	Args []string
	// Stream は読んだ出力streamである。
	Stream ProbeStream
	// Expect は期待条件である。
	Expect ProbeExpect
	// Regex は出力照合のRE2 patternである。非該当なら空。
	Regex string
	// ExpectedVersion は期待した完全versionである。非該当なら空。
	ExpectedVersion string
	// ExpectedRoot はpath-withinで期待したroot templateである。非該当なら空。
	ExpectedRoot string
	// RequiredPaths は`file:`/`directory:`prefix付きのtemplate列である。
	RequiredPaths []string
	// TimeoutMillis は打ち切りまでのmillisecondである。
	TimeoutMillis int64
	// Required は失敗をinstall全体の失敗にするかである。
	Required bool
	// Status は実行結果である。
	Status ProbeStatus
	// ReportedVersion はprobeが報告したversionである。非対象なら空。
	ReportedVersion string
	// FinishedAt はprobeが終わった時刻である。
	FinishedAt time.Time
}

// ReceiptCommandTarget はpayload内実体の完全性記録である（§14）。
//
// required runtime commandのtargetとfixed argsが参照するpayload内fileだけを持ち、
// payload全fileのmanifestは作らない。`doctor`はここを照合してpayload破損を
// 検出する。
type ReceiptCommandTarget struct {
	// Path はpayload相対pathである。
	Path string
	// Size はfileのbyte数である。
	Size int64
	// SHA256 はgdtvm自身が計算した64 lowercase hexである。
	SHA256 string
}

// Receipt は`.gdtvm-install.toml`のtyped表現である（§14）。
//
// 既存receiptはactive registry definitionで再解釈しない。runtimeはreceiptの
// command/profile/storageを正とし、registryはschema互換だけを追加判断する。
// versionをtextで持つ理由は[InstallRef]のとおりである。
type Receipt struct {
	// InstallID はこの導入を一意に識別する128 bit hexである。
	InstallID string
	// RootID は導入先rootの128 bit hexである。
	RootID string
	// Ref はtool、version、platformの組である。
	Ref InstallRef
	// InstalledAt は導入完了時刻である。
	InstalledAt time.Time
	// ClientVersion は導入したclientの完全versionまたは`devel`である。
	ClientVersion string
	// ClientCommit は導入したclientのcommit IDである。
	ClientCommit string
	// DefinitionPath はregistry相対のdefinition pathである。
	DefinitionPath string
	// DefinitionSHA256 は導入時のdefinition内容のdigestである。
	DefinitionSHA256 string
	// PayloadPath はreceipt directory相対のpayload pathである。`payload`固定。
	PayloadPath string
	// Artifact は導入元artifactの記録である。
	Artifact ReceiptArtifact
	// Storage はtyped storageである。唯一空を許すarrayである。
	Storage []ReceiptStorage
	// Commands は公開runtime commandである。
	Commands []ReceiptCommand
	// EnvironmentProfiles は実行環境である。
	EnvironmentProfiles []ReceiptEnvironmentProfile
	// Probes は実行したvalidation probeである。
	Probes []ReceiptProbe
	// CommandTargets はpayload内実体の完全性記録である。
	CommandTargets []ReceiptCommandTarget
}

// clientCommitRe は40桁lowercase hexのcommit IDである。
var clientCommitRe = regexpMustCompile(`^[0-9a-f]{40}$`)

// ParseReceipt は`.gdtvm-install.toml`を読む（§14）。
func ParseReceipt(data []byte) (Receipt, *domain.Error) {
	var file receiptFile
	if err := requireSize("receipt TOML", data, ReceiptFileMaxBytes); err != nil {
		return Receipt{}, receiptError(err)
	}
	if err := decodeTOML(data, &file); err != nil {
		return Receipt{}, receiptError(err)
	}
	value, err := buildReceipt(file)
	if err != nil {
		return Receipt{}, receiptError(err)
	}
	return value, nil
}

// EncodeReceipt はinstall receiptを書き出す（§14）。
func EncodeReceipt(value Receipt) ([]byte, *domain.Error) {
	file := receiptFileOf(value)
	if _, err := buildReceipt(file); err != nil {
		return nil, receiptError(err)
	}
	data, err := encodeTOML(file)
	if err != nil {
		return nil, receiptError(err)
	}
	if err := requireSize("receipt TOML", data, ReceiptFileMaxBytes); err != nil {
		return nil, receiptError(err)
	}
	return data, nil
}

// receiptError はreceiptの破損を表すtyped errorを作る。
//
// docs/03-cli.md §7の終了code表がreceiptの不整合を`E_RECEIPT_INVALID`とする。
// state fileの`E_STATE_CORRUPT`と分けるのは、どちらを直せばよいかが
// 利用者にとって異なるためである。
func receiptError(cause error) *domain.Error {
	return typedError(domain.CodeReceiptInvalid, "receipt.invalid", domain.RoleReceipt, cause)
}

func buildReceipt(file receiptFile) (Receipt, error) {
	var value Receipt
	if err := requireSchema("schema", file.Schema); err != nil {
		return value, err
	}
	installID, err := requireIDField("install_id", file.InstallID)
	if err != nil {
		return value, err
	}
	rootID, err := requireIDField("root_id", file.RootID)
	if err != nil {
		return value, err
	}
	ref, err := requireInstallRef("receipt", file.ToolID, file.Version, file.PlatformID)
	if err != nil {
		return value, err
	}
	installedAt, err := requireTimestampField("installed_at", file.InstalledAt)
	if err != nil {
		return value, err
	}
	clientVersion, err := requireClientVersion("client_version", file.ClientVersion)
	if err != nil {
		return value, err
	}
	clientCommit, err := requirePresent("client_commit", file.ClientCommit)
	if err != nil {
		return value, err
	}
	if !clientCommitRe.MatchString(clientCommit) {
		return value, fmt.Errorf("client_commitが40桁lowercase hexでない（%q）", clientCommit)
	}
	definitionPath, err := requirePresent("definition_path", file.DefinitionPath)
	if err != nil {
		return value, err
	}
	if _, err := requireRelativePath("definition_path", definitionPath); err != nil {
		return value, err
	}
	definitionDigest, err := requireDigestField("definition_sha256", file.DefinitionSHA256)
	if err != nil {
		return value, err
	}
	payloadPath, err := requirePresent("payload_path", file.PayloadPath)
	if err != nil {
		return value, err
	}
	if payloadPath != PayloadDirectoryName {
		return value, fmt.Errorf("payload_pathは%qだけを許す（%q）", PayloadDirectoryName, payloadPath)
	}
	if file.Artifact == nil {
		return value, fmt.Errorf("artifactが無い")
	}
	artifact, err := buildReceiptArtifact(*file.Artifact)
	if err != nil {
		return value, err
	}
	storage, storageIDs, err := buildReceiptStorage(file.Storage)
	if err != nil {
		return value, err
	}
	profiles, profileIDs, err := buildReceiptProfiles(file.EnvironmentProfiles, ref, storageIDs)
	if err != nil {
		return value, err
	}
	commands, err := buildReceiptCommands(file.Commands, storageIDs, profileIDs)
	if err != nil {
		return value, err
	}
	probes, err := buildReceiptProbes(file.Probes, storageIDs)
	if err != nil {
		return value, err
	}
	targets, err := buildReceiptTargets(file.CommandTargets)
	if err != nil {
		return value, err
	}
	return Receipt{
		InstallID: installID, RootID: rootID, Ref: ref, InstalledAt: installedAt,
		ClientVersion: clientVersion, ClientCommit: clientCommit,
		DefinitionPath: definitionPath, DefinitionSHA256: definitionDigest,
		PayloadPath: payloadPath, Artifact: artifact, Storage: storage,
		Commands: commands, EnvironmentProfiles: profiles, Probes: probes,
		CommandTargets: targets,
	}, nil
}

func buildReceiptArtifact(table receiptArtifactTable) (ReceiptArtifact, error) {
	var value ReceiptArtifact
	kind, err := requireEnum("artifact.provider_kind", table.ProviderKind, receiptProviderKinds)
	if err != nil {
		return value, err
	}
	name, err := requireNonEmpty("artifact.provider_name", table.ProviderName)
	if err != nil {
		return value, err
	}
	release, err := requireNonEmpty("artifact.provider_release", table.ProviderRelease)
	if err != nil {
		return value, err
	}
	rawURL, err := requirePresent("artifact.url", table.URL)
	if err != nil {
		return value, err
	}
	artifactURL, err := requireHTTPSURL("artifact.url", rawURL)
	if err != nil {
		return value, err
	}
	fileName, err := requireNonEmpty("artifact.file", table.File)
	if err != nil {
		return value, err
	}
	// file名はpath componentとして安全でなければならない。区切りや相対参照が
	// 入ると、download先がcache directoryの外へ出る（§6）。
	if _, err := requireRelativePath("artifact.file", fileName); err != nil {
		return value, err
	}
	if strings.Contains(fileName, "/") {
		return value, fmt.Errorf("artifact.fileはfile名でなければならない（%q）", fileName)
	}
	size, err := requireInt64("artifact.size", table.Size)
	if err != nil {
		return value, err
	}
	digestText, err := requirePresent("artifact.digest", table.Digest)
	if err != nil {
		return value, err
	}
	digest, err := parseUpstreamDigest("artifact.digest", digestText)
	if err != nil {
		return value, err
	}
	source, err := requireEnum("artifact.checksum_source", table.ChecksumSource, checksumSources)
	if err != nil {
		return value, err
	}
	thirdParty, err := requireBool("artifact.third_party_approved", table.ThirdPartyApproved)
	if err != nil {
		return value, err
	}
	licenseNotice, err := requireBool("artifact.license_notice_approved", table.LicenseNoticeApproved)
	if err != nil {
		return value, err
	}
	// §14が「third-partyなら`third_party_approved=true`必須」と定める。承認なしの
	// third-party artifactがreceiptに残ると、承認を経ずに導入された記録になる。
	if kind == ProviderThirdParty && !thirdParty {
		return value, fmt.Errorf("third-party artifactにthird_party_approved=trueが無い")
	}
	// 逆にofficialで承認fieldが立っているのも矛盾である。承認は必要なときだけ
	// 記録し、不要な承認を残さない。
	if kind == ProviderOfficial && thirdParty {
		return value, fmt.Errorf("official artifactにthird_party_approvedが立っている")
	}
	return ReceiptArtifact{
		ProviderKind: kind, ProviderName: name, ProviderRelease: release,
		URL: artifactURL, File: fileName, Size: size, Digest: digest,
		ChecksumSource: source, ThirdPartyApproved: thirdParty,
		LicenseNoticeApproved: licenseNotice,
	}, nil
}

func buildReceiptStorage(
	tables []*receiptStorageTable,
) ([]ReceiptStorage, map[string]struct{}, error) {
	entries := make([]ReceiptStorage, 0, len(tables))
	ids := make(map[string]struct{}, len(tables))
	for index, raw := range tables {
		if raw == nil {
			return nil, nil, fmt.Errorf("storage[%d]が空", index)
		}
		prefix := fmt.Sprintf("storage[%d]", index)
		id, err := requireIdentifier(prefix+".id", raw.ID)
		if err != nil {
			return nil, nil, err
		}
		kind, err := requireEnum(prefix+".kind", raw.Kind, storageKinds)
		if err != nil {
			return nil, nil, err
		}
		scope, err := requireEnum(prefix+".scope", raw.Scope, storageScopes)
		if err != nil {
			return nil, nil, err
		}
		path, err := requirePresent(prefix+".path", raw.Path)
		if err != nil {
			return nil, nil, err
		}
		relative, err := requireRelativePath(prefix+".path", path)
		if err != nil {
			return nil, nil, err
		}
		purge, err := requireEnum(prefix+".purge", raw.Purge, storagePurges)
		if err != nil {
			return nil, nil, err
		}
		if err := checkStorageScopePurge(prefix, scope, purge); err != nil {
			return nil, nil, err
		}
		entries = append(entries, ReceiptStorage{
			ID: id, Kind: kind, Scope: scope, Path: relative, Purge: purge,
		})
		ids[id] = struct{}{}
	}
	if err := requireSortedUnique("storage", len(entries), func(i int) string {
		return entries[i].ID
	}); err != nil {
		return nil, nil, err
	}
	return entries, ids, nil
}

// checkStorageScopePurge はscopeとpurgeの組合せを固定する。
//
// docs/06-tool-definition.md §8が「tool scopeは`retain|explicit`だけ…version
// scopeは`with-version`だけ」と定める。組合せを検査しないと、version scopeの
// storageがuninstall後も残ったり、tool scopeがversionと一緒に消えたりする。
func checkStorageScopePurge(prefix string, scope StorageScope, purge StoragePurge) error {
	switch scope {
	case ScopeTool:
		if purge != PurgeRetain && purge != PurgeExplicit {
			return fmt.Errorf("%s: tool scopeのpurgeは`retain|explicit`だけ（%q）", prefix, purge)
		}
	case ScopeVersion:
		if purge != PurgeWithVersion {
			return fmt.Errorf("%s: version scopeのpurgeは`with-version`だけ（%q）", prefix, purge)
		}
	}
	return nil
}

func buildReceiptProfiles(
	tables []*receiptProfileTable, ref InstallRef, storageIDs map[string]struct{},
) ([]ReceiptEnvironmentProfile, map[string]struct{}, error) {
	if len(tables) == 0 {
		return nil, nil, fmt.Errorf("environment_profilesが空")
	}
	entries := make([]ReceiptEnvironmentProfile, 0, len(tables))
	ids := make(map[string]struct{}, len(tables))
	for index, raw := range tables {
		if raw == nil {
			return nil, nil, fmt.Errorf("environment_profiles[%d]が空", index)
		}
		prefix := fmt.Sprintf("environment_profiles[%d]", index)
		entry, err := buildReceiptProfile(prefix, *raw, ref, storageIDs)
		if err != nil {
			return nil, nil, err
		}
		entries = append(entries, entry)
		ids[entry.ID] = struct{}{}
	}
	if err := requireSortedUnique("environment_profiles", len(entries), func(i int) string {
		return entries[i].ID
	}); err != nil {
		return nil, nil, err
	}
	return entries, ids, nil
}

func buildReceiptProfile(
	prefix string, raw receiptProfileTable, ref InstallRef, storageIDs map[string]struct{},
) (ReceiptEnvironmentProfile, error) {
	var value ReceiptEnvironmentProfile
	id, err := requireIdentifier(prefix+".id", raw.ID)
	if err != nil {
		return value, err
	}
	prepend, err := requireTemplateList(prefix+".path_prepend", raw.PathPrepend, commandScope, storageIDs)
	if err != nil {
		return value, err
	}
	appendPaths, err := requireTemplateList(prefix+".path_append", raw.PathAppend, commandScope, storageIDs)
	if err != nil {
		return value, err
	}
	unset, err := requireEnvNameList(prefix+".unset", raw.Unset, ref)
	if err != nil {
		return value, err
	}
	overrideAllowed, err := requireEnvNameList(prefix+".override_allowed", raw.OverrideAllowed, ref)
	if err != nil {
		return value, err
	}
	shellExport, err := requireEnvNameList(prefix+".shell_export", raw.ShellExport, ref)
	if err != nil {
		return value, err
	}
	// setはpointerで受ける。空tableとkey欠落を区別するためである。§14は
	// 「全key必須」と定めており、tableごと無いreceiptは拒否しなければならない。
	if raw.Set == nil {
		return value, fmt.Errorf("%s.setが無い", prefix)
	}
	set := *raw.Set
	// env map keyはplatform規則で一意（§14）。Windowsは環境変数名をcase非依存に
	// 扱うため、`PATH`と`Path`が両方あるprofileは1つの変数への矛盾した指定になる。
	names := mapKeys(set)
	for _, name := range names {
		if !envNameRe.MatchString(name) {
			return value, fmt.Errorf("%s.setのkey %q が環境変数名のgrammarに合わない", prefix, name)
		}
	}
	if err := requireUniqueEnvNames(prefix+".set", names, ref); err != nil {
		return value, err
	}
	for _, name := range sortedKeys(set) {
		if err := validateStorageTemplateExists(
			fmt.Sprintf("%s.set[%q]", prefix, name), set[name], storageIDs,
		); err != nil {
			return value, err
		}
		if err := validateTemplate(
			fmt.Sprintf("%s.set[%q]", prefix, name), set[name], commandScope, storageIDs,
		); err != nil {
			return value, err
		}
	}
	return ReceiptEnvironmentProfile{
		ID: id, PathPrepend: prepend, PathAppend: appendPaths,
		Set: cloneStringMap(set), Unset: unset,
		OverrideAllowed: overrideAllowed, ShellExport: shellExport,
	}, nil
}

func buildReceiptCommands(
	tables []*receiptCommandTable, storageIDs, profileIDs map[string]struct{},
) ([]ReceiptCommand, error) {
	if len(tables) == 0 {
		return nil, fmt.Errorf("commandsが空")
	}
	entries := make([]ReceiptCommand, 0, len(tables))
	for index, raw := range tables {
		if raw == nil {
			return nil, fmt.Errorf("commands[%d]が空", index)
		}
		prefix := fmt.Sprintf("commands[%d]", index)
		name, err := requireCommandName(prefix+".name", raw.Name)
		if err != nil {
			return nil, err
		}
		target, err := requirePresent(prefix+".target", raw.Target)
		if err != nil {
			return nil, err
		}
		if err := validateStorageTemplateExists(prefix+".target", target, storageIDs); err != nil {
			return nil, err
		}
		if err := validateTemplate(prefix+".target", target, commandScope, storageIDs); err != nil {
			return nil, err
		}
		fixedArgs, err := requireTemplateList(prefix+".fixed_args", raw.FixedArgs, commandScope, storageIDs)
		if err != nil {
			return nil, err
		}
		profile, err := requireIdentifier(prefix+".environment_profile", raw.EnvironmentProfile)
		if err != nil {
			return nil, err
		}
		// profileがreceiptに無ければ、runtimeが環境を組み立てられない。
		if _, known := profileIDs[profile]; !known {
			return nil, fmt.Errorf("%s.environment_profile %q がreceiptに無い", prefix, profile)
		}
		working, err := requireEnum(prefix+".working_directory", raw.WorkingDirectory, workingDirectories)
		if err != nil {
			return nil, err
		}
		passthrough, err := requireBool(prefix+".passthrough_signals", raw.PassthroughSignals)
		if err != nil {
			return nil, err
		}
		entries = append(entries, ReceiptCommand{
			Name: name, Target: target, FixedArgs: fixedArgs,
			EnvironmentProfile: profile, WorkingDirectory: working,
			PassthroughSignals: passthrough,
		})
	}
	if err := requireSortedUnique("commands", len(entries), func(i int) string {
		return entries[i].Name
	}); err != nil {
		return nil, err
	}
	return entries, nil
}

func buildReceiptProbes(
	tables []*receiptProbeTable, storageIDs map[string]struct{},
) ([]ReceiptProbe, error) {
	entries := make([]ReceiptProbe, 0, len(tables))
	for index, raw := range tables {
		if raw == nil {
			return nil, fmt.Errorf("probes[%d]が空", index)
		}
		entry, err := buildReceiptProbe(fmt.Sprintf("probes[%d]", index), *raw, storageIDs)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("probesが空")
	}
	if err := requireSortedUnique("probes", len(entries), func(i int) string {
		return entries[i].ID
	}); err != nil {
		return nil, err
	}
	return entries, nil
}

func buildReceiptProbe(
	prefix string, raw receiptProbeTable, storageIDs map[string]struct{},
) (ReceiptProbe, error) {
	var value ReceiptProbe
	id, err := requireIdentifier(prefix+".id", raw.ID)
	if err != nil {
		return value, err
	}
	runtimeCommand, err := requireCommandName(prefix+".runtime_command", raw.RuntimeCommand)
	if err != nil {
		return value, err
	}
	args, err := requireTemplateList(prefix+".args", raw.Args, probeScope, storageIDs)
	if err != nil {
		return value, err
	}
	stream, err := requireEnum(prefix+".stream", raw.Stream, probeStreams)
	if err != nil {
		return value, err
	}
	expect, err := requireEnum(prefix+".expect", raw.Expect, probeExpects)
	if err != nil {
		return value, err
	}
	regex, err := requirePresent(prefix+".regex", raw.Regex)
	if err != nil {
		return value, err
	}
	expectedVersion, err := requirePresent(prefix+".expected_version", raw.ExpectedVersion)
	if err != nil {
		return value, err
	}
	expectedRoot, err := requirePresent(prefix+".expected_root", raw.ExpectedRoot)
	if err != nil {
		return value, err
	}
	requiredPaths, err := requireRequiredPaths(prefix+".required_paths", raw.RequiredPaths, storageIDs)
	if err != nil {
		return value, err
	}
	timeout, err := requireInt64(prefix+".timeout_ms", raw.TimeoutMillis)
	if err != nil {
		return value, err
	}
	if timeout < ProbeTimeoutMinMillis || timeout > ProbeTimeoutMaxMillis {
		return value, fmt.Errorf(
			"%s.timeout_msが%d〜%d msの範囲外（%d）", prefix, ProbeTimeoutMinMillis, ProbeTimeoutMaxMillis, timeout)
	}
	required, err := requireBool(prefix+".required", raw.Required)
	if err != nil {
		return value, err
	}
	status, err := requireEnum(prefix+".status", raw.Status, probeStatuses)
	if err != nil {
		return value, err
	}
	// §14が「required=trueはpassed必須」と定める。skippedのrequired probeが
	// receiptに残ると、検証していない導入を検証済みとして扱うことになる。
	if required && status != ProbePassed {
		return value, fmt.Errorf("%s: required=trueのprobeはpassedでなければならない（%q）", prefix, status)
	}
	reportedVersion, err := requirePresent(prefix+".reported_version", raw.ReportedVersion)
	if err != nil {
		return value, err
	}
	if err := checkProbeExpectFields(prefix, expect, regex, expectedVersion, expectedRoot, reportedVersion); err != nil {
		return value, err
	}
	finishedAt, err := requireTimestampField(prefix+".finished_at", raw.FinishedAt)
	if err != nil {
		return value, err
	}
	return ReceiptProbe{
		ID: id, RuntimeCommand: runtimeCommand, Args: args, Stream: stream,
		Expect: expect, Regex: regex, ExpectedVersion: expectedVersion,
		ExpectedRoot: expectedRoot, RequiredPaths: requiredPaths,
		TimeoutMillis: timeout, Required: required, Status: status,
		ReportedVersion: reportedVersion, FinishedAt: finishedAt,
	}, nil
}

// checkProbeExpectFields はexpectごとの必須・禁止fieldを固定する。
//
// docs/06-tool-definition.md §11の表と、§14の「非該当string/arrayは空」
// 「version非対象probeはreported version空」に対応する。
func checkProbeExpectFields(
	prefix string, expect ProbeExpect, regex, expectedVersion, expectedRoot, reportedVersion string,
) error {
	switch expect {
	case ExpectVersion:
		if regex == "" || expectedVersion == "" {
			return fmt.Errorf("%s: expect=versionはregexとexpected_versionが必須", prefix)
		}
		if err := requireEmpty(prefix+".expected_root", expectedRoot); err != nil {
			return err
		}
	case ExpectSuccess:
		// §11は「expect fields禁止」と定める。regexは指定可だがversion/rootは持たない。
		if err := requireEmpty(prefix+".expected_version", expectedVersion); err != nil {
			return err
		}
		if err := requireEmpty(prefix+".expected_root", expectedRoot); err != nil {
			return err
		}
		if err := requireEmpty(prefix+".reported_version", reportedVersion); err != nil {
			return err
		}
	case ExpectPathWithin:
		if regex == "" || expectedRoot == "" {
			return fmt.Errorf("%s: expect=path-withinはregexとexpected_rootが必須", prefix)
		}
		if err := requireEmpty(prefix+".expected_version", expectedVersion); err != nil {
			return err
		}
		if err := requireEmpty(prefix+".reported_version", reportedVersion); err != nil {
			return err
		}
	}
	// version以外はreported versionを持たない（§14）。上のcaseで個別に見ているが、
	// version対象でreported versionがあるときはexpected versionと一致させる。
	if expect == ExpectVersion && reportedVersion != "" && reportedVersion != expectedVersion {
		return fmt.Errorf(
			"%s: reported_version %q がexpected_version %q と一致しない", prefix, reportedVersion, expectedVersion)
	}
	return nil
}

func buildReceiptTargets(tables []*receiptTargetTable) ([]ReceiptCommandTarget, error) {
	if len(tables) == 0 {
		return nil, fmt.Errorf("command_targetsが空")
	}
	entries := make([]ReceiptCommandTarget, 0, len(tables))
	for index, raw := range tables {
		if raw == nil {
			return nil, fmt.Errorf("command_targets[%d]が空", index)
		}
		prefix := fmt.Sprintf("command_targets[%d]", index)
		path, err := requirePresent(prefix+".path", raw.Path)
		if err != nil {
			return nil, err
		}
		relative, err := requireRelativePath(prefix+".path", path)
		if err != nil {
			return nil, err
		}
		// pathはpayload相対である（§14）。payload外のfileをmanifestへ入れると、
		// doctorがpayload破損以外の理由で失敗する。
		if !strings.HasPrefix(relative, PayloadDirectoryName+"/") {
			return nil, fmt.Errorf("%s.pathがpayload配下でない（%q）", prefix, relative)
		}
		size, err := requireInt64(prefix+".size", raw.Size)
		if err != nil {
			return nil, err
		}
		digest, err := requireDigestField(prefix+".sha256", raw.SHA256)
		if err != nil {
			return nil, err
		}
		entries = append(entries, ReceiptCommandTarget{Path: relative, Size: size, SHA256: digest})
	}
	if err := requireSortedUnique("command_targets", len(entries), func(i int) string {
		return entries[i].Path
	}); err != nil {
		return nil, err
	}
	return entries, nil
}
