package install

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain/port"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain/port/fake"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/store"
)

const (
	commitOperationDir = "/data/gdtvm/tmp/operations/op1"
	commitVersionDir   = commitOperationDir + "/1.25.0"
	commitPayloadDir   = commitVersionDir + "/payload"
	commitDestination  = "/data/gdtvm/tools/go/versions/1.25.0/linux-amd64-glibc"
	commitIndexPath    = "/data/gdtvm/state/receipt-index.toml"
	commitReceiptName  = ".gdtvm-install.toml"
)

// commitHarness はcommit 1件分のfakeをまとめる。
type commitHarness struct {
	fs        *fake.FileSystem
	inject    *fake.Injector
	committer *Committer
}

func newCommitHarness(t *testing.T) *commitHarness {
	t.Helper()
	injector := fake.NewInjector()
	filesystem := fake.NewFileSystem(injector)
	if err := filesystem.MkdirAll(commitPayloadDir+"/bin", 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	filesystem.AddFile(commitPayloadDir+"/bin/go", []byte("go"), 0o555)
	if err := filesystem.MkdirAll("/data/gdtvm/state", 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	committer, err := NewCommitter(filesystem)
	if err != nil {
		t.Fatalf("NewCommitter: %v", err)
	}
	return &commitHarness{fs: filesystem, inject: injector, committer: committer}
}

func commitRequest(t *testing.T) CommitRequest {
	t.Helper()
	return CommitRequest{
		StagingPayload: stagePathValue(t, domain.RolePayload, commitPayloadDir),
		OperationDir:   stagePathValue(t, domain.RoleStaging, commitOperationDir),
		Receipt:        conflictReceipt(t),
		Destination:    stagePathValue(t, domain.RoleVersionData, commitDestination),
		ReceiptName:    commitReceiptName,
		Index: store.ReceiptIndex{
			Revision: 3,
			RootID:   strings.Repeat("2", 32),
		},
		IndexPath:      stagePathValue(t, domain.RoleReceiptIndex, commitIndexPath),
		IndexEntryPath: "tools/go/versions/1.25.0/linux-amd64-glibc/" + commitReceiptName,
		Host:           platformOf(t, "linux-amd64-glibc"),
		Now:            time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC),
	}
}

// TestCommitRenamesAndUpdatesIndex は§7手順6〜8の成功経路を固定する。
func TestCommitRenamesAndUpdatesIndex(t *testing.T) {
	h := newCommitHarness(t)
	req := commitRequest(t)

	result, err := h.committer.Commit(req)
	if err != nil {
		t.Fatalf("Commit = %v", err)
	}
	if result.AlreadyInstalled {
		t.Error("新規installがAlreadyInstalledになった")
	}

	// 手順7: 完成先へ移動している。
	if _, statErr := h.fs.Stat(commitDestination + "/payload/bin/go"); statErr != nil {
		t.Errorf("完成先にpayloadが無い: %v", statErr)
	}
	// 手順6: receiptはversion directory直下（payloadの兄弟）である。
	if _, statErr := h.fs.Stat(commitDestination + "/" + commitReceiptName); statErr != nil {
		t.Errorf("完成先にreceiptが無い: %v", statErr)
	}
	// stagingのversion directoryは移動済みで残らない。
	if _, statErr := h.fs.Stat(commitVersionDir); statErr == nil {
		t.Error("staging側のversion directoryが残っている")
	}

	// 手順8: revisionが1つ進み、entryが入っている。
	if result.Index.Revision != req.Index.Revision+1 {
		t.Errorf("revision = %d, want %d", result.Index.Revision, req.Index.Revision+1)
	}
	if len(result.Index.Entries) != 1 {
		t.Fatalf("index entry = %d件, want 1", len(result.Index.Entries))
	}
	entry := result.Index.Entries[0]
	if entry.InstallID != req.Receipt.InstallID {
		t.Errorf("install ID = %q, want %q", entry.InstallID, req.Receipt.InstallID)
	}
	if entry.Health != domain.HealthHealthy {
		t.Errorf("health = %s, want healthy", entry.Health)
	}
	if len(entry.ReceiptSHA256) != 64 {
		t.Errorf("receipt_sha256 = %q（64 hexでない）", entry.ReceiptSHA256)
	}
}

// TestCommitTreatsIdenticalExistingAsSuccess は冪等installを固定する。
//
// docs/08-install-runtime.md §7「完成先が競合して作られた場合、両receiptと
// `command_targets`が完全一致すれば後発stagingを破棄して**成功**」。
func TestCommitTreatsIdenticalExistingAsSuccess(t *testing.T) {
	h := newCommitHarness(t)
	req := commitRequest(t)

	// 完成先に同一内容のreceiptを置く。install_idとinstalled_atは
	// 独立したinstallとして必ず異なる値にする。
	existing := conflictReceipt(t)
	existing.InstallID = strings.Repeat("7", 32)
	existing.InstalledAt = req.Receipt.InstalledAt.Add(-time.Hour)
	writeReceiptFile(t, h, existing)

	result, err := h.committer.Commit(req)
	if err != nil {
		t.Fatalf("同一内容の既存でCommit = %v", err)
	}
	if !result.AlreadyInstalled {
		t.Error("同一内容の既存をAlreadyInstalledにしていない")
	}
	// 既存を上書きしない。
	if _, statErr := h.fs.Stat(commitVersionDir); statErr != nil {
		t.Error("後発stagingが消えている（呼出し側のCleanupが行う）")
	}
}

// TestCommitRejectsDifferentExisting は内容差を`E_CONFLICT`にすることを固定する。
func TestCommitRejectsDifferentExisting(t *testing.T) {
	h := newCommitHarness(t)
	req := commitRequest(t)

	existing := conflictReceipt(t)
	existing.InstallID = strings.Repeat("7", 32)
	// 内容が違う。
	existing.CommandTargets[0].SHA256 = strings.Repeat("0", 64)
	writeReceiptFile(t, h, existing)

	_, err := h.committer.Commit(req)
	if err == nil {
		t.Fatal("内容の違う既存で成功した")
	}
	if err.Code != domain.CodeConflict {
		t.Errorf("code = %s, want %s", err.Code, domain.CodeConflict)
	}
	// 診断に不一致fieldが載ること。
	if unwrapped := err.Unwrap(); unwrapped == nil ||
		!strings.Contains(unwrapped.Error(), "command_targets") {
		t.Errorf("causeが不一致fieldを挙げていない: %v", unwrapped)
	}
}

// TestCommitRejectsUnreadableExisting は破損した既存を黙って上書きしないことを固定する。
//
// docs/04-storage-and-data.md §4「破損として扱い、黙って修復・再生成しない」。
func TestCommitRejectsUnreadableExisting(t *testing.T) {
	h := newCommitHarness(t)
	if err := h.fs.MkdirAll(commitDestination, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	h.fs.AddFile(commitDestination+"/"+commitReceiptName, []byte("not toml{{"), 0o600)

	_, err := h.committer.Commit(commitRequest(t))
	if err == nil {
		t.Fatal("破損receiptの既存で成功した")
	}
	if err.Code != domain.CodeConflict {
		t.Errorf("code = %s, want %s", err.Code, domain.CodeConflict)
	}
	// 完成先を消していないこと。
	if _, statErr := h.fs.Stat(commitDestination + "/" + commitReceiptName); statErr != nil {
		t.Error("破損receiptを消した")
	}
}

// TestCommitRejectsSymlinkInPayload は展開後に差し込まれたlinkを拒否することを固定する。
//
// §7手順1「staging payloadの全pathがroot内にあることを再検査する」。
// 展開側が同じ検査を済ませているが、**1箇所の検査に頼らない**。
func TestCommitRejectsSymlinkInPayload(t *testing.T) {
	tests := []struct {
		name string
		kind port.LinkKind
	}{
		{"symlink", port.LinkSymlink},
		// junctionはdirectoryとして報告される。
		{"junction", port.LinkJunction},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			h := newCommitHarness(t)
			h.fs.AddLink(commitPayloadDir+"/escape", test.kind, "/etc/passwd")

			_, err := h.committer.Commit(commitRequest(t))
			if err == nil {
				t.Fatal("payload内のlinkが通った")
			}
			if err.Code != domain.CodePathUnsafe {
				t.Errorf("code = %s, want %s", err.Code, domain.CodePathUnsafe)
			}
			// renameしていないこと。
			if _, statErr := h.fs.Stat(commitDestination); statErr == nil {
				t.Error("検査に失敗したのにrenameした")
			}
		})
	}
}

