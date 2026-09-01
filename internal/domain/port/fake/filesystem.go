package fake

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain/port"
)

// FileSystem操作名。Injectorへ渡す識別子である。
const (
	OpStat        = "fs.Stat"
	OpOpen        = "fs.Open"
	OpOpenAt      = "fs.OpenAt"
	OpReadFile    = "fs.ReadFile"
	OpAtomicWrite = "fs.AtomicWrite"
	OpWriteStream = "fs.WriteStream"
	OpMkdirAll    = "fs.MkdirAll"
	OpRename      = "fs.Rename"
	OpRemove      = "fs.Remove"
	OpRemoveAll   = "fs.RemoveAll"
	OpHarden      = "fs.HardenReadExecute"
	OpWalk        = "fs.Walk"
	OpChmod       = "fs.Chmod"
	OpRealPath    = "fs.RealPath"
)

// ErrNotExist はfileが存在しないことを表す。
var ErrNotExist = fs.ErrNotExist

// ErrDiskFull はdisk full failure injectionで使う。
// docs/11-quality-and-ci.md §8 のscenario 10が要求する。
var ErrDiskFull = errors.New("fake: no space left on device")

type fakeEntry struct {
	data       []byte
	mode       fs.FileMode
	modTime    time.Time
	isDir      bool
	linkKind   port.LinkKind
	linkTarget string
}

// FileSystem はメモリ上の決定的なport.FileSystemである。
//
// pathはslash区切りで正規化して保持する。実OSのseparatorを持ち込むと、
// 同じtestが両OSで別のkeyを作ってしまい決定性が崩れるためである。
// 呼出側にはOS依存の見た目を要求せず、`/`区切りで書けるようにする。
type FileSystem struct {
	mu       sync.Mutex
	entries  map[string]*fakeEntry
	injector *Injector
	// Writes はAtomicWriteが成功したpathを順に記録する。
	// 書込み封じ込め検査（docs/11-quality-and-ci.md §7.2）で使う。
	Writes []string
	// hardened はHardenReadExecuteが成功したentryを順に記録する。
	hardened []HardenRecord
}

var _ port.FileSystem = (*FileSystem)(nil)

// NewFileSystem は空のFileSystemを作る。injectorがnilなら失敗注入なしで動く。
func NewFileSystem(injector *Injector) *FileSystem {
	if injector == nil {
		injector = NewInjector()
	}
	fsys := &FileSystem{
		entries:  make(map[string]*fakeEntry),
		injector: injector,
	}
	fsys.entries["/"] = &fakeEntry{mode: fs.ModeDir | 0o755, isDir: true}
	return fsys
}

// Injector は失敗注入器を返す。
func (f *FileSystem) Injector() *Injector { return f.injector }

// clean はpathを`/`始まりのslash区切りへ正規化する。
func clean(p string) string {
	p = strings.ReplaceAll(p, "\\", "/")
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return path.Clean(p)
}

// AddDir はdirectoryを用意する。testのfixture構築に使う。
func (f *FileSystem) AddDir(p string, perm fs.FileMode) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.mkdirAllLocked(clean(p), perm)
}

// AddFile はfileを用意する。親directoryも作る。
func (f *FileSystem) AddFile(p string, data []byte, perm fs.FileMode) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := clean(p)
	f.mkdirAllLocked(path.Dir(cp), 0o755)
	f.entries[cp] = &fakeEntry{data: append([]byte(nil), data...), mode: perm}
}

// AddLink はlink entryを用意する。LinkManagerのfakeと共有する。
func (f *FileSystem) AddLink(p string, kind port.LinkKind, target string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := clean(p)
	f.mkdirAllLocked(path.Dir(cp), 0o755)
	f.entries[cp] = &fakeEntry{
		mode:       fs.ModeSymlink | 0o777,
		linkKind:   kind,
		linkTarget: target,
		isDir:      kind == port.LinkJunction,
	}
}

// mkdirAllLocked は親を含めてdirectoryを作る。
//
// **既存のentryを置き換えない。** 実OSのMkdirAllはfileやsymlinkがある位置を
// directoryへ作り替えないため、置き換えると「検査した実体と書込み先が違う」
// 状態そのものをfakeが消してしまい、symlink raceのtestが素通りする。
func (f *FileSystem) mkdirAllLocked(p string, perm fs.FileMode) {
	p = clean(p)
	if p == "/" {
		return
	}
	if _, ok := f.entries[p]; ok {
		return
	}
	f.mkdirAllLocked(path.Dir(p), perm)
	f.entries[p] = &fakeEntry{mode: fs.ModeDir | perm, isDir: true}
}

