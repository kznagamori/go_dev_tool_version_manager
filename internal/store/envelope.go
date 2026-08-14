package store

import (
	"fmt"
	"time"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
)

// ResultWarningCode は処理結果用のwarning codeである（docs/04-storage-and-data.md §16.2）。
//
// Plan approvalには使わない。security failureをresult warningへ格下げしない。
type ResultWarningCode string

// ResultWarningCode のexactly 5件（§16.2）。
const (
	WarnCacheStale                  ResultWarningCode = "W_CACHE_STALE"
	WarnCleanupIncomplete           ResultWarningCode = "W_CLEANUP_INCOMPLETE"
	WarnSelectionLinkInconsistent   ResultWarningCode = "W_SELECTION_LINK_INCONSISTENT"
	WarnEnvironmentNotificationFail ResultWarningCode = "W_ENVIRONMENT_NOTIFICATION_FAILED"
	WarnLifecycleOverrideUnused     ResultWarningCode = "W_LIFECYCLE_OVERRIDE_UNUSED"
)

var resultWarningCodes = map[ResultWarningCode]struct{}{
	WarnCacheStale: {}, WarnCleanupIncomplete: {}, WarnSelectionLinkInconsistent: {},
	WarnEnvironmentNotificationFail: {}, WarnLifecycleOverrideUnused: {},
}

// ResultWarningCodeCount は§16.2が定めるcode数である。
const ResultWarningCodeCount = 5

// JSONCommand は`--json`を持つ読取り専用commandである（§17）。
type JSONCommand string

// JSONCommand のexactly 5値。§17が「`--json`は読取り専用5 command
// （`available|installed|current|doctor|version`）だけが持つ」と定める。
const (
	CommandAvailable JSONCommand = "available"
	CommandInstalled JSONCommand = "installed"
	CommandCurrent   JSONCommand = "current"
	CommandDoctor    JSONCommand = "doctor"
	CommandVersion   JSONCommand = "version"
)

var jsonCommands = map[JSONCommand]struct{}{
	CommandAvailable: {}, CommandInstalled: {}, CommandCurrent: {},
	CommandDoctor: {}, CommandVersion: {},
}

// Severity はdiagnosticの重大度である（§17.1）。
type Severity string

// Severity のexactly 3値。
const (
	SeverityError Severity = "error"
	SeverityWarn  Severity = "warn"
	SeverityInfo  Severity = "info"
)

var severities = map[Severity]struct{}{
	SeverityError: {}, SeverityWarn: {}, SeverityInfo: {},
}

// DoctorStatus は`doctor`の総合判定である（§17.1）。
type DoctorStatus string

// DoctorStatus のexactly 3値。
const (
	DoctorHealthy   DoctorStatus = "healthy"
	DoctorDegraded  DoctorStatus = "degraded"
	DoctorUnhealthy DoctorStatus = "unhealthy"
)

var doctorStatuses = map[DoctorStatus]struct{}{
	DoctorHealthy: {}, DoctorDegraded: {}, DoctorUnhealthy: {},
}

// DiagnosticCode は`doctor`の診断項目である（§17.1）。
type DiagnosticCode string

// DiagnosticCode のexactly 10件。§17.1が「上表の10件をcode順にexactly 1件ずつ
// 返す」と定める。
const (
	DiagRoot      DiagnosticCode = "D_ROOT"
	DiagState     DiagnosticCode = "D_STATE"
	DiagRegistry  DiagnosticCode = "D_REGISTRY"
	DiagReceipt   DiagnosticCode = "D_RECEIPT"
	DiagPayload   DiagnosticCode = "D_PAYLOAD"
	DiagSelection DiagnosticCode = "D_SELECTION"
	DiagShim      DiagnosticCode = "D_SHIM"
	DiagPath      DiagnosticCode = "D_PATH"
	DiagStorage   DiagnosticCode = "D_STORAGE"
	DiagTmp       DiagnosticCode = "D_TMP"
)

// diagnosticCodeOrder は§17.1のcode順である。
//
// 「code順にexactly 1件ずつ」を検査するため、集合ではなく順序付きで持つ。
// ASCII byte順であり、表の並びとも一致する。
var diagnosticCodeOrder = []DiagnosticCode{
	DiagPath, DiagPayload, DiagReceipt, DiagRegistry, DiagRoot,
	DiagSelection, DiagShim, DiagState, DiagStorage, DiagTmp,
}

