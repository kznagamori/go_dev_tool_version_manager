package shim

import (
	"errors"
	"testing"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/store"
)

func mustPlatform(t *testing.T, id string) domain.Platform {
	t.Helper()
	platform, err := domain.ParsePlatform(id)
	if err != nil {
		t.Fatalf("ParsePlatform(%q): %v", id, err)
	}
	return platform
}

func mustToolID(t *testing.T, id string) domain.ToolID {
	t.Helper()
	toolID, err := domain.ParseToolID(id)
	if err != nil {
		t.Fatalf("ParseToolID(%q): %v", id, err)
	}
	return toolID
}

// 呼出名解決はOS APIを呼ばない純粋な変換であり、両OSのtest実行で
// Windows/Linux両方の規則を確かめられる。

func TestNormalizeCommandNameAppliesOSRules(t *testing.T) {
	t.Parallel()
	windows := mustPlatform(t, domain.PlatformWindowsAMD64)
	linux := mustPlatform(t, domain.PlatformLinuxAMD64Glibc)

	cases := map[string]struct {
		argv0 string
		host  domain.Platform
		want  string
	}{
		"Linux/そのまま":       {argv0: "/root/shims/go", host: linux, want: "go"},
		"Linux/名前だけ":       {argv0: "go", host: linux, want: "go"},
		"Windows/suffix除去": {argv0: `C:\root\shims\go.exe`, host: windows, want: "go"},
		// Windowsは`GO.EXE`と`go.exe`が同じfileである。case-insensitiveに畳む。
		"Windows/大文字": {argv0: `C:\root\shims\GO.EXE`, host: windows, want: "go"},
		// **suffixは1つだけ落とす。** 繰り返し落とすと別のcommandになる。
		"Windows/二重suffix": {argv0: `C:\root\shims\go.exe.exe`, host: windows, want: "go.exe"},
		"Windows/suffixなし": {argv0: `C:\root\shims\go`, host: windows, want: "go"},
		"CLI名":             {argv0: "/usr/local/bin/gdtvm", host: linux, want: ClientCommandName},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, err := NormalizeCommandName(tc.argv0, tc.host)
			if err != nil {
				t.Fatalf("NormalizeCommandName: %v", err)
			}
			if got != tc.want {
				t.Errorf("commandName = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestNormalizeCommandNameKeepsLinuxCase(t *testing.T) {
	t.Parallel()
	// **Linuxで`GO`と`go`は別のfileである。** Windowsの規則を持ち込むと、
	// 別commandを同一視する。
	got, err := NormalizeCommandName("/root/shims/GO", mustPlatform(t, domain.PlatformLinuxAMD64Glibc))
	if err != nil {
		t.Fatalf("NormalizeCommandName: %v", err)
	}
	if got != "GO" {
		t.Errorf("commandName = %q, want %q", got, "GO")
	}
	// Linuxでは`.exe`もcommand名の一部である。
	withSuffix, err := NormalizeCommandName("/root/shims/tool.exe",
		mustPlatform(t, domain.PlatformLinuxAMD64Glibc))
	if err != nil {
		t.Fatalf("NormalizeCommandName: %v", err)
	}
	if withSuffix != "tool.exe" {
		t.Errorf("commandName = %q, want %q", withSuffix, "tool.exe")
	}
}

func TestNormalizeCommandNameRejectsUnusableArgv0(t *testing.T) {
	t.Parallel()
	windows := mustPlatform(t, domain.PlatformWindowsAMD64)
	linux := mustPlatform(t, domain.PlatformLinuxAMD64Glibc)

	cases := map[string]struct {
		argv0 string
		host  domain.Platform
	}{
		"空":                {argv0: "", host: linux},
		"空白だけ":             {argv0: "   ", host: linux},
		"separatorだけ":      {argv0: "/", host: linux},
		"カレント":             {argv0: ".", host: linux},
		"suffixだけ":         {argv0: ".exe", host: windows},
		"host platform未設定": {argv0: "go", host: domain.Platform{}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, err := NormalizeCommandName(tc.argv0, tc.host)
			if err == nil {
				t.Fatalf("正規化できない呼出名が通った: %q", got)
			}
			if !errors.Is(err, ErrInvalidCommandName) {
				t.Errorf("error = %v, want ErrInvalidCommandName", err)
			}
		})
	}
}

func TestResolveModeSplitsCLIAndShim(t *testing.T) {
	t.Parallel()
	// docs/08-install-runtime.md §10「起動basenameが`gdtvm`ならCLI」。
	if got := ResolveMode(ClientCommandName); got != ModeCLI {
		t.Errorf("Mode(%q) = %q, want %q", ClientCommandName, got, ModeCLI)
	}
	for _, name := range []string{"go", "node", "gdtvm2", "gdtv"} {
		if got := ResolveMode(name); got != ModeShim {
			t.Errorf("Mode(%q) = %q, want %q", name, got, ModeShim)
		}
	}
}

func testIndex(t *testing.T, commands ...store.ShimCommand) store.ShimIndex {
	t.Helper()
	return store.ShimIndex{Commands: commands}
}

func TestResolveCommandFindsOwningTool(t *testing.T) {
	t.Parallel()
	index := testIndex(t,
		store.ShimCommand{Name: "node", ToolID: mustToolID(t, "node")},
		store.ShimCommand{Name: "npm", ToolID: mustToolID(t, "node")},
		store.ShimCommand{Name: "go", ToolID: mustToolID(t, "go")},
	)
	// 1 toolが複数commandを持つ形（node/npm）が正しく解決できること。
	for command, want := range map[string]string{"node": "node", "npm": "node", "go": "go"} {
		got, err := ResolveCommand(index, command)
		if err != nil {
			t.Fatalf("ResolveCommand(%q): %v", command, err)
		}
		if got.String() != want {
			t.Errorf("ResolveCommand(%q) = %q, want %q", command, got, want)
		}
	}
}

func TestResolveCommandRejectsUnusableIndex(t *testing.T) {
	t.Parallel()
	t.Run("0件", func(t *testing.T) {
		t.Parallel()
		// **docs/09-platform.md §3.3「unknown basenameをCLIとして実行しない」。**
		// CLIへ落とすと、利用者が意図しないcommandがgdtvm本体として動く。
		index := testIndex(t, store.ShimCommand{Name: "go", ToolID: mustToolID(t, "go")})
		if _, err := ResolveCommand(index, "python"); !errors.Is(err, ErrUnknownCommand) {
			t.Fatalf("error = %v, want ErrUnknownCommand", err)
		}
	})
	t.Run("複数件", func(t *testing.T) {
		t.Parallel()
		// docs/08-install-runtime.md §10手順1「0件/複数件は失敗する」。indexは
		// name一意のはずだが、破損したindexで片方を選んで進めない。
		index := testIndex(t,
			store.ShimCommand{Name: "go", ToolID: mustToolID(t, "go")},
			store.ShimCommand{Name: "go", ToolID: mustToolID(t, "node")},
		)
		if _, err := ResolveCommand(index, "go"); !errors.Is(err, ErrAmbiguousCommand) {
			t.Fatalf("error = %v, want ErrAmbiguousCommand", err)
		}
	})
	t.Run("tool_idが空", func(t *testing.T) {
		t.Parallel()
		index := testIndex(t, store.ShimCommand{Name: "go"})
		if _, err := ResolveCommand(index, "go"); err == nil {
			t.Fatal("tool_idが空のentryが通った")
		}
	})
	t.Run("空のindex", func(t *testing.T) {
		t.Parallel()
		if _, err := ResolveCommand(store.ShimIndex{}, "go"); !errors.Is(err, ErrUnknownCommand) {
			t.Fatalf("error = %v, want ErrUnknownCommand", err)
		}
	})
}

func TestIdentifyDerivesDataRootFromShimPath(t *testing.T) {
	t.Parallel()
	linux := mustPlatform(t, domain.PlatformLinuxAMD64Glibc)
	// **`filepath`で組まない。** 実行中OSの区切りで組むとWindows jobで
	// `\home\u\...`になり、Linux規則の分割に掛からない。
	root := "/home/u/.local/share/gdtvm"
	argv0 := root + "/" + ShimDirName + "/go"
	// Linuxのshimはclientへのrelative symlinkであり、module pathは辿った先の
	// client本体を指す。両者が違うことがdocs/02-architecture.md §9の
	// InvocationRequestがargv0とmodule pathの両方を持つ理由である。
	modulePath := root + "/gdtvm"

	identity, err := Identify(argv0, modulePath, linux)
	if err != nil {
		t.Fatalf("Identify: %v", err)
	}
	if identity.Mode != ModeShim {
		t.Errorf("Mode = %q, want %q", identity.Mode, ModeShim)
	}
	if identity.CommandName != "go" {
		t.Errorf("CommandName = %q, want %q", identity.CommandName, "go")
	}
	if identity.ShimDir != root+"/"+ShimDirName {
		t.Errorf("ShimDir = %q, want %q", identity.ShimDir, root+"/"+ShimDirName)
	}
	// docs/04-storage-and-data.md §11「shim pathはdata root相対`shims`固定」を
	// 逆に辿る。ここを誤ると別rootのstateを読む（§2.3）。
	if identity.DataRoot != root {
		t.Errorf("DataRoot = %q, want %q", identity.DataRoot, root)
	}
	if identity.Argv0 != argv0 || identity.ModulePath != modulePath {
		t.Errorf("argv0/module = %q/%q, want %q/%q",
			identity.Argv0, identity.ModulePath, argv0, modulePath)
	}
}

func TestIdentifyLeavesRootUnsetForCLI(t *testing.T) {
	t.Parallel()
	linux := mustPlatform(t, domain.PlatformLinuxAMD64Glibc)
	// CLIのdata rootは`--home`やmodeで変わる。shim pathから逆算しない。
	identity, err := Identify("/opt/gdtvm/gdtvm", "/opt/gdtvm/gdtvm", linux)
	if err != nil {
		t.Fatalf("Identify: %v", err)
	}
	if identity.Mode != ModeCLI {
		t.Fatalf("Mode = %q, want %q", identity.Mode, ModeCLI)
	}
	if identity.DataRoot != "" || identity.ShimDir != "" {
		t.Errorf("CLIでDataRoot=%q ShimDir=%q が埋まっている", identity.DataRoot, identity.ShimDir)
	}
}

func TestIdentifyRejectsShimOutsideShimDir(t *testing.T) {
	t.Parallel()
	linux := mustPlatform(t, domain.PlatformLinuxAMD64Glibc)
	// data rootを決められないままstateを読みに行くと、別rootのstateを混ぜる
	// （docs/09-platform.md §2.3「別rootのstate/linkを混在させない」）。
	// **`filepath`で組まない。** 実行中OSの区切りで組むとWindows jobで`\`になり、
	// Linux規則の分割に掛からない。host platformの規則で書いたpathをそのまま渡す。
	cases := map[string]string{
		"shims配下でない":       "/usr/local/bin/go",
		"shimsがfs rootの直下": "/" + ShimDirName + "/go",
	}
	for name, argv0 := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := Identify(argv0, "/opt/gdtvm/gdtvm", linux); !errors.Is(err, ErrNotShimPath) {
				t.Fatalf("error = %v, want ErrNotShimPath", err)
			}
		})
	}
}

