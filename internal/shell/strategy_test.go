package shell

import (
	"errors"
	"strings"
	"testing"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain/port"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain/port/fake"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/store"
)

// deciderFixture は1 testが使うfake portsと標準的なStrategyRequestである。
type deciderFixture struct {
	fs      *fake.FileSystem
	links   *fake.LinkManager
	decider *StrategyDecider
	req     StrategyRequest
}

func newDeciderFixture(t *testing.T, platformID string) *deciderFixture {
	t.Helper()
	filesystem := fake.NewFileSystem(fake.NewInjector())
	links := fake.NewLinkManager(filesystem)
	decider, err := NewStrategyDecider(filesystem, links)
	if err != nil {
		t.Fatalf("NewStrategyDecider: %v", err)
	}
	filesystem.AddDir(probeRootDir, 0o700)
	filesystem.AddFile(probeRootDir+"/gdtvm", []byte("client"), 0o700)
	return &deciderFixture{
		fs:      filesystem,
		links:   links,
		decider: decider,
		req: StrategyRequest{
			ShimDir:    probeRootDir + "/shims",
			ClientPath: probeRootDir + "/gdtvm",
			Host:       mustPlatform(t, platformID),
		},
	}
}

func TestDecideUsesPlatformFixedStrategies(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		platformID  string
		wantCurrent store.LinkStrategy
		wantShim    store.ShimStrategy
	}{
		// docs/04-storage-and-data.md §16はcurrent link方式を**選択肢ではなく
		// 固定**として定める。
		"Windows": {
			platformID:  domain.PlatformWindowsAMD64,
			wantCurrent: store.LinkJunction,
			wantShim:    store.ShimHardlink,
		},
		"Linux": {
			platformID:  domain.PlatformLinuxAMD64Glibc,
			wantCurrent: store.LinkSymlink,
			wantShim:    store.ShimSymlink,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			fixture := newDeciderFixture(t, tc.platformID)
			got, err := fixture.decider.Decide(fixture.req)
			if err != nil {
				t.Fatalf("Decide: %v", err)
			}
			if got.CurrentLink != tc.wantCurrent {
				t.Errorf("CurrentLink = %q, want %q", got.CurrentLink, tc.wantCurrent)
			}
			if got.Shim != tc.wantShim {
				t.Errorf("Shim = %q, want %q", got.Shim, tc.wantShim)
			}
			if got.UsesHardlink != (tc.wantShim == store.ShimHardlink) {
				t.Errorf("UsesHardlink = %v, Shim = %q と食い違う", got.UsesHardlink, got.Shim)
			}
		})
	}
}

func TestDecideLeavesNoProbeBehind(t *testing.T) {
	t.Parallel()
	fixture := newDeciderFixture(t, domain.PlatformLinuxAMD64Glibc)
	if _, err := fixture.decider.Decide(fixture.req); err != nil {
		t.Fatalf("Decide: %v", err)
	}
	// probeが残るとshim directoryに素性の分からないentryが残り、次回のprobeが
	// 既存entryで失敗する。
	entries := entriesUnder(t, fixture.fs, fixture.req.ShimDir)
	if len(entries) != 0 {
		t.Errorf("probeの残骸がある: %v", entries)
	}
	// 2回目も同じ結果になる。
	if _, err := fixture.decider.Decide(fixture.req); err != nil {
		t.Fatalf("Decide(2回目): %v", err)
	}
}

func TestDecideRejectsWhenLinkCannotBeCreated(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		platformID string
		caps       port.LinkCapabilities
		wantIn     string
	}{
		// docs/09-platform.md §3.3「同一volumeで安全にrootを導出できるとき」は
		// hardlinkが張れる条件そのものである。**volume名を比べて推測しない。**
		"Windowsでhardlinkを張れない": {
			platformID: domain.PlatformWindowsAMD64,
			caps:       port.LinkCapabilities{Junction: true, Symlink: true},
			wantIn:     "hardlink",
		},
		"Linuxでsymlinkを張れない": {
			platformID: domain.PlatformLinuxAMD64Glibc,
			caps:       port.LinkCapabilities{Junction: true, Hardlink: true},
			wantIn:     "symlink",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			fixture := newDeciderFixture(t, tc.platformID)
			fixture.links.DefaultCaps = tc.caps

			_, err := fixture.decider.Decide(fixture.req)
			if err == nil {
				t.Fatal("linkを張れないのに通った")
			}
			// `fallback-resolver`をP11-04へ回しているため、v0.1では
			// `E_PLATFORM_UNSUPPORTED`になる（docs/04-storage-and-data.md §16）。
			if err.Code != domain.CodePlatformUnsupported {
				t.Errorf("code = %q, want %q", err.Code, domain.CodePlatformUnsupported)
			}
			if err.Retryable {
				t.Error("Retryable = true, filesystemを変えるまで解消しない")
			}
			if err.Cause == nil || !strings.Contains(err.Cause.Error(), tc.wantIn) {
				t.Errorf("cause = %v, %q を含むべき", err.Cause, tc.wantIn)
			}
		})
	}
}

