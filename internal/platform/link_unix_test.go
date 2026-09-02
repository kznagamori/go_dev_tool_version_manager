//go:build !windows

package platform

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain/port"
)

func TestLinkManagerRejectsJunctionOnLinux(t *testing.T) {
	t.Parallel()
	manager := NewLinkManager()
	fixture := newLinkFixture(t)

	linkPath := filepath.Join(fixture.root, "current")
	err := manager.CreateJunction(linkPath, fixture.targetDir)
	// **symlinkで代替しない。** 呼出し側がjunctionを要求したのは、そのplatformの
	// 規則がjunctionだからである（docs/09-platform.md §3.2）。黙って別種のlinkを
	// 作ると、Windows前提の検査がLinuxで別の実体を見ることになる。
	if !errors.Is(err, ErrLinkUnsupported) {
		t.Fatalf("error = %v, want ErrLinkUnsupported", err)
	}
	if _, statErr := os.Lstat(linkPath); !errors.Is(statErr, os.ErrNotExist) {
		t.Errorf("拒否したのにentryができている: %v", statErr)
	}
}

func TestLinkManagerReportsSymlinkToOutsideTarget(t *testing.T) {
	t.Parallel()
	manager := NewLinkManager()
	fixture := newLinkFixture(t)

	outside, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}
	linkPath := filepath.Join(fixture.root, "escape")
	if err := manager.CreateSymlink(linkPath, outside, true); err != nil {
		t.Fatalf("CreateSymlink: %v", err)
	}

	// adapterはdata rootを知らないため、root外targetそのものは拒否しない
	// （docs/02-architecture.md §1）。拒否できるのはrootを知る呼出し側であり、
	// そのために**保存値をそのまま読み出せる**ことが要る。
	target, err := manager.ReadLink(linkPath)
	if err != nil {
		t.Fatalf("ReadLink: %v", err)
	}
	resolved := filepath.Join(filepath.Dir(linkPath), target)
	if filepath.Clean(resolved) != outside {
		t.Errorf("解決結果 = %q, want %q", filepath.Clean(resolved), outside)
	}
	// root外へ出るtargetは`..`で始まる。呼出し側はこの形を見て拒否できる。
	if filepath.IsAbs(target) {
		t.Errorf("target = %q, relative形で保存されるべき", target)
	}
}

func TestLinkManagerKindIgnoresSymlinkTargetKind(t *testing.T) {
	t.Parallel()
	manager := NewLinkManager()
	fixture := newLinkFixture(t)

	// Kindはpath自体を見る。辿った先を見ると、fileを指すsymlinkと
	// directoryを指すsymlinkで別の種別を返してしまう。
	toFile := filepath.Join(fixture.root, "to-file")
	if err := manager.CreateSymlink(toFile, fixture.targetFile, true); err != nil {
		t.Fatalf("CreateSymlink: %v", err)
	}
	kind, err := manager.Kind(toFile)
	if err != nil {
		t.Fatalf("Kind: %v", err)
	}
	if kind != port.LinkSymlink {
		t.Errorf("Kind = %q, want %q", kind, port.LinkSymlink)
	}

	// 壊れたsymlink（target不在）でもKindは判定できなければならない。判定
	// できないと、`doctor`が壊れたcurrentを報告する手段を失う。
	broken := filepath.Join(fixture.root, "broken")
	if err := manager.CreateSymlink(broken, filepath.Join(fixture.root, "missing"), true); err != nil {
		t.Fatalf("CreateSymlink(broken): %v", err)
	}
	brokenKind, err := manager.Kind(broken)
	if err != nil {
		t.Fatalf("Kind(broken): %v", err)
	}
	if brokenKind != port.LinkSymlink {
		t.Errorf("Kind(broken) = %q, want %q", brokenKind, port.LinkSymlink)
	}
	if err := manager.RemoveLink(broken); err != nil {
		t.Errorf("壊れたsymlinkを外せない: %v", err)
	}
}
