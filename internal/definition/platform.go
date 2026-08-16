package definition

import (
	"fmt"
	"sort"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
)

// buildPlatforms は§5の`[[platforms]]`を検証してmodelへ入れる。
//
// platformは1件以上、[PlatformMax]件以下、ID一意（§2・§21）。
func buildPlatforms(
	tables []platformTable, scheme domain.VersionScheme, value *Definition, diagnostics *Diagnostics,
) {
	switch {
	case len(tables) == 0:
		diagnostics.Add("platforms", reason(reasonMissing), "`[[platforms]]`が1件も無い")
		return
	case len(tables) > PlatformMax:
		diagnostics.Add("platforms", reason(reasonLimit),
			fmt.Sprintf("platformが%d件を超える（%d件）", PlatformMax, len(tables)))
		return
	}
	ids := make([]string, 0, len(tables))
	for index := range tables {
		field := fmt.Sprintf("platforms[%d]", index)
		platform := buildPlatform(&tables[index], field, scheme, diagnostics)
		if !platform.Platform.IsZero() {
			ids = append(ids, platform.Platform.ID())
		}
		value.Platforms = append(value.Platforms, platform)
	}
	// §5は「同一tupleは1件」と定める。IDとtupleは1対1であるためID一意で足りる。
	if err := requireUniqueIdentifiers("platform ID", ids); err != nil {
		diagnostics.Add("platforms", reason(reasonDuplicate), err.Error())
	}
}

func buildPlatform(
	table *platformTable, field string, scheme domain.VersionScheme, diagnostics *Diagnostics,
) Platform {
	var value Platform
	value.Platform = buildPlatformTuple(table, field, diagnostics)
	value.ArtifactKind = buildArtifactKind(table.ArtifactKind, field, diagnostics)
	value.LicenseNotice = buildLicenseNotice(table.LicenseNotice, field, diagnostics)
	value.Provider = buildProvider(table.Provider, field, value.ArtifactKind, diagnostics)
	value.VersionSource = buildVersionSource(
		table.VersionSource, field+".version_source", scheme, diagnostics)

	// §13の検証順序に従い、storage → install → runtime → probeの順で読む。
	// template検査がstorage IDの宣言集合を要るため、storageを先に確定させる。
	value.Storage = buildStorages(table.Storage, field+".storage", diagnostics)
	value.Install = buildInstall(table.Install, field+".install", diagnostics)

	context := newTemplateContext(value.Storage, value.VersionSource)
	value.Artifact = buildArtifact(
		table.Artifact, field+".artifact", context, value.VersionSource, diagnostics)
	windows := value.Platform.OS() == domain.OSWindows
	value.Runtime = buildRuntime(table.Runtime, field+".runtime", context, windows, diagnostics)
	value.Validation = buildValidation(
		table.Validation, field+".validation", context, value.Runtime.Commands, diagnostics)
	return value
}

// newTemplateContext は§12のtemplate検査に要る宣言済み集合を組み立てる。
func newTemplateContext(storages []Storage, source VersionSource) templateContext {
	context := templateContext{
		storageIDs:   make(map[string]struct{}, len(storages)),
		metadataKeys: make(map[string]struct{}, len(source.MetadataFields)),
		assetFields:  make(map[AssetField]struct{}, len(source.AssetFields)),
	}
	for _, storage := range storages {
		context.storageIDs[storage.ID] = struct{}{}
	}
	for key := range source.MetadataFields {
		context.metadataKeys[key] = struct{}{}
	}
	for field := range source.AssetFields {
		context.assetFields[field] = struct{}{}
	}
	// static sourceはassetを直接書くため、`asset_fields`の宣言を持たない。
	// §6.6が13 field全件必須であることから、全fieldを宣言済みとして扱う。
	if source.Kind == SourceStatic {
		for _, field := range assetFieldOrder {
			context.assetFields[field] = struct{}{}
		}
	}
	return context
}

