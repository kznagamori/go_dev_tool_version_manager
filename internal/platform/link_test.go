package platform

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain/port"
)

// linkFixture は1 testが使うdirectoryとその中身である。
type linkFixture struct {
	root      string
	targetDir string
	// payload はtargetDir内のfileである。RemoveLinkがtarget側へ触れていないことを
	// これで確かめる。
	payload string
	// targetFile はhardlink用のregular fileである。
	targetFile string
}

func newLinkFixture(t *testing.T) linkFixture {
	t.Helper()
	// t.TempDirはOSのtemp配下を返す。macOS等ではsymlinkを含むためEvalSymlinksで
	// 実体pathへ直す。canonicalでないpathはadapterが拒否する。
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}
	fixture := linkFixture{
		root:       root,
		targetDir:  filepath.Join(root, "target"),
		payload:    filepath.Join(root, "target", "payload.txt"),
		targetFile: filepath.Join(root, "regular.txt"),
	}
	if err := os.Mkdir(fixture.targetDir, 0o700); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	if err := os.WriteFile(fixture.payload, []byte("payload"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.WriteFile(fixture.targetFile, []byte("regular"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return fixture
}

// skipUnlessSymlinkable はsymlinkを作れない環境でtestを飛ばす。
//
// Windows標準userは特権もDeveloper Modeも無いとsymlinkを作れない
// （docs/09-platform.md §3.3）。作れないこと自体は仕様どおりであり、
// symlinkの振る舞いを確かめるtestの失敗にしない。
func skipUnlessSymlinkable(t *testing.T, manager *LinkManager, fixture linkFixture) {
	t.Helper()
	caps, err := manager.Capabilities(fixture.root)
	if err != nil {
		t.Fatalf("Capabilities: %v", err)
	}
	if !caps.Symlink {
		t.Skip("この環境ではsymlinkを作成できない")
	}
}

func TestLinkManagerCreatesRelativeSymlink(t *testing.T) {
	t.Parallel()
	manager := NewLinkManager()
	fixture := newLinkFixture(t)
	skipUnlessSymlinkable(t, manager, fixture)

	linkPath := filepath.Join(fixture.root, "current")
	if err := manager.CreateSymlink(linkPath, fixture.targetDir, true); err != nil {
		t.Fatalf("CreateSymlink: %v", err)
	}

	kind, err := manager.Kind(linkPath)
	if err != nil {
		t.Fatalf("Kind: %v", err)
	}
	if kind != port.LinkSymlink {
		t.Errorf("Kind = %q, want %q", kind, port.LinkSymlink)
	}

	target, err := manager.ReadLink(linkPath)
	if err != nil {
		t.Fatalf("ReadLink: %v", err)
	}
	// docs/09-platform.md §5.1「同じdata root内payloadへのrelative symlink」。
	// 保存値がabsoluteだとroot全体を移した瞬間に壊れる。
	if filepath.IsAbs(target) {
		t.Errorf("ReadLink = %q, relativeであるべき", target)
	}
	if target != "target" {
		t.Errorf("ReadLink = %q, want %q", target, "target")
	}
	// 実際に辿れることを確かめる。相対の基準を取り違えていると読めない。
	got, err := os.ReadFile(filepath.Join(linkPath, "payload.txt"))
	if err != nil {
		t.Fatalf("link経由でpayloadを読めない: %v", err)
	}
	if string(got) != "payload" {
		t.Errorf("payload = %q, want %q", got, "payload")
	}
}

func TestLinkManagerCreatesAbsoluteSymlink(t *testing.T) {
	t.Parallel()
	manager := NewLinkManager()
	fixture := newLinkFixture(t)
	skipUnlessSymlinkable(t, manager, fixture)

	linkPath := filepath.Join(fixture.root, "absolute")
	if err := manager.CreateSymlink(linkPath, fixture.targetDir, false); err != nil {
		t.Fatalf("CreateSymlink: %v", err)
	}
	target, err := manager.ReadLink(linkPath)
	if err != nil {
		t.Fatalf("ReadLink: %v", err)
	}
	// ReadLinkは保存値をそのまま返す。絶対化・正規化して返すと、§5.1の
	// 「absolute targetを拒否する」検査を呼出し側が行えない。
	if target != fixture.targetDir {
		t.Errorf("ReadLink = %q, want %q", target, fixture.targetDir)
	}
}

func TestLinkManagerRemoveLinkKeepsTarget(t *testing.T) {
	t.Parallel()
	manager := NewLinkManager()
	fixture := newLinkFixture(t)
	skipUnlessSymlinkable(t, manager, fixture)

	linkPath := filepath.Join(fixture.root, "current")
	if err := manager.CreateSymlink(linkPath, fixture.targetDir, true); err != nil {
		t.Fatalf("CreateSymlink: %v", err)
	}
	if err := manager.RemoveLink(linkPath); err != nil {
		t.Fatalf("RemoveLink: %v", err)
	}
	if _, err := os.Lstat(linkPath); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("linkが残っている: err=%v", err)
	}
	// **link先の実体を消してはならない**（port.LinkManagerのdoc comment）。
	// currentの張り替えでtool本体を消す事故がここで止まる。
	if _, err := os.Stat(fixture.targetDir); err != nil {
		t.Errorf("target directoryが消えている: %v", err)
	}
	if _, err := os.Stat(fixture.payload); err != nil {
		t.Errorf("target内のfileが消えている: %v", err)
	}
}

func TestLinkManagerRefusesToRemoveNonLink(t *testing.T) {
	t.Parallel()
	manager := NewLinkManager()
	fixture := newLinkFixture(t)

	cases := map[string]string{
		"directory":    fixture.targetDir,
		"regular file": fixture.targetFile,
	}
	for name, path := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			err := manager.RemoveLink(path)
			if !errors.Is(err, ErrNotLink) {
				t.Fatalf("RemoveLink error = %v, want ErrNotLink", err)
			}
			// 拒否したのだから実体は残っていなければならない。
			if _, statErr := os.Stat(path); statErr != nil {
				t.Errorf("拒否したのに実体が消えている: %v", statErr)
			}
		})
	}
}

