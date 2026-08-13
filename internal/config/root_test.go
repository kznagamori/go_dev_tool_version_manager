package config

import (
	"testing"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain/port"
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

func windowsUser() port.UserIdentity {
	return port.UserIdentity{
		Name:         "alice",
		ID:           "S-1-5-21-1",
		Home:         `C:\Users\alice`,
		AppDataLocal: `C:\Users\alice\AppData\Local`,
	}
}

func linuxUser() port.UserIdentity {
	return port.UserIdentity{Name: "alice", ID: "1000", Home: "/home/alice"}
}

func modeOf(mode domain.Mode) *domain.Mode { return &mode }

// TestDecideModeFollowsPriority はdocs/04-storage-and-data.md §1の優先順位を固定する。
func TestDecideModeFollowsPriority(t *testing.T) {
	tests := []struct {
		name       string
		request    ModeRequest
		wantMode   domain.Mode
		wantSource ModeSource
	}{
		{
			"overrideが最優先",
			ModeRequest{
				Override:       modeOf(domain.ModePortable),
				SetupState:     modeOf(domain.ModeUser),
				InstallDefault: domain.ModeUser,
			},
			domain.ModePortable, ModeSourceOverride,
		},
		{
			"override無しならsetup state",
			ModeRequest{
				SetupState:     modeOf(domain.ModeUser),
				InstallDefault: domain.ModePortable,
			},
			domain.ModeUser, ModeSourceSetupState,
		},
		{
			"どちらも無ければ導入経路の既定",
			ModeRequest{InstallDefault: domain.ModePortable},
			domain.ModePortable, ModeSourceInstallDefault,
		},
		{
			"bootstrap既定はuser",
			ModeRequest{InstallDefault: domain.ModeUser},
			domain.ModeUser, ModeSourceInstallDefault,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := DecideMode(test.request)
			if err != nil {
				t.Fatalf("DecideMode = %v, want nil", err)
			}
			if got.Mode != test.wantMode || got.Source != test.wantSource {
				t.Errorf("DecideMode = %+v, want mode=%s source=%s", got, test.wantMode, test.wantSource)
			}
		})
	}
}

func TestDecideModeRejectsInvalidMode(t *testing.T) {
	invalid := domain.Mode("system")

	tests := []struct {
		name    string
		request ModeRequest
	}{
		{"override", ModeRequest{Override: &invalid, InstallDefault: domain.ModeUser}},
		{"setup state", ModeRequest{SetupState: &invalid, InstallDefault: domain.ModeUser}},
		{"導入経路の既定", ModeRequest{InstallDefault: invalid}},
		{"既定が空", ModeRequest{}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := DecideMode(test.request)
			if err == nil {
				t.Fatal("DecideMode = nil, want error")
			}
			if err.Code != domain.CodeUsage {
				t.Errorf("Code = %s, want %s", err.Code, domain.CodeUsage)
			}
			if validateErr := err.Validate(); validateErr != nil {
				t.Errorf("typed errorがValidateで落ちた: %v", validateErr)
			}
		})
	}
}

// TestDecideRootsPortable はdocs/04-storage-and-data.md §1.1を固定する。
func TestDecideRootsPortable(t *testing.T) {
	tests := []struct {
		name string
		host domain.Platform
		dir  string
		want string
	}{
		{"linux", linuxHost(t), "/opt/gdtvm", "/opt/gdtvm"},
		{"windows", windowsHost(t), `D:\tools\gdtvm`, `D:\tools\gdtvm`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			roots, err := DecideRoots(RootRequest{
				Mode:          domain.ModePortable,
				Host:          test.host,
				ExecutableDir: test.dir,
			})
			if err != nil {
				t.Fatalf("DecideRoots = %v, want nil", err)
			}
			// §1.1「`gdtvm[.exe]`が存在するcanonical directoryをdistribution root兼data root」
			if roots.DataRoot.Path() != test.want {
				t.Errorf("DataRoot = %q, want %q", roots.DataRoot.Path(), test.want)
			}
			if roots.DistributionRoot.Path() != test.want {
				t.Errorf("DistributionRoot = %q, want %q", roots.DistributionRoot.Path(), test.want)
			}
			if roots.DataRoot.Role() != domain.RoleDataRoot {
				t.Errorf("DataRoot role = %q", roots.DataRoot.Role())
			}
			if roots.DistributionRoot.Role() != domain.RoleDistributionRoot {
				t.Errorf("DistributionRoot role = %q", roots.DistributionRoot.Role())
			}
			if roots.Mode.HomeOverridden {
				t.Error("HomeOverridden = true, want false")
			}
		})
	}
}

