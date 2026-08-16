package catalog

import (
	"testing"
	"time"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/definition"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
)

// 本fileはdocs/13-progress.md P3-02の境界testである。
//
// docs/06-tool-definition.md §6.1のchannel導出・写像、§6.3のlifecycle優先順位、
// §6.4のexact version overrideを境界値で固定する。上流文書の取得と評価は
// P3-03の範囲であり、ここでは判定規則だけを扱う。

// --- channel導出 ---

// TestDeriveChannelUsesSyntaxOnly は`channel_pointer`省略時のchannelが正規version
// の構文だけで決まることを固定する（§6.1）。
//
// 公開日、version番号の大小、上流の表記を見ない。0系のversionをprerelease扱いに
// する、といった推測を入れると、上流がstableとして公開したversionをgdtvmが隠す。
func TestDeriveChannelUsesSyntaxOnly(t *testing.T) {
	cases := []struct {
		scheme domain.VersionScheme
		text   string
		want   domain.Channel
	}{
		{domain.SchemeSemver, "1.0.0", domain.ChannelStable},
		// 0系でもprerelease構文が無ければstableである。
		{domain.SchemeSemver, "0.0.0", domain.ChannelStable},
		{domain.SchemeSemver, "22.18.0", domain.ChannelStable},
		{domain.SchemeSemver, "1.0.0-alpha", domain.ChannelPrerelease},
		{domain.SchemeSemver, "1.0.0-0", domain.ChannelPrerelease},
		{domain.SchemeGo, "1.25.0", domain.ChannelStable},
		{domain.SchemeGo, "1.25", domain.ChannelStable},
		{domain.SchemeGo, "1.25beta1", domain.ChannelPrerelease},
		{domain.SchemeGo, "1.25rc2", domain.ChannelPrerelease},
		{domain.SchemePython, "3.13.7", domain.ChannelStable},
		{domain.SchemePython, "3.14.0a1", domain.ChannelPrerelease},
		{domain.SchemePython, "3.14.0b2", domain.ChannelPrerelease},
		{domain.SchemePython, "3.14.0rc3", domain.ChannelPrerelease},
	}
	for _, c := range cases {
		version := mustParse(t, c.scheme, c.text)
		got, err := DeriveChannel(version)
		if err != nil {
			t.Errorf("DeriveChannel(%s %q): %v", c.scheme, c.text, err)
			continue
		}
		if got != c.want {
			t.Errorf("DeriveChannel(%s %q) = %q, want %q", c.scheme, c.text, got, c.want)
		}
	}
}

// TestDeriveChannelRejectsZeroVersion は未初期化versionを拒否することを固定する。
// 判定できない入力をstableへ倒すと、parseに失敗したitemが正式版として並ぶ。
func TestDeriveChannelRejectsZeroVersion(t *testing.T) {
	if _, err := DeriveChannel(domain.Version{}); err == nil {
		t.Fatal("未初期化versionからchannelを導出できてしまった")
	}
}

// --- channel写像 ---

// TestMapChannelAcceptsStringAndBool は`channel_pointer`先の型と値を固定する（§6.1）。
func TestMapChannelAcceptsStringAndBool(t *testing.T) {
	cases := []struct {
		name  string
		value domain.Scalar
		want  domain.Channel
	}{
		{"string stable", domain.StringScalar("stable"), domain.ChannelStable},
		{"string prerelease", domain.StringScalar("prerelease"), domain.ChannelPrerelease},
		// booleanの真がstableである。Goの`https://go.dev/dl/`が`"stable": true`で
		// 正式版を示す。向きを逆に取ると全versionのchannelが反転する。
		{"bool true", domain.BoolScalar(true), domain.ChannelStable},
		{"bool false", domain.BoolScalar(false), domain.ChannelPrerelease},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := MapChannel(c.value)
			if err != nil {
				t.Fatalf("MapChannel: %v", err)
			}
			if got != c.want {
				t.Fatalf("MapChannel = %q, want %q", got, c.want)
			}
		})
	}
}

