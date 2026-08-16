package catalog

import (
	"github.com/kznagamori/go_dev_tool_version_manager/internal/definition"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/progress"
)

// messageLifecycleOverrideUnused は未使用overrideのmessage IDである。
const messageLifecycleOverrideUnused = "catalog.lifecycle_override_unused"

// LifecycleOverrideWarnings は未使用のlifecycle overrideを警告へ変換する。
//
// docs/06-tool-definition.md §6.4が「matching source itemだけへ適用し、source
// にないoverrideはcatalog itemを合成せず`W_LIFECYCLE_OVERRIDE_UNUSED`として
// 報告する」と定める。docs/04-storage-and-data.md §16.2の`ResultWarningCode`
// exactly 5件のうちの1件である。
//
// **overrideのversionをcatalog itemにしない。** 上流に無いversionをoverride
// だけで作ると、installできないversionが`available`へ並ぶ。
//
// warningはoverride 1件につき1件とし、definitionの宣言順で返す。まとめて1件に
// すると、どのentryを直せばよいかがparameterから読めない。
func LifecycleOverrideWarnings(
	overrides []definition.LifecycleOverride, items []VersionItem,
) []progress.ResultWarning {
	versions := make([]domain.Version, 0, len(items))
	for _, item := range items {
		versions = append(versions, item.Version)
	}
	unused := UnusedOverrides(overrides, versions)
	if len(unused) == 0 {
		return nil
	}
	warnings := make([]progress.ResultWarning, 0, len(unused))
	for _, override := range unused {
		warnings = append(warnings, progress.ResultWarning{
			Code:      progress.WarnLifecycleOverrideUnused,
			MessageID: messageID(messageLifecycleOverrideUnused),
			Parameters: domain.Parameters{
				"version": domain.StringScalar(override.Version.String()),
				"status":  domain.StringScalar(string(override.Status)),
			},
		})
	}
	return warnings
}
