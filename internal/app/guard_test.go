package app

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain/port"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain/port/fake"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/security"
)

const (
	dataRoot      = "/data/gdtvm"
	outsideRoot   = "/elsewhere"
	probeExe      = "/data/gdtvm/tools/go/1.25.0/payload/bin/go"
	probeDir      = "/data/gdtvm/tmp/operations/op1/probe"
	artifactURL   = "https://example.invalid/go1.25.0.tar.gz"
	otherArtifact = "https://example.invalid/evil.tar.gz"
)

func hostOf(t *testing.T, id string) domain.Platform {
	t.Helper()
	value, err := domain.ParsePlatform(id)
	if err != nil {
		t.Fatalf("ParsePlatform(%q): %v", id, err)
	}
	return value
}

func pathValueOf(t *testing.T, role domain.PathRole, path string) domain.PathValue {
	t.Helper()
	value, err := domain.NewPathValue(role, path)
	if err != nil {
		t.Fatalf("NewPathValue(%s, %q): %v", role, path, err)
	}
	return value
}

// guardHarness はguard 1件分のfakeをまとめる。
type guardHarness struct {
	fs    *fake.FileSystem
	proc  *fake.ProcessRunner
	http  *fake.HTTPClient
	guard *Guard
}

func newGuardHarness(t *testing.T) *guardHarness {
	t.Helper()
	injector := fake.NewInjector()
	filesystem := fake.NewFileSystem(injector)
	filesystem.AddDir(dataRoot, 0o755)
	filesystem.AddDir(probeDir, 0o700)
	filesystem.AddDir(outsideRoot, 0o755)

	scope, err := NewScope(ScopeRequest{
		Roots: []domain.PathValue{pathValueOf(t, domain.RoleDataRoot, dataRoot)},
		Processes: []AllowedProcess{
			{Executable: probeExe, Args: []string{"version"}, Dir: probeDir},
		},
		Downloads: []string{artifactURL},
		Host:      hostOf(t, "linux-amd64-glibc"),
	})
	if err != nil {
		t.Fatalf("NewScope: %v", err)
	}
	guard, err := NewGuard(scope, nil)
	if err != nil {
		t.Fatalf("NewGuard: %v", err)
	}
	return &guardHarness{
		fs:    filesystem,
		proc:  fake.NewProcessRunner(injector),
		http:  fake.NewHTTPClient(injector),
		guard: guard,
	}
}