// DiagnosticCodeCount は§17.1が定める診断項目数である。
const DiagnosticCodeCount = 10

// ResultWarning は結果に添えるwarningである（§17）。
type ResultWarning struct {
	// Code は§16.2の閉じた5件のいずれかである。
	Code ResultWarningCode
	// MessageID は表示文を引くkeyである。
	MessageID domain.MessageID
	// Parameters はpathを含まないscalar mapである。
	Parameters domain.Parameters
}

// ResultError は失敗envelopeのerror objectである（§17）。
type ResultError struct {
	// Code はstable error codeである。
	Code domain.ErrorCode
	// MessageID は表示文を引くkeyである。
	MessageID domain.MessageID
	// Parameters はpathを含まないscalar mapである。
	Parameters domain.Parameters
	// Retryable は利用者が状態を直したあと再実行できるかである。
	Retryable bool
}

// InstallSummary は`installed`の1 entryである（§17）。
type InstallSummary struct {
	Ref         InstallRef
	InstallID   string
	InstalledAt time.Time
	Health      domain.Health
	ReceiptPath domain.PathValue
	DiskSize    int64
	Provider    ProviderKind
	Selected    bool
}

// SelectionSummary は`current`の1 entryである（§17）。
//
// `source`はeffective selectionの由来であり、requestの`scope`とは別のenumである
// （§17.1）。
type SelectionSummary struct {
	Source      domain.SelectionSource
	ProjectFile domain.PathValue
	Tool        domain.ToolID
	Version     string
	InstallID   string
	PayloadPath domain.PathValue
	Health      domain.Health
}

// Diagnostic は`doctor`の1項目である（§17）。
type Diagnostic struct {
	Code       DiagnosticCode
	Severity   Severity
	MessageID  domain.MessageID
	Parameters domain.Parameters
	Paths      []domain.PathValue
}

// BuildResult は`version`のbuild情報である（§17）。
type BuildResult struct {
	Version          string
	Commit           string
	BuildTime        time.Time
	GoVersion        string
	Platform         domain.Platform
	StateSchema      int64
	DefinitionSchema int64
	RegistrySchema   int64
	Development      bool
}

// AvailableData は`available`の`data`である（§17）。
type AvailableData struct {
	Tool     domain.ToolID
	Platform domain.Platform
	Items    []CatalogItem
}

// DoctorData は`doctor`の`data`である（§17）。
type DoctorData struct {
	Status      DoctorStatus
	Diagnostics []Diagnostic
	ReportPath  domain.PathValue
}

// Envelope はCLI JSON documentのtyped表現である（§17）。
//
// `--json`は読取り専用5 commandだけが持ち、stdoutは完了時のexactly 1 JSON
// documentとする。data/errorは排他である。
type Envelope struct {
	// OK は成功かどうかである。data/errorのどちらを持つかを決める。
	OK bool
	// Command は対象commandである。
	Command JSONCommand
	// Invocation はCLI呼出しの識別子である。
	Invocation domain.InvocationID
	// Available は`available`成功時のdataである。
	Available *AvailableData
	// Installs は`installed`成功時のdataである。
	Installs []InstallSummary
	// Selections は`current`成功時のdataである。
	Selections []SelectionSummary
	// Doctor は`doctor`成功時のdataである。
	Doctor *DoctorData
	// Build は`version`成功時のdataである。
	Build *BuildResult
	// Error は失敗時のerror objectである。
	Error *ResultError
	// Warnings は§16.2のresult warningである。
	Warnings []ResultWarning
}

// EncodeEnvelope はCLI JSON documentを書き出す（§17）。
//
// 表示済みmessage、Go error、stack、secretをJSONへ入れない。human/JSONは
// 同じtyped Resultから生成する。
func EncodeEnvelope(value Envelope) ([]byte, *domain.Error) {
	file, err := envelopeFileOf(value)
	if err != nil {
		return nil, envelopeError(err)
	}
	if _, err := buildEnvelope(file); err != nil {
		return nil, envelopeError(err)
	}
	data, encodeErr := encodeJSON(file)
	if encodeErr != nil {
		return nil, envelopeError(encodeErr)
	}
	return data, nil
}

