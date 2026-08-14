package store

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain/port"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/security"
)

// BackupSuffix は正本stateの1世代backupの拡張子である。
//
// docs/04-storage-and-data.md §4が「同じdirectoryの`<basename>.bak`へ最新1世代を
// atomic保持してから、temporaryをatomic replaceする」と定める。
const BackupSuffix = ".bak"

// StateFilePerm は正本stateとbackupのpermissionである。
//
// §4が「`.bak`は元fileと同じowner-only permission」と定める。owner以外に
// 読ませないのは、stateがroot IDと導入構成を含むためである。
const StateFilePerm fs.FileMode = 0o600

// FirstRevision は新規fileのrevisionである。
//
// §4が「revision fieldを持つfileはnext=current+1（新規は1）を計算する」と定める。
const FirstRevision int64 = 1

// NextRevision は次のrevisionを返す（§4 step 2）。
//
// 新規（current=0）なら[FirstRevision]、既存ならcurrent+1である。revisionを
// 進めないと、並行するprocessが同じrevisionのstateを別内容で書ける。
func NextRevision(current int64) int64 {
	if current <= 0 {
		return FirstRevision
	}
	return current + 1
}

// StateWriteRequest は正本stateのatomic writeの入力である。
//
// lock取得（§4 step 1）は呼出し側の責務である。[port.LockManager]で対象lockを
// 取ってから呼ぶ。lockをここで取ると、§12のlock順を1操作の中で守れない。
type StateWriteRequest struct {
	// Path は書込み先である。roleは診断へ載せる。
	Path domain.PathValue
	// Data はnext revisionを含む完全な内容である。
	//
	// revisionの計算とencodeは呼出し側が行う。codecがfile形式ごとに違い、
	// ここで組み立てるとfile形式の数だけ分岐が増えるためである。
	Data []byte
	// Parse はstrict再parseである（§4 step 4）。
	//
	// 対応するcodecの`Parse*`を渡す。書けない内容を公開fileにしないための
	// 検査であり、nilを許さない。
	Parse func([]byte) error
	// RootID は書込む内容のroot IDである。`.bak`の復元候補判定に使う。
	RootID string
	// Backup は`.bak`を保持するかである。
	//
	// §4が「receipt、catalog、再構築可能indexはこのgeneric backup対象外」と
	// 定める。正本state 3件（`state/schema.toml`、`state/selections.toml`、
	// `state/setup.toml`）だけtrueにする。
	Backup bool
	// FileSystem は読書きに使う。
	FileSystem port.FileSystem
}

// StateWriteResult は書込み結果である。
type StateWriteResult struct {
	// SHA256 は公開fileの内容のdigestである。
	SHA256 string
	// BackupWritten は`.bak`を更新したかである。新規fileではfalseになる。
	BackupWritten bool
	// RolledBack は検証失敗でbackupへ戻したかである。
	//
	// trueのときはerrorも返る。呼出し側が「書けなかったが旧内容は残っている」と
	// 「書けず旧内容も失った」を区別できるようにする。
	RolledBack bool
}

// WriteState は§4のatomic writeを行う。
//
// 同§の7段階のうち、lock取得（step 1）とrevision計算（step 2）は呼出し側、
// flushとdirectory metadataの永続化は[port.FileSystem]の実装（P9）の責務である。
// [port.FileSystem.AtomicWrite]がtemp書込みとreplaceを不可分に行うため、
// step 4のstrict再parseはAtomicWriteへ渡す**同じbytes**に対して行い、disk上の
// 検証はstep 7の公開file再読で担う。
//
// 「Windowsでreplace APIの制約がある場合も、旧fileを失った状態で新fileを
// 書き始めない」（§4）ため、`.bak`は必ずreplaceの前に書く。
//
// 親directoryの作成は呼出し側の責務である。§2のroot layoutをsetupが作るため、
// ここでmkdirすると、layoutに無い場所へstateを書けてしまう。
func WriteState(request StateWriteRequest) (StateWriteResult, *domain.Error) {
	var result StateWriteResult
	if err := checkStateWriteRequest(request); err != nil {
		return result, err
	}
	path := request.Path.Path()

	// step 4: 公開前にstrict再parseする。encodeの誤りをここで止めないと、
	// 次回の読込みで破損として現れて原因の特定が難しくなる。
	if err := request.Parse(request.Data); err != nil {
		return result, stateWriteError("state.write_content_invalid", request.Path, err)
	}

	existing, hadExisting, readErr := readIfExists(request.FileSystem, path)
	if readErr != nil {
		return result, stateWriteError("state.write_read_failed", request.Path, readErr)
	}

	// step 5: 既存fileがあれば`.bak`へ1世代退避してからreplaceする。
	if hadExisting && request.Backup {
		if err := request.FileSystem.AtomicWrite(backupPath(path), existing, StateFilePerm); err != nil {
			return result, stateWriteError("state.backup_write_failed", request.Path, err)
		}
		result.BackupWritten = true
	}
	if err := request.FileSystem.AtomicWrite(path, request.Data, StateFilePerm); err != nil {
		return result, stateWriteError("state.write_failed", request.Path, err)
	}

	// step 7: 公開fileを再読してexpected digestと一致させる。
	expected := security.SHA256Hex(request.Data)
	published, err := request.FileSystem.ReadFile(path, StateFileMaxBytes)
	if err != nil {
		return result, request.rollback(&result, "state.verify_read_failed", err)
	}
	if actual := security.SHA256Hex(published); actual != expected {
		return result, request.rollback(&result, "state.verify_digest_mismatch",
			fmt.Errorf("公開fileのdigestが一致しない（expected %s、actual %s）", expected, actual))
	}
	if err := request.Parse(published); err != nil {
		return result, request.rollback(&result, "state.verify_parse_failed", err)
	}
	result.SHA256 = expected
	return result, nil
}

