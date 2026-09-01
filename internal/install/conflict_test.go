package install

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/store"
)

// conflictReceipt は比較用のreceiptを返す。
//
// 全fieldへ非zero値を入れる。zero値だと「比較していない」と「一致した」を
// 区別できない。
func conflictReceipt(t *testing.T) store.Receipt {
	t.Helper()
	return store.Receipt{
		InstallID: strings.Repeat("1", 32),
		RootID:    strings.Repeat("2", 32),
		Ref: store.InstallRef{
			Tool:     testToolID(t),
			Version:  "1.25.0",
			Platform: platformOf(t, "linux-amd64-glibc"),
		},
		InstalledAt:      time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC),
		ClientVersion:    "2026.09.01.01",
		ClientCommit:     strings.Repeat("a", 40),
		DefinitionPath:   "tools/go.toml",
		DefinitionSHA256: strings.Repeat("b", 64),
		PayloadPath:      "payload",
		Artifact: store.ReceiptArtifact{
			ProviderKind:    store.ProviderOfficial,
			ProviderName:    "Go project",
			ProviderRelease: "go1.25.0",
			URL:             "https://go.dev/dl/go1.25.0.linux-amd64.tar.gz",
			File:            "go1.25.0.linux-amd64.tar.gz",
			Size:            1024,
			Digest:          testDigest(t),
			ChecksumSource:  store.ChecksumAssetField,
		},
		Storage: nil,
		Commands: []store.ReceiptCommand{{
			Name:               "go",
			Target:             "{{payload}}/bin/go",
			EnvironmentProfile: "default",
			WorkingDirectory:   store.WorkingInherit,
		}},
		EnvironmentProfiles: []store.ReceiptEnvironmentProfile{{ID: "default"}},
		Probes: []store.ReceiptProbe{{
			ID:              "go-version",
			RuntimeCommand:  "go",
			Args:            []string{"version"},
			Stream:          store.StreamStdout,
			Expect:          store.ExpectVersion,
			Regex:           `go(\d+\.\d+\.\d+)`,
			ExpectedVersion: "1.25.0",
			ReportedVersion: "1.25.0",
			TimeoutMillis:   30_000,
			Required:        true,
			Status:          store.ProbePassed,
			FinishedAt:      time.Date(2026, 9, 1, 10, 0, 5, 0, time.UTC),
		}},
		CommandTargets: []store.ReceiptCommandTarget{{
			Path: "payload/bin/go", Size: 2, SHA256: strings.Repeat("c", 64),
		}},
	}
}

// TestSameInstallIgnoresPerInstallIdentity は除外fieldを固定する。
//
// docs/08-install-runtime.md §7の一致判定から`install_id`、`installed_at`、
// `probes[].finished_at`だけを除く。**この3件は同一内容の導入でも定義上必ず
// 異なる値**であり、含めると「一致すれば成功」が到達不能なdead codeになる。
func TestSameInstallIgnoresPerInstallIdentity(t *testing.T) {
	left := conflictReceipt(t)
	right := conflictReceipt(t)
	// 独立した2つのinstallで必ず異なる値を入れる。
	right.InstallID = strings.Repeat("9", 32)
	right.InstalledAt = left.InstalledAt.Add(3 * time.Second)
	right.Probes[0].FinishedAt = left.Probes[0].FinishedAt.Add(time.Second)

	if !SameInstall(left, right) {
		t.Errorf("同一内容が不一致と判定された（不一致field: %s）",
			ConflictReason(left, right))
	}
	if reason := ConflictReason(left, right); reason != "" {
		t.Errorf("ConflictReason = %q, want 空", reason)
	}
}

// TestSameInstallDetectsContentDifference は内容差を検出することを固定する。
//
// **`client_version`と`client_commit`も比較に含む。** これらは「同一内容でも
// 必ず異なる」値ではなく、同じclientを2回動かせば同じ値になる。異なるclient版が
// 書いたreceiptは競合として表面化させる。
func TestSameInstallDetectsContentDifference(t *testing.T) {
	tests := []struct {
		field  string
		mutate func(*store.Receipt)
	}{
		{"root_id", func(r *store.Receipt) { r.RootID = strings.Repeat("8", 32) }},
		{"tool_id", func(r *store.Receipt) { r.Ref.Tool = domain.ToolID{} }},
		{"version", func(r *store.Receipt) { r.Ref.Version = "1.24.0" }},
		{"platform_id", func(r *store.Receipt) { r.Ref.Platform = domain.Platform{} }},
		{"client_version", func(r *store.Receipt) { r.ClientVersion = "2026.01.01.01" }},
		{"client_commit", func(r *store.Receipt) { r.ClientCommit = strings.Repeat("f", 40) }},
		{"definition_path", func(r *store.Receipt) { r.DefinitionPath = "tools/node.toml" }},
		{"definition_sha256", func(r *store.Receipt) {
			r.DefinitionSHA256 = strings.Repeat("e", 64)
		}},
		{"payload_path", func(r *store.Receipt) { r.PayloadPath = "other" }},
		{"artifact", func(r *store.Receipt) { r.Artifact.ProviderName = "someone else" }},
		{"storage", func(r *store.Receipt) {
			r.Storage = []store.ReceiptStorage{{ID: "cache"}}
		}},
		{"commands", func(r *store.Receipt) { r.Commands[0].Name = "gofmt" }},
		{"environment_profiles", func(r *store.Receipt) {
			r.EnvironmentProfiles[0].ID = "other"
		}},
		{"command_targets", func(r *store.Receipt) {
			r.CommandTargets[0].SHA256 = strings.Repeat("d", 64)
		}},
		// probeは終了時刻**以外**が違えば不一致である。
		{"probes", func(r *store.Receipt) { r.Probes[0].Status = store.ProbeSkipped }},
	}
	for _, test := range tests {
		t.Run(test.field, func(t *testing.T) {
			left := conflictReceipt(t)
			right := conflictReceipt(t)
			test.mutate(&right)

			if SameInstall(left, right) {
				t.Fatalf("%s が違うのに同一と判定された", test.field)
			}
			if reason := ConflictReason(left, right); reason != test.field {
				t.Errorf("ConflictReason = %q, want %q", reason, test.field)
			}
			// 診断に不一致fieldが載ること。
			if err := ConflictError(left, right); err == nil ||
				!strings.Contains(err.Error(), test.field) {
				t.Errorf("ConflictErrorが不一致fieldを挙げていない: %v", err)
			}
		})
	}
}

