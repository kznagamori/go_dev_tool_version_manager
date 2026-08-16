package definition

import (
	"fmt"
	"strings"
	"testing"
)

// TestStorageRejects は§8のtyped storage契約を固定する。
func TestStorageRejects(t *testing.T) {
	tests := []struct{ name, old, value, wantReason string }{
		{"kindがenum外", `kind = "config"`, `kind = "temp"`, reasonEnum},
		{"scopeがenum外", `scope = "tool"`, `scope = "global"`, reasonEnum},
		{"purgeがenum外", `purge = "explicit"`, `purge = "always"`, reasonEnum},
		{"idがgrammar外", `id = "config"`, `id = "Config"`, reasonIdentifier},
		{"pathがabsolute", `path = "config"`, `path = "/config"`, reasonStoragePath},
		{"pathに相対参照", `path = "config"`, `path = "../config"`, reasonStoragePath},
		{"pathが空", `path = "config"`, `path = ""`, reasonStoragePath},
		{"pathにbackslash", `path = "config"`, `path = "a\\b"`, reasonStoragePath},
		{"pathにtemplate", `path = "config"`, `path = "{{payload}}/config"`, reasonStoragePath},
		// tool scopeへ`with-version`を許すと、共有storageがversion削除で消える。
		{"tool scopeにwith-version", "scope = \"tool\"\npath = \"config\"\npurge = \"explicit\"",
			"scope = \"tool\"\npath = \"config\"\npurge = \"with-version\"", reasonConditional},
		{"version scopeにretain",
			"scope = \"version\"\npath = \"global-packages\"\npurge = \"with-version\"",
			"scope = \"version\"\npath = \"global-packages\"\npurge = \"retain\"", reasonConditional},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rejectSpec(t, test.old, test.value, test.wantReason)
		})
	}
}

// TestStorageRejectsOverlappingPaths は§8の「重複/包含しない」を固定する。
//
// 包含を許すと、片方のpurgeがもう片方を巻き込んで消す。
func TestStorageRejectsOverlappingPaths(t *testing.T) {
	t.Run("同じpath", func(t *testing.T) {
		rejectSpec(t, "id = \"cache\"\nkind = \"content-cache\"\nscope = \"tool\"\npath = \"cache\"",
			"id = \"cache\"\nkind = \"content-cache\"\nscope = \"tool\"\npath = \"config\"",
			reasonStoragePath)
	})
	t.Run("包含するpath", func(t *testing.T) {
		rejectSpec(t, "id = \"cache\"\nkind = \"content-cache\"\nscope = \"tool\"\npath = \"cache\"",
			"id = \"cache\"\nkind = \"content-cache\"\nscope = \"tool\"\npath = \"config/inner\"",
			reasonStoragePath)
	})
	t.Run("ID重複", func(t *testing.T) {
		rejectSpec(t, "id = \"cache\"\nkind = \"content-cache\"",
			"id = \"config\"\nkind = \"content-cache\"", reasonDuplicate)
	})
}

