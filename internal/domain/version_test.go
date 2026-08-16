package domain

import (
	"strings"
	"testing"
)

// 本fileはdocs/13-progress.md P3-02の境界testである。
//
// docs/06-tool-definition.md §4のversion grammar・比較規則・「入力versionは
// catalogの正規文字列完全一致」を境界値で固定する。P1-02の`domain_test.go`が
// 代表値を通す一方、ここは**規則が壊れる側**を狙う。

// --- 数値要素: 桁数 ---

// TestVersionComparesNumbersNumerically は多桁の数値要素が文字列順でなく数値順で
// 比較されることを固定する。
//
// §4はどのschemeでも数値要素を数値として比較する。文字列順だと`1.10`が`1.9`より
// 小さくなり、`--latest`が古いversionを選ぶ。Node.jsの22.9→22.18のように、
// 実際に桁が増える上流がある。
func TestVersionComparesNumbersNumerically(t *testing.T) {
	cases := []struct {
		name     string
		scheme   VersionScheme
		lower    string
		higher   string
		position string
	}{
		{"semver minor", SchemeSemver, "1.9.0", "1.10.0", "minor"},
		{"semver patch", SchemeSemver, "22.9.0", "22.18.0", "patch相当の多桁"},
		{"semver major", SchemeSemver, "9.0.0", "10.0.0", "major"},
		{"semver prerelease番号", SchemeSemver, "1.0.0-9", "1.0.0-10", "prerelease数値識別子"},
		{"go minor", SchemeGo, "1.9", "1.10", "minor"},
		{"go patch", SchemeGo, "1.20.9", "1.20.10", "patch"},
		{"go prerelease番号", SchemeGo, "1.20beta9", "1.20beta10", "prerelease番号"},
		{"python minor", SchemePython, "3.9.0", "3.10.0", "minor"},
		{"python patch", SchemePython, "3.13.9", "3.13.10", "patch"},
		{"python prerelease番号", SchemePython, "3.13.0a9", "3.13.0a10", "prerelease番号"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			lower := mustParse(t, c.scheme, c.lower)
			higher := mustParse(t, c.scheme, c.higher)
			got, err := lower.Compare(higher)
			if err != nil {
				t.Fatalf("Compare: %v", err)
			}
			if got >= 0 {
				t.Fatalf("%q < %q（%sの数値比較）のはずが Compare = %d",
					c.lower, c.higher, c.position, got)
			}
		})
	}
}

// TestParseVersionRejectsOutOfRangeNumber は表現範囲を超える数値要素を拒否する
// ことを固定する。
//
// version文字列は上流catalogのJSONに由来する外部入力である（§6.3）。桁数に上限が
// 無いため、桁あふれを想定しない実装は上流が異常値を1件返しただけで落ちる。
// **parse errorとして閉じる**。
func TestParseVersionRejectsOutOfRangeNumber(t *testing.T) {
	const overflow = "18446744073709551616" // 2^64。uint64の最大値+1
	cases := []struct {
		scheme VersionScheme
		text   string
	}{
		{SchemeSemver, overflow + ".0.0"},
		{SchemeSemver, "1." + overflow + ".0"},
		{SchemeSemver, "1.0." + overflow},
		// prerelease数値識別子も比較時に数値化する。ここで弾かないとCompareが
		// 扱えない値を持ったVersionができる。
		{SchemeSemver, "1.0.0-" + overflow},
		{SchemeSemver, "1.0.0-alpha." + overflow},
		{SchemeGo, overflow + ".0"},
		{SchemeGo, "1." + overflow},
		{SchemeGo, "1.20." + overflow},
		{SchemeGo, "1.20beta" + overflow},
		{SchemePython, overflow + ".0.0"},
		{SchemePython, "3.13." + overflow},
		{SchemePython, "3.13.0rc" + overflow},
	}
	for _, c := range cases {
		if _, err := ParseVersion(c.scheme, c.text); err == nil {
			t.Errorf("ParseVersion(%s, %q) が成功した（範囲外の数値要素）", c.scheme, c.text)
		}
	}

	// 上限そのものは受理する。境界を1つ内側で切っていないことを確かめる。
	const max = "18446744073709551615" // 2^64-1
	accepted := []struct {
		scheme VersionScheme
		text   string
	}{
		{SchemeSemver, max + ".0.0"},
		{SchemeSemver, "1.0.0-" + max},
		{SchemeGo, "1." + max},
		{SchemePython, "3.13." + max},
	}
	for _, c := range accepted {
		if _, err := ParseVersion(c.scheme, c.text); err != nil {
			t.Errorf("ParseVersion(%s, %q) = %v, want nil（uint64の上限は受理する）",
				c.scheme, c.text, err)
		}
	}
}