// TestCommitReportsFailures は各段の失敗注入を固定する。
func TestCommitReportsFailures(t *testing.T) {
	tests := []struct {
		name string
		op   string
		skip int
	}{
		// 手順6のreceipt書込み。
		{"receipt書込み", fake.OpAtomicWrite, 0},
		// 手順7のrename。
		{"rename", fake.OpRename, 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			h := newCommitHarness(t)
			h.inject.Fail(test.op, test.skip, 1, errors.New("注入した失敗"))
			if _, err := h.committer.Commit(commitRequest(t)); err == nil {
				t.Fatal("失敗注入で成功した")
			}
		})
	}
}

// TestCommitSucceedsInstallEvenIfIndexFails はrename後の失敗の扱いを固定する。
//
// §7「**手順7のrenameが完了した時点でinstallは成功とみなす**。rename後の中断は
// 導入成功でindexだけ古い状態であり、次回起動時の再構築で解消する」。
//
// **完成先を巻き戻さない。** index更新の失敗でpayloadを消すと、成功した導入を
// 失う。
func TestCommitSucceedsInstallEvenIfIndexFails(t *testing.T) {
	h := newCommitHarness(t)
	// receipt書込み(1回目)を通し、index書込み(2回目)を落とす。
	h.inject.Fail(fake.OpAtomicWrite, 1, 1, errors.New("注入したindex書込み失敗"))

	_, err := h.committer.Commit(commitRequest(t))
	if err == nil {
		t.Fatal("index更新失敗がerrorにならなかった")
	}
	// **renameは完了しており、完成先は残る。**
	if _, statErr := h.fs.Stat(commitDestination + "/payload/bin/go"); statErr != nil {
		t.Errorf("index失敗で完成先が巻き戻された: %v", statErr)
	}
}

