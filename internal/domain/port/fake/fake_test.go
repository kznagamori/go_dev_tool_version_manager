package fake

import (
	"context"
	"errors"
	"io"
	"reflect"
	"testing"
	"time"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain/port"
)

func TestSetSatisfiesPorts(t *testing.T) {
	ports := NewSet().Ports()
	if ports.Clock == nil || ports.FileSystem == nil || ports.HTTPClient == nil ||
		ports.LinkManager == nil || ports.ProcessRunner == nil || ports.UserLookup == nil {
		t.Fatalf("Ports に未設定のfieldがある: %+v", ports)
	}
}

// --- Clock ---

func TestClockAdvanceMovesBothClocks(t *testing.T) {
	c := NewClock(DefaultNow())
	start := c.Monotonic()

	c.Advance(90 * time.Second)

	if got := c.Now(); !got.Equal(DefaultNow().Add(90 * time.Second)) {
		t.Fatalf("Now = %v", got)
	}
	if got := c.Since(start); got != 90*time.Second {
		t.Fatalf("Since = %v, want 90s", got)
	}
}

// wall clockが巻き戻っても単調時間は進み続けることを確認する。
// timeout判定がwall clockに依存していると負の経過時間になる。
func TestClockRewindDoesNotAffectMonotonic(t *testing.T) {
	c := NewClock(DefaultNow())
	start := c.Monotonic()
	c.AdvanceMonotonic(30 * time.Second)

	c.SetNow(DefaultNow().Add(-time.Hour))

	if got := c.Since(start); got != 30*time.Second {
		t.Fatalf("巻き戻し後のSince = %v, want 30s", got)
	}
	if got := c.Now(); !got.Before(DefaultNow()) {
		t.Fatalf("Now = %v, want DefaultNowより前", got)
	}
}

// --- FileSystem ---

func TestFileSystemAtomicWriteAndRead(t *testing.T) {
	fsys := NewFileSystem(nil)
	fsys.AddDir("/data", 0o755)

	if err := fsys.AtomicWrite("/data/a.txt", []byte("hello"), 0o644); err != nil {
		t.Fatalf("AtomicWrite: %v", err)
	}
	got, err := fsys.ReadFile("/data/a.txt", 1024)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != "hello" {
		t.Fatalf("ReadFile = %q, want %q", got, "hello")
	}
	if !reflect.DeepEqual(fsys.Writes, []string{"/data/a.txt"}) {
		t.Fatalf("Writes = %v", fsys.Writes)
	}
}

// 失敗注入時にfileが旧内容のまま残ることを確認する。
// AtomicWriteの契約は「中断しても部分書込みを観測させない」である。
func TestFileSystemAtomicWriteFailureKeepsOldContent(t *testing.T) {
	fsys := NewFileSystem(nil)
	fsys.AddFile("/data/a.txt", []byte("old"), 0o644)
	fsys.Injector().FailOnce(OpAtomicWrite, ErrDiskFull)

	err := fsys.AtomicWrite("/data/a.txt", []byte("new"), 0o644)
	if !errors.Is(err, ErrDiskFull) {
		t.Fatalf("AtomicWrite = %v, want ErrDiskFull", err)
	}
	got, err := fsys.ReadFile("/data/a.txt", 1024)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != "old" {
		t.Fatalf("失敗後の内容 = %q, want %q", got, "old")
	}
	if len(fsys.Writes) != 0 {
		t.Fatalf("失敗した書込みがWritesへ記録された: %v", fsys.Writes)
	}
}

func TestFileSystemReadFileRejectsOverLimit(t *testing.T) {
	fsys := NewFileSystem(nil)
	fsys.AddFile("/big", []byte("0123456789"), 0o644)

	if _, err := fsys.ReadFile("/big", 4); err == nil {
		t.Fatal("上限超過でerrorにならなかった")
	}
	if _, err := fsys.ReadFile("/big", 10); err != nil {
		t.Fatalf("上限ちょうどで失敗した: %v", err)
	}
}

func TestFileSystemStatMissing(t *testing.T) {
	fsys := NewFileSystem(nil)
	if _, err := fsys.Stat("/nope"); !errors.Is(err, ErrNotExist) {
		t.Fatalf("Stat = %v, want ErrNotExist", err)
	}
}