// TestVersionCompareHandlesLargeNumbers は上限付近の値でも比較が成立することを
// 固定する。桁あふれをparseで弾いた結果、Compareは失敗しうる変換を持たない。
func TestVersionCompareLargeNumbers(t *testing.T) {
	lower := mustParse(t, SchemeSemver, "18446744073709551614.0.0")
	higher := mustParse(t, SchemeSemver, "18446744073709551615.0.0")
	got, err := lower.Compare(higher)
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}
	if got != -1 {
		t.Fatalf("Compare = %d, want -1", got)
	}
}

// --- 数値要素: leading zero ---

// TestParseVersionRejectsLeadingZeroInEveryPosition は§4の「数値要素は不要な
// leading zero禁止」を全位置で固定する。
//
// leading zeroを受けると`1.02.0`と`1.2.0`が同じcomparison keyの別文字列になり、
// 完全一致で引く利用者入力とcatalogの間で取り違えが起きる。
func TestParseVersionRejectsLeadingZeroInEveryPosition(t *testing.T) {
	cases := []struct {
		scheme VersionScheme
		text   string
	}{
		{SchemeSemver, "01.2.3"},
		{SchemeSemver, "1.02.3"},
		{SchemeSemver, "1.2.03"},
		{SchemeSemver, "1.2.3-01"},
		{SchemeSemver, "1.2.3-alpha.01"},
		{SchemeGo, "01.20"},
		{SchemeGo, "1.020"},
		{SchemeGo, "1.20.01"},
		{SchemeGo, "1.20beta01"},
		{SchemePython, "03.13.0"},
		{SchemePython, "3.013.0"},
		{SchemePython, "3.13.00"},
		{SchemePython, "3.13.0a01"},
	}
	for _, c := range cases {
		if _, err := ParseVersion(c.scheme, c.text); err == nil {
			t.Errorf("ParseVersion(%s, %q) が成功した（leading zero）", c.scheme, c.text)
		}
	}

	// 0そのものは正当な数値要素である。leading zero検査で0まで落とさない。
	zeros := []struct {
		scheme VersionScheme
		text   string
	}{
		{SchemeSemver, "0.0.0"},
		{SchemeSemver, "1.0.0-0"},
		{SchemeGo, "0.0"},
		{SchemeGo, "1.20.0"},
		{SchemePython, "0.0.0"},
	}
	for _, c := range zeros {
		if _, err := ParseVersion(c.scheme, c.text); err != nil {
			t.Errorf("ParseVersion(%s, %q) = %v, want nil（0は正当）", c.scheme, c.text, err)
		}
	}
}

// --- 前後の文字 ---