// TestCommandRejects は§10.1のcommand契約を固定する。
func TestCommandRejects(t *testing.T) {
	tests := []struct{ name, old, value, wantReason string }{
		{"nameがgrammar外", `name = "node"`, `name = "Node"`, reasonIdentifier},
		{"working_directoryがenum外", `working_directory = "inherit"`,
			`working_directory = "storage"`, reasonEnum},
		{"未宣言のprofileを参照", `environment_profile = "default"`,
			`environment_profile = "missing"`, reasonConditional},
		// targetは`{{payload}}`配下だけ。storageは利用者が公式commandで書き換え
		// られる領域であり、command targetにすると管理外の実体を起動しうる。
		{"targetがstorage", `target = "{{payload}}/node.exe"`,
			`target = "{{storage.cache}}/node.exe"`, reasonTemplate},
		{"targetがliteral", `target = "{{payload}}/node.exe"`,
			`target = "/usr/bin/node"`, reasonTemplate},
		{"targetに相対参照", `target = "{{payload}}/node.exe"`,
			`target = "{{payload}}/../node.exe"`, reasonTemplate},
		// path templateへliteral prefix/suffixを連結しない（§10.1）。
		{"targetにsuffix連結", `target = "{{payload}}/node.exe"`,
			`target = "{{payload}}bin/node.exe"`, reasonTemplate},
		{"targetにprefix連結", `target = "{{payload}}/node.exe"`,
			`target = "opt{{payload}}/node.exe"`, reasonTemplate},
		{"argsに未宣言storage", `args = ["{{payload}}/node_modules/npm/bin/npm-cli.js"]`,
			`args = ["{{storage.missing}}/x.js"]`, reasonTemplate},
		{"argsに{{probe_temp}}", `args = ["{{payload}}/node_modules/npm/bin/npm-cli.js"]`,
			`args = ["{{probe_temp}}/x"]`, reasonTemplate},
		{"argsのliteralに波括弧", `args = ["--version"]`, `args = ["--x={{version}}"]`, reasonTemplate},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rejectSpec(t, test.old, test.value, test.wantReason)
		})
	}
}

// TestCommandRejectsDuplicateName は§10.1の「nameはplatform内一意」を固定する。
func TestCommandRejectsDuplicateName(t *testing.T) {
	rejectSpec(t, `name = "npm"`, `name = "node"`, reasonDuplicate)
}

// TestCommandRequiresEveryKey は§10.1の7 key全件必須を固定する。
func TestCommandRequiresEveryKey(t *testing.T) {
	keys := []string{
		"name", "target", "args", "environment_profile",
		"required", "working_directory", "passthrough_signals",
	}
	for _, key := range keys {
		t.Run(key, func(t *testing.T) {
			source := removeFirstKeyAfter(t, "[[platforms.runtime.commands]]", key)
			if _, err := Parse(specDefinitionPath, []byte(source)); err == nil {
				t.Errorf("command key %q が無くても通った", key)
			}
		})
	}
}

// TestProfileRequiresEveryKey は§10.2のprofile 7 keyを固定する。
func TestProfileRequiresEveryKey(t *testing.T) {
	keys := []string{"id", "path_prepend", "path_append", "unset", "override_allowed", "shell_export"}
	for _, key := range keys {
		t.Run(key, func(t *testing.T) {
			source := removeFirstKeyAfter(t, "[[platforms.runtime.environment]]", key)
			if _, err := Parse(specDefinitionPath, []byte(source)); err == nil {
				t.Errorf("profile key %q が無くても通った", key)
			}
		})
	}
	t.Run("set", func(t *testing.T) {
		source := removeSections(t, "platforms.runtime.environment.set")
		if _, err := Parse(specDefinitionPath, []byte(source)); err == nil {
			t.Error("`set` tableが無くても通った")
		}
	})
}

// removeFirstKeyAfter はmarker以降で最初に現れるkeyの行を落とす。
func removeFirstKeyAfter(t *testing.T, marker, key string) string {
	t.Helper()
	lines := strings.Split(specDefinitionTOML, "\n")
	start := -1
	for index, line := range lines {
		if strings.TrimSpace(line) == marker {
			start = index
			break
		}
	}
	if start < 0 {
		t.Fatalf("marker %q が正規例に無い", marker)
	}
	for index := start + 1; index < len(lines); index++ {
		if strings.HasPrefix(strings.TrimSpace(lines[index]), key+" = ") {
			return strings.Join(append(append([]string{}, lines[:index]...), lines[index+1:]...), "\n")
		}
	}
	t.Fatalf("key %q が %q の後に無い", key, marker)
	return ""
}

