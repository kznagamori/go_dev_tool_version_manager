package store

import (
	"strings"
	"testing"
	"time"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
)

func mustToolID(t *testing.T, text string) domain.ToolID {
	t.Helper()
	value, err := domain.ParseToolID(text)
	if err != nil {
		t.Fatalf("ParseToolID(%q) = %v", text, err)
	}
	return value
}

func mustPlatform(t *testing.T, id string) domain.Platform {
	t.Helper()
	value, err := domain.ParsePlatform(id)
	if err != nil {
		t.Fatalf("ParsePlatform(%q) = %v", id, err)
	}
	return value
}

func testTime() time.Time { return time.Date(2026, 8, 7, 9, 0, 0, 0, time.UTC) }

func testRef(t *testing.T, tool, version string) InstallRef {
	t.Helper()
	return InstallRef{
		Tool:     mustToolID(t, tool),
		Version:  version,
		Platform: mustPlatform(t, "linux-amd64-glibc"),
	}
}

// TestEncodeRejectsInvalidValue はencode側でも同じ検査が働くことを固定する。
//
// parse側だけを検査していると、programが組み立てた不正な値をそのままfileへ
// 書けてしまう。書いた瞬間はerrorにならず、次回の読込みで破損として現れるため、
// 原因の特定が難しくなる。書込み経路でも同じbuild関数を通す。
func TestEncodeRejectsInvalidValue(t *testing.T) {
	t.Run("StateSchema", func(t *testing.T) {
		valid := StateSchema{
			Revision: 1, RootID: testRootID, Mode: domain.ModeUser,
			CreatedAt: testTime(), UpdatedAt: testTime(), ClientVersion: "2026.08.07.00",
			StateSchema: 1, ReceiptSchema: 1, CatalogSchema: 1,
		}
		if _, err := EncodeStateSchema(valid); err != nil {
			t.Fatalf("正当な値が落ちた: %v", err)
		}
		invalid := []struct {
			name  string
			value StateSchema
		}{
			{"root_idが空", func() StateSchema { v := valid; v.RootID = ""; return v }()},
			{"modeがenum外", func() StateSchema { v := valid; v.Mode = "system"; return v }()},
			{"revisionが負", func() StateSchema { v := valid; v.Revision = -1; return v }()},
			{"client_versionが不正", func() StateSchema { v := valid; v.ClientVersion = "1.0"; return v }()},
			{"created_atがzero", func() StateSchema { v := valid; v.CreatedAt = time.Time{}; return v }()},
		}
		for _, test := range invalid {
			t.Run(test.name, func(t *testing.T) {
				if _, err := EncodeStateSchema(test.value); err == nil {
					t.Error("EncodeStateSchema = nil, want error")
				}
			})
		}
	})

	t.Run("SetupState", func(t *testing.T) {
		valid := SetupState{
			Revision: 3, RootID: testRootID, Mode: domain.ModeUser,
			PathIntegration: PathIntegrationNone, Shell: "", ShimPath: ShimDirectoryName,
			BackupID: testBackupID, UpdatedAt: testTime(),
			Identity: IntegrationIdentity{
				Kind: IntegrationNone, BeforeSHA256: zeroDigestHex, AfterSHA256: zeroDigestHex,
			},
		}
		if _, err := EncodeSetupState(valid); err != nil {
			t.Fatalf("正当な値が落ちた: %v", err)
		}
		invalid := []struct {
			name  string
			value SetupState
		}{
			{"shim_pathが他値", func() SetupState { v := valid; v.ShimPath = "bin"; return v }()},
			{"noneなのにshellがある", func() SetupState { v := valid; v.Shell = ShellBash; return v }()},
			{"noneなのにlocationがある", func() SetupState {
				v := valid
				v.Identity.Location = `HKCU\Environment`
				return v
			}()},
			{"digestが空", func() SetupState { v := valid; v.Identity.BeforeSHA256 = ""; return v }()},
		}
		for _, test := range invalid {
			t.Run(test.name, func(t *testing.T) {
				if _, err := EncodeSetupState(test.value); err == nil {
					t.Error("EncodeSetupState = nil, want error")
				}
			})
		}
	})

	t.Run("Selections", func(t *testing.T) {
		valid := Selections{
			Revision: 8, RootID: testRootID, UpdatedAt: testTime(),
			Entries: []Selection{{
				Ref: testRef(t, "node", "22.18.0"), InstallID: testInstall, SelectedAt: testTime(),
			}},
		}
		if _, err := EncodeSelections(valid); err != nil {
			t.Fatalf("正当な値が落ちた: %v", err)
		}
		invalid := []struct {
			name  string
			value Selections
		}{
			{"versionが空", func() Selections {
				v := valid
				v.Entries = []Selection{{Ref: testRef(t, "node", ""), InstallID: testInstall, SelectedAt: testTime()}}
				return v
			}()},
			{"install_idが不正", func() Selections {
				v := valid
				v.Entries = []Selection{{Ref: testRef(t, "node", "22.18.0"), InstallID: "x", SelectedAt: testTime()}}
				return v
			}()},
			{"同一toolが2件", func() Selections {
				v := valid
				v.Entries = []Selection{
					{Ref: testRef(t, "node", "22.18.0"), InstallID: testInstall, SelectedAt: testTime()},
					{Ref: testRef(t, "node", "20.0.0"), InstallID: testInstall, SelectedAt: testTime()},
				}
				return v
			}()},
		}
		for _, test := range invalid {
			t.Run(test.name, func(t *testing.T) {
				if _, err := EncodeSelections(test.value); err == nil {
					t.Error("EncodeSelections = nil, want error")
				}
			})
		}
	})

	t.Run("ShimIndex", func(t *testing.T) {
		valid := ShimIndex{
			Revision: 4, RootID: testRootID, ClientVersion: DevelopmentClientVersion,
			ReceiptIndexRevision: 5, UpdatedAt: testTime(),
			Commands: []ShimCommand{{Name: "node", ToolID: mustToolID(t, "node")}},
		}
		if _, err := EncodeShimIndex(valid); err != nil {
			t.Fatalf("正当な値が落ちた: %v", err)
		}
		duplicate := valid
		duplicate.Commands = []ShimCommand{
			{Name: "node", ToolID: mustToolID(t, "node")},
			{Name: "node", ToolID: mustToolID(t, "go")},
		}
		if _, err := EncodeShimIndex(duplicate); err == nil {
			t.Error("command名の重複が通った")
		}
		badName := valid
		badName.Commands = []ShimCommand{{Name: "Node", ToolID: mustToolID(t, "node")}}
		if _, err := EncodeShimIndex(badName); err == nil {
			t.Error("大文字のcommand名が通った")
		}
	})

	t.Run("ReceiptIndex", func(t *testing.T) {
		valid := ReceiptIndex{
			Revision: 5, RootID: testRootID, UpdatedAt: testTime(),
			Entries: []ReceiptIndexEntry{{
				Ref: testRef(t, "node", "22.18.0"), InstallID: testInstall,
				Path: "tools/node/.gdtvm-install.toml", ReceiptSHA256: testDigestB,
				Health: domain.HealthHealthy,
			}},
		}
		if _, err := EncodeReceiptIndex(valid); err != nil {
			t.Fatalf("正当な値が落ちた: %v", err)
		}
		absolute := valid
		absolute.Entries[0].Path = "/tools/node/.gdtvm-install.toml"
		if _, err := EncodeReceiptIndex(absolute); err == nil {
			t.Error("絶対pathが通った")
		}
	})

	t.Run("SetupBackup", func(t *testing.T) {
		valid := SetupBackup{
			BackupID: testBackupID, RootID: testRootID, Kind: BackupShellProfile,
			CreatedAt: testTime(), Target: "/home/u/.bashrc", Existed: false,
			SHA256: zeroDigestHex,
		}
		if _, err := EncodeSetupBackup(valid); err != nil {
			t.Fatalf("正当な値が落ちた: %v", err)
		}
		inconsistent := valid
		inconsistent.Raw = []byte("x")
		if _, err := EncodeSetupBackup(inconsistent); err == nil {
			t.Error("existed=falseなのにrawがある値が通った")
		}
	})
}