// TestMapChannelRejectsOtherValues は暗黙変換とfallbackが無いことを固定する（§6.1）。
//
// 未知stringをstableへ倒すと、上流がchannel名を変えたことに気付けないまま
// prereleaseが正式版として並ぶ。
func TestMapChannelRejectsOtherValues(t *testing.T) {
	cases := []struct {
		name  string
		value domain.Scalar
	}{
		{"未知string", domain.StringScalar("lts")},
		{"空string", domain.StringScalar("")},
		{"大文字", domain.StringScalar("Stable")},
		{"前後の空白", domain.StringScalar(" stable")},
		{"beta表記", domain.StringScalar("beta")},
		// 数値の0/1をbooleanとして解釈しない。
		{"integer 1", domain.IntScalar(1)},
		{"integer 0", domain.IntScalar(0)},
		{"null", domain.NullScalar()},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := MapChannel(c.value); err == nil {
				t.Fatalf("MapChannel(%v) が成功した", c.value)
			}
		})
	}
}

// --- lifecycle優先順位 ---

// TestResolveLifecyclePriority は§6.3の優先順位1/2/3を固定する。
func TestResolveLifecyclePriority(t *testing.T) {
	version := mustParse(t, domain.SchemePython, "3.9.0")
	overrides := []definition.LifecycleOverride{
		newOverride(t, domain.SchemePython, "3.9.0", definition.LifecycleEOL),
	}
	supported := domain.LifecycleSupported

	t.Run("1 override", func(t *testing.T) {
		// sourceが値を持たない場合、overrideが決める。`json` sourceの
		// lifecycleは優先順位1と3だけで決まる（§6.3）。
		got := mustResolve(t, overrides, version, nil)
		if got.Lifecycle != domain.LifecycleEOL || got.From != LifecycleFromOverride {
			t.Fatalf("= %+v, want {eol override}", got)
		}
	})

	t.Run("2 source", func(t *testing.T) {
		// 対象versionのoverrideが無ければsourceの写像結果を使う。
		other := mustParse(t, domain.SchemePython, "3.13.0")
		got := mustResolve(t, overrides, other, &supported)
		if got.Lifecycle != domain.LifecycleSupported || got.From != LifecycleFromSource {
			t.Fatalf("= %+v, want {supported source}", got)
		}
	})

	t.Run("3 default", func(t *testing.T) {
		// どちらも無ければunknown。公開日やversionの古さからEOLを推測しない（§6.3）。
		got := mustResolve(t, nil, version, nil)
		if got.Lifecycle != domain.LifecycleUnknown || got.From != LifecycleFromDefault {
			t.Fatalf("= %+v, want {unknown default}", got)
		}
	})
}

// TestResolveLifecycleMatchesOverrideByExactString はoverrideの照合が正規version
// 文字列の完全一致であることを固定する。
//
// goの`1.20`と`1.20.0`はcomparison keyが同じで正規文字列が違う（§4）。comparison
// keyで照合すると、`1.20`へ書いたeol overrideが`1.20.0`へも当たる。
func TestResolveLifecycleMatchesOverrideByExactString(t *testing.T) {
	overrides := []definition.LifecycleOverride{
		newOverride(t, domain.SchemeGo, "1.20", definition.LifecycleEOL),
	}

	matched := mustResolve(t, overrides, mustParse(t, domain.SchemeGo, "1.20"), nil)
	if matched.From != LifecycleFromOverride || matched.Lifecycle != domain.LifecycleEOL {
		t.Fatalf("同一文字列へ当たらなかった: %+v", matched)
	}

	unmatched := mustResolve(t, overrides, mustParse(t, domain.SchemeGo, "1.20.0"), nil)
	if unmatched.From != LifecycleFromDefault || unmatched.Lifecycle != domain.LifecycleUnknown {
		t.Fatalf("comparison keyが同じ別文字列へoverrideが当たった: %+v", unmatched)
	}
}