// DecodeEnvelope はCLI JSON documentを読む（§17）。
//
// 出力側の契約をtestとdoctorが読み直せるようにする。書けない形のdocumentを
// 読めてしまうと、出力側の検査が空振りする。
func DecodeEnvelope(data []byte) (Envelope, *domain.Error) {
	var file envelopeFile
	if err := decodeJSON(data, &file); err != nil {
		return Envelope{}, envelopeError(err)
	}
	value, err := buildEnvelope(file)
	if err != nil {
		return Envelope{}, envelopeError(err)
	}
	return value, nil
}

// envelopeError はCLI JSON documentの組立て失敗を表す。
//
// envelopeはgdtvm自身が作るものであり、内容が契約に合わないのは内部誤りである。
// 利用者の入力起因ではないため`E_INTERNAL`とする。
func envelopeError(cause error) *domain.Error {
	return typedError(domain.CodeInternal, "cli.envelope_invalid", "", cause)
}

func buildEnvelope(file envelopeFile) (Envelope, error) {
	var value Envelope
	if err := requireSchema("schema", file.Schema); err != nil {
		return value, err
	}
	ok, err := requireBool("ok", file.OK)
	if err != nil {
		return value, err
	}
	command, err := requireEnum("command", file.Command, jsonCommands)
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
	// §17が「data/errorを排他にする」と定める。両方あるdocumentは、成功と失敗の
	// どちらとして扱えばよいか決まらない。
	switch {
	case ok && file.Error != nil:
		return value, fmt.Errorf("ok=trueなのにerrorがある")
	case ok && file.Data == nil:
		return value, fmt.Errorf("ok=trueなのにdataが無い")
	case !ok && file.Data != nil:
		return value, fmt.Errorf("ok=falseなのにdataがある")
	case !ok && file.Error == nil:
		return value, fmt.Errorf("ok=falseなのにerrorが無い")
	}
	warnings, err := buildResultWarnings(file.Warnings)
	if err != nil {
		return value, err
	}
	value = Envelope{OK: ok, Command: command, Invocation: invocation, Warnings: warnings}
	if !ok {
		resultError, err := buildResultError(*file.Error)
		if err != nil {
			return Envelope{}, err
		}
		value.Error = &resultError
		return value, nil
	}
	if err := applyEnvelopeData(&value, command, *file.Data); err != nil {
		return Envelope{}, err
	}
	return value, nil
}

// applyEnvelopeData はcommandごとの`data`のexact keyを読む（§17）。
//
// commandに対応しないkeyがあるdocumentを拒否する。`installed`のdocumentに
// `selections`が入っていると、読み手がどちらを正とするか決まらない。
func applyEnvelopeData(value *Envelope, command JSONCommand, data envelopeData) error {
	if err := checkDataKeys(command, data); err != nil {
		return err
	}
	switch command {
	case CommandAvailable:
		available, err := buildAvailableData(data)
		if err != nil {
			return err
		}
		value.Available = &available
	case CommandInstalled:
		installs, err := buildInstallSummaries(*data.Installs)
		if err != nil {
			return err
		}
		value.Installs = installs
	case CommandCurrent:
		selections, err := buildSelectionSummaries(*data.Selections)
		if err != nil {
			return err
		}
		value.Selections = selections
	case CommandDoctor:
		doctor, err := buildDoctorData(data)
		if err != nil {
			return err
		}
		value.Doctor = &doctor
	case CommandVersion:
		build, err := buildBuildResult(*data.Build)
		if err != nil {
			return err
		}
		value.Build = &build
	}
	return nil
}

// checkDataKeys はcommandに対応するkeyだけがあることを確かめる（§17）。
func checkDataKeys(command JSONCommand, data envelopeData) error {
	present := map[string]bool{
		"tool_id":     data.ToolID != nil,
		"platform_id": data.PlatformID != nil,
		"items":       data.Items != nil,
		"installs":    data.Installs != nil,
		"selections":  data.Selections != nil,
		"status":      data.Status != nil,
		"diagnostics": data.Diagnostics != nil,
		"report_path": data.ReportPath != nil,
		"build":       data.Build != nil,
	}
	want := map[JSONCommand][]string{
		CommandAvailable: {"tool_id", "platform_id", "items"},
		CommandInstalled: {"installs"},
		CommandCurrent:   {"selections"},
		CommandDoctor:    {"status", "diagnostics", "report_path"},
		CommandVersion:   {"build"},
	}[command]

	expected := make(map[string]struct{}, len(want))
	for _, key := range want {
		expected[key] = struct{}{}
		if !present[key] {
			return fmt.Errorf("command %q のdataに %q が無い", command, key)
		}
	}
	for key, exists := range present {
		if _, allowed := expected[key]; exists && !allowed {
			return fmt.Errorf("command %q のdataに %q は現れない", command, key)
		}
	}
	return nil
}

