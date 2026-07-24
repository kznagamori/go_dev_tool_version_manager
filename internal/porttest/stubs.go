package porttest

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"time"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/ports"
)

// ErrNotImplemented は stub port が呼ばれたことを表す。テストは必要な port だけ本物の
// 実装/fake へ差し替え、未使用 port が誤って呼ばれた場合に検出できる。
var ErrNotImplemented = errors.New("porttest: not implemented")

// StubFileSystem は全メソッドが ErrNotImplemented を返す ports.FileSystem である。
type StubFileSystem struct{}

func (StubFileSystem) Stat(string) (ports.FileInfo, error) {
	return ports.FileInfo{}, ErrNotImplemented
}
func (StubFileSystem) Lstat(string) (ports.FileInfo, error) {
	return ports.FileInfo{}, ErrNotImplemented
}
func (StubFileSystem) ReadFile(string) ([]byte, error)    { return nil, ErrNotImplemented }
func (StubFileSystem) Open(string) (io.ReadCloser, error) { return nil, ErrNotImplemented }
func (StubFileSystem) WriteFileAtomic(string, []byte, fs.FileMode) error {
	return ErrNotImplemented
}
func (StubFileSystem) MkdirAll(string, fs.FileMode) error       { return ErrNotImplemented }
func (StubFileSystem) Rename(string, string) error              { return ErrNotImplemented }
func (StubFileSystem) Remove(string) error                      { return ErrNotImplemented }
func (StubFileSystem) RemoveAll(string) error                   { return ErrNotImplemented }
func (StubFileSystem) ReadDir(string) ([]ports.FileInfo, error) { return nil, ErrNotImplemented }
func (StubFileSystem) RealPath(string) (string, error)          { return "", ErrNotImplemented }
func (StubFileSystem) Chmod(string, fs.FileMode) error          { return ErrNotImplemented }

// StubLinkManager は全メソッドが ErrNotImplemented を返す ports.LinkManager である。
type StubLinkManager struct{}

func (StubLinkManager) Capabilities(string) (ports.LinkCapabilities, error) {
	return ports.LinkCapabilities{}, ErrNotImplemented
}
func (StubLinkManager) LinkKindOf(string) (ports.LinkKind, error) {
	return ports.LinkNone, ErrNotImplemented
}
func (StubLinkManager) CreateJunction(string, string) error { return ErrNotImplemented }
func (StubLinkManager) CreateSymlink(string, string) error  { return ErrNotImplemented }
func (StubLinkManager) CreateHardlink(string, string) error { return ErrNotImplemented }
func (StubLinkManager) ReadTarget(string) (string, error)   { return "", ErrNotImplemented }
func (StubLinkManager) RemoveLink(string) error             { return ErrNotImplemented }

// StubHTTPClient は ErrNotImplemented を返す ports.HTTPClient である。
type StubHTTPClient struct{}

func (StubHTTPClient) Do(context.Context, ports.HTTPRequest) (*ports.HTTPResponse, error) {
	return nil, ErrNotImplemented
}

// StubProcessRunner は ErrNotImplemented を返す ports.ProcessRunner である。
type StubProcessRunner struct{}

func (StubProcessRunner) Run(context.Context, ports.ProcessSpec) (*ports.ProcessResult, error) {
	return nil, ErrNotImplemented
}

// StubArchiveExtractor は全メソッドが ErrNotImplemented を返す ports.ArchiveExtractor である。
type StubArchiveExtractor struct{}

func (StubArchiveExtractor) DetectFormat(string) (ports.ArchiveFormat, error) {
	return "", ErrNotImplemented
}
func (StubArchiveExtractor) List(context.Context, string, ports.ArchiveFormat) ([]ports.ArchiveEntry, error) {
	return nil, ErrNotImplemented
}
func (StubArchiveExtractor) Extract(context.Context, ports.ExtractRequest) error {
	return ErrNotImplemented
}

// StubHashCalculator は ErrNotImplemented を返す ports.HashCalculator である。
type StubHashCalculator struct{}

func (StubHashCalculator) SumSHA256Stream(io.Reader) (domain.Digest, error) {
	return domain.Digest{}, ErrNotImplemented
}
func (StubHashCalculator) SumSHA256File(string) (domain.Digest, error) {
	return domain.Digest{}, ErrNotImplemented
}

// StubLockManager は ErrNotImplemented を返す ports.LockManager である。
type StubLockManager struct{}

func (StubLockManager) Acquire(context.Context, string, bool, time.Duration) (ports.Lock, error) {
	return nil, ErrNotImplemented
}
func (StubLockManager) Inspect(string) (ports.LockInfo, bool, error) {
	return ports.LockInfo{}, false, ErrNotImplemented
}

// StubReleaseIntegrityVerifier は ErrNotImplemented を返す ports.ReleaseIntegrityVerifier である。
type StubReleaseIntegrityVerifier struct{}

func (StubReleaseIntegrityVerifier) VerifyArchiveDigest(string, domain.Digest) error {
	return ErrNotImplemented
}
func (StubReleaseIntegrityVerifier) VerifyRegistryTreeHash(string, domain.Digest) error {
	return ErrNotImplemented
}

// StubSignatureVerifier は ErrNotImplemented を返す ports.SignatureVerifier である。
type StubSignatureVerifier struct{}

func (StubSignatureVerifier) VerifyPGPDetached(string, string, string, string) error {
	return ErrNotImplemented
}
func (StubSignatureVerifier) VerifyMinisign(string, string, string) error {
	return ErrNotImplemented
}
