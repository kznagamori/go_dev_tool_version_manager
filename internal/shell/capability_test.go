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

const probeRootDir = "/root"

func mustPlatform(t *testing.T, id string) domain.Platform {
	t.Helper()
	platform, err := domain.ParsePlatform(id)
	if err != nil {
		t.Fatalf("ParsePlatform(%q): %v", id, err)
	}
	return platform
}

// proberFixture は1 testが使うfake portsである。
type proberFixture struct {
	fs       *fake.FileSystem
	links    *fake.LinkManager
	users    *fake.UserLookup
	injector *fake.Injector
	prober   *CapabilityProber
}

func newProberFixture(t *testing.T) *proberFixture {
	t.Helper()
	injector := fake.NewInjector()
	filesystem := fake.NewFileSystem(injector)
	links := fake.NewLinkManager(filesystem)
	users := fake.NewUserLookup(injector, port.UserIdentity{Name: "dev", ID: "1000"})
	filesystem.AddDir(probeRootDir, 0o700)
	prober, err := NewCapabilityProber(filesystem, links, users)
	if err != nil {
		t.Fatalf("NewCapabilityProber: %v", err)
	}
	return &proberFixture{
		fs: filesystem, links: links, users: users, injector: injector, prober: prober,
	}
}

func hasCapability(caps []store.FilesystemCapability, want store.FilesystemCapability) bool {
	for _, capability := range caps {
		if capability == want {
			return true
		}
	}
	return false
}

func TestProbeReportsAllCapabilitiesOnCapableFilesystem(t *testing.T) {
	t.Parallel()
	fixture := newProberFixture(t)
	caps, err := fixture.prober.Probe(probeRootDir, mustPlatform(t, domain.PlatformLinuxAMD64Glibc))
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	// fakeは全能力を持つ既定なので7値すべてが揃う。
	if len(caps) != store.FilesystemCapabilityCount {
		t.Fatalf("caps = %v（%d件）, want %d件", caps, len(caps), store.FilesystemCapabilityCount)
	}
	// docs/04-storage-and-data.md §16「§17.1の値をASCII byte順・重複なしで」。
	for i := 1; i < len(caps); i++ {
		if !(caps[i-1] < caps[i]) {
			t.Errorf("ASCII byte順でない: %v", caps)
			break
		}
	}
}

func TestProbeLeavesNothingBehind(t *testing.T) {
	t.Parallel()
	fixture := newProberFixture(t)
	before := entriesUnder(t, fixture.fs, probeRootDir)
	if _, err := fixture.prober.Probe(probeRootDir, mustPlatform(t, domain.PlatformLinuxAMD64Glibc)); err != nil {
		t.Fatalf("Probe: %v", err)
	}
	after := entriesUnder(t, fixture.fs, probeRootDir)
	// probeが作った実体はすべて消す。残すとdata rootに素性の分からないentryが
	// 残り、次回のprobeが既存entryで失敗する。
	if len(before) != len(after) {
		t.Errorf("probeの残骸がある: before=%v after=%v", before, after)
	}
	// 2回目も同じ結果になる。残骸があると変わる。
	first, err := fixture.prober.Probe(probeRootDir, mustPlatform(t, domain.PlatformLinuxAMD64Glibc))
	if err != nil {
		t.Fatalf("Probe(2回目): %v", err)
	}
	second, err := fixture.prober.Probe(probeRootDir, mustPlatform(t, domain.PlatformLinuxAMD64Glibc))
	if err != nil {
		t.Fatalf("Probe(3回目): %v", err)
	}
	if len(first) != len(second) {
		t.Errorf("実行ごとに結果が変わる: %v / %v", first, second)
	}
}

func TestProbeReportsMissingLinkCapability(t *testing.T) {
	t.Parallel()
	fixture := newProberFixture(t)
	// Windows標準userがsymlinkを作れない状況。
	fixture.links.DefaultCaps = port.LinkCapabilities{Junction: true, Hardlink: true}

	caps, err := fixture.prober.Probe(probeRootDir, mustPlatform(t, domain.PlatformWindowsAMD64))
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if hasCapability(caps, store.CapabilitySymlink) {
		t.Error("作れないsymlinkを能力として報告した")
	}
	if !hasCapability(caps, store.CapabilityJunction) {
		t.Error("作れるjunctionを報告しなかった")
	}
}

func TestProbeDoesNotAssumeOwnershipWithoutEvidence(t *testing.T) {
	t.Parallel()
	fixture := newProberFixture(t)
	// 所有者の概念が無いfilesystem（FAT等）ではOwnerOfが空を返す。
	fixture.users.SetOwner(probeRootDir, "")

	caps, err := fixture.prober.Probe(probeRootDir, mustPlatform(t, domain.PlatformLinuxAMD64Glibc))
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	// **「一致した」と見なさない。** 所有を根拠にできない状態で
	// owner-enforcementを報告すると、他userが書けるrootをsetupが受け入れる。
	if hasCapability(caps, store.CapabilityOwnerEnforce) {
		t.Error("所有者が空なのにowner-enforcementを報告した")
	}
}

func TestProbeReportsForeignOwnerAsMissing(t *testing.T) {
	t.Parallel()
	fixture := newProberFixture(t)
	fixture.users.SetOwner(probeRootDir, "someone-else")

	caps, err := fixture.prober.Probe(probeRootDir, mustPlatform(t, domain.PlatformLinuxAMD64Glibc))
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	// docs/09-platform.md §2.3「他user所有…を拒否する」。
	if hasCapability(caps, store.CapabilityOwnerEnforce) {
		t.Error("他user所有なのにowner-enforcementを報告した")
	}
}

