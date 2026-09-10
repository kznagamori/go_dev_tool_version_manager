package shim

import (
	"errors"
	"testing"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain/port"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain/port/fake"
)

// deployFixture は1 testが使うfake filesystemと標準的なDeployRequestである。
type deployFixture struct {
	fs       *fake.FileSystem
	links    *fake.LinkManager
	deployer *Deployer
	req      DeployRequest
}

// newDeployFixture はstrategyに応じたfixtureを組み立てる。
//
// **Windows/Linuxの両方をfakeで組める。** 実際のjunction/hardlinkはP7-01の
// production adapterがCIで確かめており、ここで固定するのは配置の判断である。
func newDeployFixture(t *testing.T, strategy Strategy) *deployFixture {
	t.Helper()
	platformID := domain.PlatformLinuxAMD64Glibc
	root := "/root"
	if strategy == StrategyHardlink {
		platformID = domain.PlatformWindowsAMD64
	}
	filesystem := fake.NewFileSystem(fake.NewInjector())
	links := fake.NewLinkManager(filesystem)
	deployer, err := NewDeployer(filesystem, links)
	if err != nil {
		t.Fatalf("NewDeployer: %v", err)
	}
	clientPath := root + "/gdtvm"
	filesystem.AddDir(root, 0o700)
	filesystem.AddFile(clientPath, []byte("client"), 0o700)
	return &deployFixture{
		fs:       filesystem,
		links:    links,
		deployer: deployer,
		req: DeployRequest{
			ShimDir:    root + "/" + ShimDirName,
			ClientPath: clientPath,
			Commands:   []string{"go", "node", "npm"},
			Host:       mustPlatform(t, platformID),
			Strategy:   strategy,
		},
	}
}

func actionsOf(results []Result) map[string]Action {
	actions := make(map[string]Action, len(results))
	for _, result := range results {
		actions[result.CommandName] = result.Action
	}
	return actions
}

func TestDeployCreatesShimsForBothStrategies(t *testing.T) {
	t.Parallel()
	for _, strategy := range []Strategy{StrategySymlink, StrategyHardlink} {
		t.Run(string(strategy), func(t *testing.T) {
			t.Parallel()
			fixture := newDeployFixture(t, strategy)
			results, err := fixture.deployer.Deploy(fixture.req)
			if err != nil {
				t.Fatalf("Deploy: %v", err)
			}
			if len(results) != len(fixture.req.Commands) {
				t.Fatalf("results = %d件, want %d件", len(results), len(fixture.req.Commands))
			}
			// **結果はcommand名順である。** 入力順に依存すると、setupの報告と
			// `doctor`の出力が実行ごとに入れ替わる。
			for i, want := range []string{"go", "node", "npm"} {
				if results[i].CommandName != want {
					t.Errorf("results[%d] = %q, want %q", i, results[i].CommandName, want)
				}
				if results[i].Action != ActionCreate {
					t.Errorf("%q の Action = %q, want %q",
						want, results[i].Action, ActionCreate)
				}
			}

			wantKind := port.LinkSymlink
			if strategy == StrategyHardlink {
				wantKind = port.LinkHardlink
			}
			for _, result := range results {
				kind, kindErr := fixture.links.Kind(result.Path)
				if kindErr != nil {
					t.Fatalf("Kind(%s): %v", result.Path, kindErr)
				}
				// docs/09-platform.md §3.3はWindowsをhardlink、§5.1はLinuxを
				// relative symlinkと定める。方式を取り違えると、片方のOSで
				// 作れない実体を作ろうとする。
				if kind != wantKind {
					t.Errorf("%s の kind = %q, want %q", result.Path, kind, wantKind)
				}
			}
		})
	}
}

func TestDeploySymlinkTargetIsRelative(t *testing.T) {
	t.Parallel()
	fixture := newDeployFixture(t, StrategySymlink)
	fixture.req.Commands = []string{"go"}
	results, err := fixture.deployer.Deploy(fixture.req)
	if err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	target, err := fixture.links.ReadLink(results[0].Path)
	if err != nil {
		t.Fatalf("ReadLink: %v", err)
	}
	// docs/09-platform.md §5.1「clientへの**relative** symlink」。absoluteだと
	// root全体を移した瞬間に全shimが壊れる。
	if target != "../gdtvm" {
		t.Errorf("target = %q, want %q", target, "../gdtvm")
	}
}