// TestEnvironmentRejects は§10.2のenvironment契約を固定する。
func TestEnvironmentRejects(t *testing.T) {
	const setAnchor = `NPM_CONFIG_CACHE = "{{storage.cache}}"`
	tests := []struct{ name, old, value, wantReason string }{
		{"set値が未宣言storage", setAnchor, `NPM_CONFIG_CACHE = "{{storage.missing}}"`, reasonTemplate},
		{"set値に{{version}}", setAnchor, `NPM_CONFIG_CACHE = "{{version}}"`, reasonTemplate},
		{"環境変数名に=", setAnchor, `"A=B" = "x"`, reasonEnvironment},
		{"path_prependがliteral", `path_prepend = ["{{payload}}", "{{storage.global-packages}}"]`,
			`path_prepend = ["/usr/bin"]`, reasonTemplate},
		{"path_prependが重複", `path_prepend = ["{{payload}}", "{{storage.global-packages}}"]`,
			`path_prepend = ["{{payload}}", "{{payload}}"]`, reasonDuplicate},
		{"shell_exportが重複", `shell_export = ["NPM_CONFIG_PREFIX", "NPM_CONFIG_CACHE"]`,
			`shell_export = ["A", "A"]`, reasonDuplicate},
		// 同じkeyをsetしつつunsetすると、どちらが有効かがdefinitionから決まらない。
		{"setとunsetが衝突", `unset = []`, `unset = ["NPM_CONFIG_CACHE"]`, reasonEnvironment},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rejectSpec(t, test.old, test.value, test.wantReason)
		})
	}

	t.Run("profile ID重複", func(t *testing.T) {
		profile := specDefinitionTOML[strings.Index(specDefinitionTOML, "[[platforms.runtime.environment]]"):strings.Index(specDefinitionTOML, "[[platforms.validation.probes]]")]
		source := strings.Replace(specDefinitionTOML, profile, profile+profile, 1)
		_, err := Parse(specDefinitionPath, []byte(source))
		if err == nil {
			t.Fatal("同じprofile IDが2件でも通った")
		}
		assertReason(t, err, reasonDuplicate)
	})
}

// TestWindowsEnvironmentNamesAreCaseInsensitive は§10.2のWindows規則を固定する。
//
// 「Windows env keyはcase-insensitiveに一意」。同じ変数が2つの綴りで現れると、
// OSがどちらを採るかに依存する。
func TestWindowsEnvironmentNamesAreCaseInsensitive(t *testing.T) {
	const setAnchor = `NPM_CONFIG_CACHE = "{{storage.cache}}"`
	// 正規例はwindows-amd64である。
	rejectSpec(t, setAnchor, setAnchor+"\nnpm_config_cache = \"{{storage.cache}}\"", reasonDuplicate)

	// Linux platformでは別の変数として通る。
	linux := strings.NewReplacer(
		`id = "windows-amd64"`, `id = "linux-amd64-glibc"`,
		`os = "windows"`, `os = "linux"`,
		`libc = "none"`, `libc = "glibc"`,
		`target = "{{payload}}/node.exe"`, `target = "{{payload}}/bin/node"`,
		setAnchor, setAnchor+"\nnpm_config_cache = \"{{storage.cache}}\"",
	).Replace(specDefinitionTOML)
	if _, err := Parse(specDefinitionPath, []byte(linux)); err != nil {
		t.Errorf("Linuxでcase違いの環境変数名が落ちた: %v", err)
	}
}

// TestStorageLimit は§21の「platform内storage 32」を固定する。
func TestStorageLimit(t *testing.T) {
	var builder strings.Builder
	builder.WriteString(specDefinitionTOML)
	for index := 0; index <= StorageMax; index++ {
		fmt.Fprintf(&builder, "\n[[platforms.storage]]\nid = \"extra-%d\"\nkind = \"build-cache\"\n"+
			"scope = \"tool\"\npath = \"extra-%d\"\npurge = \"retain\"\n", index, index)
	}
	_, err := Parse(specDefinitionPath, []byte(builder.String()))
	if err == nil {
		t.Fatal("storage上限超過が通った")
	}
	assertReason(t, err, reasonLimit)
}
