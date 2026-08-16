package catalog

import (
	"fmt"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/definition"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
)

// DeriveChannel は`channel_pointer`を省略したsourceのchannelを決める。
//
// docs/06-tool-definition.md §6.1が「`channel_pointer`を省略した場合、正規
// versionが各schemeのprerelease構文を持てば`prerelease`、それ以外は`stable`と
// する」と定める。
//
// **構文だけで決める。** version番号の大小、公開日、上流の表記からprereleaseを
// 推測しない。`channel_pointer`を宣言したsourceはこの関数を使わず、pointer先の
// 値を写像する（[MapChannel]）。
func DeriveChannel(version domain.Version) (domain.Channel, error) {
	if version.IsZero() {
		return "", fmt.Errorf("catalog: 未初期化のversionからchannelを決められない")
	}
	if version.IsPrerelease() {
		return domain.ChannelPrerelease, nil
	}
	return domain.ChannelStable, nil
}

// MapChannel は`channel_pointer`が読んだscalarをchannelへ写像する。
//
// §6.1が「指定したpointer先はstringの`stable|prerelease`、またはbooleanだけを
// 受け、booleanは`true`を`stable`、`false`を`prerelease`へ写像する。数値や
// 文字列への暗黙変換、未知stringのfallbackを行わない」と定める。
//
// booleanの真がstableである点に注意する。Goの`https://go.dev/dl/`が
// `"stable": true`で正式版を示すためであり、真偽の向きを逆に取ると全versionの
// channelが反転する。
func MapChannel(value domain.Scalar) (domain.Channel, error) {
	switch value.Kind() {
	case domain.ScalarString:
		text, _ := value.Str()
		switch domain.Channel(text) {
		case domain.ChannelStable, domain.ChannelPrerelease:
			return domain.Channel(text), nil
		default:
			return "", fmt.Errorf(
				"catalog: channelは%q|%qだけ（%q）", domain.ChannelStable, domain.ChannelPrerelease, text)
		}
	case domain.ScalarBool:
		stable, _ := value.Bool()
		if stable {
			return domain.ChannelStable, nil
		}
		return domain.ChannelPrerelease, nil
	default:
		// 数値やnullを暗黙変換しない。上流が型を変えたらsource errorにして
		// live smokeで気付く（§6.1）。
		return "", fmt.Errorf("catalog: channel値はstringかbooleanだけ（kind %d）", value.Kind())
	}
}

// LifecycleSource はlifecycleをどこから決めたかである。
//
// `doctor`とcatalogのevidence欄が根拠を示すために使う
// （docs/04-storage-and-data.md §15）。
type LifecycleSource string

// LifecycleSource の値。前3件は§6.3の優先順位と1対1で対応する。
const (
	// LifecycleFromOverride は§6.4のexact version overrideで決まった状態である。
	LifecycleFromOverride LifecycleSource = "override"
	// LifecycleFromSource は上流の値を`lifecycle_map`で写像して決まった状態である。
	LifecycleFromSource LifecycleSource = "source"
	// LifecycleFromDefault はどれも無く`unknown`になった状態である。
	LifecycleFromDefault LifecycleSource = "default"
	// LifecycleFromStatic は§6.6のstatic version item自身が書いた状態である。
	//
	// §6.3の優先順位はnetwork sourceの規則である。static sourceはitem自身へ
	// lifecycleを書き（§6.6）、§6.4がoverrideを禁じるため、3段の優先順位を
	// 通らない。同じ`source`で表すと、上流から読んだ値とdefinitionへ書いた値の
	// 区別が`doctor`のevidence欄で付かなくなる。
	LifecycleFromStatic LifecycleSource = "static"
)

// LifecycleDecision はlifecycleとその根拠である。
type LifecycleDecision struct {
	// Lifecycle は決まった状態である。
	Lifecycle domain.Lifecycle
	// From はどの優先順位で決まったかである。
	From LifecycleSource
}

