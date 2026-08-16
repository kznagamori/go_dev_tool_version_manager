package domain

import "testing"

// TestParseClientVersionAcceptsGrammar はdocs/11-quality-and-ci.md §2のgrammarを
// 満たす形が受理されることを確かめる。
func TestParseClientVersionAcceptsGrammar(t *testing.T) {
	cases := []string{
		// docs/07-registry-and-tools.md §3の`client_min_version`例。
		"2026.08.07.00",
		// `XX`の下限と上限。§2「tagがなければ`00`、最大は`99`」。
		"2026.08.07.99",
		// 月・日の境界。
		"2026.01.01.00",
		"2026.12.31.00",
		// 閏日。実在するので受理する。
		"2024.02.29.00",
		// 400年規則の閏年。
		"2000.02.29.00",
	}
	for _, text := range cases {
		version, err := ParseClientVersion(text)
		if err != nil {
			t.Errorf("ParseClientVersion(%q): %v", text, err)
			continue
		}
		if version.String() != text {
			t.Errorf("String() = %q, want %q", version.String(), text)
		}
		if version.IsZero() {
			t.Errorf("ParseClientVersion(%q) の結果がzero値になった", text)
		}
	}
}

// TestParseClientVersionRejects はgrammar違反と非実在日付を拒否することを
// 確かめる。
//
// §2は「SemVerではなく、SemVerへ変換せずprerelease/build suffixを付けない」と
// 定めるため、SemVer由来の形も明示的に拒否側へ置く。
func TestParseClientVersionRejects(t *testing.T) {
	cases := []struct {
		name string
		text string
	}{
		{"空", ""},
		{"development build", "devel"},
		{"3成分", "2026.08.07"},
		{"5成分", "2026.08.07.00.01"},
		{"年が3桁", "202.08.07.00"},
		{"年が5桁", "20260.08.07.00"},
		{"月が1桁", "2026.8.07.00"},
		{"月が0", "2026.00.07.00"},
		{"月が13", "2026.13.07.00"},
		{"日が1桁", "2026.08.7.00"},
		{"日が0", "2026.08.00.00"},
		{"日が32", "2026.08.32.00"},
		{"通番が1桁", "2026.08.07.0"},
		{"通番が3桁", "2026.08.07.000"},
		{"prerelease suffix", "2026.08.07.00-rc.1"},
		{"build suffix", "2026.08.07.00+abc"},
		{"v接頭", "v2026.08.07.00"},
		{"前後の空白", " 2026.08.07.00 "},
		{"末尾改行", "2026.08.07.00\n"},
		{"区切りがハイフン", "2026-08-07-00"},
		// grammarは通るが日付として存在しない。
		{"2月30日", "2026.02.30.00"},
		{"平年の2月29日", "2026.02.29.00"},
		{"100年規則で平年", "1900.02.29.00"},
		{"4月31日", "2026.04.31.00"},
		{"6月31日", "2026.06.31.00"},
		{"9月31日", "2026.09.31.00"},
		{"11月31日", "2026.11.31.00"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if version, err := ParseClientVersion(c.text); err == nil {
				t.Fatalf("ParseClientVersion(%q) が %q として成功した", c.text, version.String())
			}
		})
	}
}

// TestClientVersionCompareIsIntegerTuple は比較が4個の10進整数tupleであることを
// 確かめる（§2）。
func TestClientVersionCompareIsIntegerTuple(t *testing.T) {
	// 昇順に並べた列。隣接だけでなく全組合せを見る。
	ordered := []string{
		"2025.12.31.99",
		"2026.01.01.00",
		"2026.01.01.01",
		"2026.01.02.00",
		"2026.02.01.00",
		"2026.08.07.00",
		"2026.08.07.09",
		"2026.08.07.10",
		"2026.08.07.99",
		"2026.08.08.00",
		"2026.12.31.00",
		"2027.01.01.00",
	}
	versions := make([]ClientVersion, len(ordered))
	for i, text := range ordered {
		versions[i] = mustParseClientVersion(t, text)
	}
	for i := range versions {
		for j := range versions {
			got := versions[i].Compare(versions[j])
			switch {
			case i < j && got >= 0:
				t.Errorf("Compare(%q, %q) = %d, want 負", ordered[i], ordered[j], got)
			case i > j && got <= 0:
				t.Errorf("Compare(%q, %q) = %d, want 正", ordered[i], ordered[j], got)
			case i == j && got != 0:
				t.Errorf("Compare(%q, %q) = %d, want 0", ordered[i], ordered[j], got)
			}
		}
	}
}

// TestClientVersionCompareIgnoresTextDifference は同じ整数tupleなら等しいことを
// 確かめる。
//
// grammarが桁数を固定するため同じtupleを別の文字列では書けないが、比較が文字列
// ではなくtupleで行われていることを固定する。
func TestClientVersionCompareIgnoresTextDifference(t *testing.T) {
	a := mustParseClientVersion(t, "2026.08.07.00")
	b := mustParseClientVersion(t, "2026.08.07.00")
	if got := a.Compare(b); got != 0 {
		t.Errorf("同じversionのCompare = %d, want 0", got)
	}
}

// TestClientVersionZeroValue はzero値がParseを通していないことを表すのを
// 確かめる。
func TestClientVersionZeroValue(t *testing.T) {
	var zero ClientVersion
	if !zero.IsZero() {
		t.Error("zero値のIsZero() = false")
	}
	if zero.String() != "" {
		t.Errorf("zero値のString() = %q, want 空", zero.String())
	}
	parsed := mustParseClientVersion(t, "2026.08.07.00")
	if parsed.IsZero() {
		t.Error("parse済みversionのIsZero() = true")
	}
}

// --- helper ---

func mustParseClientVersion(t *testing.T, text string) ClientVersion {
	t.Helper()
	version, err := ParseClientVersion(text)
	if err != nil {
		t.Fatalf("ParseClientVersion(%q): %v", text, err)
	}
	return version
}