// TestResolveLifecycleRejectsConflictingOverride は§6.4の「source lifecycle field
// と同じversionで矛盾するoverrideを拒否する」を固定する。
//
// この検査はsourceの値が判明するここでしかできない。definition parse時には
// 上流の値がまだ無い。優先順位1で黙って勝たせると、上流がsupportedへ戻したのに
// 古いeol overrideが残っていることに誰も気付けない。
func TestResolveLifecycleRejectsConflictingOverride(t *testing.T) {
	version := mustParse(t, domain.SchemePython, "3.9.0")
	overrides := []definition.LifecycleOverride{
		newOverride(t, domain.SchemePython, "3.9.0", definition.LifecycleEOL),
	}

	supported := domain.LifecycleSupported
	if _, err := ResolveLifecycle(overrides, version, &supported); err == nil {
		t.Fatal("sourceと矛盾するoverrideが通った")
	}

	// 一致していれば矛盾ではない。overrideが決めたことにする（優先順位1）。
	eol := domain.LifecycleEOL
	got := mustResolve(t, overrides, version, &eol)
	if got.Lifecycle != domain.LifecycleEOL || got.From != LifecycleFromOverride {
		t.Fatalf("= %+v, want {eol override}", got)
	}
}

// TestResolveLifecycleAppliesToPrerelease はchannelとlifecycleが独立であることを
// 固定する。§6.4は「channelとlifecycleは独立なのでprereleaseへeol overrideも
// 設定できる」と定める。
func TestResolveLifecycleAppliesToPrerelease(t *testing.T) {
	version := mustParse(t, domain.SchemePython, "3.14.0rc1")
	overrides := []definition.LifecycleOverride{
		newOverride(t, domain.SchemePython, "3.14.0rc1", definition.LifecycleEOL),
	}
	got := mustResolve(t, overrides, version, nil)
	if got.Lifecycle != domain.LifecycleEOL {
		t.Fatalf("= %+v, want eol", got)
	}
	channel, err := DeriveChannel(version)
	if err != nil {
		t.Fatal(err)
	}
	if channel != domain.ChannelPrerelease {
		t.Fatalf("channel = %q, want prerelease（lifecycleはchannelへ影響しない）", channel)
	}
}

// TestResolveLifecycleRejectsZeroVersion は未初期化versionを拒否することを固定する。
func TestResolveLifecycleRejectsZeroVersion(t *testing.T) {
	if _, err := ResolveLifecycle(nil, domain.Version{}, nil); err == nil {
		t.Fatal("未初期化versionのlifecycleが決まってしまった")
	}
}

// --- lifecycle写像 ---

// TestMapLifecycleRequiresDeclaredValue は`lifecycle_map`に無い値がsource errorに
// なることを固定する（§6.1）。
//
// 黙って`unknown`へ倒すと、上流がenum値を増やしたことをlive smokeで検出できない。
func TestMapLifecycleRequiresDeclaredValue(t *testing.T) {
	table := map[string]definition.Lifecycle{
		"active":      definition.LifecycleSupported,
		"maintenance": definition.LifecycleSupported,
		"eol":         definition.LifecycleEOL,
		"preview":     definition.LifecycleUnknown,
	}
	want := map[string]domain.Lifecycle{
		"active":      domain.LifecycleSupported,
		"maintenance": domain.LifecycleSupported,
		"eol":         domain.LifecycleEOL,
		"preview":     domain.LifecycleUnknown,
	}
	for value, expected := range want {
		got, err := MapLifecycle(table, value)
		if err != nil {
			t.Errorf("MapLifecycle(%q): %v", value, err)
			continue
		}
		if got != expected {
			t.Errorf("MapLifecycle(%q) = %q, want %q", value, got, expected)
		}
	}

	for _, value := range []string{"go-live", "", "Active", "active "} {
		if _, err := MapLifecycle(table, value); err == nil {
			t.Errorf("MapLifecycle(%q) が成功した（mapに無い値）", value)
		}
	}

	// mapが無い状態でpointerの値を読むこと自体がsource errorである。写像先の
	// 無いpointerはlifecycleを決められない（§6.3）。
	if _, err := MapLifecycle(nil, "active"); err == nil {
		t.Error("`lifecycle_map`なしで写像が成功した")
	}
	if _, err := MapLifecycle(map[string]definition.Lifecycle{}, "active"); err == nil {
		t.Error("空の`lifecycle_map`で写像が成功した")
	}
}

