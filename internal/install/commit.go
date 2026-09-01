package install

import (
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"time"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain/port"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/security"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/store"
)

// CommitRequest はstagingを完成先へ確定するための入力である。
//
// docs/08-install-runtime.md §7手順1・6〜8に対応する。probe実行（手順2〜3）と
// `command_targets`収集（手順4）、permission正規化（手順5）は呼出し側が済ませ、
// 結果をここへ渡す。
type CommitRequest struct {
	// StagingPayload は展開・検証済みのpayloadである（role=payload）。
	StagingPayload domain.PathValue
	// OperationDir はstagingのoperation rootである（role=staging）。
	//
	// 手順1の「staging payloadの全pathがroot内にあること」をここで確かめる。
	OperationDir domain.PathValue
	// Receipt は書き出すreceiptである。
	Receipt store.Receipt
	// Destination は完成先のversion directoryである。
	Destination domain.PathValue
	// ReceiptName はversion directory内のreceipt file名である。
	ReceiptName string
	// Index は更新前のreceipt indexである。
	Index store.ReceiptIndex
	// IndexPath はreceipt indexのpathである（role=receipt-index）。
	IndexPath domain.PathValue
	// IndexEntryPath はindexへ書くdata root相対のreceipt pathである。
	IndexEntryPath string
	// Host はpath規則を決めるplatformである。
	Host domain.Platform
	// Now はcommit時刻である。呼出し側がClock portから取る。
	Now time.Time
}

// CommitResult はcommitの結果である。
type CommitResult struct {
	// AlreadyInstalled は既存の同一導入を見つけて後発を破棄したかである。
	//
	// docs/08-install-runtime.md §7「完成先が競合して作られた場合、両receiptと
	// `command_targets`が完全一致すれば後発stagingを破棄して**成功**」。
	AlreadyInstalled bool
	// Index は更新後のreceipt indexである。
	Index store.ReceiptIndex
}

// Committer はstagingを完成先へ確定する。
type Committer struct {
	fs port.FileSystem
}

// NewCommitter はCommitterを作る。
func NewCommitter(filesystem port.FileSystem) (*Committer, error) {
	if filesystem == nil {
		return nil, errors.New("install: FileSystem portが未設定")
	}
	return &Committer{fs: filesystem}, nil
}

// Commit は§7手順1・6〜8を順に行う。
//
//  1. staging payloadの全pathがroot内にあることを再検査する（手順1）。
//  6. receiptをstagingへ書きflushする（手順6）。
//  7. 完成先がないことを確認し、version directoryを同一volumeでatomic renameする（手順7）。
//  8. receipt indexをatomic更新する（手順8）。
//
// **手順7のrenameが完了した時点でinstallは成功とみなす**（同§）。rename前の中断は
// 未導入、rename後の中断は導入成功でindexだけ古い状態であり、次回起動時の再構築で
// 解消する。したがって手順8の失敗はinstallの失敗にしない。
func (c *Committer) Commit(req CommitRequest) (CommitResult, *domain.Error) {
	if err := req.validate(); err != nil {
		return CommitResult{}, domain.Internal(err)
	}

	// 手順1。展開後にstagingへ差し込まれたentryがないことを確かめる。
	if err := c.checkPayloadContained(req); err != nil {
		return CommitResult{}, err
	}

	// 手順6。receiptはstagingへ書く。完成先へ直接書くと、rename前の中断で
	// 半端なreceiptが完成先へ残る。
	if err := c.writeReceipt(req); err != nil {
		return CommitResult{}, err
	}

	// 手順7。
	conflict, err := c.rename(req)
	if err != nil {
		return CommitResult{}, err
	}
	if conflict {
		// 既存が同一内容だった。後発stagingは呼出し側がCleanupで破棄する。
		return CommitResult{AlreadyInstalled: true, Index: req.Index}, nil
	}

	// 手順8。**ここから先の失敗でinstallを失敗にしない。** renameが完了した
	// 時点で導入は成功しており、indexが古いだけの状態は次回起動時の再構築で
	// 解消する（同§）。呼出し側は失敗をwarningへ変換する。
	updated, indexErr := c.updateIndex(req)
	if indexErr != nil {
		return CommitResult{Index: req.Index}, indexErr
	}
	return CommitResult{Index: updated}, nil
}