// TestParseVersionRejectsSurroundingCharacters は前後の空白・改行を拒否することを
// 固定する。
//
// version文字列はJSONから読んだ生の値である（§6.3）。trimして受けると、
// catalogの正規文字列とregistryの`lifecycle_overrides`が別のbyte列で同じversionを
// 指す状態になり、完全一致の前提が崩れる。改行はGoの`$`が行末に一致する正規表現
// 実装だと通ってしまうため、明示的に固定する。
func TestParseVersionRejectsSurroundingCharacters(t *testing.T) {
	bases := []struct {
		scheme VersionScheme
		text   string
	}{
		{SchemeSemver, "1.2.3"},
		{SchemeGo, "1.20"},
		{SchemePython, "3.13.0"},
	}
	wrappers := []struct {
		name   string
		prefix string
		suffix string
	}{
		{"先頭space", " ", ""},
		{"末尾space", "", " "},
		{"末尾LF", "", "\n"},
		{"末尾CRLF", "", "\r\n"},
		{"先頭LF", "\n", ""},
		{"末尾tab", "", "\t"},
		{"末尾NUL", "", "\x00"},
	}
	for _, base := range bases {
		for _, w := range wrappers {
			text := w.prefix + base.text + w.suffix
			if _, err := ParseVersion(base.scheme, text); err == nil {
				t.Errorf("ParseVersion(%s, %q) が成功した（%s）", base.scheme, text, w.name)
			}
		}
	}
}

// --- scheme別のstage順 ---

// TestVersionSchemeStageOrderDiffers はgoとpythonでstageとpatchの比較順が違うことを
// 固定する。
//
// §4はgoを「major/minor、beta<rc<final、prerelease番号、finalのpatch」、pythonを
// 「数値3要素、a<b<rc<final、prerelease番号」と定める。goはstageをpatchより先に
// 見るがpythonは後に見る。**「prereleaseは常に小さい」と一括で実装すると
// pythonの`3.13.1a1`と`3.13.0`を逆順にする。**
func TestVersionSchemeStageOrderDiffers(t *testing.T) {
	t.Run("goはstageがpatchより先", func(t *testing.T) {
		// 同じminorではprereleaseがどのfinal patchよりも小さい。
		rc := mustParse(t, SchemeGo, "1.20rc1")
		patched := mustParse(t, SchemeGo, "1.20.9")
		if got := mustCompare(t, rc, patched); got >= 0 {
			t.Fatalf("1.20rc1 < 1.20.9 のはずが Compare = %d", got)
		}
		// minorはstageより先に決まる。次のminorのbetaは前のminorのfinalより大きい。
		next := mustParse(t, SchemeGo, "1.21beta1")
		prev := mustParse(t, SchemeGo, "1.20.9")
		if got := mustCompare(t, next, prev); got <= 0 {
			t.Fatalf("1.21beta1 > 1.20.9 のはずが Compare = %d", got)
		}
	})

	t.Run("pythonはpatchがstageより先", func(t *testing.T) {
		// 上のpatchのprereleaseは下のpatchのfinalより大きい。goに対応する形が無く、
		// scheme共通の「prereleaseは小さい」規則を作れない箇所である。
		pre := mustParse(t, SchemePython, "3.13.1a1")
		final := mustParse(t, SchemePython, "3.13.0")
		if got := mustCompare(t, pre, final); got <= 0 {
			t.Fatalf("3.13.1a1 > 3.13.0 のはずが Compare = %d", got)
		}
		// 同じpatch内ではa<b<rc<finalである。
		order := []string{"3.13.1a1", "3.13.1b1", "3.13.1rc1", "3.13.1"}
		for i := 0; i+1 < len(order); i++ {
			low := mustParse(t, SchemePython, order[i])
			high := mustParse(t, SchemePython, order[i+1])
			if got := mustCompare(t, low, high); got >= 0 {
				t.Fatalf("%q < %q のはずが Compare = %d", order[i], order[i+1], got)
			}
		}
	})
}

// TestVersionGoPrereleaseIgnoresPatch はgoのprereleaseがpatchを持たないことを
// 固定する。§4のgo grammarは`MAJOR.MINORbetaN|rcN`だけを認め、`1.20.1rc1`のような
// 形を持たない。
func TestVersionGoPrereleaseIgnoresPatch(t *testing.T) {
	for _, text := range []string{"1.20.1beta1", "1.20.0rc1", "1.20beta1.1"} {
		if _, err := ParseVersion(SchemeGo, text); err == nil {
			t.Errorf("ParseVersion(go, %q) が成功した（prereleaseはpatchを持たない）", text)
		}
	}
}

// --- semver prerelease識別子 ---

