package install

import (
	"errors"
	"fmt"
	"strings"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/definition"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/store"
)

// buildPlanProbes はdefinitionのvalidation probeを§16の展開済みprobeへ変換する。
//
// docs/04-storage-and-data.md §16「definition probeを**完全展開**した値。
// executable/cwd/write pathは`PathValue`。完全version、artifact URL/digest、
// provider license、理由を空にしない。Plan外probeを実行しない」。
//
// 「完全展開」であることが要点である。Planに残ったtemplateをExecuteが評価すると、
// 利用者が承認した文字列と実際に起動するargvが違いうる。ここですべて解決し、
// Executeは値をそのまま使う。
func buildPlanProbes(req PlanRequest) ([]store.PlanProbe, error) {
	declared := req.Platform.Validation.Probes
	if len(declared) == 0 {
		return nil, nil
	}
	// probe文脈だけが`{{probe_temp}}`を解決できる（docs/06-tool-definition.md §12）。
	// 呼出し側がProbeTempを渡していなければ、`{{probe_temp}}`を含むprobeは
	// [RenderPath]が拒否する。ここで先回りして空へ潰さない。
	roots := req.Roots

	values := make([]store.PlanProbe, 0, len(declared))
	for index := range declared {
		probe, err := buildPlanProbe(req, declared[index], roots)
		if err != nil {
			return nil, fmt.Errorf("install: probe %q: %w", declared[index].ID, err)
		}
		values = append(values, probe)
	}
	return values, nil
}

// buildPlanProbe は1件のprobeを展開する。
func buildPlanProbe(
	req PlanRequest, declared definition.Probe, roots RenderRoots,
) (store.PlanProbe, error) {
	command, ok := lookupCommand(req.Platform.Runtime, declared.RuntimeCommand)
	if !ok {
		// §11のprobeは§10.1で宣言したcommandだけを起動できる。宣言に無い名前を
		// 許すと、Planが定義されていないprogramの起動を要求できる。
		return store.PlanProbe{}, fmt.Errorf(
			"runtime command %q が`[platforms.runtime]`に無い", declared.RuntimeCommand)
	}
	executable, err := RenderPath(command.Target, roots)
	if err != nil {
		return store.PlanProbe{}, fmt.Errorf("target: %w", err)
	}

	args, err := buildPlanArgs(command, declared.Args, roots)
	if err != nil {
		return store.PlanProbe{}, err
	}

	stream, err := convertProbeStream(declared.Stream)
	if err != nil {
		return store.PlanProbe{}, err
	}
	expect, err := convertProbeExpect(declared.Expect)
	if err != nil {
		return store.PlanProbe{}, err
	}

	var expectedVersion string
	if declared.ExpectedVersion != "" {
		// §11の`expected_version`は`{{version}}`だけを取る。path文脈ではないため
		// [RenderText]を使う。
		expectedVersion, err = RenderText(declared.ExpectedVersion, roots)
		if err != nil {
			return store.PlanProbe{}, fmt.Errorf("expected_version: %w", err)
		}
	}

	var expectedRoot *domain.PathValue
	if declared.ExpectedRoot != "" {
		value, rootErr := RenderPath(declared.ExpectedRoot, roots)
		if rootErr != nil {
			return store.PlanProbe{}, fmt.Errorf("expected_root: %w", rootErr)
		}
		expectedRoot = &value
	}

	requiredPaths, err := buildRequiredPaths(declared.RequiredPaths, roots)
	if err != nil {
		return store.PlanProbe{}, err
	}
	reason, err := planMessageID(messageProbeReason)
	if err != nil {
		return store.PlanProbe{}, err
	}

	return store.PlanProbe{
		ID:             declared.ID,
		RuntimeCommand: declared.RuntimeCommand,
		Executable:     executable,
		// §16「完全version、artifact URL/digest、provider license、理由を空にしない」。
		// probeが起動するのは今installするartifactそのものなので、summaryと同じ
		// 値を持たせる。別の値を入れる余地を作らない。
		Version:         req.Item.VersionText,
		Source:          req.Item.ArtifactURL,
		ArtifactDigest:  req.Item.ArtifactDigest,
		License:         req.Platform.Provider.License,
		ReasonMessageID: reason,
		Args:            args,
		// probeのcwdはpayload rootとする。§11はcwdを宣言させないため、
		// payload外を指す余地を作らない一意な選び方はこれだけである。
		WorkingDirectory: roots.Payload,
		// §11のprobeは読取り専用である。書込みを許すのは`{{probe_temp}}`を
		// 渡した場合だけで、渡していなければ空になる。
		WritePaths:      probeWritePaths(roots),
		Stream:          stream,
		Expect:          expect,
		Regex:           declared.Regex,
		ExpectedVersion: expectedVersion,
		ExpectedRoot:    expectedRoot,
		RequiredPaths:   requiredPaths,
		TimeoutMillis:   declared.Timeout.Milliseconds(),
		Required:        declared.Required,
	}, nil
}

