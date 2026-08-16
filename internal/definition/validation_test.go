package definition

import (
	"strings"
	"testing"
)

// TestProbeRejects は§11のprobe契約を固定する。
func TestProbeRejects(t *testing.T) {
	tests := []struct{ name, old, value, wantReason string }{
		{"idがgrammar外", `id = "version"`, `id = "Version"`, reasonIdentifier},
		{"streamがenum外", `stream = "stdout"`, `stream = "both"`, reasonEnum},
		{"expectがenum外", `expect = "version"`, `expect = "match"`, reasonEnum},
		// probeは宣言済みruntime commandを通してだけ起動する。
		{"未宣言のruntime_command", `runtime_command = "node"`,
			`runtime_command = "python"`, reasonConditional},
		{"timeoutが下限未満", `timeout = "30s"`, `timeout = "500ms"`, reasonDuration},
		{"timeoutが上限超過", `timeout = "30s"`, `timeout = "3m"`, reasonDuration},
		{"timeoutが0", `timeout = "30s"`, `timeout = "0s"`, reasonDuration},
		{"timeoutが解釈できない", `timeout = "30s"`, `timeout = "30"`, reasonDuration},
		{"argsに未宣言storage", `args = ["--version"]`, `args = ["{{storage.missing}}/x"]`, reasonTemplate},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rejectSpec(t, test.old, test.value, test.wantReason)
		})
	}
}

// TestProbeAcceptsProbeTemp は§12の「`{{probe_temp}}`はvalidation probe内だけ」を固定する。
func TestProbeAcceptsProbeTemp(t *testing.T) {
	acceptSpec(t, `args = ["--version"]`, `args = ["-e", "{{probe_temp}}/script.js"]`)
	// commandからは使えない（probe終了後に削除される一時directoryのため）。
	rejectSpec(t, `args = ["{{payload}}/node_modules/npm/bin/npm-cli.js"]`,
		`args = ["{{probe_temp}}/x.js"]`, reasonTemplate)
}

// TestProbeExpectConditionalFields は§11のexpect別必須・禁止fieldを固定する。
func TestProbeExpectConditionalFields(t *testing.T) {
	// 正規例のversion probe blockを差し替える。
	const versionProbe = `id = "version"
runtime_command = "node"
args = ["--version"]
stream = "stdout"
expect = "version"
regex = "^v(?P<version>[0-9]+[.][0-9]+[.][0-9]+(?:-[0-9A-Za-z.-]+)?)$"
expected_version = "{{version}}"
timeout = "30s"
required = true`

	tests := []struct{ name, value, wantReason string }{
		{"versionでregexが無い", strings.Replace(versionProbe,
			"regex = \"^v(?P<version>[0-9]+[.][0-9]+[.][0-9]+(?:-[0-9A-Za-z.-]+)?)$\"\n", "", 1),
			reasonMissing},
		{"versionでexpected_versionが無い",
			strings.Replace(versionProbe, "expected_version = \"{{version}}\"\n", "", 1), reasonMissing},
		{"versionのregexにcaptureが無い", strings.Replace(versionProbe,
			"regex = \"^v(?P<version>[0-9]+[.][0-9]+[.][0-9]+(?:-[0-9A-Za-z.-]+)?)$\"",
			`regex = "^v[0-9.]+$"`, 1), reasonRegex},
		{"versionのcapture名が違う", strings.Replace(versionProbe,
			"?P<version>", "?P<ver>", 1), reasonRegex},
		{"versionでexpected_root", versionProbe + "\nexpected_root = \"payload\"", reasonConditional},
		{"expected_versionが{{version}}でない",
			strings.Replace(versionProbe, `expected_version = "{{version}}"`,
				`expected_version = "{{payload}}"`, 1), reasonTemplate},

		{"successでexpected_version", strings.Replace(versionProbe,
			`expect = "version"`, `expect = "success"`, 1), reasonConditional},

		{"path-withinでregexが無い", strings.Replace(strings.Replace(versionProbe,
			`expect = "version"`, `expect = "path-within"`, 1),
			"regex = \"^v(?P<version>[0-9]+[.][0-9]+[.][0-9]+(?:-[0-9A-Za-z.-]+)?)$\"\nexpected_version = \"{{version}}\"\n",
			"", 1), reasonMissing},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rejectSpec(t, versionProbe, test.value, test.wantReason)
		})
	}

	t.Run("path-withinが通る", func(t *testing.T) {
		pathProbe := `id = "prefix"
runtime_command = "node"
args = ["-p", "process.execPath"]
stream = "stdout"
expect = "path-within"
regex = "^(?P<path>.+)$"
expected_root = "payload"
timeout = "30s"
required = true`
		value := acceptSpec(t, versionProbe, pathProbe)
		probe := value.Platforms[0].Validation.Probes[0]
		if probe.Expect != ExpectPathWithin || probe.ExpectedRoot != "payload" {
			t.Errorf("probe = %+v", probe)
		}
	})

	t.Run("expected_rootが未宣言storage", func(t *testing.T) {
		pathProbe := strings.Replace(versionProbe, `expect = "version"`, `expect = "path-within"`, 1)
		pathProbe = strings.Replace(pathProbe,
			"regex = \"^v(?P<version>[0-9]+[.][0-9]+[.][0-9]+(?:-[0-9A-Za-z.-]+)?)$\"",
			`regex = "^(?P<path>.+)$"`, 1)
		pathProbe = strings.Replace(pathProbe,
			`expected_version = "{{version}}"`, `expected_root = "storage.missing"`, 1)
		rejectSpec(t, versionProbe, pathProbe, reasonConditional)
	})

	t.Run("expected_rootがenum外", func(t *testing.T) {
		pathProbe := strings.Replace(versionProbe, `expect = "version"`, `expect = "path-within"`, 1)
		pathProbe = strings.Replace(pathProbe,
			"regex = \"^v(?P<version>[0-9]+[.][0-9]+[.][0-9]+(?:-[0-9A-Za-z.-]+)?)$\"",
			`regex = "^(?P<path>.+)$"`, 1)
		pathProbe = strings.Replace(pathProbe,
			`expected_version = "{{version}}"`, `expected_root = "tmp"`, 1)
		rejectSpec(t, versionProbe, pathProbe, reasonEnum)
	})
}

