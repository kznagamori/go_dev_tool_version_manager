package catalog

import (
	"testing"
	"time"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/store"
)

// itemWith はchannel/lifecycle/installableを指定したitemを作る。
func itemWith(
	t *testing.T, version string, channel domain.Channel,
	lifecycle domain.Lifecycle, installable bool,
) store.CatalogItem {
	t.Helper()
	item := sampleItem(t, version, installable)
	item.Channel = channel
	item.Lifecycle = lifecycle
	return item
}

// catalogWith はitemを持つcatalogを作る。並びは呼出し側が§15の降順で渡す。
func catalogWith(t *testing.T, items ...store.CatalogItem) store.Catalog {
	t.Helper()
	return sampleCatalog(t, baseTime().Add(time.Hour), items...)
}

// TestResolveExactMatchesVersionBytes はbyte完全一致を固定する。
//
// docs/08-install-runtime.md §3.1手順3「入力をtrim、補完、range展開せず、catalogの
// 正規version文字列とbyte完全一致で探す」。`22.18`を`22.18.0`へ補完しない。
func TestResolveExactMatchesVersionBytes(t *testing.T) {
	catalog := catalogWith(t,
		sampleItem(t, "22.18.0", true),
		sampleItem(t, "20.15.1", true),
	)

	resolved, err := ResolveExact(catalog, "22.18.0")
	if err != nil {
		t.Fatalf("ResolveExact = %v", err.Cause)
	}
	if resolved.Item.VersionText != "22.18.0" {
		t.Errorf("version = %q, want 22.18.0", resolved.Item.VersionText)
	}

	// 部分版・空白・接頭辞は補完も除去もしない。
	for _, input := range []string{"22.18", "v22.18.0", " 22.18.0", "22.18.0 ", "22.18.0.0", ""} {
		t.Run("拒否: "+input, func(t *testing.T) {
			_, err := ResolveExact(catalog, input)
			if err == nil {
				t.Fatalf("%q が解決できた", input)
			}
			if err.Code != domain.CodeVersionNotFound {
				t.Errorf("code = %s, want %s", err.Code, domain.CodeVersionNotFound)
			}
		})
	}
}

// TestResolveExactRejectsUninstallableVersion は現platformで導入できないversionを
// 拒否することを固定する。
//
// 対応するerror codeは§3.1が明示していないため、versionは見つかっている事実に
// 合わせて`E_PLATFORM_UNSUPPORTED`とした（resolve.goの判断記録を参照）。
func TestResolveExactRejectsUninstallableVersion(t *testing.T) {
	catalog := catalogWith(t, sampleItem(t, "22.18.0", false))

	_, err := ResolveExact(catalog, "22.18.0")
	if err == nil {
		t.Fatal("導入できないversionが解決できた")
	}
	if err.Code != domain.CodePlatformUnsupported {
		t.Errorf("code = %s, want %s", err.Code, domain.CodePlatformUnsupported)
	}
	// 理由のmessage IDをerrorへ載せる。利用者が何が無いのか分かるようにする。
	if got := err.Cause.Error(); got == "" {
		t.Error("理由が空")
	}
}

// TestResolveLatestPicksMaxStableNonEOL は`--latest`の候補条件を固定する。
//
// docs/08-install-runtime.md §3.2「channel=stable、lifecycle!=eol、かつ現platformで
// installableなversionだけを…最大の完全version 1件へ解決する」。
func TestResolveLatestPicksMaxStableNonEOL(t *testing.T) {
	catalog := catalogWith(t,
		itemWith(t, "23.0.0", domain.ChannelPrerelease, domain.LifecycleSupported, true),
		itemWith(t, "22.20.0", domain.ChannelStable, domain.LifecycleSupported, false),
		itemWith(t, "22.19.0", domain.ChannelStable, domain.LifecycleEOL, true),
		itemWith(t, "22.18.0", domain.ChannelStable, domain.LifecycleSupported, true),
		itemWith(t, "20.15.1", domain.ChannelStable, domain.LifecycleSupported, true),
	)

	resolved, err := ResolveLatest(catalog)
	if err != nil {
		t.Fatalf("ResolveLatest = %v", err.Cause)
	}
	// prerelease（23.0.0）、installable=false（22.20.0）、EOL（22.19.0）を飛ばす。
	if resolved.Item.VersionText != "22.18.0" {
		t.Errorf("version = %q, want 22.18.0", resolved.Item.VersionText)
	}
}

