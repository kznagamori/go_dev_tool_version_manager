package install

import (
	"testing"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/definition"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/store"
)

// TestEnumTablesCoverDefinition は変換表がdefinition側の全値を持つことを固定する。
//
// `internal/definition`と`internal/store`は同じ値集合を別の型で持つ。変換表が
// 片方の値を落とすと、その値を使うtoolのPlanだけが作れなくなる。件数と値の
// 両方をここで固定し、どちらかへ値が増えたら落ちるようにする。
//
// **string castにしない理由がここにある。** castなら値が増えても素通りし、
// storeが知らない値を持つPlanを黙って作れてしまう。
func TestEnumTablesCoverDefinition(t *testing.T) {
	t.Run("artifact kind", func(t *testing.T) {
		want := []definition.ArtifactKind{
			definition.KindOfficial, definition.KindThirdParty,
		}
		assertTableCovers(t, providerKinds, want)
	})
	t.Run("archive format", func(t *testing.T) {
		want := []definition.ArchiveFormat{definition.FormatZip, definition.FormatTarGz}
		assertTableCovers(t, archiveFormats, want)
	})
	t.Run("storage kind", func(t *testing.T) {
		want := []definition.StorageKind{
			definition.StorageConfig, definition.StorageContentCache,
			definition.StorageBuildCache, definition.StorageGlobalBin,
			definition.StorageGlobalPackages, definition.StorageRuntimeData,
		}
		assertTableCovers(t, storageKinds, want)
	})
	t.Run("storage scope", func(t *testing.T) {
		want := []definition.StorageScope{definition.ScopeTool, definition.ScopeVersion}
		assertTableCovers(t, storageScopes, want)
	})
	t.Run("storage purge", func(t *testing.T) {
		want := []definition.StoragePurge{
			definition.StorageRetain, definition.StorageExplicit,
			definition.StorageWithVersion,
		}
		assertTableCovers(t, storagePurges, want)
	})
	t.Run("probe stream", func(t *testing.T) {
		want := []definition.ProbeStream{
			definition.StreamStdout, definition.StreamStderr, definition.StreamCombined,
		}
		assertTableCovers(t, probeStreams, want)
	})
	t.Run("probe expect", func(t *testing.T) {
		want := []definition.ProbeExpect{
			definition.ExpectVersion, definition.ExpectSuccess,
			definition.ExpectPathWithin,
		}
		assertTableCovers(t, probeExpects, want)
	})
	t.Run("required path kind", func(t *testing.T) {
		want := []definition.RequiredPathKind{
			definition.RequiredFile, definition.RequiredDirectory,
		}
		assertTableCovers(t, requiredPathKinds, want)
	})
}

// assertTableCovers は変換表がwantの全値を持ち、余分を持たないことを確かめる。
func assertTableCovers[K comparable, V any](t *testing.T, table map[K]V, want []K) {
	t.Helper()
	if len(table) != len(want) {
		t.Fatalf("表の件数 = %d, want %d", len(table), len(want))
	}
	for _, key := range want {
		if _, ok := table[key]; !ok {
			t.Errorf("表に %v が無い", key)
		}
	}
}

// TestEnumTablesPreserveValue は変換が値を変えないことを固定する。
//
// 型は違うが文字列値は同じである。ここが崩れると、Planのenumがschemaと
// 合わなくなる。
func TestEnumTablesPreserveValue(t *testing.T) {
	for key, value := range providerKinds {
		if string(key) != string(value) {
			t.Errorf("artifact kind %q -> %q", key, value)
		}
	}
	for key, value := range archiveFormats {
		if string(key) != string(value) {
			t.Errorf("archive format %q -> %q", key, value)
		}
	}
	for key, value := range storageKinds {
		if string(key) != string(value) {
			t.Errorf("storage kind %q -> %q", key, value)
		}
	}
	for key, value := range storageScopes {
		if string(key) != string(value) {
			t.Errorf("storage scope %q -> %q", key, value)
		}
	}
	for key, value := range storagePurges {
		if string(key) != string(value) {
			t.Errorf("storage purge %q -> %q", key, value)
		}
	}
	for key, value := range probeStreams {
		if string(key) != string(value) {
			t.Errorf("probe stream %q -> %q", key, value)
		}
	}
	for key, value := range probeExpects {
		if string(key) != string(value) {
			t.Errorf("probe expect %q -> %q", key, value)
		}
	}
	for key, value := range requiredPathKinds {
		if string(key) != string(value) {
			t.Errorf("required path kind %q -> %q", key, value)
		}
	}
}

