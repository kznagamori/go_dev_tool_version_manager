package store

import (
	"fmt"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
)

func buildPlanSummary(raw planSummaryJSON, kind PlanOperation) (PlanSummary, error) {
	var value PlanSummary
	providerKind, err := requireEnum("summary.provider_kind", raw.ProviderKind, planProviderKinds)
	if err != nil {
		return value, err
	}
	// 対象toolがないoperationはprovider kind=`none`である（§16）。setup系だけが
	// これに当たる。install/use/uninstallでnoneを許すと、どのartifactを扱うかが
	// 不明なPlanを承認できてしまう。
	toolless := providerKind == ProviderNone
	if toolless && kind != OperationSetup && kind != OperationSetupRemove {
		return value, fmt.Errorf("operation %q でprovider_kind=noneは許されない", kind)
	}
	if !toolless && (kind == OperationSetup || kind == OperationSetupRemove) {
		return value, fmt.Errorf("operation %q のprovider_kindはnoneでなければならない", kind)
	}

	toolText, err := requirePresent("summary.tool_id", raw.ToolID)
	if err != nil {
		return value, err
	}
	versionText, err := requirePresent("summary.version", raw.Version)
	if err != nil {
		return value, err
	}
	platformText, err := requirePresent("summary.platform_id", raw.PlatformID)
	if err != nil {
		return value, err
	}
	channelText, err := requirePresent("summary.channel", raw.Channel)
	if err != nil {
		return value, err
	}
	lifecycleText, err := requirePresent("summary.lifecycle", raw.Lifecycle)
	if err != nil {
		return value, err
	}
	digestText, err := requirePresent("summary.expected_digest", raw.ExpectedDigest)
	if err != nil {
		return value, err
	}
	checksumText, err := requirePresent("summary.checksum_source", raw.ChecksumSource)
	if err != nil {
		return value, err
	}
	for _, pair := range []struct {
		name  string
		value **string
	}{
		{"summary.provider_name", &raw.ProviderName},
		{"summary.provider_repository", &raw.ProviderRepository},
		{"summary.provider_homepage", &raw.ProviderHomepage},
		{"summary.provider_license", &raw.ProviderLicense},
		{"summary.provider_release", &raw.ProviderRelease},
		{"summary.license_notice", &raw.LicenseNotice},
	} {
		if _, err := requirePresent(pair.name, *pair.value); err != nil {
			return value, err
		}
	}
	warningCount, err := requireInt64("summary.warning_count", raw.WarningCount)
	if err != nil {
		return value, err
	}

	value = PlanSummary{
		ProviderKind: providerKind, ProviderName: *raw.ProviderName,
		ProviderRepository: *raw.ProviderRepository, ProviderHomepage: *raw.ProviderHomepage,
		ProviderLicense: *raw.ProviderLicense, ProviderRelease: *raw.ProviderRelease,
		LicenseNotice: *raw.LicenseNotice, Version: versionText, WarningCount: warningCount,
	}
	if toolless {
		// 「対象外のstringは空、数値は0とする」（§16）。
		for _, pair := range []struct{ name, text string }{
			{"summary.tool_id", toolText}, {"summary.version", versionText},
			{"summary.platform_id", platformText}, {"summary.channel", channelText},
			{"summary.lifecycle", lifecycleText}, {"summary.expected_digest", digestText},
			{"summary.checksum_source", checksumText},
			{"summary.provider_name", *raw.ProviderName},
			{"summary.provider_repository", *raw.ProviderRepository},
			{"summary.provider_homepage", *raw.ProviderHomepage},
			{"summary.provider_license", *raw.ProviderLicense},
			{"summary.provider_release", *raw.ProviderRelease},
		} {
			if err := requireEmpty(pair.name, pair.text); err != nil {
				return PlanSummary{}, err
			}
		}
		return value, nil
	}

	tool, err := domain.ParseToolID(toolText)
	if err != nil {
		return PlanSummary{}, fmt.Errorf("summary.tool_id: %w", err)
	}
	if err := requireExactVersion("summary.version", versionText); err != nil {
		return PlanSummary{}, err
	}
	platform, err := domain.ParsePlatform(platformText)
	if err != nil {
		return PlanSummary{}, fmt.Errorf("summary.platform_id: %w", err)
	}
	channel, err := domain.ParseChannel(channelText)
	if err != nil {
		return PlanSummary{}, fmt.Errorf("summary.channel: %w", err)
	}
	lifecycle, err := domain.ParseLifecycle(lifecycleText)
	if err != nil {
		return PlanSummary{}, fmt.Errorf("summary.lifecycle: %w", err)
	}
	digest, err := parseUpstreamDigest("summary.expected_digest", digestText)
	if err != nil {
		return PlanSummary{}, err
	}
	source, err := requireEnum("summary.checksum_source", raw.ChecksumSource, checksumSources)
	if err != nil {
		return PlanSummary{}, err
	}
	if _, err := requireNonEmpty("summary.provider_name", raw.ProviderName); err != nil {
		return PlanSummary{}, err
	}
	if _, err := requireNonEmpty("summary.provider_release", raw.ProviderRelease); err != nil {
		return PlanSummary{}, err
	}
	// third-partyは取得元と根拠を必ず示す（10-security.md §8「外部programはPlanで
	// 名称、完全版、取得元、digest、license、実行理由、argv要約、書込み先を表示」）。
	if providerKind == ProviderThirdParty {
		for _, pair := range []struct{ name, text string }{
			{"summary.provider_repository", *raw.ProviderRepository},
			{"summary.provider_homepage", *raw.ProviderHomepage},
			{"summary.provider_license", *raw.ProviderLicense},
		} {
			if pair.text == "" {
				return PlanSummary{}, fmt.Errorf("third-party providerの%sが空", pair.name)
			}
		}
	}
	value.Tool, value.Platform, value.Channel = tool, platform, channel
	value.Lifecycle, value.ExpectedDigest, value.ChecksumSource = lifecycle, digest, source
	return value, nil
}

