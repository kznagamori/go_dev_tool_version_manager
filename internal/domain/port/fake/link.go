package fake

import (
	"errors"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain/port"
)

// LinkManager操作名。
const (
	OpCapabilities   = "link.Capabilities"
	OpCreateJunction = "link.CreateJunction"
	OpCreateSymlink  = "link.CreateSymlink"
	OpCreateHardlink = "link.CreateHardlink"
	OpLinkKind       = "link.Kind"
	OpReadLink       = "link.ReadLink"
	OpRemoveLink     = "link.RemoveLink"
)

// ErrLinkUnsupported は対象filesystemがそのlink種別を作れないことを表す。
// Windows標準ユーザーでsymlinkを作れない状況の再現に使う。
var ErrLinkUnsupported = errors.New("fake: link kind not supported on this filesystem")

// LinkManager は決定的なport.LinkManagerである。
//
// entryはFileSystem fakeと共有する。link作成が実際にfilesystem上のentryとして
// 見えないと、Walkやcontainment検査がlinkを見落とすためである。
type LinkManager struct {
	fsys     *FileSystem
	injector *Injector
	// Caps は各directoryの能力である。keyは正規化済みdir path。
	// 未登録のdirにはDefaultCapsを使う。
	Caps map[string]port.LinkCapabilities
	// DefaultCaps は未登録directoryの能力である。
	DefaultCaps port.LinkCapabilities
}

var _ port.LinkManager = (*LinkManager)(nil)

// NewLinkManager はFileSystemと同じentryを共有するLinkManagerを作る。
func NewLinkManager(fsys *FileSystem) *LinkManager {
	return &LinkManager{
		fsys:     fsys,
		injector: fsys.Injector(),
		Caps:     make(map[string]port.LinkCapabilities),
		DefaultCaps: port.LinkCapabilities{
			Junction: true,
			Symlink:  true,
			Hardlink: true,
		},
	}
}

// Capabilities はdir上で作成可能なlink種別を返す。
func (l *LinkManager) Capabilities(dir string) (port.LinkCapabilities, error) {
	if err := l.injector.Check(OpCapabilities); err != nil {
		return port.LinkCapabilities{}, err
	}
	if caps, ok := l.Caps[clean(dir)]; ok {
		return caps, nil
	}
	return l.DefaultCaps, nil
}

func (l *LinkManager) create(op string, kind port.LinkKind, linkPath, target string) error {
	if err := l.injector.Check(op); err != nil {
		return err
	}
	caps, err := l.Capabilities(dirOf(linkPath))
	if err != nil {
		return err
	}
	switch kind {
	case port.LinkJunction:
		if !caps.Junction {
			return ErrLinkUnsupported
		}
	case port.LinkSymlink:
		if !caps.Symlink {
			return ErrLinkUnsupported
		}
	case port.LinkHardlink:
		if !caps.Hardlink {
			return ErrLinkUnsupported
		}
	}
	l.fsys.AddLink(linkPath, kind, target)
	return nil
}

// CreateJunction はdirectory junctionを作る。
func (l *LinkManager) CreateJunction(linkPath, targetDir string) error {
	return l.create(OpCreateJunction, port.LinkJunction, linkPath, targetDir)
}

// CreateSymlink はsymbolic linkを作る。relativeでもtargetはそのまま保持し、
// 解決はRealPathが行う。
func (l *LinkManager) CreateSymlink(linkPath, target string, relative bool) error {
	_ = relative
	return l.create(OpCreateSymlink, port.LinkSymlink, linkPath, target)
}

// CreateHardlink はhard linkを作る。
func (l *LinkManager) CreateHardlink(linkPath, targetFile string) error {
	return l.create(OpCreateHardlink, port.LinkHardlink, linkPath, targetFile)
}

// Kind はpath自体のlink種別を返す。
func (l *LinkManager) Kind(p string) (port.LinkKind, error) {
	if err := l.injector.Check(OpLinkKind); err != nil {
		return port.LinkNone, err
	}
	l.fsys.mu.Lock()
	defer l.fsys.mu.Unlock()
	e, ok := l.fsys.entries[clean(p)]
	if !ok {
		return port.LinkNone, ErrNotExist
	}
	if e.linkKind == "" {
		return port.LinkNone, nil
	}
	return e.linkKind, nil
}

// ReadLink はtargetを解決せずに返す。
func (l *LinkManager) ReadLink(p string) (string, error) {
	if err := l.injector.Check(OpReadLink); err != nil {
		return "", err
	}
	l.fsys.mu.Lock()
	defer l.fsys.mu.Unlock()
	e, ok := l.fsys.entries[clean(p)]
	if !ok {
		return "", ErrNotExist
	}
	if e.linkKind == "" || e.linkKind == port.LinkNone {
		return "", errors.New("fake: not a link")
	}
	return e.linkTarget, nil
}

// RemoveLink はlink entryだけを消す。link先の実体は消さない。
func (l *LinkManager) RemoveLink(p string) error {
	if err := l.injector.Check(OpRemoveLink); err != nil {
		return err
	}
	l.fsys.mu.Lock()
	defer l.fsys.mu.Unlock()
	cp := clean(p)
	e, ok := l.fsys.entries[cp]
	if !ok {
		return ErrNotExist
	}
	if e.linkKind == "" || e.linkKind == port.LinkNone {
		return errors.New("fake: not a link")
	}
	delete(l.fsys.entries, cp)
	return nil
}

func dirOf(p string) string {
	cp := clean(p)
	for i := len(cp) - 1; i > 0; i-- {
		if cp[i] == '/' {
			return cp[:i]
		}
	}
	return "/"
}