func TestLinkManagerHardlink(t *testing.T) {
	t.Parallel()
	manager := NewLinkManager()
	fixture := newLinkFixture(t)
	caps, err := manager.Capabilities(fixture.root)
	if err != nil {
		t.Fatalf("Capabilities: %v", err)
	}
	if !caps.Hardlink {
		t.Skip("この環境ではhardlinkを作成できない")
	}

	linkPath := filepath.Join(fixture.root, "shim.bin")
	if err := manager.CreateHardlink(linkPath, fixture.targetFile); err != nil {
		t.Fatalf("CreateHardlink: %v", err)
	}
	kind, err := manager.Kind(linkPath)
	if err != nil {
		t.Fatalf("Kind: %v", err)
	}
	// docs/09-platform.md §3.3の公開command shimがこの形である。setupの冪等性は
	// 「既にhardlinkである」と分かることに依存する。
	if kind != port.LinkHardlink {
		t.Errorf("Kind = %q, want %q", kind, port.LinkHardlink)
	}
	// 元の名前も同じ実体を指すためhardlinkと判定される。
	originKind, err := manager.Kind(fixture.targetFile)
	if err != nil {
		t.Fatalf("Kind(origin): %v", err)
	}
	if originKind != port.LinkHardlink {
		t.Errorf("Kind(origin) = %q, want %q", originKind, port.LinkHardlink)
	}

	if err := manager.RemoveLink(linkPath); err != nil {
		t.Fatalf("RemoveLink: %v", err)
	}
	content, err := os.ReadFile(fixture.targetFile)
	if err != nil {
		t.Fatalf("hardlinkを外したら元のfileも消えた: %v", err)
	}
	if string(content) != "regular" {
		t.Errorf("content = %q, want %q", content, "regular")
	}
	// 名前が1件に戻ったのでhardlinkではなくなる。
	afterKind, err := manager.Kind(fixture.targetFile)
	if err != nil {
		t.Fatalf("Kind(after): %v", err)
	}
	if afterKind != port.LinkNone {
		t.Errorf("Kind(after) = %q, want %q", afterKind, port.LinkNone)
	}
}

func TestLinkManagerRejectsHardlinkToNonRegularFile(t *testing.T) {
	t.Parallel()
	manager := NewLinkManager()
	fixture := newLinkFixture(t)

	// docs/09-platform.md §3.3はshimを「clientへのhardlink」と定める。link先が
	// さらにlinkだと、shimが起動するexecutableがそのlinkの状態で変わる。
	err := manager.CreateHardlink(filepath.Join(fixture.root, "to-dir"), fixture.targetDir)
	if err == nil {
		t.Fatal("directoryへのhardlinkが成功した")
	}
	if !strings.Contains(err.Error(), "regular file") {
		t.Errorf("error = %v, regular fileでないことを述べるべき", err)
	}

	caps, capErr := manager.Capabilities(fixture.root)
	if capErr != nil {
		t.Fatalf("Capabilities: %v", capErr)
	}
	if !caps.Symlink {
		return
	}
	indirect := filepath.Join(fixture.root, "indirect")
	if symErr := manager.CreateSymlink(indirect, fixture.targetFile, true); symErr != nil {
		t.Fatalf("CreateSymlink: %v", symErr)
	}
	if err := manager.CreateHardlink(filepath.Join(fixture.root, "to-symlink"), indirect); err == nil {
		t.Error("symlinkへのhardlinkが成功した")
	}
}