func buildAvailableData(data envelopeData) (AvailableData, error) {
	var value AvailableData
	tool, err := requireToolID("data.tool_id", data.ToolID)
	if err != nil {
		return value, err
	}
	platformText, err := requirePresent("data.platform_id", data.PlatformID)
	if err != nil {
		return value, err
	}
	platform, err := domain.ParsePlatform(platformText)
	if err != nil {
		return value, fmt.Errorf("data.platform_id: %w", err)
	}
	// itemのschemeはcatalogと同じくdefinitionが決める。`available`は既に
	// 解決済みのcatalog itemを表示するだけなので、順序契約はcatalog側で
	// 検査済みである。ここではentryのkeyと値の形だけを見る。
	items := make([]CatalogItem, 0, len(*data.Items))
	for index, raw := range *data.Items {
		if raw == nil {
			return value, fmt.Errorf("data.items[%d]が空", index)
		}
		item, err := buildDisplayCatalogItem(index, *raw)
		if err != nil {
			return value, err
		}
		items = append(items, item)
	}
	return AvailableData{Tool: tool, Platform: platform, Items: items}, nil
}

func buildDoctorData(data envelopeData) (DoctorData, error) {
	var value DoctorData
	status, err := requireEnum("data.status", data.Status, doctorStatuses)
	if err != nil {
		return value, err
	}
	diagnostics, err := buildDiagnostics(*data.Diagnostics)
	if err != nil {
		return value, err
	}
	// §17が「`doctor`の`report_path.role`は常に`report`、`path`は`--report`
	// 指定時だけ非空」と定める。
	reportPath, err := buildPathValue("data.report_path", data.ReportPath, domain.RoleReport, pathOptional)
	if err != nil {
		return value, err
	}
	if err := checkDoctorStatus(status, diagnostics); err != nil {
		return value, err
	}
	return DoctorData{Status: status, Diagnostics: diagnostics, ReportPath: reportPath}, nil
}

// checkDoctorStatus は§17.1のstatus導出規則を確かめる。
//
// 「errorが1件以上なら`unhealthy`、errorなしでwarnが1件以上なら`degraded`、
// それ以外は`healthy`」。statusとdiagnosticsが食い違うdocumentを出すと、
// 利用者が総合判定と個別項目のどちらを信じればよいか決まらない。
func checkDoctorStatus(status DoctorStatus, diagnostics []Diagnostic) error {
	errors, warns := 0, 0
	for _, diagnostic := range diagnostics {
		switch diagnostic.Severity {
		case SeverityError:
			errors++
		case SeverityWarn:
			warns++
		}
	}
	want := DoctorHealthy
	switch {
	case errors > 0:
		want = DoctorUnhealthy
	case warns > 0:
		want = DoctorDegraded
	}
	if status != want {
		return fmt.Errorf(
			"data.statusが%qだがdiagnosticsからは%qになる（error %d件、warn %d件）",
			status, want, errors, warns)
	}
	return nil
}

