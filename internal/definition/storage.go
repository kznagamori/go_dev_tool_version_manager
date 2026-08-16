package definition

import (
	"fmt"
	"strings"
)

// StorageKind は§8のtyped storage種別である。
type StorageKind string

// StorageKind のexactly 6値。
const (
	StorageConfig         StorageKind = "config"
	StorageContentCache   StorageKind = "content-cache"
	StorageBuildCache     StorageKind = "build-cache"
	StorageGlobalBin      StorageKind = "global-bin"
	StorageGlobalPackages StorageKind = "global-packages"
	StorageRuntimeData    StorageKind = "runtime-data"
)

// StorageScope は§8のstorage有効範囲である。
type StorageScope string

// StorageScope の値。
const (
	// ScopeTool はtool全体で共有する。通常uninstallで保持する。
	ScopeTool StorageScope = "tool"
	// ScopeVersion は特定versionに紐づく。version uninstallで一緒に消える。
	ScopeVersion StorageScope = "version"
)

// StoragePurge は§8のstorage削除方針である。
type StoragePurge string

// StoragePurge の値。
const (
	StorageRetain      StoragePurge = "retain"
	StorageExplicit    StoragePurge = "explicit"
	StorageWithVersion StoragePurge = "with-version"
)

// StorageMax は1 platformのstorage数の上限である
// （docs/04-storage-and-data.md §21「platform内storage 32」）。
const StorageMax = 32

// Storage は§8の`[[platforms.storage]]` 1件である。5 key全件必須。
type Storage struct {
	ID    string
	Kind  StorageKind
	Scope StorageScope
	// Path はstorage root内のPOSIX relative pathである。
	Path  string
	Purge StoragePurge
}

type storageTable struct {
	ID    *string `toml:"id"`
	Kind  *string `toml:"kind"`
	Scope *string `toml:"scope"`
	Path  *string `toml:"path"`
	Purge *string `toml:"purge"`
}

// buildStorages は§8の`storage`配列を検証する（§13-7）。
func buildStorages(raw *[]storageTable, field string, diagnostics *Diagnostics) []Storage {
	if raw == nil {
		diagnostics.Add(field, reason(reasonMissing), "`storage`が無い")
		return nil
	}
	entries := *raw
	if len(entries) > StorageMax {
		diagnostics.Add(field, reason(reasonLimit),
			fmt.Sprintf("storageが%d件を超える（%d件）", StorageMax, len(entries)))
		return nil
	}
	values := make([]Storage, 0, len(entries))
	ids := make([]string, 0, len(entries))
	paths := make([]string, 0, len(entries))
	for index := range entries {
		scope := fmt.Sprintf("%s[%d]", field, index)
		value, ok := buildStorage(&entries[index], scope, diagnostics)
		if !ok {
			continue
		}
		values = append(values, value)
		ids = append(ids, value.ID)
		paths = append(paths, value.Path)
	}
	if err := requireUniqueIdentifiers("storage ID", ids); err != nil {
		diagnostics.Add(field, reason(reasonDuplicate), err.Error())
		return nil
	}
	// 「同一platform内でrender後pathが重複/包含しないこと」（§8）。
	if err := checkPathDisjoint(paths); err != nil {
		diagnostics.Add(field, reason(reasonStoragePath), err.Error())
		return nil
	}
	return values
}

func buildStorage(table *storageTable, field string, diagnostics *Diagnostics) (Storage, bool) {
	var value Storage
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

	if kind, kindOK := requireEnumText(
		table.Kind, field+".kind", parseStorageKind, diagnostics); kindOK {
		value.Kind = kind
	} else {
		ok = false
	}
	if scope, scopeOK := requireEnumText(
		table.Scope, field+".scope", parseStorageScope, diagnostics); scopeOK {
		value.Scope = scope
	} else {
		ok = false
	}
	if purge, purgeOK := requireEnumText(
		table.Purge, field+".purge", parseStoragePurge, diagnostics); purgeOK {
		value.Purge = purge
	} else {
		ok = false
	}
	if path, pathOK := requireStoragePath(table.Path, field+".path", diagnostics); pathOK {
		value.Path = path
	} else {
		ok = false
	}
	if !checkScopePurge(value.Scope, value.Purge, field, diagnostics) {
		ok = false
	}
	return value, ok
}

// checkScopePurge は§8のscopeとpurgeの組を検査する。
//
// 「tool scopeは`retain|explicit`だけで通常uninstall時に保持し、`explicit`だけ
// 最後のmanaged versionで`--purge-shared`対象にできる。version scopeは
// `with-version`だけで、対象version uninstall時にversion directoryと一緒に
// 削除する」。組合せを表で閉じるのは、tool scopeへ`with-version`を許すと
// 共有storageがversion削除で消え、他versionの設定を失うためである。
func checkScopePurge(
	scope StorageScope, purge StoragePurge, field string, diagnostics *Diagnostics,
) bool {
	if scope == "" || purge == "" {
		return true
	}
	allowed := map[StorageScope]map[StoragePurge]struct{}{
		ScopeTool:    {StorageRetain: {}, StorageExplicit: {}},
		ScopeVersion: {StorageWithVersion: {}},
	}
	if _, ok := allowed[scope][purge]; ok {
		return true
	}
	diagnostics.Add(field+".purge", reason(reasonConditional),
		fmt.Sprintf("scope=%sでpurge=%sは使えない", scope, purge))
	return false
}

