package catalog

import (
	"testing"
	"time"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/definition"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
)

// staticEntry はdocs/06-tool-definition.md §16.3のPython static versionである。
func staticEntry(
	t *testing.T, version string, channel definition.Channel, lifecycle definition.Lifecycle,
) definition.StaticVersion {
	t.Helper()
	return definition.StaticVersion{
		Version:             mustParse(t, domain.SchemePython, version),
		Channel:             channel,
		Lifecycle:           lifecycle,
		LifecycleEvidence:   "https://devguide.python.org/versions/",
		LifecycleAssessedAt: time.Date(2026, 8, 7, 0, 0, 0, 0, time.UTC),
		PublishedAt:         time.Date(2025, 8, 14, 0, 0, 0, 0, time.UTC),
	}
}

func staticSource(t *testing.T, entries ...definition.StaticVersion) definition.VersionSource {
	t.Helper()
	return definition.VersionSource{
		Kind:           definition.SourceStatic,
		StaticVersions: entries,
		MaxItems:       10000,
	}
}

// TestBuildStaticItemsSortsByComparisonKey は§6.6の並びを固定する。
//
// 「static sourceはversion itemをfile記載順で解釈せず、正規version byteで一意
// 検査してcomparison keyでsortする」。記載順に依存すると、registryのdiffで行を
// 並べ替えただけでcatalogの内容が変わる。
func TestBuildStaticItemsSortsByComparisonKey(t *testing.T) {
	source := staticSource(t,
		staticEntry(t, "3.13.7", definition.ChannelStable, definition.LifecycleSupported),
		staticEntry(t, "3.9.0", definition.ChannelStable, definition.LifecycleEOL),
		// 多桁。文字列順なら3.10.0は3.9.0より前に来る。
		staticEntry(t, "3.10.0", definition.ChannelStable, definition.LifecycleSupported),
		staticEntry(t, "3.14.0rc1", definition.ChannelPrerelease, definition.LifecycleUnknown),
	)
	items, err := BuildStaticItems(source)
	if err != nil {
		t.Fatalf("BuildStaticItems = %s", describeErr(err))
	}
	want := []string{"3.9.0", "3.10.0", "3.13.7", "3.14.0rc1"}
	if len(items) != len(want) {
		t.Fatalf("items = %d件, want %d", len(items), len(want))
	}
	for index, version := range want {
		if items[index].Version.String() != version {
			t.Errorf("items[%d] = %q, want %q", index, items[index].Version.String(), version)
		}
	}
}

// TestBuildStaticItemsUsesEntryValues はitem自身が書いた値を使うことを固定する。
//
// §6.6はchannelとlifecycleをitemへ書き、§6.4はstatic sourceのoverrideを禁じる。
// §6.3の3段の優先順位を通らないため、根拠は専用の値になる。
func TestBuildStaticItemsUsesEntryValues(t *testing.T) {
	source := staticSource(t,
		staticEntry(t, "3.13.7", definition.ChannelStable, definition.LifecycleSupported),
		// channelとlifecycleは独立である。prereleaseでもeolを表現できる。
		staticEntry(t, "3.14.0rc1", definition.ChannelPrerelease, definition.LifecycleEOL),
		staticEntry(t, "3.15.0", definition.ChannelStable, definition.LifecycleUnknown),
	)
	items, err := BuildStaticItems(source)
	if err != nil {
		t.Fatalf("BuildStaticItems = %s", describeErr(err))
	}
	wants := []struct {
		version   string
		channel   domain.Channel
		lifecycle domain.Lifecycle
	}{
		{"3.13.7", domain.ChannelStable, domain.LifecycleSupported},
		{"3.14.0rc1", domain.ChannelPrerelease, domain.LifecycleEOL},
		{"3.15.0", domain.ChannelStable, domain.LifecycleUnknown},
	}
	for index, want := range wants {
		item := items[index]
		if item.Version.String() != want.version {
			t.Fatalf("items[%d] = %q", index, item.Version.String())
		}
		if item.Channel != want.channel {
			t.Errorf("%s channel = %q, want %q", want.version, item.Channel, want.channel)
		}
		if item.Lifecycle.Lifecycle != want.lifecycle {
			t.Errorf("%s lifecycle = %q, want %q",
				want.version, item.Lifecycle.Lifecycle, want.lifecycle)
		}
		if item.Lifecycle.From != LifecycleFromStatic {
			t.Errorf("%s from = %q, want %q", want.version, item.Lifecycle.From, LifecycleFromStatic)
		}
		// 公開日時はUTC RFC 3339へ揃える。
		if item.PublishedAt != "2025-08-14T00:00:00Z" {
			t.Errorf("%s published_at = %q", want.version, item.PublishedAt)
		}
		// static sourceはJSON nodeを持たない。元entryを参照させる。
		if item.Node != nil {
			t.Errorf("%s Nodeがnilでない", want.version)
		}
		if item.Static == nil {
			t.Fatalf("%s Staticがnil", want.version)
		}
		if item.Static.LifecycleEvidence != "https://devguide.python.org/versions/" {
			t.Errorf("%s evidence = %q", want.version, item.Static.LifecycleEvidence)
		}
		// raw versionは正規version文字列そのものである。static sourceはregexを
		// 持たず、§6.5のprovider_releaseはassetの`release_tag`を使う。
		if item.RawVersion != want.version {
			t.Errorf("%s raw = %q", want.version, item.RawVersion)
		}
	}
}

