//go:build windows

package platform

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain/port"
)

func TestLinkManagerCreatesJunction(t *testing.T) {
	t.Parallel()
	manager := NewLinkManager()
	fixture := newLinkFixture(t)

	linkPath := filepath.Join(fixture.root, "current")
	if err := manager.CreateJunction(linkPath, fixture.targetDir); err != nil {
		t.Fatalf("CreateJunction: %v", err)
	}

	// docs/09-platform.md §3.2「managed user selectionの`tools/<tool>/current`は
	// directory junctionとする」。symlinkになっていると標準userで作れない環境が
	// あり、§3.3のfallback判断も狂う。
	kind, err := manager.Kind(linkPath)
	if err != nil {
		t.Fatalf("Kind: %v", err)
	}
	if kind != port.LinkJunction {
		t.Errorf("Kind = %q, want %q", kind, port.LinkJunction)
	}

	target, err := manager.ReadLink(linkPath)
	if err != nil {
		t.Fatalf("ReadLink: %v", err)
	}
	if target != fixture.targetDir {
		t.Errorf("ReadLink = %q, want %q", target, fixture.targetDir)
	}
	// 実際に辿れること。reparse bufferのoffsetを取り違えていると読めない。
	content, err := os.ReadFile(filepath.Join(linkPath, "payload.txt"))
	if err != nil {
		t.Fatalf("junction経由でpayloadを読めない: %v", err)
	}
	if string(content) != "payload" {
		t.Errorf("content = %q, want %q", content, "payload")
	}
}

func TestLinkManagerRemoveJunctionKeepsTarget(t *testing.T) {
	t.Parallel()
	manager := NewLinkManager()
	fixture := newLinkFixture(t)

	linkPath := filepath.Join(fixture.root, "current")
	if err := manager.CreateJunction(linkPath, fixture.targetDir); err != nil {
		t.Fatalf("CreateJunction: %v", err)
	}
	if err := manager.RemoveLink(linkPath); err != nil {
		t.Fatalf("RemoveLink: %v", err)
	}
	if _, err := os.Lstat(linkPath); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("junctionが残っている: err=%v", err)
	}
	// **docs/09-platform.md §3.2「junction targetを再帰削除しない」。**
	// junctionはdirectory属性を持つため、RemoveAll系の削除は中へ入って
	// tool本体を消す。
	if _, err := os.Stat(fixture.targetDir); err != nil {
		t.Errorf("target directoryが消えている: %v", err)
	}
	if _, err := os.Stat(fixture.payload); err != nil {
		t.Errorf("target内のfileが消えている: %v", err)
	}
}

func TestLinkManagerReplacesJunction(t *testing.T) {
	t.Parallel()
	manager := NewLinkManager()
	fixture := newLinkFixture(t)

	next := filepath.Join(fixture.root, "next")
	if err := os.Mkdir(next, 0o700); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(next, "payload.txt"), []byte("next"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	current := filepath.Join(fixture.root, "current")
	if err := manager.CreateJunction(current, fixture.targetDir); err != nil {
		t.Fatalf("CreateJunction: %v", err)
	}

	// docs/09-platform.md §3.2の置換列。
	temp := filepath.Join(fixture.root, "current.tmp")
	if err := manager.CreateJunction(temp, next); err != nil {
		t.Fatalf("CreateJunction(temp): %v", err)
	}
	target, err := manager.ReadLink(temp)
	if err != nil {
		t.Fatalf("ReadLink(temp): %v", err)
	}
	if target != next {
		t.Fatalf("temp target = %q, want %q", target, next)
	}
	if err := manager.RemoveLink(current); err != nil {
		t.Fatalf("RemoveLink(current): %v", err)
	}
	if err := os.Rename(temp, current); err != nil {
		t.Fatalf("Rename: %v", err)
	}

	got, err := manager.ReadLink(current)
	if err != nil {
		t.Fatalf("ReadLink(current): %v", err)
	}
	if got != next {
		t.Errorf("置換後のtarget = %q, want %q", got, next)
	}
	content, err := os.ReadFile(filepath.Join(current, "payload.txt"))
	if err != nil {
		t.Fatalf("置換後のjunction経由で読めない: %v", err)
	}
	if string(content) != "next" {
		t.Errorf("content = %q, want %q", content, "next")
	}
	if _, err := os.Stat(fixture.payload); err != nil {
		t.Errorf("旧targetの中身が消えた: %v", err)
	}
}

func TestLinkManagerRejectsJunctionToFile(t *testing.T) {
	t.Parallel()
	manager := NewLinkManager()
	fixture := newLinkFixture(t)

	linkPath := filepath.Join(fixture.root, "to-file")
	// junctionはdirectoryにしか張れない。fileを渡された場合、reparse pointを
	// 設定する段で失敗する前に空のdirectoryを作ってしまわないことを確かめる。
	if err := manager.CreateJunction(linkPath, fixture.targetFile); err == nil {
		t.Fatal("fileへのjunctionが成功した")
	}
	if _, err := os.Lstat(linkPath); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("失敗したのに空のdirectoryが残っている: %v", err)
	}
}

func TestLinkManagerJunctionExistsRejected(t *testing.T) {
	t.Parallel()
	manager := NewLinkManager()
	fixture := newLinkFixture(t)

	linkPath := filepath.Join(fixture.root, "current")
	if err := manager.CreateJunction(linkPath, fixture.targetDir); err != nil {
		t.Fatalf("CreateJunction: %v", err)
	}
	// 既存entryへ上書きしない。docs/09-platform.md §3.2の置換はtemporary名を
	// 経由するのであり、その場での上書きは行わない。
	if err := manager.CreateJunction(linkPath, fixture.targetDir); err == nil {
		t.Fatal("既存pathへのCreateJunctionが成功した")
	}
	// 失敗しても既存のjunctionは壊れていない。
	kind, err := manager.Kind(linkPath)
	if err != nil {
		t.Fatalf("Kind: %v", err)
	}
	if kind != port.LinkJunction {
		t.Errorf("Kind = %q, want %q", kind, port.LinkJunction)
	}
}
