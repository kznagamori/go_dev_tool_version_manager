package store

import (
	"fmt"
	"sort"
	"time"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
)

// DevelopmentClientVersion はdevelopment buildが記録するclient versionである。
//
// docs/04-storage-and-data.md §8が「development buildはclient_version=`devel`を
// 許す」と定める。release buildは§11-quality-and-ci.mdの`YYYY.MM.DD.XX`を使う。
const DevelopmentClientVersion = "devel"

// ShimDirectoryName はdata root相対のshim directory名である（§9）。
//
// 同§が「shim pathはdata root相対`shims`固定」と定める。setup stateへkeyとして
// 残すのは、値ではなく契約の存在を明示するためである。
const ShimDirectoryName = "shims"

// StateSchema は`state/schema.toml`のtyped表現である（§8）。
//
// root IDは作成後不変で、別root IDのstate、junction/symlink、receiptを
// 混在させない判断の基点になる。
type StateSchema struct {
	// Revision はfileのrevisionである。書込みごとに増やす。
	Revision int64
	// RootID はrootを一意に識別する128 bit hexである。作成後不変。
	RootID string
	// Mode はportableかuserかである。
	Mode domain.Mode
	// CreatedAt はrootを作成した時刻である。
	CreatedAt time.Time
	// UpdatedAt は最後に更新した時刻である。
	UpdatedAt time.Time
	// ClientVersion は最後に書いたclientの完全versionまたは`devel`である。
	ClientVersion string
	// StateSchema はstate fileのschema revisionである。
	StateSchema int64
	// ReceiptSchema はreceiptのschema revisionである。
	ReceiptSchema int64
	// CatalogSchema はcatalog JSONのschema revisionである。
	CatalogSchema int64
}

// schemaFile は§8のexact key集合である。
//
// 全fieldをpointerにするのは、§8が「許可keyは上記だけで全件必須」と定めるため
// である。keyの欠落をzero値として黙って通すと、必須検査が空振りする。
type schemaFile struct {
	Schema        *int64  `toml:"schema"`
	Revision      *int64  `toml:"revision"`
	RootID        *string `toml:"root_id"`
	Mode          *string `toml:"mode"`
	CreatedAt     *string `toml:"created_at"`
	UpdatedAt     *string `toml:"updated_at"`
	ClientVersion *string `toml:"client_version"`
	StateSchema   *int64  `toml:"state_schema"`
	ReceiptSchema *int64  `toml:"receipt_schema"`
	CatalogSchema *int64  `toml:"catalog_schema"`
}

// ParseStateSchema は`state/schema.toml`を読む（§8）。
func ParseStateSchema(data []byte) (StateSchema, *domain.Error) {
	var file schemaFile
	if err := loadStateTOML(data, &file); err != nil {
		return StateSchema{}, stateError("state.schema_invalid", domain.RoleState, err)
	}
	value, err := buildStateSchema(file)
	if err != nil {
		return StateSchema{}, stateError("state.schema_invalid", domain.RoleState, err)
	}
	return value, nil
}

func buildStateSchema(file schemaFile) (StateSchema, error) {
	var value StateSchema
	if err := requireSchema("schema", file.Schema); err != nil {
		return value, err
	}
	revision, err := requireRevision("revision", file.Revision)
	if err != nil {
		return value, err
	}
	rootID, err := requireIDField("root_id", file.RootID)
	if err != nil {
		return value, err
	}
	mode, err := requireMode("mode", file.Mode)
	if err != nil {
		return value, err
	}
	createdAt, err := requireTimestampField("created_at", file.CreatedAt)
	if err != nil {
		return value, err
	}
	updatedAt, err := requireTimestampField("updated_at", file.UpdatedAt)
	if err != nil {
		return value, err
	}
	clientVersion, err := requireClientVersion("client_version", file.ClientVersion)
	if err != nil {
		return value, err
	}
	// 3つのschema revisionはすべて1である（§7）。個別に読むのは、fileが
	// どのschemaを前提に書かれたかを利用者へ示すためである。
	for _, pair := range []struct {
		name  string
		value *int64
	}{
		{"state_schema", file.StateSchema},
		{"receipt_schema", file.ReceiptSchema},
		{"catalog_schema", file.CatalogSchema},
	} {
		if err := requireSchema(pair.name, pair.value); err != nil {
			return value, err
		}
	}
	return StateSchema{
		Revision:      revision,
		RootID:        rootID,
		Mode:          mode,
		CreatedAt:     createdAt,
		UpdatedAt:     updatedAt,
		ClientVersion: clientVersion,
		StateSchema:   SchemaVersion,
		ReceiptSchema: SchemaVersion,
		CatalogSchema: SchemaVersion,
	}, nil
}