func TestDeployIsIdempotent(t *testing.T) {
	t.Parallel()
	for _, strategy := range []Strategy{StrategySymlink, StrategyHardlink} {
		t.Run(string(strategy), func(t *testing.T) {
			t.Parallel()
			fixture := newDeployFixture(t, strategy)
			if _, err := fixture.deployer.Deploy(fixture.req); err != nil {
				t.Fatalf("Deploy(1回目): %v", err)
			}
			results, err := fixture.deployer.Deploy(fixture.req)
			if err != nil {
				t.Fatalf("Deploy(2回目): %v", err)
			}
			// docs/09-platform.md §7「`setup`は冪等とする。…既に一致する項目を
			// **no-opとして報告する**」。作り直すと、実行中のshimが指すfileを
			// 差し替える。
			for command, action := range actionsOf(results) {
				if action != ActionNoop {
					t.Errorf("%q の Action = %q, want %q", command, action, ActionNoop)
				}
			}
		})
	}
}

func TestDeployReplacesShimPointingElsewhere(t *testing.T) {
	t.Parallel()
	fixture := newDeployFixture(t, StrategySymlink)
	fixture.req.Commands = []string{"go"}
	shimPath := fixture.req.ShimDir + "/go"

	// 旧clientを指すshimがある状態（clientを入れ替えたあとのsetup再実行）。
	fixture.fs.AddDir(fixture.req.ShimDir, 0o700)
	fixture.fs.AddFile("/root/old-gdtvm", []byte("old"), 0o700)
	fixture.fs.AddLink(shimPath, port.LinkSymlink, "../old-gdtvm")

	results, err := fixture.deployer.Deploy(fixture.req)
	if err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	if results[0].Action != ActionReplace {
		t.Errorf("Action = %q, want %q", results[0].Action, ActionReplace)
	}
	target, err := fixture.links.ReadLink(shimPath)
	if err != nil {
		t.Fatalf("ReadLink: %v", err)
	}
	if target != "../gdtvm" {
		t.Errorf("target = %q, want %q", target, "../gdtvm")
	}
	// **link先の実体を消していない。** 張り替えでclientを消すと、他のshimも
	// まとめて壊れる。
	if _, statErr := fixture.fs.Stat("/root/old-gdtvm"); statErr != nil {
		t.Errorf("旧clientが消えている: %v", statErr)
	}
}

func TestDeployReplacesShimOfWrongKind(t *testing.T) {
	t.Parallel()
	fixture := newDeployFixture(t, StrategySymlink)
	fixture.req.Commands = []string{"go"}
	shimPath := fixture.req.ShimDir + "/go"

	// 方式が変わった場合（setup再実行でstrategyが変わる）は作り直す。
	// link同士の置換であり、実体を消さない。
	fixture.fs.AddDir(fixture.req.ShimDir, 0o700)
	fixture.fs.AddLink(shimPath, port.LinkHardlink, fixture.req.ClientPath)

	results, err := fixture.deployer.Deploy(fixture.req)
	if err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	if results[0].Action != ActionReplace {
		t.Errorf("Action = %q, want %q", results[0].Action, ActionReplace)
	}
	kind, err := fixture.links.Kind(shimPath)
	if err != nil {
		t.Fatalf("Kind: %v", err)
	}
	if kind != port.LinkSymlink {
		t.Errorf("kind = %q, want %q", kind, port.LinkSymlink)
	}
}

func TestDeployRefusesForeignEntry(t *testing.T) {
	t.Parallel()
	cases := map[string]func(*deployFixture, string){
		"通常file": func(f *deployFixture, path string) {
			f.fs.AddFile(path, []byte("user data"), 0o600)
		},
		"directory": func(f *deployFixture, path string) {
			f.fs.AddDir(path, 0o700)
		},
	}
	for name, place := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			fixture := newDeployFixture(t, StrategySymlink)
			fixture.req.Commands = []string{"go"}
			shimPath := fixture.req.ShimDir + "/go"
			fixture.fs.AddDir(fixture.req.ShimDir, 0o700)
			place(fixture, shimPath)

			// **利用者が置いたものを黙って消さない**（docs/09-platform.md §3.2の
			// 「自動置換せず`doctor`診断とする」と同じ扱い）。
			_, err := fixture.deployer.Deploy(fixture.req)
			if !errors.Is(err, ErrForeignShim) {
				t.Fatalf("error = %v, want ErrForeignShim", err)
			}
			if _, statErr := fixture.fs.Stat(shimPath); statErr != nil {
				t.Errorf("拒否したのに実体が消えている: %v", statErr)
			}
		})
	}
}