func TestProbeDistinguishesFailureFromMissingCapability(t *testing.T) {
	t.Parallel()
	fixture := newProberFixture(t)
	fixture.injector.Fail(fake.OpUserCurrent, 0, 0, errors.New("lookup unavailable"))

	// **「能力が無い」と「確かめられなかった」を混ぜない。** 前者はfilesystemを
	// 変えれば解決し、後者は権限やdiskの問題である。
	_, err := fixture.prober.Probe(probeRootDir, mustPlatform(t, domain.PlatformLinuxAMD64Glibc))
	if !errors.Is(err, ErrCapabilityProbe) {
		t.Fatalf("error = %v, want ErrCapabilityProbe", err)
	}
}

func TestProbeRejectsUnusableRequest(t *testing.T) {
	t.Parallel()
	fixture := newProberFixture(t)
	if _, err := fixture.prober.Probe(probeRootDir, domain.Platform{}); !errors.Is(err, ErrCapabilityProbe) {
		t.Errorf("host未設定 error = %v, want ErrCapabilityProbe", err)
	}
	if _, err := fixture.prober.Probe("", mustPlatform(t, domain.PlatformLinuxAMD64Glibc)); !errors.Is(err, ErrCapabilityProbe) {
		t.Errorf("空dir error = %v, want ErrCapabilityProbe", err)
	}
}

func TestNewCapabilityProberRequiresPorts(t *testing.T) {
	t.Parallel()
	injector := fake.NewInjector()
	filesystem := fake.NewFileSystem(injector)
	links := fake.NewLinkManager(filesystem)
	users := fake.NewUserLookup(injector, port.UserIdentity{ID: "1000"})
	if _, err := NewCapabilityProber(nil, links, users); err == nil {
		t.Error("FileSystem未設定が通った")
	}
	if _, err := NewCapabilityProber(filesystem, nil, users); err == nil {
		t.Error("LinkManager未設定が通った")
	}
	if _, err := NewCapabilityProber(filesystem, links, nil); err == nil {
		t.Error("UserLookup未設定が通った")
	}
}

func TestCheckRequiredNamesMissingCapabilities(t *testing.T) {
	t.Parallel()
	windows := mustPlatform(t, domain.PlatformWindowsAMD64)
	linux := mustPlatform(t, domain.PlatformLinuxAMD64Glibc)

	full := []store.FilesystemCapability{
		store.CapabilityAtomicReplace, store.CapabilityDirectoryRename,
		store.CapabilityFileIdentity, store.CapabilityOwnerEnforce,
		store.CapabilityJunction, store.CapabilitySymlink,
	}
	if err := CheckRequired(full, windows); err != nil {
		t.Errorf("Windowsの必須が揃っているのに落ちた: %v", err)
	}
	if err := CheckRequired(full, linux); err != nil {
		t.Errorf("Linuxの必須が揃っているのに落ちた: %v", err)
	}

	// **足りないcapabilityを名指しする。** 「対象外」だけでは利用者が何を
	// 直せばよいか分からない（docs/09-platform.md §9）。
	cases := map[string]struct {
		caps   []store.FilesystemCapability
		host   domain.Platform
		wantIn string
	}{
		"Windowsでjunctionが無い": {
			caps: []store.FilesystemCapability{
				store.CapabilityAtomicReplace, store.CapabilityDirectoryRename,
				store.CapabilityFileIdentity, store.CapabilityOwnerEnforce,
				store.CapabilitySymlink,
			},
			host: windows, wantIn: string(store.CapabilityJunction),
		},
		"Linuxでsymlinkが無い": {
			caps: []store.FilesystemCapability{
				store.CapabilityAtomicReplace, store.CapabilityDirectoryRename,
				store.CapabilityFileIdentity, store.CapabilityOwnerEnforce,
				store.CapabilityJunction,
			},
			host: linux, wantIn: string(store.CapabilitySymlink),
		},
		"atomic-replaceが無い": {
			caps: []store.FilesystemCapability{
				store.CapabilityDirectoryRename, store.CapabilityFileIdentity,
				store.CapabilityOwnerEnforce, store.CapabilitySymlink,
			},
			host: linux, wantIn: string(store.CapabilityAtomicReplace),
		},
		"何も無い": {caps: nil, host: linux, wantIn: string(store.CapabilityFileIdentity)},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			err := CheckRequired(tc.caps, tc.host)
			if err == nil {
				t.Fatal("必須が欠けているのに通った")
			}
			if err.Code != domain.CodePlatformUnsupported {
				t.Errorf("code = %q, want %q", err.Code, domain.CodePlatformUnsupported)
			}
			// filesystemを変えるまで解消しない。retryさせない。
			if err.Retryable {
				t.Error("Retryable = true, 能力不足は再試行で解消しない")
			}
			if err.Cause == nil || !strings.Contains(err.Cause.Error(), tc.wantIn) {
				t.Errorf("cause = %v, %q を含むべき", err.Cause, tc.wantIn)
			}
		})
	}
}

func TestSortedCapabilitiesDropsUnavailable(t *testing.T) {
	t.Parallel()
	got := sortedCapabilities(map[store.FilesystemCapability]bool{
		store.CapabilitySymlink:       true,
		store.CapabilityAtomicReplace: true,
		store.CapabilityHardlink:      false,
	})
	want := []store.FilesystemCapability{store.CapabilityAtomicReplace, store.CapabilitySymlink}
	if len(got) != len(want) {
		t.Fatalf("caps = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("caps = %v, want %v", got, want)
		}
	}
}

// entriesUnder はroot配下のentryを列挙する。probeの残骸検査に使う。
func entriesUnder(t *testing.T, filesystem *fake.FileSystem, root string) []string {
	t.Helper()
	var paths []string
	err := filesystem.Walk(root, func(path string, info port.FileInfo) error {
		if path != root {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	return paths
}
