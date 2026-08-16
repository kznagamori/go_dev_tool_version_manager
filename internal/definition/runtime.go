package definition

import (
	"fmt"
	"sort"
	"strings"
)

// WorkingDirectory は§10.1のcommand実行cwdである。
type WorkingDirectory string

// WorkingDirectory の値。
const (
	// WorkingInherit は呼出し元のcurrent directoryを継承する。
	WorkingInherit WorkingDirectory = "inherit"
	// WorkingPayload はpayload rootをcwdにする。
	WorkingPayload WorkingDirectory = "payload"
)

// 上限（docs/04-storage-and-data.md §21「platform内runtime command 64 /
// environment profile 16 / probe 64」）。
const (
	CommandMax = 64
	ProfileMax = 16
	ProbeMax   = 64
)

// Runtime は§10の`[platforms.runtime]`である。
type Runtime struct {
	Commands    []Command
	Environment []EnvironmentProfile
}

// Command は§10.1の`[[platforms.runtime.commands]]`である。7 key全件必須。
type Command struct {
	// Name はplatform内一意な公開command名である。
	Name string
	// Target は`{{payload}}`配下の実行fileを指すpath templateである。
	Target string
	// Args はliteralまたはpath templateのargvである。空可。
	Args []string
	// EnvironmentProfile は参照するprofile IDである。exactly 1件を指す。
	EnvironmentProfile string
	// Required はinstall時に必須のcommandかである。
	Required bool
	// WorkingDirectory は実行cwdである。
	WorkingDirectory WorkingDirectory
	// PassthroughSignals は子processへsignalを透過するかである。
	PassthroughSignals bool
}

// EnvironmentProfile は§10.2の`[[platforms.runtime.environment]]`である。
type EnvironmentProfile struct {
	ID          string
	PathPrepend []string
	PathAppend  []string
	// Set は環境変数名から値へのmapである。値はliteralまたはpath templateである。
	Set map[string]string
	// Unset は削除する環境変数名である。
	Unset []string
	// OverrideAllowed は親環境値を優先できるkeyである。
	OverrideAllowed []string
	// ShellExport はshim経由の子processへ渡す公開値のkeyである。
	ShellExport []string
}

type runtimeTable struct {
	Commands    *[]commandTable `toml:"commands"`
	Environment *[]profileTable `toml:"environment"`
}

type commandTable struct {
	Name               *string   `toml:"name"`
	Target             *string   `toml:"target"`
	Args               *[]string `toml:"args"`
	EnvironmentProfile *string   `toml:"environment_profile"`
	Required           *bool     `toml:"required"`
	WorkingDirectory   *string   `toml:"working_directory"`
	PassthroughSignals *bool     `toml:"passthrough_signals"`
}

type profileTable struct {
	ID              *string            `toml:"id"`
	PathPrepend     *[]string          `toml:"path_prepend"`
	PathAppend      *[]string          `toml:"path_append"`
	Set             *map[string]string `toml:"set"`
	Unset           *[]string          `toml:"unset"`
	OverrideAllowed *[]string          `toml:"override_allowed"`
	ShellExport     *[]string          `toml:"shell_export"`
}

// buildRuntime は§10の`runtime`を検証する（§13-9）。
func buildRuntime(
	table *runtimeTable, field string, context templateContext,
	windows bool, diagnostics *Diagnostics,
) Runtime {
	var value Runtime
	if table == nil {
		diagnostics.Add(field, reason(reasonMissing), "`[platforms.runtime]`が無い")
		return value
	}
	value.Environment = buildProfiles(table.Environment, field+".environment", context, windows, diagnostics)
	profileIDs := make(map[string]struct{}, len(value.Environment))
	for _, profile := range value.Environment {
		profileIDs[profile.ID] = struct{}{}
	}
	value.Commands = buildCommands(table.Commands, field+".commands", context, profileIDs, diagnostics)
	return value
}

