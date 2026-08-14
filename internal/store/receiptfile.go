package store

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
)

// receiptFile はdocs/04-storage-and-data.md §14のexact key集合である。
//
// 全fieldをpointer/sliceで受けるのは、§14が「全件必須、arrayはstorageだけ空可」と
// 定めるためである。keyの欠落をzero値として黙って通すと必須検査が空振りする。
type receiptFile struct {
	Schema              *int64                 `toml:"schema"`
	InstallID           *string                `toml:"install_id"`
	RootID              *string                `toml:"root_id"`
	ToolID              *string                `toml:"tool_id"`
	Version             *string                `toml:"version"`
	PlatformID          *string                `toml:"platform_id"`
	InstalledAt         *string                `toml:"installed_at"`
	ClientVersion       *string                `toml:"client_version"`
	ClientCommit        *string                `toml:"client_commit"`
	DefinitionPath      *string                `toml:"definition_path"`
	DefinitionSHA256    *string                `toml:"definition_sha256"`
	PayloadPath         *string                `toml:"payload_path"`
	Artifact            *receiptArtifactTable  `toml:"artifact"`
	Storage             []*receiptStorageTable `toml:"storage"`
	Commands            []*receiptCommandTable `toml:"commands"`
	EnvironmentProfiles []*receiptProfileTable `toml:"environment_profiles"`
	Probes              []*receiptProbeTable   `toml:"probes"`
	CommandTargets      []*receiptTargetTable  `toml:"command_targets"`
}

type receiptArtifactTable struct {
	ProviderKind          *string `toml:"provider_kind"`
	ProviderName          *string `toml:"provider_name"`
	ProviderRelease       *string `toml:"provider_release"`
	URL                   *string `toml:"url"`
	File                  *string `toml:"file"`
	Size                  *int64  `toml:"size"`
	Digest                *string `toml:"digest"`
	ChecksumSource        *string `toml:"checksum_source"`
	ThirdPartyApproved    *bool   `toml:"third_party_approved"`
	LicenseNoticeApproved *bool   `toml:"license_notice_approved"`
}

type receiptStorageTable struct {
	ID    *string `toml:"id"`
	Kind  *string `toml:"kind"`
	Scope *string `toml:"scope"`
	Path  *string `toml:"path"`
	Purge *string `toml:"purge"`
}

type receiptCommandTable struct {
	Name               *string   `toml:"name"`
	Target             *string   `toml:"target"`
	FixedArgs          *[]string `toml:"fixed_args"`
	EnvironmentProfile *string   `toml:"environment_profile"`
	WorkingDirectory   *string   `toml:"working_directory"`
	PassthroughSignals *bool     `toml:"passthrough_signals"`
}

type receiptProfileTable struct {
	ID              *string            `toml:"id"`
	PathPrepend     *[]string          `toml:"path_prepend"`
	PathAppend      *[]string          `toml:"path_append"`
	Set             *map[string]string `toml:"set"`
	Unset           *[]string          `toml:"unset"`
	OverrideAllowed *[]string          `toml:"override_allowed"`
	ShellExport     *[]string          `toml:"shell_export"`
}

type receiptProbeTable struct {
	ID              *string   `toml:"id"`
	RuntimeCommand  *string   `toml:"runtime_command"`
	Args            *[]string `toml:"args"`
	Stream          *string   `toml:"stream"`
	Expect          *string   `toml:"expect"`
	Regex           *string   `toml:"regex"`
	ExpectedVersion *string   `toml:"expected_version"`
	ExpectedRoot    *string   `toml:"expected_root"`
	RequiredPaths   *[]string `toml:"required_paths"`
	TimeoutMillis   *int64    `toml:"timeout_ms"`
	Required        *bool     `toml:"required"`
	Status          *string   `toml:"status"`
	ReportedVersion *string   `toml:"reported_version"`
	FinishedAt      *string   `toml:"finished_at"`
}

type receiptTargetTable struct {
	Path   *string `toml:"path"`
	Size   *int64  `toml:"size"`
	SHA256 *string `toml:"sha256"`
}

