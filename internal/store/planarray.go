package store

import (
	"fmt"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
)

func buildPlanDownloads(raws []*planDownloadJSON) ([]PlanDownload, map[string]struct{}, error) {
	if raws == nil {
		return nil, nil, fmt.Errorf("downloadsが無い")
	}
	entries := make([]PlanDownload, 0, len(raws))
	ids := make(map[string]struct{}, len(raws))
	for index, raw := range raws {
		if raw == nil {
			return nil, nil, fmt.Errorf("downloads[%d]が空", index)
		}
		entry, err := buildPlanDownload(index, *raw)
		if err != nil {
			return nil, nil, err
		}
		entries = append(entries, entry)
		ids[entry.ID] = struct{}{}
	}
	if err := requireSortedUnique("downloads", len(entries), func(i int) string {
		return entries[i].ID
	}); err != nil {
		return nil, nil, err
	}
	return entries, ids, nil
}

func buildPlanDownload(index int, raw planDownloadJSON) (PlanDownload, error) {
	var value PlanDownload
	prefix := fmt.Sprintf("downloads[%d]", index)
	id, err := requireIdentifier(prefix+".id", raw.ID)
	if err != nil {
		return value, err
	}
	providerKind, err := requireEnum(prefix+".provider_kind", raw.ProviderKind, receiptProviderKinds)
	if err != nil {
		return value, err
	}
	name, err := requireNonEmpty(prefix+".provider_name", raw.ProviderName)
	if err != nil {
		return value, err
	}
	repository, err := requirePresent(prefix+".provider_repository", raw.ProviderRepository)
	if err != nil {
		return value, err
	}
	homepage, err := requirePresent(prefix+".provider_homepage", raw.ProviderHomepage)
	if err != nil {
		return value, err
	}
	release, err := requireNonEmpty(prefix+".provider_release", raw.ProviderRelease)
	if err != nil {
		return value, err
	}
	urlText, err := requirePresent(prefix+".url", raw.URL)
	if err != nil {
		return value, err
	}
	downloadURL, err := requireHTTPSURL(prefix+".url", urlText)
	if err != nil {
		return value, err
	}
	fileName, err := requireNonEmpty(prefix+".file_name", raw.FileName)
	if err != nil {
		return value, err
	}
	if _, err := requireFileName(prefix+".file_name", fileName); err != nil {
		return value, err
	}
	size, err := requireInt64(prefix+".size", raw.Size)
	if err != nil {
		return value, err
	}
	digestText, err := requirePresent(prefix+".expected_digest", raw.ExpectedDigest)
	if err != nil {
		return value, err
	}
	digest, err := parseUpstreamDigest(prefix+".expected_digest", digestText)
	if err != nil {
		return value, err
	}
	source, err := requireEnum(prefix+".checksum_source", raw.ChecksumSource, checksumSources)
	if err != nil {
		return value, err
	}
	license, err := requirePresent(prefix+".license", raw.License)
	if err != nil {
		return value, err
	}
	reason, err := requirePresent(prefix+".adoption_reason_message_id", raw.AdoptionReasonMessageID)
	if err != nil {
		return value, err
	}
	// §16が「officialのadoption reasonだけ空」と定める。third-partyは採用理由を
	// 必ず表示する（10-security.md §8）。
	if providerKind == ProviderOfficial {
		if err := requireEmpty(prefix+".adoption_reason_message_id", reason); err != nil {
			return value, err
		}
	} else {
		if _, err := requireMessageID(prefix+".adoption_reason_message_id", raw.AdoptionReasonMessageID); err != nil {
			return value, err
		}
		for _, pair := range []struct{ name, text string }{
			{prefix + ".provider_repository", repository},
			{prefix + ".provider_homepage", homepage},
			{prefix + ".license", license},
		} {
			if pair.text == "" {
				return value, fmt.Errorf("third-party downloadの%sが空", pair.name)
			}
		}
	}
	destination, err := buildPathValue(prefix+".destination", raw.Destination, "", pathAbsolute)
	if err != nil {
		return value, err
	}
	if _, ok := downloadDestinationRoles[destination.Role()]; !ok {
		return value, fmt.Errorf(
			"%s.destination.roleは`download-cache|staging`だけ（%q）", prefix, destination.Role())
	}
	return PlanDownload{
		ID: id, ProviderKind: providerKind, ProviderName: name,
		ProviderRepository: repository, ProviderHomepage: homepage, ProviderRelease: release,
		URL: downloadURL, FileName: fileName, Size: size, ExpectedDigest: digest,
		ChecksumSource: source, License: license, AdoptionReasonMessage: reason,
		Destination: destination,
	}, nil
}