func buildCommands(
	raw *[]commandTable, field string, context templateContext,
	profileIDs map[string]struct{}, diagnostics *Diagnostics,
) []Command {
	if raw == nil {
		diagnostics.Add(field, reason(reasonMissing), "`commands`が無い")
		return nil
	}
	entries := *raw
	switch {
	case len(entries) == 0:
		// command 0件のtoolはinstallしても何も実行できない。
		diagnostics.Add(field, reason(reasonConditional), "`commands`が空配列")
		return nil
	case len(entries) > CommandMax:
		diagnostics.Add(field, reason(reasonLimit),
			fmt.Sprintf("commandが%d件を超える（%d件）", CommandMax, len(entries)))
		return nil
	}
	values := make([]Command, 0, len(entries))
	names := make([]string, 0, len(entries))
	for index := range entries {
		scope := fmt.Sprintf("%s[%d]", field, index)
		value, ok := buildCommand(&entries[index], scope, context, profileIDs, diagnostics)
		if !ok {
			continue
		}
		values = append(values, value)
		names = append(names, value.Name)
	}
	if err := requireUniqueIdentifiers("command name", names); err != nil {
		diagnostics.Add(field, reason(reasonDuplicate), err.Error())
		return nil
	}
	return values
}

func buildCommand(
	table *commandTable, field string, context templateContext,
	profileIDs map[string]struct{}, diagnostics *Diagnostics,
) (Command, bool) {
	var value Command
	ok := true

	if table.Name == nil {
		diagnostics.Add(field+".name", reason(reasonMissing), "`name`が無い")
		ok = false
	} else if err := ValidateCommandName(*table.Name); err != nil {
		diagnostics.Add(field+".name", reason(reasonIdentifier), err.Error())
		ok = false
	} else {
		value.Name = *table.Name
	}

	// targetは`{{payload}}`配下の実行fileである。storage配下は実行対象にしない。
	// storageは利用者が公式commandで書き換えられる領域であり（§8）、そこを
	// command targetにすると管理外の実体を起動しうる。
	if table.Target == nil {
		diagnostics.Add(field+".target", reason(reasonMissing), "`target`が無い")
		ok = false
	} else if err := context.checkPathTemplate(
		field+".target", *table.Target, payloadOnlyScope); err != nil {
		diagnostics.Add(field+".target", reason(reasonTemplate), err.Error())
		ok = false
	} else {
		value.Target = *table.Target
	}

	if args, argsOK := buildArgs(table.Args, field+".args", context, commandScope, diagnostics); argsOK {
		value.Args = args
	} else {
		ok = false
	}

	// 「command参照先はexactly 1件」（§10.2）。
	if table.EnvironmentProfile == nil {
		diagnostics.Add(field+".environment_profile", reason(reasonMissing),
			"`environment_profile`が無い")
		ok = false
	} else if _, declared := profileIDs[*table.EnvironmentProfile]; !declared {
		diagnostics.Add(field+".environment_profile", reason(reasonConditional),
			fmt.Sprintf("environment profile %q がこのplatformに宣言されていない",
				*table.EnvironmentProfile))
		ok = false
	} else {
		value.EnvironmentProfile = *table.EnvironmentProfile
	}

	if required, boolOK := requireBool(table.Required, field+".required", diagnostics); boolOK {
		value.Required = required
	} else {
		ok = false
	}
	if signals, boolOK := requireBool(
		table.PassthroughSignals, field+".passthrough_signals", diagnostics); boolOK {
		value.PassthroughSignals = signals
	} else {
		ok = false
	}
	if working, workingOK := requireEnumText(
		table.WorkingDirectory, field+".working_directory", parseWorkingDirectory, diagnostics); workingOK {
		value.WorkingDirectory = working
	} else {
		ok = false
	}
	return value, ok
}

