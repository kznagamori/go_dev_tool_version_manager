package definition

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// overrideBlock はdocs/06-tool-definition.md §6.4の正規例である。
const overrideBlock = `
[[platforms.version_source.lifecycle_overrides]]
version = "18.20.8"
status = "eol"
evidence = "https://example.invalid/official-lifecycle"
assessed_at = 2026-08-07T00:00:00Z
`

// TestLifecycleOverrideAcceptsSpecExample は§6.4の正規例が通ることを固定する。
func TestLifecycleOverrideAcceptsSpecExample(t *testing.T) {
	value, err := withSource(t, specVersionSourceBlock+overrideBlock)
	if err != nil {
		t.Fatalf("Parse = %s", describe(err))
	}
	overrides := value.Platforms[0].VersionSource.LifecycleOverrides
	if len(overrides) != 1 {
		t.Fatalf("overrides = %d件", len(overrides))
	}
	override := overrides[0]
	if override.Version.String() != "18.20.8" || override.Status != LifecycleEOL {
		t.Errorf("override = %+v", override)
	}
	if !override.AssessedAt.Equal(time.Date(2026, 8, 7, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("assessed_at = %v", override.AssessedAt)
	}
}

// TestLifecycleOverrideRejects は§6.4の4 key契約を固定する。
func TestLifecycleOverrideRejects(t *testing.T) {
	tests := []struct {
		name       string
		old, value string
		wantReason string
	}{
		{"versionが欠落", `version = "18.20.8"` + "\n", "", reasonMissing},
		{"statusが欠落", `status = "eol"` + "\n", "", reasonMissing},
		{"evidenceが欠落", `evidence = "https://example.invalid/official-lifecycle"` + "\n", "", reasonMissing},
		{"assessed_atが欠落", `assessed_at = 2026-08-07T00:00:00Z` + "\n", "", reasonMissing},
		// §6.4はstatusを`supported|eol`へ限る。`unknown`は「判断していない」で
		// あり、根拠を添えて上書きする対象にならない。
		{"statusがunknown", `status = "eol"`, `status = "unknown"`, reasonEnum},
		{"statusがenum外", `status = "eol"`, `status = "retired"`, reasonEnum},
		{"versionが部分版", `version = "18.20.8"`, `version = "18.20"`, reasonVersion},
		{"versionがrange", `version = "18.20.8"`, `version = ">=18"`, reasonVersion},
		{"versionにleading v", `version = "18.20.8"`, `version = "v18.20.8"`, reasonVersion},
		{"evidenceがHTTP", `evidence = "https://example.invalid/official-lifecycle"`,
			`evidence = "http://example.invalid/x"`, reasonURL},
		{"assessed_atが非UTC", `assessed_at = 2026-08-07T00:00:00Z`,
			`assessed_at = 2026-08-07T00:00:00+09:00`, reasonTime},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			block := specVersionSourceBlock + strings.Replace(overrideBlock, test.old, test.value, 1)
			_, err := withSource(t, block)
			if err == nil {
				t.Fatal("Parse = nil, want error")
			}
			assertReason(t, err, test.wantReason)
		})
	}
}

// TestLifecycleOverrideRejectsDuplicateVersion は§6.4の「同一source内で一意」を固定する。
func TestLifecycleOverrideRejectsDuplicateVersion(t *testing.T) {
	_, err := withSource(t, specVersionSourceBlock+overrideBlock+overrideBlock)
	if err == nil {
		t.Fatal("同じversionのoverrideが2件でも通った")
	}
	assertReason(t, err, reasonDuplicate)
}

// TestStaticVersionRejects は§6.6の7 key契約を固定する。
func TestStaticVersionRejects(t *testing.T) {
	tests := []struct {
		name       string
		old, value string
		wantReason string
	}{
		{"versionが欠落", `version = "3.13.7"` + "\n", "", reasonMissing},
		{"channelが欠落", `channel = "stable"` + "\n", "", reasonMissing},
		{"lifecycleが欠落", `lifecycle = "supported"` + "\n", "", reasonMissing},
		{"lifecycle_evidenceが欠落",
			`lifecycle_evidence = "https://devguide.python.org/versions/"` + "\n", "", reasonMissing},
		{"lifecycle_assessed_atが欠落",
			`lifecycle_assessed_at = 2026-08-07T00:00:00Z` + "\n", "", reasonMissing},
		{"published_atが欠落", `published_at = "2025-08-14T00:00:00Z"` + "\n" + `
[[platforms.version_source.static_versions.assets]]`, `
[[platforms.version_source.static_versions.assets]]`, reasonMissing},
		{"channelがenum外", `channel = "stable"`, `channel = "beta"`, reasonEnum},
		{"lifecycleがenum外", `lifecycle = "supported"`, `lifecycle = "active"`, reasonEnum},
		{"versionがschemeに合わない", `version = "3.13.7"`, `version = "3.13"`, reasonVersion},
		{"lifecycle_assessed_atが非UTC", `lifecycle_assessed_at = 2026-08-07T00:00:00Z`,
			`lifecycle_assessed_at = 2026-08-07T00:00:00+09:00`, reasonTime},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			block := strings.Replace(staticSourceBlock, test.old, test.value, 1)
			if block == staticSourceBlock {
				t.Fatalf("差し替え対象 %q がblockに無い", test.old)
			}
			_, err := parseSpec(t,
				`version_scheme = "semver"`, `version_scheme = "python"`,
				specVersionSourceBlock, block)
			if err == nil {
				t.Fatal("Parse = nil, want error")
			}
			assertReason(t, err, test.wantReason)
		})
	}
}

