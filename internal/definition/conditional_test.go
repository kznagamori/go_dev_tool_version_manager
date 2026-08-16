package definition

import (
	"fmt"
	"strings"
	"testing"
)

// 本fileはdocs/13-progress.md P3-04の2本目である。
//
// §5の`license_notice`、§8 typed storage、§9 install parameter、§10 runtime、
// §11 validationの**conditional違反**を、既存testが覆っていない箇所まで含めて
// 網羅する。positive側（§15〜§16の実定義がparseを通ること）は1本目の
// `registry_test.go`が固定している。

// TestLicenseNoticeRejectsInvalidMessageID は§5の`license_notice`を固定する。
//
// 「配布物がOSI承認OSS licenseでない場合の**message ID**」であり、message ID
// grammar（`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)*$`、docs/04-storage-and-data.md §7）
// に合わない値を受けると、message catalogを引けない警告がPlanへ出る。
func TestLicenseNoticeRejectsInvalidMessageID(t *testing.T) {
	const anchor = `artifact_kind = "official"`
	cases := []struct{ name, value string }{
		{"hyphenを含む", "license.dotnet.windows-library"},
		{"大文字を含む", "License.Dotnet"},
		{"空文字", ""},
		{"URLを書いている", "https://example.invalid/license"},
		{"末尾がdot", "license.dotnet."},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rejectSpec(t, anchor,
				anchor+"\nlicense_notice = \""+c.value+"\"", reasonMessageID)
		})
	}
}

// TestInstallRequiresStripComponents は§9の「許可keyは`strip_components`だけで
// 必須」を固定する。
//
// engineは全toolで固定順序の展開を行い、definitionは展開parameterだけを与える。
// 既定値で補うと、archive layoutが変わったことに気付かないままinstallが進む。
func TestInstallRequiresStripComponents(t *testing.T) {
	t.Run("keyが無い", func(t *testing.T) {
		rejectSpec(t, "[platforms.install]\nstrip_components = 1", "[platforms.install]", reasonMissing)
	})
	t.Run("tableが無い", func(t *testing.T) {
		rejectSpec(t, "[platforms.install]\nstrip_components = 1\n", "", reasonMissing)
	})
	t.Run("unknown key", func(t *testing.T) {
		rejectSpec(t, "strip_components = 1", "strip_components = 1\nformat = \"zip\"", reasonDecode)
	})
}

// TestStorageRequiresEveryKey は§8の「許可keyは`id`, `kind`, `scope`, `path`,
// `purge`。全件必須」を1件ずつ落として固定する。
//
// 既定値で補うと、purgeやscopeを書き忘れたdefinitionが「意図した設定」として
// 通り、uninstall時に消える範囲が変わる。
func TestStorageRequiresEveryKey(t *testing.T) {
	// 正規例のstorage 1件を対象にする。
	const block = "[[platforms.storage]]\nid = \"config\"\nkind = \"config\"\n" +
		"scope = \"tool\"\npath = \"config\"\npurge = \"explicit\""
	lines := strings.Split(block, "\n")[1:]
	if len(lines) != storageKeyCount {
		t.Fatalf("storage key = %d件, want %d", len(lines), storageKeyCount)
	}
	for _, line := range lines {
		key := strings.SplitN(line, " ", 2)[0]
		t.Run(key, func(t *testing.T) {
			reduced := strings.Replace(block, "\n"+line, "", 1)
			rejectSpec(t, block, reduced, reasonMissing)
		})
	}
}

// storageKeyCount は§8のstorage key数である。
//
// 件数を定数で持つのは、keyを足したり消したりしたときに全件必須testが気付ける
// ようにするためである。
const storageKeyCount = 5

// TestStorageKindsAreExhaustive は§8のstorage kind 6値がすべて受理されることを
// 固定する。enumが欠けると、その用途のstorageを宣言できない。
func TestStorageKindsAreExhaustive(t *testing.T) {
	kinds := []StorageKind{
		StorageConfig, StorageRuntimeData, StorageContentCache,
		StorageBuildCache, StorageGlobalPackages, StorageGlobalBin,
	}
	// §8のkindはexactly 6値である。件数をここで固定し、enumの増減に気付く。
	if len(kinds) != 6 {
		t.Fatalf("kind = %d件, want 6", len(kinds))
	}
	for _, kind := range kinds {
		t.Run(string(kind), func(t *testing.T) {
			// `global-packages`はversion scopeの正規例が既にあるため、tool scopeの
			// `config`側を差し替える。scopeとpurgeの組は別testが固定する。
			acceptSpec(t, "id = \"config\"\nkind = \"config\"",
				"id = \"config\"\nkind = \""+string(kind)+"\"")
		})
	}
}

// TestProbeExpectRejectsForeignFields は§11のexpect別契約を固定する。
//
// 「`success`はexpected fields禁止」。expect種別に無関係なfieldを黙って無視すると、
// definitionの意図と実際の検査内容が食い違う。
func TestProbeExpectRejectsForeignFields(t *testing.T) {
	// 正規例の`npm` probeはexpect=successである。
	const anchor = "id = \"npm\"\nruntime_command = \"npm\"\nargs = [\"--version\"]\n" +
		"stream = \"stdout\"\nexpect = \"success\"\ntimeout = \"60s\"\nrequired = true"
	// 禁止されるのは`expected_version`と`expected_root`である。`required_paths`は
	// expectに依存しないkeyで、probe成功直後の存在確認に使う（§11）。
	cases := []struct{ name, extra string }{
		{"expected_version", `expected_version = "{{version}}"`},
		{"expected_root", `expected_root = "payload"`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rejectSpec(t, anchor, anchor+"\n"+c.extra, reasonConditional)
		})
	}
}