func buildPlanInputs(raw planInputsJSON) (PlanInputs, error) {
	var value PlanInputs
	rootID, err := requireIDField("inputs.root_id", raw.RootID)
	if err != nil {
		return value, err
	}
	digests := make([]string, 0, 5)
	for _, pair := range []struct {
		name string
		raw  *string
	}{
		{"inputs.config_sha256", raw.ConfigSHA256},
		{"inputs.project_sha256", raw.ProjectSHA256},
		{"inputs.definition_sha256", raw.DefinitionSHA256},
		{"inputs.catalog_sha256", raw.CatalogSHA256},
		{"inputs.registry_sha256", raw.RegistrySHA256},
	} {
		text, err := requirePresent(pair.name, pair.raw)
		if err != nil {
			return value, err
		}
		// 「対象外のstringは空」（§16）。configやprojectが無い環境では空になる。
		if text != "" {
			if _, err := parseInternalDigest(pair.name, text); err != nil {
				return value, err
			}
		}
		digests = append(digests, text)
	}
	selections, err := requireInt64("inputs.selections_revision", raw.SelectionsRevision)
	if err != nil {
		return value, err
	}
	setup, err := requireInt64("inputs.setup_revision", raw.SetupRevision)
	if err != nil {
		return value, err
	}
	receiptIndex, err := requireInt64("inputs.receipt_index_revision", raw.ReceiptIndexRevision)
	if err != nil {
		return value, err
	}
	return PlanInputs{
		RootID: rootID, ConfigSHA256: digests[0], ProjectSHA256: digests[1],
		DefinitionSHA256: digests[2], CatalogSHA256: digests[3], RegistrySHA256: digests[4],
		SelectionsRevision: selections, SetupRevision: setup,
		ReceiptIndexRevision: receiptIndex,
	}, nil
}

