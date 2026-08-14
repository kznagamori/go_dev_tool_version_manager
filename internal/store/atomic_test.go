package store

import (
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain/port"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain/port/fake"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/security"
)

const testStatePath = "/data/gdtvm/state/schema.toml"

func statePathValue(t *testing.T) domain.PathValue {
	t.Helper()
	value, err := domain.NewPathValue(domain.RoleState, testStatePath)
	if err != nil {
		t.Fatalf("NewPathValue = %v", err)
	}
	return value
}

// parseSchema はstrict再parseとして渡す関数である。
func parseSchema(data []byte) error {
	if _, err := ParseStateSchema(data); err != nil {
		return err
	}
	return nil
}

// schemaWithRevision は指定revisionの`state/schema.toml`を作る。
func schemaWithRevision(t *testing.T, revision int64) []byte {
	t.Helper()
	value, err := ParseStateSchema([]byte(specSchemaTOML))
	if err != nil {
		t.Fatalf("ParseStateSchema = %s", describe(err))
	}
	value.Revision = revision
	data, encodeErr := EncodeStateSchema(value)
	if encodeErr != nil {
		t.Fatalf("EncodeStateSchema = %s", describe(encodeErr))
	}
	return data
}

// newStateSet は親directoryを作ったfake setを返す。
//
// 親directoryの作成は呼出し側（setupのroot layout生成）の責務であり、
// [WriteState]は行わない。
func newStateSet(t *testing.T) *fake.Set {
	t.Helper()
	set := fake.NewSet()
	if err := set.FileSystem.MkdirAll("/data/gdtvm/state", 0o700); err != nil {
		t.Fatalf("MkdirAll = %v", err)
	}
	return set
}

func newWriteRequest(t *testing.T, set *fake.Set, data []byte) StateWriteRequest {
	t.Helper()
	return StateWriteRequest{
		Path: statePathValue(t), Data: data, Parse: parseSchema,
		RootID: testRootID, Backup: true, FileSystem: set.FileSystem,
	}
}

// TestNextRevisionFollowsSpec は§4 step 2の「next=current+1（新規は1）」を固定する。
func TestNextRevisionFollowsSpec(t *testing.T) {
	tests := []struct{ current, want int64 }{
		{0, 1}, {1, 2}, {8, 9}, {-1, 1},
	}
	for _, test := range tests {
		if got := NextRevision(test.current); got != test.want {
			t.Errorf("NextRevision(%d) = %d, want %d", test.current, got, test.want)
		}
	}
}

// TestWriteStateCreatesNewFile は新規fileの書込みを固定する。
//
// 既存fileが無ければ`.bak`は作らない。存在しない内容をbackupすると、
// 復元候補として空のstateが残る。
func TestWriteStateCreatesNewFile(t *testing.T) {
	set := newStateSet(t)
	data := schemaWithRevision(t, 1)
	result, err := WriteState(newWriteRequest(t, set, data))
	if err != nil {
		t.Fatalf("WriteState = %s", describe(err))
	}
	if result.BackupWritten {
		t.Error("新規fileなのに.bakを書いた")
	}
	if result.SHA256 != security.SHA256Hex(data) {
		t.Errorf("SHA256 = %q", result.SHA256)
	}
	published, readErr := set.FileSystem.ReadFile(testStatePath, StateFileMaxBytes)
	if readErr != nil {
		t.Fatalf("ReadFile = %v", readErr)
	}
	if string(published) != string(data) {
		t.Error("公開fileの内容が一致しない")
	}
}

