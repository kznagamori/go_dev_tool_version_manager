package definition

import (
	"fmt"
	"sort"
	"time"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
)

// LifecycleOverride は§6.4のexact version lifecycle上書きである。
//
// 4 key全件必須。sourceが返すlifecycleより優先する（§6.3の優先順位1）。
type LifecycleOverride struct {
	// Version は正規完全versionである。同一source内で一意。
	Version domain.Version
	// Status は上書きする状態である。`unknown`へは上書きできない。
	Status Lifecycle
	// Evidence はprovider/official projectのHTTPS一次資料である。
	Evidence string
	// AssessedAt は根拠を確認したUTC時刻である。
	AssessedAt time.Time
}

type overrideTable struct {
	Version    *string    `toml:"version"`
	Status     *string    `toml:"status"`
	Evidence   *string    `toml:"evidence"`
	AssessedAt *time.Time `toml:"assessed_at"`
}

// buildLifecycleOverrides は§6.4の`lifecycle_overrides`を検証する。
func buildLifecycleOverrides(
	raw *[]overrideTable, field string, scheme domain.VersionScheme, diagnostics *Diagnostics,
) []LifecycleOverride {
	if raw == nil {
		return nil
	}
	entries := *raw
	if len(entries) > OverrideMax {
		diagnostics.Add(field, reason(reasonLimit),
			fmt.Sprintf("lifecycle_overrideが%d件を超える（%d件）", OverrideMax, len(entries)))
		return nil
	}
	values := make([]LifecycleOverride, 0, len(entries))
	versions := make([]string, 0, len(entries))
	for index := range entries {
		scope := fmt.Sprintf("%s[%d]", field, index)
		value, ok := buildLifecycleOverride(&entries[index], scope, scheme, diagnostics)
		if !ok {
			continue
		}
		values = append(values, value)
		versions = append(versions, value.Version.String())
	}
	// 同じversionへ2件のoverrideがあると、どちらを適用するかがdefinitionから
	// 決まらない（§6.4「同一source内で一意」）。
	if err := requireUniqueIdentifiers("lifecycle_override version", versions); err != nil {
		diagnostics.Add(field, reason(reasonDuplicate), err.Error())
		return nil
	}
	return values
}

func buildLifecycleOverride(
	table *overrideTable, field string, scheme domain.VersionScheme, diagnostics *Diagnostics,
) (LifecycleOverride, bool) {
	var value LifecycleOverride
	ok := true

	version, versionOK := requireExactVersion(table.Version, field+".version", scheme, diagnostics)
	if !versionOK {
		ok = false
	}
	value.Version = version

	switch {
	case table.Status == nil:
		diagnostics.Add(field+".status", reason(reasonMissing), "`status`が無い")
		ok = false
	// §6.4はstatusを`supported|eol`の2値へ限る。`unknown`は「判断していない」で
	// あり、根拠を添えて上書きする対象にならない。
	case Lifecycle(*table.Status) != LifecycleSupported && Lifecycle(*table.Status) != LifecycleEOL:
		diagnostics.Add(field+".status", reason(reasonEnum),
			fmt.Sprintf("statusは%s|%sだけ（%q）", LifecycleSupported, LifecycleEOL, *table.Status))
		ok = false
	default:
		value.Status = Lifecycle(*table.Status)
	}

	value.Evidence = requireHTTPSURL(table.Evidence, field+".evidence", urlReference, diagnostics)
	if value.Evidence == "" {
		ok = false
	}
	assessedAt, timeOK := requireUTCTime(table.AssessedAt, field+".assessed_at", diagnostics)
	if !timeOK {
		ok = false
	}
	value.AssessedAt = assessedAt
	return value, ok
}

// OverrideMax はlifecycle overrideの上限である。
//
// docs/04-storage-and-data.md §21の「lifecycle override / static version /
// static asset per version 10,000 / 10,000 / 16」。
const OverrideMax = 10000