// EncodeStateSchema は`state/schema.toml`を書き出す（§8）。
func EncodeStateSchema(value StateSchema) ([]byte, *domain.Error) {
	if _, err := buildStateSchema(schemaFileOf(value)); err != nil {
		return nil, stateError("state.schema_invalid", domain.RoleState, err)
	}
	data, err := encodeTOML(schemaFileOf(value))
	if err != nil {
		return nil, stateError("state.schema_invalid", domain.RoleState, err)
	}
	return data, nil
}

func schemaFileOf(value StateSchema) schemaFile {
	schema := int64(SchemaVersion)
	mode := string(value.Mode)
	createdAt := formatTimestamp(value.CreatedAt)
	updatedAt := formatTimestamp(value.UpdatedAt)
	return schemaFile{
		Schema: &schema, Revision: &value.Revision, RootID: &value.RootID,
		Mode: &mode, CreatedAt: &createdAt, UpdatedAt: &updatedAt,
		ClientVersion: &value.ClientVersion,
		StateSchema:   &schema, ReceiptSchema: &schema, CatalogSchema: &schema,
	}
}

// PathIntegration はPATH統合の方式である（§9・§17.1）。
type PathIntegration string

// PathIntegration のexactly 3値。
const (
	PathIntegrationUserPath     PathIntegration = "user-path"
	PathIntegrationShellProfile PathIntegration = "shell-profile"
	PathIntegrationNone         PathIntegration = "none"
)

var pathIntegrations = map[PathIntegration]struct{}{
	PathIntegrationUserPath: {}, PathIntegrationShellProfile: {}, PathIntegrationNone: {},
}

// Shell はshell profile統合の対象shellである（§9・§17.1）。
type Shell string

// Shell のexactly 3値。統合しない場合は空文字列を使う。
const (
	ShellBash Shell = "bash"
	ShellZsh  Shell = "zsh"
	ShellFish Shell = "fish"
)

var shells = map[Shell]struct{}{ShellBash: {}, ShellZsh: {}, ShellFish: {}}

// IntegrationKind はintegration identityの対象種別である（§9・§17.1）。
type IntegrationKind string

// IntegrationKind のexactly 3値。
const (
	IntegrationWindowsRegistryValue IntegrationKind = "windows-registry-value"
	IntegrationShellProfileFile     IntegrationKind = "shell-profile-file"
	IntegrationNone                 IntegrationKind = "none"
)

var integrationKinds = map[IntegrationKind]struct{}{
	IntegrationWindowsRegistryValue: {}, IntegrationShellProfileFile: {}, IntegrationNone: {},
}

// IntegrationIdentity は統合対象の識別と前後のdigestである（§9）。
//
// registry/profileのraw内容をsetup stateへ入れない。前後のdigestだけを持つのは、
// 変更したのが自分かどうかを判定するためであり、内容を復元するためではない。
type IntegrationIdentity struct {
	// Kind は統合対象の種別である。
	Kind IntegrationKind
	// Location はregistry keyまたはprofile fileの位置である。noneなら空。
	Location string
	// Name はregistry value名である。noneなら空。
	Name string
	// BeforeSHA256 は変更前の内容のdigestである。不存在なら64 zero。
	BeforeSHA256 string
	// AfterSHA256 は変更後の内容のdigestである。不存在なら64 zero。
	AfterSHA256 string
}

// SetupState は`state/setup.toml`のtyped表現である（§9）。
type SetupState struct {
	// Revision はfileのrevisionである。
	Revision int64
	// RootID はrootを一意に識別する128 bit hexである。
	RootID string
	// Mode はportableかuserかである。
	Mode domain.Mode
	// PathIntegration はPATH統合の方式である。
	PathIntegration PathIntegration
	// Shell はshell-profile統合の対象shellである。他方式では空。
	Shell Shell
	// ShimPath はdata root相対のshim directoryである。`shims`固定。
	ShimPath string
	// BackupID は直近のsetup backupを指す128 bit hexである。
	BackupID string
	// UpdatedAt は最後に更新した時刻である。
	UpdatedAt time.Time
	// Identity は統合対象の識別情報である。
	Identity IntegrationIdentity
}

type integrationIdentityTable struct {
	Kind         *string `toml:"kind"`
	Location     *string `toml:"location"`
	Name         *string `toml:"name"`
	BeforeSHA256 *string `toml:"before_sha256"`
	AfterSHA256  *string `toml:"after_sha256"`
}

// setupFile は§9のexact key集合である。
type setupFile struct {
	Schema          *int64                    `toml:"schema"`
	Revision        *int64                    `toml:"revision"`
	RootID          *string                   `toml:"root_id"`
	Mode            *string                   `toml:"mode"`
	PathIntegration *string                   `toml:"path_integration"`
	Shell           *string                   `toml:"shell"`
	ShimPath        *string                   `toml:"shim_path"`
	BackupID        *string                   `toml:"backup_id"`
	UpdatedAt       *string                   `toml:"updated_at"`
	Identity        *integrationIdentityTable `toml:"integration_identity"`
}

