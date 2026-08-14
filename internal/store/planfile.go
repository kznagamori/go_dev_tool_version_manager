package store

import (
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
)

// planFile は§16のtop-level key集合である。全件必須。
type planFile struct {
	Schema        *int64              `json:"schema"`
	ClientVersion *string             `json:"client_version"`
	InvocationID  *string             `json:"invocation_id"`
	OperationID   *string             `json:"operation_id"`
	Operation     *string             `json:"operation"`
	CreatedAt     *string             `json:"created_at"`
	Summary       *planSummaryJSON    `json:"summary"`
	Setup         *setupPlanJSON      `json:"setup"`
	Inputs        *planInputsJSON     `json:"inputs"`
	Downloads     []*planDownloadJSON `json:"downloads"`
	Extracts      []*planExtractJSON  `json:"extracts"`
	Probes        []*planProbeJSON    `json:"probes"`
	Writes        []*planWriteJSON    `json:"writes"`
	Storage       []*planStorageJSON  `json:"storage"`
	Warnings      []*planWarningJSON  `json:"warnings"`
}

type planSummaryJSON struct {
	ToolID             *string `json:"tool_id"`
	Version            *string `json:"version"`
	PlatformID         *string `json:"platform_id"`
	ProviderKind       *string `json:"provider_kind"`
	ProviderName       *string `json:"provider_name"`
	ProviderRepository *string `json:"provider_repository"`
	ProviderHomepage   *string `json:"provider_homepage"`
	ProviderLicense    *string `json:"provider_license"`
	ProviderRelease    *string `json:"provider_release"`
	LicenseNotice      *string `json:"license_notice"`
	Channel            *string `json:"channel"`
	Lifecycle          *string `json:"lifecycle"`
	ExpectedDigest     *string `json:"expected_digest"`
	ChecksumSource     *string `json:"checksum_source"`
	WarningCount       *int64  `json:"warning_count"`
}

type planInputsJSON struct {
	RootID               *string `json:"root_id"`
	ConfigSHA256         *string `json:"config_sha256"`
	ProjectSHA256        *string `json:"project_sha256"`
	DefinitionSHA256     *string `json:"definition_sha256"`
	CatalogSHA256        *string `json:"catalog_sha256"`
	RegistrySHA256       *string `json:"registry_sha256"`
	SelectionsRevision   *int64  `json:"selections_revision"`
	SetupRevision        *int64  `json:"setup_revision"`
	ReceiptIndexRevision *int64  `json:"receipt_index_revision"`
}

type setupPlanJSON struct {
	Mode                     *string        `json:"mode"`
	PreviousMode             *string        `json:"previous_mode"`
	DataRoot                 *pathValueJSON `json:"data_root"`
	DistributionRoot         *pathValueJSON `json:"distribution_root"`
	PreviousDataRoot         *pathValueJSON `json:"previous_data_root"`
	PreviousDistributionRoot *pathValueJSON `json:"previous_distribution_root"`
	FilesystemCapabilities   *[]string      `json:"filesystem_capabilities"`
	CurrentLinkStrategy      *string        `json:"current_link_strategy"`
	ShimStrategy             *string        `json:"shim_strategy"`
	ShimDirectory            *pathValueJSON `json:"shim_directory"`
	PathIntegration          *string        `json:"path_integration"`
	Shell                    *string        `json:"shell"`
	IntegrationTarget        *pathValueJSON `json:"integration_target"`
	BackupPath               *pathValueJSON `json:"backup_path"`
	RestartRequired          *bool          `json:"restart_required"`
}

type planDownloadJSON struct {
	ID                      *string        `json:"id"`
	ProviderKind            *string        `json:"provider_kind"`
	ProviderName            *string        `json:"provider_name"`
	ProviderRepository      *string        `json:"provider_repository"`
	ProviderHomepage        *string        `json:"provider_homepage"`
	ProviderRelease         *string        `json:"provider_release"`
	URL                     *string        `json:"url"`
	FileName                *string        `json:"file_name"`
	Size                    *int64         `json:"size"`
	ExpectedDigest          *string        `json:"expected_digest"`
	ChecksumSource          *string        `json:"checksum_source"`
	License                 *string        `json:"license"`
	AdoptionReasonMessageID *string        `json:"adoption_reason_message_id"`
	Destination             *pathValueJSON `json:"destination"`
}

