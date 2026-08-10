package progress

import (
	"fmt"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
)

// ResultWarningCode は処理結果用のwarning codeである
// （docs/04-storage-and-data.md §16.2）。
//
// Plan approvalには使わない。approvalの単位は§16.1の`PlanWarningCode`であり、
// 両者を混ぜるとsecurity failureが承認可能な警告として扱われうる。
type ResultWarningCode string

// ResultWarningCode のexactly 5値。§16.2で閉じている。
const (
	WarnCacheStale                 ResultWarningCode = "W_CACHE_STALE"
	WarnCleanupIncomplete          ResultWarningCode = "W_CLEANUP_INCOMPLETE"
	WarnSelectionLinkInconsistent  ResultWarningCode = "W_SELECTION_LINK_INCONSISTENT"
	WarnEnvironmentNotificationErr ResultWarningCode = "W_ENVIRONMENT_NOTIFICATION_FAILED"
	WarnLifecycleOverrideUnused    ResultWarningCode = "W_LIFECYCLE_OVERRIDE_UNUSED"
)

// ResultWarningCodeCount は§16.2が定めるcode数である。
const ResultWarningCodeCount = 5

var resultWarningCodes = map[ResultWarningCode]struct{}{
	WarnCacheStale: {}, WarnCleanupIncomplete: {}, WarnSelectionLinkInconsistent: {},
	WarnEnvironmentNotificationErr: {}, WarnLifecycleOverrideUnused: {},
}

// ParseResultWarningCode は文字列をResultWarningCodeへ変換する。
func ParseResultWarningCode(text string) (ResultWarningCode, error) {
	code := ResultWarningCode(text)
	if _, ok := resultWarningCodes[code]; !ok {
		return "", fmt.Errorf("progress: result warning code %q は§16.2の5値に含まれない", text)
	}
	return code, nil
}

// AllResultWarningCodes は§16.2の表順で全codeを返す。
func AllResultWarningCodes() []ResultWarningCode {
	return []ResultWarningCode{
		WarnCacheStale, WarnCleanupIncomplete, WarnSelectionLinkInconsistent,
		WarnEnvironmentNotificationErr, WarnLifecycleOverrideUnused,
	}
}

// ResultWarning は`ResultMeta.Warnings`とCLI JSON envelopeの`warnings` entryである
// （docs/04-storage-and-data.md §16.2・§17）。
//
// human表示、`ResultMeta.Warnings`、JSON envelopeはこの同じ値から生成する。
// security failureをこの型へ格下げしない。
type ResultWarning struct {
	Code       ResultWarningCode
	MessageID  domain.MessageID
	Parameters domain.Parameters
}

// Validate は公開境界へ出す前の不変条件を検査する。
func (w ResultWarning) Validate() error {
	if _, ok := resultWarningCodes[w.Code]; !ok {
		return fmt.Errorf("progress: result warning code %q は§16.2の5値に含まれない", w.Code)
	}
	if w.MessageID.IsZero() {
		return fmt.Errorf("progress: result warning %s のmessage IDが未設定", w.Code)
	}
	return w.Parameters.Validate()
}