// rollback は検証失敗時に検証済みbackupへ戻す（§4 step 7）。
//
// backupが復元候補として妥当でない場合は戻さない。壊れたbackupで公開fileを
// 上書きすると、破損を1世代分広げることになる。
func (r StateWriteRequest) rollback(
	result *StateWriteResult, messageID string, cause error,
) *domain.Error {
	if !r.Backup {
		return stateWriteError(messageID, r.Path, cause)
	}
	backup, hadBackup, readErr := readIfExists(r.FileSystem, backupPath(r.Path.Path()))
	if readErr != nil || !hadBackup {
		return stateWriteError(messageID, r.Path, cause)
	}
	if err := r.checkRestoreCandidate(backup); err != nil {
		return stateWriteError(messageID, r.Path, errors.Join(cause, err))
	}
	if err := r.FileSystem.AtomicWrite(r.Path.Path(), backup, StateFilePerm); err != nil {
		return stateWriteError(messageID, r.Path, errors.Join(cause, err))
	}
	result.RolledBack = true
	return stateWriteError(messageID, r.Path, cause)
}

// RestoreRequest は破損した正本stateをbackupから復旧する入力である。
type RestoreRequest struct {
	// Path は復旧先の正本stateである。
	Path domain.PathValue
	// Parse はstrict再parseである。
	Parse func([]byte) error
	// RootID は期待するroot IDである。
	RootID string
	// FileSystem は読書きに使う。
	FileSystem port.FileSystem
}

// RestoreFromBackup は`<basename>.bak`から正本stateを復旧する（§4）。
//
// 同§が「strict parse/digest/root IDが一致する場合だけ復元候補にする」と
// 定める。候補にならないbackupで上書きすると、破損を1世代分広げる。
//
// 復元しなかった場合は`E_STATE_CORRUPT`を返す。indexやcacheと違い、正本stateを
// 推測再生成しない（[02-architecture.md](../02-architecture.md) §13）。
func RestoreFromBackup(request RestoreRequest) (string, *domain.Error) {
	if request.FileSystem == nil || request.Parse == nil {
		return "", stateWriteError("state.restore_request_invalid", request.Path, nil)
	}
	backup, hadBackup, readErr := readIfExists(request.FileSystem, backupPath(request.Path.Path()))
	if readErr != nil {
		return "", stateError("state.restore_read_failed", request.Path.Role(), readErr)
	}
	if !hadBackup {
		return "", stateError("state.restore_backup_missing", request.Path.Role(), nil)
	}
	candidate := StateWriteRequest{Parse: request.Parse, RootID: request.RootID}
	if err := candidate.checkRestoreCandidate(backup); err != nil {
		return "", stateError("state.restore_backup_invalid", request.Path.Role(), err)
	}
	if err := request.FileSystem.AtomicWrite(request.Path.Path(), backup, StateFilePerm); err != nil {
		return "", stateError("state.restore_write_failed", request.Path.Role(), err)
	}
	return security.SHA256Hex(backup), nil
}

// checkRestoreCandidate は§4の復元候補条件を確かめる。
//
// strict parseとroot ID一致を見る。digestは内容から計算するため、内容が
// parseできてroot IDが一致すれば「そのrootのstateとして読める」ことが決まる。
func (r StateWriteRequest) checkRestoreCandidate(backup []byte) error {
	if err := r.Parse(backup); err != nil {
		return fmt.Errorf("backupがstrict parseできない: %w", err)
	}
	if r.RootID == "" {
		return errors.New("期待するroot IDが未設定")
	}
	if !strings.Contains(string(backup), r.RootID) {
		return fmt.Errorf("backupのroot IDが一致しない")
	}
	return nil
}

func checkStateWriteRequest(request StateWriteRequest) *domain.Error {
	switch {
	case request.FileSystem == nil:
		return stateWriteError("state.write_request_invalid", request.Path, errors.New("FileSystemが未設定"))
	case request.Parse == nil:
		return stateWriteError("state.write_request_invalid", request.Path, errors.New("Parseが未設定"))
	case request.Path.IsZero() || request.Path.Path() == "":
		return stateWriteError("state.write_request_invalid", request.Path, errors.New("pathが未設定"))
	case len(request.Data) == 0:
		return stateWriteError("state.write_request_invalid", request.Path, errors.New("dataが空"))
	case request.Backup && request.RootID == "":
		return stateWriteError("state.write_request_invalid", request.Path, errors.New("root IDが未設定"))
	}
	if err := requireSize("state TOML", request.Data, StateFileMaxBytes); err != nil {
		return stateWriteError("state.write_request_invalid", request.Path, err)
	}
	return nil
}

func backupPath(path string) string { return path + BackupSuffix }

// readIfExists はfileを読む。不在は空とfalseで返し、errorにしない。
func readIfExists(fsys port.FileSystem, path string) ([]byte, bool, error) {
	data, err := fsys.ReadFile(path, StateFileMaxBytes)
	if err == nil {
		return data, true, nil
	}
	if isNotExistError(err) {
		return nil, false, nil
	}
	return nil, false, err
}

func isNotExistError(err error) bool { return errors.Is(err, fs.ErrNotExist) }

func stateWriteError(messageID string, path domain.PathValue, cause error) *domain.Error {
	return stateError(messageID, path.Role(), cause)
}