// TestResolveLatestAcceptsUnknownLifecycle はlifecycle=unknownを候補に含めることを
// 固定する。
//
// docs/08-install-runtime.md §3.1「lifecycle=unknownは状態を明示するがEOLと
// 断定しない」。除外すると、上流がlifecycleを公開していないtoolの`--latest`が
// 常に失敗する。
func TestResolveLatestAcceptsUnknownLifecycle(t *testing.T) {
	catalog := catalogWith(t,
		itemWith(t, "22.18.0", domain.ChannelStable, domain.LifecycleUnknown, true),
	)

	resolved, err := ResolveLatest(catalog)
	if err != nil {
		t.Fatalf("ResolveLatest = %v", err.Cause)
	}
	if resolved.Item.Lifecycle != domain.LifecycleUnknown {
		t.Errorf("lifecycle = %s", resolved.Item.Lifecycle)
	}
}

// TestResolveLatestFailsWithoutCandidate は候補0件で失敗することを固定する（§3.2）。
func TestResolveLatestFailsWithoutCandidate(t *testing.T) {
	tests := []struct {
		name  string
		items []store.CatalogItem
	}{
		{"itemが無い", nil},
		{"prereleaseだけ", []store.CatalogItem{
			itemWith(t, "23.0.0", domain.ChannelPrerelease, domain.LifecycleSupported, true)}},
		{"EOLだけ", []store.CatalogItem{
			itemWith(t, "22.18.0", domain.ChannelStable, domain.LifecycleEOL, true)}},
		{"installable=falseだけ", []store.CatalogItem{
			itemWith(t, "22.18.0", domain.ChannelStable, domain.LifecycleSupported, false)}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := ResolveLatest(catalogWith(t, test.items...))
			if err == nil {
				t.Fatal("候補0件で解決できた")
			}
			if err.Code != domain.CodeVersionNotFound {
				t.Errorf("code = %s, want %s", err.Code, domain.CodeVersionNotFound)
			}
		})
	}
}

// TestResolveLatestRejectsTiedMaximum は同順位複数を拒否することを固定する（§3.2）。
//
// §15の並び規則は「version comparison降順、同値ならversion byte順」であり、
// comparison上同値のitemが2件並びうる。どちらを選ぶかを暗黙に決めない。
func TestResolveLatestRejectsTiedMaximum(t *testing.T) {
	// goのversion schemeは省略patchを許す。`1.22`と`1.22.0`はcomparison上同値。
	first := sampleItem(t, "22.18.0", true)
	second := sampleItem(t, "22.18.0", true)
	second.VersionText = "22.18.0+build"
	// 同じcomparison keyのまま別表記にする。
	second.Version = first.Version

	_, err := ResolveLatest(catalogWith(t, first, second))
	if err == nil {
		t.Fatal("同順位複数が解決できた")
	}
	if err.Code != domain.CodeVersionNotFound {
		t.Errorf("code = %s, want %s", err.Code, domain.CodeVersionNotFound)
	}
}

// TestListAvailableReturnsEveryItem は全件返すことを固定する。
//
// docs/03-cli.md §3.2「常に全件表示する。channel/lifecycleで絞り込むoptionは
// v0.1に存在しない」。installable=falseも理由付きで含める。
func TestListAvailableReturnsEveryItem(t *testing.T) {
	catalog := catalogWith(t,
		itemWith(t, "23.0.0", domain.ChannelPrerelease, domain.LifecycleSupported, true),
		itemWith(t, "22.19.0", domain.ChannelStable, domain.LifecycleEOL, true),
		itemWith(t, "22.18.0", domain.ChannelStable, domain.LifecycleSupported, false),
	)

	got := ListAvailable(catalog)
	if len(got) != 3 {
		t.Fatalf("件数 = %d, want 3", len(got))
	}
	// 保存順のまま返す。ここで並べ直すと、§15の順序検査を通っていない並びになる。
	want := []string{"23.0.0", "22.19.0", "22.18.0"}
	for index := range want {
		if got[index].VersionText != want[index] {
			t.Errorf("[%d] = %q, want %q", index, got[index].VersionText, want[index])
		}
	}
	if got[2].UnavailableReason == "" {
		t.Error("installable=falseのitemに理由が無い")
	}
}

// TestListAvailableIsImmutable は返したsliceがcatalogと切り離されていることを固定する。
//
// docs/02-architecture.md §4「request/resultは境界通過後にimmutableとして扱う」。
func TestListAvailableIsImmutable(t *testing.T) {
	catalog := catalogWith(t, sampleItem(t, "22.18.0", true))

	got := ListAvailable(catalog)
	got[0].VersionText = "tampered"

	if catalog.Items[0].VersionText != "22.18.0" {
		t.Error("返したsliceの書換えがcatalogへ伝わった")
	}
}