// payloadOnlyScope はcommand targetの許可rootである。
//
// `{{payload}}`だけを許す。§10.1が「targetは`{{payload}}`配下のregular
// executable、または固定interpreterとして別required command targetと同じ実体を
// 指せる」と定めており、どちらもpayload配下である。
var payloadOnlyScope = templateScope{name: "command target", payload: true}

// buildArgs は§10.1・§11のargsを検査する。
//
// 「literalまたはentry全体が path templateとその子pathであるものだけ。
// path templateへliteral prefix/suffixを連結しない」。entryごとにどちらかへ
// 振り分けるのは、Planが`PlanArg`でliteralとpathを分けるためである
// （[04-storage-and-data.md](../../docs/04-storage-and-data.md) §16）。
func buildArgs(
	raw *[]string, field string, context templateContext,
	scope templateScope, diagnostics *Diagnostics,
) ([]string, bool) {
	if raw == nil {
		diagnostics.Add(field, reason(reasonMissing), fmt.Sprintf("`%s`が無い", field))
		return nil, false
	}
	args := *raw
	if len(args) > ArrayMax {
		diagnostics.Add(field, reason(reasonLimit),
			fmt.Sprintf("argsが%d件を超える（%d件）", ArrayMax, len(args)))
		return nil, false
	}
	for index, arg := range args {
		scopeField := fmt.Sprintf("%s[%d]", field, index)
		var err error
		if isPathTemplate(arg) {
			err = context.checkPathTemplate(scopeField, arg, scope)
		} else {
			err = checkLiteralArg(scopeField, arg)
		}
		if err != nil {
			diagnostics.Add(scopeField, reason(reasonTemplate), err.Error())
			return nil, false
		}
	}
	return append([]string{}, args...), true
}

func parseWorkingDirectory(text string) (WorkingDirectory, error) {
	switch WorkingDirectory(text) {
	case WorkingInherit, WorkingPayload:
		return WorkingDirectory(text), nil
	default:
		return "", fmt.Errorf("working_directoryは%s|%sだけ（%q）",
			WorkingInherit, WorkingPayload, text)
	}
}

func buildProfiles(
	raw *[]profileTable, field string, context templateContext,
	windows bool, diagnostics *Diagnostics,
) []EnvironmentProfile {
	if raw == nil {
		diagnostics.Add(field, reason(reasonMissing), "`environment`が無い")
		return nil
	}
	entries := *raw
	switch {
	case len(entries) == 0:
		// commandは必ずprofileを参照するため、0件では成立しない。
		diagnostics.Add(field, reason(reasonConditional), "`environment`が空配列")
		return nil
	case len(entries) > ProfileMax:
		diagnostics.Add(field, reason(reasonLimit),
			fmt.Sprintf("environment profileが%d件を超える（%d件）", ProfileMax, len(entries)))
		return nil
	}
	values := make([]EnvironmentProfile, 0, len(entries))
	ids := make([]string, 0, len(entries))
	for index := range entries {
		scope := fmt.Sprintf("%s[%d]", field, index)
		value, ok := buildProfile(&entries[index], scope, context, windows, diagnostics)
		if !ok {
			continue
		}
		values = append(values, value)
		ids = append(ids, value.ID)
	}
	if err := requireUniqueIdentifiers("environment profile ID", ids); err != nil {
		diagnostics.Add(field, reason(reasonDuplicate), err.Error())
		return nil
	}
	return values
}