// checkStaticVersionSets は§6.6の両platform一致を検査する。
//
// 「`version_source`はplatform配下にあるため、static sourceは両platformへ同じ
// version集合を記述する。registry validatorは両platformの正規version集合が
// 完全一致することを検査し、片方だけの更新漏れを拒否する」。
func checkStaticVersionSets(value *Definition, diagnostics *Diagnostics) {
	var reference []string
	referenceIndex := -1
	for index, platform := range value.Platforms {
		if platform.VersionSource.Kind != SourceStatic {
			continue
		}
		versions := make([]string, 0, len(platform.VersionSource.StaticVersions))
		for _, static := range platform.VersionSource.StaticVersions {
			versions = append(versions, static.Version.String())
		}
		sort.Strings(versions)
		if referenceIndex < 0 {
			reference, referenceIndex = versions, index
			continue
		}
		if !equalStrings(reference, versions) {
			diagnostics.Add(
				fmt.Sprintf("platforms[%d].version_source.static_versions", index),
				reason(reasonPlatformSet),
				fmt.Sprintf("platforms[%d]とversion集合が一致しない（%v / %v）",
					referenceIndex, reference, versions))
		}
	}
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

// buildPlatformTuple は§5のIDとOS/arch/libcの一致を検査する（§13-4）。
//
// IDから導いたtupleと、fileが書いた個別fieldの両方を突き合わせる。IDだけを見ると
// definitionが誤ったOSを書いても気付けず、個別fieldだけを見ると仕様表にない組を
// 受理してしまう。
func buildPlatformTuple(table *platformTable, field string, diagnostics *Diagnostics) domain.Platform {
	if table.ID == nil {
		diagnostics.Add(field+".id", reason(reasonMissing), "`id`が無い")
		return domain.Platform{}
	}
	platform, err := domain.ParsePlatform(*table.ID)
	if err != nil {
		diagnostics.Add(field+".id", reason(reasonEnum), err.Error())
		return domain.Platform{}
	}
	expected := []struct {
		key   string
		raw   *string
		value string
	}{
		{"os", table.OS, string(platform.OS())},
		{"arch", table.Arch, string(platform.Arch())},
		{"libc", table.Libc, string(platform.Libc())},
	}
	consistent := true
	for _, entry := range expected {
		switch {
		case entry.raw == nil:
			diagnostics.Add(field+"."+entry.key, reason(reasonMissing),
				fmt.Sprintf("`%s`が無い", entry.key))
			consistent = false
		case *entry.raw != entry.value:
			diagnostics.Add(field+"."+entry.key, reason(reasonPlatformTuple),
				fmt.Sprintf("platform %q の`%s`は%qでなければならない（%q）",
					platform.ID(), entry.key, entry.value, *entry.raw))
			consistent = false
		}
	}
	if !consistent {
		return domain.Platform{}
	}
	return platform
}

func buildArtifactKind(raw *string, field string, diagnostics *Diagnostics) ArtifactKind {
	if raw == nil {
		diagnostics.Add(field+".artifact_kind", reason(reasonMissing), "`artifact_kind`が無い")
		return ""
	}
	switch ArtifactKind(*raw) {
	case KindOfficial, KindThirdParty:
		return ArtifactKind(*raw)
	default:
		diagnostics.Add(field+".artifact_kind", reason(reasonEnum),
			fmt.Sprintf("artifact_kindは%s|%sだけ（%q）", KindOfficial, KindThirdParty, *raw))
		return ""
	}
}

// buildLicenseNotice は§5の`license_notice`を検査する（§13-4）。
//
// 値はmessage IDである。文そのものをdefinitionへ書かせないのは、表示文をcatalog
// で管理し、CLI human表示とJSON envelopeで同じ文言になるようにするためである
// （docs/02-architecture.md §14）。
//
// **宣言の要否は検査しない。** 「OSI承認OSS licenseでない配布物へ宣言する」の
// 判定にはSPDX/OSIのlicense listが要る。listをclientへ同梱しない判断
// （[checkLicenseExpression]）に従い、この対応はdocs/07-registry-and-tools.md §5
// 項目9のregistry reviewとP4-01のcontract testが担う。
func buildLicenseNotice(raw *string, field string, diagnostics *Diagnostics) domain.MessageID {
	if raw == nil {
		return domain.MessageID{}
	}
	messageID, err := domain.ParseMessageID(*raw)
	if err != nil {
		diagnostics.Add(field+".license_notice", reason(reasonMessageID), err.Error())
		return domain.MessageID{}
	}
	return messageID
}

// buildProvider は§5.1の`provider`を検査する。
//
// 許可keyは`name`, `repository`, `homepage`, `license`, `adoption_reason`。
// officialとthird-partyで必須・禁止が変わる唯一のtableである。
func buildProvider(
	table *providerTable, field string, kind ArtifactKind, diagnostics *Diagnostics,
) Provider {
	scope := field + ".provider"
	if table == nil {
		diagnostics.Add(scope, reason(reasonMissing), "`[platforms.provider]`が無い")
		return Provider{}
	}
	value := Provider{
		Name:     requireText(table.Name, scope+".name", 1, NameMaxBytes, diagnostics),
		Homepage: requireHTTPSURL(table.Homepage, scope+".homepage", urlReference, diagnostics),
		License:  requireLicense(table.License, scope+".license", diagnostics),
	}
	value.Repository = optionalHTTPSURL(table.Repository, scope+".repository", urlReference, diagnostics)

	switch kind {
	case KindOfficial:
		// officialのadoption_reasonを禁じるのは、公式配布物に「採用理由」を
		// 書かせると、third-partyだけに求めている説明責任が薄まるためである。
		if table.AdoptionReason != nil {
			diagnostics.Add(scope+".adoption_reason", reason(reasonProviderKey),
				"artifact_kind=officialでは`adoption_reason`を書けない")
		}
	case KindThirdParty:
		// third-partyは全件必須。Planでprovider、repository、license、
		// adoption_reasonを常に表示する（§5.1、docs/10-security.md §8）。
		if table.Repository == nil {
			diagnostics.Add(scope+".repository", reason(reasonMissing),
				"artifact_kind=third-partyでは`repository`が必須")
		}
		value.AdoptionReason = requireText(
			table.AdoptionReason, scope+".adoption_reason", 1, DescriptionMaxBytes, diagnostics)
	default:
		// artifact_kindが決まらなければ条件付きkeyの可否を判定できない。
		// kind側の診断が既に出ているため、ここでは追加で報告しない。
	}
	return value
}
