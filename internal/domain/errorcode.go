package domain

import "fmt"

// ErrorCode は公開境界へ出すstable error codeである（docs/02-architecture.md §14）。
//
// v0.1の閉じた集合はdocs/03-cli.md §7の終了code表と
// docs/09-platform.md §9のplatform error表の和集合であり、後者は前者の部分集合の
// ため実体は§7の34件である。同じ失敗条件はCLI human、CLI JSON、shimで同じcodeに
// する。未分類codeを公開境界へ返さない。
type ErrorCode string

// ErrorCode のexactly 34値。docs/03-cli.md §7の終了code表と1対1で対応する。
const (
	// 終了code 1: internal/unknown。
	CodeInternal ErrorCode = "E_INTERNAL"

	// 終了code 2: 使い方。
	CodeUsage ErrorCode = "E_USAGE"

	// 終了code 3: 設定。
	CodeConfigInvalid        ErrorCode = "E_CONFIG_INVALID"
	CodeProjectConfigInvalid ErrorCode = "E_PROJECT_CONFIG_INVALID"

	// 終了code 4: 対象不明・不正。
	CodeToolUnknown         ErrorCode = "E_TOOL_UNKNOWN"
	CodeVersionInvalid      ErrorCode = "E_VERSION_INVALID"
	CodeVersionNotFound     ErrorCode = "E_VERSION_NOT_FOUND"
	CodeVersionNotInstalled ErrorCode = "E_VERSION_NOT_INSTALLED"
	CodePlatformUnsupported ErrorCode = "E_PLATFORM_UNSUPPORTED"
	CodeUnsupportedShell    ErrorCode = "E_UNSUPPORTED_SHELL"

	// 終了code 5: network。
	CodeNetwork        ErrorCode = "E_NETWORK"
	CodeOffline        ErrorCode = "E_OFFLINE"
	CodeCatalogMissing ErrorCode = "E_CATALOG_MISSING"

	// 終了code 6: 完全性。
	CodeChecksumMismatch  ErrorCode = "E_CHECKSUM_MISMATCH"
	CodeRegistryInvalid   ErrorCode = "E_REGISTRY_INVALID"
	CodeDefinitionInvalid ErrorCode = "E_DEFINITION_INVALID"
	CodeArchiveUnsafe     ErrorCode = "E_ARCHIVE_UNSAFE"
	CodeProbeFailed       ErrorCode = "E_PROBE_FAILED"

	// 終了code 7: filesystem/権限/path。
	CodePermission            ErrorCode = "E_PERMISSION"
	CodeFilesystem            ErrorCode = "E_FILESYSTEM"
	CodePathConflict          ErrorCode = "E_PATH_CONFLICT"
	CodePathUnsafe            ErrorCode = "E_PATH_UNSAFE"
	CodeLinkFailed            ErrorCode = "E_LINK_FAILED"
	CodeShellProfileConflict  ErrorCode = "E_SHELL_PROFILE_CONFLICT"
	CodePathIntegrationFailed ErrorCode = "E_PATH_INTEGRATION_FAILED"

	// 終了code 8: 競合。
	CodeLockTimeout   ErrorCode = "E_LOCK_TIMEOUT"
	CodePlanStale     ErrorCode = "E_PLAN_STALE"
	CodeStateConflict ErrorCode = "E_STATE_CONFLICT"
	CodeConflict      ErrorCode = "E_CONFLICT"

	// 終了code 9: 状態破損。
	CodeStateCorrupt   ErrorCode = "E_STATE_CORRUPT"
	CodeReceiptInvalid ErrorCode = "E_RECEIPT_INVALID"

	// 終了code 10: 中断。
	CodeCancelled ErrorCode = "E_CANCELLED"

	// 終了code 11: 承認。
	CodeApprovalRequired ErrorCode = "E_APPROVAL_REQUIRED"
	CodeApprovalDenied   ErrorCode = "E_APPROVAL_DENIED"
)

// ErrorCodeCount はdocs/03-cli.md §7が定めるerror code数である。
//
// 「全34 error code」という仕様の記述と実装を突き合わせるために定数で持つ。
const ErrorCodeCount = 34

// ExitSuccess は成功の終了codeである。変更不要だった場合を含む（docs/03-cli.md §7）。
const ExitSuccess = 0