// buildPlanArgs は§16の`PlanArg[]`を作る。
//
// 「definitionの1個のargs entryを複数argvへ分割せず、pathをliteralやwarning
// parameterへ埋め込まない」。したがってentry 1件がargv 1要素へ対応し、
// templateを含むentryは`kind=path`、含まないentryは`kind=literal`になる。
//
// commandの宣言argsが先、probe固有のargsが後である。§10.1のcommand argsは
// 常に前置するものであり、順序を入れ替えるとcommandの意味が変わる。
func buildPlanArgs(
	command definition.Command, probeArgs []string, roots RenderRoots,
) ([]store.PlanArg, error) {
	entries := make([]string, 0, len(command.Args)+len(probeArgs))
	entries = append(entries, command.Args...)
	entries = append(entries, probeArgs...)

	args := make([]store.PlanArg, 0, len(entries))
	for _, entry := range entries {
		if !strings.Contains(entry, "{{") {
			args = append(args, store.PlanArg{Kind: store.ArgLiteral, Value: entry})
			continue
		}
		// entry全体が1つのpath templateでなければならない（§11「argsはliteralに
		// 加えて、**entry全体として**`{{payload}}`…を使える」）。部分置換を許すと
		// pathがliteralの一部として埋まり、§16の「pathをliteralへ埋め込まない」に
		// 反する。
		path, err := RenderPath(entry, roots)
		if err != nil {
			return nil, fmt.Errorf("args %q: %w", entry, err)
		}
		args = append(args, store.PlanArg{Kind: store.ArgPath, Path: path})
	}
	return args, nil
}

// buildRequiredPaths は§11の`required_paths`を展開する。
func buildRequiredPaths(
	declared []definition.RequiredPath, roots RenderRoots,
) ([]store.PlanRequiredPath, error) {
	if len(declared) == 0 {
		return nil, nil
	}
	values := make([]store.PlanRequiredPath, 0, len(declared))
	for _, entry := range declared {
		kind, err := convertRequiredPathKind(entry.Kind)
		if err != nil {
			return nil, err
		}
		path, err := RenderPath(entry.Path, roots)
		if err != nil {
			return nil, fmt.Errorf("required_paths %q: %w", entry.Path, err)
		}
		values = append(values, store.PlanRequiredPath{Kind: kind, Path: path})
	}
	return values, nil
}

// probeWritePaths はprobeへ許す書込み先を返す。
//
// §11のprobeは導入物の検証であり、書込みを要するのは`{{probe_temp}}`を使う
// 場合だけである。渡されていなければ0件とし、[Guard]が全書込みを拒否する。
func probeWritePaths(roots RenderRoots) []domain.PathValue {
	if roots.ProbeTemp.IsZero() {
		return nil
	}
	return []domain.PathValue{roots.ProbeTemp}
}

// lookupCommand は§10.1のcommandを名前で引く。
func lookupCommand(runtime definition.Runtime, name string) (definition.Command, bool) {
	for index := range runtime.Commands {
		if runtime.Commands[index].Name == name {
			return runtime.Commands[index], true
		}
	}
	return definition.Command{}, false
}

// messageProbeReason は§16の`reason_message_id`である。
//
// v0.1のprobeは「導入した実体が動作することの確認」だけを理由に持つ。
// definitionへ理由を書かせないため、client側の固定message IDを使う。
const messageProbeReason = "plan.probe_reason"

// planMessageID は固定message IDを型付き値へ変換する。
//
// errorを握り潰さない。IDはsourceの定数であり失敗しないが、握り潰すと
// 定数を書き換えたときにzero値のmessage IDがPlanへ載る。
func planMessageID(id string) (domain.MessageID, error) {
	value, err := domain.ParseMessageID(id)
	if err != nil {
		return domain.MessageID{}, fmt.Errorf("install: message ID %q: %w", id, err)
	}
	return value, nil
}

