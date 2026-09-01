package store

import (
	"fmt"
	"sort"
	"time"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
)

// PlanWarningCode は事前表示・承認用のwarning codeである（docs/04-storage-and-data.md §16.1）。
type PlanWarningCode string

// PlanWarningCode のexactly 8件（§16.1）。未定義codeをPlanへ出さない。
const (
	WarnThirdParty         PlanWarningCode = "W_THIRD_PARTY"
	WarnRestrictiveLicense PlanWarningCode = "W_RESTRICTIVE_LICENSE"
	WarnPrerelease         PlanWarningCode = "W_PRERELEASE"
	WarnEOL                PlanWarningCode = "W_EOL"
	WarnDestructive        PlanWarningCode = "W_DESTRUCTIVE"
	WarnShellModification  PlanWarningCode = "W_SHELL_MODIFICATION"
	WarnModeChange         PlanWarningCode = "W_MODE_CHANGE"
	WarnRestartRequired    PlanWarningCode = "W_RESTART_REQUIRED"
)

// planWarningApproval は各codeが明示承認を要するかである（§16.1の表）。
//
// bool値まで表で持つのは、「`requires_explicit_approval=true`のcode集合が
// Approvalの単位」（同§）だからである。codeごとの真偽をPlan作成側の判断に
// させると、同じcodeが場面によって承認要否を変えられてしまう。
var planWarningApproval = map[PlanWarningCode]bool{
	WarnThirdParty: true, WarnRestrictiveLicense: true, WarnPrerelease: true,
	WarnEOL: true, WarnDestructive: true, WarnShellModification: true,
	WarnModeChange: true,
	// §16.1が「`W_RESTART_REQUIRED`は情報提供であり承認の対象にしない」と定める。
	WarnRestartRequired: false,
}

// PlanWarningCodeCount とPlanApprovalCodeCount は§16.1が定める件数である。
const (
	PlanWarningCodeCount  = 8
	PlanApprovalCodeCount = 7
)

// PlanOperation はPlanが表す変更operationである（§17.1）。
type PlanOperation string

// PlanOperation のexactly 5値。
const (
	OperationSetup       PlanOperation = "setup"
	OperationSetupRemove PlanOperation = "setup-remove"
	OperationInstall     PlanOperation = "install"
	OperationUse         PlanOperation = "use"
	OperationUninstall   PlanOperation = "uninstall"
)

var planOperations = map[PlanOperation]struct{}{
	OperationSetup: {}, OperationSetupRemove: {}, OperationInstall: {},
	OperationUse: {}, OperationUninstall: {},
}

// ProviderNone は対象toolがないPlan operationのprovider kindである（§17.1）。
const ProviderNone ProviderKind = "none"

// planProviderKinds はPlanが許すprovider kindである。
//
// §17.1が「対象toolがないPlan operationだけ`none`」と定めるため、receiptや
// catalogと違って`none`を含む。
var planProviderKinds = map[ProviderKind]struct{}{
	ProviderOfficial: {}, ProviderThirdParty: {}, ProviderNone: {},
}

// FilesystemCapability はsetupが確認したfilesystem機能である（§17.1）。
type FilesystemCapability string

// FilesystemCapability のexactly 7値。
const (
	CapabilityAtomicReplace   FilesystemCapability = "atomic-replace"
	CapabilityDirectoryRename FilesystemCapability = "directory-rename"
	CapabilityFileIdentity    FilesystemCapability = "file-identity"
	CapabilityOwnerEnforce    FilesystemCapability = "owner-enforcement"
	CapabilityJunction        FilesystemCapability = "junction"
	CapabilitySymlink         FilesystemCapability = "symlink"
	CapabilityHardlink        FilesystemCapability = "hardlink"
)

var filesystemCapabilities = map[FilesystemCapability]struct{}{
	CapabilityAtomicReplace: {}, CapabilityDirectoryRename: {}, CapabilityFileIdentity: {},
	CapabilityOwnerEnforce: {}, CapabilityJunction: {}, CapabilitySymlink: {},
	CapabilityHardlink: {},
}

// FilesystemCapabilityCount は§17.1が定めるcapability数である。
const FilesystemCapabilityCount = 7

// LinkStrategy はcurrent linkの実現方式である（§17.1）。
type LinkStrategy string