// --- 未使用override ---

// TestUnusedOverridesReportsMissingVersions は§6.4の
// `W_LIFECYCLE_OVERRIDE_UNUSED`対象を固定する。
//
// sourceにないoverrideはcatalog itemを合成しない。黙って捨てると、上流から
// versionが消えたことにもoverrideのversion誤記にも気付けない。
func TestUnusedOverridesReportsMissingVersions(t *testing.T) {
	overrides := []definition.LifecycleOverride{
		newOverride(t, domain.SchemeSemver, "18.20.8", definition.LifecycleEOL),
		newOverride(t, domain.SchemeSemver, "20.19.5", definition.LifecycleEOL),
		newOverride(t, domain.SchemeSemver, "22.18.0", definition.LifecycleSupported),
	}
	versions := []domain.Version{
		mustParse(t, domain.SchemeSemver, "22.18.0"),
		mustParse(t, domain.SchemeSemver, "24.0.0"),
	}
	unused := UnusedOverrides(overrides, versions)
	if len(unused) != 2 {
		t.Fatalf("未使用override = %d件, want 2", len(unused))
	}
	// 戻り値はdefinitionの宣言順である。並べ替えると、どのentryを直せばよいかを
	// definitionと突き合わせにくくなる。
	if unused[0].Version.String() != "18.20.8" || unused[1].Version.String() != "20.19.5" {
		t.Fatalf("宣言順で返っていない: %q, %q",
			unused[0].Version.String(), unused[1].Version.String())
	}

	// 全件が使われていればnilを返す。
	if got := UnusedOverrides(overrides[2:], versions); len(got) != 0 {
		t.Fatalf("使用済みのoverrideが未使用として返った: %d件", len(got))
	}
	// overrideが無ければ報告も無い。
	if got := UnusedOverrides(nil, versions); len(got) != 0 {
		t.Fatalf("override無しで %d件が返った", len(got))
	}
}

// TestUnusedOverridesMatchesByExactString は照合が正規version文字列の完全一致で
// あることを固定する。goの`1.20`と`1.20.0`はcomparison keyが同じでも別itemである。
func TestUnusedOverridesMatchesByExactString(t *testing.T) {
	overrides := []definition.LifecycleOverride{
		newOverride(t, domain.SchemeGo, "1.20", definition.LifecycleEOL),
	}
	versions := []domain.Version{mustParse(t, domain.SchemeGo, "1.20.0")}
	if got := UnusedOverrides(overrides, versions); len(got) != 1 {
		t.Fatalf("comparison keyが同じ別文字列を使用済みとみなした: %d件", len(got))
	}
}

// --- helper ---

func mustParse(t *testing.T, scheme domain.VersionScheme, text string) domain.Version {
	t.Helper()
	version, err := domain.ParseVersion(scheme, text)
	if err != nil {
		t.Fatalf("ParseVersion(%s, %q): %v", scheme, text, err)
	}
	return version
}

func mustResolve(
	t *testing.T, overrides []definition.LifecycleOverride,
	version domain.Version, mapped *domain.Lifecycle,
) LifecycleDecision {
	t.Helper()
	got, err := ResolveLifecycle(overrides, version, mapped)
	if err != nil {
		t.Fatalf("ResolveLifecycle(%q): %v", version.String(), err)
	}
	return got
}

func newOverride(
	t *testing.T, scheme domain.VersionScheme, text string, status definition.Lifecycle,
) definition.LifecycleOverride {
	t.Helper()
	return definition.LifecycleOverride{
		Version:    mustParse(t, scheme, text),
		Status:     status,
		Evidence:   "https://example.invalid/official-lifecycle",
		AssessedAt: time.Date(2026, 8, 7, 0, 0, 0, 0, time.UTC),
	}
}
