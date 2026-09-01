package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain/port"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/store"
)

// InstallInputSource は`inputs`の現在値を集める。
//
// docs/02-architecture.md §8手順3「`inputs`に固定したroot/config/registry/
// definition/catalog/receipt index/selection/setupのrevision/digest identity」。
//
// **interfaceで受けるのは、供給元がpackageをまたぐためである。** revision/digestの
// 計算主体はそれぞれのfileを所有するpackage（config／registry／catalog／store）で
// あり、Application Serviceはそれらを束ねる順序と時点だけを決める。
//
// **portではない。** docs/02-architecture.md §4「効果がすべて既存portの背後へ
// 閉じているorchestrationはportにしない」。読取りはすべて[port.FileSystem]の
// 背後にある。portにすると再取得そのものを差し替えられ、`E_PLAN_STALE`の検査を
// testで確かめられなくなる。
type InstallInputSource interface {
	// PlanInputs は今の実体から`inputs`を組み立てる。
	PlanInputs(ctx context.Context) (store.PlanInputs, error)
}

// InstallEngine はPlanをinstall完了まで実行する。
//
// `internal/install`の`Engine`が実装する。interfaceで受けるのは、Execute側が
// engineの生成（Guardで包んだportの注入）を呼出し側から受け取るためである。
type InstallEngine interface {
	Run(ctx context.Context, plan store.Plan) (InstallEngineResult, error)
}

// InstallEngineResult はengineの結果のうちExecuteが使う部分である。
type InstallEngineResult struct {
	// AlreadyInstalled は既存の同一導入を見つけて後発を破棄したかである。
	AlreadyInstalled bool
	// IndexStale はcommit成功後にindex更新だけが失敗したかである。
	IndexStale bool
	// Cleanup はoperation stagingを破棄する。成功・失敗のどちらでも呼ぶ。
	Cleanup func() error
}

// ExecuteInstallRequest はExecuteInstallの入力である。
type ExecuteInstallRequest struct {
	// Plan は承認対象のPlanである。
	Plan store.Plan
	// Approval は利用者の承認である。
	Approval Approval
	// Build は実行中clientのbuild情報である。
	Build BuildInfo
	// Invocation は現在のinvocation IDである。
	Invocation domain.InvocationID
	// Inputs は`inputs`の現在値を集める供給元である。
	Inputs InstallInputSource
	// Engine はinstallを実行するengineである。
	Engine InstallEngine
	// Locks はlock managerである。
	Locks port.LockManager
}

// ExecuteInstallResult はExecuteInstallの結果である。
type ExecuteInstallResult struct {
	// AlreadyInstalled は冪等installだったかである。
	AlreadyInstalled bool
	// Warnings は結果へ載せるwarningである。
	Warnings []ResultWarning
}

// ResultWarning は結果warningである（docs/04-storage-and-data.md §16.2）。
type ResultWarning struct {
	Code store.ResultWarningCode
}