// LinkStrategy のexactly 2値。
const (
	LinkJunction LinkStrategy = "junction"
	LinkSymlink  LinkStrategy = "symlink"
)

var linkStrategies = map[LinkStrategy]struct{}{LinkJunction: {}, LinkSymlink: {}}

// ShimStrategy はshim実体の実現方式である（§17.1）。
type ShimStrategy string

// ShimStrategy のexactly 3値。
const (
	ShimHardlink         ShimStrategy = "hardlink"
	ShimSymlink          ShimStrategy = "symlink"
	ShimFallbackResolver ShimStrategy = "fallback-resolver"
)

var shimStrategies = map[ShimStrategy]struct{}{
	ShimHardlink: {}, ShimSymlink: {}, ShimFallbackResolver: {},
}

// WriteAction はPlanが列挙する書込みの種別である（§17.1）。
type WriteAction string

// WriteAction のexactly 6値。
const (
	WriteCreate        WriteAction = "create"
	WriteReplace       WriteAction = "replace"
	WriteRemove        WriteAction = "remove"
	WriteJunction      WriteAction = "junction"
	WriteSymlink       WriteAction = "symlink"
	WriteRegistryValue WriteAction = "registry-value"
)

var writeActions = map[WriteAction]struct{}{
	WriteCreate: {}, WriteReplace: {}, WriteRemove: {},
	WriteJunction: {}, WriteSymlink: {}, WriteRegistryValue: {},
}

// StorageAction はPlanが表すstorageの扱いである（§17.1）。
type StorageAction string

// StorageAction のexactly 3値。
const (
	StorageCreate StorageAction = "create"
	StorageRetain StorageAction = "retain"
	StoragePurgeA StorageAction = "purge"
)

var storageActions = map[StorageAction]struct{}{
	StorageCreate: {}, StorageRetain: {}, StoragePurgeA: {},
}

// ArchiveFormat はartifactの書庫形式である（§17.1）。
type ArchiveFormat string

// ArchiveFormat のexactly 2値。
const (
	FormatZip   ArchiveFormat = "zip"
	FormatTarGz ArchiveFormat = "tar.gz"
)

var archiveFormats = map[ArchiveFormat]struct{}{FormatZip: {}, FormatTarGz: {}}

// PlanArgKind はPlanのargv要素の種別である（§16）。
type PlanArgKind string

// PlanArgKind のexactly 2値。
const (
	ArgLiteral PlanArgKind = "literal"
	ArgPath    PlanArgKind = "path"
)

var planArgKinds = map[PlanArgKind]struct{}{ArgLiteral: {}, ArgPath: {}}

// RequiredPathKind はprobeが要求するpathの種別である（§17.1）。
type RequiredPathKind string

// RequiredPathKind のexactly 2値。
const (
	RequiredFile      RequiredPathKind = "file"
	RequiredDirectory RequiredPathKind = "directory"
)

var requiredPathKinds = map[RequiredPathKind]struct{}{
	RequiredFile: {}, RequiredDirectory: {},
}

// DownloadDestinationRoles はdownload先として許すroleである（§16）。
//
// 同§が「`destination.role=download-cache|staging`」と定める。
var downloadDestinationRoles = map[domain.PathRole]struct{}{
	domain.RoleDownloadCache: {}, domain.RoleStaging: {},
}

// PlanArg はPlanが確定したargv 1要素である（§16）。
//
// definitionの1個のargs entryを複数argvへ分割せず、pathをliteralやwarning
// parameterへ埋め込まない。
type PlanArg struct {
	// Kind はliteralかpathかである。
	Kind PlanArgKind
	// Value はkind=literalのargv要素である。kind=pathでは空。
	Value string
	// Path はkind=pathのargv要素である。kind=literalではzero。
	Path domain.PathValue
}

// PlanRequiredPath はprobe成功直後に存在を要求するpathである（§16）。
type PlanRequiredPath struct {
	Kind RequiredPathKind
	Path domain.PathValue
}