type planExtractJSON struct {
	ID               *string        `json:"id"`
	SourceDownloadID *string        `json:"source_download_id"`
	Format           *string        `json:"format"`
	StripComponents  *int64         `json:"strip_components"`
	Destination      *pathValueJSON `json:"destination"`
}

type planArgJSON struct {
	Kind  *string        `json:"kind"`
	Value *string        `json:"value"`
	Path  *pathValueJSON `json:"path"`
}

type planRequiredPathJSON struct {
	Kind *string        `json:"kind"`
	Path *pathValueJSON `json:"path"`
}

type planProbeJSON struct {
	ID               *string                  `json:"id"`
	RuntimeCommand   *string                  `json:"runtime_command"`
	Executable       *pathValueJSON           `json:"executable"`
	Version          *string                  `json:"version"`
	Source           *string                  `json:"source"`
	ArtifactDigest   *string                  `json:"artifact_digest"`
	License          *string                  `json:"license"`
	ReasonMessageID  *string                  `json:"reason_message_id"`
	Args             *[]*planArgJSON          `json:"args"`
	WorkingDirectory *pathValueJSON           `json:"working_directory"`
	WritePaths       *[]*pathValueJSON        `json:"write_paths"`
	Stream           *string                  `json:"stream"`
	Expect           *string                  `json:"expect"`
	Regex            *string                  `json:"regex"`
	ExpectedVersion  *string                  `json:"expected_version"`
	ExpectedRoot     *pathValueJSON           `json:"expected_root"`
	RequiredPaths    *[]*planRequiredPathJSON `json:"required_paths"`
	TimeoutMillis    *int64                   `json:"timeout_ms"`
	Required         *bool                    `json:"required"`
}

type planWriteJSON struct {
	ID     *string        `json:"id"`
	Action *string        `json:"action"`
	Target *pathValueJSON `json:"target"`
}

type planStorageJSON struct {
	ID     *string        `json:"id"`
	Kind   *string        `json:"kind"`
	Scope  *string        `json:"scope"`
	Target *pathValueJSON `json:"target"`
	Purge  *string        `json:"purge"`
	Action *string        `json:"action"`
}

type planWarningJSON struct {
	Code                     *string         `json:"code"`
	MessageID                *string         `json:"message_id"`
	Parameters               *map[string]any `json:"parameters"`
	RequiresExplicitApproval *bool           `json:"requires_explicit_approval"`
}