// TestCommitUpsertsIndexEntry は同じtupleを二重に持たないことを固定する。
//
// docs/04-storage-and-data.md §13「tupleで一意・sort」。
func TestCommitUpsertsIndexEntry(t *testing.T) {
	h := newCommitHarness(t)
	req := commitRequest(t)
	// 同じtupleの古いentryを置く。
	req.Index.Entries = []store.ReceiptIndexEntry{{
		Ref:           req.Receipt.Ref,
		InstallID:     strings.Repeat("5", 32),
		Path:          "old/path",
		ReceiptSHA256: strings.Repeat("6", 64),
		Health:        domain.HealthUnknown,
	}}

	result, err := h.committer.Commit(req)
	if err != nil {
		t.Fatalf("Commit = %v", err)
	}
	if len(result.Index.Entries) != 1 {
		t.Fatalf("index entry = %d件, want 1（同じtupleは置換）", len(result.Index.Entries))
	}
	if result.Index.Entries[0].InstallID != req.Receipt.InstallID {
		t.Error("古いentryが残っている")
	}
}

// TestCommitRejectsInvalidRequest は前提違反を拒否することを固定する。
func TestCommitRejectsInvalidRequest(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*CommitRequest)
	}{
		{"staging payloadが未設定", func(r *CommitRequest) {
			r.StagingPayload = domain.PathValue{}
		}},
		{"staging payloadのroleが違う", func(r *CommitRequest) {
			r.StagingPayload = stagePathValue(t, domain.RoleStaging, commitPayloadDir)
		}},
		{"operation directoryが未設定", func(r *CommitRequest) {
			r.OperationDir = domain.PathValue{}
		}},
		{"完成先が未設定", func(r *CommitRequest) { r.Destination = domain.PathValue{} }},
		{"receipt名が未設定", func(r *CommitRequest) { r.ReceiptName = "" }},
		{"index pathが未設定", func(r *CommitRequest) { r.IndexPath = domain.PathValue{} }},
		{"index entry pathが未設定", func(r *CommitRequest) { r.IndexEntryPath = "" }},
		{"hostが未設定", func(r *CommitRequest) { r.Host = domain.Platform{} }},
		{"時刻が未設定", func(r *CommitRequest) { r.Now = time.Time{} }},
		{"install IDが未設定", func(r *CommitRequest) { r.Receipt.InstallID = "" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			h := newCommitHarness(t)
			req := commitRequest(t)
			test.mutate(&req)
			if _, err := h.committer.Commit(req); err == nil {
				t.Fatal("前提違反が通った")
			}
		})
	}
}

// TestNewCommitterRequiresFileSystem は依存不足を拒否することを固定する。
func TestNewCommitterRequiresFileSystem(t *testing.T) {
	if _, err := NewCommitter(nil); err == nil {
		t.Error("FileSystem無しで作れた")
	}
}

// writeReceiptFile は完成先へreceiptを置く。
func writeReceiptFile(t *testing.T, h *commitHarness, receipt store.Receipt) {
	t.Helper()
	data, err := store.EncodeReceipt(receipt)
	if err != nil {
		t.Fatalf("EncodeReceipt: %v", err)
	}
	if mkErr := h.fs.MkdirAll(commitDestination, 0o755); mkErr != nil {
		t.Fatalf("MkdirAll: %v", mkErr)
	}
	h.fs.AddFile(commitDestination+"/"+commitReceiptName, data, 0o600)
}