func buildDiagnostics(raws []*diagnosticJSON) ([]Diagnostic, error) {
	entries := make([]Diagnostic, 0, len(raws))
	for index, raw := range raws {
		if raw == nil {
			return nil, fmt.Errorf("data.diagnostics[%d]が空", index)
		}
		entry, err := buildDiagnostic(index, *raw)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	// §17.1が「上表の10件をcode順にexactly 1件ずつ返す」と定める。件数が
	// 足りないdoctor結果は、検査していない項目を「問題なし」に見せてしまう。
	if len(entries) != DiagnosticCodeCount {
		return nil, fmt.Errorf(
			"data.diagnosticsは%d件でなければならない（%d件）", DiagnosticCodeCount, len(entries))
	}
	for index, want := range diagnosticCodeOrder {
		if entries[index].Code != want {
			return nil, fmt.Errorf(
				"data.diagnostics[%d]のcodeが%qでなければならない（%q）", index, want, entries[index].Code)
		}
	}
	return entries, nil
}

func buildDiagnostic(index int, raw diagnosticJSON) (Diagnostic, error) {
	var value Diagnostic
	prefix := fmt.Sprintf("data.diagnostics[%d]", index)
	codeText, err := requirePresent(prefix+".code", raw.Code)
	if err != nil {
		return value, err
	}
	code := DiagnosticCode(codeText)
	if !isKnownDiagnosticCode(code) {
		return value, fmt.Errorf("%s.codeが許可された値でない（%q）", prefix, codeText)
	}
	severity, err := requireEnum(prefix+".severity", raw.Severity, severities)
	if err != nil {
		return value, err
	}
	messageID, err := requireMessageID(prefix+".message_id", raw.MessageID)
	if err != nil {
		return value, err
	}
	parameters, err := requireScalarMap(prefix+".parameters", raw.Parameters)
	if err != nil {
		return value, err
	}
	if raw.Paths == nil {
		return value, fmt.Errorf("%s.pathsが無い", prefix)
	}
	paths := make([]domain.PathValue, 0, len(*raw.Paths))
	for pathIndex, rawPath := range *raw.Paths {
		// 個別のroleは対象によって変わるため固定しない。§17.2の22値のいずれかで
		// あることと、absolute pathであることだけを見る。
		path, err := buildPathValue(
			fmt.Sprintf("%s.paths[%d]", prefix, pathIndex), rawPath, "", pathLocatorOrAbsolute)
		if err != nil {
			return value, err
		}
		paths = append(paths, path)
	}
	return Diagnostic{
		Code: code, Severity: severity, MessageID: messageID,
		Parameters: parameters, Paths: paths,
	}, nil
}

func isKnownDiagnosticCode(code DiagnosticCode) bool {
	for _, known := range diagnosticCodeOrder {
		if known == code {
			return true
		}
	}
	return false
}

func buildInstallSummaries(raws []*installSummaryJSON) ([]InstallSummary, error) {
	entries := make([]InstallSummary, 0, len(raws))
	for index, raw := range raws {
		if raw == nil {
			return nil, fmt.Errorf("data.installs[%d]が空", index)
		}
		prefix := fmt.Sprintf("data.installs[%d]", index)
		ref, err := requireInstallRef(prefix, raw.ToolID, raw.Version, raw.PlatformID)
		if err != nil {
			return nil, err
		}
		installID, err := requireIDField(prefix+".install_id", raw.InstallID)
		if err != nil {
			return nil, err
		}
		installedAt, err := requireTimestampField(prefix+".installed_at", raw.InstalledAt)
		if err != nil {
			return nil, err
		}
		healthText, err := requirePresent(prefix+".health", raw.Health)
		if err != nil {
			return nil, err
		}
		health, err := domain.ParseHealth(healthText)
		if err != nil {
			return nil, fmt.Errorf("%s.health: %w", prefix, err)
		}
		receiptPath, err := buildPathValue(
			prefix+".receipt_path", raw.ReceiptPath, domain.RoleReceipt, pathAbsolute)
		if err != nil {
			return nil, err
		}
		diskSize, err := requireInt64(prefix+".disk_size", raw.DiskSize)
		if err != nil {
			return nil, err
		}
		provider, err := requireEnum(prefix+".provider_kind", raw.ProviderKind, receiptProviderKinds)
		if err != nil {
			return nil, err
		}
		selected, err := requireBool(prefix+".selected", raw.Selected)
		if err != nil {
			return nil, err
		}
		entries = append(entries, InstallSummary{
			Ref: ref, InstallID: installID, InstalledAt: installedAt, Health: health,
			ReceiptPath: receiptPath, DiskSize: diskSize, Provider: provider, Selected: selected,
		})
	}
	// §17が「listはtool ID、version比較、diagnostic codeの各規則で決定的にsort」と
	// 定める。version比較はschemeを要するため、tupleのbyte順までを検査する。
	// scheme込みの比較はcatalogが担当する（P2-04 2/3の判断と同じ分担）。
	if err := requireSortedUnique("data.installs", len(entries), func(i int) string {
		return entries[i].Ref.SortKey()
	}); err != nil {
		return nil, err
	}
	return entries, nil
}

func buildSelectionSummaries(raws []*selectionSummaryJSON) ([]SelectionSummary, error) {
	entries := make([]SelectionSummary, 0, len(raws))
	for index, raw := range raws {
		if raw == nil {
			return nil, fmt.Errorf("data.selections[%d]が空", index)
		}
		entry, err := buildSelectionSummary(index, *raw)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	if err := requireSortedUnique("data.selections", len(entries), func(i int) string {
		return entries[i].Tool.String()
	}); err != nil {
		return nil, err
	}
	return entries, nil
}

func buildSelectionSummary(index int, raw selectionSummaryJSON) (SelectionSummary, error) {
	var value SelectionSummary
	prefix := fmt.Sprintf("data.selections[%d]", index)
	sourceText, err := requirePresent(prefix+".source", raw.Source)
	if err != nil {
		return value, err
	}
	source, err := domain.ParseSelectionSource(sourceText)
	if err != nil {
		return value, fmt.Errorf("%s.source: %w", prefix, err)
	}
	// §17が「`SelectionSummary.project_file.role`は`project-file`、sourceが
	// project以外ならpathを空にする」と定める。
	projectFile, err := buildPathValue(
		prefix+".project_file", raw.ProjectFile, domain.RoleProjectFile, pathOptional)
	if err != nil {
		return value, err
	}
	if source != domain.SelectionSourceProject && projectFile.Path() != "" {
		return value, fmt.Errorf("%s: source=%qならproject_fileのpathは空", prefix, source)
	}
	if source == domain.SelectionSourceProject && projectFile.Path() == "" {
		return value, fmt.Errorf("%s: source=projectならproject_fileのpathが必要", prefix)
	}
	tool, err := requireToolID(prefix+".tool_id", raw.ToolID)
	if err != nil {
		return value, err
	}
	versionText, err := requirePresent(prefix+".version", raw.Version)
	if err != nil {
		return value, err
	}
	installID, err := requirePresent(prefix+".install_id", raw.InstallID)
	if err != nil {
		return value, err
	}
	// §17が「sourceがnoneの`payload_path`はrole=`payload`かつpath空」と定める。
	payloadPath, err := buildPathValue(
		prefix+".payload_path", raw.PayloadPath, domain.RolePayload, pathOptional)
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
	if source == domain.SelectionSourceNone {
		if err := requireEmpty(prefix+".version", versionText); err != nil {
			return value, err
		}
		if err := requireEmpty(prefix+".install_id", installID); err != nil {
			return value, err
		}
		if payloadPath.Path() != "" {
			return value, fmt.Errorf("%s: source=noneならpayload_pathのpathは空", prefix)
		}
	} else {
		if err := requireExactVersion(prefix+".version", versionText); err != nil {
			return value, err
		}
		if _, err := requireIDField(prefix+".install_id", raw.InstallID); err != nil {
			return value, err
		}
	}
	return SelectionSummary{
		Source: source, ProjectFile: projectFile, Tool: tool, Version: versionText,
		InstallID: installID, PayloadPath: payloadPath, Health: health,
	}, nil
}

func buildBuildResult(raw buildResultJSON) (BuildResult, error) {
	var value BuildResult
	version, err := requirePresent("data.build.version", raw.Version)
	if err != nil {
		return value, err
	}
	if _, err := requireClientVersion("data.build.version", raw.Version); err != nil {
		return value, err
	}
	commit, err := requirePresent("data.build.commit", raw.Commit)
	if err != nil {
		return value, err
	}
	if !clientCommitRe.MatchString(commit) {
		return value, fmt.Errorf("data.build.commitが40桁lowercase hexでない（%q）", commit)
	}
	buildTimeText, err := requirePresent("data.build.build_time", raw.BuildTime)
	if err != nil {
		return value, err
	}
	buildTime, err := parseTimestamp("data.build.build_time", buildTimeText)
	if err != nil {
		return value, err
	}
	goVersion, err := requireNonEmpty("data.build.go_version", raw.GoVersion)
	if err != nil {
		return value, err
	}
	platformText, err := requirePresent("data.build.platform_id", raw.PlatformID)
	if err != nil {
		return value, err
	}
	platform, err := domain.ParsePlatform(platformText)
	if err != nil {
		return value, fmt.Errorf("data.build.platform_id: %w", err)
	}
	for _, pair := range []struct {
		name  string
		value *int64
	}{
		{"data.build.state_schema", raw.StateSchema},
		{"data.build.definition_schema", raw.DefinitionSchema},
		{"data.build.registry_schema", raw.RegistrySchema},
	} {
		if err := requireSchema(pair.name, pair.value); err != nil {
			return value, err
		}
	}
	development, err := requireBool("data.build.development", raw.Development)
	if err != nil {
		return value, err
	}
	// developmentとversionは同値である。`devel`なのにdevelopment=falseだと、
	// releaseされていないbinaryをrelease版として報告することになる。
	if development != (version == DevelopmentClientVersion) {
		return value, fmt.Errorf(
			"data.build.developmentが%vだがversionは%qである", development, version)
	}
	return BuildResult{
		Version: version, Commit: commit, BuildTime: buildTime, GoVersion: goVersion,
		Platform: platform, StateSchema: SchemaVersion, DefinitionSchema: SchemaVersion,
		RegistrySchema: SchemaVersion, Development: development,
	}, nil
}

func buildResultError(raw resultErrorJSON) (ResultError, error) {
	var value ResultError
	codeText, err := requirePresent("error.code", raw.Code)
	if err != nil {
		return value, err
	}
	code, err := domain.ParseErrorCode(codeText)
	if err != nil {
		return value, fmt.Errorf("error.code: %w", err)
	}
	messageID, err := requireMessageID("error.message_id", raw.MessageID)
	if err != nil {
		return value, err
	}
	parameters, err := requireScalarMap("error.parameters", raw.Parameters)
	if err != nil {
		return value, err
	}
	retryable, err := requireBool("error.retryable", raw.Retryable)
	if err != nil {
		return value, err
	}
	// docs/02-architecture.md §14の非retryable codeにretryable=trueを載せない。
	// 「再実行できる」と表示された失敗が実際には何度やっても直らないためである。
	if retryable && !domain.IsRetryableAllowed(code) {
		return value, fmt.Errorf("error.code %q はretryable=trueにできない", code)
	}
	return ResultError{
		Code: code, MessageID: messageID, Parameters: parameters, Retryable: retryable,
	}, nil
}

func buildResultWarnings(raws []*resultWarningJSON) ([]ResultWarning, error) {
	if raws == nil {
		return nil, fmt.Errorf("warningsが無い")
	}
	entries := make([]ResultWarning, 0, len(raws))
	for index, raw := range raws {
		if raw == nil {
			return nil, fmt.Errorf("warnings[%d]が空", index)
		}
		prefix := fmt.Sprintf("warnings[%d]", index)
		code, err := requireEnum(prefix+".code", raw.Code, resultWarningCodes)
		if err != nil {
			return nil, err
		}
		messageID, err := requireMessageID(prefix+".message_id", raw.MessageID)
		if err != nil {
			return nil, err
		}
		parameters, err := requireScalarMap(prefix+".parameters", raw.Parameters)
		if err != nil {
			return nil, err
		}
		entries = append(entries, ResultWarning{
			Code: code, MessageID: messageID, Parameters: parameters,
		})
	}
	return entries, nil
}

// requireMessageID は必須message IDを読む。
func requireMessageID(field string, raw *string) (domain.MessageID, error) {
	text, err := requirePresent(field, raw)
	if err != nil {
		return domain.MessageID{}, err
	}
	value, err := domain.ParseMessageID(text)
	if err != nil {
		return domain.MessageID{}, fmt.Errorf("%s: %w", field, err)
	}
	return value, nil
}

// requireScalarMap は必須のscalar parameter mapを読む（§7）。
//
// §17が「parametersはstring/bool/integer/nullのmapだけ」「pathを含まない
// scalar map」と定める。keyの欠落と空mapを区別するためpointerで受ける。
func requireScalarMap(field string, raw *map[string]any) (domain.Parameters, error) {
	if raw == nil {
		return nil, fmt.Errorf("%sが無い", field)
	}
	parameters, err := decodeScalarMap(*raw, domain.ParameterKeyMaxLength)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", field, err)
	}
	return parameters, nil
}