// TestWriteStateKeepsOneBackupGeneration は§4 step 5の1世代保持を固定する。
func TestWriteStateKeepsOneBackupGeneration(t *testing.T) {
	set := newStateSet(t)
	first := schemaWithRevision(t, 1)
	if _, err := WriteState(newWriteRequest(t, set, first)); err != nil {
		t.Fatalf("1回目 = %s", describe(err))
	}
	second := schemaWithRevision(t, 2)
	result, err := WriteState(newWriteRequest(t, set, second))
	if err != nil {
		t.Fatalf("2回目 = %s", describe(err))
	}
	if !result.BackupWritten {
		t.Error("既存fileがあるのに.bakを書いていない")
	}
	backup, readErr := set.FileSystem.ReadFile(testStatePath+BackupSuffix, StateFileMaxBytes)
	if readErr != nil {
		t.Fatalf("backup読込み = %v", readErr)
	}
	// .bakは**1つ前**の内容である。最新1世代だけを保つ。
	if string(backup) != string(first) {
		t.Error(".bakが1つ前の内容でない")
	}

	third := schemaWithRevision(t, 3)
	if _, err := WriteState(newWriteRequest(t, set, third)); err != nil {
		t.Fatalf("3回目 = %s", describe(err))
	}
	backup, _ = set.FileSystem.ReadFile(testStatePath+BackupSuffix, StateFileMaxBytes)
	if string(backup) != string(second) {
		t.Error("3回目の.bakが2回目の内容でない")
	}
	// 2世代前のfileは残らない。
	if _, err := set.FileSystem.ReadFile(testStatePath+BackupSuffix+BackupSuffix, StateFileMaxBytes); err == nil {
		t.Error("2世代目のbackupが作られている")
	}
}

// TestWriteStateRejectsUnparsableContent は§4 step 4のstrict再parseを固定する。
//
// 公開前に止めないと、次回の読込みで破損として現れて原因の特定が難しくなる。
func TestWriteStateRejectsUnparsableContent(t *testing.T) {
	set := newStateSet(t)
	request := newWriteRequest(t, set, []byte("schema = 2\n"))
	if _, err := WriteState(request); err == nil {
		t.Fatal("strict parseできない内容が通った")
	}
	// 公開fileを作っていない。検証前に書き始めない。
	if _, err := set.FileSystem.ReadFile(testStatePath, StateFileMaxBytes); err == nil {
		t.Error("拒否したのに公開fileが作られている")
	}
}

// TestWriteStateBackupPrecedesReplace は§4の「旧fileを失った状態で新fileを
// 書き始めない」を固定する。
//
// 公開fileのreplaceが失敗しても、`.bak`は既に書かれていて旧内容が残る。
func TestWriteStateBackupPrecedesReplace(t *testing.T) {
	set := newStateSet(t)
	first := schemaWithRevision(t, 1)
	if _, err := WriteState(newWriteRequest(t, set, first)); err != nil {
		t.Fatalf("1回目 = %s", describe(err))
	}
	// `.bak`の書込みは通し、公開fileのreplaceだけ失敗させる。
	set.Injector.Fail(fake.OpAtomicWrite, 1, 1, errors.New("replace失敗"))
	second := schemaWithRevision(t, 2)
	if _, err := WriteState(newWriteRequest(t, set, second)); err == nil {
		t.Fatal("replace失敗が成功として返った")
	}
	backup, readErr := set.FileSystem.ReadFile(testStatePath+BackupSuffix, StateFileMaxBytes)
	if readErr != nil {
		t.Fatalf("backupが残っていない: %v", readErr)
	}
	if string(backup) != string(first) {
		t.Error("replace失敗後に旧内容が失われた")
	}
}

// TestWriteStateRollsBackOnVerifyFailure は§4 step 7のrollbackを固定する。
func TestWriteStateRollsBackOnVerifyFailure(t *testing.T) {
	set := newStateSet(t)
	first := schemaWithRevision(t, 1)
	if _, err := WriteState(newWriteRequest(t, set, first)); err != nil {
		t.Fatalf("1回目 = %s", describe(err))
	}
	// 検証のためのReadFileだけを失敗させる（.bak読込みは成功させる）。
	set.Injector.Fail(fake.OpReadFile, 1, 1, errors.New("verify read失敗"))
	second := schemaWithRevision(t, 2)
	result, err := WriteState(newWriteRequest(t, set, second))
	if err == nil {
		t.Fatal("検証失敗が成功として返った")
	}
	if !result.RolledBack {
		t.Fatal("検証失敗なのにrollbackしていない")
	}
	// 公開fileが1つ前の内容へ戻っている。
	published, readErr := set.FileSystem.ReadFile(testStatePath, StateFileMaxBytes)
	if readErr != nil {
		t.Fatalf("ReadFile = %v", readErr)
	}
	if string(published) != string(first) {
		t.Error("rollback後の内容が1つ前でない")
	}
}

