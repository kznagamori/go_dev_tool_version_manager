package definition

import (
	"fmt"
	"strings"
	"time"
)

// ProbeStream は§11のprobe出力streamである。
type ProbeStream string

// ProbeStream の値。
const (
	StreamStdout   ProbeStream = "stdout"
	StreamStderr   ProbeStream = "stderr"
	StreamCombined ProbeStream = "combined"
)

// ProbeExpect は§11のprobe判定方式である。
type ProbeExpect string

// ProbeExpect の値。
const (
	// ExpectVersion はregexで取り出したversionが`{{version}}`と一致することを要求する。
	ExpectVersion ProbeExpect = "version"
	// ExpectSuccess はexit code 0を要求する。
	ExpectSuccess ProbeExpect = "success"
	// ExpectPathWithin はregexで取り出したpathが指定root内にあることを要求する。
	ExpectPathWithin ProbeExpect = "path-within"
)

// RequiredPathKind は§11の`required_paths` entry種別である。
type RequiredPathKind string

// RequiredPathKind の値。
const (
	RequiredFile      RequiredPathKind = "file"
	RequiredDirectory RequiredPathKind = "directory"
)

// probe timeoutの範囲（§11「timeoutは1s～2m」）。
const (
	ProbeTimeoutMin = time.Second
	ProbeTimeoutMax = 2 * time.Minute
)

// expectedRootPayload / expectedRootProbeTemp は§11の`expected_root`が取る
// root名である。`storage.<id>`はprefixで判定する。
//
// templateの`{{...}}`形ではなくroot名そのものを書く点がargsと違う。
const (
	expectedRootPayload   = "payload"
	expectedRootProbeTemp = "probe-temp"
	expectedRootStorage   = "storage."
)

// Validation は§11の`[platforms.validation]`である。
type Validation struct {
	Probes []Probe
}

// Probe は§11の`[[platforms.validation.probes]]`である。
//
// `id`, `runtime_command`, `args`, `stream`, `expect`, `timeout`, `required`が
// 全件必須で、残りは`expect`ごとの条件付きである。
type Probe struct {
	ID             string
	RuntimeCommand string
	Args           []string
	Stream         ProbeStream
	Expect         ProbeExpect
	// Regex は`version`と`path-within`で必須、`success`では任意である。
	Regex string
	// ExpectedVersion は`version`で必須である。`{{version}}`だけを取る。
	ExpectedVersion string
	// ExpectedRoot は`path-within`で必須である。
	ExpectedRoot string
	// RequiredPaths はprobe成功直後に存在を要求するpathである。
	RequiredPaths []RequiredPath
	Timeout       time.Duration
	Required      bool
}

// RequiredPath は§11の`required_paths` 1件である。
//
// `file:<template>|directory:<template>`のstringとして書く。
type RequiredPath struct {
	Kind RequiredPathKind
	Path string
}

type validationTable struct {
	Probes *[]probeTable `toml:"probes"`
}

type probeTable struct {
	ID              *string   `toml:"id"`
	RuntimeCommand  *string   `toml:"runtime_command"`
	Args            *[]string `toml:"args"`
	Stream          *string   `toml:"stream"`
	Expect          *string   `toml:"expect"`
	Regex           *string   `toml:"regex"`
	ExpectedVersion *string   `toml:"expected_version"`
	ExpectedRoot    *string   `toml:"expected_root"`
	RequiredPaths   *[]string `toml:"required_paths"`
	Timeout         *string   `toml:"timeout"`
	Required        *bool     `toml:"required"`
}