// PlanSummary はPlanの重要要約である（§16）。
//
// 対象toolがないoperationのprovider/channel/lifecycle/checksum/license fieldは
// 空または`none`とする。
type PlanSummary struct {
	Tool               domain.ToolID
	Version            string
	Platform           domain.Platform
	ProviderKind       ProviderKind
	ProviderName       string
	ProviderRepository string
	ProviderHomepage   string
	ProviderLicense    string
	ProviderRelease    string
	LicenseNotice      string
	Channel            domain.Channel
	Lifecycle          domain.Lifecycle
	ExpectedDigest     domain.Digest
	ChecksumSource     ChecksumSource
	WarningCount       int64
}

// PlanInputs はExecuteが再検査する入力identityである（§16）。
//
// ExecuteはPlanのschema/client/invocationと全inputを、対応する実体から
// 再取得して承認前とlock取得後に再検査する。不一致なら`E_PLAN_STALE`とする。
type PlanInputs struct {
	RootID               string
	ConfigSHA256         string
	ProjectSHA256        string
	DefinitionSHA256     string
	CatalogSHA256        string
	RegistrySHA256       string
	SelectionsRevision   int64
	SetupRevision        int64
	ReceiptIndexRevision int64
}

// SetupPlan はsetup/setup-removeの詳細である（§16）。
type SetupPlan struct {
	Mode                     domain.Mode
	PreviousMode             domain.Mode
	DataRoot                 domain.PathValue
	DistributionRoot         domain.PathValue
	PreviousDataRoot         domain.PathValue
	PreviousDistributionRoot domain.PathValue
	FilesystemCapabilities   []FilesystemCapability
	CurrentLinkStrategy      LinkStrategy
	ShimStrategy             ShimStrategy
	ShimDirectory            domain.PathValue
	PathIntegration          PathIntegration
	Shell                    Shell
	IntegrationTarget        domain.PathValue
	BackupPath               domain.PathValue
	RestartRequired          bool
}

// PlanDownload は1件のdownloadである（§16）。
type PlanDownload struct {
	ID                    string
	ProviderKind          ProviderKind
	ProviderName          string
	ProviderRepository    string
	ProviderHomepage      string
	ProviderRelease       string
	URL                   string
	FileName              string
	Size                  int64
	ExpectedDigest        domain.Digest
	ChecksumSource        ChecksumSource
	License               string
	AdoptionReasonMessage string
	Destination           domain.PathValue
}

// PlanExtract は1件の展開である（§16）。
type PlanExtract struct {
	ID               string
	SourceDownloadID string
	Format           ArchiveFormat
	StripComponents  int64
	Destination      domain.PathValue
}

// PlanProbe は展開済みのvalidation probeである（§16）。
//
// Plan外probeを実行しない。完全version、artifact URL/digest、provider license、
// 理由を空にしない。
type PlanProbe struct {
	ID               string
	RuntimeCommand   string
	Executable       domain.PathValue
	Version          string
	Source           string
	ArtifactDigest   domain.Digest
	License          string
	ReasonMessageID  domain.MessageID
	Args             []PlanArg
	WorkingDirectory domain.PathValue
	WritePaths       []domain.PathValue
	Stream           ProbeStream
	Expect           ProbeExpect
	Regex            string
	ExpectedVersion  string
	ExpectedRoot     *domain.PathValue
	RequiredPaths    []PlanRequiredPath
	TimeoutMillis    int64
	Required         bool
}

// PlanWrite は利用者可視の書込みである（§16）。
//
// staging、download cache、state、receipt、index、shim、storageなどdata root
// 内部の書込みはPlanへ列挙しない。
type PlanWrite struct {
	ID     string
	Action WriteAction
	Target domain.PathValue
}

// PlanStorage はstorageの扱いである（§16）。
type PlanStorage struct {
	ID     string
	Kind   StorageKind
	Scope  StorageScope
	Target domain.PathValue
	Purge  StoragePurge
	Action StorageAction
}

// PlanWarning は事前表示・承認用のwarningである（§16）。
type PlanWarning struct {
	Code                     PlanWarningCode
	MessageID                domain.MessageID
	Parameters               domain.Parameters
	RequiresExplicitApproval bool
}

// IsPlanWarningCode はcodeが§16.1のexactly 8件に含まれるかを返す。
//
// 表はこのpackageが持つため、外のpackageが件数を複製せずに検査できるよう公開する。
func IsPlanWarningCode(code PlanWarningCode) bool {
	_, ok := planWarningApproval[code]
	return ok
}