// identifierRe はdocs/06-tool-definition.md §3のkebab-case identifierである。
//
// storage ID、profile ID、probe IDが共有する。
var identifierRe = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// envNameRe は環境変数名のgrammarである。
//
// docs/04-storage-and-data.md §14は「env map keyはplatform規則で一意」と定めるが
// 文字集合を定めていない。POSIXとWindowsの両方で安全な範囲へ限る。`=`とNULは
// どちらのOSでも環境block自体を壊す。
var envNameRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// regexpMustCompile は[regexp.MustCompile]の別名である。
//
// receipt.goがpackage levelでcompileするregexpを、同じ規約で1か所に集めるために置く。
func regexpMustCompile(pattern string) *regexp.Regexp { return regexp.MustCompile(pattern) }

// requireNonEmpty は必須かつ非空のstringを読む。
func requireNonEmpty(field string, raw *string) (string, error) {
	text, err := requirePresent(field, raw)
	if err != nil {
		return "", err
	}
	if text == "" {
		return "", fmt.Errorf("%sが空", field)
	}
	return text, nil
}

// requireIdentifier はkebab-case identifierを読む（docs/06-tool-definition.md §3）。
func requireIdentifier(field string, raw *string) (string, error) {
	text, err := requirePresent(field, raw)
	if err != nil {
		return "", err
	}
	if !identifierRe.MatchString(text) {
		return "", fmt.Errorf("%sがkebab-case identifierでない（%q）", field, text)
	}
	return text, nil
}

// requireInt64 は必須かつ非負・JSON安全範囲のintegerを読む（§7）。
func requireInt64(field string, raw *int64) (int64, error) {
	if raw == nil {
		return 0, fmt.Errorf("%sが無い", field)
	}
	return requireNonNegative(field, *raw)
}

// requireTemplateList はtemplateを含みうるstring arrayを読む。
//
// keyの欠落と空arrayを区別するためpointerで受ける。§14は「全key必須で
// 非該当string/arrayは空」と定めており、keyの欠落は許さない。
func requireTemplateList(
	field string, raw *[]string, scope templateScope, storageIDs map[string]struct{},
) ([]string, error) {
	if raw == nil {
		return nil, fmt.Errorf("%sが無い", field)
	}
	values := *raw
	for index, value := range values {
		item := fmt.Sprintf("%s[%d]", field, index)
		if err := validateStorageTemplateExists(item, value, storageIDs); err != nil {
			return nil, err
		}
		if err := validateTemplate(item, value, scope, storageIDs); err != nil {
			return nil, err
		}
	}
	return append([]string(nil), values...), nil
}

// requireRequiredPaths は`file:`/`directory:`prefix付きtemplate列を読む。
//
// docs/06-tool-definition.md §11が「entryは`file:<template>|directory:<template>`の
// 文字列として記述し、unknown prefixを拒否する」と定める。
func requireRequiredPaths(
	field string, raw *[]string, storageIDs map[string]struct{},
) ([]string, error) {
	if raw == nil {
		return nil, fmt.Errorf("%sが無い", field)
	}
	values := *raw
	for index, value := range values {
		item := fmt.Sprintf("%s[%d]", field, index)
		prefix, template, found := strings.Cut(value, ":")
		if !found {
			return nil, fmt.Errorf("%sに`file:`または`directory:`prefixが無い（%q）", item, value)
		}
		if _, known := requiredPathPrefixes[prefix]; !known {
			return nil, fmt.Errorf("%sのprefix %q は`file|directory`でない", item, prefix)
		}
		if err := validateStorageTemplateExists(item, template, storageIDs); err != nil {
			return nil, err
		}
		if err := validateTemplate(item, template, probeScope, storageIDs); err != nil {
			return nil, err
		}
	}
	return append([]string(nil), values...), nil
}

// requireEnvNameList は環境変数名のarrayを読む。
func requireEnvNameList(field string, raw *[]string, ref InstallRef) ([]string, error) {
	if raw == nil {
		return nil, fmt.Errorf("%sが無い", field)
	}
	values := *raw
	for index, name := range values {
		if !envNameRe.MatchString(name) {
			return nil, fmt.Errorf("%s[%d]が環境変数名のgrammarに合わない（%q）", field, index, name)
		}
	}
	if err := requireUniqueEnvNames(field, values, ref); err != nil {
		return nil, err
	}
	return append([]string(nil), values...), nil
}

