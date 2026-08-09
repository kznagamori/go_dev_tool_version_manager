package domain

import (
	"testing"
)

// --- ToolID ---

func TestParseToolID(t *testing.T) {
	valid := []string{"go", "node", "python", "dotnet-sdk", "a1", "a-1-b"}
	for _, text := range valid {
		if _, err := ParseToolID(text); err != nil {
			t.Errorf("ParseToolID(%q) = %v, want nil", text, err)
		}
	}
	invalid := []string{"", "Go", "go_lang", "-go", "go-", "go--lang", "go lang", "go.lang", "ゴー"}
	for _, text := range invalid {
		if _, err := ParseToolID(text); err == nil {
			t.Errorf("ParseToolID(%q) が成功した", text)
		}
	}
}

// --- Version: grammar ---

func TestParseVersionValid(t *testing.T) {
	cases := []struct {
		scheme VersionScheme
		text   string
	}{
		{SchemeSemver, "1.2.3"},
		{SchemeSemver, "0.0.0"},
		{SchemeSemver, "1.0.0-alpha"},
		{SchemeSemver, "1.0.0-rc.1"},
		{SchemeSemver, "1.0.0-0.3.7"},
		{SchemeGo, "1.20"},
		{SchemeGo, "1.20.1"},
		{SchemeGo, "1.20beta1"},
		{SchemeGo, "1.21rc2"},
		{SchemePython, "3.12.0"},
		{SchemePython, "3.13.0a1"},
		{SchemePython, "3.13.0b2"},
		{SchemePython, "3.13.0rc3"},
	}
	for _, c := range cases {
		version, err := ParseVersion(c.scheme, c.text)
		if err != nil {
			t.Errorf("ParseVersion(%s, %q) = %v, want nil", c.scheme, c.text, err)
			continue
		}
		if version.String() != c.text {
			t.Errorf("String() = %q, want %q（正規文字列をそのまま保持する）", version.String(), c.text)
		}
		if version.Scheme() != c.scheme {
			t.Errorf("Scheme() = %q, want %q", version.Scheme(), c.scheme)
		}
	}
}

func TestParseVersionInvalid(t *testing.T) {
	cases := []struct {
		scheme VersionScheme
		text   string
		why    string
	}{
		{SchemeSemver, "", "空"},
		{SchemeSemver, "v1.2.3", "leading v"},
		{SchemeSemver, "1.2.3+build.1", "build metadata"},
		{SchemeSemver, "1.2", "patch欠落"},
		{SchemeSemver, "01.2.3", "leading zero"},
		{SchemeSemver, "1.2.3-01", "prerelease数値のleading zero"},
		{SchemeSemver, "1.2.3-", "空prerelease"},
		{SchemeGo, "v1.20", "leading v"},
		{SchemeGo, "1.20beta0", "prerelease番号が0"},
		{SchemeGo, "1.20beta01", "prerelease番号のleading zero"},
		{SchemeGo, "1.20.1beta1", "patchとprereleaseの併用"},
		{SchemeGo, "1.20-rc1", "hyphen区切り"},
		{SchemeGo, "1", "minor欠落"},
		{SchemePython, "3.12", "patch欠落"},
		{SchemePython, "v3.12.0", "leading v"},
		{SchemePython, "3.13.0a0", "prerelease番号が0"},
		{SchemePython, "3.13.0alpha1", "未定義のprerelease種別"},
		{SchemePython, "3.13.0-rc1", "hyphen区切り"},
	}
	for _, c := range cases {
		if _, err := ParseVersion(c.scheme, c.text); err == nil {
			t.Errorf("ParseVersion(%s, %q) が成功した（%s）", c.scheme, c.text, c.why)
		}
	}
	if _, err := ParseVersion(VersionScheme("calver"), "2026.08.10"); err == nil {
		t.Error("未定義schemeが成功した")
	}
}

// --- Version: 比較 ---