// TestRestoreFromBackupChecksCandidate は§4の復元候補条件を固定する。
//
// 「strict parse/digest/root IDが一致する場合だけ復元候補にする」。壊れた
// backupで上書きすると破損を1世代分広げる。
func TestRestoreFromBackupChecksCandidate(t *testing.T) {
	setup := func(t *testing.T, backup []byte) *fake.Set {
		t.Helper()
		set := newStateSet(t)
		if err := set.FileSystem.AtomicWrite(testStatePath, []byte("broken"), StateFilePerm); err != nil {
			t.Fatalf("AtomicWrite = %v", err)
		}
		if backup != nil {
			if err := set.FileSystem.AtomicWrite(
				testStatePath+BackupSuffix, backup, StateFilePerm); err != nil {
				t.Fatalf("AtomicWrite = %v", err)
			}
		}
		return set
	}

	// 妥当なbackupからは復旧できる。
	valid := schemaWithRevision(t, 7)
	set := setup(t, valid)
	digest, err := RestoreFromBackup(RestoreRequest{
		Path: statePathValue(t), Parse: parseSchema, RootID: testRootID, FileSystem: set.FileSystem,
	})
	if err != nil {
		t.Fatalf("RestoreFromBackup = %s", describe(err))
	}
	if digest != security.SHA256Hex(valid) {
		t.Errorf("digest = %q", digest)
	}
	restored, _ := set.FileSystem.ReadFile(testStatePath, StateFileMaxBytes)
	if string(restored) != string(valid) {
		t.Error("復旧後の内容がbackupと一致しない")
	}

	rejects := []struct {
		name   string
		backup []byte
		rootID string
	}{
		{"backupが無い", nil, testRootID},
		{"backupがstrict parseできない", []byte("schema = 2\n"), testRootID},
		{"root IDが一致しない", valid, "ffffffffffffffffffffffffffffffff"},
	}
	for _, test := range rejects {
		t.Run(test.name, func(t *testing.T) {
			set := setup(t, test.backup)
			if _, err := RestoreFromBackup(RestoreRequest{
				Path: statePathValue(t), Parse: parseSchema,
				RootID: test.rootID, FileSystem: set.FileSystem,
			}); err == nil {
				t.Error("RestoreFromBackup = nil, want error")
			}
			// 復旧しなかった場合は公開fileを触らない。
			published, _ := set.FileSystem.ReadFile(testStatePath, StateFileMaxBytes)
			if string(published) != "broken" {
				t.Error("復旧しなかったのに公開fileを書き換えた")
			}
		})
	}
}

// TestWriteStateSkipsBackupWhenDisabled は§4の対象外fileを固定する。
//
// 「receipt、catalog、再構築可能indexはこのgeneric backup対象外」。
func TestWriteStateSkipsBackupWhenDisabled(t *testing.T) {
	set := newStateSet(t)
	first := schemaWithRevision(t, 1)
	request := newWriteRequest(t, set, first)
	request.Backup = false
	request.RootID = ""
	if _, err := WriteState(request); err != nil {
		t.Fatalf("1回目 = %s", describe(err))
	}
	second := schemaWithRevision(t, 2)
	request.Data = second
	if _, err := WriteState(request); err != nil {
		t.Fatalf("2回目 = %s", describe(err))
	}
	if _, err := set.FileSystem.ReadFile(testStatePath+BackupSuffix, StateFileMaxBytes); err == nil {
		t.Error("Backup=falseなのに.bakが作られている")
	}
}