func TestLinkManagerRejectsUnsafePaths(t *testing.T) {
	t.Parallel()
	manager := NewLinkManager()
	fixture := newLinkFixture(t)

	relative := filepath.Join("relative", "path")
	// filepath.Joinは畳んでしまうため、separatorを直接繋いで`..`を残す。
	separator := string(filepath.Separator)
	notCanonical := fixture.root + separator + "a" + separator + ".." + separator + "b"

	// absoluteでないpathは暗黙のcurrent directoryに依存する。正規形でないpathは
	// 途中componentがlinkのとき畳んだ結果が実体と食い違う。どちらも拒否する。
	cases := []struct {
		name string
		call func() error
		want error
	}{
		{"Capabilities/相対", func() error { _, err := manager.Capabilities(relative); return err }, ErrPathNotAbsolute},
		{"Capabilities/非正規", func() error { _, err := manager.Capabilities(notCanonical); return err }, ErrPathNotCanonical},
		{"CreateSymlink/link相対", func() error { return manager.CreateSymlink(relative, fixture.targetDir, true) }, ErrPathNotAbsolute},
		{"CreateSymlink/target相対", func() error {
			return manager.CreateSymlink(filepath.Join(fixture.root, "l"), "target", true)
		}, ErrPathNotAbsolute},
		{"CreateSymlink/target非正規", func() error {
			return manager.CreateSymlink(filepath.Join(fixture.root, "l"), notCanonical, true)
		}, ErrPathNotCanonical},
		{"CreateJunction/link相対", func() error { return manager.CreateJunction(relative, fixture.targetDir) }, ErrPathNotAbsolute},
		{"CreateJunction/target相対", func() error {
			return manager.CreateJunction(filepath.Join(fixture.root, "j"), "target")
		}, ErrPathNotAbsolute},
		{"CreateHardlink/link相対", func() error { return manager.CreateHardlink(relative, fixture.targetFile) }, ErrPathNotAbsolute},
		{"CreateHardlink/target相対", func() error {
			return manager.CreateHardlink(filepath.Join(fixture.root, "h"), "regular.txt")
		}, ErrPathNotAbsolute},
		{"Kind/相対", func() error { _, err := manager.Kind(relative); return err }, ErrPathNotAbsolute},
		{"ReadLink/相対", func() error { _, err := manager.ReadLink(relative); return err }, ErrPathNotAbsolute},
		{"RemoveLink/相対", func() error { return manager.RemoveLink(relative) }, ErrPathNotAbsolute},
		{"RemoveLink/非正規", func() error { return manager.RemoveLink(notCanonical) }, ErrPathNotCanonical},
		{"Kind/空", func() error { _, err := manager.Kind(""); return err }, ErrPathNotAbsolute},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if err := tc.call(); !errors.Is(err, tc.want) {
				t.Errorf("error = %v, want %v", err, tc.want)
			}
		})
	}
	// 拒否したcallがfilesystemへ何も残していないこと。検査だけで作用が起きるなら
	// 「拒否」になっていない。
	entries, err := os.ReadDir(fixture.root)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 2 {
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		t.Errorf("拒否したのにentryが増えている: %v", names)
	}
}

func TestLinkManagerKindAndReadLinkOnNonLink(t *testing.T) {
	t.Parallel()
	manager := NewLinkManager()
	fixture := newLinkFixture(t)

	t.Run("存在しないpath", func(t *testing.T) {
		t.Parallel()
		missing := filepath.Join(fixture.root, "missing")
		if _, err := manager.Kind(missing); err == nil {
			t.Error("Kindが存在しないpathで成功した")
		}
		if _, err := manager.ReadLink(missing); err == nil {
			t.Error("ReadLinkが存在しないpathで成功した")
		}
	})
	t.Run("通常directory", func(t *testing.T) {
		t.Parallel()
		kind, err := manager.Kind(fixture.targetDir)
		if err != nil {
			t.Fatalf("Kind: %v", err)
		}
		if kind != port.LinkNone {
			t.Errorf("Kind = %q, want %q", kind, port.LinkNone)
		}
		if _, err := manager.ReadLink(fixture.targetDir); !errors.Is(err, ErrNotLink) {
			t.Errorf("ReadLink error = %v, want ErrNotLink", err)
		}
	})
	t.Run("通常file", func(t *testing.T) {
		t.Parallel()
		kind, err := manager.Kind(fixture.targetFile)
		if err != nil {
			t.Fatalf("Kind: %v", err)
		}
		if kind != port.LinkNone {
			t.Errorf("Kind = %q, want %q", kind, port.LinkNone)
		}
		if _, err := manager.ReadLink(fixture.targetFile); !errors.Is(err, ErrNotLink) {
			t.Errorf("ReadLink error = %v, want ErrNotLink", err)
		}
	})
}