func TestIdentifyHandlesWindowsPaths(t *testing.T) {
	t.Parallel()
	windows := mustPlatform(t, domain.PlatformWindowsAMD64)
	// path分割をhost platformで行うため、Linuxからでも**Windowsの規則**を
	// 確かめられる。filepath.Dir/Baseに任せると、この検査はWindows jobまで
	// 動かない。
	cases := map[string]struct {
		argv0    string
		wantRoot string
		wantDir  string
		wantName string
	}{
		"backslash": {
			argv0:    `C:\Users\dev\AppData\Local\gdtvm\shims\go.exe`,
			wantRoot: `C:\Users\dev\AppData\Local\gdtvm`,
			wantDir:  `C:\Users\dev\AppData\Local\gdtvm\shims`,
			wantName: "go",
		},
		// Windows APIは`/`も区切りとして受ける。
		"slash": {
			argv0:    `C:/gdtvm/shims/node.exe`,
			wantRoot: `C:/gdtvm`,
			wantDir:  `C:/gdtvm/shims`,
			wantName: "node",
		},
		// Windowsのdirectory名はcase-insensitiveである。
		"shims大文字": {
			argv0:    `C:\gdtvm\SHIMS\GO.EXE`,
			wantRoot: `C:\gdtvm`,
			wantDir:  `C:\gdtvm\SHIMS`,
			wantName: "go",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			identity, err := Identify(tc.argv0, `C:\gdtvm\gdtvm.exe`, windows)
			if err != nil {
				t.Fatalf("Identify: %v", err)
			}
			if identity.CommandName != tc.wantName {
				t.Errorf("CommandName = %q, want %q", identity.CommandName, tc.wantName)
			}
			if identity.ShimDir != tc.wantDir {
				t.Errorf("ShimDir = %q, want %q", identity.ShimDir, tc.wantDir)
			}
			if identity.DataRoot != tc.wantRoot {
				t.Errorf("DataRoot = %q, want %q", identity.DataRoot, tc.wantRoot)
			}
		})
	}
}