// TestBuildStaticItemsAppliesItemLimit は§6.1のitem数上限がstaticにも効くことを
// 固定する。static sourceが使えるkeyは`static_versions`と`max_items`だけである。
func TestBuildStaticItemsAppliesItemLimit(t *testing.T) {
	source := staticSource(t,
		staticEntry(t, "3.13.7", definition.ChannelStable, definition.LifecycleSupported),
		staticEntry(t, "3.13.8", definition.ChannelStable, definition.LifecycleSupported),
	)
	source.MaxItems = 1
	if _, err := BuildStaticItems(source); err == nil {
		t.Fatal("max_items超過が成功した")
	}
}

// TestBuildStaticItemsRejectsInvalidChannel は未知channelを拒否することを固定する。
//
// definitionのschema検証が通す値だけが渡る契約だが、enumが片側だけ増えたときに
// 黙って空channelのcatalogを作らないようにする。
func TestBuildStaticItemsRejectsInvalidChannel(t *testing.T) {
	source := staticSource(t, staticEntry(t, "3.13.7", "lts", definition.LifecycleSupported))
	_, err := BuildStaticItems(source)
	if err == nil {
		t.Fatal("未知channelが成功した")
	}
	if err.Code != domain.CodeDefinitionInvalid {
		t.Fatalf("code = %s", err.Code)
	}
}

// TestBuildStaticItemsRejectsMixedScheme は比較できないversionの混在を拒否する
// ことを固定する。sortが順序を決められないまま結果を返さない。
func TestBuildStaticItemsRejectsMixedScheme(t *testing.T) {
	python := staticEntry(t, "3.13.7", definition.ChannelStable, definition.LifecycleSupported)
	other := python
	other.Version = mustParse(t, domain.SchemeGo, "1.25.0")
	if _, err := BuildStaticItems(staticSource(t, python, other)); err == nil {
		t.Fatal("scheme混在が成功した")
	}
}

// TestBuildStaticItemsAcceptsEmptyPublishedAt は未設定の公開日時を空文字にする
// ことを固定する。取得時刻で代用しない（§6.1）。
func TestBuildStaticItemsAcceptsEmptyPublishedAt(t *testing.T) {
	entry := staticEntry(t, "3.13.7", definition.ChannelStable, definition.LifecycleSupported)
	entry.PublishedAt = time.Time{}
	items, err := BuildStaticItems(staticSource(t, entry))
	if err != nil {
		t.Fatalf("BuildStaticItems = %s", describeErr(err))
	}
	if items[0].PublishedAt != "" {
		t.Fatalf("published_at = %q, want \"\"", items[0].PublishedAt)
	}
}