// validate はcommit要求の前提を確かめる。
func (r CommitRequest) validate() error {
	switch {
	case r.StagingPayload.IsZero() || r.StagingPayload.Path() == "":
		return errors.New("install: staging payloadが未設定")
	case r.StagingPayload.Role() != domain.RolePayload:
		return fmt.Errorf("install: staging payloadのroleが%sである", r.StagingPayload.Role())
	case r.OperationDir.IsZero() || r.OperationDir.Path() == "":
		return errors.New("install: operation directoryが未設定")
	case r.Destination.IsZero() || r.Destination.Path() == "":
		return errors.New("install: 完成先が未設定")
	case r.ReceiptName == "":
		return errors.New("install: receipt file名が未設定")
	case r.IndexPath.IsZero() || r.IndexPath.Path() == "":
		return errors.New("install: receipt index pathが未設定")
	case r.IndexEntryPath == "":
		return errors.New("install: index entryのpathが未設定")
	case r.Host.IsZero():
		return errors.New("install: host platformが未設定")
	case r.Now.IsZero():
		return errors.New("install: commit時刻が未設定")
	case r.Receipt.InstallID == "":
		return errors.New("install: receiptのinstall IDが未設定")
	}
	return nil
}

// checkPayloadContained は§7手順1を行う。
//
// 「staging payloadの全pathがroot内にあることを再検査する」。展開側
// （[Extractor]）が同じ検査を済ませているが、展開からcommitまでの間に
// symlinkが差し込まれていないことをここで確かめる。**1箇所の検査に頼らない。**
func (c *Committer) checkPayloadContained(req CommitRequest) *domain.Error {
	root := req.StagingPayload.Path()
	walkErr := c.fs.Walk(root, func(path string, info port.FileInfo) error {
		if path != root && !security.IsContained(root, path, req.Host) {
			return fmt.Errorf("payload外のentry %q がある", path)
		}
		if info.IsSymlink {
			// §6が展開時にsymlinkを拒否している。現れたのは展開後である。
			return fmt.Errorf("payload内にsymlink/reparse point %q がある", path)
		}
		// operation staging全体の外へ出ていないことも見る。
		if !security.IsContained(req.OperationDir.Path(), path, req.Host) {
			return fmt.Errorf("operation staging外のentry %q がある", path)
		}
		return nil
	})
	if walkErr != nil {
		return &domain.Error{
			Code:     domain.CodePathUnsafe,
			PathRole: domain.RolePayload,
			Cause:    fmt.Errorf("install: staging payloadの再検査に失敗した: %w", walkErr),
		}
	}
	return nil
}

// writeReceipt は§7手順6を行う。
func (c *Committer) writeReceipt(req CommitRequest) *domain.Error {
	data, encodeErr := store.EncodeReceipt(req.Receipt)
	if encodeErr != nil {
		return encodeErr
	}
	path, err := c.receiptPath(req.StagingPayload, req)
	if err != nil {
		return err
	}
	// atomic writeはport側がtmp→flush→renameを行う（docs/04-storage-and-data.md §4）。
	if writeErr := c.fs.AtomicWrite(path, data, receiptPerm); writeErr != nil {
		return stagingError(fmt.Errorf("install: receiptを書けない: %w", writeErr))
	}
	return nil
}