// schemeごとの昇順列。隣接ペアがすべて < になることを確認する。
func TestVersionCompareOrder(t *testing.T) {
	cases := []struct {
		name   string
		scheme VersionScheme
		sorted []string
	}{
		{
			// 比較順は major/minor、beta<rc<final、prerelease番号、finalのpatch。
			"go", SchemeGo,
			[]string{"1.20beta1", "1.20beta2", "1.20rc1", "1.20", "1.20.1", "1.21beta1", "1.21", "2.0"},
		},
		{
			// 比較順は数値3要素、a<b<rc<final、prerelease番号。
			"python", SchemePython,
			[]string{"3.13.0a1", "3.13.0a2", "3.13.0b1", "3.13.0rc1", "3.13.0", "3.13.1", "3.14.0"},
		},
		{
			// SemVer 2.0.0のprecedence。
			"semver", SchemeSemver,
			[]string{
				"1.0.0-alpha", "1.0.0-alpha.1", "1.0.0-alpha.beta", "1.0.0-beta",
				"1.0.0-beta.2", "1.0.0-beta.11", "1.0.0-rc.1", "1.0.0", "1.0.1", "1.1.0", "2.0.0",
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			versions := make([]Version, len(c.sorted))
			for i, text := range c.sorted {
				v, err := ParseVersion(c.scheme, text)
				if err != nil {
					t.Fatalf("ParseVersion(%q): %v", text, err)
				}
				versions[i] = v
			}
			for i := 0; i+1 < len(versions); i++ {
				got, err := versions[i].Compare(versions[i+1])
				if err != nil {
					t.Fatalf("Compare: %v", err)
				}
				if got >= 0 {
					t.Errorf("%q < %q のはずが Compare = %d", c.sorted[i], c.sorted[i+1], got)
				}
				back, err := versions[i+1].Compare(versions[i])
				if err != nil {
					t.Fatalf("Compare: %v", err)
				}
				if back <= 0 {
					t.Errorf("%q > %q のはずが Compare = %d", c.sorted[i+1], c.sorted[i], back)
				}
			}
		})
	}
}

// go schemeの 1.20 と 1.20.0 は同じcomparison keyになる。
// 同一catalogへ併存させない前提だが、比較そのものは0を返す。
func TestVersionGoOmittedPatchEqualsZero(t *testing.T) {
	a, err := ParseVersion(SchemeGo, "1.20")
	if err != nil {
		t.Fatal(err)
	}
	b, err := ParseVersion(SchemeGo, "1.20.0")
	if err != nil {
		t.Fatal(err)
	}
	got, err := a.Compare(b)
	if err != nil {
		t.Fatal(err)
	}
	if got != 0 {
		t.Fatalf("Compare(1.20, 1.20.0) = %d, want 0", got)
	}
	// 正規文字列は別物であり、入力一致はbyte完全一致で行う。
	if a.String() == b.String() {
		t.Fatal("正規文字列まで同一になっている")
	}
}

func TestVersionCompareRejectsMixedScheme(t *testing.T) {
	goVersion, err := ParseVersion(SchemeGo, "1.20")
	if err != nil {
		t.Fatal(err)
	}
	pyVersion, err := ParseVersion(SchemePython, "1.20.0")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := goVersion.Compare(pyVersion); err == nil {
		t.Fatal("異なるscheme同士の比較が成功した")
	}
}

func TestVersionCompareRejectsZeroValue(t *testing.T) {
	v, err := ParseVersion(SchemeSemver, "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := v.Compare(Version{}); err == nil {
		t.Fatal("未初期化versionとの比較が成功した")
	}
}

func TestParseVersionScheme(t *testing.T) {
	for _, text := range []string{"semver", "go", "python"} {
		if _, err := ParseVersionScheme(text); err != nil {
			t.Errorf("ParseVersionScheme(%q) = %v", text, err)
		}
	}
	for _, text := range []string{"", "calver", "SemVer"} {
		if _, err := ParseVersionScheme(text); err == nil {
			t.Errorf("ParseVersionScheme(%q) が成功した", text)
		}
	}
}

// --- Platform ---

