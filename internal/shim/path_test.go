package shim

import (
	"testing"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
)

// path helperはhost platformを引数に取る純粋な変換であり、両OSのtest実行で
// Windows/Linux両方の規則を確かめられる。`filepath`に委ねるとこれができない。

func TestIsAbsolutePathAppliesOSRules(t *testing.T) {
	t.Parallel()
	windows := mustPlatform(t, domain.PlatformWindowsAMD64)
	linux := mustPlatform(t, domain.PlatformLinuxAMD64Glibc)

	cases := map[string]struct {
		path string
		host domain.Platform
		want bool
	}{
		"Linux/absolute":   {path: "/root/shims", host: linux, want: true},
		"Linux/relative":   {path: "root/shims", host: linux, want: false},
		"Windows/drive":    {path: `C:\root\shims`, host: windows, want: true},
		"Windows/slash":    {path: `C:/root/shims`, host: windows, want: true},
		"Windows/UNC":      {path: `\\server\share\shims`, host: windows, want: true},
		"Windows/relative": {path: `root\shims`, host: windows, want: false},
		// **`\a`はdrive相対である。** driveのcurrent directory次第で実体が変わる
		// ため、absoluteとして扱わない。
		"Windows/drive相対": {path: `\root\shims`, host: windows, want: false},
		// Linuxのpathとして`C:\root`はただの相対path（`:`も`\`も普通の文字）。
		"Linux/Windows形式": {path: `C:\root`, host: linux, want: false},
		// Windowsのpathとして`/root`はdrive相対である。
		"Windows/Linux形式": {path: "/root", host: windows, want: false},
		"空":               {path: "", host: linux, want: false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := isAbsolutePath(tc.path, tc.host); got != tc.want {
				t.Errorf("isAbsolutePath(%q) = %v, want %v", tc.path, got, tc.want)
			}
		})
	}
}

func TestJoinPathUsesPlatformSeparator(t *testing.T) {
	t.Parallel()
	windows := mustPlatform(t, domain.PlatformWindowsAMD64)
	linux := mustPlatform(t, domain.PlatformLinuxAMD64Glibc)

	if got := joinPath("/root/shims", "go", linux); got != "/root/shims/go" {
		t.Errorf("Linux = %q, want %q", got, "/root/shims/go")
	}
	if got := joinPath(`C:\root\shims`, "go.exe", windows); got != `C:\root\shims\go.exe` {
		t.Errorf("Windows = %q, want %q", got, `C:\root\shims\go.exe`)
	}
	// rootは区切りを含んだ形である。重ねると`//go`や`C:\\go`になる。
	if got := joinPath("/", "go", linux); got != "/go" {
		t.Errorf("Linux root = %q, want %q", got, "/go")
	}
	if got := joinPath(`C:\`, "go.exe", windows); got != `C:\go.exe` {
		t.Errorf("Windows root = %q, want %q", got, `C:\go.exe`)
	}
}

func TestCleanPathResolvesDotSegments(t *testing.T) {
	t.Parallel()
	windows := mustPlatform(t, domain.PlatformWindowsAMD64)
	linux := mustPlatform(t, domain.PlatformLinuxAMD64Glibc)

	cases := map[string]struct {
		path string
		host domain.Platform
		want string
	}{
		// 保存されたrelative targetを解決してから比べるために要る。畳まずに
		// 比べると、同じ実体を指すpathどうしが形式差で不一致になる。
		"Linux/親へ":          {path: "/root/shims/../gdtvm", host: linux, want: "/root/gdtvm"},
		"Linux/カレント":        {path: "/root/./gdtvm", host: linux, want: "/root/gdtvm"},
		"Linux/重複separator": {path: "/root//gdtvm", host: linux, want: "/root/gdtvm"},
		"Windows/親へ":        {path: `C:\root\shims\..\gdtvm.exe`, host: windows, want: `C:\root\gdtvm.exe`},
		// Windowsは`/`区切りも受けるが、組み直しは`\`で行う。
		"Windows/slash入力": {path: `C:/root/shims/../gdtvm.exe`, host: windows, want: `C:\root\gdtvm.exe`},
		// absoluteのrootより上は存在しない。捨てる。
		"root超え":   {path: "/../../a", host: linux, want: "/a"},
		"relative": {path: "a/../b", host: linux, want: "b"},
		// relativeはrootより上へ畳めないため形として残す。
		"relative/上へ": {path: "../a", host: linux, want: "../a"},
		"空になる":        {path: "a/..", host: linux, want: "."},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := cleanPath(tc.path, tc.host); got != tc.want {
				t.Errorf("cleanPath(%q) = %q, want %q", tc.path, got, tc.want)
			}
		})
	}
}

func TestSplitRootSeparatesRootFromRest(t *testing.T) {
	t.Parallel()
	windows := mustPlatform(t, domain.PlatformWindowsAMD64)
	linux := mustPlatform(t, domain.PlatformLinuxAMD64Glibc)

	cases := map[string]struct {
		path     string
		host     domain.Platform
		wantRoot string
		wantRest string
	}{
		"Linux":          {path: "/a/b", host: linux, wantRoot: "/", wantRest: "a/b"},
		"Linux/relative": {path: "a/b", host: linux, wantRoot: "", wantRest: "a/b"},
		"Windows/drive":  {path: `C:\a\b`, host: windows, wantRoot: `C:\`, wantRest: `a\b`},
		"Windows/UNC":    {path: `\\srv\share`, host: windows, wantRoot: `\\`, wantRest: `srv\share`},
		// drive letterでない1文字＋`:`はrootにしない。
		"Windows/擬似drive": {path: `1:\a`, host: windows, wantRoot: "", wantRest: `1:\a`},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root, rest := splitRoot(tc.path, tc.host)
			if root != tc.wantRoot || rest != tc.wantRest {
				t.Errorf("splitRoot(%q) = %q/%q, want %q/%q",
					tc.path, root, rest, tc.wantRoot, tc.wantRest)
			}
		})
	}
}