func TestFileSystemOpenReturnsContent(t *testing.T) {
	fsys := NewFileSystem(nil)
	fsys.AddFile("/a", []byte("body"), 0o644)

	rc, err := fsys.Open("/a")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = rc.Close() }()
	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(got) != "body" {
		t.Fatalf("Open内容 = %q", got)
	}
}

func TestFileSystemRenameMovesTree(t *testing.T) {
	fsys := NewFileSystem(nil)
	fsys.AddFile("/staging/bin/tool", []byte("x"), 0o755)

	if err := fsys.Rename("/staging", "/current"); err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if _, err := fsys.Stat("/current/bin/tool"); err != nil {
		t.Fatalf("移動後のStat: %v", err)
	}
	if _, err := fsys.Stat("/staging/bin/tool"); !errors.Is(err, ErrNotExist) {
		t.Fatalf("移動元が残っている: %v", err)
	}
}

func TestFileSystemRemoveAllDropsSubtree(t *testing.T) {
	fsys := NewFileSystem(nil)
	fsys.AddFile("/tmp/op/a", []byte("1"), 0o644)
	fsys.AddFile("/tmp/op/b/c", []byte("2"), 0o644)
	fsys.AddFile("/tmp/keep", []byte("3"), 0o644)

	if err := fsys.RemoveAll("/tmp/op"); err != nil {
		t.Fatalf("RemoveAll: %v", err)
	}
	if _, err := fsys.Stat("/tmp/op/b/c"); !errors.Is(err, ErrNotExist) {
		t.Fatalf("subtreeが残っている: %v", err)
	}
	if _, err := fsys.Stat("/tmp/keep"); err != nil {
		t.Fatalf("無関係のfileが消えた: %v", err)
	}
}

func TestFileSystemWalkIsSortedAndDoesNotFollowLinks(t *testing.T) {
	fsys := NewFileSystem(nil)
	fsys.AddFile("/root/b.txt", []byte("b"), 0o644)
	fsys.AddFile("/root/a.txt", []byte("a"), 0o644)
	fsys.AddLink("/root/link", port.LinkSymlink, "/outside")
	fsys.AddFile("/outside/secret", []byte("s"), 0o644)

	var seen []string
	err := fsys.Walk("/root", func(p string, info port.FileInfo) error {
		seen = append(seen, p)
		return nil
	})
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	want := []string{"/root", "/root/a.txt", "/root/b.txt", "/root/link"}
	if !reflect.DeepEqual(seen, want) {
		t.Fatalf("Walk = %v, want %v", seen, want)
	}
}

func TestFileSystemWalkPropagatesError(t *testing.T) {
	fsys := NewFileSystem(nil)
	fsys.AddFile("/root/a", []byte("a"), 0o644)
	sentinel := errors.New("stop")

	err := fsys.Walk("/root", func(string, port.FileInfo) error { return sentinel })
	if !errors.Is(err, sentinel) {
		t.Fatalf("Walk = %v, want sentinel", err)
	}
}

func TestFileSystemRealPathResolvesLink(t *testing.T) {
	fsys := NewFileSystem(nil)
	fsys.AddFile("/dist/1.0.0/bin", []byte("x"), 0o755)
	fsys.AddLink("/dist/current", port.LinkSymlink, "1.0.0")

	got, err := fsys.RealPath("/dist/current")
	if err != nil {
		t.Fatalf("RealPath: %v", err)
	}
	if got != "/dist/1.0.0" {
		t.Fatalf("RealPath = %q, want /dist/1.0.0", got)
	}
}

func TestFileSystemRealPathRejectsLinkCycle(t *testing.T) {
	fsys := NewFileSystem(nil)
	fsys.AddLink("/a", port.LinkSymlink, "/b")
	fsys.AddLink("/b", port.LinkSymlink, "/a")

	if _, err := fsys.RealPath("/a"); err == nil {
		t.Fatal("循環linkでerrorにならなかった")
	}
}

// --- LinkManager ---

func TestLinkManagerCreateAndKind(t *testing.T) {
	set := NewSet()
	set.FileSystem.AddDir("/dist/1.0.0", 0o755)

	if err := set.LinkManager.CreateSymlink("/dist/current", "1.0.0", true); err != nil {
		t.Fatalf("CreateSymlink: %v", err)
	}
	kind, err := set.LinkManager.Kind("/dist/current")
	if err != nil {
		t.Fatalf("Kind: %v", err)
	}
	if kind != port.LinkSymlink {
		t.Fatalf("Kind = %q, want symlink", kind)
	}
	target, err := set.LinkManager.ReadLink("/dist/current")
	if err != nil {
		t.Fatalf("ReadLink: %v", err)
	}
	if target != "1.0.0" {
		t.Fatalf("ReadLink = %q, want 1.0.0", target)
	}
}