func buildSetupPlan(
	raw *setupPlanJSON, kind PlanOperation, warnings []PlanWarning,
) (*SetupPlan, error) {
	isSetup := kind == OperationSetup || kind == OperationSetupRemove
	if raw == nil {
		if isSetup {
			return nil, fmt.Errorf("operation %q にsetupが無い", kind)
		}
		return nil, nil
	}
	// §16が「`setup`は`operation=setup|setup-remove`のときだけ`SetupPlan` object、
	// それ以外は`null`」と定める。
	if !isSetup {
		return nil, fmt.Errorf("operation %q でsetupはnullでなければならない", kind)
	}

	mode, err := requireMode("setup.mode", raw.Mode)
	if err != nil {
		return nil, err
	}
	previousModeText, err := requirePresent("setup.previous_mode", raw.PreviousMode)
	if err != nil {
		return nil, err
	}
	var previousMode domain.Mode
	if previousModeText != "" {
		previousMode, err = domain.ParseMode(previousModeText)
		if err != nil {
			return nil, fmt.Errorf("setup.previous_mode: %w", err)
		}
	}
	dataRoot, err := buildPathValue("setup.data_root", raw.DataRoot, domain.RoleDataRoot, pathAbsolute)
	if err != nil {
		return nil, err
	}
	distributionRoot, err := buildPathValue(
		"setup.distribution_root", raw.DistributionRoot, domain.RoleDistributionRoot, pathAbsolute)
	if err != nil {
		return nil, err
	}
	previousDataRoot, err := buildPathValue(
		"setup.previous_data_root", raw.PreviousDataRoot, domain.RoleDataRoot, pathOptional)
	if err != nil {
		return nil, err
	}
	previousDistributionRoot, err := buildPathValue(
		"setup.previous_distribution_root", raw.PreviousDistributionRoot,
		domain.RoleDistributionRoot, pathOptional)
	if err != nil {
		return nil, err
	}
	capabilities, err := buildCapabilities(raw.FilesystemCapabilities)
	if err != nil {
		return nil, err
	}
	linkStrategy, err := requireEnum("setup.current_link_strategy", raw.CurrentLinkStrategy, linkStrategies)
	if err != nil {
		return nil, err
	}
	shimStrategy, err := requireEnum("setup.shim_strategy", raw.ShimStrategy, shimStrategies)
	if err != nil {
		return nil, err
	}
	shimDirectory, err := buildPathValue(
		"setup.shim_directory", raw.ShimDirectory, domain.RoleShim, pathAbsolute)
	if err != nil {
		return nil, err
	}
	integration, err := requireEnum("setup.path_integration", raw.PathIntegration, pathIntegrations)
	if err != nil {
		return nil, err
	}
	shell, err := requireShell(raw.Shell, integration)
	if err != nil {
		return nil, fmt.Errorf("setup.%w", err)
	}
	// integration targetは空（none）、Windows PATH locator、shell profileの
	// absolute pathの3通りを取る（§17.2）。
	integrationTarget, err := buildPathValue(
		"setup.integration_target", raw.IntegrationTarget, domain.RoleConfig, pathOptionalLocator)
	if err != nil {
		return nil, err
	}
	if integration == PathIntegrationNone && integrationTarget.Path() != "" {
		return nil, fmt.Errorf("setup.path_integration=noneならintegration_targetのpathは空")
	}
	if integration != PathIntegrationNone && integrationTarget.Path() == "" {
		return nil, fmt.Errorf("setup.path_integration=%qならintegration_targetのpathが必要", integration)
	}
	backupPath, err := buildPathValue(
		"setup.backup_path", raw.BackupPath, domain.RoleStateBackup, pathOptional)
	if err != nil {
		return nil, err
	}
	restartRequired, err := requireBool("setup.restart_required", raw.RestartRequired)
	if err != nil {
		return nil, err
	}

	if err := checkSetupPreviousFields(previousModeText, previousDataRoot, previousDistributionRoot, warnings); err != nil {
		return nil, err
	}
	if err := checkRestartWarning(restartRequired, warnings); err != nil {
		return nil, err
	}
	if err := checkPlatformCapabilities(capabilities, linkStrategy, shimStrategy); err != nil {
		return nil, err
	}
	return &SetupPlan{
		Mode: mode, PreviousMode: previousMode, DataRoot: dataRoot,
		DistributionRoot: distributionRoot, PreviousDataRoot: previousDataRoot,
		PreviousDistributionRoot: previousDistributionRoot,
		FilesystemCapabilities:   capabilities, CurrentLinkStrategy: linkStrategy,
		ShimStrategy: shimStrategy, ShimDirectory: shimDirectory,
		PathIntegration: integration, Shell: shell,
		IntegrationTarget: integrationTarget, BackupPath: backupPath,
		RestartRequired: restartRequired,
	}, nil
}