// ExecuteInstall は§8手順1〜5を順に行ってからengineを走らせる。
//
// docs/02-architecture.md §8「Executeは次を順に検査する」。
//
//  1. Plan schema/client/invocationの一致
//  2. Approvalが必要な`PlanWarningCode`を含む
//  3. `inputs`に固定した…revision/digest identity
//  4. **lock取得後に同じ検査を繰り返す**
//  5. Execute中の作用がPlanの列挙と一致すること（Guardが担う）
//
// **手順3をlockの前後で2回行う。** 1回目は承認直後の無駄な作業を避けるため、
// 2回目はlock取得までの間に他processが状態を変えていないことを確かめるため
// である。1回目だけだと、待っている間の変化を見逃す。
//
// 手順5のGuardは呼出し側がengineへ注入済みである（[InstallEngine]のdoc comment）。
func ExecuteInstall(
	ctx context.Context, req ExecuteInstallRequest,
) (ExecuteInstallResult, *domain.Error) {
	if err := req.validate(); err != nil {
		return ExecuteInstallResult{}, domain.Internal(err)
	}

	// 手順1。
	if err := CheckPlanIdentity(req.Plan, req.Build, req.Invocation); err != nil {
		return ExecuteInstallResult{}, err
	}
	// 手順2。
	if err := CheckApproval(req.Plan, req.Approval); err != nil {
		return ExecuteInstallResult{}, err
	}
	// 手順3（1回目、承認直後）。
	if err := checkInputs(ctx, req); err != nil {
		return ExecuteInstallResult{}, err
	}

	// 手順4のlock。§12の`ClassInstall`はToolID、version、platform順である。
	lock, err := acquireInstallLock(ctx, req)
	if err != nil {
		return ExecuteInstallResult{}, err
	}
	defer func() { _ = lock.Release() }()

	// 手順3（2回目、lock取得後）。
	if inputErr := checkInputs(ctx, req); inputErr != nil {
		return ExecuteInstallResult{}, inputErr
	}

	engineResult, runErr := req.Engine.Run(ctx, req.Plan)
	// **成功・失敗のどちらでもstagingを破棄する**（docs/08-install-runtime.md §6）。
	var warnings []ResultWarning
	if engineResult.Cleanup != nil {
		if cleanupErr := engineResult.Cleanup(); cleanupErr != nil && runErr == nil {
			// §2「commit後の一時file清掃失敗は成功＋`W_CLEANUP_INCOMPLETE`とし、
			// 正常payloadを巻き戻さない」。**失敗時はcleanup失敗を握り潰す** ——
			// 元の失敗のほうが利用者に必要な情報である。
			warnings = append(warnings,
				ResultWarning{Code: store.WarnCleanupIncomplete})
		}
	}
	if runErr != nil {
		return ExecuteInstallResult{}, engineError(runErr)
	}
	if engineResult.IndexStale {
		// index更新だけの失敗。導入は成功しており、次回起動時の再構築で解消する
		// （§7）。清掃未完了と同じく成功＋warningとして扱う。
		warnings = append(warnings, ResultWarning{Code: store.WarnCleanupIncomplete})
	}
	return ExecuteInstallResult{
		AlreadyInstalled: engineResult.AlreadyInstalled,
		Warnings:         warnings,
	}, nil
}

// validate はExecute要求の前提を確かめる。
func (r ExecuteInstallRequest) validate() error {
	switch {
	case r.Plan.Kind != store.OperationInstall:
		return fmt.Errorf("app: operationが%sである（installだけを扱う）", r.Plan.Kind)
	case r.Inputs == nil:
		return errors.New("app: input供給元が未設定")
	case r.Engine == nil:
		return errors.New("app: install engineが未設定")
	case r.Locks == nil:
		return errors.New("app: LockManager portが未設定")
	}
	return nil
}

// checkInputs は§8手順3を行う。
func checkInputs(ctx context.Context, req ExecuteInstallRequest) *domain.Error {
	current, err := req.Inputs.PlanInputs(ctx)
	if err != nil {
		// 再取得できなければstaleかどうかを判定できない。**「変わっていない」と
		// 見なして進めない。**
		return &domain.Error{
			Code:      domain.CodeInternal,
			Retryable: true,
			Cause:     fmt.Errorf("app: `inputs`を再取得できない: %w", err),
		}
	}
	freshness := CheckPlanFreshness(req.Plan, current)
	if staleErr := StalePlanError(freshness); staleErr != nil {
		return staleErr
	}
	return nil
}

// acquireInstallLock は§8手順4のlockを取る。
//
// docs/02-architecture.md §12「installは導入単位のlock。ToolID、version、
// platform順で取る」。qualifierの並びがそのまま同一class内の取得順になる。
func acquireInstallLock(
	ctx context.Context, req ExecuteInstallRequest,
) (port.Lock, *domain.Error) {
	summary := req.Plan.Summary
	lock, err := req.Locks.Acquire(ctx, port.LockRequest{
		Class: port.ClassInstall,
		Qualifier: []string{
			summary.Tool.String(), summary.Version, summary.Platform.ID(),
		},
		Operation: req.Plan.Operation,
	})
	if err != nil {
		if errors.Is(err, port.ErrLockTimeout) {
			return nil, &domain.Error{
				Code:      domain.CodeLockTimeout,
				Retryable: true,
				Cause:     fmt.Errorf("app: install lockを取得できない: %w", err),
			}
		}
		// 順序違反は実装の誤りであり、待っても解消しない（[port.ErrLockOrder]）。
		return nil, domain.Internal(fmt.Errorf("app: install lockの取得に失敗した: %w", err))
	}
	return lock, nil
}

// engineError はengineのerrorを公開境界の型へ揃える。
func engineError(err error) *domain.Error {
	var typed *domain.Error
	if errors.As(err, &typed) {
		return typed
	}
	return domain.Internal(err)
}