// ParseSetupState は`state/setup.toml`を読む（§9）。
func ParseSetupState(data []byte) (SetupState, *domain.Error) {
	var file setupFile
	if err := loadStateTOML(data, &file); err != nil {
		return SetupState{}, stateError("state.setup_invalid", domain.RoleState, err)
	}
	value, err := buildSetupState(file)
	if err != nil {
		return SetupState{}, stateError("state.setup_invalid", domain.RoleState, err)
	}
	return value, nil
}

func buildSetupState(file setupFile) (SetupState, error) {
	var value SetupState
	if err := requireSchema("schema", file.Schema); err != nil {
		return value, err
	}
	revision, err := requireRevision("revision", file.Revision)
	if err != nil {
		return value, err
	}
	rootID, err := requireIDField("root_id", file.RootID)
	if err != nil {
		return value, err
	}
	mode, err := requireMode("mode", file.Mode)
	if err != nil {
		return value, err
	}
	integration, err := requireEnum("path_integration", file.PathIntegration, pathIntegrations)
	if err != nil {
		return value, err
	}
	shell, err := requireShell(file.Shell, integration)
	if err != nil {
		return value, err
	}
	shimPath, err := requirePresent("shim_path", file.ShimPath)
	if err != nil {
		return value, err
	}
	if shimPath != ShimDirectoryName {
		return value, fmt.Errorf("shim_pathは%qだけを許す（%q）", ShimDirectoryName, shimPath)
	}
	backupID, err := requireIDField("backup_id", file.BackupID)
	if err != nil {
		return value, err
	}
	updatedAt, err := requireTimestampField("updated_at", file.UpdatedAt)
	if err != nil {
		return value, err
	}
	if file.Identity == nil {
		return value, fmt.Errorf("integration_identityが無い")
	}
	identity, err := buildIntegrationIdentity(*file.Identity)
	if err != nil {
		return value, err
	}
	return SetupState{
		Revision: revision, RootID: rootID, Mode: mode,
		PathIntegration: integration, Shell: shell, ShimPath: shimPath,
		BackupID: backupID, UpdatedAt: updatedAt, Identity: identity,
	}, nil
}

// requireShell はshellがpath integrationと整合することを確かめる（§9）。
//
// 同§が「shellはnone/user-pathなら空、shell-profileなら`bash|zsh|fish`」と
// 定める。組合せを検査しないと、user-path統合なのにshellが残ったstateを
// 受理してしまい、undoが誤った対象を触る。
func requireShell(raw *string, integration PathIntegration) (Shell, error) {
	text, err := requirePresent("shell", raw)
	if err != nil {
		return "", err
	}
	if integration != PathIntegrationShellProfile {
		if err := requireEmpty("shell", text); err != nil {
			return "", err
		}
		return "", nil
	}
	if _, ok := shells[Shell(text)]; !ok {
		return "", fmt.Errorf("shellが`bash|zsh|fish`でない（%q）", text)
	}
	return Shell(text), nil
}

// buildIntegrationIdentity はintegration identityを組み立てる（§9）。
//
// kind=`none`では他stringが空でdigestが64 zeroであることを要求する。
// 「不存在」の表現を1通りに固定しないと、undo可否の判定が実装ごとにぶれる。
func buildIntegrationIdentity(table integrationIdentityTable) (IntegrationIdentity, error) {
	var value IntegrationIdentity
	kind, err := requireEnum("integration_identity.kind", table.Kind, integrationKinds)
	if err != nil {
		return value, err
	}
	location, err := requirePresent("integration_identity.location", table.Location)
	if err != nil {
		return value, err
	}
	name, err := requirePresent("integration_identity.name", table.Name)
	if err != nil {
		return value, err
	}
	before, err := requireDigestField("integration_identity.before_sha256", table.BeforeSHA256)
	if err != nil {
		return value, err
	}
	after, err := requireDigestField("integration_identity.after_sha256", table.AfterSHA256)
	if err != nil {
		return value, err
	}
	if kind == IntegrationNone {
		if err := requireEmpty("integration_identity.location", location); err != nil {
			return value, err
		}
		if err := requireEmpty("integration_identity.name", name); err != nil {
			return value, err
		}
		if before != zeroDigestHex || after != zeroDigestHex {
			return value, fmt.Errorf("kind=noneのintegration_identityのdigestは64 zeroでなければならない")
		}
	}
	return IntegrationIdentity{
		Kind: kind, Location: location, Name: name,
		BeforeSHA256: before, AfterSHA256: after,
	}, nil
}

// EncodeSetupState は`state/setup.toml`を書き出す（§9）。
func EncodeSetupState(value SetupState) ([]byte, *domain.Error) {
	file := setupFileOf(value)
	if _, err := buildSetupState(file); err != nil {
		return nil, stateError("state.setup_invalid", domain.RoleState, err)
	}
	data, err := encodeTOML(file)
	if err != nil {
		return nil, stateError("state.setup_invalid", domain.RoleState, err)
	}
	return data, nil
}