// TestGuardAllowsWritesInsideRoot は許可root内の書込みを通し、記録することを固定する。
func TestGuardAllowsWritesInsideRoot(t *testing.T) {
	harness := newGuardHarness(t)
	filesystem := harness.guard.FileSystem(harness.fs)

	if err := filesystem.MkdirAll(dataRoot+"/state", 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := filesystem.AtomicWrite(dataRoot+"/state/schema.toml", []byte("x"), 0o600); err != nil {
		t.Fatalf("AtomicWrite: %v", err)
	}
	if err := filesystem.Chmod(dataRoot+"/state/schema.toml", 0o400); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	if err := filesystem.Remove(dataRoot + "/state/schema.toml"); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	records := harness.guard.Records()
	want := []struct {
		action WriteAction
		path   string
	}{
		{WriteCreate, dataRoot + "/state"},
		{WriteCreate, dataRoot + "/state/schema.toml"},
		{WritePermission, dataRoot + "/state/schema.toml"},
		{WriteRemove, dataRoot + "/state/schema.toml"},
	}
	if len(records.Writes) != len(want) {
		t.Fatalf("記録 = %+v, want %d件", records.Writes, len(want))
	}
	for index, expected := range want {
		got := records.Writes[index]
		if got.Action != expected.action || got.Path != expected.path {
			t.Errorf("記録[%d] = %s %q, want %s %q",
				index, got.Action, got.Path, expected.action, expected.path)
		}
		// docs/11-quality-and-ci.md §7.2「判定は§17.2のpath_roleで行う」。
		if got.Role != domain.RoleDataRoot {
			t.Errorf("記録[%d].Role = %s, want %s", index, got.Role, domain.RoleDataRoot)
		}
	}
}

// TestGuardRejectsWritesOutsideRoot は許可root外の書込みを拒否することを固定する。
//
// docs/02-architecture.md §8手順5「全書込みがdata root、distribution root、
// 宣言済みintegration対象、project fileの中にあり」。
func TestGuardRejectsWritesOutsideRoot(t *testing.T) {
	harness := newGuardHarness(t)
	filesystem := harness.guard.FileSystem(harness.fs)

	tests := []struct {
		name string
		call func() error
	}{
		{"AtomicWrite", func() error {
			return filesystem.AtomicWrite(outsideRoot+"/x", []byte("x"), 0o600)
		}},
		{"WriteStream", func() error {
			_, err := filesystem.WriteStream(outsideRoot+"/x", 0o600, strings.NewReader("x"))
			return err
		}},
		{"MkdirAll", func() error { return filesystem.MkdirAll(outsideRoot+"/sub", 0o755) }},
		{"Remove", func() error { return filesystem.Remove(outsideRoot) }},
		{"RemoveAll", func() error { return filesystem.RemoveAll(outsideRoot) }},
		{"Chmod", func() error { return filesystem.Chmod(outsideRoot, 0o700) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.call()
			if err == nil {
				t.Fatal("許可root外の書込みが通った")
			}
			if code, _ := domain.CodeOf(err); code != domain.CodePathUnsafe {
				t.Errorf("code = %s, want %s（%v）", code, domain.CodePathUnsafe, err)
			}
			// 拒否したものは記録しない。記録は「実際に通した書込み」の証跡である。
			if writes := harness.guard.Records().Writes; len(writes) != 0 {
				t.Errorf("拒否した書込みが記録された: %+v", writes)
			}
		})
	}
}

// TestGuardRejectsRenameWhenEitherSideIsOutside はrenameの両端を見ることを固定する。
//
// 片方だけ見ると、管理root内へ管理外のものを引き込む／管理root外へ持ち出すの
// どちらかが素通りする。
func TestGuardRejectsRenameWhenEitherSideIsOutside(t *testing.T) {
	tests := []struct {
		name    string
		oldPath string
		newPath string
	}{
		{"移動元が外", outsideRoot + "/a", dataRoot + "/b"},
		{"移動先が外", dataRoot + "/a", outsideRoot + "/b"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			harness := newGuardHarness(t)
			harness.fs.AddFile(dataRoot+"/a", []byte("x"), 0o600)
			harness.fs.AddFile(outsideRoot+"/a", []byte("x"), 0o600)
			filesystem := harness.guard.FileSystem(harness.fs)

			err := filesystem.Rename(test.oldPath, test.newPath)
			if err == nil {
				t.Fatal("片側が許可root外のrenameが通った")
			}
			if code, _ := domain.CodeOf(err); code != domain.CodePathUnsafe {
				t.Errorf("code = %s, want %s", code, domain.CodePathUnsafe)
			}
		})
	}
}

// TestGuardRejectsWriteThroughSymlink はlink経由の逸脱を拒否することを固定する。
//
// docs/10-security.md §6「symlink/reparse point経由の逸脱を拒否する」。
// 解決前のpathで比べると、管理root内の名前で管理外へ書けてしまう。
func TestGuardRejectsWriteThroughSymlink(t *testing.T) {
	harness := newGuardHarness(t)
	harness.fs.AddLink(dataRoot+"/escape", port.LinkSymlink, outsideRoot)
	filesystem := harness.guard.FileSystem(harness.fs)

	err := filesystem.MkdirAll(dataRoot+"/escape/sub", 0o755)
	if err == nil {
		t.Fatal("symlink経由で許可root外へ書けた")
	}
	if code, _ := domain.CodeOf(err); code != domain.CodePathUnsafe {
		t.Errorf("code = %s, want %s（%v）", code, domain.CodePathUnsafe, err)
	}
}