func buildPlanExtracts(
	raws []*planExtractJSON, downloadIDs map[string]struct{},
) ([]PlanExtract, error) {
	if raws == nil {
		return nil, fmt.Errorf("extractsが無い")
	}
	entries := make([]PlanExtract, 0, len(raws))
	for index, raw := range raws {
		if raw == nil {
			return nil, fmt.Errorf("extracts[%d]が空", index)
		}
		prefix := fmt.Sprintf("extracts[%d]", index)
		id, err := requireIdentifier(prefix+".id", raw.ID)
		if err != nil {
			return nil, err
		}
		sourceID, err := requireIdentifier(prefix+".source_download_id", raw.SourceDownloadID)
		if err != nil {
			return nil, err
		}
		// §16が「`source_download_id`は同じPlanのdownload ID」と定める。
		// 存在しないIDを参照するextractは実行できない。
		if _, known := downloadIDs[sourceID]; !known {
			return nil, fmt.Errorf("%s.source_download_id %q が同じPlanのdownloadに無い", prefix, sourceID)
		}
		format, err := requireEnum(prefix+".format", raw.Format, archiveFormats)
		if err != nil {
			return nil, err
		}
		strip, err := requireInt64(prefix+".strip_components", raw.StripComponents)
		if err != nil {
			return nil, err
		}
		// §16が「stripは`0|1`」と定める。2以上を許すとarchiveの構造に依存した
		// 展開になり、containment検査の前提が崩れる。
		if strip != 0 && strip != 1 {
			return nil, fmt.Errorf("%s.strip_componentsは0か1だけ（%d）", prefix, strip)
		}
		destination, err := buildPathValue(
			prefix+".destination", raw.Destination, domain.RoleStaging, pathAbsolute)
		if err != nil {
			return nil, err
		}
		entries = append(entries, PlanExtract{
			ID: id, SourceDownloadID: sourceID, Format: format,
			StripComponents: strip, Destination: destination,
		})
	}
	if err := requireSortedUnique("extracts", len(entries), func(i int) string {
		return entries[i].ID
	}); err != nil {
		return nil, err
	}
	return entries, nil
}