func setupFileOf(value SetupState) setupFile {
	schema := int64(SchemaVersion)
	mode := string(value.Mode)
	integration := string(value.PathIntegration)
	shell := string(value.Shell)
	updatedAt := formatTimestamp(value.UpdatedAt)
	kind := string(value.Identity.Kind)
	return setupFile{
		Schema: &schema, Revision: &value.Revision, RootID: &value.RootID,
		Mode: &mode, PathIntegration: &integration, Shell: &shell,
		ShimPath: &value.ShimPath, BackupID: &value.BackupID, UpdatedAt: &updatedAt,
		Identity: &integrationIdentityTable{
			Kind: &kind, Location: &value.Identity.Location, Name: &value.Identity.Name,
			BeforeSHA256: &value.Identity.BeforeSHA256, AfterSHA256: &value.Identity.AfterSHA256,
		},
	}
}

// Selection は`state/selections.toml`の1 entryである（§11）。
type Selection struct {
	// Ref はtool、version、platformの組である。
	Ref InstallRef
	// InstallID は対応するreceiptの128 bit hexである。
	InstallID string
	// SelectedAt は選択した時刻である。
	SelectedAt time.Time
}

// Selections は`state/selections.toml`のtyped表現である（§11）。
//
// v0.1のuser selectionはmanaged版だけを表す。selectionはreceiptと`install_id`で
// 一致させ、receiptが欠落・破損していればinactiveとして扱う。
type Selections struct {
	// Revision はfileのrevisionである。
	Revision int64
	// RootID はrootを一意に識別する128 bit hexである。
	RootID string
	// UpdatedAt は最後に更新した時刻である。
	UpdatedAt time.Time
	// Entries はtool ID byte順で一意なselectionである。
	Entries []Selection
}

type selectionEntry struct {
	ToolID     *string `toml:"tool_id"`
	Version    *string `toml:"version"`
	PlatformID *string `toml:"platform_id"`
	InstallID  *string `toml:"install_id"`
	SelectedAt *string `toml:"selected_at"`
}

// selectionsFile は§11のexact key集合である。
type selectionsFile struct {
	Schema     *int64            `toml:"schema"`
	Revision   *int64            `toml:"revision"`
	RootID     *string           `toml:"root_id"`
	UpdatedAt  *string           `toml:"updated_at"`
	Selections []*selectionEntry `toml:"selections"`
}

// ParseSelections は`state/selections.toml`を読む（§11）。
func ParseSelections(data []byte) (Selections, *domain.Error) {
	var file selectionsFile
	if err := loadStateTOML(data, &file); err != nil {
		return Selections{}, stateError("state.selections_invalid", domain.RoleState, err)
	}
	value, err := buildSelections(file)
	if err != nil {
		return Selections{}, stateError("state.selections_invalid", domain.RoleState, err)
	}
	return value, nil
}

func buildSelections(file selectionsFile) (Selections, error) {
	var value Selections
	header, err := buildStateHeader(file.Schema, file.Revision, file.RootID, file.UpdatedAt)
	if err != nil {
		return value, err
	}
	entries := make([]Selection, 0, len(file.Selections))
	for index, raw := range file.Selections {
		if raw == nil {
			return value, fmt.Errorf("selections[%d]が空", index)
		}
		entry, err := buildSelection(index, *raw)
		if err != nil {
			return value, err
		}
		entries = append(entries, entry)
	}
	// toolごとに最大1件、tool ID byte順で一意（§11）。順序をparserが正とするのは
	// §7が「parserは順序に意味を持たせない」と定めるためだが、一意性と整列は
	// 検査する。整列していないfileを受理すると、書き戻しでdiffが暴れる。
	if err := requireSortedUnique("selections", len(entries), func(i int) string {
		return entries[i].Ref.Tool.String()
	}); err != nil {
		return value, err
	}
	return Selections{
		Revision: header.revision, RootID: header.rootID,
		UpdatedAt: header.updatedAt, Entries: entries,
	}, nil
}

func buildSelection(index int, raw selectionEntry) (Selection, error) {
	var value Selection
	prefix := fmt.Sprintf("selections[%d]", index)
	ref, err := requireInstallRef(prefix, raw.ToolID, raw.Version, raw.PlatformID)
	if err != nil {
		return value, err
	}
	installID, err := requireIDField(prefix+".install_id", raw.InstallID)
	if err != nil {
		return value, err
	}
	selectedAt, err := requireTimestampField(prefix+".selected_at", raw.SelectedAt)
	if err != nil {
		return value, err
	}
	return Selection{Ref: ref, InstallID: installID, SelectedAt: selectedAt}, nil
}

// EncodeSelections は`state/selections.toml`を書き出す（§11）。
func EncodeSelections(value Selections) ([]byte, *domain.Error) {
	file := selectionsFileOf(value)
	if _, err := buildSelections(file); err != nil {
		return nil, stateError("state.selections_invalid", domain.RoleState, err)
	}
	data, err := encodeTOML(file)
	if err != nil {
		return nil, stateError("state.selections_invalid", domain.RoleState, err)
	}
	return data, nil
}

