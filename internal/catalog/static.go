package catalog

import (
	"fmt"
	"sort"
	"time"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/definition"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
)

// BuildStaticItems は§6.6の`static_versions`からversion itemを作る。
//
// networkを使わない。`static_versions`と`max_items`だけを使い、他のpointerは
// definitionのschema検証が禁止している（§6.1）。
//
// **file記載順で解釈しない。** §6.6が「正規version byteで一意検査して
// comparison keyでsortする」と定める。一意検査はdefinitionが済ませているため、
// ここではcomparison keyでsortする。記載順に依存すると、registryのdiffで行を
// 並べ替えただけでcatalogの内容が変わる。
func BuildStaticItems(source definition.VersionSource) ([]VersionItem, *domain.Error) {
	origin := "static_versions"
	if limitErr := CheckItemLimit(len(source.StaticVersions), source, origin); limitErr != nil {
		return nil, limitErr
	}

	items := make([]VersionItem, 0, len(source.StaticVersions))
	for index := range source.StaticVersions {
		entry := &source.StaticVersions[index]
		item, err := buildStaticItem(entry)
		if err != nil {
			return nil, sourceError(fmt.Sprintf("%s[%d]", origin, index), err)
		}
		items = append(items, item)
	}
	if err := sortByComparisonKey(items); err != nil {
		return nil, sourceError(origin, err)
	}
	return items, nil
}

// buildStaticItem は1件のstatic versionをversion itemへ変換する。
func buildStaticItem(entry *definition.StaticVersion) (VersionItem, error) {
	channel, err := staticChannel(entry.Channel)
	if err != nil {
		return VersionItem{}, err
	}
	published := ""
	if !entry.PublishedAt.IsZero() {
		published = entry.PublishedAt.UTC().Format(time.RFC3339)
	}
	return VersionItem{
		Version:     entry.Version,
		RawVersion:  entry.Version.String(),
		Channel:     channel,
		PublishedAt: published,
		// static sourceはitem自身がlifecycleを書き、§6.4がoverrideを禁じる。
		// §6.3の3段の優先順位を通らないため、根拠を専用の値で表す。
		Lifecycle: LifecycleDecision{
			Lifecycle: domain.Lifecycle(entry.Lifecycle), From: LifecycleFromStatic,
		},
		Static: entry,
	}, nil
}

// staticChannel はdefinitionのchannelをdomainのchannelへ移す。
//
// 両者は同じ2値だが型が別である。definitionの値をそのまま公開境界へ流さず、
// domain側のenumを通すことで、片方が増えたときにcompileで気付ける。
func staticChannel(value definition.Channel) (domain.Channel, error) {
	switch value {
	case definition.ChannelStable:
		return domain.ChannelStable, nil
	case definition.ChannelPrerelease:
		return domain.ChannelPrerelease, nil
	default:
		return "", fmt.Errorf("channelは%s|%sだけ（%q）",
			definition.ChannelStable, definition.ChannelPrerelease, value)
	}
}

// sortByComparisonKey はversion itemをcomparison key昇順へ並べ替える。
//
// §6.6のstatic source向けだが、比較そのものは§4の規則である。同一schemeの
// version同士しか比較できないため、schemeが混ざっていればerrorにする。
//
// catalog JSONの並び（§15の「comparison降順、同値ならversion byte順」）は
// catalog組立て側が行う。ここでは記載順への依存を断つことだけを目的にする。
func sortByComparisonKey(items []VersionItem) error {
	var failure error
	sort.SliceStable(items, func(left, right int) bool {
		order, err := items[left].Version.Compare(items[right].Version)
		if err != nil {
			// sortの比較関数からerrorを返せないため記録して継続する。
			// 途中で打ち切ると並びが中途半端なまま返るため、最後に判定する。
			if failure == nil {
				failure = err
			}
			return false
		}
		return order < 0
	})
	return failure
}