// requireUniqueEnvNames は環境変数名がplatform規則で一意であることを確かめる（§14）。
//
// Windowsは環境変数名をcase非依存に扱うため、`PATH`と`Path`が同時にあると
// 1つの変数への矛盾した指定になる。Linuxはcase sensitiveのため別変数である。
func requireUniqueEnvNames(field string, names []string, ref InstallRef) error {
	seen := make(map[string]string, len(names))
	windows := ref.Platform.OS() == "windows"
	for _, name := range names {
		key := name
		if windows {
			key = strings.ToUpper(name)
		}
		if previous, duplicate := seen[key]; duplicate {
			return fmt.Errorf("%sの環境変数名 %q と %q がplatform規則で衝突する", field, previous, name)
		}
		seen[key] = name
	}
	return nil
}

func mapKeys(source map[string]string) []string {
	keys := make([]string, 0, len(source))
	for key := range source {
		keys = append(keys, key)
	}
	return keys
}

func sortedKeys(source map[string]string) []string {
	keys := mapKeys(source)
	sort.Strings(keys)
	return keys
}

func cloneStringMap(source map[string]string) map[string]string {
	if source == nil {
		return nil
	}
	clone := make(map[string]string, len(source))
	for key, value := range source {
		clone[key] = value
	}
	return clone
}

// receiptFileOf はtyped receiptをexact key集合のTOML構造へ変換する。
//
// 各arrayは§7のkey順で整列してから返す。同じ内容から同じbyte列が出るようにし、
// receipt自身のSHA-256（`receipt-index.toml`の`receipt_sha256`）が内容だけで
// 決まるようにするためである。
func receiptFileOf(value Receipt) receiptFile {
	schema := int64(SchemaVersion)
	toolID := value.Ref.Tool.String()
	versionText := value.Ref.Version
	platformID := value.Ref.Platform.ID()
	installedAt := formatTimestamp(value.InstalledAt)

	return receiptFile{
		Schema: &schema, InstallID: &value.InstallID, RootID: &value.RootID,
		ToolID: &toolID, Version: &versionText, PlatformID: &platformID,
		InstalledAt: &installedAt, ClientVersion: &value.ClientVersion,
		ClientCommit: &value.ClientCommit, DefinitionPath: &value.DefinitionPath,
		DefinitionSHA256: &value.DefinitionSHA256, PayloadPath: &value.PayloadPath,
		Artifact:            artifactTableOf(value.Artifact),
		Storage:             storageTablesOf(value.Storage),
		Commands:            commandTablesOf(value.Commands),
		EnvironmentProfiles: profileTablesOf(value.EnvironmentProfiles),
		Probes:              probeTablesOf(value.Probes),
		CommandTargets:      targetTablesOf(value.CommandTargets),
	}
}

func artifactTableOf(value ReceiptArtifact) *receiptArtifactTable {
	kind := string(value.ProviderKind)
	source := string(value.ChecksumSource)
	digest := upstreamDigestText(value.Digest)
	return &receiptArtifactTable{
		ProviderKind: &kind, ProviderName: &value.ProviderName,
		ProviderRelease: &value.ProviderRelease, URL: &value.URL, File: &value.File,
		Size: &value.Size, Digest: &digest, ChecksumSource: &source,
		ThirdPartyApproved:    &value.ThirdPartyApproved,
		LicenseNoticeApproved: &value.LicenseNoticeApproved,
	}
}

// upstreamDigestText はupstream digestを`<algorithm>:<hex>`へ戻す。
//
// zero値のときに`:`だけの文字列を作らないようにする。zero値はparse側が
// 「digestが無い」として拒否する。
func upstreamDigestText(digest domain.Digest) string {
	if digest.IsZero() {
		return ""
	}
	return digest.Upstream()
}