func selectionsFileOf(value Selections) selectionsFile {
	schema := int64(SchemaVersion)
	updatedAt := formatTimestamp(value.UpdatedAt)
	entries := make([]*selectionEntry, 0, len(value.Entries))
	for _, entry := range value.Entries {
		toolID := entry.Ref.Tool.String()
		version := entry.Ref.Version
		platformID := entry.Ref.Platform.ID()
		selectedAt := formatTimestamp(entry.SelectedAt)
		installID := entry.InstallID
		entries = append(entries, &selectionEntry{
			ToolID: &toolID, Version: &version, PlatformID: &platformID,
			InstallID: &installID, SelectedAt: &selectedAt,
		})
	}
	sortEntriesByKey(entries, func(entry *selectionEntry) string { return *entry.ToolID })
	return selectionsFile{
		Schema: &schema, Revision: &value.Revision, RootID: &value.RootID,
		UpdatedAt: &updatedAt, Selections: entries,
	}
}

// ShimCommand は`state/shim-index.toml`の1 entryである（§12）。
type ShimCommand struct {
	// Name はPATHへ出るcommand名である。
	Name string
	// ToolID はcommandを提供するtoolの正規IDである。
	ToolID domain.ToolID
}

// ShimIndex は`state/shim-index.toml`のtyped表現である（§12）。
//
// version targetを固定せず、runtimeがproject/user selectionとreceiptから
// 解決する。receipt revision不一致時は正本receiptから再生成できるが、
// 未知commandをPATH探索しない。
type ShimIndex struct {
	// Revision はfileのrevisionである。
	Revision int64
	// RootID はrootを一意に識別する128 bit hexである。
	RootID string
	// ClientVersion は最後に書いたclientの完全versionまたは`devel`である。
	ClientVersion string
	// ReceiptIndexRevision は生成時に見たreceipt indexのrevisionである。
	ReceiptIndexRevision int64
	// UpdatedAt は最後に更新した時刻である。
	UpdatedAt time.Time
	// Commands はcommand名byte順で一意なentryである。
	Commands []ShimCommand
}

type shimCommandEntry struct {
	Name   *string `toml:"name"`
	ToolID *string `toml:"tool_id"`
}

// shimIndexFile は§12のexact key集合である。
type shimIndexFile struct {
	Schema               *int64              `toml:"schema"`
	Revision             *int64              `toml:"revision"`
	RootID               *string             `toml:"root_id"`
	ClientVersion        *string             `toml:"client_version"`
	ReceiptIndexRevision *int64              `toml:"receipt_index_revision"`
	UpdatedAt            *string             `toml:"updated_at"`
	Commands             []*shimCommandEntry `toml:"commands"`
}

// ParseShimIndex は`state/shim-index.toml`を読む（§12）。
func ParseShimIndex(data []byte) (ShimIndex, *domain.Error) {
	var file shimIndexFile
	if err := loadStateTOML(data, &file); err != nil {
		return ShimIndex{}, stateError("state.shim_index_invalid", domain.RoleShimIndex, err)
	}
	value, err := buildShimIndex(file)
	if err != nil {
		return ShimIndex{}, stateError("state.shim_index_invalid", domain.RoleShimIndex, err)
	}
	return value, nil
}

func buildShimIndex(file shimIndexFile) (ShimIndex, error) {
	var value ShimIndex
	header, err := buildStateHeader(file.Schema, file.Revision, file.RootID, file.UpdatedAt)
	if err != nil {
		return value, err
	}
	clientVersion, err := requireClientVersion("client_version", file.ClientVersion)
	if err != nil {
		return value, err
	}
	receiptRevision, err := requireRevision("receipt_index_revision", file.ReceiptIndexRevision)
	if err != nil {
		return value, err
	}
	commands := make([]ShimCommand, 0, len(file.Commands))
	for index, raw := range file.Commands {
		if raw == nil {
			return value, fmt.Errorf("commands[%d]が空", index)
		}
		prefix := fmt.Sprintf("commands[%d]", index)
		name, err := requireCommandName(prefix+".name", raw.Name)
		if err != nil {
			return value, err
		}
		toolID, err := requireToolID(prefix+".tool_id", raw.ToolID)
		if err != nil {
			return value, err
		}
		commands = append(commands, ShimCommand{Name: name, ToolID: toolID})
	}
	// commandsはname byte順で一意（§12）。registry全体のrequired command衝突検査に
	// よりcommandごとにexactly 1 toolへ対応するため、name重複は破損である。
	if err := requireSortedUnique("commands", len(commands), func(i int) string {
		return commands[i].Name
	}); err != nil {
		return value, err
	}
	return ShimIndex{
		Revision: header.revision, RootID: header.rootID, ClientVersion: clientVersion,
		ReceiptIndexRevision: receiptRevision, UpdatedAt: header.updatedAt, Commands: commands,
	}, nil
}

