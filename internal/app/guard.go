package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"sync"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain/port"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/security"
)

// Guard は1 operationの外部作用を[Scope]で縛り、記録する。
//
// docs/11-quality-and-ci.md §7.2「E2E実行時はFileSystem、Registry、Process portを
// 記録用wrapperで包み」の記録側と、docs/02-architecture.md §8手順5「Execute中の
// download/extract/probeがPlanの列挙と一致し、全書込みが…の中にあり、任意
// helper/backend processを起動しないこと」の拒否側を、同じ1箇所で担う。
//
// **記録と拒否を分けない。** 記録だけのwrapperにすると、違反はE2E後の照合で
// 初めて分かる。その時点では既に書込みが起きており、検査は事後報告になる。
// 通す前に判定し、通したものだけを記録する。
//
// Registry portは包まない。同portはWindows user PATH integration専用で、
// 実装はP7である（docs/13-progress.md）。包むためだけに中身の無いportを
// 先に作らない。P7で同portを追加するとき、ここへ同じ形のwrapperを足す。
type Guard struct {
	scope *Scope
	// masker はdownload URLの記録へ適用する。
	masker *security.PathMasker

	mu      sync.Mutex
	records Records
}

// NewGuard はGuardを作る。
func NewGuard(scope *Scope, masker *security.PathMasker) (*Guard, error) {
	if scope == nil {
		return nil, errors.New("app: Scopeが無い")
	}
	return &Guard{scope: scope, masker: masker}, nil
}

// Records は記録のcopyを返す。
//
// copyを返すのは、境界を通過した記録を呼出し側から書き換えられないようにする
// ためである（docs/02-architecture.md §4「request/resultは境界通過後に
// immutableとして扱う」）。
func (g *Guard) Records() Records {
	g.mu.Lock()
	defer g.mu.Unlock()
	out := Records{
		Writes:    append([]WriteRecord(nil), g.records.Writes...),
		Downloads: append([]DownloadRecord(nil), g.records.Downloads...),
	}
	for _, record := range g.records.Processes {
		out.Processes = append(out.Processes, ProcessRecord{
			Executable: record.Executable,
			Args:       append([]string(nil), record.Args...),
			Dir:        record.Dir,
			EnvNames:   append([]string(nil), record.EnvNames...),
		})
	}
	return out
}

// FileSystem は書込みを縛って記録するport.FileSystemを返す。
func (g *Guard) FileSystem(inner port.FileSystem) port.FileSystem {
	return &guardedFileSystem{inner: inner, guard: g}
}

// ProcessRunner は起動を縛って記録するport.ProcessRunnerを返す。
func (g *Guard) ProcessRunner(inner port.ProcessRunner) port.ProcessRunner {
	return &guardedProcessRunner{inner: inner, guard: g}
}

// HTTPClient は取得先を縛って記録するport.HTTPClientを返す。
func (g *Guard) HTTPClient(inner port.HTTPClient) port.HTTPClient {
	return &guardedHTTPClient{inner: inner, guard: g}
}

// authorizeWrite は書込み先を判定し、通れば記録する。
//
// 判定は**解決済みpath**で行う。docs/10-security.md §6が「symlink/reparse point
// 経由の逸脱を拒否する」と定めるため、解決前のpathで比べては意味がない。
//
// 対象がまだ存在しない場合は親を解決してから組み立てる。作成する側のpathは
// 定義上まだ解決できないためで、親が管理root内にあることが判定の要になる。
func (g *Guard) authorizeWrite(inner port.FileSystem, action WriteAction, path string) error {
	resolved, err := g.resolveForContainment(inner, path)
	if err != nil {
		return err
	}
	role, allowed := g.scope.AllowsWrite(resolved)
	if !allowed {
		// pathを載せない。typed errorは個人pathを露出させない
		// （docs/04-storage-and-data.md §17.2）。
		return &domain.Error{
			Code:      domain.CodePathUnsafe,
			Retryable: false,
			PathRole:  domain.RoleDataRoot,
			Cause: fmt.Errorf(
				"app: %s先がoperationの許可rootの外にある", action),
		}
	}
	g.mu.Lock()
	g.records.Writes = append(g.records.Writes, WriteRecord{
		Action: action, Role: role, Path: resolved,
	})
	g.mu.Unlock()
	return nil
}