// TestConvertRejectsUnknownValue は未知値をerrorにすることを固定する。
//
// 素通りさせると、storeが知らないenum値を持つPlanができる。
func TestConvertRejectsUnknownValue(t *testing.T) {
	if _, err := convertProviderKind(""); err == nil {
		t.Error("未設定のartifact kindが通った")
	}
	if _, err := convertProviderKind(definition.ArtifactKind("vendor")); err == nil {
		t.Error("未知のartifact kindが通った")
	}
	if _, err := convertArchiveFormat(definition.ArchiveFormat("7z")); err == nil {
		t.Error("未知のarchive formatが通った")
	}
	if _, err := convertStorageKind(definition.StorageKind("scratch")); err == nil {
		t.Error("未知のstorage kindが通った")
	}
	if _, err := convertStorageScope(definition.StorageScope("global")); err == nil {
		t.Error("未知のstorage scopeが通った")
	}
	if _, err := convertStoragePurge(definition.StoragePurge("always")); err == nil {
		t.Error("未知のstorage purgeが通った")
	}
	if _, err := convertProbeStream(definition.ProbeStream("both")); err == nil {
		t.Error("未知のprobe streamが通った")
	}
	if _, err := convertProbeExpect(definition.ProbeExpect("exit")); err == nil {
		t.Error("未知のprobe expectが通った")
	}
	if _, err := convertRequiredPathKind(definition.RequiredPathKind("socket")); err == nil {
		t.Error("未知のrequired path kindが通った")
	}
	// 空文字列も未知値である。zero値のdefinitionを渡したときに、
	// 最初のenum値へ落ちないことを見る。
	if _, err := convertStorageKind(""); err == nil {
		t.Error("空のstorage kindが通った")
	}
}

// TestPlanWarningApprovalComesFromTable は承認要否が§16.1の表から来ることを固定する。
//
// 作成側が真偽を選べると、同じcodeが場面によって承認要否を変えられる。
func TestPlanWarningApprovalComesFromTable(t *testing.T) {
	tests := []struct {
		code store.PlanWarningCode
		want bool
	}{
		{store.WarnThirdParty, true},
		{store.WarnRestrictiveLicense, true},
		{store.WarnPrerelease, true},
		{store.WarnEOL, true},
		{store.WarnDestructive, true},
		{store.WarnShellModification, true},
		{store.WarnModeChange, true},
		// §16.1「`W_RESTART_REQUIRED`は情報提供であり承認の対象にしない」。
		{store.WarnRestartRequired, false},
	}
	if len(tests) != store.PlanWarningCodeCount {
		t.Fatalf("caseが%d件、want %d件", len(tests), store.PlanWarningCodeCount)
	}
	approvals := 0
	for _, test := range tests {
		warning := store.NewPlanWarning(test.code, testMessageID(t), nil)
		if warning.RequiresExplicitApproval != test.want {
			t.Errorf("%s の承認要否 = %v, want %v",
				test.code, warning.RequiresExplicitApproval, test.want)
		}
		if warning.RequiresExplicitApproval {
			approvals++
		}
	}
	if approvals != store.PlanApprovalCodeCount {
		t.Errorf("承認対象 = %d件, want %d件", approvals, store.PlanApprovalCodeCount)
	}
}