// TestSemverPrereleaseIdentifierGrammar はSemVer 2.0.0の識別子grammarを固定する。
func TestSemverPrereleaseIdentifierGrammar(t *testing.T) {
	valid := []string{
		"1.0.0-alpha",
		"1.0.0-alpha.1",
		"1.0.0-alpha-1",   // 識別子内のhyphenは正当
		"1.0.0--",         // hyphenだけの識別子も文法上は正当
		"1.0.0-0.3.7",     // 数値識別子と非数値識別子の混在
		"1.0.0-x.7.z.92",  //
		"1.0.0-ALPHA",     // 大文字も正当
		"1.0.0-rc.1.beta", //
	}
	for _, text := range valid {
		if _, err := ParseVersion(SchemeSemver, text); err != nil {
			t.Errorf("ParseVersion(semver, %q) = %v, want nil", text, err)
		}
	}
	invalid := []struct {
		text string
		why  string
	}{
		{"1.0.0-alpha..1", "空識別子"},
		{"1.0.0-alpha.", "末尾の区切り"},
		{"1.0.0-.alpha", "先頭の区切り"},
		{"1.0.0-alpha_1", "underscore"},
		{"1.0.0-alpha+build", "build metadata"},
		{"1.0.0-アルファ", "非ASCII"},
		{"1.0.0-alpha 1", "space"},
	}
	for _, c := range invalid {
		if _, err := ParseVersion(SchemeSemver, c.text); err == nil {
			t.Errorf("ParseVersion(semver, %q) が成功した（%s）", c.text, c.why)
		}
	}
}

// TestSemverPrereleasePrecedence はSemVer 2.0.0のprecedence規則を境界で固定する。
func TestSemverPrereleasePrecedence(t *testing.T) {
	cases := []struct {
		lower  string
		higher string
		why    string
	}{
		{"1.0.0-alpha", "1.0.0", "prereleaseはfinalより小さい"},
		{"1.0.0-1", "1.0.0-alpha", "数値識別子は非数値識別子より小さい"},
		{"1.0.0-2", "1.0.0-10", "数値識別子は数値順（ASCII順ではない）"},
		{"1.0.0-alpha", "1.0.0-alpha.1", "前方一致なら識別子が多い側が大きい"},
		{"1.0.0-Alpha", "1.0.0-alpha", "非数値識別子はASCII順で大文字が先"},
		{"1.0.0-alpha.1", "1.0.0-alpha.beta", "同位置で数値<非数値"},
	}
	for _, c := range cases {
		lower := mustParse(t, SchemeSemver, c.lower)
		higher := mustParse(t, SchemeSemver, c.higher)
		if got := mustCompare(t, lower, higher); got >= 0 {
			t.Errorf("%q < %q（%s）のはずが Compare = %d", c.lower, c.higher, c.why, got)
		}
	}
}

// --- 大文字小文字 ---

// TestParseVersionStageMarkerIsCaseSensitive はgo/pythonのstage表記が小文字だけで
// あることを固定する。§4は`beta`/`rc`/`a`/`b`をそのまま書いており、大文字を
// 受けると同じversionが2通りの正規文字列を持つ。
func TestParseVersionStageMarkerIsCaseSensitive(t *testing.T) {
	cases := []struct {
		scheme VersionScheme
		text   string
	}{
		{SchemeGo, "1.20Beta1"},
		{SchemeGo, "1.20BETA1"},
		{SchemeGo, "1.20RC1"},
		{SchemeGo, "1.20Rc1"},
		{SchemePython, "3.13.0A1"},
		{SchemePython, "3.13.0B1"},
		{SchemePython, "3.13.0RC1"},
	}
	for _, c := range cases {
		if _, err := ParseVersion(c.scheme, c.text); err == nil {
			t.Errorf("ParseVersion(%s, %q) が成功した（stage表記は小文字だけ）", c.scheme, c.text)
		}
	}
}

// --- 完全一致 ---