// ResolveLifecycle は§6.3の優先順位でlifecycleを決める。
//
//  1. §6.4のexact version override
//  2. lifecycle pointerが読んだ値を`lifecycle_map`で写像した結果
//  3. どれも無ければ`unknown`
//
// **公開日やversionの古さからEOLを推測しない**（同§）。`mapped`がnilは「pointer
// を宣言していない、または写像結果が無い」を表す。写像そのものは[MapLifecycle]が
// 行い、mapに無い値はそこでsource errorになる。
//
// overrideとsourceの値が食い違う場合はerrorにする。§6.4が「source lifecycle
// fieldと同じversionで矛盾するoverrideを拒否する」と定めるためで、この検査は
// source値が判明するここでしかできない（definition parse時には上流の値が無い）。
// 優先順位1で黙って勝たせると、上流がsupportedへ戻したのに古いeol overrideが
// 残っていることに誰も気付けない。
func ResolveLifecycle(
	overrides []definition.LifecycleOverride, version domain.Version, mapped *domain.Lifecycle,
) (LifecycleDecision, error) {
	if version.IsZero() {
		return LifecycleDecision{}, fmt.Errorf("catalog: 未初期化のversionのlifecycleは決められない")
	}
	// 優先順位1。overrideは正規version文字列の完全一致で照合する。comparison key
	// へ変換した近似一致をしない（§4）。`1.20`と`1.20.0`は同じcomparison keyだが
	// 別の正規文字列であり、片方のoverrideをもう片方へ適用しない。
	for _, override := range overrides {
		if override.Version.String() != version.String() {
			continue
		}
		status := domain.Lifecycle(override.Status)
		if mapped != nil && *mapped != status {
			return LifecycleDecision{}, fmt.Errorf(
				"catalog: version %s のlifecycle overrideがsourceの値と矛盾する（override %q / source %q）",
				version, status, *mapped)
		}
		return LifecycleDecision{Lifecycle: status, From: LifecycleFromOverride}, nil
	}
	if mapped != nil {
		return LifecycleDecision{Lifecycle: *mapped, From: LifecycleFromSource}, nil
	}
	return LifecycleDecision{Lifecycle: domain.LifecycleUnknown, From: LifecycleFromDefault}, nil
}

// MapLifecycle は上流のstring値を`lifecycle_map`で写像する。
//
// §6.1が「**mapに無い値はsource error**とし、黙って`unknown`へ倒さない。上流が
// enum値を増やした場合にlive smokeで検出するための規定である」と定める。
func MapLifecycle(
	table map[string]definition.Lifecycle, value string,
) (domain.Lifecycle, error) {
	if len(table) == 0 {
		return "", fmt.Errorf("catalog: `lifecycle_map`が無い状態でlifecycle値 %q を読んだ", value)
	}
	mapped, found := table[value]
	if !found {
		return "", fmt.Errorf("catalog: lifecycle値 %q が`lifecycle_map`に無い", value)
	}
	return domain.Lifecycle(mapped), nil
}

// UnusedOverrides はsourceに現れなかったoverrideを返す。
//
// §6.4が「matching source itemだけへ適用し、sourceにないoverrideはcatalog item
// を合成せず`W_LIFECYCLE_OVERRIDE_UNUSED`として報告する」と定める。
// 未使用のoverrideを黙って捨てると、上流でversionが消えたことに気付けない。
//
// 戻り値はdefinitionの宣言順である。並べ替えると、どのentryを直せばよいかが
// definitionと突き合わせにくくなる。
func UnusedOverrides(
	overrides []definition.LifecycleOverride, versions []domain.Version,
) []definition.LifecycleOverride {
	present := make(map[string]struct{}, len(versions))
	for _, version := range versions {
		if !version.IsZero() {
			present[version.String()] = struct{}{}
		}
	}
	var unused []definition.LifecycleOverride
	for _, override := range overrides {
		if _, found := present[override.Version.String()]; !found {
			unused = append(unused, override)
		}
	}
	return unused
}