func buildPlanProbes(raws []*planProbeJSON) ([]PlanProbe, error) {
	if raws == nil {
		return nil, fmt.Errorf("probesが無い")
	}
	entries := make([]PlanProbe, 0, len(raws))
	for index, raw := range raws {
		if raw == nil {
			return nil, fmt.Errorf("probes[%d]が空", index)
		}
		entry, err := buildPlanProbe(index, *raw)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	if err := requireSortedUnique("probes", len(entries), func(i int) string {
		return entries[i].ID
	}); err != nil {
		return nil, err
	}
	return entries, nil
}

func buildPlanProbe(index int, raw planProbeJSON) (PlanProbe, error) {
	var value PlanProbe
	prefix := fmt.Sprintf("probes[%d]", index)
	id, err := requireIdentifier(prefix+".id", raw.ID)
	if err != nil {
		return value, err
	}
	runtimeCommand, err := requireCommandName(prefix+".runtime_command", raw.RuntimeCommand)
	if err != nil {
		return value, err
	}
	executable, err := buildPathValue(prefix+".executable", raw.Executable, "", pathAbsolute)
	if err != nil {
		return value, err
	}
	// §16が「完全version、artifact URL/digest、provider license、理由を空にしない」と
	// 定める。子processを起動する前に、何を起動するかを利用者へ完全に示すためである。
	version, err := requireNonEmpty(prefix+".version", raw.Version)
	if err != nil {
		return value, err
	}
	if err := requireExactVersion(prefix+".version", version); err != nil {
		return value, err
	}
	sourceText, err := requirePresent(prefix+".source", raw.Source)
	if err != nil {
		return value, err
	}
	source, err := requireHTTPSURL(prefix+".source", sourceText)
	if err != nil {
		return value, err
	}
	digestText, err := requirePresent(prefix+".artifact_digest", raw.ArtifactDigest)
	if err != nil {
		return value, err
	}
	digest, err := parseUpstreamDigest(prefix+".artifact_digest", digestText)
	if err != nil {
		return value, err
	}
	license, err := requireNonEmpty(prefix+".license", raw.License)
	if err != nil {
		return value, err
	}
	reason, err := requireMessageID(prefix+".reason_message_id", raw.ReasonMessageID)
	if err != nil {
		return value, err
	}
	args, err := buildPlanArgs(prefix, raw.Args)
	if err != nil {
		return value, err
	}
	workingDirectory, err := buildPathValue(
		prefix+".working_directory", raw.WorkingDirectory, "", pathAbsolute)
	if err != nil {
		return value, err
	}
	writePaths, err := buildPathList(prefix+".write_paths", raw.WritePaths)
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
	// §16が「`expected_root`は`PathValue|null`」と定める。nullは非該当を表す。
	var expectedRoot *domain.PathValue
	if raw.ExpectedRoot != nil {
		root, err := buildPathValue(prefix+".expected_root", raw.ExpectedRoot, "", pathAbsolute)
		if err != nil {
			return value, err
		}
		expectedRoot = &root
	}
	requiredPaths, err := buildPlanRequiredPaths(prefix, raw.RequiredPaths)
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
	rootText := ""
	if expectedRoot != nil {
		rootText = expectedRoot.Path()
	}
	if err := checkProbeExpectFields(prefix, expect, regex, expectedVersion, rootText, ""); err != nil {
		return value, err
	}
	return PlanProbe{
		ID: id, RuntimeCommand: runtimeCommand, Executable: executable, Version: version,
		Source: source, ArtifactDigest: digest, License: license, ReasonMessageID: reason,
		Args: args, WorkingDirectory: workingDirectory, WritePaths: writePaths,
		Stream: stream, Expect: expect, Regex: regex, ExpectedVersion: expectedVersion,
		ExpectedRoot: expectedRoot, RequiredPaths: requiredPaths,
		TimeoutMillis: timeout, Required: required,
	}, nil
}

// buildPlanArgs は§16の`PlanArg`を読む。
//
// 「`kind=literal`では`value`をそのままargv 1要素とし`path=null`、`kind=path`では
// `value`を空、`path`を非空の`PathValue`とし、そのnative pathをargv 1要素とする」。
func buildPlanArgs(prefix string, raws *[]*planArgJSON) ([]PlanArg, error) {
	if raws == nil {
		return nil, fmt.Errorf("%s.argsが無い", prefix)
	}
	entries := make([]PlanArg, 0, len(*raws))
	for index, raw := range *raws {
		if raw == nil {
			return nil, fmt.Errorf("%s.args[%d]が空", prefix, index)
		}
		item := fmt.Sprintf("%s.args[%d]", prefix, index)
		kind, err := requireEnum(item+".kind", raw.Kind, planArgKinds)
		if err != nil {
			return nil, err
		}
		text, err := requirePresent(item+".value", raw.Value)
		if err != nil {
			return nil, err
		}
		switch kind {
		case ArgLiteral:
			if text == "" {
				return nil, fmt.Errorf("%s: kind=literalのvalueが空", item)
			}
			if raw.Path != nil {
				return nil, fmt.Errorf("%s: kind=literalのpathはnullでなければならない", item)
			}
			entries = append(entries, PlanArg{Kind: kind, Value: text})
		case ArgPath:
			if err := requireEmpty(item+".value", text); err != nil {
				return nil, err
			}
			path, err := buildPathValue(item+".path", raw.Path, "", pathAbsolute)
			if err != nil {
				return nil, err
			}
			entries = append(entries, PlanArg{Kind: kind, Path: path})
		}
	}
	return entries, nil
}

func buildPlanRequiredPaths(prefix string, raws *[]*planRequiredPathJSON) ([]PlanRequiredPath, error) {
	if raws == nil {
		return nil, fmt.Errorf("%s.required_pathsが無い", prefix)
	}
	entries := make([]PlanRequiredPath, 0, len(*raws))
	for index, raw := range *raws {
		if raw == nil {
			return nil, fmt.Errorf("%s.required_paths[%d]が空", prefix, index)
		}
		item := fmt.Sprintf("%s.required_paths[%d]", prefix, index)
		kind, err := requireEnum(item+".kind", raw.Kind, requiredPathKinds)
		if err != nil {
			return nil, err
		}
		path, err := buildPathValue(item+".path", raw.Path, "", pathAbsolute)
		if err != nil {
			return nil, err
		}
		entries = append(entries, PlanRequiredPath{Kind: kind, Path: path})
	}
	return entries, nil
}

func buildPathList(field string, raws *[]*pathValueJSON) ([]domain.PathValue, error) {
	if raws == nil {
		return nil, fmt.Errorf("%sが無い", field)
	}
	entries := make([]domain.PathValue, 0, len(*raws))
	for index, raw := range *raws {
		path, err := buildPathValue(fmt.Sprintf("%s[%d]", field, index), raw, "", pathAbsolute)
		if err != nil {
			return nil, err
		}
		entries = append(entries, path)
	}
	return entries, nil
}

func buildPlanWrites(raws []*planWriteJSON) ([]PlanWrite, error) {
	if raws == nil {
		return nil, fmt.Errorf("writesが無い")
	}
	entries := make([]PlanWrite, 0, len(raws))
	for index, raw := range raws {
		if raw == nil {
			return nil, fmt.Errorf("writes[%d]が空", index)
		}
		prefix := fmt.Sprintf("writes[%d]", index)
		id, err := requireIdentifier(prefix+".id", raw.ID)
		if err != nil {
			return nil, err
		}
		action, err := requireEnum(prefix+".action", raw.Action, writeActions)
		if err != nil {
			return nil, err
		}
		// §17.2の唯一の例外として、registry-valueだけがlocatorを持てる。
		mode := pathAbsolute
		if action == WriteRegistryValue {
			mode = pathLocatorOrAbsolute
		}
		target, err := buildPathValue(prefix+".target", raw.Target, "", mode)
		if err != nil {
			return nil, err
		}
		if action == WriteRegistryValue && target.Role() != domain.RoleConfig {
			return nil, fmt.Errorf("%s: action=registry-valueのtarget.roleは`config`だけ（%q）", prefix, target.Role())
		}
		entries = append(entries, PlanWrite{ID: id, Action: action, Target: target})
	}
	if err := requireSortedUnique("writes", len(entries), func(i int) string {
		return entries[i].ID
	}); err != nil {
		return nil, err
	}
	return entries, nil
}

func buildPlanStorage(raws []*planStorageJSON) ([]PlanStorage, error) {
	if raws == nil {
		return nil, fmt.Errorf("storageが無い")
	}
	entries := make([]PlanStorage, 0, len(raws))
	for index, raw := range raws {
		if raw == nil {
			return nil, fmt.Errorf("storage[%d]が空", index)
		}
		prefix := fmt.Sprintf("storage[%d]", index)
		id, err := requireIdentifier(prefix+".id", raw.ID)
		if err != nil {
			return nil, err
		}
		kind, err := requireEnum(prefix+".kind", raw.Kind, storageKinds)
		if err != nil {
			return nil, err
		}
		scope, err := requireEnum(prefix+".scope", raw.Scope, storageScopes)
		if err != nil {
			return nil, err
		}
		target, err := buildPathValue(prefix+".target", raw.Target, "", pathAbsolute)
		if err != nil {
			return nil, err
		}
		purge, err := requireEnum(prefix+".purge", raw.Purge, storagePurges)
		if err != nil {
			return nil, err
		}
		if err := checkStorageScopePurge(prefix, scope, purge); err != nil {
			return nil, err
		}
		action, err := requireEnum(prefix+".action", raw.Action, storageActions)
		if err != nil {
			return nil, err
		}
		entries = append(entries, PlanStorage{
			ID: id, Kind: kind, Scope: scope, Target: target, Purge: purge, Action: action,
		})
	}
	if err := requireSortedUnique("storage", len(entries), func(i int) string {
		return entries[i].ID
	}); err != nil {
		return nil, err
	}
	return entries, nil
}