// buildValidation は§11の`validation`を検証する（§13-10）。
func buildValidation(
	table *validationTable, field string, context templateContext,
	commands []Command, diagnostics *Diagnostics,
) Validation {
	var value Validation
	if table == nil {
		diagnostics.Add(field, reason(reasonMissing), "`[platforms.validation]`が無い")
		return value
	}
	if table.Probes == nil {
		diagnostics.Add(field+".probes", reason(reasonMissing), "`probes`が無い")
		return value
	}
	entries := *table.Probes
	if len(entries) > ProbeMax {
		diagnostics.Add(field+".probes", reason(reasonLimit),
			fmt.Sprintf("probeが%d件を超える（%d件）", ProbeMax, len(entries)))
		return value
	}
	commandNames := make(map[string]struct{}, len(commands))
	for _, command := range commands {
		commandNames[command.Name] = struct{}{}
	}
	ids := make([]string, 0, len(entries))
	for index := range entries {
		scope := fmt.Sprintf("%s.probes[%d]", field, index)
		probe, ok := buildProbe(&entries[index], scope, context, commandNames, diagnostics)
		if !ok {
			continue
		}
		value.Probes = append(value.Probes, probe)
		ids = append(ids, probe.ID)
	}
	if err := requireUniqueIdentifiers("probe ID", ids); err != nil {
		diagnostics.Add(field+".probes", reason(reasonDuplicate), err.Error())
		return Validation{}
	}
	return value
}

func buildProbe(
	table *probeTable, field string, context templateContext,
	commandNames map[string]struct{}, diagnostics *Diagnostics,
) (Probe, bool) {
	var value Probe
	ok := true

	if table.ID == nil {
		diagnostics.Add(field+".id", reason(reasonMissing), "`id`が無い")
		ok = false
	} else if err := ValidateScopedID(*table.ID); err != nil {
		diagnostics.Add(field+".id", reason(reasonIdentifier), err.Error())
		ok = false
	} else {
		value.ID = *table.ID
	}

	// probeは宣言済みruntime commandを通してだけ起動する。Plan外のprocessを
	// 実行しない（docs/10-security.md §13）。
	if table.RuntimeCommand == nil {
		diagnostics.Add(field+".runtime_command", reason(reasonMissing), "`runtime_command`が無い")
		ok = false
	} else if _, declared := commandNames[*table.RuntimeCommand]; !declared {
		diagnostics.Add(field+".runtime_command", reason(reasonConditional),
			fmt.Sprintf("runtime command %q がこのplatformに宣言されていない", *table.RuntimeCommand))
		ok = false
	} else {
		value.RuntimeCommand = *table.RuntimeCommand
	}

	if args, argsOK := buildArgs(table.Args, field+".args", context, probeArgScope, diagnostics); argsOK {
		value.Args = args
	} else {
		ok = false
	}
	if stream, streamOK := requireEnumText(
		table.Stream, field+".stream", parseProbeStream, diagnostics); streamOK {
		value.Stream = stream
	} else {
		ok = false
	}
	if required, boolOK := requireBool(table.Required, field+".required", diagnostics); boolOK {
		value.Required = required
	} else {
		ok = false
	}
	if timeout, timeoutOK := requireProbeTimeout(table.Timeout, field+".timeout", diagnostics); timeoutOK {
		value.Timeout = timeout
	} else {
		ok = false
	}
	if paths, pathsOK := buildRequiredPaths(
		table.RequiredPaths, field+".required_paths", context, diagnostics); pathsOK {
		value.RequiredPaths = paths
	} else {
		ok = false
	}

	expect, expectOK := requireEnumText(table.Expect, field+".expect", parseProbeExpect, diagnostics)
	if !expectOK {
		return value, false
	}
	value.Expect = expect
	if !buildProbeExpect(table, field, expect, context, &value, diagnostics) {
		ok = false
	}
	return value, ok
}