// TestWriteStateRejectsInvalidRequest はrequest不備を固定する。
func TestWriteStateRejectsInvalidRequest(t *testing.T) {
	set := newStateSet(t)
	data := schemaWithRevision(t, 1)
	tests := []struct {
		name   string
		mutate func(*StateWriteRequest)
	}{
		{"FileSystem未設定", func(r *StateWriteRequest) { r.FileSystem = nil }},
		{"Parse未設定", func(r *StateWriteRequest) { r.Parse = nil }},
		{"path未設定", func(r *StateWriteRequest) { r.Path = domain.PathValue{} }},
		{"dataが空", func(r *StateWriteRequest) { r.Data = nil }},
		{"Backup=trueでroot ID未設定", func(r *StateWriteRequest) { r.RootID = "" }},
		{"dataが上限超過", func(r *StateWriteRequest) {
			r.Data = []byte(strings.Repeat("x", StateFileMaxBytes+1))
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := newWriteRequest(t, set, data)
			test.mutate(&request)
			if _, err := WriteState(request); err == nil {
				t.Error("WriteState = nil, want error")
			}
		})
	}
}

// tamperingFileSystem は指定回目のReadFileだけ内容を差し替えるtest doubleである。
//
// fakeのReadFileは書いた内容をそのまま返すため、§4 step 7のdigest照合が
// 不一致になる状況をfakeだけでは作れない。他processが公開file直後に書き換えた
// 場合を、この層で決定的に再現する。
type tamperingFileSystem struct {
	port.FileSystem
	target    string
	skip      int
	tampered  []byte
	callCount int
}

func (f *tamperingFileSystem) ReadFile(path string, limit int64) ([]byte, error) {
	data, err := f.FileSystem.ReadFile(path, limit)
	if err != nil || path != f.target {
		return data, err
	}
	f.callCount++
	if f.callCount > f.skip {
		return f.tampered, nil
	}
	return data, nil
}

// TestWriteStateRollsBackOnDigestMismatch は§4 step 7のdigest照合を固定する。
//
// 「公開fileを再読してexpected digest/revisionと一致させる。不一致なら検証済み
// backupへrollbackする」。revisionはdataの一部であるため、byte単位のdigestが
// 一致すればrevisionも一致する。digestだけを比べれば両方を満たす。
func TestWriteStateRollsBackOnDigestMismatch(t *testing.T) {
	set := newStateSet(t)
	first := schemaWithRevision(t, 1)
	if _, err := WriteState(newWriteRequest(t, set, first)); err != nil {
		t.Fatalf("1回目 = %s", describe(err))
	}
	// 2回目の書込みで、既存file読込み（step 5）は素通しし、検証の再読
	// （step 7）だけ別revisionの内容へ差し替える。
	tampered := schemaWithRevision(t, 99)
	wrapped := &tamperingFileSystem{
		FileSystem: set.FileSystem, target: testStatePath, skip: 1, tampered: tampered,
	}
	request := newWriteRequest(t, set, schemaWithRevision(t, 2))
	request.FileSystem = wrapped

	result, err := WriteState(request)
	if err == nil {
		t.Fatal("digest不一致が成功として返った")
	}
	if !result.RolledBack {
		t.Fatal("digest不一致なのにrollbackしていない")
	}
	if result.SHA256 != "" {
		t.Errorf("失敗したのにSHA256を返した: %q", result.SHA256)
	}
	published, readErr := set.FileSystem.ReadFile(testStatePath, StateFileMaxBytes)
	if readErr != nil {
		t.Fatalf("ReadFile = %v", readErr)
	}
	if string(published) != string(first) {
		t.Error("rollback後の内容が1つ前でない")
	}
}