// TestEncodeSortsEntries は§7の「mapを永続化するときはkey順で出力する」を固定する。
//
// programが組み立てた順序に依存せず、同じ内容なら同じbyte列になる。順序が
// 揺れると、内容が変わっていないrevisionでもfileのdiffが出る。
func TestEncodeSortsEntries(t *testing.T) {
	unsorted := Selections{
		Revision: 1, RootID: testRootID, UpdatedAt: testTime(),
		Entries: []Selection{
			{Ref: testRef(t, "python", "3.13.7"), InstallID: testInstall, SelectedAt: testTime()},
			{Ref: testRef(t, "go", "1.25.0"), InstallID: testInstall, SelectedAt: testTime()},
			{Ref: testRef(t, "node", "22.18.0"), InstallID: testInstall, SelectedAt: testTime()},
		},
	}
	data, err := EncodeSelections(unsorted)
	if err != nil {
		t.Fatalf("EncodeSelections = %v", err)
	}
	text := string(data)
	positions := []int{
		indexOfValue(t, text, "tool_id", "go"),
		indexOfValue(t, text, "tool_id", "node"),
		indexOfValue(t, text, "tool_id", "python"),
	}
	for index := 1; index < len(positions); index++ {
		if positions[index-1] < 0 || positions[index] < positions[index-1] {
			t.Fatalf("tool ID順に出力されていない: %v\n%s", positions, text)
		}
	}
	// 出力は再parseできる。整列がfileの妥当性を壊していないことの確認。
	if _, parseErr := ParseSelections(data); parseErr != nil {
		t.Errorf("整列後の出力が再parseできない: %v", parseErr)
	}

	index := ReceiptIndex{
		Revision: 1, RootID: testRootID, UpdatedAt: testTime(),
		Entries: []ReceiptIndexEntry{
			{Ref: testRef(t, "node", "22.18.0"), InstallID: testInstall,
				Path: "b/.gdtvm-install.toml", ReceiptSHA256: testDigestB, Health: domain.HealthUnknown},
			{Ref: testRef(t, "node", "20.0.0"), InstallID: testInstall,
				Path: "a/.gdtvm-install.toml", ReceiptSHA256: testDigestB, Health: domain.HealthHealthy},
		},
	}
	indexData, indexErr := EncodeReceiptIndex(index)
	if indexErr != nil {
		t.Fatalf("EncodeReceiptIndex = %v", indexErr)
	}
	if indexOfValue(t, string(indexData), "version", "20.0.0") >
		indexOfValue(t, string(indexData), "version", "22.18.0") {
		t.Errorf("tupleのbyte順で出力されていない\n%s", indexData)
	}
}