// resolveForContainment はcontainment判定に使うpathを返す。
//
// 対象が存在すればそれ自身を、無ければ最も近い既存の祖先を解決し、残りの
// componentを繋ぎ直す。存在しないpathへRealPathを掛けても解決できないため、
// 「これから作る位置」を判定するにはこの形が要る。
func (g *Guard) resolveForContainment(inner port.FileSystem, path string) (string, error) {
	if resolved, err := inner.RealPath(path); err == nil {
		return resolved, nil
	}
	// 存在しない位置は、最も近い既存の祖先まで遡って解決する。祖先を1段ずつ
	// 辿るのは、`a/b/c`のうち`a`しか無い場合でも判定できるようにするためである。
	separator := security.PathSeparator(g.scope.host)
	components := splitComponents(path, g.scope.host)
	for depth := len(components) - 1; depth >= 1; depth-- {
		ancestor := joinComponents(path, components[:depth], separator)
		resolved, err := inner.RealPath(ancestor)
		if err != nil {
			continue
		}
		for _, remaining := range components[depth:] {
			resolved += separator + remaining
		}
		return resolved, nil
	}
	return "", &domain.Error{
		Code:      domain.CodePathUnsafe,
		Retryable: false,
		PathRole:  domain.RoleDataRoot,
		Cause:     errors.New("app: 書込み先の既存の祖先を解決できない"),
	}
}

// guardedFileSystem は書込みだけを縛るport.FileSystemである。
//
// 読取りは縛らない。docs/11-quality-and-ci.md §7.2が検査対象とするのは
// 「全write/move/delete先」であり、読取りは管理外pathも正当に対象になる
// （project fileの探索、既存installの検査）。
type guardedFileSystem struct {
	inner port.FileSystem
	guard *Guard
}

var _ port.FileSystem = (*guardedFileSystem)(nil)

func (f *guardedFileSystem) Stat(path string) (port.FileInfo, error) { return f.inner.Stat(path) }
func (f *guardedFileSystem) Open(path string) (io.ReadCloser, error) { return f.inner.Open(path) }

func (f *guardedFileSystem) OpenAt(path string) (port.ReaderAtCloser, error) {
	return f.inner.OpenAt(path)
}

func (f *guardedFileSystem) ReadFile(path string, limit int64) ([]byte, error) {
	return f.inner.ReadFile(path, limit)
}

func (f *guardedFileSystem) Walk(root string, fn port.WalkFunc) error {
	return f.inner.Walk(root, fn)
}

func (f *guardedFileSystem) RealPath(path string) (string, error) { return f.inner.RealPath(path) }

func (f *guardedFileSystem) AtomicWrite(path string, data []byte, perm fs.FileMode) error {
	if err := f.guard.authorizeWrite(f.inner, WriteCreate, path); err != nil {
		return err
	}
	return f.inner.AtomicWrite(path, data, perm)
}

func (f *guardedFileSystem) WriteStream(path string, perm fs.FileMode, src io.Reader) (int64, error) {
	if err := f.guard.authorizeWrite(f.inner, WriteCreate, path); err != nil {
		return 0, err
	}
	return f.inner.WriteStream(path, perm, src)
}

func (f *guardedFileSystem) MkdirAll(path string, perm fs.FileMode) error {
	if err := f.guard.authorizeWrite(f.inner, WriteCreate, path); err != nil {
		return err
	}
	return f.inner.MkdirAll(path, perm)
}

// Rename は移動元と移動先の両方を判定する。
//
// 片方だけ見ると、管理root内へ管理外のものを引き込む／管理root外へ持ち出す
// のどちらかが素通りする。
func (f *guardedFileSystem) Rename(oldPath, newPath string) error {
	if err := f.guard.authorizeWrite(f.inner, WriteMove, oldPath); err != nil {
		return err
	}
	if err := f.guard.authorizeWrite(f.inner, WriteMove, newPath); err != nil {
		return err
	}
	return f.inner.Rename(oldPath, newPath)
}