// TestProbeRejectsUndeclaredRuntimeCommand は§11の「§10.1で宣言したcommand名
// だけを`runtime_command`に取る」を固定する。
//
// Plan外のprocessを実行しないための規定である。
func TestProbeRejectsUndeclaredRuntimeCommand(t *testing.T) {
	rejectSpec(t, `runtime_command = "node"`, `runtime_command = "deno"`, reasonConditional)
}

// TestProbeTempIsProbeOnly は§12の「`{{probe_temp}}`はvalidation probe内だけ」を
// 固定する。
//
// commandが使えると、probe終了後に削除される一時directoryを恒久的な参照先に
// してしまう。probeは専用temp directoryをcwdとして起動するため、そこへ書いた
// 内容はprobeの外に残らない。
func TestProbeTempIsProbeOnly(t *testing.T) {
	t.Run("probeのrequired_pathsでは使える", func(t *testing.T) {
		// entryは`file:<template>|directory:<template>`である（§11）。
		const anchor = "id = \"npm\"\nruntime_command = \"npm\"\nargs = [\"--version\"]\n" +
			"stream = \"stdout\"\nexpect = \"success\"\ntimeout = \"60s\"\nrequired = true"
		acceptSpec(t, anchor, anchor+"\nrequired_paths = [\"directory:{{probe_temp}}\"]")
	})
	t.Run("environmentのsetでは使えない", func(t *testing.T) {
		rejectSpec(t, `NPM_CONFIG_CACHE = "{{storage.cache}}"`,
			`NPM_CONFIG_CACHE = "{{probe_temp}}"`, reasonTemplate)
	})
	t.Run("path_prependでは使えない", func(t *testing.T) {
		rejectSpec(t, `path_prepend = ["{{payload}}", "{{storage.global-packages}}"]`,
			`path_prepend = ["{{probe_temp}}"]`, reasonTemplate)
	})
}

// TestRuntimeRequiresAtLeastOneCommand は§10.1のrequired command集合が空に
// ならないことを固定する。commandが1件も無いtoolはinstallしてもshimが作られず、
// installedとして扱う意味がない。
func TestRuntimeRequiresAtLeastOneCommand(t *testing.T) {
	start := strings.Index(specDefinitionTOML, "[[platforms.runtime.commands]]")
	end := strings.Index(specDefinitionTOML, "[[platforms.runtime.environment]]")
	if start < 0 || end < 0 || end <= start {
		t.Fatal("正規例からcommand blockを切り出せない")
	}
	source := specDefinitionTOML[:start] + specDefinitionTOML[end:]
	if _, err := Parse(specDefinitionPath, []byte(source)); err == nil {
		t.Fatal("commandが0件でも通った")
	}
}

// TestValidationRequiresAtLeastOneProbe は§11のprobeが空にならないことを固定する。
//
// probeが無いとinstall後の検証が何も行われず、archiveを展開しただけの状態を
// installedにしてしまう。
func TestValidationRequiresAtLeastOneProbe(t *testing.T) {
	start := strings.Index(specDefinitionTOML, "[[platforms.validation.probes]]")
	if start < 0 {
		t.Fatal("正規例からprobe blockを切り出せない")
	}
	source := specDefinitionTOML[:start]
	if _, err := Parse(specDefinitionPath, []byte(source)); err == nil {
		t.Fatal("probeが0件でも通った")
	}
}

// TestProbeLimit は§21の「platform内probe 64」を固定する。
func TestProbeLimit(t *testing.T) {
	var builder strings.Builder
	builder.WriteString(specDefinitionTOML)
	for index := 0; index <= ProbeMax; index++ {
		fmt.Fprintf(&builder, "\n[[platforms.validation.probes]]\nid = \"extra-%d\"\n"+
			"runtime_command = \"node\"\nargs = [\"--version\"]\nstream = \"stdout\"\n"+
			"expect = \"success\"\ntimeout = \"30s\"\nrequired = false\n", index)
	}
	if _, err := Parse(specDefinitionPath, []byte(builder.String())); err == nil {
		t.Fatalf("probeが%d件を超えても通った", ProbeMax)
	}
}

// TestCommandLimit は§21の「platform内runtime command 64」を固定する。
func TestCommandLimit(t *testing.T) {
	var builder strings.Builder
	builder.WriteString(specDefinitionTOML)
	for index := 0; index <= CommandMax; index++ {
		fmt.Fprintf(&builder, "\n[[platforms.runtime.commands]]\nname = \"extra-%d\"\n"+
			"target = \"{{payload}}/node.exe\"\nargs = []\nenvironment_profile = \"default\"\n"+
			"required = false\nworking_directory = \"inherit\"\npassthrough_signals = true\n", index)
	}
	if _, err := Parse(specDefinitionPath, []byte(builder.String())); err == nil {
		t.Fatalf("commandが%d件を超えても通った", CommandMax)
	}
}