// TestDecideRootsUserDefault はdocs/04-storage-and-data.md §1.2のOS既定を固定する。
func TestDecideRootsUserDefault(t *testing.T) {
	t.Run("windowsはLocalAppData直下gdtvm", func(t *testing.T) {
		roots, err := DecideRoots(RootRequest{
			Mode:          domain.ModeUser,
			Host:          windowsHost(t),
			ExecutableDir: `C:\Users\alice\AppData\Local\gdtvm\distribution\current`,
			User:          windowsUser(),
		})
		if err != nil {
			t.Fatalf("DecideRoots = %v, want nil", err)
		}
		const want = `C:\Users\alice\AppData\Local\gdtvm`
		if roots.DataRoot.Path() != want {
			t.Errorf("DataRoot = %q, want %q", roots.DataRoot.Path(), want)
		}
	})

	t.Run("linuxはhome直下.local/share/gdtvm", func(t *testing.T) {
		roots, err := DecideRoots(RootRequest{
			Mode:          domain.ModeUser,
			Host:          linuxHost(t),
			ExecutableDir: "/home/alice/.local/share/gdtvm/distribution/current",
			User:          linuxUser(),
		})
		if err != nil {
			t.Fatalf("DecideRoots = %v, want nil", err)
		}
		const want = "/home/alice/.local/share/gdtvm"
		if roots.DataRoot.Path() != want {
			t.Errorf("DataRoot = %q, want %q", roots.DataRoot.Path(), want)
		}
	})
}

// TestDecideRootsUsesExecutableDirAsDistributionRoot はconfig locatorの前提を固定する。
//
// docs/05-configuration.md §2は「active distribution rootの`gdtvm[.exe]`と同じ
// directoryにある`gdtvm.toml`だけをglobal設定として読む」と定める。
func TestDecideRootsUsesExecutableDirAsDistributionRoot(t *testing.T) {
	roots, err := DecideRoots(RootRequest{
		Mode:          domain.ModeUser,
		Host:          linuxHost(t),
		ExecutableDir: "/home/alice/.local/share/gdtvm/distribution/current",
		User:          linuxUser(),
	})
	if err != nil {
		t.Fatalf("DecideRoots = %v, want nil", err)
	}

	const wantDist = "/home/alice/.local/share/gdtvm/distribution/current"
	if roots.DistributionRoot.Path() != wantDist {
		t.Errorf("DistributionRoot = %q, want %q", roots.DistributionRoot.Path(), wantDist)
	}
	const wantConfig = wantDist + "/" + ConfigFileName
	if roots.ConfigFile.Path() != wantConfig {
		t.Errorf("ConfigFile = %q, want %q", roots.ConfigFile.Path(), wantConfig)
	}
	if roots.ConfigFile.Role() != domain.RoleConfig {
		t.Errorf("ConfigFile role = %q, want %q", roots.ConfigFile.Role(), domain.RoleConfig)
	}
}