// buildProbeExpect は§11の`expect`別の必須・禁止fieldを検査する。
//
// 表で分岐を閉じるのは、expectが増減したときにどのfieldの扱いが漏れたかを
// 読み取れるようにするためである。
func buildProbeExpect(
	table *probeTable, field string, expect ProbeExpect,
	context templateContext, value *Probe, diagnostics *Diagnostics,
) bool {
	ok := true
	switch expect {
	case ExpectVersion:
		// 「regexとexpected_version必須。RE2 named capture`version` exactly 1件」
		if regex, regexOK := requireCaptureRegex(
			table.Regex, field+".regex", "version", diagnostics); regexOK {
			value.Regex = regex
		} else {
			ok = false
		}
		if table.ExpectedVersion == nil {
			diagnostics.Add(field+".expected_version", reason(reasonMissing),
				"expect=versionでは`expected_version`が必須")
			ok = false
		} else if err := context.checkPathTemplate(
			field+".expected_version", *table.ExpectedVersion, expectedVersionScope); err != nil {
			diagnostics.Add(field+".expected_version", reason(reasonTemplate), err.Error())
			ok = false
		} else {
			value.ExpectedVersion = *table.ExpectedVersion
		}
		if table.ExpectedRoot != nil {
			diagnostics.Add(field+".expected_root", reason(reasonConditional),
				"expect=versionでは`expected_root`を書けない")
			ok = false
		}
	case ExpectSuccess:
		// 「exit code 0を要求。regexは指定時に完全matchを1件以上要求。
		// expected fields禁止」
		if table.Regex != nil {
			if err := checkRegexSyntax(*table.Regex, field+".regex"); err != nil {
				diagnostics.Add(field+".regex", reason(reasonRegex), err.Error())
				ok = false
			} else {
				value.Regex = *table.Regex
			}
		}
		for _, forbidden := range []struct {
			key string
			set bool
		}{
			{"expected_version", table.ExpectedVersion != nil},
			{"expected_root", table.ExpectedRoot != nil},
		} {
			if forbidden.set {
				diagnostics.Add(field+"."+forbidden.key, reason(reasonConditional),
					fmt.Sprintf("expect=successでは`%s`を書けない", forbidden.key))
				ok = false
			}
		}
	case ExpectPathWithin:
		// 「regexとexpected_root必須。named capture`path` exactly 1件を
		// absolute path化し、指定root内にcontain」
		if regex, regexOK := requireCaptureRegex(
			table.Regex, field+".regex", "path", diagnostics); regexOK {
			value.Regex = regex
		} else {
			ok = false
		}
		if root, rootOK := requireExpectedRoot(
			table.ExpectedRoot, field+".expected_root", context, diagnostics); rootOK {
			value.ExpectedRoot = root
		} else {
			ok = false
		}
		if table.ExpectedVersion != nil {
			diagnostics.Add(field+".expected_version", reason(reasonConditional),
				"expect=path-withinでは`expected_version`を書けない")
			ok = false
		}
	}
	return ok
}

// requireCaptureRegex は指定名のnamed captureをexactly 1件持つregexを要求する。
func requireCaptureRegex(
	raw *string, field, name string, diagnostics *Diagnostics,
) (string, bool) {
	if raw == nil {
		diagnostics.Add(field, reason(reasonMissing), fmt.Sprintf("`%s`が必須", field))
		return "", false
	}
	if err := checkNamedCaptureRegex(*raw, name, field); err != nil {
		diagnostics.Add(field, reason(reasonRegex), err.Error())
		return "", false
	}
	return *raw, true
}

// requireExpectedRoot は§11の`expected_root`を検査する。
//
// `payload|probe-temp|storage.<id>`のroot名を書く。template形式ではないため、
// storage IDの実在だけを突き合わせる。
func requireExpectedRoot(
	raw *string, field string, context templateContext, diagnostics *Diagnostics,
) (string, bool) {
	if raw == nil {
		diagnostics.Add(field, reason(reasonMissing), "expect=path-withinでは`expected_root`が必須")
		return "", false
	}
	value := *raw
	switch {
	case value == expectedRootPayload, value == expectedRootProbeTemp:
		return value, true
	case strings.HasPrefix(value, expectedRootStorage):
		id := strings.TrimPrefix(value, expectedRootStorage)
		if _, declared := context.storageIDs[id]; !declared {
			diagnostics.Add(field, reason(reasonConditional),
				fmt.Sprintf("storage ID %q がこのplatformに宣言されていない", id))
			return "", false
		}
		return value, true
	}
	diagnostics.Add(field, reason(reasonEnum),
		fmt.Sprintf("expected_rootは%s|%s|%s<id>だけ（%q）",
			expectedRootPayload, expectedRootProbeTemp, expectedRootStorage, value))
	return "", false
}