func TestDecideRejectsUnusableRequest(t *testing.T) {
	t.Parallel()
	cases := map[string]func(*StrategyRequest){
		"host未設定":       func(r *StrategyRequest) { r.Host = domain.Platform{} },
		"shim dirが空":    func(r *StrategyRequest) { r.ShimDir = "" },
		"client pathが空": func(r *StrategyRequest) { r.ClientPath = "" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			fixture := newDeciderFixture(t, domain.PlatformLinuxAMD64Glibc)
			mutate(&fixture.req)
			if _, err := fixture.decider.Decide(fixture.req); err == nil {
				t.Fatal("不正な要求が通った")
			}
		})
	}
}

func TestNewStrategyDeciderRequiresPorts(t *testing.T) {
	t.Parallel()
	filesystem := fake.NewFileSystem(fake.NewInjector())
	if _, err := NewStrategyDecider(nil, fake.NewLinkManager(filesystem)); err == nil {
		t.Error("FileSystem未設定が通った")
	}
	if _, err := NewStrategyDecider(filesystem, nil); err == nil {
		t.Error("LinkManager未設定が通った")
	}
}

func TestSelectCapabilitiesIncludesHardlinkOnlyWhenUsed(t *testing.T) {
	t.Parallel()
	windows := mustPlatform(t, domain.PlatformWindowsAMD64)
	linux := mustPlatform(t, domain.PlatformLinuxAMD64Glibc)
	// 7値すべてを実測できた状態。
	probed := []store.FilesystemCapability{
		store.CapabilityAtomicReplace, store.CapabilityDirectoryRename,
		store.CapabilityFileIdentity, store.CapabilityHardlink,
		store.CapabilityJunction, store.CapabilityOwnerEnforce,
		store.CapabilitySymlink,
	}

	t.Run("hardlinkを使う", func(t *testing.T) {
		t.Parallel()
		got, err := SelectCapabilities(probed,
			Strategies{Shim: store.ShimHardlink, UsesHardlink: true}, windows)
		if err != nil {
			t.Fatalf("SelectCapabilities: %v", err)
		}
		if !hasCapability(got, store.CapabilityHardlink) {
			t.Error("hardlinkを使うのにcapabilityへ含めていない")
		}
		// docs/04-storage-and-data.md §16はWindowsの必須へsymlinkを含めない。
		// **実測できた能力をそのまま載せない** —— 載せると「setupが依存する
		// 能力」ではなく「たまたま使えた能力」の一覧になる。
		if hasCapability(got, store.CapabilitySymlink) {
			t.Error("使わないsymlinkをcapabilityへ含めた")
		}
	})

	t.Run("hardlinkを使わない", func(t *testing.T) {
		t.Parallel()
		got, err := SelectCapabilities(probed,
			Strategies{Shim: store.ShimSymlink, UsesHardlink: false}, linux)
		if err != nil {
			t.Fatalf("SelectCapabilities: %v", err)
		}
		// §16「**hardlinkを使う場合だけ**capabilityへ`hardlink`を含める」。
		if hasCapability(got, store.CapabilityHardlink) {
			t.Error("使わないhardlinkをcapabilityへ含めた")
		}
		if hasCapability(got, store.CapabilityJunction) {
			t.Error("Linuxで使わないjunctionをcapabilityへ含めた")
		}
		if !hasCapability(got, store.CapabilitySymlink) {
			t.Error("使うsymlinkをcapabilityへ含めていない")
		}
	})

	t.Run("ASCII byte順", func(t *testing.T) {
		t.Parallel()
		got, err := SelectCapabilities(probed,
			Strategies{Shim: store.ShimHardlink, UsesHardlink: true}, windows)
		if err != nil {
			t.Fatalf("SelectCapabilities: %v", err)
		}
		// §16「ASCII byte順・重複なしで1～7件」。
		if len(got) < 1 || len(got) > store.FilesystemCapabilityCount {
			t.Fatalf("件数 = %d, 1〜%d件であるべき", len(got), store.FilesystemCapabilityCount)
		}
		for i := 1; i < len(got); i++ {
			if !(got[i-1] < got[i]) {
				t.Fatalf("ASCII byte順でない: %v", got)
			}
		}
	})
}