func TestParsePlatform(t *testing.T) {
	windows, err := ParsePlatform(PlatformWindowsAMD64)
	if err != nil {
		t.Fatal(err)
	}
	if windows.OS() != OSWindows || windows.Arch() != ArchAMD64 || windows.Libc() != LibcNone {
		t.Fatalf("windows tuple が不正: %+v", windows)
	}
	if windows.ExecutableSuffix() != ".exe" {
		t.Fatalf("windows suffix = %q, want .exe", windows.ExecutableSuffix())
	}

	linux, err := ParsePlatform(PlatformLinuxAMD64Glibc)
	if err != nil {
		t.Fatal(err)
	}
	if linux.OS() != OSLinux || linux.Libc() != LibcGlibc {
		t.Fatalf("linux tuple が不正: %+v", linux)
	}
	if linux.ExecutableSuffix() != "" {
		t.Fatalf("linux suffix = %q, want 空", linux.ExecutableSuffix())
	}

	for _, id := range []string{"", "darwin-arm64", "linux-amd64", "windows-amd64-msvc"} {
		if _, err := ParsePlatform(id); err == nil {
			t.Errorf("ParsePlatform(%q) が成功した", id)
		}
	}
}

// --- Digest ---

const (
	sha256Hex = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	sha512Hex = sha256Hex + sha256Hex
)

func TestParseUpstreamDigest(t *testing.T) {
	d256, err := ParseUpstreamDigest("sha256:" + sha256Hex)
	if err != nil {
		t.Fatal(err)
	}
	if d256.Algorithm() != AlgoSHA256 || d256.Hex() != sha256Hex {
		t.Fatalf("sha256 digest が不正: %+v", d256)
	}
	if d256.Upstream() != "sha256:"+sha256Hex {
		t.Fatalf("Upstream() = %q", d256.Upstream())
	}
	if _, err := ParseUpstreamDigest("sha512:" + sha512Hex); err != nil {
		t.Fatal(err)
	}

	invalid := []string{
		"",
		sha256Hex,                  // algorithm欠落
		"sha1:" + sha256Hex,        // 未対応algorithm
		"sha256:" + sha512Hex,      // 桁数不一致
		"sha512:" + sha256Hex,      // 桁数不一致
		"sha256:" + sha256Hex[:63], // 短い
		"sha256:0123456789ABCDEF" + sha256Hex[16:], // 大文字hex
	}
	for _, text := range invalid {
		if _, err := ParseUpstreamDigest(text); err == nil {
			t.Errorf("ParseUpstreamDigest(%q) が成功した", text)
		}
	}
}

func TestInternalDigestIsSHA256Only(t *testing.T) {
	internal, err := ParseInternalDigest(sha256Hex)
	if err != nil {
		t.Fatal(err)
	}
	hex, err := internal.Internal()
	if err != nil {
		t.Fatal(err)
	}
	if hex != sha256Hex {
		t.Fatalf("Internal() = %q", hex)
	}

	// sha512をinternal形式で出そうとするのは形式の取り違えである。
	upstream512, err := ParseUpstreamDigest("sha512:" + sha512Hex)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := upstream512.Internal(); err == nil {
		t.Fatal("sha512でInternal()が成功した")
	}

	if _, err := ParseInternalDigest(sha512Hex); err == nil {
		t.Fatal("128桁のinternal digestが成功した")
	}
}

func TestDigestEqualRequiresSameAlgorithm(t *testing.T) {
	a, err := ParseUpstreamDigest("sha256:" + sha256Hex)
	if err != nil {
		t.Fatal(err)
	}
	b, err := ParseInternalDigest(sha256Hex)
	if err != nil {
		t.Fatal(err)
	}
	if !a.Equal(b) {
		t.Fatal("同じalgorithm・同じhexが不一致になった")
	}
	c, err := ParseUpstreamDigest("sha512:" + sha512Hex)
	if err != nil {
		t.Fatal(err)
	}
	if a.Equal(c) {
		t.Fatal("algorithmが異なる値が一致になった")
	}
}

// --- PathRole / PathValue ---