// buildLifecycleMap は§6.1の`lifecycle_map`を検証する。
//
// `required`はlifecycle pointerのどちらかが宣言されているかである。片方だけの
// 宣言を拒否するのは、mapの無いpointerが全itemをsource errorにするだけで
// lifecycleを決められず、pointerの無いmapが参照されないためである。
func buildLifecycleMap(
	raw *map[string]string, field string, required bool, diagnostics *Diagnostics,
) map[string]Lifecycle {
	if raw == nil {
		if required {
			diagnostics.Add(field, reason(reasonConditional),
				"lifecycle pointerを宣言した場合は`lifecycle_map`が必須")
		}
		return nil
	}
	if !required {
		diagnostics.Add(field, reason(reasonConditional),
			"`lifecycle_map`はlifecycle pointerと組でだけ宣言できる")
		return nil
	}
	source := *raw
	if len(source) == 0 {
		diagnostics.Add(field, reason(reasonConditional), "`lifecycle_map`が空table")
		return nil
	}
	if len(source) > ArrayMax {
		diagnostics.Add(field, reason(reasonLimit),
			fmt.Sprintf("lifecycle_mapが%d件を超える（%d件）", ArrayMax, len(source)))
		return nil
	}
	// mapの反復順は不定である。診断の順序を宣言内容だけで決めるためkeyでsortする。
	keys := make([]string, 0, len(source))
	for key := range source {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	value := make(map[string]Lifecycle, len(source))
	for _, key := range keys {
		if key == "" {
			diagnostics.Add(field, reason(reasonText), "`lifecycle_map`のkeyが空")
			return nil
		}
		lifecycle, err := parseLifecycle(source[key])
		if err != nil {
			diagnostics.Add(field+"."+key, reason(reasonEnum), err.Error())
			return nil
		}
		value[key] = lifecycle
	}
	return value
}

// parseLifecycle は§6.1のlifecycle 3値を読む。
func parseLifecycle(text string) (Lifecycle, error) {
	switch Lifecycle(text) {
	case LifecycleSupported, LifecycleEOL, LifecycleUnknown:
		return Lifecycle(text), nil
	default:
		return "", fmt.Errorf("lifecycleは%s|%s|%sだけ（%q）",
			LifecycleSupported, LifecycleEOL, LifecycleUnknown, text)
	}
}

// parseChannel は§6.1のchannel 2値を読む。
func parseChannel(text string) (Channel, error) {
	switch Channel(text) {
	case ChannelStable, ChannelPrerelease:
		return Channel(text), nil
	default:
		return "", fmt.Errorf("channelは%s|%sだけ（%q）", ChannelStable, ChannelPrerelease, text)
	}
}

// requireExactVersion は正規完全versionを検査する（§6.4・§6.6）。
//
// tool schemeのgrammarへ完全一致させる。schemeが決まっていない場合（`[tool]`側の
// 診断が既に出ている場合）は、ここで追加の診断を出さない。同じ原因で2件の
// errorを出すと、どちらを直せばよいかが読み取れない。
func requireExactVersion(
	raw *string, field string, scheme domain.VersionScheme, diagnostics *Diagnostics,
) (domain.Version, bool) {
	if raw == nil {
		diagnostics.Add(field, reason(reasonMissing), fmt.Sprintf("`%s`が無い", field))
		return domain.Version{}, false
	}
	if scheme == "" {
		return domain.Version{}, false
	}
	version, err := domain.ParseVersion(scheme, *raw)
	if err != nil {
		diagnostics.Add(field, reason(reasonVersion), err.Error())
		return domain.Version{}, false
	}
	return version, true
}

// requireUTCTime はUTC RFC 3339の時刻を検査する（§6.4・§6.6）。
//
// go-tomlはoffset date-timeをtime.Timeへ読む。offsetがUTCでない値を受けると、
// 同じ時刻が別の文字列でregistryへ入り、diffが読みにくくなる。
func requireUTCTime(raw *time.Time, field string, diagnostics *Diagnostics) (time.Time, bool) {
	if raw == nil {
		diagnostics.Add(field, reason(reasonMissing), fmt.Sprintf("`%s`が無い", field))
		return time.Time{}, false
	}
	value := *raw
	if value.IsZero() {
		diagnostics.Add(field, reason(reasonTime), fmt.Sprintf("%sが未設定の時刻", field))
		return time.Time{}, false
	}
	if _, offset := value.Zone(); offset != 0 {
		diagnostics.Add(field, reason(reasonTime),
			fmt.Sprintf("%sはUTCでなければならない（offset %d秒）", field, offset))
		return time.Time{}, false
	}
	return value.UTC(), true
}