// ApprovalRequiredCodes は明示承認が必要なcodeをcode順で返す（§16.1）。
//
// 「`requires_explicit_approval=true`のcode集合がApprovalの単位」であり、
// `--yes`が承認できるのはこの集合そのものである（docs/08-install-runtime.md §4）。
// 呼出し側が自分で7件を並べると、表を変えたときに片方だけが古いままになる。
//
// 毎回新しいsliceを返す。呼出し側が書き換えても表に影響しないようにするためである。
func ApprovalRequiredCodes() []PlanWarningCode {
	codes := make([]PlanWarningCode, 0, PlanApprovalCodeCount)
	for code, required := range planWarningApproval {
		if required {
			codes = append(codes, code)
		}
	}
	sort.Slice(codes, func(i, j int) bool { return codes[i] < codes[j] })
	return codes
}

// NewPlanWarning は§16.1の表から承認要否を引いてwarningを作る。
//
// **承認要否をPlan作成側に決めさせないために公開する。** §16.1は
// 「`requires_explicit_approval=true`のcode集合がApprovalの単位」と定めており、
// codeごとの真偽は表が一意に決める。作成側が自分で真偽を置けるようにすると、
// 同じcodeが場面によって承認要否を変えられてしまう。表はこのpackageが持つため、
// 外のpackageが正しい値を得る唯一の経路をここにする。
//
// 未知codeは`requires_explicit_approval=false`のwarningを作らず、codeをそのまま
// 載せて返す。[EncodePlan]が§16.1の8件に無いcodeとして拒否するため、
// 誤りは黙って通らない。
func NewPlanWarning(
	code PlanWarningCode, messageID domain.MessageID, parameters domain.Parameters,
) PlanWarning {
	return PlanWarning{
		Code:                     code,
		MessageID:                messageID,
		Parameters:               parameters,
		RequiresExplicitApproval: planWarningApproval[code],
	}
}

// Plan は変更operationのtyped表現である（§16）。
//
// 永続fileではないが、human表示とapprovalの正本となる。human簡略表示の都合で
// typed fieldを削らない。
type Plan struct {
	ClientVersion string
	Invocation    domain.InvocationID
	Operation     domain.OperationID
	Kind          PlanOperation
	CreatedAt     time.Time
	Summary       PlanSummary
	Setup         *SetupPlan
	Inputs        PlanInputs
	Downloads     []PlanDownload
	Extracts      []PlanExtract
	Probes        []PlanProbe
	Writes        []PlanWrite
	Storage       []PlanStorage
	Warnings      []PlanWarning
}

// ApprovalCodes は明示承認が必要なcode集合を返す（§16.1）。
//
// 「Approvalは`requires_explicit_approval=true`のwarning `code`集合そのもの」
// である。Executeは同じPlan objectのcode集合をApprovalが満たすことを検査する。
func (p Plan) ApprovalCodes() []PlanWarningCode {
	codes := make([]PlanWarningCode, 0, len(p.Warnings))
	for _, warning := range p.Warnings {
		if warning.RequiresExplicitApproval {
			codes = append(codes, warning.Code)
		}
	}
	return codes
}

// ParsePlan はPlan JSONを読む（§16）。
func ParsePlan(data []byte) (Plan, *domain.Error) {
	var file planFile
	if err := decodeJSON(data, &file); err != nil {
		return Plan{}, planError(err)
	}
	value, err := buildPlan(file)
	if err != nil {
		return Plan{}, planError(err)
	}
	return value, nil
}

// EncodePlan はPlanをJSONへ書き出す（§16）。
func EncodePlan(value Plan) ([]byte, *domain.Error) {
	file, err := planFileOf(value)
	if err != nil {
		return nil, planError(err)
	}
	if _, err := buildPlan(file); err != nil {
		return nil, planError(err)
	}
	data, encodeErr := encodeJSON(file)
	if encodeErr != nil {
		return nil, planError(encodeErr)
	}
	return data, nil
}

// planError はPlanの組立て失敗を表す。
//
// Planはgdtvm自身が作るものであり、契約に合わないのは内部誤りである。
func planError(cause error) *domain.Error {
	return typedError(domain.CodeInternal, "plan.invalid", "", cause)
}

