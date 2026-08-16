package catalog

import (
	"fmt"
	"testing"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/definition"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/progress"
)

// childDocument は§16.4の.NET SDKの子文書である。
const childDocument = `{
	"support-phase":"%s",
	"releases":[
		{"release-date":"2026-01-14","sdks":[{"version":"9.0.101"},{"version":"9.0.102"}]}
	]
}`

func childDocumentWith(t *testing.T, phase string) any {
	t.Helper()
	return mustDecode(t, fmt.Sprintf(childDocument, phase))
}

// TestBuildItemsAppliesDocumentLifecycle は§6.2の`document_lifecycle_pointer`を
// 固定する。
//
// 「子文書の**top-level**から1つの値を読み、その子文書由来の全itemへ同じ
// lifecycleを与える。」
func TestBuildItemsAppliesDocumentLifecycle(t *testing.T) {
	cases := map[string]domain.Lifecycle{
		"active":      domain.LifecycleSupported,
		"maintenance": domain.LifecycleSupported,
		"preview":     domain.LifecycleSupported,
		"go-live":     domain.LifecycleSupported,
		"eol":         domain.LifecycleEOL,
	}
	for phase, want := range cases {
		t.Run(phase, func(t *testing.T) {
			items := mustBuildItems(t, ItemsRequest{
				Source:   indexSource(),
				Scheme:   domain.SchemeSemver,
				Document: childDocumentWith(t, phase),
				Origin:   dotnetChild90,
			})
			if len(items) != 2 {
				t.Fatalf("items = %d件, want 2", len(items))
			}
			// 同じ子文書由来の全itemへ同じlifecycleを与える。
			for index, item := range items {
				if item.Lifecycle.Lifecycle != want {
					t.Errorf("items[%d] = %q, want %q", index, item.Lifecycle.Lifecycle, want)
				}
				if item.Lifecycle.From != LifecycleFromSource {
					t.Errorf("items[%d].From = %q, want source", index, item.Lifecycle.From)
				}
			}
		})
	}
}

// TestBuildItemsRejectsUnmappedLifecycle は§6.1の「mapに無い値はsource error」を
// 固定する。黙って`unknown`へ倒すと、上流がenum値を増やしたことをlive smokeで
// 検出できない。
func TestBuildItemsRejectsUnmappedLifecycle(t *testing.T) {
	_, err := BuildItems(ItemsRequest{
		Source:   indexSource(),
		Scheme:   domain.SchemeSemver,
		Document: childDocumentWith(t, "extended-support"),
		Origin:   dotnetChild90,
	})
	if err == nil {
		t.Fatal("mapに無いlifecycle値が通った")
	}
	if err.Code != domain.CodeDefinitionInvalid {
		t.Fatalf("code = %s", err.Code)
	}
}

// TestBuildItemsRejectsMissingDocumentLifecycle は参照fieldの欠落を固定する。
func TestBuildItemsRejectsMissingDocumentLifecycle(t *testing.T) {
	document := mustDecode(t, `{"releases":[
		{"release-date":"2026-01-14","sdks":[{"version":"9.0.101"}]}
	]}`)
	if _, err := BuildItems(ItemsRequest{
		Source:   indexSource(),
		Scheme:   domain.SchemeSemver,
		Document: document,
		Origin:   dotnetChild90,
	}); err == nil {
		t.Fatal("`support-phase`の欠落が通った")
	}
}

// TestBuildItemsAppliesItemLifecyclePointer は`lifecycle_pointer`（item相対）を
// 固定する。子文書単位ではなくitem単位で値が変わる。
func TestBuildItemsAppliesItemLifecyclePointer(t *testing.T) {
	source := indexSource()
	source.DocumentLifecyclePointer = definition.OptionalPointer{}
	source.LifecyclePointer = definition.DeclaredPointer("/phase")
	document := mustDecode(t, `{"releases":[
		{"release-date":"2026-01-14","sdks":[
			{"version":"9.0.101","phase":"active"},
			{"version":"9.0.102","phase":"eol"}
		]}
	]}`)
	items := mustBuildItems(t, ItemsRequest{
		Source:   source,
		Scheme:   domain.SchemeSemver,
		Document: document,
		Origin:   dotnetChild90,
	})
	if items[0].Lifecycle.Lifecycle != domain.LifecycleSupported {
		t.Errorf("items[0] = %q", items[0].Lifecycle.Lifecycle)
	}
	if items[1].Lifecycle.Lifecycle != domain.LifecycleEOL {
		t.Errorf("items[1] = %q", items[1].Lifecycle.Lifecycle)
	}
}