// TestGuardDoesNotRestrictReads は読取りを縛らないことを固定する。
//
// §7.2の検査対象は「全write/move/delete先」である。読取りはproject fileの探索の
// ように管理外pathも正当に対象になる。
func TestGuardDoesNotRestrictReads(t *testing.T) {
	harness := newGuardHarness(t)
	harness.fs.AddFile(outsideRoot+"/project.toml", []byte("body"), 0o644)
	filesystem := harness.guard.FileSystem(harness.fs)

	if _, err := filesystem.ReadFile(outsideRoot+"/project.toml", 0); err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if _, err := filesystem.Stat(outsideRoot + "/project.toml"); err != nil {
		t.Fatalf("Stat: %v", err)
	}
	reader, err := filesystem.Open(outsideRoot + "/project.toml")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, err := io.ReadAll(reader); err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	_ = reader.Close()
	if _, err := filesystem.OpenAt(outsideRoot + "/project.toml"); err != nil {
		t.Fatalf("OpenAt: %v", err)
	}
	if _, err := filesystem.RealPath(outsideRoot + "/project.toml"); err != nil {
		t.Fatalf("RealPath: %v", err)
	}
	if writes := harness.guard.Records().Writes; len(writes) != 0 {
		t.Errorf("読取りが書込みとして記録された: %+v", writes)
	}
}

// TestGuardAllowsDeclaredProcess は宣言どおりのprocessを通し、記録することを固定する。
func TestGuardAllowsDeclaredProcess(t *testing.T) {
	harness := newGuardHarness(t)
	harness.proc.Stub(probeExe, fake.ProcessStub{Stdout: "go version go1.25.0"})
	runner := harness.guard.ProcessRunner(harness.proc)

	result, err := runner.Run(context.Background(), port.ProcessSpec{
		Executable: probeExe,
		Args:       []string{"version"},
		Dir:        probeDir,
		Env:        map[string]string{"GOTOOLCHAIN": "local", "EXAMPLE_TOKEN": "s3cr3t"},
	})
	if err != nil {
		t.Fatalf("Run = %v", err)
	}
	if result.Stdout != "go version go1.25.0" {
		t.Errorf("stdout = %q", result.Stdout)
	}

	records := harness.guard.Records().Processes
	if len(records) != 1 {
		t.Fatalf("記録 = %+v, want 1件", records)
	}
	if records[0].Executable != probeExe || records[0].Dir != probeDir {
		t.Errorf("記録 = %+v", records[0])
	}
	// docs/10-security.md §9.2「環境変数の全量dumpを出さず、宣言したkeyの有無だけ」。
	want := []string{"EXAMPLE_TOKEN", "GOTOOLCHAIN"}
	for index := range want {
		if records[0].EnvNames[index] != want[index] {
			t.Errorf("EnvNames[%d] = %q, want %q", index, records[0].EnvNames[index], want[index])
		}
	}
}