// EncodeShimIndex は`state/shim-index.toml`を書き出す（§12）。
func EncodeShimIndex(value ShimIndex) ([]byte, *domain.Error) {
	file := shimIndexFileOf(value)
	if _, err := buildShimIndex(file); err != nil {
		return nil, stateError("state.shim_index_invalid", domain.RoleShimIndex, err)
	}
	data, err := encodeTOML(file)
	if err != nil {
		return nil, stateError("state.shim_index_invalid", domain.RoleShimIndex, err)
	}
	return data, nil
}

func shimIndexFileOf(value ShimIndex) shimIndexFile {
	schema := int64(SchemaVersion)
	updatedAt := formatTimestamp(value.UpdatedAt)
	commands := make([]*shimCommandEntry, 0, len(value.Commands))
	for _, command := range value.Commands {
		name := command.Name
		toolID := command.ToolID.String()
		commands = append(commands, &shimCommandEntry{Name: &name, ToolID: &toolID})
	}
	sortEntriesByKey(commands, func(entry *shimCommandEntry) string { return *entry.Name })
	return shimIndexFile{
		Schema: &schema, Revision: &value.Revision, RootID: &value.RootID,
		ClientVersion: &value.ClientVersion, ReceiptIndexRevision: &value.ReceiptIndexRevision,
		UpdatedAt: &updatedAt, Commands: commands,
	}
}

// ReceiptIndexEntry は`state/receipt-index.toml`の1 entryである（§13）。
type ReceiptIndexEntry struct {
	// Ref はtool、version、platformの組である。
	Ref InstallRef
	// InstallID は対応するreceiptの128 bit hexである。
	InstallID string
	// Path はdata root相対のreceipt pathである。
	Path string
	// ReceiptSHA256 はreceipt fileの内容のdigestである。
	ReceiptSHA256 string
	// Health はreceiptの健全性である。
	Health domain.Health
}

// ReceiptIndex は`state/receipt-index.toml`のtyped表現である（§13）。
//
// indexはcacheでありreceipt走査から再構築できるが、破損receiptを健康扱いしない。
type ReceiptIndex struct {
	// Revision はfileのrevisionである。
	Revision int64
	// RootID はrootを一意に識別する128 bit hexである。
	RootID string
	// UpdatedAt は最後に更新した時刻である。
	UpdatedAt time.Time
	// Entries はtool/version/platform tupleで一意・sortされたentryである。
	Entries []ReceiptIndexEntry
}

type receiptIndexEntryTable struct {
	ToolID        *string `toml:"tool_id"`
	Version       *string `toml:"version"`
	PlatformID    *string `toml:"platform_id"`
	InstallID     *string `toml:"install_id"`
	Path          *string `toml:"path"`
	ReceiptSHA256 *string `toml:"receipt_sha256"`
	Health        *string `toml:"health"`
}

// receiptIndexFile は§13のexact key集合である。
type receiptIndexFile struct {
	Schema    *int64                    `toml:"schema"`
	Revision  *int64                    `toml:"revision"`
	RootID    *string                   `toml:"root_id"`
	UpdatedAt *string                   `toml:"updated_at"`
	Receipts  []*receiptIndexEntryTable `toml:"receipts"`
}

// ParseReceiptIndex は`state/receipt-index.toml`を読む（§13）。
func ParseReceiptIndex(data []byte) (ReceiptIndex, *domain.Error) {
	var file receiptIndexFile
	if err := loadStateTOML(data, &file); err != nil {
		return ReceiptIndex{}, stateError("state.receipt_index_invalid", domain.RoleReceiptIndex, err)
	}
	value, err := buildReceiptIndex(file)
	if err != nil {
		return ReceiptIndex{}, stateError("state.receipt_index_invalid", domain.RoleReceiptIndex, err)
	}
	return value, nil
}

func buildReceiptIndex(file receiptIndexFile) (ReceiptIndex, error) {
	var value ReceiptIndex
	header, err := buildStateHeader(file.Schema, file.Revision, file.RootID, file.UpdatedAt)
	if err != nil {
		return value, err
	}
	entries := make([]ReceiptIndexEntry, 0, len(file.Receipts))
	for index, raw := range file.Receipts {
		if raw == nil {
			return value, fmt.Errorf("receipts[%d]が空", index)
		}
		entry, err := buildReceiptIndexEntry(index, *raw)
		if err != nil {
			return value, err
		}
		entries = append(entries, entry)
	}
	if err := requireSortedUnique("receipts", len(entries), func(i int) string {
		return entries[i].Ref.SortKey()
	}); err != nil {
		return value, err
	}
	return ReceiptIndex{
		Revision: header.revision, RootID: header.rootID,
		UpdatedAt: header.updatedAt, Entries: entries,
	}, nil
}