func buildProfile(
	table *profileTable, field string, context templateContext,
	windows bool, diagnostics *Diagnostics,
) (EnvironmentProfile, bool) {
	var value EnvironmentProfile
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

	// PATHはlogical path配列としてmergeする。raw separator文字列をdefinitionへ
	// 書かせない（§10.2）ため、各entryをpath templateとして検査する。
	prepend, prependOK := buildPathList(table.PathPrepend, field+".path_prepend", context, diagnostics)
	value.PathPrepend = prepend
	appendPaths, appendOK := buildPathList(table.PathAppend, field+".path_append", context, diagnostics)
	value.PathAppend = appendPaths
	set, setOK := buildEnvironmentSet(table.Set, field+".set", context, windows, diagnostics)
	value.Set = set
	unset, unsetOK := buildEnvNameList(table.Unset, field+".unset", windows, diagnostics)
	value.Unset = unset
	override, overrideOK := buildEnvNameList(
		table.OverrideAllowed, field+".override_allowed", windows, diagnostics)
	value.OverrideAllowed = override
	export, exportOK := buildEnvNameList(
		table.ShellExport, field+".shell_export", windows, diagnostics)
	value.ShellExport = export

	if !prependOK || !appendOK || !setOK || !unsetOK || !overrideOK || !exportOK {
		ok = false
	}
	if !checkEnvironmentConflicts(value, field, windows, diagnostics) {
		ok = false
	}
	return value, ok
}

// buildPathList は§10.2の`path_prepend`/`path_append`を検査する。
//
// 各entryは`{{payload}}`または`{{storage.<id>}}`とその子pathである。
// literalのabsolute pathを許すと、管理rootの外をPATHへ入れられる。
func buildPathList(
	raw *[]string, field string, context templateContext, diagnostics *Diagnostics,
) ([]string, bool) {
	if raw == nil {
		diagnostics.Add(field, reason(reasonMissing), fmt.Sprintf("`%s`が無い", field))
		return nil, false
	}
	entries := *raw
	if len(entries) > ArrayMax {
		diagnostics.Add(field, reason(reasonLimit),
			fmt.Sprintf("%sが%d件を超える（%d件）", field, ArrayMax, len(entries)))
		return nil, false
	}
	for index, entry := range entries {
		scopeField := fmt.Sprintf("%s[%d]", field, index)
		if err := context.checkPathTemplate(scopeField, entry, commandScope); err != nil {
			diagnostics.Add(scopeField, reason(reasonTemplate), err.Error())
			return nil, false
		}
	}
	// 「canonical重複を除去する」（§10.2）。同じpathを2回書いた定義は、
	// 除去後のPATHが宣言と食い違うため定義時に拒否する。
	if err := requireUniqueIdentifiers(field, entries); err != nil {
		diagnostics.Add(field, reason(reasonDuplicate), err.Error())
		return nil, false
	}
	return append([]string{}, entries...), true
}

// buildEnvironmentSet は§10.2の`set`を検査する。
//
// 「set値はliteral、`{{payload}}`、`{{storage.<id>}}`とその子pathだけ」。
func buildEnvironmentSet(
	raw *map[string]string, field string, context templateContext,
	windows bool, diagnostics *Diagnostics,
) (map[string]string, bool) {
	if raw == nil {
		diagnostics.Add(field, reason(reasonMissing), fmt.Sprintf("`%s`が無い", field))
		return nil, false
	}
	source := *raw
	if len(source) > EnvironmentEntryMax {
		diagnostics.Add(field, reason(reasonLimit),
			fmt.Sprintf("%sが%d件を超える（%d件）", field, EnvironmentEntryMax, len(source)))
		return nil, false
	}
	names := make([]string, 0, len(source))
	value := make(map[string]string, len(source))
	for _, key := range sortedKeys(source) {
		if err := checkEnvironmentName(key, windows); err != nil {
			diagnostics.Add(field+"."+key, reason(reasonEnvironment), err.Error())
			return nil, false
		}
		entry := source[key]
		var err error
		if isPathTemplate(entry) {
			err = context.checkPathTemplate(field+"."+key, entry, commandScope)
		} else {
			err = checkLiteralArg(field+"."+key, entry)
		}
		if err != nil {
			diagnostics.Add(field+"."+key, reason(reasonTemplate), err.Error())
			return nil, false
		}
		names = append(names, key)
		value[key] = entry
	}
	// 「Windows env keyはcase-insensitiveに一意」（§10.2）。
	if windows {
		if err := requireUniqueIdentifiers("environment変数名", names); err != nil {
			diagnostics.Add(field, reason(reasonDuplicate), err.Error())
			return nil, false
		}
	}
	return value, true
}