// TestGuardRejectsUndeclaredProcess はPlan外のprocess起動を拒否することを固定する。
//
// docs/10-security.md §7「Plan `probes[]`にないexternal executableをExecute中に
// 発見して起動しない」。**起動する前に**拒否する。
func TestGuardRejectsUndeclaredProcess(t *testing.T) {
	tests := []struct {
		name string
		spec port.ProcessSpec
	}{
		{"別のexecutable", port.ProcessSpec{
			Executable: "/usr/bin/curl", Args: []string{"version"}, Dir: probeDir}},
		{"argsが違う", port.ProcessSpec{
			Executable: probeExe, Args: []string{"env", "-w", "GOPROXY=off"}, Dir: probeDir}},
		{"argsを後ろへ足した", port.ProcessSpec{
			Executable: probeExe, Args: []string{"version", "-json"}, Dir: probeDir}},
		{"argsが無い", port.ProcessSpec{
			Executable: probeExe, Dir: probeDir}},
		{"cwdが違う", port.ProcessSpec{
			Executable: probeExe, Args: []string{"version"}, Dir: dataRoot}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			harness := newGuardHarness(t)
			harness.proc.Stub(test.spec.Executable, fake.ProcessStub{})
			runner := harness.guard.ProcessRunner(harness.proc)

			if _, err := runner.Run(context.Background(), test.spec); err == nil {
				t.Fatal("宣言していないprocessが起動できた")
			} else if code, _ := domain.CodeOf(err); code != domain.CodePathUnsafe {
				t.Errorf("code = %s, want %s", code, domain.CodePathUnsafe)
			}
			// 内側のportまで到達していないことを確かめる。到達していれば
			// 「起動してから記録を見る」実装になっている。
			if len(harness.proc.Invocations) != 0 {
				t.Errorf("拒否したはずのprocessが内側へ到達した: %+v", harness.proc.Invocations)
			}
			if records := harness.guard.Records().Processes; len(records) != 0 {
				t.Errorf("拒否したprocessが記録された: %+v", records)
			}
		})
	}
}

// TestGuardAllowsDeclaredDownload は宣言どおりのdownloadを通し、maskして記録する
// ことを固定する。
func TestGuardAllowsDeclaredDownload(t *testing.T) {
	harness := newGuardHarness(t)
	harness.http.Stub(artifactURL, fake.HTTPStub{StatusCode: 200, Body: []byte("payload")})
	client := harness.guard.HTTPClient(harness.http)

	response, err := client.Get(context.Background(), port.HTTPRequest{URL: artifactURL, MaxBodyBytes: 1 << 20})
	if err != nil {
		t.Fatalf("Get = %v", err)
	}
	_ = response.Body.Close()

	records := harness.guard.Records().Downloads
	if len(records) != 1 || records[0].URL != artifactURL {
		t.Fatalf("記録 = %+v", records)
	}
}

// TestGuardMasksRecordedDownloadURL は記録するURLからcredentialを落とすことを固定する。
func TestGuardMasksRecordedDownloadURL(t *testing.T) {
	const withCredential = "https://user:pw@example.invalid/a?access_token=SECRETVALUE"
	scope, err := NewScope(ScopeRequest{
		Roots:     []domain.PathValue{pathValueOf(t, domain.RoleDataRoot, dataRoot)},
		Downloads: []string{withCredential},
		Host:      hostOf(t, "linux-amd64-glibc"),
	})
	if err != nil {
		t.Fatalf("NewScope: %v", err)
	}
	guard, err := NewGuard(scope, security.NewPathMasker("/home/tester", "tester", "devbox"))
	if err != nil {
		t.Fatalf("NewGuard: %v", err)
	}
	client := fake.NewHTTPClient(nil)
	client.Stub(withCredential, fake.HTTPStub{StatusCode: 200, Body: []byte("x")})

	response, err := guard.HTTPClient(client).Get(context.Background(),
		port.HTTPRequest{URL: withCredential, MaxBodyBytes: 1 << 20})
	if err != nil {
		t.Fatalf("Get = %v", err)
	}
	_ = response.Body.Close()

	recorded := guard.Records().Downloads[0].URL
	if strings.Contains(recorded, "SECRETVALUE") || strings.Contains(recorded, "user:pw@") {
		t.Errorf("記録にcredentialが残った: %q", recorded)
	}
}