// receiptPath はversion directory内のreceipt pathを組み立てる。
//
// receiptはpayloadの**兄弟**である（§14の`payload_path=payload`固定と、
// receipt保存pathが`.gdtvm-install.toml`であることから、version directory直下）。
func (c *Committer) receiptPath(payload domain.PathValue, req CommitRequest) (string, *domain.Error) {
	separator := security.PathSeparator(req.Host)
	parent := payload.Path()
	// payloadの親がversion directoryである。
	index := lastIndexOf(parent, separator)
	if index <= 0 {
		return "", domain.Internal(fmt.Errorf(
			"install: payload path %q から親を求められない", parent))
	}
	return parent[:index] + separator + req.ReceiptName, nil
}

// rename は§7手順7を行う。
//
// 「**完成先がないことを確認し**、version directoryを同一volumeでatomic rename
// する」。仕様が事前確認を求めるのは、rename先が存在する場合の挙動がOSで違う
// ためである（POSIXは空directoryを置換しうるが、Windowsは失敗する）。
//
// 確認とrenameの間に別processが完成先を作る余地は残る。その競合こそ同§が
// 「完成先が競合して作られた場合」として扱う場面であり、[SameInstall]で
// 内容一致を判定する。
//
// 戻り値のboolは「既存が同一内容だったので後発を破棄してよい」である。
func (c *Committer) rename(req CommitRequest) (bool, *domain.Error) {
	source, err := c.versionDir(req)
	if err != nil {
		return false, err
	}
	if existing, conflict, checkErr := c.checkDestination(req); checkErr != nil {
		return false, checkErr
	} else if existing {
		return conflict, nil
	}

	if renameErr := c.fs.Rename(source, req.Destination.Path()); renameErr != nil {
		// renameが失敗する主因は、確認後に完成先が作られたことである。
		// もう一度見て、同一内容なら成功として扱う。
		if existing, conflict, checkErr := c.checkDestination(req); checkErr == nil && existing {
			return conflict, nil
		}
		return false, &domain.Error{
			Code:      domain.CodeFilesystem,
			Retryable: true,
			PathRole:  domain.RoleVersionData,
			Cause:     fmt.Errorf("install: 完成先へrenameできない: %w", renameErr),
		}
	}
	return false, nil
}

// checkDestination は完成先の有無と、あった場合の内容一致を返す。
//
// 第1戻り値が「存在した」、第2戻り値が「同一内容だった」である。存在して
// 内容が違えば`E_CONFLICT`を返す。
func (c *Committer) checkDestination(req CommitRequest) (bool, bool, *domain.Error) {
	if _, statErr := c.fs.Stat(req.Destination.Path()); statErr != nil {
		// 無ければrenameできる。
		return false, false, nil
	}
	existing, readErr := c.readReceipt(req)
	if readErr != nil {
		// 完成先はあるがreceiptを読めない。破損として扱い、黙って上書きしない
		// （docs/04-storage-and-data.md §4「破損として扱い、黙って修復・再生成
		// しない」）。
		return true, false, &domain.Error{
			Code:     domain.CodeConflict,
			PathRole: domain.RoleVersionData,
			Cause: fmt.Errorf(
				"install: 完成先が既にあるがreceiptを読めない: %w", readErr),
		}
	}
	if !SameInstall(existing, req.Receipt) {
		return true, false, &domain.Error{
			Code:     domain.CodeConflict,
			PathRole: domain.RoleVersionData,
			Cause:    ConflictError(existing, req.Receipt),
		}
	}
	return true, true, nil
}

// readReceipt は完成先のreceiptを読む。
func (c *Committer) readReceipt(req CommitRequest) (store.Receipt, error) {
	separator := security.PathSeparator(req.Host)
	path := req.Destination.Path() + separator + req.ReceiptName
	data, err := c.fs.ReadFile(path, store.ReceiptFileMaxBytes)
	if err != nil {
		return store.Receipt{}, err
	}
	receipt, parseErr := store.ParseReceipt(data)
	if parseErr != nil {
		return store.Receipt{}, parseErr
	}
	return receipt, nil
}