func TestLinkManagerCapabilitiesLeavesNothingBehind(t *testing.T) {
	t.Parallel()
	manager := NewLinkManager()
	fixture := newLinkFixture(t)

	before, err := os.ReadDir(fixture.root)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	caps, err := manager.Capabilities(fixture.root)
	if err != nil {
		t.Fatalf("Capabilities: %v", err)
	}
	after, err := os.ReadDir(fixture.root)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	// probeは作った実体をすべて消す。残すと、次回のprobeが既存entryで失敗し
	// 「能力が無い」と誤判定する。
	if len(before) != len(after) {
		names := make([]string, 0, len(after))
		for _, entry := range after {
			names = append(names, entry.Name())
		}
		t.Errorf("probeの残骸がある: %v", names)
	}
	// probeはtarget側を消してはならない。
	if _, err := os.Stat(fixture.payload); err != nil {
		t.Errorf("probeがfixtureのpayloadを消した: %v", err)
	}

	// junctionはWindows専用である（docs/09-platform.md §5.1）。
	if runtime.GOOS != "windows" && caps.Junction {
		t.Error("Linuxでjunctionが作成可能と報告された")
	}
	if runtime.GOOS != "windows" && !caps.Symlink {
		t.Error("Linuxでsymlinkが作成不可と報告された")
	}
	if !caps.Hardlink {
		t.Error("hardlinkが作成不可と報告された")
	}
	// 2回目も同じ結果になる。1回目の残骸があると変わる。
	again, err := manager.Capabilities(fixture.root)
	if err != nil {
		t.Fatalf("Capabilities(2回目): %v", err)
	}
	if again != caps {
		t.Errorf("2回目 = %+v, 1回目 = %+v", again, caps)
	}
}

// TestLinkManagerReplaceSequence はdocs/09-platform.md §3.2・§5.1のcurrent置換列を
// 両OSで確かめる。
//
// portへ`Replace`を足さず、既存操作の列で表す。仕様が定めるのは
// 「temporary linkを作る→targetを再読する→旧linkだけをunlinkする→rename」で
// あり、renameは[port.FileSystem]の操作である。ここではその列が成立すること
// （とくに**旧targetの中身が残ること**）を固定する。
func TestLinkManagerReplaceSequence(t *testing.T) {
	t.Parallel()
	manager := NewLinkManager()
	fixture := newLinkFixture(t)
	skipUnlessSymlinkable(t, manager, fixture)

	next := filepath.Join(fixture.root, "next")
	if err := os.Mkdir(next, 0o700); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	nextPayload := filepath.Join(next, "payload.txt")
	if err := os.WriteFile(nextPayload, []byte("next"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	current := filepath.Join(fixture.root, "current")
	if err := manager.CreateSymlink(current, fixture.targetDir, true); err != nil {
		t.Fatalf("CreateSymlink: %v", err)
	}

	// 1. temporary linkを作る。
	temp := filepath.Join(fixture.root, "current.tmp")
	if err := manager.CreateSymlink(temp, next, true); err != nil {
		t.Fatalf("CreateSymlink(temp): %v", err)
	}
	// 2. targetを再読する。作った直後でも確かめる —— 作成と検査の間に別processが
	//    差し替えている場合があり、検査していないlinkをcommitしない。
	target, err := manager.ReadLink(temp)
	if err != nil {
		t.Fatalf("ReadLink(temp): %v", err)
	}
	if target != "next" {
		t.Fatalf("temp target = %q, want %q", target, "next")
	}
	// 3. 旧linkだけをunlinkする。
	if err := manager.RemoveLink(current); err != nil {
		t.Fatalf("RemoveLink(current): %v", err)
	}
	// 4. temporary名をcurrentへrenameする。
	if err := os.Rename(temp, current); err != nil {
		t.Fatalf("Rename: %v", err)
	}

	got, err := manager.ReadLink(current)
	if err != nil {
		t.Fatalf("ReadLink(current): %v", err)
	}
	if got != "next" {
		t.Errorf("置換後のtarget = %q, want %q", got, "next")
	}
	content, err := os.ReadFile(filepath.Join(current, "payload.txt"))
	if err != nil {
		t.Fatalf("置換後のlink経由で読めない: %v", err)
	}
	if string(content) != "next" {
		t.Errorf("content = %q, want %q", content, "next")
	}
	// **旧targetは残る。** 置換でtool本体を消さないことがこの列の要点である。
	if _, err := os.Stat(fixture.payload); err != nil {
		t.Errorf("旧targetの中身が消えた: %v", err)
	}
}