// requireStoragePath は§8のstorage pathを検査する。
//
// 「pathはstorage root内POSIX relative path、空/absolute/`.`/`..`禁止」。
func requireStoragePath(raw *string, field string, diagnostics *Diagnostics) (string, bool) {
	if raw == nil {
		diagnostics.Add(field, reason(reasonMissing), fmt.Sprintf("`%s`が無い", field))
		return "", false
	}
	value := *raw
	switch {
	case value == "":
		diagnostics.Add(field, reason(reasonStoragePath), fmt.Sprintf("%sが空", field))
		return "", false
	case strings.HasPrefix(value, "/"):
		diagnostics.Add(field, reason(reasonStoragePath),
			fmt.Sprintf("%sはrelative pathでなければならない（%q）", field, value))
		return "", false
	case strings.ContainsAny(value, `\`):
		diagnostics.Add(field, reason(reasonStoragePath),
			fmt.Sprintf("%sの区切りはPOSIX slashだけ（%q）", field, value))
		return "", false
	case strings.Contains(value, "{{"):
		// storage pathはtemplateを取らない。§8はliteral relative pathだけを定める。
		diagnostics.Add(field, reason(reasonStoragePath),
			fmt.Sprintf("%sにtemplate変数を書けない（%q）", field, value))
		return "", false
	}
	for _, component := range strings.Split(value, "/") {
		switch {
		case component == "":
			diagnostics.Add(field, reason(reasonStoragePath),
				fmt.Sprintf("%sに空componentがある（%q）", field, value))
			return "", false
		case component == "." || component == "..":
			diagnostics.Add(field, reason(reasonStoragePath),
				fmt.Sprintf("%sに相対参照がある（%q）", field, value))
			return "", false
		case len(component) > PathComponentMaxBytes:
			diagnostics.Add(field, reason(reasonStoragePath),
				fmt.Sprintf("%sのcomponentが%d byteを超える", field, PathComponentMaxBytes))
			return "", false
		}
	}
	return value, true
}

// checkPathDisjoint はstorage pathが重複も包含もしないことを確かめる（§8）。
//
// 包含を許すと、片方のpurgeがもう片方を巻き込んで消す。
func checkPathDisjoint(paths []string) error {
	for i := range paths {
		for j := range paths {
			if i == j {
				continue
			}
			if paths[i] == paths[j] {
				return fmt.Errorf("storage path %q が重複している", paths[i])
			}
			if strings.HasPrefix(paths[j], paths[i]+"/") {
				return fmt.Errorf("storage path %q が %q を含む", paths[i], paths[j])
			}
		}
	}
	return nil
}

func parseStorageKind(text string) (StorageKind, error) {
	switch StorageKind(text) {
	case StorageConfig, StorageContentCache, StorageBuildCache,
		StorageGlobalBin, StorageGlobalPackages, StorageRuntimeData:
		return StorageKind(text), nil
	default:
		return "", fmt.Errorf("storage kindが§8の6値に含まれない（%q）", text)
	}
}

func parseStorageScope(text string) (StorageScope, error) {
	switch StorageScope(text) {
	case ScopeTool, ScopeVersion:
		return StorageScope(text), nil
	default:
		return "", fmt.Errorf("scopeは%s|%sだけ（%q）", ScopeTool, ScopeVersion, text)
	}
}

func parseStoragePurge(text string) (StoragePurge, error) {
	switch StoragePurge(text) {
	case StorageRetain, StorageExplicit, StorageWithVersion:
		return StoragePurge(text), nil
	default:
		return "", fmt.Errorf("purgeは%s|%s|%sだけ（%q）",
			StorageRetain, StorageExplicit, StorageWithVersion, text)
	}
}

// Install は§9の`[platforms.install]`である。許可keyは`strip_components`だけ。
type Install struct {
	// StripComponents は展開時に除去する階層数である。`0|1`だけを取る。
	StripComponents int
}

type installTable struct {
	StripComponents *int64 `toml:"strip_components"`
}

// buildInstall は§9の`install`を検証する（§13-8）。
//
// 「`strip_components`はintegerの`0|1`とし、除去後に空/衝突となる場合は拒否する。
// 2階層以上の除去が必要なartifactはv0.1の標準registryへ採用しない」。
// 除去後の衝突判定はarchive entryを見るためextract engine（P5-03）の責務であり、
// ここでは値域だけを固定する。
func buildInstall(table *installTable, field string, diagnostics *Diagnostics) Install {
	var value Install
	if table == nil {
		diagnostics.Add(field, reason(reasonMissing), "`[platforms.install]`が無い")
		return value
	}
	if table.StripComponents == nil {
		diagnostics.Add(field+".strip_components", reason(reasonMissing),
			"`strip_components`が無い")
		return value
	}
	switch *table.StripComponents {
	case 0, 1:
		value.StripComponents = int(*table.StripComponents)
	default:
		diagnostics.Add(field+".strip_components", reason(reasonEnum),
			fmt.Sprintf("strip_componentsは0|1だけ（%d）", *table.StripComponents))
	}
	return value
}