// TestVersionStringIsInputBytes はString()が入力byte列をそのまま返すことを固定する。
//
// §4の「入力versionはcatalogの正規文字列完全一致であり、comparison keyへ変換した
// 近似一致をしない」の前提である。parseがどこかで正規化すると、利用者が入力した
// 文字列とcatalogのkeyが一致しなくなる。
func TestVersionStringIsInputBytes(t *testing.T) {
	cases := []struct {
		scheme VersionScheme
		text   string
	}{
		{SchemeSemver, "1.0.0-ALPHA.1"},
		{SchemeSemver, "22.18.0"},
		{SchemeGo, "1.20"},
		{SchemeGo, "1.20.0"},
		{SchemeGo, "1.25rc2"},
		{SchemePython, "3.13.7"},
		{SchemePython, "3.14.0rc3"},
	}
	for _, c := range cases {
		version := mustParse(t, c.scheme, c.text)
		if version.String() != c.text {
			t.Errorf("String() = %q, want %q", version.String(), c.text)
		}
		if len(version.String()) != len(c.text) {
			t.Errorf("byte長が変わった: %d != %d", len(version.String()), len(c.text))
		}
	}
}

// TestVersionExactMatchDistinguishesEqualComparisonKeys は同じcomparison keyでも
// 正規文字列が違えば別のversionとして引けることを固定する。
//
// goの`1.20`と`1.20.0`が該当する。§4は両者を同一catalogへ併存させないが、
// **完全一致の照合はcomparison keyでなく文字列で行う**。keyで引くと、片方しか
// 登録していないcatalogでもう片方の入力が当たってしまう。
func TestVersionExactMatchDistinguishesEqualComparisonKeys(t *testing.T) {
	short := mustParse(t, SchemeGo, "1.20")
	long := mustParse(t, SchemeGo, "1.20.0")
	if got := mustCompare(t, short, long); got != 0 {
		t.Fatalf("Compare(1.20, 1.20.0) = %d, want 0", got)
	}
	if short.String() == long.String() {
		t.Fatal("正規文字列まで同一になっている")
	}

	// catalogに`1.20`だけがある状態で`1.20.0`を引いても当たらない。
	catalog := []Version{short}
	if found := lookupExact(catalog, "1.20.0"); found {
		t.Error("comparison keyが同じというだけで別の正規文字列が当たった")
	}
	if found := lookupExact(catalog, "1.20"); !found {
		t.Error("同一の正規文字列が当たらなかった")
	}
}

// lookupExact は正規文字列のbyte完全一致でcatalogを引く。
func lookupExact(catalog []Version, text string) bool {
	for _, version := range catalog {
		if version.String() == text {
			return true
		}
	}
	return false
}

// --- 全順序 ---

// TestVersionCompareIsTotalOrder は全ペアで反射律・反対称律・推移律が成り立つことを
// 固定する。
//
// 既存の昇順testは隣接ペアだけを見る。隣接だけでは、比較段の抜けで生じる非推移的な
// 順序（`a<b`, `b<c` なのに `a>c`）を検出できない。sortの結果が入力順に依存すると
// `--latest`の選択が不安定になる。
func TestVersionCompareIsTotalOrder(t *testing.T) {
	cases := []struct {
		name   string
		scheme VersionScheme
		sorted []string
	}{
		{
			"go", SchemeGo,
			[]string{
				"0.0", "1.9", "1.10beta1", "1.10beta2", "1.10rc1", "1.10", "1.10.1",
				"1.10.10", "1.20", "2.0", "10.0",
			},
		},
		{
			"python", SchemePython,
			[]string{
				"0.0.0", "3.9.0", "3.10.0a1", "3.10.0b1", "3.10.0rc1", "3.10.0",
				"3.10.1a1", "3.10.1", "3.13.10", "3.14.0", "10.0.0",
			},
		},
		{
			"semver", SchemeSemver,
			[]string{
				"0.0.0", "1.0.0-0", "1.0.0-1", "1.0.0-10", "1.0.0-Alpha", "1.0.0-alpha",
				"1.0.0-alpha.1", "1.0.0-alpha.beta", "1.0.0-beta", "1.0.0-beta.2",
				"1.0.0-beta.11", "1.0.0-rc.1", "1.0.0", "1.0.1", "1.9.0", "1.10.0",
				"2.0.0", "10.0.0",
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			versions := make([]Version, len(c.sorted))
			for i, text := range c.sorted {
				versions[i] = mustParse(t, c.scheme, text)
			}
			for i := range versions {
				// 反射律。
				if got := mustCompare(t, versions[i], versions[i]); got != 0 {
					t.Errorf("Compare(%q, %q) = %d, want 0", c.sorted[i], c.sorted[i], got)
				}
				for j := range versions {
					got := mustCompare(t, versions[i], versions[j])
					want := compareInts(i, j)
					if got != want {
						t.Errorf("Compare(%q, %q) = %d, want %d（宣言順が昇順）",
							c.sorted[i], c.sorted[j], got, want)
					}
					// 反対称律。
					back := mustCompare(t, versions[j], versions[i])
					if back != -got {
						t.Errorf("Compare(%q, %q) = %d だが逆順は %d",
							c.sorted[i], c.sorted[j], got, back)
					}
				}
			}
		})
	}
}

