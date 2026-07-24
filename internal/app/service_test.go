package app

import (
	"io"
	"strings"
	"testing"
	"time"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/infra"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/ports"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/porttest"
)

// validDeps は fake/stub port で構成した最小構成の依存を返す。idPrefix で ID 生成器を
// 個体ごとに区別できる。
func validDeps(idPrefix string) Dependencies {
	clk := porttest.NewFakeClock(time.Unix(0, 0).UTC())
	return Dependencies{
		Mode:              domain.ModePortable,
		FS:                porttest.StubFileSystem{},
		Links:             porttest.StubLinkManager{},
		HTTP:              porttest.StubHTTPClient{},
		Process:           porttest.StubProcessRunner{},
		Archive:           porttest.StubArchiveExtractor{},
		Hash:              porttest.StubHashCalculator{},
		Locks:             porttest.StubLockManager{},
		Clock:             clk,
		IDs:               porttest.NewFakeIDGenerator(idPrefix),
		Logger:            infra.NewLogger(io.Discard, ports.LevelInfo, clk),
		Locale:            porttest.StubLocaleProvider{Locale: ports.Locale{Language: "en"}},
		ReleaseVerifier:   porttest.StubReleaseIntegrityVerifier{},
		SignatureVerifier: porttest.StubSignatureVerifier{},
		Events:            &porttest.RecordingSink{},
		Approvals:         porttest.DenyApprovals{},
	}
}

func TestNewService_Success(t *testing.T) {
	svc, err := NewService(validDeps("op"))
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	if svc.Mode() != domain.ModePortable {
		t.Errorf("Mode = %q", svc.Mode())
	}
}

func TestNewService_InvalidMode(t *testing.T) {
	deps := validDeps("op")
	deps.Mode = domain.Mode("bogus")
	if _, err := NewService(deps); err == nil {
		t.Fatal("不正 mode はエラーになるべき")
	}
}

func TestNewService_MissingDependency(t *testing.T) {
	// 代表として複数の必須 port を個別に欠落させ、名前がエラーへ現れることを確認する。
	cases := map[string]func(*Dependencies){
		"FileSystem":       func(d *Dependencies) { d.FS = nil },
		"HTTPClient":       func(d *Dependencies) { d.HTTP = nil },
		"Clock":            func(d *Dependencies) { d.Clock = nil },
		"EventSink":        func(d *Dependencies) { d.Events = nil },
		"ApprovalProvider": func(d *Dependencies) { d.Approvals = nil },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			deps := validDeps("op")
			mutate(&deps)
			_, err := NewService(deps)
			if err == nil {
				t.Fatalf("%s 欠落はエラーになるべき", name)
			}
			if !strings.Contains(err.Error(), name) {
				t.Fatalf("エラーに %s が現れるべき: %v", name, err)
			}
		})
	}
}

// TestService_NoPackageGlobalState は 2 つの Service が独立した IDGenerator を持ち、
// operation ID の採番が互いに干渉しないことを確認する。package global counter が
// あればこの独立性は壊れる。
func TestService_NoPackageGlobalState(t *testing.T) {
	a, err := NewService(validDeps("a"))
	if err != nil {
		t.Fatalf("service a: %v", err)
	}
	b, err := NewService(validDeps("b"))
	if err != nil {
		t.Fatalf("service b: %v", err)
	}
	if got := a.operationID(); got != "a-1" {
		t.Errorf("a.operationID() = %q, want a-1", got)
	}
	if got := b.operationID(); got != "b-1" {
		t.Errorf("b.operationID() = %q, want b-1", got)
	}
	if got := a.operationID(); got != "a-2" {
		t.Errorf("a.operationID() = %q, want a-2（b の採番に干渉されない）", got)
	}
}
