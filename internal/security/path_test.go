package security

import (
	"strings"
	"testing"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
)

func mustPlatform(t *testing.T, id string) domain.Platform {
	t.Helper()
	platform, err := domain.ParsePlatform(id)
	if err != nil {
		t.Fatalf("ParsePlatform(%q) = %v", id, err)
	}
	return platform
}

func windowsHost(t *testing.T) domain.Platform { return mustPlatform(t, "windows-amd64") }
func linuxHost(t *testing.T) domain.Platform   { return mustPlatform(t, "linux-amd64-glibc") }

func mustRoot(t *testing.T, path string) domain.PathValue {
	t.Helper()
	value, err := domain.NewPathValue(domain.RoleDataRoot, path)
	if err != nil {
		t.Fatalf("NewPathValue = %v", err)
	}
	return value
}

func TestJoinBuildsPathUnderRoot(t *testing.T) {
	tests := []struct {
		name       string
		host       domain.Platform
		root       string
		components []string
		want       string
	}{
		{"linux", linuxHost(t), "/data/gdtvm", []string{"tools", "go", "versions"}, "/data/gdtvm/tools/go/versions"},
		{"linux 末尾区切り", linuxHost(t), "/data/gdtvm/", []string{"state"}, "/data/gdtvm/state"},
		{"linux componentなし", linuxHost(t), "/data/gdtvm", nil, "/data/gdtvm"},
		{"windows", windowsHost(t), `D:\gdtvm`, []string{"tools", "go"}, `D:\gdtvm\tools\go`},
		{"windows 末尾区切り", windowsHost(t), `D:\gdtvm\`, []string{"state"}, `D:\gdtvm\state`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := Join(JoinRequest{
				Root: mustRoot(t, test.root), Components: test.components, Host: test.host,
			})
			if err != nil {
				t.Fatalf("Join = %v, want nil", err)
			}
			if got.Path() != test.want {
				t.Errorf("Path = %q, want %q", got.Path(), test.want)
			}
			// roleはrootから引き継ぐ。role未定義のpathを公開境界へ出さないため。
			if got.Role() != domain.RoleDataRoot {
				t.Errorf("Role = %q, want %q", got.Role(), domain.RoleDataRoot)
			}
		})
	}
}

// TestJoinRejectsUnsafeComponents はdocs/04-storage-and-data.md §6の拒否条件を固定する。
func TestJoinRejectsUnsafeComponents(t *testing.T) {
	tests := []struct {
		name      string
		host      domain.Platform
		component string
	}{
		{"空component（linux）", linuxHost(t), ""},
		{"空component（windows）", windowsHost(t), ""},
		{"相対参照 ..（linux）", linuxHost(t), ".."},
		{"相対参照 ..（windows）", windowsHost(t), ".."},
		{"相対参照 .", linuxHost(t), "."},
		{"slash混在（linux）", linuxHost(t), "a/b"},
		{"backslash混在（linux）", linuxHost(t), `a\b`},
		{"slash混在（windows）", windowsHost(t), "a/b"},
		{"backslash混在（windows）", windowsHost(t), `a\b`},
		{"絶対component（linux）", linuxHost(t), "/etc"},
		{"絶対component（windows）", windowsHost(t), `C:\Windows`},
		{"NUL（linux）", linuxHost(t), "a\x00b"},
		{"NUL（windows）", windowsHost(t), "a\x00b"},
		{"不正UTF-8", linuxHost(t), "a\xffb"},
		{"component長超過", linuxHost(t), strings.Repeat("a", PathComponentMaxBytes+1)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Join(JoinRequest{
				Root: mustRoot(t, "/data/gdtvm"), Components: []string{test.component}, Host: test.host,
			})
			if err == nil {
				t.Fatalf("Join(%q) = nil, want error", test.component)
			}
		})
	}
}

// TestJoinWindowsOnlyRules はWindowsだけで拒否する条件を固定する。
//
// ADSの`:`と予約device名、末尾の空白/dotはWindows固有である。Linuxでは通常の
// file名として有効であり、拒否すると正当なpathを扱えなくなる。
func TestJoinWindowsOnlyRules(t *testing.T) {
	windowsOnly := []string{
		"file:stream", // ADS
		"CON", "con", "NUL", "aux", "COM1", "lpt9",
		"CON.txt", // 拡張子を付けても予約は解除されない
		"name ",   // 末尾空白
		"name.",   // 末尾dot
	}

	for _, component := range windowsOnly {
		t.Run(component, func(t *testing.T) {
			if _, err := Join(JoinRequest{
				Root: mustRoot(t, `D:\gdtvm`), Components: []string{component}, Host: windowsHost(t),
			}); err == nil {
				t.Errorf("Windowsで %q が通った", component)
			}
			if _, err := Join(JoinRequest{
				Root: mustRoot(t, "/data/gdtvm"), Components: []string{component}, Host: linuxHost(t),
			}); err != nil {
				t.Errorf("Linuxで %q が落ちた: %v", component, err)
			}
		})
	}

	// 予約名に見えるが予約ではない名前は通す。過剰拒否しないことの確認。
	for _, component := range []string{"console", "com0", "com10", "lpt", "connection"} {
		if _, err := Join(JoinRequest{
			Root: mustRoot(t, `D:\gdtvm`), Components: []string{component}, Host: windowsHost(t),
		}); err != nil {
			t.Errorf("Windowsで %q が落ちた: %v", component, err)
		}
	}
}

func TestJoinRejectsInvalidRequest(t *testing.T) {
	valid := mustRoot(t, "/data/gdtvm")
	emptyPath, err := domain.NewPathValue(domain.RoleDataRoot, "")
	if err != nil {
		t.Fatalf("NewPathValue = %v", err)
	}

	tests := []struct {
		name    string
		request JoinRequest
	}{
		{"root未設定", JoinRequest{Host: linuxHost(t)}},
		{"rootのpathが空", JoinRequest{Root: emptyPath, Host: linuxHost(t)}},
		{"host未設定", JoinRequest{Root: valid}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, joinErr := Join(test.request); joinErr == nil {
				t.Error("Join = nil, want error")
			}
		})
	}
}

// TestJoinRejectsOversizedLogicalPath は§21のlogical path上限を固定する。
func TestJoinRejectsOversizedLogicalPath(t *testing.T) {
	root := mustRoot(t, "/data/gdtvm")
	component := strings.Repeat("a", PathComponentMaxBytes)

	// 上限内は通す。
	within := make([]string, 0, 64)
	for len(within)*(PathComponentMaxBytes+1) < LogicalPathMaxBytes-1024 {
		within = append(within, component)
	}
	if _, err := Join(JoinRequest{Root: root, Components: within, Host: linuxHost(t)}); err != nil {
		t.Fatalf("上限内が落ちた: %v", err)
	}

	over := make([]string, 0, 256)
	for len(over)*(PathComponentMaxBytes+1) <= LogicalPathMaxBytes {
		over = append(over, component)
	}
	if _, err := Join(JoinRequest{Root: root, Components: over, Host: linuxHost(t)}); err == nil {
		t.Error("logical path上限超過が通った")
	}
}

// TestIsContainedComparesComponents は文字列prefix比較でないことを固定する。
func TestIsContainedComparesComponents(t *testing.T) {
	tests := []struct {
		name  string
		host  domain.Platform
		root  string
		child string
		want  bool
	}{
		{"配下", linuxHost(t), "/data/gdtvm", "/data/gdtvm/tools/go", true},
		{"root自身", linuxHost(t), "/data/gdtvm", "/data/gdtvm", true},
		{"末尾区切り", linuxHost(t), "/data/gdtvm/", "/data/gdtvm/state", true},
		{"prefixが同じ別directory", linuxHost(t), "/data/gdtvm", "/data/gdtvm-evil/x", false},
		{"上位", linuxHost(t), "/data/gdtvm", "/data", false},
		{"兄弟", linuxHost(t), "/data/gdtvm", "/data/other", false},
		{"無関係", linuxHost(t), "/data/gdtvm", "/etc/passwd", false},
		{"windows配下", windowsHost(t), `D:\gdtvm`, `D:\gdtvm\tools`, true},
		{"windows case違いは同一", windowsHost(t), `D:\gdtvm`, `d:\GDTVM\tools`, true},
		{"windows prefixが同じ別directory", windowsHost(t), `D:\gdtvm`, `D:\gdtvm-evil\x`, false},
		{"windows別drive", windowsHost(t), `D:\gdtvm`, `C:\gdtvm\tools`, false},
		// Windowsは`/`も区切りとして扱う。片方だけで分けると、同じ位置を指す
		// pathを別物と判定してcontainment検査が誤って失敗する。
		{"windows区切り混在", windowsHost(t), `D:\gdtvm`, `D:/gdtvm/tools`, true},
		{"windows区切り混在（root側）", windowsHost(t), `D:/gdtvm`, `D:\gdtvm\tools`, true},
		{"連続区切りは同じ位置", linuxHost(t), "/data/gdtvm", "/data//gdtvm/tools", true},
		// component 0件のrootは何にでも一致する。fail openにしない。
		{"rootが空", linuxHost(t), "", "/data/gdtvm", false},
		{"rootがfilesystem root", linuxHost(t), "/", "/etc/passwd", false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := IsContained(test.root, test.child, test.host); got != test.want {
				t.Errorf("IsContained(%q, %q) = %v, want %v", test.root, test.child, got, test.want)
			}
		})
	}
}

// TestIsContainedIsCaseSensitiveOnLinux はLinuxでcase違いを別pathとすることを固定する。
//
// Linuxのfilesystemはcase sensitiveのため、`/data/GDTVM`は別のdirectoryである。
// Windowsと同じくcase非依存にすると、管理外へ書けてしまう。
func TestIsContainedIsCaseSensitiveOnLinux(t *testing.T) {
	if IsContained("/data/gdtvm", "/data/GDTVM/tools", linuxHost(t)) {
		t.Error("Linuxでcase違いが配下と判定された")
	}
}