// --- prerelease判定 ---

// TestVersionIsPrerelease は§6.1の「正規versionが各schemeのprerelease構文を持てば
// prerelease」を全schemeで固定する。
func TestVersionIsPrerelease(t *testing.T) {
	cases := []struct {
		scheme VersionScheme
		text   string
		want   bool
	}{
		{SchemeSemver, "1.0.0", false},
		{SchemeSemver, "0.0.0", false},
		{SchemeSemver, "1.0.0-alpha", true},
		{SchemeSemver, "1.0.0-0", true},
		{SchemeGo, "1.20", false},
		{SchemeGo, "1.20.1", false},
		{SchemeGo, "1.20beta1", true},
		{SchemeGo, "1.20rc1", true},
		{SchemePython, "3.13.0", false},
		{SchemePython, "3.13.0a1", true},
		{SchemePython, "3.13.0b1", true},
		{SchemePython, "3.13.0rc1", true},
	}
	for _, c := range cases {
		version := mustParse(t, c.scheme, c.text)
		if got := version.IsPrerelease(); got != c.want {
			t.Errorf("IsPrerelease(%s, %q) = %v, want %v", c.scheme, c.text, got, c.want)
		}
	}
	// 未初期化値はprereleaseでない。channel導出側が別途errorにする。
	if (Version{}).IsPrerelease() {
		t.Error("未初期化のVersionがprerelease扱いになった")
	}
}

// --- 長い入力 ---

// TestParseVersionRejectsLongGarbage は長大な入力でparseが破綻しないことを固定する。
// grammarに合わない入力はいくら長くてもerrorであり、途中まで受理しない。
func TestParseVersionRejectsLongGarbage(t *testing.T) {
	long := "1.2.3-" + strings.Repeat("a", 4096)
	if _, err := ParseVersion(SchemeSemver, long); err != nil {
		t.Errorf("長い識別子は文法上正当なので受理する: %v", err)
	}
	broken := strings.Repeat("1.", 4096) + "0"
	for _, scheme := range []VersionScheme{SchemeSemver, SchemeGo, SchemePython} {
		if _, err := ParseVersion(scheme, broken); err == nil {
			t.Errorf("ParseVersion(%s, <長い非文法入力>) が成功した", scheme)
		}
	}
}

// --- helper ---

func mustParse(t *testing.T, scheme VersionScheme, text string) Version {
	t.Helper()
	version, err := ParseVersion(scheme, text)
	if err != nil {
		t.Fatalf("ParseVersion(%s, %q): %v", scheme, text, err)
	}
	return version
}

func mustCompare(t *testing.T, a, b Version) int {
	t.Helper()
	got, err := a.Compare(b)
	if err != nil {
		t.Fatalf("Compare(%q, %q): %v", a.String(), b.String(), err)
	}
	return got
}