// planFileOf はtyped PlanをJSON構造へ変換する。
//
// 各配列はIDのASCII byte順で出す（§16）。同じPlanから同じdocumentが出ることで、
// 承認前と実行時の突き合わせが単純になる。
func planFileOf(value Plan) (planFile, error) {
	schema := int64(SchemaVersion)
	clientVersion := value.ClientVersion
	invocation := value.Invocation.String()
	operation := value.Operation.String()
	kind := string(value.Kind)
	createdAt := formatTimestamp(value.CreatedAt)

	warnings := make([]*planWarningJSON, 0, len(value.Warnings))
	for _, warning := range value.Warnings {
		code := string(warning.Code)
		messageID := warning.MessageID.String()
		parameters, err := encodeScalarMap(warning.Parameters, domain.ParameterKeyMaxLength)
		if err != nil {
			return planFile{}, err
		}
		approval := warning.RequiresExplicitApproval
		warnings = append(warnings, &planWarningJSON{
			Code: &code, MessageID: &messageID, Parameters: &parameters,
			RequiresExplicitApproval: &approval,
		})
	}

	downloads := make([]*planDownloadJSON, 0, len(value.Downloads))
	for _, entry := range value.Downloads {
		downloads = append(downloads, planDownloadJSONOf(entry))
	}
	sortEntriesByKey(downloads, func(entry *planDownloadJSON) string { return *entry.ID })

	extracts := make([]*planExtractJSON, 0, len(value.Extracts))
	for _, entry := range value.Extracts {
		extracts = append(extracts, planExtractJSONOf(entry))
	}
	sortEntriesByKey(extracts, func(entry *planExtractJSON) string { return *entry.ID })

	probes := make([]*planProbeJSON, 0, len(value.Probes))
	for _, entry := range value.Probes {
		probes = append(probes, planProbeJSONOf(entry))
	}
	sortEntriesByKey(probes, func(entry *planProbeJSON) string { return *entry.ID })

	writes := make([]*planWriteJSON, 0, len(value.Writes))
	for _, entry := range value.Writes {
		id, action := entry.ID, string(entry.Action)
		writes = append(writes, &planWriteJSON{
			ID: &id, Action: &action, Target: pathValueOf(entry.Target),
		})
	}
	sortEntriesByKey(writes, func(entry *planWriteJSON) string { return *entry.ID })

	storage := make([]*planStorageJSON, 0, len(value.Storage))
	for _, entry := range value.Storage {
		id := entry.ID
		kindText, scope := string(entry.Kind), string(entry.Scope)
		purge, action := string(entry.Purge), string(entry.Action)
		storage = append(storage, &planStorageJSON{
			ID: &id, Kind: &kindText, Scope: &scope,
			Target: pathValueOf(entry.Target), Purge: &purge, Action: &action,
		})
	}
	sortEntriesByKey(storage, func(entry *planStorageJSON) string { return *entry.ID })

	return planFile{
		Schema: &schema, ClientVersion: &clientVersion, InvocationID: &invocation,
		OperationID: &operation, Operation: &kind, CreatedAt: &createdAt,
		Summary: planSummaryJSONOf(value.Summary), Setup: setupPlanJSONOf(value.Setup),
		Inputs:    planInputsJSONOf(value.Inputs),
		Downloads: downloads, Extracts: extracts, Probes: probes,
		Writes: writes, Storage: storage, Warnings: warnings,
	}, nil
}

func planSummaryJSONOf(value PlanSummary) *planSummaryJSON {
	toolID := value.Tool.String()
	version := value.Version
	platformID := value.Platform.ID()
	providerKind := string(value.ProviderKind)
	channel := string(value.Channel)
	lifecycle := string(value.Lifecycle)
	digest := upstreamDigestText(value.ExpectedDigest)
	source := string(value.ChecksumSource)
	count := value.WarningCount
	return &planSummaryJSON{
		ToolID: &toolID, Version: &version, PlatformID: &platformID,
		ProviderKind: &providerKind, ProviderName: &value.ProviderName,
		ProviderRepository: &value.ProviderRepository, ProviderHomepage: &value.ProviderHomepage,
		ProviderLicense: &value.ProviderLicense, ProviderRelease: &value.ProviderRelease,
		LicenseNotice: &value.LicenseNotice, Channel: &channel, Lifecycle: &lifecycle,
		ExpectedDigest: &digest, ChecksumSource: &source, WarningCount: &count,
	}
}

func planInputsJSONOf(value PlanInputs) *planInputsJSON {
	return &planInputsJSON{
		RootID: &value.RootID, ConfigSHA256: &value.ConfigSHA256,
		ProjectSHA256: &value.ProjectSHA256, DefinitionSHA256: &value.DefinitionSHA256,
		CatalogSHA256: &value.CatalogSHA256, RegistrySHA256: &value.RegistrySHA256,
		SelectionsRevision: &value.SelectionsRevision, SetupRevision: &value.SetupRevision,
		ReceiptIndexRevision: &value.ReceiptIndexRevision,
	}
}

