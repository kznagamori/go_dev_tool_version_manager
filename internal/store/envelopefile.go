package store

import (
	"fmt"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
)

// envelopeFile は§17のtop-level key集合である。
//
// `data`と`error`はどちらか一方だけが現れる。両方をpointerで持ち、
// [buildEnvelope]が排他を検査する。
type envelopeFile struct {
	Schema       *int64               `json:"schema"`
	OK           *bool                `json:"ok"`
	Command      *string              `json:"command"`
	InvocationID *string              `json:"invocation_id"`
	Data         *envelopeData        `json:"data,omitempty"`
	Error        *resultErrorJSON     `json:"error,omitempty"`
	Warnings     []*resultWarningJSON `json:"warnings"`
}

// envelopeData は5 commandの`data`を合わせたkey集合である。
//
// commandごとに現れるkeyは[checkDataKeys]が固定する。1つの構造体にまとめるのは、
// `encoding/json`のDisallowUnknownFieldsを効かせるためである。commandごとに
// 別型にするとdecode前にcommandを読む必要があり、strict decodeが2 passになる。
type envelopeData struct {
	ToolID      *string                  `json:"tool_id,omitempty"`
	PlatformID  *string                  `json:"platform_id,omitempty"`
	Items       *[]*catalogItemJSON      `json:"items,omitempty"`
	Installs    *[]*installSummaryJSON   `json:"installs,omitempty"`
	Selections  *[]*selectionSummaryJSON `json:"selections,omitempty"`
	Status      *string                  `json:"status,omitempty"`
	Diagnostics *[]*diagnosticJSON       `json:"diagnostics,omitempty"`
	ReportPath  *pathValueJSON           `json:"report_path,omitempty"`
	Build       *buildResultJSON         `json:"build,omitempty"`
}

type resultErrorJSON struct {
	Code       *string         `json:"code"`
	MessageID  *string         `json:"message_id"`
	Parameters *map[string]any `json:"parameters"`
	Retryable  *bool           `json:"retryable"`
}

type resultWarningJSON struct {
	Code       *string         `json:"code"`
	MessageID  *string         `json:"message_id"`
	Parameters *map[string]any `json:"parameters"`
}

type installSummaryJSON struct {
	ToolID       *string        `json:"tool_id"`
	Version      *string        `json:"version"`
	PlatformID   *string        `json:"platform_id"`
	InstallID    *string        `json:"install_id"`
	InstalledAt  *string        `json:"installed_at"`
	Health       *string        `json:"health"`
	ReceiptPath  *pathValueJSON `json:"receipt_path"`
	DiskSize     *int64         `json:"disk_size"`
	ProviderKind *string        `json:"provider_kind"`
	Selected     *bool          `json:"selected"`
}

type selectionSummaryJSON struct {
	Source      *string        `json:"source"`
	ProjectFile *pathValueJSON `json:"project_file"`
	ToolID      *string        `json:"tool_id"`
	Version     *string        `json:"version"`
	InstallID   *string        `json:"install_id"`
	PayloadPath *pathValueJSON `json:"payload_path"`
	Health      *string        `json:"health"`
}

type diagnosticJSON struct {
	Code       *string           `json:"code"`
	Severity   *string           `json:"severity"`
	MessageID  *string           `json:"message_id"`
	Parameters *map[string]any   `json:"parameters"`
	Paths      *[]*pathValueJSON `json:"paths"`
}

type buildResultJSON struct {
	Version          *string `json:"version"`
	Commit           *string `json:"commit"`
	BuildTime        *string `json:"build_time"`
	GoVersion        *string `json:"go_version"`
	PlatformID       *string `json:"platform_id"`
	StateSchema      *int64  `json:"state_schema"`
	DefinitionSchema *int64  `json:"definition_schema"`
	RegistrySchema   *int64  `json:"registry_schema"`
	Development      *bool   `json:"development"`
}