// TestConflictReasonCoversEveryField は比較表がreceiptのfieldを網羅することを固定する。
//
// **取りこぼしたfieldは常に「一致」と判定される**ため、違う導入を同一と誤認する。
// receiptへfieldが増えたらここが落ち、比較表の更新を促す。
func TestConflictReasonCoversEveryField(t *testing.T) {
	// receiptのtop-level field数。
	const receiptFields = 15
	got := reflect.TypeOf(store.Receipt{}).NumField()
	if got != receiptFields {
		t.Fatalf("Receiptのfieldが%d件、want %d件。"+
			"ConflictReasonの比較表とcomparableReceiptの除外を見直す", got, receiptFields)
	}

	// 比較表の件数。top-level 15件から除外2件（`install_id`、`installed_at`）を
	// 引き、`Ref`を3件（tool/version/platform）へ展開して 15 - 2 - 1 + 3 = 15。
	// `probes[].finished_at`はtop-levelではなくprobe内の除外である。
	const compared = 15
	left := conflictReceipt(t)
	seen := make(map[string]struct{}, compared)
	mutations := []func(*store.Receipt){
		func(r *store.Receipt) { r.RootID = "x" },
		func(r *store.Receipt) { r.Ref.Tool = domain.ToolID{} },
		func(r *store.Receipt) { r.Ref.Version = "x" },
		func(r *store.Receipt) { r.Ref.Platform = domain.Platform{} },
		func(r *store.Receipt) { r.ClientVersion = "x" },
		func(r *store.Receipt) { r.ClientCommit = "x" },
		func(r *store.Receipt) { r.DefinitionPath = "x" },
		func(r *store.Receipt) { r.DefinitionSHA256 = "x" },
		func(r *store.Receipt) { r.PayloadPath = "x" },
		func(r *store.Receipt) { r.Artifact.ProviderName = "x" },
		func(r *store.Receipt) { r.Storage = []store.ReceiptStorage{{ID: "x"}} },
		func(r *store.Receipt) { r.Commands[0].Name = "x" },
		func(r *store.Receipt) { r.EnvironmentProfiles[0].ID = "x" },
		func(r *store.Receipt) { r.CommandTargets[0].Path = "x" },
		func(r *store.Receipt) { r.Probes[0].ID = "x" },
	}
	if len(mutations) != compared {
		t.Fatalf("mutationが%d件、want %d件", len(mutations), compared)
	}
	for _, mutate := range mutations {
		right := conflictReceipt(t)
		mutate(&right)
		reason := ConflictReason(left, right)
		if reason == "" || reason == "unknown" {
			t.Errorf("比較表が拾えないfieldがある（reason=%q）", reason)
			continue
		}
		seen[reason] = struct{}{}
	}
	if len(seen) != compared {
		t.Errorf("比較表が区別したfield = %d件, want %d件", len(seen), compared)
	}
}

// TestSameInstallHandlesNilProbes はprobe無しreceiptを扱えることを固定する。
func TestSameInstallHandlesNilProbes(t *testing.T) {
	left := conflictReceipt(t)
	left.Probes = nil
	right := conflictReceipt(t)
	right.Probes = nil
	if !SameInstall(left, right) {
		t.Error("probe無しの同一receiptが不一致になった")
	}
	// 片方だけprobeを持てば不一致である。
	right = conflictReceipt(t)
	if SameInstall(left, right) {
		t.Error("probeの有無が無視された")
	}
}

// TestSameInstallSurvivesCodecRoundTrip はTOML往復後も同一と判定されることを固定する。
//
// **これが実際の比較の形である。** 完成先のreceiptはdiskから読んだもの、
// 比較相手は今memoryで組み立てたものであり、両者は同じ経路を通っていない。
// TOMLを往復すると空arrayは長さ0のsliceになるが、memory側はnilを持つ。
// 区別すると同一内容でも常に不一致になり、§7の「一致すれば成功」が実際には
// 到達しない。
func TestSameInstallSurvivesCodecRoundTrip(t *testing.T) {
	original := conflictReceipt(t)
	data, err := store.EncodeReceipt(original)
	if err != nil {
		t.Fatalf("EncodeReceipt: %v", err)
	}
	parsed, parseErr := store.ParseReceipt(data)
	if parseErr != nil {
		t.Fatalf("ParseReceipt: %v", parseErr)
	}
	// 独立したinstallとして必ず異なる値を入れる。
	parsed.InstallID = strings.Repeat("7", 32)
	parsed.InstalledAt = original.InstalledAt.Add(time.Hour)

	if !SameInstall(original, parsed) {
		t.Errorf("往復後に不一致と判定された（不一致field: %s）",
			ConflictReason(original, parsed))
	}
	// 内容が違えばやはり検出する。
	parsed.CommandTargets[0].SHA256 = strings.Repeat("0", 64)
	if SameInstall(original, parsed) {
		t.Error("往復後に内容差を見逃した")
	}
}
