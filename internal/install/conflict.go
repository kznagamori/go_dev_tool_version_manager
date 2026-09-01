package install

import (
	"fmt"
	"reflect"
	"time"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/store"
)

// SameInstall は2つのreceiptが同じ導入を表すかを返す。
//
// docs/08-install-runtime.md §7「完成先が競合して作られた場合、両receiptと
// `command_targets`が完全一致すれば後発stagingを破棄して成功、違えば
// `E_CONFLICT`」。
//
// **install固有の識別子と時刻を比較から除く。** receiptは`install_id`
// （install毎のrandom 128 bit ID）と`installed_at`（時刻）を必須fieldに持つ。
// 文字どおり全fieldを比べると独立した2つのinstallでは絶対に一致せず、
// 「一致すれば成功」という条項が到達不能なdead codeになる。除くのは
// **同一内容でも定義上必ず異なる値**だけであり、判断の余地を残さない。
//
// | field | 比較 | 理由 |
// |---|---|---|
// | `install_id` | 除く | install毎のrandom値。同一内容でも必ず異なる |
// | `installed_at` | 除く | 導入完了時刻。同上 |
// | `probes[].finished_at` | 除く | probe終了時刻。同上 |
// | それ以外すべて | **含む** | `client_version`／`client_commit`も含む |
//
// `client_version`と`client_commit`を含めるのは、これらが「同一内容でも必ず
// 異なる」値ではないためである。同じclientを2回動かせば同じ値になる。異なる
// client版が書いたreceiptは競合として表面化させる。
//
// この除外はdocs/08-install-runtime.md §7へ明記した（本PRで仕様を同期した）。
func SameInstall(left, right store.Receipt) bool {
	return reflect.DeepEqual(comparableReceipt(left), comparableReceipt(right))
}

// ConflictReason は不一致の理由を返す。一致していれば空文字列を返す。
//
// 診断のために最初の不一致fieldを名指しする。`E_CONFLICT`のcauseへ入れて、
// 「何が違ったのか」を後から辿れるようにする。
func ConflictReason(left, right store.Receipt) string {
	if SameInstall(left, right) {
		return ""
	}
	comparisons := []struct {
		field string
		same  bool
	}{
		{"root_id", left.RootID == right.RootID},
		{"tool_id", left.Ref.Tool == right.Ref.Tool},
		{"version", left.Ref.Version == right.Ref.Version},
		{"platform_id", left.Ref.Platform == right.Ref.Platform},
		{"client_version", left.ClientVersion == right.ClientVersion},
		{"client_commit", left.ClientCommit == right.ClientCommit},
		{"definition_path", left.DefinitionPath == right.DefinitionPath},
		{"definition_sha256", left.DefinitionSHA256 == right.DefinitionSHA256},
		{"payload_path", left.PayloadPath == right.PayloadPath},
		{"artifact", reflect.DeepEqual(left.Artifact, right.Artifact)},
		{"storage", reflect.DeepEqual(
			emptyToNil(left.Storage), emptyToNil(right.Storage))},
		{"commands", reflect.DeepEqual(
			emptyToNil(left.Commands), emptyToNil(right.Commands))},
		{"environment_profiles", reflect.DeepEqual(
			emptyToNil(left.EnvironmentProfiles), emptyToNil(right.EnvironmentProfiles))},
		{"command_targets", reflect.DeepEqual(
			emptyToNil(left.CommandTargets), emptyToNil(right.CommandTargets))},
		{"probes", reflect.DeepEqual(
			comparableProbes(left.Probes), comparableProbes(right.Probes))},
	}
	for _, comparison := range comparisons {
		if !comparison.same {
			return comparison.field
		}
	}
	// [SameInstall]がfalseなのにここへ来るのは、上の列挙がreceiptのfieldを
	// 網羅していないことを意味する。`TestConflictReasonCoversEveryField`が
	// 件数を固定しているが、取りこぼしを黙って「一致」と報告しない。
	return "unknown"
}

// comparableReceipt は比較対象のfieldだけを持つ複製を返す。
//
// 除外fieldをzero値へ落とす。fieldを1つずつ拾う形にすると、receiptへfieldが
// 増えたときに比較から漏れる——**漏れた側は常に「一致」と判定される**ため、
// 違う導入を同一と誤認する。zero値へ落とす形なら、増えたfieldは自動的に
// 比較対象へ入る。
func comparableReceipt(value store.Receipt) store.Receipt {
	value.InstallID = ""
	value.InstalledAt = time.Time{}
	value.Probes = comparableProbes(value.Probes)
	// **空sliceとnilを同じに扱う。** TOMLを往復したreceiptは空arrayを長さ0の
	// sliceとして持ち、memory上で組み立てたreceiptはnilを持つ。区別すると、
	// diskのreceiptと今作ったreceiptが同一内容でも常に不一致になり、
	// §7の「一致すれば成功」が実際には到達しない。§14も「arrayはstorageだけ
	// 空可」として空と不在を区別していない。
	value.Storage = emptyToNil(value.Storage)
	value.Commands = emptyToNil(value.Commands)
	value.EnvironmentProfiles = emptyToNil(value.EnvironmentProfiles)
	value.CommandTargets = emptyToNil(value.CommandTargets)
	value.Probes = emptyToNil(value.Probes)
	return value
}

// emptyToNil は長さ0のsliceをnilへ落とす。
func emptyToNil[T any](values []T) []T {
	if len(values) == 0 {
		return nil
	}
	return values
}

// comparableProbes はprobeの終了時刻を落とした複製を返す。
func comparableProbes(probes []store.ReceiptProbe) []store.ReceiptProbe {
	if len(probes) == 0 {
		return nil
	}
	copied := make([]store.ReceiptProbe, len(probes))
	copy(copied, probes)
	for index := range copied {
		copied[index].FinishedAt = time.Time{}
	}
	return copied
}

// ConflictError は`E_CONFLICT`を作る。
func ConflictError(left, right store.Receipt) error {
	return fmt.Errorf(
		"install: 既存の導入と内容が一致しない（最初の不一致: %s）",
		ConflictReason(left, right))
}