func TestIdentifyRejectsWindowsDriveRoot(t *testing.T) {
	t.Parallel()
	windows := mustPlatform(t, domain.PlatformWindowsAMD64)
	// `C:\shims\go.exe`のdata rootは`C:\`である。docs/09-platform.md §2.3は
	// filesystem root自体をdata rootとして拒否する。
	if _, err := Identify(`C:\shims\go.exe`, `C:\gdtvm.exe`, windows); !errors.Is(err, ErrNotShimPath) {
		t.Fatalf("error = %v, want ErrNotShimPath", err)
	}
}

func TestDriveRelativeArgv0IsRejectedByIdentify(t *testing.T) {
	t.Parallel()
	windows := mustPlatform(t, domain.PlatformWindowsAMD64)
	// `C:go`はdrive相対pathである。**command名としては`go`が正しい** ——
	// drive指定はcomponent境界であり、名前の一部ではない。
	got, err := NormalizeCommandName("C:go", windows)
	if err != nil {
		t.Fatalf("NormalizeCommandName: %v", err)
	}
	if got != "go" {
		t.Errorf("commandName = %q, want %q", got, "go")
	}
	// 一方でdata rootは決められない。driveのcurrent directory次第で実体が変わる
	// pathからstateの置き場を逆算しない。
	if _, err := Identify("C:go", `C:\gdtvm\gdtvm.exe`, windows); !errors.Is(err, ErrNotShimPath) {
		t.Fatalf("error = %v, want ErrNotShimPath", err)
	}
}

func TestShimFileNameUsesPlatformSuffix(t *testing.T) {
	t.Parallel()
	if got := ShimFileName("go", mustPlatform(t, domain.PlatformWindowsAMD64)); got != "go.exe" {
		t.Errorf("Windows = %q, want %q", got, "go.exe")
	}
	if got := ShimFileName("go", mustPlatform(t, domain.PlatformLinuxAMD64Glibc)); got != "go" {
		t.Errorf("Linux = %q, want %q", got, "go")
	}
	// 作った名前をそのまま正規化すると元のcommand名へ戻る。往復しないと、
	// 配置したshimを自分で解決できない。
	for _, id := range []string{domain.PlatformWindowsAMD64, domain.PlatformLinuxAMD64Glibc} {
		host := mustPlatform(t, id)
		for _, command := range []string{"go", "node", "npm", "dotnet"} {
			fileName := ShimFileName(command, host)
			got, err := NormalizeCommandName(fileName, host)
			if err != nil {
				t.Fatalf("NormalizeCommandName(%q, %s): %v", fileName, id, err)
			}
			if got != command {
				t.Errorf("%s: %q → %q, want %q", id, fileName, got, command)
			}
		}
	}
}