// buildDisplayCatalogItem は`available`の`data.items`を読む。
//
// §17が「`CatalogItem`は§15 itemのexact key集合」と定めるため、catalogと同じ
// 構造体で読む。versionだけ扱いが違い、[CatalogItem.VersionText]へtextで入れて
// [CatalogItem.Version]はzeroのままにする。CLI JSONにはschemeが無く、
// catalog（§15）と違ってschemeを一意にできないためである。順序契約はcatalogを
// 読んだ時点で検査済みであり、§17は再掲していない。
func buildDisplayCatalogItem(index int, raw catalogItemJSON) (CatalogItem, error) {
	prefix := fmt.Sprintf("data.items[%d]", index)
	versionText, err := requireExactVersionField(prefix+".version", raw.Version)
	if err != nil {
		return CatalogItem{}, err
	}
	fields, err := buildCatalogItemFields(prefix, raw)
	if err != nil {
		return CatalogItem{}, err
	}
	return catalogItemOf(fields, domain.Version{}, versionText), nil
}

// envelopeFileOf はtyped envelopeをJSON構造へ変換する。
func envelopeFileOf(value Envelope) (envelopeFile, error) {
	schema := int64(SchemaVersion)
	ok := value.OK
	command := string(value.Command)
	invocation := value.Invocation.String()

	warnings := make([]*resultWarningJSON, 0, len(value.Warnings))
	for _, warning := range value.Warnings {
		code := string(warning.Code)
		messageID := warning.MessageID.String()
		parameters, err := encodeScalarMap(warning.Parameters, domain.ParameterKeyMaxLength)
		if err != nil {
			return envelopeFile{}, err
		}
		warnings = append(warnings, &resultWarningJSON{
			Code: &code, MessageID: &messageID, Parameters: &parameters,
		})
	}

	file := envelopeFile{
		Schema: &schema, OK: &ok, Command: &command,
		InvocationID: &invocation, Warnings: warnings,
	}
	if !value.OK {
		if value.Error == nil {
			return envelopeFile{}, fmt.Errorf("ok=falseなのにErrorが未設定")
		}
		errorJSON, err := resultErrorJSONOf(*value.Error)
		if err != nil {
			return envelopeFile{}, err
		}
		file.Error = errorJSON
		return file, nil
	}
	data, err := envelopeDataOf(value)
	if err != nil {
		return envelopeFile{}, err
	}
	file.Data = data
	return file, nil
}

func resultErrorJSONOf(value ResultError) (*resultErrorJSON, error) {
	code := string(value.Code)
	messageID := value.MessageID.String()
	parameters, err := encodeScalarMap(value.Parameters, domain.ParameterKeyMaxLength)
	if err != nil {
		return nil, err
	}
	retryable := value.Retryable
	return &resultErrorJSON{
		Code: &code, MessageID: &messageID, Parameters: &parameters, Retryable: &retryable,
	}, nil
}

// envelopeDataOf はcommandに対応するdataだけを書き出す。
//
// commandに対応しないfieldが埋まったEnvelopeは組立て側の誤りである。黙って
// 落とすと、呼出し側が渡したはずのdataが出力から消える。
func envelopeDataOf(value Envelope) (*envelopeData, error) {
	if err := checkEnvelopeFieldsMatchCommand(value); err != nil {
		return nil, err
	}
	data := &envelopeData{}
	switch value.Command {
	case CommandAvailable:
		if value.Available == nil {
			return nil, fmt.Errorf("command=availableなのにAvailableが未設定")
		}
		toolID := value.Available.Tool.String()
		platformID := value.Available.Platform.ID()
		items := make([]*catalogItemJSON, 0, len(value.Available.Items))
		for _, item := range value.Available.Items {
			items = append(items, displayCatalogItemJSONOf(item))
		}
		data.ToolID, data.PlatformID, data.Items = &toolID, &platformID, &items
	case CommandInstalled:
		installs := make([]*installSummaryJSON, 0, len(value.Installs))
		for _, install := range value.Installs {
			installs = append(installs, installSummaryJSONOf(install))
		}
		data.Installs = &installs
	case CommandCurrent:
		selections := make([]*selectionSummaryJSON, 0, len(value.Selections))
		for _, selection := range value.Selections {
			selections = append(selections, selectionSummaryJSONOf(selection))
		}
		data.Selections = &selections
	case CommandDoctor:
		if value.Doctor == nil {
			return nil, fmt.Errorf("command=doctorなのにDoctorが未設定")
		}
		status := string(value.Doctor.Status)
		diagnostics := make([]*diagnosticJSON, 0, len(value.Doctor.Diagnostics))
		for _, diagnostic := range value.Doctor.Diagnostics {
			entry, err := diagnosticJSONOf(diagnostic)
			if err != nil {
				return nil, err
			}
			diagnostics = append(diagnostics, entry)
		}
		data.Status, data.Diagnostics = &status, &diagnostics
		data.ReportPath = pathValueOf(value.Doctor.ReportPath)
	case CommandVersion:
		if value.Build == nil {
			return nil, fmt.Errorf("command=versionなのにBuildが未設定")
		}
		data.Build = buildResultJSONOf(*value.Build)
	default:
		return nil, fmt.Errorf("commandが許可された値でない（%q）", value.Command)
	}
	return data, nil
}