func setupPlanJSONOf(value *SetupPlan) *setupPlanJSON {
	if value == nil {
		return nil
	}
	mode := string(value.Mode)
	previousMode := string(value.PreviousMode)
	capabilities := make([]string, 0, len(value.FilesystemCapabilities))
	for _, capability := range value.FilesystemCapabilities {
		capabilities = append(capabilities, string(capability))
	}
	link := string(value.CurrentLinkStrategy)
	shim := string(value.ShimStrategy)
	integration := string(value.PathIntegration)
	shell := string(value.Shell)
	restart := value.RestartRequired
	return &setupPlanJSON{
		Mode: &mode, PreviousMode: &previousMode,
		DataRoot: pathValueOf(value.DataRoot), DistributionRoot: pathValueOf(value.DistributionRoot),
		PreviousDataRoot:         pathValueOf(value.PreviousDataRoot),
		PreviousDistributionRoot: pathValueOf(value.PreviousDistributionRoot),
		FilesystemCapabilities:   &capabilities, CurrentLinkStrategy: &link,
		ShimStrategy: &shim, ShimDirectory: pathValueOf(value.ShimDirectory),
		PathIntegration: &integration, Shell: &shell,
		IntegrationTarget: pathValueOf(value.IntegrationTarget),
		BackupPath:        pathValueOf(value.BackupPath), RestartRequired: &restart,
	}
}

func planDownloadJSONOf(value PlanDownload) *planDownloadJSON {
	id := value.ID
	kind := string(value.ProviderKind)
	digest := upstreamDigestText(value.ExpectedDigest)
	source := string(value.ChecksumSource)
	size := value.Size
	return &planDownloadJSON{
		ID: &id, ProviderKind: &kind, ProviderName: &value.ProviderName,
		ProviderRepository: &value.ProviderRepository, ProviderHomepage: &value.ProviderHomepage,
		ProviderRelease: &value.ProviderRelease, URL: &value.URL, FileName: &value.FileName,
		Size: &size, ExpectedDigest: &digest, ChecksumSource: &source,
		License: &value.License, AdoptionReasonMessageID: &value.AdoptionReasonMessage,
		Destination: pathValueOf(value.Destination),
	}
}

func planExtractJSONOf(value PlanExtract) *planExtractJSON {
	id, sourceID := value.ID, value.SourceDownloadID
	format := string(value.Format)
	strip := value.StripComponents
	return &planExtractJSON{
		ID: &id, SourceDownloadID: &sourceID, Format: &format,
		StripComponents: &strip, Destination: pathValueOf(value.Destination),
	}
}

func planProbeJSONOf(value PlanProbe) *planProbeJSON {
	id, command := value.ID, value.RuntimeCommand
	version, source := value.Version, value.Source
	digest := upstreamDigestText(value.ArtifactDigest)
	license := value.License
	reason := value.ReasonMessageID.String()
	stream, expect := string(value.Stream), string(value.Expect)
	regex, expectedVersion := value.Regex, value.ExpectedVersion
	timeout, required := value.TimeoutMillis, value.Required

	args := make([]*planArgJSON, 0, len(value.Args))
	for _, arg := range value.Args {
		kind, text := string(arg.Kind), arg.Value
		entry := &planArgJSON{Kind: &kind, Value: &text}
		if arg.Kind == ArgPath {
			entry.Path = pathValueOf(arg.Path)
		}
		args = append(args, entry)
	}
	writePaths := make([]*pathValueJSON, 0, len(value.WritePaths))
	for _, path := range value.WritePaths {
		writePaths = append(writePaths, pathValueOf(path))
	}
	requiredPaths := make([]*planRequiredPathJSON, 0, len(value.RequiredPaths))
	for _, entry := range value.RequiredPaths {
		kind := string(entry.Kind)
		requiredPaths = append(requiredPaths, &planRequiredPathJSON{
			Kind: &kind, Path: pathValueOf(entry.Path),
		})
	}

	probe := &planProbeJSON{
		ID: &id, RuntimeCommand: &command, Executable: pathValueOf(value.Executable),
		Version: &version, Source: &source, ArtifactDigest: &digest, License: &license,
		ReasonMessageID: &reason, Args: &args,
		WorkingDirectory: pathValueOf(value.WorkingDirectory), WritePaths: &writePaths,
		Stream: &stream, Expect: &expect, Regex: &regex, ExpectedVersion: &expectedVersion,
		RequiredPaths: &requiredPaths, TimeoutMillis: &timeout, Required: &required,
	}
	if value.ExpectedRoot != nil {
		probe.ExpectedRoot = pathValueOf(*value.ExpectedRoot)
	}
	return probe
}