func buildReceiptIndexEntry(index int, raw receiptIndexEntryTable) (ReceiptIndexEntry, error) {
	var value ReceiptIndexEntry
	prefix := fmt.Sprintf("receipts[%d]", index)
	ref, err := requireInstallRef(prefix, raw.ToolID, raw.Version, raw.PlatformID)
	if err != nil {
		return value, err
	}
	installID, err := requireIDField(prefix+".install_id", raw.InstallID)
	if err != nil {
		return value, err
	}
	path, err := requirePresent(prefix+".path", raw.Path)
	if err != nil {
		return value, err
	}
	relative, err := requireRelativePath(prefix+".path", path)
	if err != nil {
		return value, err
	}
	digest, err := requireDigestField(prefix+".receipt_sha256", raw.ReceiptSHA256)
	if err != nil {
		return value, err
	}
	healthText, err := requirePresent(prefix+".health", raw.Health)
	if err != nil {
		return value, err
	}
	health, err := domain.ParseHealth(healthText)
	if err != nil {
		return value, fmt.Errorf("%s.health: %w", prefix, err)
	}
	return ReceiptIndexEntry{
		Ref: ref, InstallID: installID, Path: relative,
		ReceiptSHA256: digest, Health: health,
	}, nil
}

// EncodeReceiptIndex は`state/receipt-index.toml`を書き出す（§13）。
func EncodeReceiptIndex(value ReceiptIndex) ([]byte, *domain.Error) {
	file := receiptIndexFileOf(value)
	if _, err := buildReceiptIndex(file); err != nil {
		return nil, stateError("state.receipt_index_invalid", domain.RoleReceiptIndex, err)
	}
	data, err := encodeTOML(file)
	if err != nil {
		return nil, stateError("state.receipt_index_invalid", domain.RoleReceiptIndex, err)
	}
	return data, nil
}

func receiptIndexFileOf(value ReceiptIndex) receiptIndexFile {
	schema := int64(SchemaVersion)
	updatedAt := formatTimestamp(value.UpdatedAt)
	entries := make([]*receiptIndexEntryTable, 0, len(value.Entries))
	for _, entry := range value.Entries {
		toolID := entry.Ref.Tool.String()
		version := entry.Ref.Version
		platformID := entry.Ref.Platform.ID()
		installID := entry.InstallID
		path := entry.Path
		digest := entry.ReceiptSHA256
		health := string(entry.Health)
		entries = append(entries, &receiptIndexEntryTable{
			ToolID: &toolID, Version: &version, PlatformID: &platformID,
			InstallID: &installID, Path: &path, ReceiptSHA256: &digest, Health: &health,
		})
	}
	sortEntriesByKey(entries, func(entry *receiptIndexEntryTable) string {
		return *entry.ToolID + "\x00" + *entry.Version + "\x00" + *entry.PlatformID
	})
	return receiptIndexFile{
		Schema: &schema, Revision: &value.Revision, RootID: &value.RootID,
		UpdatedAt: &updatedAt, Receipts: entries,
	}
}

// BackupKind はsetup backupの対象種別である（§10・§17.1）。
type BackupKind string

// BackupKind のexactly 2値。
const (
	BackupWindowsUserPath BackupKind = "windows-user-path"
	BackupShellProfile    BackupKind = "shell-profile"
)

var backupKinds = map[BackupKind]struct{}{
	BackupWindowsUserPath: {}, BackupShellProfile: {},
}

// BackupRawMaxBytes はbase64 decode後のraw bytesの上限である。
//
// docs/04-storage-and-data.md §10が「Base64 decode後のsize上限を適用」と定め、
// §21の`Windows user PATH 24,576 UTF-16 code unit`が最大の対象である。
// UTF-16LEは1 code unit 2 byteのため49,152 byteだが、shell profileも同じfileへ
// 入るため、§21で最も近い一般上限であるstate TOML 1 MiBを採る。base64は
// 4/3へ膨らむため、file上限1 MiBの内側に収まる値でなければならない。
const BackupRawMaxBytes = 512 << 10

// SetupBackup はsetup integrationのbackupである（§10）。
//
// backupは即時rollback/diagnose用で、remove時に利用者の後続変更全体を
// 巻き戻す用途にしない。
type SetupBackup struct {
	// BackupID はbackupを一意に識別する128 bit hexである。
	BackupID string
	// RootID はrootを一意に識別する128 bit hexである。
	RootID string
	// Kind は対象種別である。
	Kind BackupKind
	// CreatedAt はbackupを取得した時刻である。
	CreatedAt time.Time
	// Target は対象のlocatorである。registry valueまたはprofile path。
	Target string
	// Existed は取得時点で対象が存在したかである。
	Existed bool
	// ValueType はregistry valueの型である。profileでは空。
	ValueType string
	// Raw は対象のraw bytesである。不存在なら空。
	Raw []byte
	// SHA256 はraw bytesのdigestである。不存在なら64 zero。
	SHA256 string
}