// checkEnvelopeFieldsMatchCommand はcommand以外のdata fieldが空であることを確かめる。
func checkEnvelopeFieldsMatchCommand(value Envelope) error {
	populated := map[JSONCommand]bool{
		CommandAvailable: value.Available != nil,
		CommandInstalled: len(value.Installs) > 0,
		CommandCurrent:   len(value.Selections) > 0,
		CommandDoctor:    value.Doctor != nil,
		CommandVersion:   value.Build != nil,
	}
	for command, filled := range populated {
		if filled && command != value.Command {
			return fmt.Errorf("command=%qのEnvelopeに%q用のdataが入っている", value.Command, command)
		}
	}
	return nil
}

func installSummaryJSONOf(value InstallSummary) *installSummaryJSON {
	toolID := value.Ref.Tool.String()
	version := value.Ref.Version
	platformID := value.Ref.Platform.ID()
	installID := value.InstallID
	installedAt := formatTimestamp(value.InstalledAt)
	health := string(value.Health)
	diskSize := value.DiskSize
	provider := string(value.Provider)
	selected := value.Selected
	return &installSummaryJSON{
		ToolID: &toolID, Version: &version, PlatformID: &platformID,
		InstallID: &installID, InstalledAt: &installedAt, Health: &health,
		ReceiptPath: pathValueOf(value.ReceiptPath), DiskSize: &diskSize,
		ProviderKind: &provider, Selected: &selected,
	}
}

func selectionSummaryJSONOf(value SelectionSummary) *selectionSummaryJSON {
	source := string(value.Source)
	toolID := value.Tool.String()
	version := value.Version
	installID := value.InstallID
	health := string(value.Health)
	return &selectionSummaryJSON{
		Source: &source, ProjectFile: pathValueOf(value.ProjectFile),
		ToolID: &toolID, Version: &version, InstallID: &installID,
		PayloadPath: pathValueOf(value.PayloadPath), Health: &health,
	}
}

func diagnosticJSONOf(value Diagnostic) (*diagnosticJSON, error) {
	code := string(value.Code)
	severity := string(value.Severity)
	messageID := value.MessageID.String()
	parameters, err := encodeScalarMap(value.Parameters, domain.ParameterKeyMaxLength)
	if err != nil {
		return nil, err
	}
	paths := make([]*pathValueJSON, 0, len(value.Paths))
	for _, path := range value.Paths {
		paths = append(paths, pathValueOf(path))
	}
	return &diagnosticJSON{
		Code: &code, Severity: &severity, MessageID: &messageID,
		Parameters: &parameters, Paths: &paths,
	}, nil
}

func buildResultJSONOf(value BuildResult) *buildResultJSON {
	schema := int64(SchemaVersion)
	version := value.Version
	commit := value.Commit
	buildTime := formatTimestamp(value.BuildTime)
	goVersion := value.GoVersion
	platformID := value.Platform.ID()
	development := value.Development
	return &buildResultJSON{
		Version: &version, Commit: &commit, BuildTime: &buildTime,
		GoVersion: &goVersion, PlatformID: &platformID,
		StateSchema: &schema, DefinitionSchema: &schema, RegistrySchema: &schema,
		Development: &development,
	}
}

// displayCatalogItemJSONOf は`available`のitemをJSON構造へ変換する。
//
// catalogのitemと同じkey集合であり、[catalogItemJSONOf]がVersionTextを
// 優先して書くため、schemeの有無によらず同じ出力になる。
func displayCatalogItemJSONOf(item CatalogItem) *catalogItemJSON {
	return catalogItemJSONOf(item)
}