func TestPathRoleIsExactly22(t *testing.T) {
	if len(pathRoles) != PathRoleCount {
		t.Fatalf("path_roleが%d件、want %d件（§17.2）", len(pathRoles), PathRoleCount)
	}
	for _, text := range []string{"data-root", "report", "current-link", "shim-index"} {
		if _, err := ParsePathRole(text); err != nil {
			t.Errorf("ParsePathRole(%q) = %v", text, err)
		}
	}
	for _, text := range []string{"", "root", "Data-Root", "temp"} {
		if _, err := ParsePathRole(text); err == nil {
			t.Errorf("ParsePathRole(%q) が成功した", text)
		}
	}
}

func TestPathValueWithoutPathKeepsRole(t *testing.T) {
	value, err := NewPathValue(RoleReceipt, "/data/tools/go/1.26.5/.gdtvm-install.toml")
	if err != nil {
		t.Fatal(err)
	}
	masked := value.WithoutPath()
	if masked.Role() != RoleReceipt {
		t.Fatalf("Role() = %q", masked.Role())
	}
	if masked.Path() != "" {
		t.Fatalf("Path() = %q, want 空", masked.Path())
	}
	if value.Path() == "" {
		t.Fatal("元のPathValueが変更された")
	}

	// pathが空でもroleが正しければ作れる（typed error用）。
	if _, err := NewPathValue(RoleConfig, ""); err != nil {
		t.Fatalf("空pathのPathValue: %v", err)
	}
	if _, err := NewPathValue(PathRole("temp"), "/tmp"); err == nil {
		t.Fatal("未定義roleのPathValueが成功した")
	}
}

// --- InstallKey ---

