package install

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/definition"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain/port"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/store"
)

// EngineRequest はPlanをinstall完了まで実行するための入力である。
//
// docs/08-install-runtime.md §2の`downloading → verifying → staging →
// validating → committing → cleaning → succeeded`に対応する。
//
// **§8手順1〜5の前提検査はここへ含めない。** Plan identity、Approval、`inputs`
// 実体再取得、lock取得は[02-architecture.md](../../docs/02-architecture.md)§2が
// `internal/app`へ割り当てた「トランザクション境界」であり、Execute側が済ませて
// から呼ぶ。engineはPlanが通った前提で作用を並べる。
type EngineRequest struct {
	// Plan は承認・検証済みのPlanである。
	Plan store.Plan
	// Platform はdefinitionの当該platform blockである。
	//
	// `command_targets`の収集がrequired commandの宣言を要る（§7手順4）。
	Platform definition.Platform
	// Roots は§12のpath render rootである。`Payload`はstaging側を指す。
	Roots RenderRoots
	// OperationsRoot は`tmp/operations/`である（role=staging）。
	OperationsRoot domain.PathValue
	// Version はprogress通知へ載せる解決済みversionである。
	Version domain.Version
	// MaxRedirects はdownloadのredirect追跡上限である。
	MaxRedirects int

	// Receipt は`command_targets`と`probes`以外を埋めたreceiptである。
	//
	// 両fieldはengineが実行結果から埋める。呼出し側が先に埋めても上書きする。
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
	// Now はcommit時刻である。
	Now time.Time
}

// EngineResult はinstall完了の結果である。
type EngineResult struct {
	// OperationDir はstagingのoperation rootである。
	//
	// **成功・失敗のどちらでも返す。** 呼出し側がこれを消すことで後始末が
	// 終わる（docs/08-install-runtime.md §6）。
	OperationDir domain.PathValue
	// AlreadyInstalled は既存の同一導入を見つけて後発を破棄したかである。
	AlreadyInstalled bool
	// Receipt は実際に書いたreceiptである。
	Receipt store.Receipt
	// Index は更新後のreceipt indexである。
	Index store.ReceiptIndex
	// IndexStale はcommit成功後にindex更新だけが失敗したかである。
	//
	// docs/08-install-runtime.md §7「手順7のrenameが完了した時点でinstallは
	// 成功とみなす」。呼出し側はこれを`W_CLEANUP_INCOMPLETE`ではなく、
	// 次回起動時の再構築で解消する状態として扱う。
	IndexStale bool
}

// Engine はPlanをinstall完了まで実行する。
//
// **portは呼出し側がGuardで包んでから渡す。** docs/11-quality-and-ci.md §7.2の
// 記録wrapperはPlan固有のScopeを持つため、engineの生成もoperationごとになる。
// engine自身はGuardを知らない——知らせると、Guardを通さずに動かす経路を
// engine内に作れてしまう。
type Engine struct {
	fs        port.FileSystem
	stager    *Stager
	probes    *ProbeRunner
	committer *Committer
}

// NewEngine はEngineを組み立てる。
//
// `filesystem`は`command_targets`収集とpermission正規化が直接使う。部品と
// **同じ**（Guardで包んだ）実装を渡す——別の実装を渡せると、engineの一部だけが
// Guardを通らない経路ができる。
func NewEngine(
	filesystem port.FileSystem, stager *Stager, probes *ProbeRunner, committer *Committer,
) (*Engine, error) {
	switch {
	case filesystem == nil:
		return nil, errors.New("install: FileSystem portが未設定")
	case stager == nil:
		return nil, errors.New("install: Stagerが未設定")
	case probes == nil:
		return nil, errors.New("install: ProbeRunnerが未設定")
	case committer == nil:
		return nil, errors.New("install: Committerが未設定")
	}
	return &Engine{fs: filesystem, stager: stager, probes: probes, committer: committer}, nil
}