func (f *guardedFileSystem) Remove(path string) error {
	if err := f.guard.authorizeWrite(f.inner, WriteRemove, path); err != nil {
		return err
	}
	return f.inner.Remove(path)
}

func (f *guardedFileSystem) RemoveAll(path string) error {
	if err := f.guard.authorizeWrite(f.inner, WriteRemove, path); err != nil {
		return err
	}
	return f.inner.RemoveAll(path)
}

func (f *guardedFileSystem) Chmod(path string, perm fs.FileMode) error {
	if err := f.guard.authorizeWrite(f.inner, WritePermission, path); err != nil {
		return err
	}
	return f.inner.Chmod(path, perm)
}

// guardedProcessRunner はPlan外のprocess起動を拒否するport.ProcessRunnerである。
type guardedProcessRunner struct {
	inner port.ProcessRunner
	guard *Guard
}

var _ port.ProcessRunner = (*guardedProcessRunner)(nil)

// Run は許可listと完全一致する起動だけを通す。
//
// docs/10-security.md §7「Plan `probes[]`にないexternal executableをExecute中に
// 発見して起動しない」。**発見した時点で拒否する**のであって、起動してから
// 記録を見るのではない。
func (p *guardedProcessRunner) Run(ctx context.Context, spec port.ProcessSpec) (port.ProcessResult, error) {
	if !p.guard.scope.AllowsProcess(spec.Executable, spec.Args, spec.Dir) {
		return port.ProcessResult{}, &domain.Error{
			Code:      domain.CodePathUnsafe,
			Retryable: false,
			PathRole:  domain.RolePayload,
			Cause:     errors.New("app: Planが宣言していないprocessを起動しようとした"),
		}
	}
	p.guard.mu.Lock()
	p.guard.records.Processes = append(p.guard.records.Processes, ProcessRecord{
		Executable: spec.Executable,
		Args:       append([]string(nil), spec.Args...),
		Dir:        spec.Dir,
		EnvNames:   envNames(spec.Env),
	})
	p.guard.mu.Unlock()
	return p.inner.Run(ctx, spec)
}

// guardedHTTPClient はPlan外のdownloadを拒否するport.HTTPClientである。
type guardedHTTPClient struct {
	inner port.HTTPClient
	guard *Guard
}

var _ port.HTTPClient = (*guardedHTTPClient)(nil)

// Get は許可listにあるURLだけを通す。
//
// docs/02-architecture.md §8手順5「Execute中のdownload/extract/probeがPlanの
// 列挙と一致」。Planが表示・承認したartifactだけを取得する。
func (c *guardedHTTPClient) Get(ctx context.Context, req port.HTTPRequest) (*port.HTTPResponse, error) {
	if err := c.guard.authorizeDownload(req.URL); err != nil {
		return nil, err
	}
	return c.inner.Get(ctx, req)
}

// Head も同じ許可listで縛る。
//
// HEADは内容を取らないが、宛先へ到達する点はGETと変わらない。Planに無いhostへ
// 到達できると、そこが存在確認の経路になる。
func (c *guardedHTTPClient) Head(ctx context.Context, req port.HTTPRequest) (*port.HTTPResponse, error) {
	if err := c.guard.authorizeDownload(req.URL); err != nil {
		return nil, err
	}
	return c.inner.Head(ctx, req)
}

// authorizeDownload は取得先を判定し、通ればmaskして記録する。
func (g *Guard) authorizeDownload(raw string) error {
	if !g.scope.AllowsDownload(raw) {
		return &domain.Error{
			Code:      domain.CodePathUnsafe,
			Retryable: false,
			PathRole:  domain.RoleDownloadCache,
			Cause:     errors.New("app: Planが宣言していない取得先へ接続しようとした"),
		}
	}
	masked := security.MaskURL(raw)
	if g.masker != nil {
		masked = g.masker.Mask(masked)
	}
	g.mu.Lock()
	g.records.Downloads = append(g.records.Downloads, DownloadRecord{URL: masked})
	g.mu.Unlock()
	return nil
}