func newTestKey(t *testing.T) InstallKey {
	t.Helper()
	tool, err := ParseToolID("go")
	if err != nil {
		t.Fatal(err)
	}
	version, err := ParseVersion(SchemeGo, "1.26.5")
	if err != nil {
		t.Fatal(err)
	}
	platform, err := ParsePlatform(PlatformLinuxAMD64Glibc)
	if err != nil {
		t.Fatal(err)
	}
	key, err := NewInstallKey(tool, version, platform)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func TestInstallKey(t *testing.T) {
	key := newTestKey(t)
	if got, want := key.String(), "go@1.26.5/linux-amd64-glibc"; got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
	if !key.Equal(newTestKey(t)) {
		t.Fatal("同一要素のInstallKeyが不一致になった")
	}

	windows, err := ParsePlatform(PlatformWindowsAMD64)
	if err != nil {
		t.Fatal(err)
	}
	other, err := NewInstallKey(key.Tool(), key.Version(), windows)
	if err != nil {
		t.Fatal(err)
	}
	if key.Equal(other) {
		t.Fatal("platformが違うInstallKeyが一致になった")
	}
}

// 1.20 と 1.20.0 は同じcomparison keyだが、InstallKeyとしては別物である。
// 入力一致を正規文字列のbyte完全一致で行う契約を型で守る。
func TestInstallKeyUsesExactVersionText(t *testing.T) {
	tool, err := ParseToolID("go")
	if err != nil {
		t.Fatal(err)
	}
	platform, err := ParsePlatform(PlatformLinuxAMD64Glibc)
	if err != nil {
		t.Fatal(err)
	}
	short, err := ParseVersion(SchemeGo, "1.20")
	if err != nil {
		t.Fatal(err)
	}
	long, err := ParseVersion(SchemeGo, "1.20.0")
	if err != nil {
		t.Fatal(err)
	}
	a, err := NewInstallKey(tool, short, platform)
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewInstallKey(tool, long, platform)
	if err != nil {
		t.Fatal(err)
	}
	if a.Equal(b) {
		t.Fatal("正規文字列が異なるInstallKeyが一致になった")
	}
}

func TestNewInstallKeyRejectsMissingParts(t *testing.T) {
	key := newTestKey(t)
	if _, err := NewInstallKey(ToolID{}, key.Version(), key.Platform()); err == nil {
		t.Error("tool未設定が成功した")
	}
	if _, err := NewInstallKey(key.Tool(), Version{}, key.Platform()); err == nil {
		t.Error("version未設定が成功した")
	}
	if _, err := NewInstallKey(key.Tool(), key.Version(), Platform{}); err == nil {
		t.Error("platform未設定が成功した")
	}
}

// --- EffectiveSelection ---

func TestEffectiveSelection(t *testing.T) {
	key := newTestKey(t)
	origin, err := NewPathValue(RoleProjectFile, "/work/.gdtvm.toml")
	if err != nil {
		t.Fatal(err)
	}

	selection, err := NewEffectiveSelection(
		key.Tool(), key.Version(), SelectionSourceProject, origin, HealthHealthy)
	if err != nil {
		t.Fatal(err)
	}
	if !selection.HasSelection() {
		t.Fatal("HasSelection() = false")
	}
	if selection.Source() != SelectionSourceProject || selection.Origin().Role() != RoleProjectFile {
		t.Fatalf("selection が不正: %+v", selection)
	}

	none, err := NoSelection(key.Tool())
	if err != nil {
		t.Fatal(err)
	}
	if none.HasSelection() {
		t.Fatal("NoSelection が選択ありになっている")
	}
	if none.Source() != SelectionSourceNone || !none.Version().IsZero() {
		t.Fatalf("NoSelection が不正: %+v", none)
	}
}

func TestNewEffectiveSelectionRejectsInvalidCombinations(t *testing.T) {
	key := newTestKey(t)
	origin, err := NewPathValue(RoleConfig, "/home/u/.config/gdtvm/selections.toml")
	if err != nil {
		t.Fatal(err)
	}

	// 選択ありで source=none は矛盾である。
	if _, err := NewEffectiveSelection(
		key.Tool(), key.Version(), SelectionSourceNone, origin, HealthHealthy); err == nil {
		t.Error("source=none が成功した")
	}
	// origin 未設定は由来を追えない。
	if _, err := NewEffectiveSelection(
		key.Tool(), key.Version(), SelectionSourceUser, PathValue{}, HealthHealthy); err == nil {
		t.Error("origin未設定が成功した")
	}
	// 未定義のhealthは受理しない。
	if _, err := NewEffectiveSelection(
		key.Tool(), key.Version(), SelectionSourceUser, origin, Health("broken")); err == nil {
		t.Error("未定義healthが成功した")
	}
	if _, err := NewEffectiveSelection(
		ToolID{}, key.Version(), SelectionSourceUser, origin, HealthHealthy); err == nil {
		t.Error("tool未設定が成功した")
	}
}

// --- enum ---

func TestEnumParsers(t *testing.T) {
	type parser struct {
		name    string
		fn      func(string) error
		valid   []string
		invalid []string
	}
	parsers := []parser{
		{"mode", func(s string) error { _, err := ParseMode(s); return err },
			[]string{"portable", "user"}, []string{"", "system", "Portable"}},
		{"scope", func(s string) error { _, err := ParseScope(s); return err },
			[]string{"user", "project"}, []string{"", "global", "User"}},
		{"channel", func(s string) error { _, err := ParseChannel(s); return err },
			[]string{"stable", "prerelease", ""}, []string{"beta", "Stable"}},
		{"lifecycle", func(s string) error { _, err := ParseLifecycle(s); return err },
			[]string{"supported", "eol", "unknown", ""}, []string{"maintenance", "EOL"}},
		{"source", func(s string) error { _, err := ParseSelectionSource(s); return err },
			[]string{"project", "user", "none"}, []string{"", "default"}},
		{"health", func(s string) error { _, err := ParseHealth(s); return err },
			[]string{"healthy", "unhealthy", "unknown"}, []string{"", "degraded"}},
	}
	for _, p := range parsers {
		t.Run(p.name, func(t *testing.T) {
			for _, text := range p.valid {
				if err := p.fn(text); err != nil {
					t.Errorf("%q = %v, want nil", text, err)
				}
			}
			for _, text := range p.invalid {
				if err := p.fn(text); err == nil {
					t.Errorf("%q が成功した", text)
				}
			}
		})
	}
}