// TestRequiredPaths は§11の`required_paths`を固定する。
func TestRequiredPaths(t *testing.T) {
	const anchor = `timeout = "60s"`
	t.Run("file/directoryが通る", func(t *testing.T) {
		value := acceptSpec(t, anchor,
			"required_paths = [\"file:{{probe_temp}}/venv/bin/python\", \"directory:{{payload}}/lib\"]\n"+anchor)
		probe := value.Platforms[0].Validation.Probes[1]
		if len(probe.RequiredPaths) != 2 {
			t.Fatalf("required_paths = %v", probe.RequiredPaths)
		}
		if probe.RequiredPaths[0].Kind != RequiredFile ||
			probe.RequiredPaths[1].Kind != RequiredDirectory {
			t.Errorf("kind = %q/%q", probe.RequiredPaths[0].Kind, probe.RequiredPaths[1].Kind)
		}
	})

	tests := []struct{ name, entries, wantReason string }{
		{"prefixが無い", `["{{payload}}/lib"]`, reasonConditional},
		{"未知prefix", `["link:{{payload}}/lib"]`, reasonEnum},
		{"templateでない", `["file:/usr/bin/node"]`, reasonTemplate},
		{"未宣言storage", `["file:{{storage.missing}}/x"]`, reasonTemplate},
		{"相対参照", `["file:{{payload}}/../x"]`, reasonTemplate},
		{"重複", `["file:{{payload}}/a", "file:{{payload}}/a"]`, reasonDuplicate},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rejectSpec(t, anchor, "required_paths = "+test.entries+"\n"+anchor, test.wantReason)
		})
	}
}

// TestProbeRequiresEveryKey は§11の必須7 keyを固定する。
func TestProbeRequiresEveryKey(t *testing.T) {
	keys := []string{"id", "runtime_command", "args", "stream", "expect", "timeout", "required"}
	for _, key := range keys {
		t.Run(key, func(t *testing.T) {
			source := removeFirstKeyAfter(t, "[[platforms.validation.probes]]", key)
			if _, err := Parse(specDefinitionPath, []byte(source)); err == nil {
				t.Errorf("probe key %q が無くても通った", key)
			}
		})
	}
}

// TestProbeRejectsDuplicateID は§11のprobe ID一意を固定する。
func TestProbeRejectsDuplicateID(t *testing.T) {
	rejectSpec(t, `id = "npm"`, `id = "version"`, reasonDuplicate)
}