// buildRequiredPaths は§11の`required_paths`を検査する。
//
// 「entryは`file:<template>|directory:<template>`の文字列として記述し、
// unknown prefixを拒否する」。
func buildRequiredPaths(
	raw *[]string, field string, context templateContext, diagnostics *Diagnostics,
) ([]RequiredPath, bool) {
	if raw == nil {
		// `required_paths`は任意keyである（§11の必須7 keyに含まれない）。
		return nil, true
	}
	entries := *raw
	if len(entries) > ArrayMax {
		diagnostics.Add(field, reason(reasonLimit),
			fmt.Sprintf("required_pathsが%d件を超える（%d件）", ArrayMax, len(entries)))
		return nil, false
	}
	values := make([]RequiredPath, 0, len(entries))
	for index, entry := range entries {
		scopeField := fmt.Sprintf("%s[%d]", field, index)
		prefix, template, found := strings.Cut(entry, ":")
		if !found {
			diagnostics.Add(scopeField, reason(reasonConditional),
				fmt.Sprintf("required_paths entryは`file:`か`directory:`で始まる（%q）", entry))
			return nil, false
		}
		kind := RequiredPathKind(prefix)
		if kind != RequiredFile && kind != RequiredDirectory {
			diagnostics.Add(scopeField, reason(reasonEnum),
				fmt.Sprintf("required_pathsのprefixは%s|%sだけ（%q）",
					RequiredFile, RequiredDirectory, prefix))
			return nil, false
		}
		if err := context.checkPathTemplate(scopeField, template, probeArgScope); err != nil {
			diagnostics.Add(scopeField, reason(reasonTemplate), err.Error())
			return nil, false
		}
		values = append(values, RequiredPath{Kind: kind, Path: template})
	}
	if err := requireUniqueIdentifiers("required_paths", entries); err != nil {
		diagnostics.Add(field, reason(reasonDuplicate), err.Error())
		return nil, false
	}
	return values, true
}

// requireProbeTimeout は§11の「timeoutは1s～2m」を検査する。
func requireProbeTimeout(raw *string, field string, diagnostics *Diagnostics) (time.Duration, bool) {
	if raw == nil {
		diagnostics.Add(field, reason(reasonMissing), fmt.Sprintf("`%s`が無い", field))
		return 0, false
	}
	value, err := time.ParseDuration(*raw)
	if err != nil {
		diagnostics.Add(field, reason(reasonDuration),
			fmt.Sprintf("%sがGo duration文字列として解釈できない（%q）", field, *raw))
		return 0, false
	}
	if value < ProbeTimeoutMin || value > ProbeTimeoutMax {
		diagnostics.Add(field, reason(reasonDuration),
			fmt.Sprintf("%sが%v〜%vの範囲外（%v）", field, ProbeTimeoutMin, ProbeTimeoutMax, value))
		return 0, false
	}
	return value, true
}

func parseProbeStream(text string) (ProbeStream, error) {
	switch ProbeStream(text) {
	case StreamStdout, StreamStderr, StreamCombined:
		return ProbeStream(text), nil
	default:
		return "", fmt.Errorf("streamは%s|%s|%sだけ（%q）",
			StreamStdout, StreamStderr, StreamCombined, text)
	}
}

func parseProbeExpect(text string) (ProbeExpect, error) {
	switch ProbeExpect(text) {
	case ExpectVersion, ExpectSuccess, ExpectPathWithin:
		return ProbeExpect(text), nil
	default:
		return "", fmt.Errorf("expectは%s|%s|%sだけ（%q）",
			ExpectVersion, ExpectSuccess, ExpectPathWithin, text)
	}
}