// TestStaticVersionAcceptsUnknownLifecycle は§6.6のchannel/lifecycle独立を固定する。
//
// 「両者は独立であり全6組合せを表現できる」。`unknown`でも「不明と判断した
// 調査根拠」をevidenceへ残す。
func TestStaticVersionAcceptsAllChannelLifecyclePairs(t *testing.T) {
	channels := []Channel{ChannelStable, ChannelPrerelease}
	lifecycles := []Lifecycle{LifecycleSupported, LifecycleEOL, LifecycleUnknown}
	pairs := 0
	for _, channel := range channels {
		for _, lifecycle := range lifecycles {
			pairs++
			t.Run(fmt.Sprintf("%s/%s", channel, lifecycle), func(t *testing.T) {
				block := strings.Replace(staticSourceBlock,
					`channel = "stable"`, fmt.Sprintf("channel = %q", channel), 1)
				block = strings.Replace(block,
					`lifecycle = "supported"`, fmt.Sprintf("lifecycle = %q", lifecycle), 1)
				// prereleaseはpython schemeの正規prerelease構文で書く。
				if channel == ChannelPrerelease {
					block = strings.Replace(block, `version = "3.13.7"`, `version = "3.14.0rc1"`, 1)
				}
				if _, err := parseSpec(t,
					`version_scheme = "semver"`, `version_scheme = "python"`,
					specVersionSourceBlock, block); err != nil {
					t.Errorf("%s/%s が落ちた: %s", channel, lifecycle, describe(err))
				}
			})
		}
	}
	if pairs != 6 {
		t.Fatalf("組合せ = %d件, want 6件", pairs)
	}
}

// TestStaticAssetRejects は§6.6のasset field契約を固定する。
func TestStaticAssetRejects(t *testing.T) {
	tests := []struct {
		name       string
		old, value string
		wantReason string
	}{
		{"sizeが0", "size = 1", "size = 0", reasonLimit},
		{"sizeが負", "size = 1", "size = -1", reasonLimit},
		{"digest_algorithmがenum外", `digest_algorithm = "sha256"`,
			`digest_algorithm = "sha1"`, reasonEnum},
		// hex長がalgorithmと一致しない値を拒否する（§6.5）。
		{"sha256でhexが128文字", `digest_algorithm = "sha256"`,
			`digest_algorithm = "sha512"`, reasonDigest},
		{"digestが大文字", strings.Repeat("0", 64), strings.Repeat("A", 64), reasonDigest},
		{"digestが短い", strings.Repeat("0", 64), strings.Repeat("0", 63), reasonDigest},
		{"digestにprefix", strings.Repeat("0", 64), "sha256:" + strings.Repeat("0", 64), reasonDigest},
		{"osがenum外", `os = "windows"`, `os = "darwin"`, reasonEnum},
		{"archがenum外", `arch = "amd64"`, `arch = "arm64"`, reasonEnum},
		{"libcがenum外", `libc = "none"`, `libc = "musl"`, reasonEnum},
		{"release_idが負", `release_id = "0"`, `release_id = "-1"`, reasonText},
		{"release_idにleading zero", `release_id = "0"`, `release_id = "01"`, reasonText},
		{"asset_idが数値でない", `asset_id = "0"`, `asset_id = "abc"`, reasonText},
		{"urlがHTTP", "https://github.com/astral-sh/python-build-standalone/releases/download/20250814/cpython.tar.gz",
			"http://example.invalid/cpython.tar.gz", reasonURL},
		{"nameに区切り", `name = "cpython-3.13.7-x86_64-pc-windows-msvc-install_only_stripped.tar.gz"`,
			`name = "dist/cpython.tar.gz"`, reasonText},
		{"published_atが非UTC", `published_at = "2025-08-14T00:00:00Z"`,
			`published_at = "2025-08-14T00:00:00+09:00"`, reasonTime},
		{"published_atが形式外", `published_at = "2025-08-14T00:00:00Z"`,
			`published_at = "2025/08/14"`, reasonTime},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			block := strings.Replace(staticSourceBlock, test.old, test.value, 1)
			if block == staticSourceBlock {
				t.Fatalf("差し替え対象 %q がblockに無い", test.old)
			}
			_, err := parseSpec(t,
				`version_scheme = "semver"`, `version_scheme = "python"`,
				specVersionSourceBlock, block)
			if err == nil {
				t.Fatal("Parse = nil, want error")
			}
			assertReason(t, err, test.wantReason)
		})
	}
}