// indexOfValue は`key = <value>`の出現位置を返す。
//
// TOML encoderがliteral string（`'`）とbasic string（`"`）のどちらを選ぶかは
// 値の内容で決まる。順序の検査で引用符の選択に依存しないよう、両方を試す。
func indexOfValue(t *testing.T, text, key, value string) int {
	t.Helper()
	for _, quote := range []string{"'", `"`} {
		if position := strings.Index(text, key+" = "+quote+value+quote); position >= 0 {
			return position
		}
	}
	t.Fatalf("%s = %s が出力に無い\n%s", key, value, text)
	return -1
}

// TestEncodeIsDeterministic は同じ値から同じbyte列が出ることを固定する。
func TestEncodeIsDeterministic(t *testing.T) {
	value := ShimIndex{
		Revision: 4, RootID: testRootID, ClientVersion: DevelopmentClientVersion,
		ReceiptIndexRevision: 5, UpdatedAt: testTime(),
		Commands: []ShimCommand{
			{Name: "npm", ToolID: mustToolID(t, "node")},
			{Name: "node", ToolID: mustToolID(t, "node")},
			{Name: "npx", ToolID: mustToolID(t, "node")},
		},
	}
	first, firstErr := EncodeShimIndex(value)
	if firstErr != nil {
		t.Fatalf("EncodeShimIndex = %v", firstErr)
	}
	for attempt := 0; attempt < 8; attempt++ {
		again, err := EncodeShimIndex(value)
		if err != nil {
			t.Fatalf("EncodeShimIndex = %v", err)
		}
		if string(again) != string(first) {
			t.Fatalf("出力が一致しない\n%s\n---\n%s", first, again)
		}
	}
}