func storageTablesOf(values []ReceiptStorage) []*receiptStorageTable {
	tables := make([]*receiptStorageTable, 0, len(values))
	for _, value := range values {
		id, path := value.ID, value.Path
		kind, scope, purge := string(value.Kind), string(value.Scope), string(value.Purge)
		tables = append(tables, &receiptStorageTable{
			ID: &id, Kind: &kind, Scope: &scope, Path: &path, Purge: &purge,
		})
	}
	sortEntriesByKey(tables, func(table *receiptStorageTable) string { return *table.ID })
	return tables
}

func commandTablesOf(values []ReceiptCommand) []*receiptCommandTable {
	tables := make([]*receiptCommandTable, 0, len(values))
	for _, value := range values {
		name, target, profile := value.Name, value.Target, value.EnvironmentProfile
		working := string(value.WorkingDirectory)
		args := nonNilStrings(value.FixedArgs)
		passthrough := value.PassthroughSignals
		tables = append(tables, &receiptCommandTable{
			Name: &name, Target: &target, FixedArgs: &args,
			EnvironmentProfile: &profile, WorkingDirectory: &working,
			PassthroughSignals: &passthrough,
		})
	}
	sortEntriesByKey(tables, func(table *receiptCommandTable) string { return *table.Name })
	return tables
}

func profileTablesOf(values []ReceiptEnvironmentProfile) []*receiptProfileTable {
	tables := make([]*receiptProfileTable, 0, len(values))
	for _, value := range values {
		id := value.ID
		prepend, appendPaths := nonNilStrings(value.PathPrepend), nonNilStrings(value.PathAppend)
		unset, override := nonNilStrings(value.Unset), nonNilStrings(value.OverrideAllowed)
		shellExport := nonNilStrings(value.ShellExport)
		set := value.Set
		if set == nil {
			set = map[string]string{}
		}
		setTable := set
		tables = append(tables, &receiptProfileTable{
			ID: &id, PathPrepend: &prepend, PathAppend: &appendPaths, Set: &setTable,
			Unset: &unset, OverrideAllowed: &override, ShellExport: &shellExport,
		})
	}
	sortEntriesByKey(tables, func(table *receiptProfileTable) string { return *table.ID })
	return tables
}

func probeTablesOf(values []ReceiptProbe) []*receiptProbeTable {
	tables := make([]*receiptProbeTable, 0, len(values))
	for _, value := range values {
		id, command := value.ID, value.RuntimeCommand
		stream, expect, status := string(value.Stream), string(value.Expect), string(value.Status)
		regex, expectedVersion := value.Regex, value.ExpectedVersion
		expectedRoot, reportedVersion := value.ExpectedRoot, value.ReportedVersion
		args, requiredPaths := nonNilStrings(value.Args), nonNilStrings(value.RequiredPaths)
		timeout, required := value.TimeoutMillis, value.Required
		finishedAt := formatTimestamp(value.FinishedAt)
		tables = append(tables, &receiptProbeTable{
			ID: &id, RuntimeCommand: &command, Args: &args, Stream: &stream,
			Expect: &expect, Regex: &regex, ExpectedVersion: &expectedVersion,
			ExpectedRoot: &expectedRoot, RequiredPaths: &requiredPaths,
			TimeoutMillis: &timeout, Required: &required, Status: &status,
			ReportedVersion: &reportedVersion, FinishedAt: &finishedAt,
		})
	}
	sortEntriesByKey(tables, func(table *receiptProbeTable) string { return *table.ID })
	return tables
}

func targetTablesOf(values []ReceiptCommandTarget) []*receiptTargetTable {
	tables := make([]*receiptTargetTable, 0, len(values))
	for _, value := range values {
		path, digest, size := value.Path, value.SHA256, value.Size
		tables = append(tables, &receiptTargetTable{Path: &path, Size: &size, SHA256: &digest})
	}
	sortEntriesByKey(tables, func(table *receiptTargetTable) string { return *table.Path })
	return tables
}

// nonNilStrings はnil sliceを空sliceへ正規化する。
//
// TOML encoderはnilを`key`の欠落として書くため、§14の「全key必須で非該当
// arrayは空」を満たすには空sliceが要る。
func nonNilStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return append([]string(nil), values...)
}