// buildCapabilities は§16の「§17.1の値をASCII byte順・重複なしで1～7件」を読む。
func buildCapabilities(raw *[]string) ([]FilesystemCapability, error) {
	if raw == nil {
		return nil, fmt.Errorf("setup.filesystem_capabilitiesが無い")
	}
	values := *raw
	if len(values) < 1 || len(values) > FilesystemCapabilityCount {
		return nil, fmt.Errorf(
			"setup.filesystem_capabilitiesは1〜%d件でなければならない（%d件）",
			FilesystemCapabilityCount, len(values))
	}
	entries := make([]FilesystemCapability, 0, len(values))
	for index, text := range values {
		capability := FilesystemCapability(text)
		if _, known := filesystemCapabilities[capability]; !known {
			return nil, fmt.Errorf(
				"setup.filesystem_capabilities[%d]が許可された値でない（%q）", index, text)
		}
		entries = append(entries, capability)
	}
	if err := requireSortedUnique("setup.filesystem_capabilities", len(entries), func(i int) string {
		return string(entries[i])
	}); err != nil {
		return nil, err
	}
	return entries, nil
}

// checkPlatformCapabilities は§16のplatform別必須capabilityを確かめる。
//
// Windowsは`junction`、Linuxは`symlink`を必須とし、それぞれcurrent link strategy
// とshim strategyの組合せが決まる。必須capabilityを確認できない場合はPlanを
// 作らず`E_PLATFORM_UNSUPPORTED`にする（§16）が、その判定はsetup engineの責務で
// ある。ここではPlanとして矛盾していないかだけを見る。
func checkPlatformCapabilities(
	capabilities []FilesystemCapability, link LinkStrategy, shim ShimStrategy,
) error {
	have := make(map[FilesystemCapability]struct{}, len(capabilities))
	for _, capability := range capabilities {
		have[capability] = struct{}{}
	}
	// 両OS共通の必須4件。
	for _, required := range []FilesystemCapability{
		CapabilityAtomicReplace, CapabilityDirectoryRename,
		CapabilityFileIdentity, CapabilityOwnerEnforce,
	} {
		if _, ok := have[required]; !ok {
			return fmt.Errorf("setup.filesystem_capabilitiesに%qが無い", required)
		}
	}
	switch link {
	case LinkJunction:
		if _, ok := have[CapabilityJunction]; !ok {
			return fmt.Errorf("current_link_strategy=junctionにはcapability junctionが必要")
		}
		if shim != ShimHardlink && shim != ShimFallbackResolver {
			return fmt.Errorf("junction環境のshim_strategyは`hardlink|fallback-resolver`だけ（%q）", shim)
		}
	case LinkSymlink:
		if _, ok := have[CapabilitySymlink]; !ok {
			return fmt.Errorf("current_link_strategy=symlinkにはcapability symlinkが必要")
		}
		if shim != ShimSymlink && shim != ShimFallbackResolver {
			return fmt.Errorf("symlink環境のshim_strategyは`symlink|fallback-resolver`だけ（%q）", shim)
		}
	}
	// §16が「hardlinkを使う場合だけcapabilityへ`hardlink`を含める」と定める。
	_, hasHardlink := have[CapabilityHardlink]
	if hasHardlink != (shim == ShimHardlink) {
		return fmt.Errorf(
			"capability hardlinkの有無(%v)とshim_strategy(%q)が一致しない", hasHardlink, shim)
	}
	return nil
}