// 能力が無いlink種別の作成が失敗することを確認する。
// Windows標準ユーザーでsymlinkを作れない状況に相当する。
func TestLinkManagerRejectsUnsupportedKind(t *testing.T) {
	set := NewSet()
	set.FileSystem.AddDir("/dist", 0o755)
	set.LinkManager.Caps["/dist"] = port.LinkCapabilities{Junction: true}

	err := set.LinkManager.CreateSymlink("/dist/current", "1.0.0", true)
	if !errors.Is(err, ErrLinkUnsupported) {
		t.Fatalf("CreateSymlink = %v, want ErrLinkUnsupported", err)
	}
	if err := set.LinkManager.CreateJunction("/dist/current", "/dist/1.0.0"); err != nil {
		t.Fatalf("CreateJunction: %v", err)
	}
}

// RemoveLinkがlinkだけを外し、link先の実体を消さないことを確認する。
// currentの張り替えでtool本体を消す事故を防ぐ契約である。
func TestLinkManagerRemoveLinkKeepsTarget(t *testing.T) {
	set := NewSet()
	set.FileSystem.AddFile("/dist/1.0.0/bin", []byte("payload"), 0o755)
	if err := set.LinkManager.CreateSymlink("/dist/current", "1.0.0", true); err != nil {
		t.Fatalf("CreateSymlink: %v", err)
	}

	if err := set.LinkManager.RemoveLink("/dist/current"); err != nil {
		t.Fatalf("RemoveLink: %v", err)
	}
	if _, err := set.FileSystem.Stat("/dist/current"); !errors.Is(err, ErrNotExist) {
		t.Fatalf("linkが残っている: %v", err)
	}
	if _, err := set.FileSystem.Stat("/dist/1.0.0/bin"); err != nil {
		t.Fatalf("link先の実体が消えた: %v", err)
	}
}

func TestLinkManagerRemoveLinkRejectsRegularFile(t *testing.T) {
	set := NewSet()
	set.FileSystem.AddFile("/dist/real", []byte("x"), 0o644)

	if err := set.LinkManager.RemoveLink("/dist/real"); err == nil {
		t.Fatal("通常fileをRemoveLinkできてしまった")
	}
}

// --- HTTPClient ---

func TestHTTPClientGetReturnsStub(t *testing.T) {
	h := NewHTTPClient(nil)
	h.Stub("https://example.test/a", HTTPStub{StatusCode: 200, Body: []byte("payload")})

	resp, err := h.Get(context.Background(), port.HTTPRequest{
		URL: "https://example.test/a", MaxBodyBytes: 1024,
	})
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(body) != "payload" {
		t.Fatalf("body = %q", body)
	}
	if resp.FinalURL != "https://example.test/a" {
		t.Fatalf("FinalURL = %q", resp.FinalURL)
	}
}

func TestHTTPClientRejectsPlainHTTP(t *testing.T) {
	h := NewHTTPClient(nil)
	_, err := h.Get(context.Background(), port.HTTPRequest{URL: "http://example.test/a", MaxBodyBytes: 10})
	if err == nil {
		t.Fatal("httpを受け入れてしまった")
	}
}

func TestHTTPClientRejectsMissingBodyLimit(t *testing.T) {
	h := NewHTTPClient(nil)
	_, err := h.Get(context.Background(), port.HTTPRequest{URL: "https://example.test/a"})
	if err == nil {
		t.Fatal("MaxBodyBytes未指定を受け入れてしまった")
	}
}

func TestHTTPClientRejectsOverLimitBody(t *testing.T) {
	h := NewHTTPClient(nil)
	h.Stub("https://example.test/a", HTTPStub{StatusCode: 200, Body: []byte("0123456789")})

	_, err := h.Get(context.Background(), port.HTTPRequest{URL: "https://example.test/a", MaxBodyBytes: 4})
	if err == nil {
		t.Fatal("上限超過bodyを受け入れてしまった")
	}
}