func buildPlan(file planFile) (Plan, error) {
	var value Plan
	if err := requireSchema("schema", file.Schema); err != nil {
		return value, err
	}
	clientVersion, err := requireClientVersion("client_version", file.ClientVersion)
	if err != nil {
		return value, err
	}
	invocationText, err := requirePresent("invocation_id", file.InvocationID)
	if err != nil {
		return value, err
	}
	invocation, err := domain.ParseInvocationID(invocationText)
	if err != nil {
		return value, fmt.Errorf("invocation_id: %w", err)
	}
	operationText, err := requirePresent("operation_id", file.OperationID)
	if err != nil {
		return value, err
	}
	operation, err := domain.ParseOperationID(operationText)
	if err != nil {
		return value, fmt.Errorf("operation_id: %w", err)
	}
	kind, err := requireEnum("operation", file.Operation, planOperations)
	if err != nil {
		return value, err
	}
	createdAt, err := requireTimestampField("created_at", file.CreatedAt)
	if err != nil {
		return value, err
	}
	if file.Summary == nil {
		return value, fmt.Errorf("summaryが無い")
	}
	summary, err := buildPlanSummary(*file.Summary, kind)
	if err != nil {
		return value, err
	}
	if file.Inputs == nil {
		return value, fmt.Errorf("inputsが無い")
	}
	inputs, err := buildPlanInputs(*file.Inputs)
	if err != nil {
		return value, err
	}
	warnings, err := buildPlanWarnings(file.Warnings)
	if err != nil {
		return value, err
	}
	// §16が「`warning_count`は`warnings`の件数と一致させる」と定める。
	// 一致しないと、要約だけを見た利用者が警告を見落とす。
	if summary.WarningCount != int64(len(warnings)) {
		return value, fmt.Errorf(
			"summary.warning_countが%dだがwarningsは%d件", summary.WarningCount, len(warnings))
	}
	setup, err := buildSetupPlan(file.Setup, kind, warnings)
	if err != nil {
		return value, err
	}
	downloads, downloadIDs, err := buildPlanDownloads(file.Downloads)
	if err != nil {
		return value, err
	}
	extracts, err := buildPlanExtracts(file.Extracts, downloadIDs)
	if err != nil {
		return value, err
	}
	probes, err := buildPlanProbes(file.Probes)
	if err != nil {
		return value, err
	}
	writes, err := buildPlanWrites(file.Writes)
	if err != nil {
		return value, err
	}
	storage, err := buildPlanStorage(file.Storage)
	if err != nil {
		return value, err
	}
	if err := requireUniquePlanIDs(downloads, extracts, probes, writes, storage); err != nil {
		return value, err
	}
	return Plan{
		ClientVersion: clientVersion, Invocation: invocation, Operation: operation,
		Kind: kind, CreatedAt: createdAt, Summary: summary, Setup: setup,
		Inputs: inputs, Downloads: downloads, Extracts: extracts, Probes: probes,
		Writes: writes, Storage: storage, Warnings: warnings,
	}, nil
}

// requireUniquePlanIDs は§16の「IDはPlan内で種類をまたいで一意」を確かめる。
//
// 種類をまたいで一意にするのは、Plan表示とlogでIDだけを見て対象を特定できる
// ようにするためである。同じIDのdownloadとwriteがあると、どちらの話か決まらない。
func requireUniquePlanIDs(
	downloads []PlanDownload, extracts []PlanExtract, probes []PlanProbe,
	writes []PlanWrite, storage []PlanStorage,
) error {
	seen := make(map[string]string)
	add := func(kind, id string) error {
		if previous, duplicate := seen[id]; duplicate {
			return fmt.Errorf("Plan ID %q が%sと%sで重複している", id, previous, kind)
		}
		seen[id] = kind
		return nil
	}
	for _, entry := range downloads {
		if err := add("downloads", entry.ID); err != nil {
			return err
		}
	}
	for _, entry := range extracts {
		if err := add("extracts", entry.ID); err != nil {
			return err
		}
	}
	for _, entry := range probes {
		if err := add("probes", entry.ID); err != nil {
			return err
		}
	}
	for _, entry := range writes {
		if err := add("writes", entry.ID); err != nil {
			return err
		}
	}
	for _, entry := range storage {
		if err := add("storage", entry.ID); err != nil {
			return err
		}
	}
	return nil
}