// backupFile は§10のexact key集合である。
type backupFile struct {
	Schema         *int64  `toml:"schema"`
	BackupID       *string `toml:"backup_id"`
	RootID         *string `toml:"root_id"`
	Kind           *string `toml:"kind"`
	CreatedAt      *string `toml:"created_at"`
	Target         *string `toml:"target"`
	Existed        *bool   `toml:"existed"`
	ValueType      *string `toml:"value_type"`
	RawBytesBase64 *string `toml:"raw_bytes_base64"`
	SHA256         *string `toml:"sha256"`
}

// requireIDField は128 bit randomの32 lowercase hexを読む（§7）。
func requireIDField(field string, raw *string) (string, error) {
	text, err := requirePresent(field, raw)
	if err != nil {
		return "", err
	}
	if !idHexRe.MatchString(text) {
		return "", fmt.Errorf("%sは32 lowercase hexでなければならない（%q）", field, text)
	}
	return text, nil
}

// requireDigestField はgdtvm自身が計算した64 lowercase hexを読む（§7）。
func requireDigestField(field string, raw *string) (string, error) {
	text, err := requirePresent(field, raw)
	if err != nil {
		return "", err
	}
	if _, err := parseInternalDigest(field, text); err != nil {
		return "", err
	}
	return text, nil
}

// requireTimestampField は必須timestampを読む（§7）。
func requireTimestampField(field string, raw *string) (time.Time, error) {
	text, err := requirePresent(field, raw)
	if err != nil {
		return time.Time{}, err
	}
	return parseTimestamp(field, text)
}

// requireMode はmode enumを読む（§17.1）。
func requireMode(field string, raw *string) (domain.Mode, error) {
	text, err := requirePresent(field, raw)
	if err != nil {
		return "", err
	}
	mode, err := domain.ParseMode(text)
	if err != nil {
		return "", fmt.Errorf("%s: %w", field, err)
	}
	return mode, nil
}

// requireToolID は正規tool IDを読む。aliasはregistryが解決済みである前提。
func requireToolID(field string, raw *string) (domain.ToolID, error) {
	text, err := requirePresent(field, raw)
	if err != nil {
		return domain.ToolID{}, err
	}
	toolID, err := domain.ParseToolID(text)
	if err != nil {
		return domain.ToolID{}, fmt.Errorf("%s: %w", field, err)
	}
	return toolID, nil
}

// requireClientVersion はclient versionを読む（§8）。
//
// 完全versionのgrammarは[11-quality-and-ci.md]の`YYYY.MM.DD.XX`だが、
// developmentは`devel`を使う。値の妥当性は本packageでは形だけを見て、
// build metadataとの一致はbuild側の責務とする。
func requireClientVersion(field string, raw *string) (string, error) {
	text, err := requirePresent(field, raw)
	if err != nil {
		return "", err
	}
	if text == DevelopmentClientVersion {
		return text, nil
	}
	if !clientVersionRe.MatchString(text) {
		return "", fmt.Errorf("%sが`YYYY.MM.DD.XX`でも%qでもない（%q）", field, DevelopmentClientVersion, text)
	}
	return text, nil
}

// stateHeader は複数のstate fileが共有するtop-level fieldである。
type stateHeader struct {
	revision  int64
	rootID    string
	updatedAt time.Time
}

func buildStateHeader(schema, revision *int64, rootID, updatedAt *string) (stateHeader, error) {
	var header stateHeader
	if err := requireSchema("schema", schema); err != nil {
		return header, err
	}
	value, err := requireRevision("revision", revision)
	if err != nil {
		return header, err
	}
	id, err := requireIDField("root_id", rootID)
	if err != nil {
		return header, err
	}
	updated, err := requireTimestampField("updated_at", updatedAt)
	if err != nil {
		return header, err
	}
	return stateHeader{revision: value, rootID: id, updatedAt: updated}, nil
}

// requireSortedUnique はentryがkey昇順かつ一意であることを確かめる（§7）。
func requireSortedUnique(field string, count int, keyOf func(int) string) error {
	for index := 1; index < count; index++ {
		previous, current := keyOf(index-1), keyOf(index)
		if previous == current {
			return fmt.Errorf("%sのkey %q が重複している", field, current)
		}
		if previous > current {
			return fmt.Errorf("%sがkey昇順でない（%q の後に %q）", field, previous, current)
		}
	}
	return nil
}

// sortEntriesByKey は書出し前にentryをkey昇順へ並べる（§7）。
func sortEntriesByKey[T any](entries []T, keyOf func(T) string) {
	sort.SliceStable(entries, func(i, j int) bool {
		return keyOf(entries[i]) < keyOf(entries[j])
	})
}

// loadStateTOML はstate TOMLをsize検査つきでdecodeする。
func loadStateTOML(data []byte, target any) error {
	if err := requireSize("state TOML", data, StateFileMaxBytes); err != nil {
		return err
	}
	return decodeTOML(data, target)
}