// Run はdownloadからcommitまでを順に行う。
//
// docs/08-install-runtime.md §7の手順を、部品の呼出し順として並べる。
//
//	staging（§5・§6）→ 手順1 payload再検査 → 手順2〜3 probe
//	→ 手順4 command_targets → 手順5 permission正規化 → 手順6〜8 commit
//
// **手順1はCommitterが行う。** 再検査はcommitの直前でなければ意味がなく、
// probeとpermission正規化の後に差し込まれたentryも捕まえる必要がある。
//
// **失敗しても後始末をここで行わない。** §6が「中断・失敗・cancel時は
// `tmp/operations/<operation-id>/`をdirectory単位で削除すれば復旧する」と定める。
// 結果へoperation directoryを返し、呼出し側が[Stager.Cleanup]を1回呼ぶ。
func (e *Engine) Run(ctx context.Context, req EngineRequest) (EngineResult, *domain.Error) {
	if err := req.validate(); err != nil {
		return EngineResult{}, domain.Internal(err)
	}

	staged, err := e.stager.Stage(ctx, StageRequest{
		Plan:           req.Plan,
		OperationsRoot: req.OperationsRoot,
		Host:           req.Roots.Host,
		Version:        req.Version,
		MaxRedirects:   req.MaxRedirects,
	})
	result := EngineResult{OperationDir: staged.OperationDir}
	if err != nil {
		return result, err
	}

	// 展開先をrender rootのpayloadとして使う。Planのprobe pathは
	// このrootを前提に組み立てられている。
	roots := req.Roots
	roots.Payload = staged.PayloadDir

	// 手順2〜3。
	outcomes, probeErr := e.probes.Run(ctx, ProbeRequest{Plan: req.Plan, Host: roots.Host})
	if probeErr != nil {
		return result, probeErr
	}

	// 手順4。permission正規化の**前**に行う。read onlyにした後だと、
	// filesystemによってはread自体が制限されうる。
	targets, targetErr := CollectCommandTargets(e.fs, CommandTargetRequest{
		Platform:   req.Platform,
		PayloadDir: staged.PayloadDir,
		Roots:      roots,
	})
	if targetErr != nil {
		return result, domain.Internal(targetErr)
	}

	// 手順5。
	if hardenErr := HardenPayload(e.fs, staged.PayloadDir, roots.Host); hardenErr != nil {
		return result, &domain.Error{
			Code:      domain.CodeFilesystem,
			Retryable: true,
			PathRole:  domain.RolePayload,
			Cause:     hardenErr,
		}
	}

	receipt := req.Receipt
	receipt.CommandTargets = targets
	receipt.Probes = mergeProbeOutcomes(req.Plan.Probes, outcomes)
	result.Receipt = receipt

	// 手順1・6〜8。
	committed, commitErr := e.committer.Commit(CommitRequest{
		StagingPayload: staged.PayloadDir,
		OperationDir:   staged.OperationDir,
		Receipt:        receipt,
		Destination:    req.Destination,
		ReceiptName:    req.ReceiptName,
		Index:          req.Index,
		IndexPath:      req.IndexPath,
		IndexEntryPath: req.IndexEntryPath,
		Host:           roots.Host,
		Now:            req.Now,
	})
	result.AlreadyInstalled = committed.AlreadyInstalled
	result.Index = committed.Index
	if commitErr != nil {
		// **index更新だけの失敗はinstallを失敗にしない**（§7）。renameが
		// 完了していれば導入は成功しており、indexが古いだけの状態は次回
		// 起動時の再構築で解消する。
		if commitErr.Code == domain.CodeFilesystem &&
			commitErr.PathRole == domain.RoleReceiptIndex {
			result.IndexStale = true
			return result, nil
		}
		return result, commitErr
	}
	return result, nil
}

// validate はengine要求の前提を確かめる。
func (r EngineRequest) validate() error {
	switch {
	case r.Plan.Kind != store.OperationInstall:
		return fmt.Errorf("install: operationが%sである（installだけを扱う）", r.Plan.Kind)
	case r.Roots.Host.IsZero():
		return errors.New("install: host platformが未設定")
	case r.Destination.IsZero() || r.Destination.Path() == "":
		return errors.New("install: 完成先が未設定")
	case r.ReceiptName == "":
		return errors.New("install: receipt file名が未設定")
	case r.Now.IsZero():
		return errors.New("install: commit時刻が未設定")
	}
	return nil
}

// mergeProbeOutcomes はPlanのprobe宣言と実行結果を突き合わせてreceipt用へ変換する。
//
// docs/04-storage-and-data.md §14のreceipt probeは、Planの宣言（args、regex、
// timeout等）と実行結果（status、reported version、finished at）の両方を持つ。
//
// **Planに無いprobeの結果を作らない。** 宣言と結果はPlanのprobe IDで対応する。
func mergeProbeOutcomes(
	planned []store.PlanProbe, outcomes []ProbeOutcome,
) []store.ReceiptProbe {
	if len(planned) == 0 {
		return nil
	}
	byID := make(map[string]ProbeOutcome, len(outcomes))
	for _, outcome := range outcomes {
		byID[outcome.ID] = outcome
	}
	receipts := make([]store.ReceiptProbe, 0, len(planned))
	for _, probe := range planned {
		outcome, ran := byID[probe.ID]
		status := store.ProbeSkipped
		if ran {
			status = outcome.Status
		}
		args := make([]string, 0, len(probe.Args))
		for _, arg := range probe.Args {
			value, err := planArgValue(arg)
			if err != nil {
				// Planのcodecが弾いているはずの形である。receiptへ空を書くより
				// 元の値が分かる形を残す。
				value = arg.Value
			}
			args = append(args, value)
		}
		required := make([]string, 0, len(probe.RequiredPaths))
		for _, path := range probe.RequiredPaths {
			required = append(required, string(path.Kind)+":"+path.Path.Path())
		}
		receipts = append(receipts, store.ReceiptProbe{
			ID:              probe.ID,
			RuntimeCommand:  probe.RuntimeCommand,
			Args:            args,
			Stream:          probe.Stream,
			Expect:          probe.Expect,
			Regex:           probe.Regex,
			ExpectedVersion: probe.ExpectedVersion,
			ExpectedRoot:    expectedRootText(probe),
			RequiredPaths:   required,
			TimeoutMillis:   probe.TimeoutMillis,
			Required:        probe.Required,
			Status:          status,
			ReportedVersion: outcome.ReportedVersion,
			FinishedAt:      outcome.FinishedAt,
		})
	}
	return receipts
}

// expectedRootText はreceiptへ書くexpected rootを返す。
func expectedRootText(probe store.PlanProbe) string {
	if probe.ExpectedRoot == nil {
		return ""
	}
	return probe.ExpectedRoot.Path()
}
