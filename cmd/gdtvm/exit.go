package main

import (
	"errors"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
)

// 終了コード（04章7節）。CLI 薄層の責務として CoreError を終了コードへ変換する。
const (
	exitSuccess     = 0  // 成功、または変更不要
	exitGeneral     = 1  // 一般的操作失敗
	exitUsage       = 2  // CLI usage/構文エラー
	exitConfig      = 3  // 設定/definition schema エラー
	exitNotFound    = 4  // tool/version/platform が見つからない
	exitNetwork     = 5  // network/offline/cache 不足
	exitSecurity    = 6  // digest/signature/security policy 違反
	exitFilesystem  = 7  // 権限/link/filesystem 失敗
	exitProcess     = 8  // 外部 process/hook 失敗
	exitLock        = 9  // lock 競合/timeout
	exitCancelled   = 10 // 操作 cancel
	exitPartial     = 11 // 部分成功
	exitDoctorError = 12 // doctor が error 項目を検出（E_* を持たない）
)

// exitCodeByErrorCode は 04章7節の E_* → 終了コード対応表である。E_TIMEOUT は文脈
// 依存のため本表に含めず、exitCode で category を見て決める。
var exitCodeByErrorCode = map[domain.ErrorCode]int{
	domain.CodeUsage:                exitUsage,
	domain.CodeConfigParse:          exitConfig,
	domain.CodeConfigSchema:         exitConfig,
	domain.CodeRegistryInvalid:      exitConfig,
	domain.CodeDependencyCycle:      exitConfig,
	domain.CodeToolUnknown:          exitNotFound,
	domain.CodeVersionInvalid:       exitNotFound,
	domain.CodeVersionNotFound:      exitNotFound,
	domain.CodeVersionNotInstalled:  exitNotFound,
	domain.CodePlatformUnsupported:  exitNotFound,
	domain.CodeCatalogMissing:       exitNetwork,
	domain.CodeOffline:              exitNetwork,
	domain.CodeNetwork:              exitNetwork,
	domain.CodeDigestMismatch:       exitSecurity,
	domain.CodeSignatureInvalid:     exitSecurity,
	domain.CodePolicyDenied:         exitSecurity,
	domain.CodeArchiveUnsafe:        exitSecurity,
	domain.CodeDefinitionUnapproved: exitSecurity,
	domain.CodeHomeNotWritable:      exitFilesystem,
	domain.CodeLinkFailed:           exitFilesystem,
	domain.CodeProcessFailed:        exitProcess,
	domain.CodeLocked:               exitLock,
	domain.CodeCancelled:            exitCancelled,
	domain.CodePartial:              exitPartial,
	// 以下は code 1（一般的操作失敗）へ集約する（04章7節）。
	domain.CodeStateCorrupt:       exitGeneral,
	domain.CodePlanStale:          exitGeneral,
	domain.CodeCommandAmbiguous:   exitGeneral,
	domain.CodeUpdateFailed:       exitGeneral,
	domain.CodeDependencyConflict: exitGeneral,
}

// exitCode は err を 04章7節の終了コードへ変換する。nil は成功、CoreError は code
// （E_TIMEOUT は category）で対応させ、それ以外の error は一般失敗とする。doctor の
// code 12 は診断結果から CLI が別途決定する。
func exitCode(err error) int {
	if err == nil {
		return exitSuccess
	}
	var ce *domain.CoreError
	if errors.As(err, &ce) {
		if ce.Code() == domain.CodeTimeout {
			// network 起因は 5、外部 process/hook 起因は 8（04章7節）。既定は network。
			if ce.Category() == domain.CategoryProcess {
				return exitProcess
			}
			return exitNetwork
		}
		if code, ok := exitCodeByErrorCode[ce.Code()]; ok {
			return code
		}
		return exitGeneral
	}
	return exitGeneral
}