func TestSelectCapabilitiesRejectsMissingRequired(t *testing.T) {
	t.Parallel()
	linux := mustPlatform(t, domain.PlatformLinuxAMD64Glibc)
	// owner-enforcementが無い。
	probed := []store.FilesystemCapability{
		store.CapabilityAtomicReplace, store.CapabilityDirectoryRename,
		store.CapabilityFileIdentity, store.CapabilitySymlink,
	}
	_, err := SelectCapabilities(probed, Strategies{Shim: store.ShimSymlink}, linux)
	if err == nil {
		t.Fatal("必須が欠けているのに通った")
	}
	if err.Code != domain.CodePlatformUnsupported {
		t.Errorf("code = %q, want %q", err.Code, domain.CodePlatformUnsupported)
	}
}

func TestSelectCapabilitiesRejectsContradictoryHardlink(t *testing.T) {
	t.Parallel()
	windows := mustPlatform(t, domain.PlatformWindowsAMD64)
	// Windowsの必須は揃うが、hardlinkの能力が無い。
	probed := []store.FilesystemCapability{
		store.CapabilityAtomicReplace, store.CapabilityDirectoryRename,
		store.CapabilityFileIdentity, store.CapabilityJunction,
		store.CapabilityOwnerEnforce,
	}
	// 方式としてhardlinkを選んだのに能力が無い。矛盾した状態でPlanを作らない。
	_, err := SelectCapabilities(probed,
		Strategies{Shim: store.ShimHardlink, UsesHardlink: true}, windows)
	if err == nil {
		t.Fatal("矛盾した組合せが通った")
	}
	if err.Code != domain.CodePlatformUnsupported {
		t.Errorf("code = %q, want %q", err.Code, domain.CodePlatformUnsupported)
	}
	if err.Cause == nil || !strings.Contains(err.Cause.Error(), "hardlink") {
		t.Errorf("cause = %v, hardlinkを名指しすべき", err.Cause)
	}
}

func TestFallbackResolverIsNotSelected(t *testing.T) {
	t.Parallel()
	// P7-02着手時の利用者判断により、内蔵fallback resolverはP11-04で扱う。
	// **どのplatformでも選ばれないこと**を固定する。選ばれると、まだ存在しない
	// resolver binaryを展開しようとする。
	for _, platformID := range []string{domain.PlatformWindowsAMD64, domain.PlatformLinuxAMD64Glibc} {
		fixture := newDeciderFixture(t, platformID)
		got, err := fixture.decider.Decide(fixture.req)
		if err != nil {
			t.Fatalf("Decide(%s): %v", platformID, err)
		}
		if got.Shim == store.ShimFallbackResolver {
			t.Errorf("%s でfallback-resolverが選ばれた", platformID)
		}
	}
	// 表そのものにも入っていない。
	for host, strategy := range platformShimStrategies {
		if strategy == store.ShimFallbackResolver {
			t.Errorf("%s の表にfallback-resolverが入っている", host)
		}
	}
}

func TestDecideReportsFilesystemErrorSeparately(t *testing.T) {
	t.Parallel()
	fixture := newDeciderFixture(t, domain.PlatformLinuxAMD64Glibc)
	fixture.fs.Injector().Fail(fake.OpMkdirAll, 0, 0, errors.New("disk full"))

	_, err := fixture.decider.Decide(fixture.req)
	if err == nil {
		t.Fatal("shim directoryを作れないのに通った")
	}
	// **「能力が無い」と「書けなかった」を混ぜない。** diskの問題は
	// filesystemを変えても解決しないが、空ければ解決する。
	if err.Code != domain.CodeFilesystem {
		t.Errorf("code = %q, want %q", err.Code, domain.CodeFilesystem)
	}
	if !err.Retryable {
		t.Error("Retryable = false, 書込み失敗は状態を直せば再実行できる")
	}
}