// buildEnvNameList は環境変数名だけを持つ配列を検査する。
func buildEnvNameList(
	raw *[]string, field string, windows bool, diagnostics *Diagnostics,
) ([]string, bool) {
	if raw == nil {
		diagnostics.Add(field, reason(reasonMissing), fmt.Sprintf("`%s`が無い", field))
		return nil, false
	}
	entries := *raw
	if len(entries) > EnvironmentEntryMax {
		diagnostics.Add(field, reason(reasonLimit),
			fmt.Sprintf("%sが%d件を超える（%d件）", field, EnvironmentEntryMax, len(entries)))
		return nil, false
	}
	for index, name := range entries {
		if err := checkEnvironmentName(name, windows); err != nil {
			diagnostics.Add(fmt.Sprintf("%s[%d]", field, index), reason(reasonEnvironment), err.Error())
			return nil, false
		}
	}
	if err := requireUniqueIdentifiers(field, entries); err != nil {
		diagnostics.Add(field, reason(reasonDuplicate), err.Error())
		return nil, false
	}
	return append([]string{}, entries...), true
}

// EnvironmentEntryMax は環境変数entry数の上限である
// （docs/04-storage-and-data.md §21「environment entry 4,096」）。
const EnvironmentEntryMax = 4096

// checkEnvironmentName は環境変数名を検査する。
//
// `=`とNULはOSのenvironment blockの区切りであり、含めるとblockを壊せる。
// 前後空白はWindowsとPOSIXで扱いが揺れるため受け付けない。
func checkEnvironmentName(name string, windows bool) error {
	switch {
	case name == "":
		return fmt.Errorf("環境変数名が空")
	case len(name) > NameMaxBytes:
		return fmt.Errorf("環境変数名 %q が%d byteを超える", name, NameMaxBytes)
	case strings.ContainsAny(name, "=\x00"):
		return fmt.Errorf("環境変数名 %q に`=`かNULが含まれる", name)
	case strings.TrimSpace(name) != name:
		return fmt.Errorf("環境変数名 %q の前後に空白がある", name)
	}
	for _, char := range name {
		if char > 0x7F {
			return fmt.Errorf("環境変数名 %q がASCIIでない", name)
		}
	}
	_ = windows
	return nil
}

// checkEnvironmentConflicts は§10.2の重複禁止を検査する。
//
// 「unset/set重複禁止」。同じkeyをsetしつつunsetすると、どちらが有効かが
// definitionから決まらない。
func checkEnvironmentConflicts(
	value EnvironmentProfile, field string, windows bool, diagnostics *Diagnostics,
) bool {
	fold := func(name string) string {
		if windows {
			return strings.ToLower(name)
		}
		return name
	}
	setNames := make(map[string]struct{}, len(value.Set))
	for name := range value.Set {
		setNames[fold(name)] = struct{}{}
	}
	conflicts := make([]string, 0)
	for _, name := range value.Unset {
		if _, both := setNames[fold(name)]; both {
			conflicts = append(conflicts, name)
		}
	}
	if len(conflicts) == 0 {
		return true
	}
	sort.Strings(conflicts)
	diagnostics.Add(field+".unset", reason(reasonEnvironment),
		fmt.Sprintf("`set`と`unset`が同じ環境変数名を持つ（%s）", strings.Join(conflicts, ", ")))
	return false
}

// requireBool は必須のbooleanを読む。
func requireBool(raw *bool, field string, diagnostics *Diagnostics) (bool, bool) {
	if raw == nil {
		diagnostics.Add(field, reason(reasonMissing), fmt.Sprintf("`%s`が無い", field))
		return false, false
	}
	return *raw, true
}