// TestBuildItemsDefaultsLifecycleToUnknown は§6.3の優先順位3を固定する。
//
// 「`json` sourceのlifecycleは1と3だけで決まる。」pointerを宣言できないため、
// overrideが無ければ全itemが`unknown`になる。**公開日やversionの古さからEOLを
// 推測しない。**
func TestBuildItemsDefaultsLifecycleToUnknown(t *testing.T) {
	items := mustBuildItems(t, ItemsRequest{
		Source:   nodeStyleSource(),
		Scheme:   domain.SchemeSemver,
		Document: mustDecode(t, `[{"version":"v10.0.0","date":"2018-04-24"}]`),
		Origin:   "https://nodejs.org/dist/index.json",
	})
	if items[0].Lifecycle.Lifecycle != domain.LifecycleUnknown {
		t.Fatalf("lifecycle = %q, want unknown", items[0].Lifecycle.Lifecycle)
	}
	if items[0].Lifecycle.From != LifecycleFromDefault {
		t.Fatalf("from = %q, want default", items[0].Lifecycle.From)
	}
}

// TestBuildItemsAppliesLifecycleOverride は§6.3の優先順位1を固定する。
func TestBuildItemsAppliesLifecycleOverride(t *testing.T) {
	source := nodeStyleSource()
	source.LifecycleOverrides = []definition.LifecycleOverride{
		newOverride(t, domain.SchemeSemver, "18.20.8", definition.LifecycleEOL),
	}
	items := mustBuildItems(t, ItemsRequest{
		Source: source,
		Scheme: domain.SchemeSemver,
		Document: mustDecode(t, `[
			{"version":"v18.20.8","date":"2025-03-01"},
			{"version":"v22.18.0","date":"2025-07-01"}
		]`),
		Origin: "https://nodejs.org/dist/index.json",
	})
	if items[0].Lifecycle.Lifecycle != domain.LifecycleEOL ||
		items[0].Lifecycle.From != LifecycleFromOverride {
		t.Errorf("items[0] = %+v", items[0].Lifecycle)
	}
	// overrideの無いversionは既定の`unknown`のままである。
	if items[1].Lifecycle.Lifecycle != domain.LifecycleUnknown ||
		items[1].Lifecycle.From != LifecycleFromDefault {
		t.Errorf("items[1] = %+v", items[1].Lifecycle)
	}
}

// TestBuildItemsRejectsConflictingOverride は§6.4の矛盾拒否がsource評価経路でも
// 効くことを固定する。
func TestBuildItemsRejectsConflictingOverride(t *testing.T) {
	source := indexSource()
	source.LifecycleOverrides = []definition.LifecycleOverride{
		newOverride(t, domain.SchemeSemver, "9.0.101", definition.LifecycleEOL),
	}
	// 子文書は`active`＝supportedと言っている。
	_, err := BuildItems(ItemsRequest{
		Source:   source,
		Scheme:   domain.SchemeSemver,
		Document: childDocumentWith(t, "active"),
		Origin:   dotnetChild90,
	})
	if err == nil {
		t.Fatal("sourceと矛盾するoverrideが通った")
	}
}

// TestLifecycleOverrideWarningsReportsUnused は§6.4の
// `W_LIFECYCLE_OVERRIDE_UNUSED`を固定する。
//
// 「sourceにないoverrideはcatalog itemを合成せず報告する。」上流に無いversionを
// overrideだけで作ると、installできないversionが`available`へ並ぶ。
func TestLifecycleOverrideWarningsReportsUnused(t *testing.T) {
	source := nodeStyleSource()
	source.LifecycleOverrides = []definition.LifecycleOverride{
		newOverride(t, domain.SchemeSemver, "16.20.2", definition.LifecycleEOL),
		newOverride(t, domain.SchemeSemver, "18.20.8", definition.LifecycleEOL),
		newOverride(t, domain.SchemeSemver, "22.18.0", definition.LifecycleSupported),
	}
	items := mustBuildItems(t, ItemsRequest{
		Source:   source,
		Scheme:   domain.SchemeSemver,
		Document: mustDecode(t, `[{"version":"v22.18.0","date":"2025-07-01"}]`),
		Origin:   "https://nodejs.org/dist/index.json",
	})
	// overrideのversionをcatalog itemにしない。
	if len(items) != 1 {
		t.Fatalf("items = %d件, want 1（overrideからitemを合成しない）", len(items))
	}

	warnings := LifecycleOverrideWarnings(source.LifecycleOverrides, items)
	if len(warnings) != 2 {
		t.Fatalf("warnings = %d件, want 2", len(warnings))
	}
	// definitionの宣言順で返す。
	wantVersions := []string{"16.20.2", "18.20.8"}
	for index, warning := range warnings {
		if warning.Code != progress.WarnLifecycleOverrideUnused {
			t.Errorf("warnings[%d].Code = %q", index, warning.Code)
		}
		if err := warning.Validate(); err != nil {
			t.Errorf("warnings[%d].Validate = %v", index, err)
		}
		version, _ := warning.Parameters["version"].Str()
		if version != wantVersions[index] {
			t.Errorf("warnings[%d].version = %q, want %q", index, version, wantVersions[index])
		}
	}

	// 全件が使われていれば警告は出ない。
	used := []definition.LifecycleOverride{source.LifecycleOverrides[2]}
	if got := LifecycleOverrideWarnings(used, items); len(got) != 0 {
		t.Fatalf("使用済みのoverrideが報告された: %d件", len(got))
	}
	if got := LifecycleOverrideWarnings(nil, items); len(got) != 0 {
		t.Fatalf("override無しで%d件が返った", len(got))
	}
}
