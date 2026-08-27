package catalog

import (
	"errors"
	"fmt"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/store"
)

// Resolution は解決した1 itemである。
type Resolution struct {
	// Item は解決したcatalog itemである。
	Item store.CatalogItem
	// Stale は期限切れcatalogから解決したかを表す。
	//
	// trueのとき、呼出し側はdocs/04-storage-and-data.md §16.2の`W_CACHE_STALE`を
	// 結果warningへ載せる。warningの組立てはresult側の責務のため、ここでは事実だけ
	// を返す。
	Stale bool
}

// ResolveExact はcatalogから完全versionをbyte完全一致で探す。
//
// docs/08-install-runtime.md §3.1手順3「入力をtrim、補完、range展開せず、catalogの
// 正規version文字列と**byte完全一致**で探す」。`22.18`を`22.18.0`へ補完しない。
// 該当しなければ`E_VERSION_NOT_FOUND`とし、近似versionを自動提案・選択しない。
//
// docs/02-architecture.md §3も「入力一致はcomparison keyではなく、catalogに保存
// された正規完全versionのbyte完全一致とする」と定める。comparison keyで比べると、
// `1.0`と`1.0.0`のように表記の違うものが同じversionとして通る。
func ResolveExact(catalog store.Catalog, input string) (Resolution, *domain.Error) {
	if input == "" {
		return Resolution{}, versionNotFound(catalog, errors.New("versionが空"))
	}
	for index := range catalog.Items {
		item := catalog.Items[index]
		if item.VersionText != input {
			continue
		}
		if !item.Installable {
			return Resolution{}, platformUnsupported(fmt.Errorf(
				"catalog: version %q は現在のplatform %s で導入できない（%s）",
				input, catalog.Platform.ID(), item.UnavailableReason))
		}
		return Resolution{Item: item}, nil
	}
	return Resolution{}, versionNotFound(catalog, fmt.Errorf("version %q が無い", input))
}

// ResolveLatest はcatalogから`--latest`の1件を選ぶ。
//
// docs/08-install-runtime.md §3.2「channel=stable、lifecycle!=eol、かつ現platformで
// installableなversionだけをtoolのversion schemeで比較し、最大の完全version 1件へ
// 解決する。候補0件・比較不能・**同順位複数**は失敗する」。
//
// catalog itemは§15で「version comparison降順、同値ならversion byte順」に並ぶ。
// 先頭から条件を満たす最初のitemが最大である。同順位複数の検出のため、選んだ後に
// 次のitemが同順位でないことを確かめる。
func ResolveLatest(catalog store.Catalog) (Resolution, *domain.Error) {
	candidates := make([]store.CatalogItem, 0, len(catalog.Items))
	for index := range catalog.Items {
		if isLatestCandidate(catalog.Items[index]) {
			candidates = append(candidates, catalog.Items[index])
		}
	}
	if len(candidates) == 0 {
		return Resolution{}, versionNotFound(catalog, errors.New(
			"catalog: stableかつEOLでない導入可能なversionが無い"))
	}
	best := candidates[0]
	// 同順位複数を拒否する。catalogは降順に並ぶため、2件目が同順位なら
	// 「最大が1件に決まらない」ということである。
	if len(candidates) > 1 {
		order, err := best.Version.Compare(candidates[1].Version)
		if err != nil {
			return Resolution{}, versionNotFound(catalog, fmt.Errorf(
				"catalog: versionを比較できない: %w", err))
		}
		if order == 0 {
			return Resolution{}, versionNotFound(catalog, fmt.Errorf(
				"catalog: 最大versionが1件に決まらない（%q と %q）",
				best.Version.String(), candidates[1].Version.String()))
		}
	}
	return Resolution{Item: best}, nil
}

// isLatestCandidate は`--latest`の候補条件を満たすかを返す（§3.2）。
//
// lifecycle=unknownは候補に含める。§3.1が「lifecycle=unknownは状態を明示するが
// **EOLと断定しない**」と定めるためで、除外すると上流がlifecycleを公開していない
// toolの`--latest`が常に失敗する。選んだ場合は呼出し側がPlanへ状態を表示する。
func isLatestCandidate(item store.CatalogItem) bool {
	return item.Installable &&
		item.Channel == domain.ChannelStable &&
		item.Lifecycle != domain.LifecycleEOL
}

// ListAvailable はcatalogの全itemを表示順で返す。
//
// docs/03-cli.md §3.2「完全versionをversion降順で、channelとlifecycleの列とともに
// **常に全件表示する**。channel/lifecycleで絞り込むoptionはv0.1に存在しない」。
// installable=falseのitemも理由付きで含める（docs/08-install-runtime.md §3.1
// 「platform artifactがないversionは理由付き`installable=false`として表示する」）。
//
// 並べ替えはしない。§15がcatalog保存時の順序を「version comparison降順、同値なら
// version byte順」と定め、[store.ParseCatalog]がその順序を検査済みだからである。
// ここで並べ直すと、保存順の検査を通っていない並びを表示することになる。
func ListAvailable(catalog store.Catalog) []store.CatalogItem {
	return append([]store.CatalogItem(nil), catalog.Items...)
}

// versionNotFound は該当versionが無いことをtyped errorにする。
func versionNotFound(catalog store.Catalog, cause error) *domain.Error {
	return &domain.Error{
		Code: domain.CodeVersionNotFound,
		// 同じcatalogを何度引いても同じ結果になる（docs/02-architecture.md §14）。
		Retryable: false,
		PathRole:  domain.RoleCatalog,
		Cause: fmt.Errorf(
			"catalog: %s / %s: %w", catalog.Tool.String(), catalog.Platform.ID(), cause),
	}
}

// platformUnsupported は現platformで導入できないことをtyped errorにする。
//
// **docs/08-install-runtime.md §3.1はこの場合のerror codeを明示していない。**
// 同§は「platform artifactがないversionは理由付き`installable=false`として
// **表示する**」とだけ定め、exact指定で当たったときの扱いを書いていない。
// versionは見つかっているため`E_VERSION_NOT_FOUND`ではなく、「このversionに
// ついてこのplatformが対象外」を表す`E_PLATFORM_UNSUPPORTED`を使う。
// docs/03-cli.md §7の終了codeはどちらも4であり、利用者が見る終了codeは変わらない。
func platformUnsupported(cause error) *domain.Error {
	return &domain.Error{
		Code:      domain.CodePlatformUnsupported,
		Retryable: false,
		PathRole:  domain.RoleCatalog,
		Cause:     cause,
	}
}