func TestDecideRootsConfigFileOnWindows(t *testing.T) {
	roots, err := DecideRoots(RootRequest{
		Mode:          domain.ModePortable,
		Host:          windowsHost(t),
		ExecutableDir: `D:\tools\gdtvm`,
	})
	if err != nil {
		t.Fatalf("DecideRoots = %v, want nil", err)
	}
	const want = `D:\tools\gdtvm\` + ConfigFileName
	if roots.ConfigFile.Path() != want {
		t.Errorf("ConfigFile = %q, want %q", roots.ConfigFile.Path(), want)
	}
}

// TestDecideRootsHomeOverride は`user --home`がdata rootだけを上書きすることを固定する。
func TestDecideRootsHomeOverride(t *testing.T) {
	roots, err := DecideRoots(RootRequest{
		Mode:          domain.ModeUser,
		Host:          linuxHost(t),
		ExecutableDir: "/opt/gdtvm",
		User:          linuxUser(),
		HomeOverride:  "/srv/gdtvm-home",
	})
	if err != nil {
		t.Fatalf("DecideRoots = %v, want nil", err)
	}
	if roots.DataRoot.Path() != "/srv/gdtvm-home" {
		t.Errorf("DataRoot = %q, want /srv/gdtvm-home", roots.DataRoot.Path())
	}
	// distribution rootは`--home`で動かない。clientとregistryの位置は
	// 実行中binaryが決める（docs/05-configuration.md §2）。
	if roots.DistributionRoot.Path() != "/opt/gdtvm" {
		t.Errorf("DistributionRoot = %q, want /opt/gdtvm", roots.DistributionRoot.Path())
	}
	// §1「`--home`実行のshimを永続作成しない」の判断材料になる。
	if !roots.Mode.HomeOverridden {
		t.Error("HomeOverridden = false, want true")
	}
}

// TestDecideRootsRejectsPortableWithHome は§1「`portable`と`--home`は排他」を固定する。
func TestDecideRootsRejectsPortableWithHome(t *testing.T) {
	_, err := DecideRoots(RootRequest{
		Mode:          domain.ModePortable,
		Host:          linuxHost(t),
		ExecutableDir: "/opt/gdtvm",
		HomeOverride:  "/srv/other",
	})
	if err == nil {
		t.Fatal("DecideRoots = nil, want error")
	}
	if err.Code != domain.CodeUsage {
		t.Errorf("Code = %s, want %s", err.Code, domain.CodeUsage)
	}
}

// TestDecideRootsRejectsUnsafeHomeOverride は§1.2の受入条件のうち
// filesystem操作を要しない3件を固定する。
func TestDecideRootsRejectsUnsafeHomeOverride(t *testing.T) {
	tests := []struct {
		name          string
		host          domain.Platform
		executableDir string
		home          string
	}{
		{"相対path（linux）", linuxHost(t), "/opt/gdtvm", "relative/dir"},
		{"相対path（windows）", windowsHost(t), `D:\tools\gdtvm`, `relative\dir`},
		{"空でない単なる名前", linuxHost(t), "/opt/gdtvm", "gdtvm"},
		{"filesystem root（linux）", linuxHost(t), "/opt/gdtvm", "/"},
		{"filesystem root（windows drive）", windowsHost(t), `D:\tools\gdtvm`, `C:\`},
		{"filesystem root（windows UNC share）", windowsHost(t), `D:\tools\gdtvm`, `\\server\share`},
		{"distribution rootそのもの（linux）", linuxHost(t), "/opt/gdtvm", "/opt/gdtvm"},
		{"distribution rootそのもの（末尾区切り）", linuxHost(t), "/opt/gdtvm", "/opt/gdtvm/"},
		{"distribution rootそのもの（windows、case違い）", windowsHost(t), `D:\tools\gdtvm`, `d:\TOOLS\GDTVM`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := DecideRoots(RootRequest{
				Mode:          domain.ModeUser,
				Host:          test.host,
				ExecutableDir: test.executableDir,
				User:          linuxUser(),
				HomeOverride:  test.home,
			})
			if err == nil {
				t.Fatal("DecideRoots = nil, want error")
			}
			if err.Code != domain.CodeUsage {
				t.Errorf("Code = %s, want %s", err.Code, domain.CodeUsage)
			}
			if validateErr := err.Validate(); validateErr != nil {
				t.Errorf("typed errorがValidateで落ちた: %v", validateErr)
			}
		})
	}
}

// TestDecideRootsConfiguredUserDataRoot はglobal configの`paths.user_data_root`を見る。
func TestDecideRootsConfiguredUserDataRoot(t *testing.T) {
	roots, err := DecideRoots(RootRequest{
		Mode:                   domain.ModeUser,
		Host:                   linuxHost(t),
		ExecutableDir:          "/opt/gdtvm",
		User:                   linuxUser(),
		ConfiguredUserDataRoot: "/srv/managed",
	})
	if err != nil {
		t.Fatalf("DecideRoots = %v, want nil", err)
	}
	if roots.DataRoot.Path() != "/srv/managed" {
		t.Errorf("DataRoot = %q, want /srv/managed", roots.DataRoot.Path())
	}

	// `--home`は設定より強い。§2の優先順位で一時optionが最上位のため。
	roots, err = DecideRoots(RootRequest{
		Mode:                   domain.ModeUser,
		Host:                   linuxHost(t),
		ExecutableDir:          "/opt/gdtvm",
		User:                   linuxUser(),
		ConfiguredUserDataRoot: "/srv/managed",
		HomeOverride:           "/srv/temporary",
	})
	if err != nil {
		t.Fatalf("DecideRoots = %v, want nil", err)
	}
	if roots.DataRoot.Path() != "/srv/temporary" {
		t.Errorf("DataRoot = %q, want /srv/temporary", roots.DataRoot.Path())
	}

	// 相対pathの設定は`E_CONFIG_INVALID`にする。
	_, cfgErr := DecideRoots(RootRequest{
		Mode:                   domain.ModeUser,
		Host:                   linuxHost(t),
		ExecutableDir:          "/opt/gdtvm",
		User:                   linuxUser(),
		ConfiguredUserDataRoot: "relative/dir",
	})
	if cfgErr == nil {
		t.Fatal("DecideRoots = nil, want error")
	}
	if cfgErr.Code != domain.CodeConfigInvalid {
		t.Errorf("Code = %s, want %s", cfgErr.Code, domain.CodeConfigInvalid)
	}
}

// TestDecideRootsRejectsMissingUserLookupResult はOS user lookupが値を返さない場合を見る。
//
// 環境変数HOMEやXDG_*で代用しないため（§1.2）、値が無ければ失敗させる。
func TestDecideRootsRejectsMissingUserLookupResult(t *testing.T) {
	tests := []struct {
		name string
		host domain.Platform
		user port.UserIdentity
	}{
		{"windowsでLocalAppDataが空", windowsHost(t), port.UserIdentity{Name: "alice", Home: `C:\Users\alice`}},
		{"linuxでhomeが空", linuxHost(t), port.UserIdentity{Name: "alice", ID: "1000"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := DecideRoots(RootRequest{
				Mode:          domain.ModeUser,
				Host:          test.host,
				ExecutableDir: "/opt/gdtvm",
				User:          test.user,
			})
			if err == nil {
				t.Fatal("DecideRoots = nil, want error")
			}
			if err.Code != domain.CodeFilesystem {
				t.Errorf("Code = %s, want %s", err.Code, domain.CodeFilesystem)
			}
		})
	}
}

func TestDecideRootsRejectsIncompleteRequest(t *testing.T) {
	tests := []struct {
		name    string
		request RootRequest
	}{
		{"executable dirが空", RootRequest{Mode: domain.ModePortable, Host: linuxHost(t)}},
		{"host未設定", RootRequest{Mode: domain.ModePortable, ExecutableDir: "/opt/gdtvm"}},
		{
			"mode未設定",
			RootRequest{Host: linuxHost(t), ExecutableDir: "/opt/gdtvm"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := DecideRoots(test.request)
			if err == nil {
				t.Fatal("DecideRoots = nil, want error")
			}
			if err.Code != domain.CodeUsage {
				t.Errorf("Code = %s, want %s", err.Code, domain.CodeUsage)
			}
		})
	}
}

// TestDecideRootsDoesNotUseEnvironment は環境変数を読まないことを固定する。
//
// §1.2は「`HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`で暗黙置換
// しない」と定める。環境変数を設定してもrootが変わらないことを確認する。
func TestDecideRootsDoesNotUseEnvironment(t *testing.T) {
	for _, name := range []string{"HOME", "XDG_DATA_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME"} {
		t.Setenv(name, "/tmp/should-be-ignored")
	}

	roots, err := DecideRoots(RootRequest{
		Mode:          domain.ModeUser,
		Host:          linuxHost(t),
		ExecutableDir: "/opt/gdtvm",
		User:          linuxUser(),
	})
	if err != nil {
		t.Fatalf("DecideRoots = %v, want nil", err)
	}
	const want = "/home/alice/.local/share/gdtvm"
	if roots.DataRoot.Path() != want {
		t.Errorf("DataRoot = %q, want %q（環境変数が使われている）", roots.DataRoot.Path(), want)
	}
}