// TestStaticAssetRequiresEveryField は§6.6の「asset itemの許可keyは全件必須」を
// 1行ずつ落として確かめる。
func TestStaticAssetRequiresEveryField(t *testing.T) {
	marker := "[[platforms.version_source.static_versions.assets]]"
	index := strings.Index(staticSourceBlock, marker)
	head, assetBlock := staticSourceBlock[:index], staticSourceBlock[index:]
	lines := strings.Split(strings.TrimRight(assetBlock, "\n"), "\n")

	removed := 0
	for position, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "[") {
			continue
		}
		key, _, found := strings.Cut(trimmed, " =")
		if !found {
			continue
		}
		removed++
		t.Run(key, func(t *testing.T) {
			reduced := strings.Join(
				append(append([]string{}, lines[:position]...), lines[position+1:]...), "\n")
			_, err := parseSpec(t,
				`version_scheme = "semver"`, `version_scheme = "python"`,
				specVersionSourceBlock, head+reduced+"\n")
			if err == nil {
				t.Errorf("asset key %q が無くても通った", key)
			}
		})
	}
	if removed != AssetFieldCount {
		t.Fatalf("削除対象のasset keyが%d件、want %d件", removed, AssetFieldCount)
	}
}

// TestStaticVersionRejectsDuplicates は§6.6の一意契約を固定する。
func TestStaticVersionRejectsDuplicates(t *testing.T) {
	marker := "[[platforms.version_source.static_versions]]"
	index := strings.Index(staticSourceBlock, marker)
	entry := staticSourceBlock[index:]

	t.Run("同じversionが2件", func(t *testing.T) {
		_, err := parseSpec(t,
			`version_scheme = "semver"`, `version_scheme = "python"`,
			specVersionSourceBlock, staticSourceBlock+"\n"+entry)
		if err == nil {
			t.Fatal("同じstatic versionが2件でも通った")
		}
		assertReason(t, err, reasonDuplicate)
	})

	t.Run("同じasset nameが2件", func(t *testing.T) {
		assetMarker := "[[platforms.version_source.static_versions.assets]]"
		assetIndex := strings.Index(staticSourceBlock, assetMarker)
		asset := staticSourceBlock[assetIndex:]
		_, err := parseSpec(t,
			`version_scheme = "semver"`, `version_scheme = "python"`,
			specVersionSourceBlock, staticSourceBlock+"\n"+asset)
		if err == nil {
			t.Fatal("同名assetが2件でも通った")
		}
		assertReason(t, err, reasonDuplicate)
	})
}

// TestStaticVersionSetsMustMatchAcrossPlatforms は§6.6のplatform間一致を固定する。
//
// 「registry validatorは両platformの正規version集合が完全一致することを検査し、
// 片方だけの更新漏れを拒否する」。
func TestStaticVersionSetsMustMatchAcrossPlatforms(t *testing.T) {
	windows := strings.Replace(specDefinitionTOML, specVersionSourceBlock, staticSourceBlock, 1)
	windows = strings.Replace(windows, `version_scheme = "semver"`, `version_scheme = "python"`, 1)
	platformIndex := strings.Index(windows, "[[platforms]]")
	linux := strings.NewReplacer(
		`id = "windows-amd64"`, `id = "linux-amd64-glibc"`,
		`os = "windows"`, `os = "linux"`,
		`libc = "none"`, `libc = "glibc"`,
	).Replace(windows[platformIndex:])

	t.Run("同じversion集合は通る", func(t *testing.T) {
		if _, err := Parse(specDefinitionPath, []byte(windows+"\n"+linux)); err != nil {
			t.Fatalf("Parse = %s", describe(err))
		}
	})

	t.Run("片方だけ更新されている", func(t *testing.T) {
		stale := strings.Replace(linux, `version = "3.13.7"`, `version = "3.13.6"`, 1)
		_, err := Parse(specDefinitionPath, []byte(windows+"\n"+stale))
		if err == nil {
			t.Fatal("platform間でversion集合が違っても通った")
		}
		assertReason(t, err, reasonPlatformSet)
	})

	t.Run("片方だけ件数が多い", func(t *testing.T) {
		extraEntry := strings.Replace(
			staticSourceBlock[strings.Index(staticSourceBlock, "[[platforms.version_source.static_versions]]"):],
			`version = "3.13.7"`, `version = "3.12.11"`, 1)
		extra := linux + "\n" + extraEntry
		_, err := Parse(specDefinitionPath, []byte(windows+"\n"+extra))
		if err == nil {
			t.Fatal("platform間でversion件数が違っても通った")
		}
		assertReason(t, err, reasonPlatformSet)
	})
}