// TestGuardRejectsUndeclaredDownload はPlan外の取得先を拒否することを固定する。
//
// docs/02-architecture.md §8手順5「Execute中のdownload/extract/probeがPlanの
// 列挙と一致」。HEADもGETと同じ許可listで縛る。宛先へ到達する点は変わらない。
func TestGuardRejectsUndeclaredDownload(t *testing.T) {
	for _, method := range []string{"Get", "Head"} {
		t.Run(method, func(t *testing.T) {
			harness := newGuardHarness(t)
			harness.http.Stub(otherArtifact, fake.HTTPStub{StatusCode: 200, Body: []byte("x")})
			client := harness.guard.HTTPClient(harness.http)

			var err error
			if method == "Get" {
				_, err = client.Get(context.Background(), port.HTTPRequest{URL: otherArtifact})
			} else {
				_, err = client.Head(context.Background(), port.HTTPRequest{URL: otherArtifact})
			}
			if err == nil {
				t.Fatal("宣言していない取得先へ接続できた")
			}
			if code, _ := domain.CodeOf(err); code != domain.CodePathUnsafe {
				t.Errorf("code = %s, want %s", code, domain.CodePathUnsafe)
			}
			if records := harness.guard.Records().Downloads; len(records) != 0 {
				t.Errorf("拒否した取得先が記録された: %+v", records)
			}
		})
	}
}

// TestGuardErrorsDoNotLeakPaths はtyped errorへpathやURLを載せないことを固定する。
//
// docs/04-storage-and-data.md §17.2「typed errorは秘密値や個人pathを露出させず、
// exact keyを保ったままpathを空にしてroleだけを伝えられる」。
func TestGuardErrorsDoNotLeakPaths(t *testing.T) {
	harness := newGuardHarness(t)
	const secretPath = outsideRoot + "/home-of-someone/token.txt"
	harness.fs.AddDir(outsideRoot+"/home-of-someone", 0o755)

	err := harness.guard.FileSystem(harness.fs).AtomicWrite(secretPath, []byte("x"), 0o600)
	if err == nil {
		t.Fatal("許可root外の書込みが通った")
	}
	if strings.Contains(err.Error(), "home-of-someone") {
		t.Errorf("errorへpathが載った: %v", err)
	}

	_, urlErr := harness.guard.HTTPClient(harness.http).Get(
		context.Background(), port.HTTPRequest{URL: otherArtifact})
	if urlErr == nil {
		t.Fatal("宣言していない取得先が通った")
	}
	if strings.Contains(urlErr.Error(), "evil.tar.gz") {
		t.Errorf("errorへURLが載った: %v", urlErr)
	}
}

// TestGuardPicksDeepestRootForRole は入れ子rootで最も深いroleを記録することを固定する。
//
// 宣言順に依存して別のroleを記録すると、§7.2のrole単位の照合が入力順で変わる。
func TestGuardPicksDeepestRootForRole(t *testing.T) {
	scope, err := NewScope(ScopeRequest{
		Roots: []domain.PathValue{
			pathValueOf(t, domain.RoleDataRoot, dataRoot),
			pathValueOf(t, domain.RoleDownloadCache, dataRoot+"/cache/downloads"),
		},
		Host: hostOf(t, "linux-amd64-glibc"),
	})
	if err != nil {
		t.Fatalf("NewScope: %v", err)
	}
	guard, err := NewGuard(scope, nil)
	if err != nil {
		t.Fatalf("NewGuard: %v", err)
	}
	filesystem := fake.NewFileSystem(nil)
	filesystem.AddDir(dataRoot+"/cache/downloads", 0o700)

	if err := guard.FileSystem(filesystem).AtomicWrite(
		dataRoot+"/cache/downloads/go.tar.gz", []byte("x"), 0o600); err != nil {
		t.Fatalf("AtomicWrite: %v", err)
	}
	if role := guard.Records().Writes[0].Role; role != domain.RoleDownloadCache {
		t.Errorf("Role = %s, want %s", role, domain.RoleDownloadCache)
	}
}