func TestHTTPClientFollowsRedirectWithinLimit(t *testing.T) {
	h := NewHTTPClient(nil)
	h.Stub("https://a.test/x", HTTPStub{RedirectTo: "https://b.test/y"})
	h.Stub("https://b.test/y", HTTPStub{StatusCode: 200, Body: []byte("final")})

	resp, err := h.Get(context.Background(), port.HTTPRequest{
		URL: "https://a.test/x", MaxRedirects: 1, MaxBodyBytes: 1024,
	})
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.FinalURL != "https://b.test/y" {
		t.Fatalf("FinalURL = %q", resp.FinalURL)
	}
	// redirect元と先の両方が記録され、host allowlist検査で照合できる。
	want := []string{"https://a.test/x", "https://b.test/y"}
	if !reflect.DeepEqual(h.Requests, want) {
		t.Fatalf("Requests = %v, want %v", h.Requests, want)
	}
}

func TestHTTPClientRejectsTooManyRedirects(t *testing.T) {
	h := NewHTTPClient(nil)
	h.Stub("https://a.test/x", HTTPStub{RedirectTo: "https://b.test/y"})
	h.Stub("https://b.test/y", HTTPStub{StatusCode: 200, Body: []byte("final")})

	_, err := h.Get(context.Background(), port.HTTPRequest{
		URL: "https://a.test/x", MaxRedirects: 0, MaxBodyBytes: 1024,
	})
	if err == nil {
		t.Fatal("MaxRedirects=0でredirectを追跡してしまった")
	}
}

func TestHTTPClientUnknownURLIsError(t *testing.T) {
	h := NewHTTPClient(nil)
	_, err := h.Get(context.Background(), port.HTTPRequest{URL: "https://unknown.test/", MaxBodyBytes: 10})
	if err == nil {
		t.Fatal("未登録URLが成功してしまった")
	}
}

func TestHTTPClientDownloadFailureInjection(t *testing.T) {
	h := NewHTTPClient(nil)
	h.Stub("https://example.test/a", HTTPStub{StatusCode: 200, Body: []byte("x")})
	h.Injector().FailOnce(OpHTTPGet, ErrDownloadFailed)

	_, err := h.Get(context.Background(), port.HTTPRequest{URL: "https://example.test/a", MaxBodyBytes: 10})
	if !errors.Is(err, ErrDownloadFailed) {
		t.Fatalf("Get = %v, want ErrDownloadFailed", err)
	}
}

func TestHTTPClientHonorsCancelledContext(t *testing.T) {
	h := NewHTTPClient(nil)
	h.Stub("https://example.test/a", HTTPStub{StatusCode: 200, Body: []byte("x")})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := h.Get(ctx, port.HTTPRequest{URL: "https://example.test/a", MaxBodyBytes: 10})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Get = %v, want context.Canceled", err)
	}
}

func TestHTTPClientHeadHasNoBody(t *testing.T) {
	h := NewHTTPClient(nil)
	h.Stub("https://example.test/a", HTTPStub{StatusCode: 200, Body: []byte("payload")})

	resp, err := h.Head(context.Background(), port.HTTPRequest{URL: "https://example.test/a", MaxBodyBytes: 1024})
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	if resp.Body != nil {
		t.Fatal("HeadがBodyを返した")
	}
	if resp.ContentLength != 7 {
		t.Fatalf("ContentLength = %d, want 7", resp.ContentLength)
	}
}

// --- ProcessRunner ---