// checkSetupPreviousFields は§16のprevious 3 fieldの同時性を確かめる。
//
// 「`previous_mode`、`previous_data_root`、`previous_distribution_root`は同時に
// 空または同時に非空とする。非空と`W_MODE_CHANGE` exactly 1件を同値にする」。
func checkSetupPreviousFields(
	previousMode string, previousDataRoot, previousDistributionRoot domain.PathValue,
	warnings []PlanWarning,
) error {
	present := 0
	if previousMode != "" {
		present++
	}
	if previousDataRoot.Path() != "" {
		present++
	}
	if previousDistributionRoot.Path() != "" {
		present++
	}
	if present != 0 && present != 3 {
		return fmt.Errorf("setupのprevious 3 fieldは同時に空または同時に非空でなければならない")
	}
	modeChanges := countWarning(warnings, WarnModeChange)
	if present == 3 && modeChanges != 1 {
		return fmt.Errorf("previous fieldが非空だがW_MODE_CHANGEが%d件である", modeChanges)
	}
	if present == 0 && modeChanges != 0 {
		return fmt.Errorf("previous fieldが空だがW_MODE_CHANGEが%d件ある", modeChanges)
	}
	return nil
}

// checkRestartWarning は§16の「trueと`W_RESTART_REQUIRED` exactly 1件を同値にする」
// を確かめる。
func checkRestartWarning(restartRequired bool, warnings []PlanWarning) error {
	count := countWarning(warnings, WarnRestartRequired)
	if restartRequired && count != 1 {
		return fmt.Errorf("restart_required=trueだがW_RESTART_REQUIREDが%d件である", count)
	}
	if !restartRequired && count != 0 {
		return fmt.Errorf("restart_required=falseだがW_RESTART_REQUIREDが%d件ある", count)
	}
	return nil
}

func countWarning(warnings []PlanWarning, code PlanWarningCode) int {
	count := 0
	for _, warning := range warnings {
		if warning.Code == code {
			count++
		}
	}
	return count
}

func buildPlanWarnings(raws []*planWarningJSON) ([]PlanWarning, error) {
	if raws == nil {
		return nil, fmt.Errorf("warningsが無い")
	}
	entries := make([]PlanWarning, 0, len(raws))
	seen := make(map[PlanWarningCode]struct{}, len(raws))
	for index, raw := range raws {
		if raw == nil {
			return nil, fmt.Errorf("warnings[%d]が空", index)
		}
		prefix := fmt.Sprintf("warnings[%d]", index)
		codeText, err := requirePresent(prefix+".code", raw.Code)
		if err != nil {
			return nil, err
		}
		code := PlanWarningCode(codeText)
		wantApproval, known := planWarningApproval[code]
		if !known {
			return nil, fmt.Errorf("%s.codeが§16.1の8件に無い（%q）", prefix, codeText)
		}
		// 同じcodeを2度出すと、承認単位（code集合）が件数と一致しなくなる。
		if _, duplicate := seen[code]; duplicate {
			return nil, fmt.Errorf("warningsのcode %q が重複している", code)
		}
		seen[code] = struct{}{}
		messageID, err := requireMessageID(prefix+".message_id", raw.MessageID)
		if err != nil {
			return nil, err
		}
		parameters, err := requireScalarMap(prefix+".parameters", raw.Parameters)
		if err != nil {
			return nil, err
		}
		approval, err := requireBool(prefix+".requires_explicit_approval", raw.RequiresExplicitApproval)
		if err != nil {
			return nil, err
		}
		// §16.1の表がcodeごとに承認要否を決める。Plan作成側の判断で変えられると、
		// 同じcodeが場面によって承認を要したり要さなかったりする。
		if approval != wantApproval {
			return nil, fmt.Errorf(
				"%s: code %q のrequires_explicit_approvalは%vでなければならない（%v）",
				prefix, code, wantApproval, approval)
		}
		entries = append(entries, PlanWarning{
			Code: code, MessageID: messageID, Parameters: parameters,
			RequiresExplicitApproval: approval,
		})
	}
	return entries, nil
}