// 以下はdefinitionのenumをstoreのenumへ移す変換表である。
//
// **string castを使わない。** 両packageは同じ値集合を別の型で持っており、
// castだと`definition`側へ値が増えたときに、storeが知らない値を持つPlanを
// 黙って作れてしまう。表で受けて未知値をerrorにすれば、値が増えたときに
// ここが落ちて追随漏れが分かる。件数の一致は`TestEnumTablesCoverDefinition`が
// 別途固定する。

var providerKinds = map[definition.ArtifactKind]store.ProviderKind{
	definition.KindOfficial:   store.ProviderOfficial,
	definition.KindThirdParty: store.ProviderThirdParty,
}

var archiveFormats = map[definition.ArchiveFormat]store.ArchiveFormat{
	definition.FormatZip:   store.FormatZip,
	definition.FormatTarGz: store.FormatTarGz,
}

var storageKinds = map[definition.StorageKind]store.StorageKind{
	definition.StorageConfig:         store.StorageConfig,
	definition.StorageContentCache:   store.StorageContentCache,
	definition.StorageBuildCache:     store.StorageBuildCache,
	definition.StorageGlobalBin:      store.StorageGlobalBin,
	definition.StorageGlobalPackages: store.StorageGlobalPackages,
	definition.StorageRuntimeData:    store.StorageRuntimeData,
}

var storageScopes = map[definition.StorageScope]store.StorageScope{
	definition.ScopeTool:    store.ScopeTool,
	definition.ScopeVersion: store.ScopeVersion,
}

var storagePurges = map[definition.StoragePurge]store.StoragePurge{
	definition.StorageRetain:      store.PurgeRetain,
	definition.StorageExplicit:    store.PurgeExplicit,
	definition.StorageWithVersion: store.PurgeWithVersion,
}

var probeStreams = map[definition.ProbeStream]store.ProbeStream{
	definition.StreamStdout:   store.StreamStdout,
	definition.StreamStderr:   store.StreamStderr,
	definition.StreamCombined: store.StreamCombined,
}

var probeExpects = map[definition.ProbeExpect]store.ProbeExpect{
	definition.ExpectVersion:    store.ExpectVersion,
	definition.ExpectSuccess:    store.ExpectSuccess,
	definition.ExpectPathWithin: store.ExpectPathWithin,
}

var requiredPathKinds = map[definition.RequiredPathKind]store.RequiredPathKind{
	definition.RequiredFile:      store.RequiredFile,
	definition.RequiredDirectory: store.RequiredDirectory,
}

// convert は変換表を引き、未知値をerrorにする。
func convert[K comparable, V any](table map[K]V, key K, name string) (V, error) {
	value, ok := table[key]
	if !ok {
		var zero V
		return zero, fmt.Errorf("install: 未知の%s %v", name, key)
	}
	return value, nil
}

func convertProviderKind(kind definition.ArtifactKind) (store.ProviderKind, error) {
	if kind == "" {
		return "", errors.New("install: artifact kindが未設定")
	}
	return convert(providerKinds, kind, "artifact kind")
}

func convertArchiveFormat(format definition.ArchiveFormat) (store.ArchiveFormat, error) {
	return convert(archiveFormats, format, "archive format")
}

func convertStorageKind(kind definition.StorageKind) (store.StorageKind, error) {
	return convert(storageKinds, kind, "storage kind")
}

func convertStorageScope(scope definition.StorageScope) (store.StorageScope, error) {
	return convert(storageScopes, scope, "storage scope")
}

func convertStoragePurge(purge definition.StoragePurge) (store.StoragePurge, error) {
	return convert(storagePurges, purge, "storage purge")
}

func convertProbeStream(stream definition.ProbeStream) (store.ProbeStream, error) {
	return convert(probeStreams, stream, "probe stream")
}

func convertProbeExpect(expect definition.ProbeExpect) (store.ProbeExpect, error) {
	return convert(probeExpects, expect, "probe expect")
}

func convertRequiredPathKind(kind definition.RequiredPathKind) (store.RequiredPathKind, error) {
	return convert(requiredPathKinds, kind, "required path kind")
}