func TestProcessRunnerRecordsInvocation(t *testing.T) {
	p := NewProcessRunner(nil)
	p.Stub("/tools/go/bin/go", ProcessStub{ExitCode: 0, Stdout: "go1.26.5"})

	res, err := p.Run(context.Background(), port.ProcessSpec{
		Executable: "/tools/go/bin/go",
		Args:       []string{"version"},
		Dir:        "/work",
		Env:        map[string]string{"GOTOOLCHAIN": "local"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Stdout != "go1.26.5" {
		t.Fatalf("Stdout = %q", res.Stdout)
	}
	want := []string{"/tools/go/bin/go version"}
	if !reflect.DeepEqual(p.CommandLines(), want) {
		t.Fatalf("CommandLines = %v, want %v", p.CommandLines(), want)
	}
	if p.Invocations[0].Env["GOTOOLCHAIN"] != "local" {
		t.Fatalf("Env が記録されていない: %v", p.Invocations[0].Env)
	}
}

// 未登録executableの起動を失敗させることで、仕様が禁止する
// 任意helper processの起動をtestが黙って通さないようにする。
func TestProcessRunnerRejectsUnstubbedExecutable(t *testing.T) {
	p := NewProcessRunner(nil)
	_, err := p.Run(context.Background(), port.ProcessSpec{
		Executable: "/usr/bin/helper", Dir: "/work",
	})
	if err == nil {
		t.Fatal("未登録executableが成功してしまった")
	}
	// 記録には残るので、何が起動されようとしたかを検査できる。
	if got := p.ExecutablesRun(); !reflect.DeepEqual(got, []string{"/usr/bin/helper"}) {
		t.Fatalf("ExecutablesRun = %v", got)
	}
}

func TestProcessRunnerRejectsEmptyDir(t *testing.T) {
	p := NewProcessRunner(nil)
	p.Stub("/bin/tool", ProcessStub{})

	_, err := p.Run(context.Background(), port.ProcessSpec{Executable: "/bin/tool"})
	if err == nil {
		t.Fatal("Dir未指定を受け入れてしまった")
	}
}

// PassthroughStdio時にgdtvmが内容を保存しない契約をfakeも守る。
func TestProcessRunnerPassthroughDoesNotCapture(t *testing.T) {
	p := NewProcessRunner(nil)
	p.Stub("/bin/tool", ProcessStub{ExitCode: 3, Stdout: "secret", Stderr: "secret"})

	res, err := p.Run(context.Background(), port.ProcessSpec{
		Executable: "/bin/tool", Dir: "/work", PassthroughStdio: true,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Stdout != "" || res.Stderr != "" {
		t.Fatalf("passthroughで内容を保持した: %+v", res)
	}
	if res.ExitCode != 3 {
		t.Fatalf("ExitCode = %d, want 3", res.ExitCode)
	}
}

func TestProcessRunnerProbeFailureInjection(t *testing.T) {
	p := NewProcessRunner(nil)
	p.Stub("/bin/tool", ProcessStub{})
	p.Injector().FailOnce(OpProcessRun, ErrProbeFailed)

	_, err := p.Run(context.Background(), port.ProcessSpec{Executable: "/bin/tool", Dir: "/work"})
	if !errors.Is(err, ErrProbeFailed) {
		t.Fatalf("Run = %v, want ErrProbeFailed", err)
	}
}

// --- UserLookup ---

func TestUserLookupCurrentAndOwner(t *testing.T) {
	set := NewSet()

	id, err := set.UserLookup.Current()
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	if id.Home != "/home/testuser" {
		t.Fatalf("Home = %q", id.Home)
	}

	// 未登録pathは実行中userの所有として扱う。
	owner, err := set.UserLookup.OwnerOf("/home/testuser/gdtvm.toml")
	if err != nil {
		t.Fatalf("OwnerOf: %v", err)
	}
	if owner != id.ID {
		t.Fatalf("OwnerOf = %q, want %q", owner, id.ID)
	}

	set.UserLookup.SetOwner("/home/other/gdtvm.toml", "1001")
	owner, err = set.UserLookup.OwnerOf("/home/other/gdtvm.toml")
	if err != nil {
		t.Fatalf("OwnerOf: %v", err)
	}
	if owner != "1001" {
		t.Fatalf("他userのOwnerOf = %q, want 1001", owner)
	}
}

// --- 横断 ---

// 1つのInjectorを共有することで、port横断の失敗順序を1箇所で組めることを確認する。
func TestSharedInjectorAcrossPorts(t *testing.T) {
	set := NewSet()
	set.FileSystem.AddDir("/staging", 0o755)
	set.HTTPClient.Stub("https://example.test/a", HTTPStub{StatusCode: 200, Body: []byte("x")})

	set.Injector.FailOnce(OpHTTPGet, ErrDownloadFailed)
	set.Injector.FailOnce(OpAtomicWrite, ErrDiskFull)

	if _, err := set.HTTPClient.Get(context.Background(), port.HTTPRequest{
		URL: "https://example.test/a", MaxBodyBytes: 10,
	}); !errors.Is(err, ErrDownloadFailed) {
		t.Fatalf("Get = %v", err)
	}
	if err := set.FileSystem.AtomicWrite("/staging/a", []byte("x"), 0o644); !errors.Is(err, ErrDiskFull) {
		t.Fatalf("AtomicWrite = %v", err)
	}
	if pending := set.Injector.Pending(); len(pending) != 0 {
		t.Fatalf("消化されない注入が残った: %v", pending)
	}
	want := []string{OpAtomicWrite, OpHTTPGet}
	if got := set.Injector.Operations(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Operations = %v, want %v", got, want)
	}
}