func (f *FileSystem) infoLocked(p string, e *fakeEntry) port.FileInfo {
	return port.FileInfo{
		Name:      path.Base(p),
		Size:      int64(len(e.data)),
		Mode:      e.mode,
		ModTime:   e.modTime,
		IsDir:     e.isDir,
		IsSymlink: e.linkKind != "" && e.linkKind != port.LinkNone,
	}
}

// Stat はfile情報を返す。
func (f *FileSystem) Stat(p string) (port.FileInfo, error) {
	if err := f.injector.Check(OpStat); err != nil {
		return port.FileInfo{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := clean(p)
	e, ok := f.entries[cp]
	if !ok {
		return port.FileInfo{}, ErrNotExist
	}
	return f.infoLocked(cp, e), nil
}

// Open は読取り用に開く。
func (f *FileSystem) Open(p string) (io.ReadCloser, error) {
	if err := f.injector.Check(OpOpen); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	e, ok := f.entries[clean(p)]
	if !ok || e.isDir {
		return nil, ErrNotExist
	}
	return io.NopCloser(bytes.NewReader(append([]byte(nil), e.data...))), nil
}

// OpenAt はrandom access読取り用に開く。
func (f *FileSystem) OpenAt(p string) (port.ReaderAtCloser, error) {
	if err := f.injector.Check(OpOpenAt); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	e, ok := f.entries[clean(p)]
	if !ok || e.isDir {
		return nil, ErrNotExist
	}
	return readerAtCloser{bytes.NewReader(append([]byte(nil), e.data...))}, nil
}

// readerAtCloser はbytes.ReaderへCloseを足す。
//
// 実装がhandleを閉じ忘れてもtestが緑になる状態を避けるため、Closeを持つ型を
// 返してport.ReaderAtCloserを満たす。memory上のfakeでは解放するものが無い。
type readerAtCloser struct{ *bytes.Reader }

func (readerAtCloser) Close() error { return nil }

// ReadFile はfile全体を読む。limitを超える場合はerrorを返す。
func (f *FileSystem) ReadFile(p string, limit int64) ([]byte, error) {
	if err := f.injector.Check(OpReadFile); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	e, ok := f.entries[clean(p)]
	if !ok || e.isDir {
		return nil, ErrNotExist
	}
	if limit > 0 && int64(len(e.data)) > limit {
		return nil, errors.New("fake: file size exceeds limit")
	}
	return append([]byte(nil), e.data...), nil
}

// AtomicWrite は書込みを記録する。失敗注入時はentryを変更しない。
// 中断してもpathが旧内容のまま残ることを再現する。
func (f *FileSystem) AtomicWrite(p string, data []byte, perm fs.FileMode) error {
	if err := f.injector.Check(OpAtomicWrite); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := clean(p)
	if _, ok := f.entries[path.Dir(cp)]; !ok {
		return ErrNotExist
	}
	f.entries[cp] = &fakeEntry{data: append([]byte(nil), data...), mode: perm}
	f.Writes = append(f.Writes, cp)
	return nil
}

// WriteStream はsrcを読み切ってpathへ書く。
//
// 失敗・cancel時は書きかけのentryを削除してから返す。途中まで書けたfileを
// 次回の再開に使わないことを再現する（docs/15-deferred.md D-24）。
//
// 読取り途中で失敗するreaderを渡せば、partial破棄の経路をtestできる。
func (f *FileSystem) WriteStream(p string, perm fs.FileMode, src io.Reader) (int64, error) {
	if err := f.injector.Check(OpWriteStream); err != nil {
		return 0, err
	}
	cp := clean(p)

	f.mu.Lock()
	if _, ok := f.entries[path.Dir(cp)]; !ok {
		f.mu.Unlock()
		return 0, ErrNotExist
	}
	// 書きかけを観測できるよう、読み切る前にentryを作る。実装が途中経過を
	// 残すかどうかではなく、失敗時に消えることをtestできるようにするため。
	f.entries[cp] = &fakeEntry{mode: perm}
	f.mu.Unlock()

	data, err := io.ReadAll(src)
	f.mu.Lock()
	defer f.mu.Unlock()
	if err != nil {
		delete(f.entries, cp)
		return int64(len(data)), err
	}
	f.entries[cp] = &fakeEntry{data: append([]byte(nil), data...), mode: perm}
	f.Writes = append(f.Writes, cp)
	return int64(len(data)), nil
}

// MkdirAll は親を含めてdirectoryを作る。
func (f *FileSystem) MkdirAll(p string, perm fs.FileMode) error {
	if err := f.injector.Check(OpMkdirAll); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.mkdirAllLocked(clean(p), perm)
	return nil
}

// Rename は移動する。
func (f *FileSystem) Rename(oldPath, newPath string) error {
	if err := f.injector.Check(OpRename); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	from, to := clean(oldPath), clean(newPath)
	e, ok := f.entries[from]
	if !ok {
		return ErrNotExist
	}
	prefix := from + "/"
	for key, child := range f.entries {
		if strings.HasPrefix(key, prefix) {
			f.entries[to+"/"+strings.TrimPrefix(key, prefix)] = child
			delete(f.entries, key)
		}
	}
	f.entries[to] = e
	delete(f.entries, from)
	return nil
}

// Remove はfileまたは空directoryを消す。存在しない場合もerrorにしない。
func (f *FileSystem) Remove(p string) error {
	if err := f.injector.Check(OpRemove); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.entries, clean(p))
	return nil
}

// RemoveAll はtreeを消す。
func (f *FileSystem) RemoveAll(p string) error {
	if err := f.injector.Check(OpRemoveAll); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := clean(p)
	prefix := cp + "/"
	for key := range f.entries {
		if key == cp || strings.HasPrefix(key, prefix) {
			delete(f.entries, key)
		}
	}
	return nil
}

// Walk はrootから再帰的に辿る。link先は辿らずentryとして報告する。
func (f *FileSystem) Walk(root string, fn port.WalkFunc) error {
	if err := f.injector.Check(OpWalk); err != nil {
		return err
	}
	f.mu.Lock()
	cr := clean(root)
	prefix := cr + "/"
	keys := make([]string, 0, len(f.entries))
	for key := range f.entries {
		if key == cr || strings.HasPrefix(key, prefix) {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	infos := make([]port.FileInfo, len(keys))
	for idx, key := range keys {
		infos[idx] = f.infoLocked(key, f.entries[key])
	}
	f.mu.Unlock()

	if len(keys) == 0 {
		return ErrNotExist
	}
	for idx, key := range keys {
		if err := fn(key, infos[idx]); err != nil {
			return err
		}
	}
	return nil
}

// Chmod はpermissionを設定する。
func (f *FileSystem) Chmod(p string, perm fs.FileMode) error {
	if err := f.injector.Check(OpChmod); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	e, ok := f.entries[clean(p)]
	if !ok {
		return ErrNotExist
	}
	e.mode = (e.mode &^ fs.ModePerm) | (perm & fs.ModePerm)
	return nil
}

// HardenReadExecute は通常利用でread/execute onlyへ正規化する。
//
// fakeはLinuxのmode表現で記録する（docs/08-install-runtime.md §7手順5の
// directory 0555／executable 0555／その他0444）。Windowsのwrite ACE除去は
// production adapterの責務であり、fakeで模す意味がない——検査したいのは
// 「どのentryをどの種別で正規化したか」であって、OS固有の実現方法ではない。
func (f *FileSystem) HardenReadExecute(p string, kind port.PermissionKind) error {
	if err := f.injector.Check(OpHarden); err != nil {
		return err
	}
	if !kind.IsValid() {
		return fmt.Errorf("fake: 未知のPermissionKind %d", kind)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	e, ok := f.entries[clean(p)]
	if !ok {
		return ErrNotExist
	}
	perm := fs.FileMode(0o444)
	if kind == port.PermissionDirectory || kind == port.PermissionExecutable {
		perm = 0o555
	}
	e.mode = (e.mode &^ fs.ModePerm) | perm
	f.hardened = append(f.hardened, HardenRecord{Path: clean(p), Kind: kind})
	return nil
}

// Hardened は正規化した順に記録を返す。
//
// 「どのentryをどの種別で正規化したか」を検査するために公開する。
func (f *FileSystem) Hardened() []HardenRecord {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]HardenRecord(nil), f.hardened...)
}

// HardenRecord は1件のpermission正規化である。
type HardenRecord struct {
	Path string
	Kind port.PermissionKind
}

// RealPath はlinkを解決した絶対pathを返す。
// 循環参照は上限回数で打ち切り、無限loopにしない。
func (f *FileSystem) RealPath(p string) (string, error) {
	if err := f.injector.Check(OpRealPath); err != nil {
		return "", err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	current := clean(p)
	for hop := 0; hop < 40; hop++ {
		e, ok := f.entries[current]
		if !ok {
			return "", ErrNotExist
		}
		if e.linkKind == "" || e.linkKind == port.LinkNone {
			return current, nil
		}
		target := e.linkTarget
		if !strings.HasPrefix(target, "/") {
			target = path.Join(path.Dir(current), target)
		}
		current = clean(target)
	}
	return "", errors.New("fake: too many link hops")
}