// TestGuardRecordsAreImmutable は返した記録が内部状態と切り離されていることを固定する。
//
// docs/02-architecture.md §4「request/resultは境界通過後にimmutableとして扱う」。
func TestGuardRecordsAreImmutable(t *testing.T) {
	harness := newGuardHarness(t)
	harness.proc.Stub(probeExe, fake.ProcessStub{})
	if _, err := harness.guard.ProcessRunner(harness.proc).Run(
		context.Background(), port.ProcessSpec{
			Executable: probeExe, Args: []string{"version"}, Dir: probeDir,
			Env: map[string]string{"A": "1"},
		}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	first := harness.guard.Records()
	first.Processes[0].Executable = "tampered"
	first.Processes[0].Args[0] = "tampered"
	first.Processes[0].EnvNames[0] = "tampered"

	second := harness.guard.Records()
	if second.Processes[0].Executable != probeExe {
		t.Error("返した記録の書換えが内部状態へ伝わった（Executable）")
	}
	if second.Processes[0].Args[0] != "version" {
		t.Error("返した記録の書換えが内部状態へ伝わった（Args）")
	}
	if second.Processes[0].EnvNames[0] != "A" {
		t.Error("返した記録の書換えが内部状態へ伝わった（EnvNames）")
	}
}

// TestGuardRecordsConcurrently は並行呼出しで記録が壊れないことを固定する。
func TestGuardRecordsConcurrently(t *testing.T) {
	harness := newGuardHarness(t)
	filesystem := harness.guard.FileSystem(harness.fs)

	var wait sync.WaitGroup
	for index := 0; index < 16; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			_ = filesystem.MkdirAll(dataRoot+"/state", 0o755)
			_ = harness.guard.Records()
		}(index)
	}
	wait.Wait()
	if got := len(harness.guard.Records().Writes); got != 16 {
		t.Errorf("記録 = %d件, want 16件", got)
	}
}

// TestNewGuardRequiresScope は依存不足を拒否することを固定する。
func TestNewGuardRequiresScope(t *testing.T) {
	if _, err := NewGuard(nil, nil); err == nil {
		t.Fatal("Scopeなしで作れた")
	}
}

// TestGuardRejectsUnresolvableWrite は祖先を解決できない書込みを拒否することを固定する。
//
// 解決できないままcontainmentを判定すると、判定していないのに通したことになる。
func TestGuardRejectsUnresolvableWrite(t *testing.T) {
	harness := newGuardHarness(t)
	err := harness.guard.FileSystem(harness.fs).AtomicWrite(
		"/nowhere/at/all/x", []byte("x"), 0o600)
	if err == nil {
		t.Fatal("解決できない書込み先が通った")
	}
	if code, _ := domain.CodeOf(err); code != domain.CodePathUnsafe {
		t.Errorf("code = %s, want %s（%v）", code, domain.CodePathUnsafe, err)
	}
}

// TestGuardResolvesMissingLeafThroughExistingAncestor は未作成のpathを既存の祖先
// 経由で判定できることを固定する。
func TestGuardResolvesMissingLeafThroughExistingAncestor(t *testing.T) {
	harness := newGuardHarness(t)
	// `state`も`a`も`b`も未作成。既存の祖先はdataRootだけである。
	if err := harness.guard.FileSystem(harness.fs).MkdirAll(
		dataRoot+"/state/a/b", 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	records := harness.guard.Records().Writes
	if len(records) != 1 || records[0].Path != dataRoot+"/state/a/b" {
		t.Fatalf("記録 = %+v", records)
	}
}

// TestGuardPropagatesInnerError は内側portのerrorをそのまま返すことを固定する。
func TestGuardPropagatesInnerError(t *testing.T) {
	harness := newGuardHarness(t)
	harness.fs.Injector().Fail(fake.OpAtomicWrite, 0, 1, fake.ErrDiskFull)

	err := harness.guard.FileSystem(harness.fs).AtomicWrite(
		dataRoot+"/state.toml", []byte("x"), 0o600)
	if !errors.Is(err, fake.ErrDiskFull) {
		t.Fatalf("err = %v, want ErrDiskFull", err)
	}
	// 通した書込みは、内側が失敗しても記録に残る。実際に試みた証跡だからである。
	if writes := harness.guard.Records().Writes; len(writes) != 1 {
		t.Errorf("記録 = %+v, want 1件", writes)
	}
}