// ExitDoctorUnhealthy は`doctor`が`status=unhealthy`を返したときの終了codeである。
//
// これは診断operationの失敗ではないためerror objectを作らない。[ErrorCode]からは
// 到達せず、`doctor`の集約結果からCLIが直接返す（docs/03-cli.md §7）。
const ExitDoctorUnhealthy = 12

// exitCodes はerror codeから終了codeへのexactly 1件の写像である。
//
// docs/03-cli.md §7の表をそのまま持つ。分類ではなく表で持つのは、codeの命名から
// 終了codeを推測すると表の変更に追随できないためである。
var exitCodes = map[ErrorCode]int{
	CodeInternal: 1,

	CodeUsage: 2,

	CodeConfigInvalid:        3,
	CodeProjectConfigInvalid: 3,

	CodeToolUnknown:         4,
	CodeVersionInvalid:      4,
	CodeVersionNotFound:     4,
	CodeVersionNotInstalled: 4,
	CodePlatformUnsupported: 4,
	CodeUnsupportedShell:    4,

	CodeNetwork:        5,
	CodeOffline:        5,
	CodeCatalogMissing: 5,

	CodeChecksumMismatch:  6,
	CodeRegistryInvalid:   6,
	CodeDefinitionInvalid: 6,
	CodeArchiveUnsafe:     6,
	CodeProbeFailed:       6,

	CodePermission:            7,
	CodeFilesystem:            7,
	CodePathConflict:          7,
	CodePathUnsafe:            7,
	CodeLinkFailed:            7,
	CodeShellProfileConflict:  7,
	CodePathIntegrationFailed: 7,

	CodeLockTimeout:   8,
	CodePlanStale:     8,
	CodeStateConflict: 8,
	CodeConflict:      8,

	CodeStateCorrupt:   9,
	CodeReceiptInvalid: 9,

	CodeCancelled: 10,

	CodeApprovalRequired: 11,
	CodeApprovalDenied:   11,
}

// ParseErrorCode は文字列をErrorCodeへ変換する。
//
// 閉じた34件以外を拒否する。公開境界へ未分類codeを出さないためである（§14）。
func ParseErrorCode(text string) (ErrorCode, error) {
	code := ErrorCode(text)
	if _, ok := exitCodes[code]; !ok {
		return "", fmt.Errorf("domain: error code %q はdocs/03-cli.md §7の34件に含まれない", text)
	}
	return code, nil
}

// ExitCode はerror codeに対応する終了codeを返す。
//
// 閉じた集合の外はfail closedで扱い、`E_INTERNAL`の終了code 1へ落とす。
// §14が「未分類codeを公開境界へ返さない。想定外の内部失敗だけは公開code
// `E_INTERNAL`、終了code 1へ変換する」と定めるためである。
func (c ErrorCode) ExitCode() int {
	if exit, ok := exitCodes[c]; ok {
		return exit
	}
	return exitCodes[CodeInternal]
}

// IsKnown は閉じた34件に含まれるかどうかを返す。
func (c ErrorCode) IsKnown() bool {
	_, ok := exitCodes[c]
	return ok
}

// String はstable codeの文字列表現を返す。
func (c ErrorCode) String() string { return string(c) }

// AllErrorCodes は全error codeをdocs/03-cli.md §7の表順で返す。
//
// 表順はexit code昇順、同一exit code内は§7の表内の並びとする。message catalogの
// 網羅性検査や文書生成が、mapのiteration順に依存せず全codeを列挙できるようにする。
func AllErrorCodes() []ErrorCode {
	return []ErrorCode{
		CodeInternal,
		CodeUsage,
		CodeConfigInvalid, CodeProjectConfigInvalid,
		CodeToolUnknown, CodeVersionInvalid, CodeVersionNotFound,
		CodeVersionNotInstalled, CodePlatformUnsupported, CodeUnsupportedShell,
		CodeNetwork, CodeOffline, CodeCatalogMissing,
		CodeChecksumMismatch, CodeRegistryInvalid, CodeDefinitionInvalid,
		CodeArchiveUnsafe, CodeProbeFailed,
		CodePermission, CodeFilesystem, CodePathConflict, CodePathUnsafe,
		CodeLinkFailed, CodeShellProfileConflict, CodePathIntegrationFailed,
		CodeLockTimeout, CodePlanStale, CodeStateConflict, CodeConflict,
		CodeStateCorrupt, CodeReceiptInvalid,
		CodeCancelled,
		CodeApprovalRequired, CodeApprovalDenied,
	}
}
