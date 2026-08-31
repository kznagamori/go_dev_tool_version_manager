package install

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/definition"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain/port/fake"
)

const targetPayloadDir = "/data/gdtvm/tmp/operations/op1/payload"

// targetHarness はcommand target収集用のfake filesystemを用意する。
type targetHarness struct {
	fs     *fake.FileSystem
	inject *fake.Injector
}

func newTargetHarness(t *testing.T, files map[string][]byte) *targetHarness {
	t.Helper()
	injector := fake.NewInjector()
	filesystem := fake.NewFileSystem(injector)
	if err := filesystem.MkdirAll(targetPayloadDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	for path, data := range files {
		filesystem.AddFile(targetPayloadDir+"/"+path, data, 0o755)
	}
	return &targetHarness{fs: filesystem, inject: injector}
}

// targetRequest はrequired command 1件を持つ収集入力を返す。
func targetRequest(t *testing.T) CommandTargetRequest {
	t.Helper()
	roots := testRoots(t, "linux-amd64-glibc")
	roots.Payload = renderPathValue(t, domain.RolePayload, targetPayloadDir)
	return CommandTargetRequest{
		Platform: definition.Platform{
			Runtime: definition.Runtime{Commands: []definition.Command{{
				Name:     "go",
				Target:   "{{payload}}/bin/go",
				Required: true,
			}}},
		},
		PayloadDir: renderPathValue(t, domain.RolePayload, targetPayloadDir),
		Roots:      roots,
	}
}

func wantSHA256(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// TestCollectCommandTargetsRecordsRequiredTargets は記録内容を固定する。
//
// docs/04-storage-and-data.md §14「keyは`path`, `size`, `sha256`」。
func TestCollectCommandTargetsRecordsRequiredTargets(t *testing.T) {
	payload := []byte("go binary")
	h := newTargetHarness(t, map[string][]byte{"bin/go": payload})

	targets, err := CollectCommandTargets(h.fs, targetRequest(t))
	if err != nil {
		t.Fatalf("CollectCommandTargets = %v", err)
	}
	if len(targets) != 1 {
		t.Fatalf("targets = %d件, want 1", len(targets))
	}
	// §14の例が`payload/node.exe`であり、`payload_path=payload`固定のprefixを持つ。
	if want := "payload/bin/go"; targets[0].Path != want {
		t.Errorf("path = %q, want %q", targets[0].Path, want)
	}
	if targets[0].Size != int64(len(payload)) {
		t.Errorf("size = %d, want %d", targets[0].Size, len(payload))
	}
	if targets[0].SHA256 != wantSHA256(payload) {
		t.Errorf("sha256 = %q, want %q", targets[0].SHA256, wantSHA256(payload))
	}
}

// TestCollectCommandTargetsSkipsOptionalCommands は`required=false`を除くことを固定する。
//
// docs/08-install-runtime.md §7手順4が`required`だけを対象とする。任意command
// まで記録すると、そのcommandを使わない利用者のpayloadで`doctor`が存在しない
// fileの破損を報告する。
func TestCollectCommandTargetsSkipsOptionalCommands(t *testing.T) {
	h := newTargetHarness(t, map[string][]byte{
		"bin/go":    []byte("go"),
		"bin/gofmt": []byte("gofmt"),
	})
	req := targetRequest(t)
	req.Platform.Runtime.Commands = append(req.Platform.Runtime.Commands,
		definition.Command{Name: "gofmt", Target: "{{payload}}/bin/gofmt", Required: false})

	targets, err := CollectCommandTargets(h.fs, req)
	if err != nil {
		t.Fatalf("CollectCommandTargets = %v", err)
	}
	if len(targets) != 1 || targets[0].Path != "payload/bin/go" {
		t.Errorf("targets = %+v, want required commandだけ", targets)
	}
}

// TestCollectCommandTargetsIncludesPathArgs はfixed argsのpayload内fileを含めることを固定する。
func TestCollectCommandTargetsIncludesPathArgs(t *testing.T) {
	h := newTargetHarness(t, map[string][]byte{
		"bin/node":   []byte("node"),
		"lib/npm.js": []byte("npm"),
	})
	req := targetRequest(t)
	req.Platform.Runtime.Commands = []definition.Command{{
		Name:     "npm",
		Target:   "{{payload}}/bin/node",
		Args:     []string{"{{payload}}/lib/npm.js", "--no-update-notifier"},
		Required: true,
	}}

	targets, err := CollectCommandTargets(h.fs, req)
	if err != nil {
		t.Fatalf("CollectCommandTargets = %v", err)
	}
	if len(targets) != 2 {
		t.Fatalf("targets = %d件, want 2", len(targets))
	}
	// §14「payload相対path byte順」。
	if targets[0].Path != "payload/bin/node" || targets[1].Path != "payload/lib/npm.js" {
		t.Errorf("targets = %+v, want byte順", targets)
	}
}

// TestCollectCommandTargetsExcludesStorage はpayload外を記録しないことを固定する。
//
// **storageは利用者が書き換える領域である。** 完全性記録の対象にすると、
// 正常な変更を`doctor`が破損として報告する。
func TestCollectCommandTargetsExcludesStorage(t *testing.T) {
	h := newTargetHarness(t, map[string][]byte{"bin/node": []byte("node")})
	h.fs.AddFile(cacheRoot+"/config.json", []byte("{}"), 0o644)

	req := targetRequest(t)
	req.Platform.Runtime.Commands = []definition.Command{{
		Name:     "npm",
		Target:   "{{payload}}/bin/node",
		Args:     []string{"{{storage.npm-cache}}/config.json"},
		Required: true,
	}}

	targets, err := CollectCommandTargets(h.fs, req)
	if err != nil {
		t.Fatalf("CollectCommandTargets = %v", err)
	}
	if len(targets) != 1 || targets[0].Path != "payload/bin/node" {
		t.Errorf("targets = %+v, want payload内だけ", targets)
	}
}

// TestCollectCommandTargetsDeduplicates は同じfileを2度記録しないことを固定する。
//
// §14「payload相対path byte順・**一意**で持つ」。
func TestCollectCommandTargetsDeduplicates(t *testing.T) {
	h := newTargetHarness(t, map[string][]byte{"bin/node": []byte("node")})
	req := targetRequest(t)
	req.Platform.Runtime.Commands = []definition.Command{
		{Name: "node", Target: "{{payload}}/bin/node", Required: true},
		// 別commandが同じtargetを指す。
		{Name: "nodejs", Target: "{{payload}}/bin/node", Required: true},
	}
	targets, err := CollectCommandTargets(h.fs, req)
	if err != nil {
		t.Fatalf("CollectCommandTargets = %v", err)
	}
	if len(targets) != 1 {
		t.Errorf("targets = %+v, want 1件（重複排除）", targets)
	}
}

// TestCollectCommandTargetsReportsMissingFile は実体が無ければ失敗することを固定する。
//
// required commandのtargetが無いpayloadをcommitすると、`doctor`が破損として
// 報告する状態を作ってしまう。展開直後に落とす。
func TestCollectCommandTargetsReportsMissingFile(t *testing.T) {
	h := newTargetHarness(t, nil)
	if _, err := CollectCommandTargets(h.fs, targetRequest(t)); err == nil {
		t.Fatal("実体が無いのに成功した")
	}
}

// TestCollectCommandTargetsReportsReadFailure は読取り失敗の注入を固定する。
func TestCollectCommandTargetsReportsReadFailure(t *testing.T) {
	h := newTargetHarness(t, map[string][]byte{"bin/go": []byte("go")})
	h.inject.Fail(fake.OpOpen, 0, 1, errors.New("注入したopen失敗"))
	if _, err := CollectCommandTargets(h.fs, targetRequest(t)); err == nil {
		t.Fatal("open失敗で成功した")
	}
}

// TestCollectCommandTargetsRejectsInvalidInput は前提違反を拒否することを固定する。
func TestCollectCommandTargetsRejectsInvalidInput(t *testing.T) {
	h := newTargetHarness(t, map[string][]byte{"bin/go": []byte("go")})
	if _, err := CollectCommandTargets(nil, targetRequest(t)); err == nil {
		t.Error("FileSystem無しで成功した")
	}
	req := targetRequest(t)
	req.PayloadDir = domain.PathValue{}
	if _, err := CollectCommandTargets(h.fs, req); err == nil {
		t.Error("payload未設定で成功した")
	}
}