// TestWriteStateRollsBackOnVerifyParseFailure は§4 step 7の再parseを固定する。
//
// digestが一致しても、公開fileがstrict parseできなければ復旧対象である。
func TestWriteStateRollsBackOnVerifyParseFailure(t *testing.T) {
	set := newStateSet(t)
	first := schemaWithRevision(t, 1)
	if _, err := WriteState(newWriteRequest(t, set, first)); err != nil {
		t.Fatalf("1回目 = %s", describe(err))
	}
	second := schemaWithRevision(t, 2)
	request := newWriteRequest(t, set, second)
	// digest照合は通し、再parseだけ落ちるようにする。Parseは1回目がstep 4の
	// 書込み前検査、2回目がstep 7の検証、3回目がrollbackの復元候補判定である。
	// 2回目だけ失敗させ、backupは復元候補として通す。
	calls := 0
	request.Parse = func(data []byte) error {
		calls++
		if calls == 2 {
			return errors.New("再parse失敗")
		}
		return parseSchema(data)
	}

	result, err := WriteState(request)
	if err == nil {
		t.Fatal("再parse失敗が成功として返った")
	}
	if !result.RolledBack {
		t.Fatal("再parse失敗なのにrollbackしていない")
	}
	published, _ := set.FileSystem.ReadFile(testStatePath, StateFileMaxBytes)
	if string(published) != string(first) {
		t.Error("rollback後の内容が1つ前でない")
	}
}

// TestWriteStateKeepsPublishedFileWholeInParallel は並行書込みでも公開fileが
// 常に完全な1文書であることを固定する。
//
// §4 step 1のlockは更新の**順序**を決めるためのもので、本関数はそれを取らない。
// lockを取らずに並べても、公開fileが途中まで書かれた内容になってはならない。
// [port.FileSystem.AtomicWrite] が「旧内容のまま残るか、まったく存在しないかの
// どちらか」を保証することへの依存をここで固定する。
func TestWriteStateKeepsPublishedFileWholeInParallel(t *testing.T) {
	const workers = 8
	set := newStateSet(t)
	requests := make([]StateWriteRequest, workers)
	for index := range requests {
		requests[index] = newWriteRequest(t, set, schemaWithRevision(t, int64(index+1)))
	}

	var group sync.WaitGroup
	failures := make([]*domain.Error, workers)
	group.Add(workers)
	for index := 0; index < workers; index++ {
		go func(index int) {
			defer group.Done()
			_, failures[index] = WriteState(requests[index])
		}(index)
	}
	group.Wait()

	// 検証の再読が他workerの書込みとすれ違うと、digest不一致でrollbackする。
	// これはlockを取らないことの帰結であり、想定内の失敗である。壊れた内容が
	// 公開されていないことだけを確かめる。
	for index, err := range failures {
		if err != nil && err.Code != "E_STATE_CORRUPT" {
			t.Errorf("worker %d = %s", index, describe(err))
		}
	}
	published, readErr := set.FileSystem.ReadFile(testStatePath, StateFileMaxBytes)
	if readErr != nil {
		t.Fatalf("ReadFile = %v", readErr)
	}
	value, parseErr := ParseStateSchema(published)
	if parseErr != nil {
		t.Fatalf("公開fileが完全な1文書でない: %s\n%s", describe(parseErr), published)
	}
	if value.Revision < 1 || value.Revision > workers {
		t.Errorf("revision = %d, want 1〜%d", value.Revision, workers)
	}
}

// TestWriteStateErrorCarriesRoleWithoutPath はdocs/10-security.md §9.2を固定する。
func TestWriteStateErrorCarriesRoleWithoutPath(t *testing.T) {
	set := newStateSet(t)
	_, err := WriteState(newWriteRequest(t, set, []byte("schema = 2\n")))
	if err == nil {
		t.Fatal("不正な内容が通った")
	}
	if err.Code != "E_STATE_CORRUPT" {
		t.Errorf("code = %q, want E_STATE_CORRUPT", err.Code)
	}
	if err.PathRole != "state" {
		t.Errorf("path role = %q, want state", err.PathRole)
	}
	if len(err.Parameters) != 0 {
		t.Errorf("parametersが空でない: %v", err.Parameters)
	}
	if strings.Contains(err.Error(), testStatePath) {
		t.Errorf("実pathが公開文へ漏れている: %s", err.Error())
	}
}