// versionDir はstaging側のversion directory（payloadの親）を返す。
func (c *Committer) versionDir(req CommitRequest) (string, *domain.Error) {
	separator := security.PathSeparator(req.Host)
	index := lastIndexOf(req.StagingPayload.Path(), separator)
	if index <= 0 {
		return "", domain.Internal(fmt.Errorf(
			"install: staging payload %q から親を求められない", req.StagingPayload.Path()))
	}
	return req.StagingPayload.Path()[:index], nil
}

// updateIndex は§7手順8を行う。
//
// 「receipt indexをatomic更新する。indexが古い状態で中断してもreceipt走査から
// 再構築できる」。**再構築そのものはここで行わない** — 同§が実施時点を
// 「次回起動時」と定めており、起動時処理の責務である。
func (c *Committer) updateIndex(req CommitRequest) (store.ReceiptIndex, *domain.Error) {
	// §13の`receipt_sha256`はreceipt fileの内容のdigestである。書いたものと
	// 同じbyte列から計算するため、ここでencodeし直す。
	data, encodeErr := store.EncodeReceipt(req.Receipt)
	if encodeErr != nil {
		return req.Index, encodeErr
	}
	digest := security.SHA256Hex(data)

	updated := req.Index
	updated.Revision = req.Index.Revision + 1
	updated.UpdatedAt = req.Now.UTC()
	updated.Entries = upsertIndexEntry(req.Index.Entries, store.ReceiptIndexEntry{
		Ref:           req.Receipt.Ref,
		InstallID:     req.Receipt.InstallID,
		Path:          req.IndexEntryPath,
		ReceiptSHA256: digest,
		// commit直後は検証済みである。§13「破損receiptを健康扱いしない」の
		// 逆で、今しがたprobeまで通したものをunknownにする理由がない。
		Health: domain.HealthHealthy,
	})

	encoded, indexErr := store.EncodeReceiptIndex(updated)
	if indexErr != nil {
		return req.Index, indexErr
	}
	if writeErr := c.fs.AtomicWrite(req.IndexPath.Path(), encoded, receiptPerm); writeErr != nil {
		return req.Index, &domain.Error{
			Code:      domain.CodeFilesystem,
			Retryable: true,
			PathRole:  domain.RoleReceiptIndex,
			Cause:     fmt.Errorf("install: receipt indexを更新できない: %w", writeErr),
		}
	}
	return updated, nil
}

// upsertIndexEntry は同じtupleのentryを置き換え、無ければ足してsortする。
//
// §13「tupleで一意・sort」。同じtupleが2件並ぶと、どちらが正かを決められない。
func upsertIndexEntry(
	entries []store.ReceiptIndexEntry, entry store.ReceiptIndexEntry,
) []store.ReceiptIndexEntry {
	updated := make([]store.ReceiptIndexEntry, 0, len(entries)+1)
	replaced := false
	for _, existing := range entries {
		if existing.Ref == entry.Ref {
			updated = append(updated, entry)
			replaced = true
			continue
		}
		updated = append(updated, existing)
	}
	if !replaced {
		updated = append(updated, entry)
	}
	sort.Slice(updated, func(i, j int) bool {
		return indexSortKey(updated[i]) < indexSortKey(updated[j])
	})
	return updated
}

// indexSortKey は§13のtuple順を決める。
func indexSortKey(entry store.ReceiptIndexEntry) string {
	return entry.Ref.Tool.String() + "\x00" +
		entry.Ref.Version + "\x00" + entry.Ref.Platform.ID()
}

// lastIndexOf は区切りの最後の位置を返す。見つからなければ-1である。
func lastIndexOf(value, separator string) int {
	for index := len(value) - len(separator); index >= 0; index-- {
		if value[index:index+len(separator)] == separator {
			return index
		}
	}
	return -1
}

// receiptPerm はreceiptとindexのpermissionである。
//
// docs/10-security.md §6「Windowsは現在user所有かつ他user書込み不可のACL、
// Linuxは現在UID所有かつgroup/other書込み不可」。
const receiptPerm fs.FileMode = 0o600