func TestDeployRejectsMismatchedStrategy(t *testing.T) {
	t.Parallel()
	// docs/04-storage-and-data.md §16はWindowsを`hardlink|fallback-resolver`、
	// Linuxを`symlink|fallback-resolver`と定める。platformが定める方式と食い違う
	// 組合せを通すと、作れない実体を作ろうとして途中で止まる。
	windowsSymlink := newDeployFixture(t, StrategyHardlink)
	windowsSymlink.req.Strategy = StrategySymlink
	if _, err := windowsSymlink.deployer.Deploy(windowsSymlink.req); err == nil {
		t.Error("Windows + symlink が通った")
	}

	linuxHardlink := newDeployFixture(t, StrategySymlink)
	linuxHardlink.req.Strategy = StrategyHardlink
	if _, err := linuxHardlink.deployer.Deploy(linuxHardlink.req); err == nil {
		t.Error("Linux + hardlink が通った")
	}
}

func TestDeployRejectsUnusableRequest(t *testing.T) {
	t.Parallel()
	cases := map[string]func(*DeployRequest){
		"host未設定":        func(r *DeployRequest) { r.Host = domain.Platform{} },
		"shim dirが相対":    func(r *DeployRequest) { r.ShimDir = "shims" },
		"client pathが相対": func(r *DeployRequest) { r.ClientPath = "gdtvm" },
		"strategyが空":     func(r *DeployRequest) { r.Strategy = "" },
		"未知のstrategy":    func(r *DeployRequest) { r.Strategy = "fallback-resolver" },
		"空のcommand名":     func(r *DeployRequest) { r.Commands = []string{"go", ""} },
		// 同じcommandを2回作ると2回目が1回目を置き換える。宣言の誤りを畳まない。
		"command名の重複": func(r *DeployRequest) { r.Commands = []string{"go", "go"} },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			fixture := newDeployFixture(t, StrategySymlink)
			mutate(&fixture.req)
			if _, err := fixture.deployer.Deploy(fixture.req); err == nil {
				t.Fatal("不正な要求が通った")
			}
			// 検査だけで作用が起きていないこと。
			if _, statErr := fixture.fs.Stat(fixture.req.ShimDir + "/go"); statErr == nil {
				t.Error("拒否したのにshimができている")
			}
		})
	}
}

func TestDeployStopsOnFirstFailure(t *testing.T) {
	t.Parallel()
	fixture := newDeployFixture(t, StrategySymlink)
	fixture.req.Commands = []string{"go", "node", "npm"}
	// 2件目（node）のshim pathへ通常fileを置く。
	fixture.fs.AddDir(fixture.req.ShimDir, 0o700)
	fixture.fs.AddFile(fixture.req.ShimDir+"/node", []byte("user"), 0o600)

	results, err := fixture.deployer.Deploy(fixture.req)
	if !errors.Is(err, ErrForeignShim) {
		t.Fatalf("error = %v, want ErrForeignShim", err)
	}
	// **途中で止める。** 残りを続けると、失敗したcommandだけが古いままPATHに
	// 載った状態になる。処理済みは返して、どこまで進んだか分かるようにする。
	if len(results) != 1 || results[0].CommandName != "go" {
		t.Errorf("results = %+v, goの1件だけであるべき", results)
	}
	if _, statErr := fixture.fs.Stat(fixture.req.ShimDir + "/npm"); statErr == nil {
		t.Error("失敗後もnpmを作っている")
	}
}

func TestNewDeployerRequiresPorts(t *testing.T) {
	t.Parallel()
	filesystem := fake.NewFileSystem(fake.NewInjector())
	if _, err := NewDeployer(nil, fake.NewLinkManager(filesystem)); err == nil {
		t.Error("FileSystem未設定が通った")
	}
	if _, err := NewDeployer(filesystem, nil); err == nil {
		t.Error("LinkManager未設定が通った")
	}
}
