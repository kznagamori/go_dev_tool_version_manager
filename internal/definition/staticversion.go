package definition

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
)

// 上限（docs/04-storage-and-data.md §21）。
const (
	// StaticVersionMax は1 sourceのstatic version数の上限である。
	StaticVersionMax = 10000
	// StaticAssetMax は1 versionのasset数の上限である。
	StaticAssetMax = 16
)

// DigestAlgorithm はchecksumのalgorithmである（§6.6・§7.2）。
type DigestAlgorithm string

// DigestAlgorithm の値。schema 1はこの2件だけを扱う。
const (
	// AlgorithmSHA256 はSHA-256である。hexは64文字。
	AlgorithmSHA256 DigestAlgorithm = "sha256"
	// AlgorithmSHA512 はSHA-512である。hexは128文字。
	AlgorithmSHA512 DigestAlgorithm = "sha512"
)

// digestHexLength はalgorithmごとのhex長である（§6.5「hex長がalgorithmと
// 一致しない値を拒否する」）。
var digestHexLength = map[DigestAlgorithm]int{
	AlgorithmSHA256: 64,
	AlgorithmSHA512: 128,
}

var (
	// lowerHexRe はalgorithm prefixなしのlowercase hexである（§6.5）。
	lowerHexRe = regexp.MustCompile(`^[0-9a-f]+$`)
	// decimalIDRe は非負decimal stringである（§6.6）。
	//
	// 数値でなくstringとして扱うのは、上流のIDが2^53を超えてもprecision lossを
	// 起こさないようにするためである（§6.5）。
	decimalIDRe = regexp.MustCompile(`^(?:0|[1-9][0-9]*)$`)
)

// StaticVersion は§6.6の`static_versions` 1件である。7 key全件必須。
type StaticVersion struct {
	// Version は正規完全versionである。source内で一意。
	Version domain.Version
	// Channel はversionの安定度である。
	Channel Channel
	// Lifecycle は上流のsupport状態である。channelとは独立で全6組合せを取れる。
	Lifecycle Lifecycle
	// LifecycleEvidence はprovider/official projectのHTTPS一次資料である。
	//
	// `unknown`でも「不明と判断した調査根拠」を残す（§6.6）。
	LifecycleEvidence string
	// LifecycleAssessedAt は根拠を確認したUTC時刻である。
	LifecycleAssessedAt time.Time
	// PublishedAt は上流の公開日時（UTC RFC 3339）である。
	PublishedAt time.Time
	// Assets はこのversionの配布物である。
	Assets []StaticAsset
}

// StaticAsset は§6.6の`static_versions.assets` 1件である。
//
// §6.5のasset field集合をすべて持ち、全件必須である。
type StaticAsset struct {
	Name            string
	URL             string
	Size            int64
	Digest          string
	DigestAlgorithm DigestAlgorithm
	OS              domain.OS
	Arch            domain.Arch
	Libc            domain.Libc
	PublishedAt     time.Time
	ReleaseTag      string
	ReleaseURL      string
	ReleaseID       string
	AssetID         string
}

type staticTable struct {
	Version             *string             `toml:"version"`
	Channel             *string             `toml:"channel"`
	Lifecycle           *string             `toml:"lifecycle"`
	LifecycleEvidence   *string             `toml:"lifecycle_evidence"`
	LifecycleAssessedAt *time.Time          `toml:"lifecycle_assessed_at"`
	PublishedAt         *string             `toml:"published_at"`
	Assets              *[]staticAssetTable `toml:"assets"`
}

type staticAssetTable struct {
	Name            *string `toml:"name"`
	URL             *string `toml:"url"`
	Size            *int64  `toml:"size"`
	Digest          *string `toml:"digest"`
	DigestAlgorithm *string `toml:"digest_algorithm"`
	OS              *string `toml:"os"`
	Arch            *string `toml:"arch"`
	Libc            *string `toml:"libc"`
	PublishedAt     *string `toml:"published_at"`
	ReleaseTag      *string `toml:"release_tag"`
	ReleaseURL      *string `toml:"release_url"`
	ReleaseID       *string `toml:"release_id"`
	AssetID         *string `toml:"asset_id"`
}

// buildStaticVersions は§6.6の`static_versions`を検証する。
//
// 「static sourceはversion itemをfile記載順で解釈せず、正規version byteで一意
// 検査してcomparison keyでsortする」。ここでは一意検査までを行い、sortは
// catalog生成側（P3-03）が行う。
func buildStaticVersions(
	raw *[]staticTable, field string, scheme domain.VersionScheme, diagnostics *Diagnostics,
) []StaticVersion {
	if raw == nil {
		diagnostics.Add(field, reason(reasonMissing), "kind=staticでは`static_versions`が必須")
		return nil
	}
	entries := *raw
	switch {
	case len(entries) == 0:
		diagnostics.Add(field, reason(reasonConditional), "`static_versions`が空配列")
		return nil
	case len(entries) > StaticVersionMax:
		diagnostics.Add(field, reason(reasonLimit),
			fmt.Sprintf("static_versionが%d件を超える（%d件）", StaticVersionMax, len(entries)))
		return nil
	}
	values := make([]StaticVersion, 0, len(entries))
	versions := make([]string, 0, len(entries))
	for index := range entries {
		scope := fmt.Sprintf("%s[%d]", field, index)
		value, ok := buildStaticVersion(&entries[index], scope, scheme, diagnostics)
		if !ok {
			continue
		}
		values = append(values, value)
		versions = append(versions, value.Version.String())
	}
	if err := requireUniqueIdentifiers("static version", versions); err != nil {
		diagnostics.Add(field, reason(reasonDuplicate), err.Error())
		return nil
	}
	return values
}

func buildStaticVersion(
	table *staticTable, field string, scheme domain.VersionScheme, diagnostics *Diagnostics,
) (StaticVersion, bool) {
	var value StaticVersion
	ok := true

	version, versionOK := requireExactVersion(table.Version, field+".version", scheme, diagnostics)
	if !versionOK {
		ok = false
	}
	value.Version = version

	if channel, channelOK := requireEnumText(
		table.Channel, field+".channel", parseChannel, diagnostics); channelOK {
		value.Channel = channel
	} else {
		ok = false
	}
	if lifecycle, lifecycleOK := requireEnumText(
		table.Lifecycle, field+".lifecycle", parseLifecycle, diagnostics); lifecycleOK {
		value.Lifecycle = lifecycle
	} else {
		ok = false
	}

	value.LifecycleEvidence = requireHTTPSURL(
		table.LifecycleEvidence, field+".lifecycle_evidence", urlReference, diagnostics)
	if value.LifecycleEvidence == "" {
		ok = false
	}
	assessedAt, assessedOK := requireUTCTime(
		table.LifecycleAssessedAt, field+".lifecycle_assessed_at", diagnostics)
	if !assessedOK {
		ok = false
	}
	value.LifecycleAssessedAt = assessedAt

	publishedAt, publishedOK := requireRFC3339(table.PublishedAt, field+".published_at", diagnostics)
	if !publishedOK {
		ok = false
	}
	value.PublishedAt = publishedAt

	assets, assetsOK := buildStaticAssets(table.Assets, field+".assets", diagnostics)
	if !assetsOK {
		ok = false
	}
	value.Assets = assets
	return value, ok
}

func buildStaticAssets(
	raw *[]staticAssetTable, field string, diagnostics *Diagnostics,
) ([]StaticAsset, bool) {
	if raw == nil {
		diagnostics.Add(field, reason(reasonMissing), "`assets`が無い")
		return nil, false
	}
	entries := *raw
	switch {
	case len(entries) == 0:
		diagnostics.Add(field, reason(reasonConditional), "`assets`が空配列")
		return nil, false
	case len(entries) > StaticAssetMax:
		diagnostics.Add(field, reason(reasonLimit),
			fmt.Sprintf("assetが%d件を超える（%d件）", StaticAssetMax, len(entries)))
		return nil, false
	}
	values := make([]StaticAsset, 0, len(entries))
	names := make([]string, 0, len(entries))
	ok := true
	for index := range entries {
		scope := fmt.Sprintf("%s[%d]", field, index)
		value, entryOK := buildStaticAsset(&entries[index], scope, diagnostics)
		if !entryOK {
			ok = false
			continue
		}
		values = append(values, value)
		names = append(names, value.Name)
	}
	// selectorが`name_regex`でexactly 1件を選ぶため、同名assetがあると
	// 選択が一意に決まらない（§7.1）。
	if err := requireUniqueIdentifiers("asset name", names); err != nil {
		diagnostics.Add(field, reason(reasonDuplicate), err.Error())
		return nil, false
	}
	return values, ok
}

func buildStaticAsset(
	table *staticAssetTable, field string, diagnostics *Diagnostics,
) (StaticAsset, bool) {
	var value StaticAsset
	ok := true

	value.Name = requireFileName(table.Name, field+".name", diagnostics)
	if value.Name == "" {
		ok = false
	}
	value.URL = requireHTTPSURL(table.URL, field+".url", urlEndpoint, diagnostics)
	if value.URL == "" {
		ok = false
	}
	// §6.6は「sizeは正整数」と定める。0はtemplate sourceのunknownを表す値であり
	// （§7.1）、静的に書けるassetでは実size不明を許さない。
	if size, sizeOK := requirePositiveInt(table.Size, field+".size", diagnostics); sizeOK {
		value.Size = size
	} else {
		ok = false
	}

	algorithm, algorithmOK := requireDigestAlgorithm(
		table.DigestAlgorithm, field+".digest_algorithm", diagnostics)
	value.DigestAlgorithm = algorithm
	if !algorithmOK {
		ok = false
	}
	if digest, digestOK := requireDigest(
		table.Digest, field+".digest", algorithm, diagnostics); digestOK {
		value.Digest = digest
	} else {
		ok = false
	}

	if !buildAssetPlatform(table, field, &value, diagnostics) {
		ok = false
	}

	publishedAt, publishedOK := requireRFC3339(table.PublishedAt, field+".published_at", diagnostics)
	if !publishedOK {
		ok = false
	}
	value.PublishedAt = publishedAt

	value.ReleaseTag = requireText(table.ReleaseTag, field+".release_tag", 1, NameMaxBytes, diagnostics)
	if value.ReleaseTag == "" {
		ok = false
	}
	value.ReleaseURL = requireHTTPSURL(table.ReleaseURL, field+".release_url", urlEndpoint, diagnostics)
	if value.ReleaseURL == "" {
		ok = false
	}
	if id, idOK := requireDecimalID(table.ReleaseID, field+".release_id", diagnostics); idOK {
		value.ReleaseID = id
	} else {
		ok = false
	}
	if id, idOK := requireDecimalID(table.AssetID, field+".asset_id", diagnostics); idOK {
		value.AssetID = id
	} else {
		ok = false
	}
	return value, ok
}

// buildAssetPlatform はasset側のos/arch/libcを検査する。
//
// platform entryとの一致はここで見ない。§7.1のselectorが`os`/`arch`/`libc`で
// 絞り込む契約であり、asset listには他platform向けのentryが含まれてよいためである。
func buildAssetPlatform(
	table *staticAssetTable, field string, value *StaticAsset, diagnostics *Diagnostics,
) bool {
	ok := true
	if os, osOK := requireEnumText(table.OS, field+".os", parseOS, diagnostics); osOK {
		value.OS = os
	} else {
		ok = false
	}
	if arch, archOK := requireEnumText(table.Arch, field+".arch", parseArch, diagnostics); archOK {
		value.Arch = arch
	} else {
		ok = false
	}
	if libc, libcOK := requireEnumText(table.Libc, field+".libc", parseLibc, diagnostics); libcOK {
		value.Libc = libc
	} else {
		ok = false
	}
	return ok
}

func parseOS(text string) (domain.OS, error) {
	switch domain.OS(text) {
	case domain.OSWindows, domain.OSLinux:
		return domain.OS(text), nil
	default:
		return "", fmt.Errorf("osは%s|%sだけ（%q）", domain.OSWindows, domain.OSLinux, text)
	}
}

func parseArch(text string) (domain.Arch, error) {
	if domain.Arch(text) == domain.ArchAMD64 {
		return domain.ArchAMD64, nil
	}
	return "", fmt.Errorf("archは%sだけ（%q）", domain.ArchAMD64, text)
}

func parseLibc(text string) (domain.Libc, error) {
	switch domain.Libc(text) {
	case domain.LibcNone, domain.LibcGlibc:
		return domain.Libc(text), nil
	default:
		return "", fmt.Errorf("libcは%s|%sだけ（%q）", domain.LibcNone, domain.LibcGlibc, text)
	}
}

// requireEnumText は必須のenum textを読む。
func requireEnumText[T ~string](
	raw *string, field string, parse func(string) (T, error), diagnostics *Diagnostics,
) (T, bool) {
	var zero T
	if raw == nil {
		diagnostics.Add(field, reason(reasonMissing), fmt.Sprintf("`%s`が無い", field))
		return zero, false
	}
	value, err := parse(*raw)
	if err != nil {
		diagnostics.Add(field, reason(reasonEnum), err.Error())
		return zero, false
	}
	return value, true
}

func requireDigestAlgorithm(
	raw *string, field string, diagnostics *Diagnostics,
) (DigestAlgorithm, bool) {
	return requireEnumText(raw, field, func(text string) (DigestAlgorithm, error) {
		switch DigestAlgorithm(text) {
		case AlgorithmSHA256, AlgorithmSHA512:
			return DigestAlgorithm(text), nil
		default:
			return "", fmt.Errorf("digest_algorithmは%s|%sだけ（%q）",
				AlgorithmSHA256, AlgorithmSHA512, text)
		}
	}, diagnostics)
}

// requireDigest は§6.5のdigest表現を検査する。
//
// 「sourceの`digest`はalgorithm prefixなしのlowercase hexとして読む」。
// hex長がalgorithmと一致しない値を拒否するのは、長さが合わないdigestを
// 通すとdownload後の照合が必ず失敗し、原因がdefinition側だと分からなくなる
// ためである。algorithmが決まっていない場合は長さを判定できないため、
// 追加の診断を出さない。
func requireDigest(
	raw *string, field string, algorithm DigestAlgorithm, diagnostics *Diagnostics,
) (string, bool) {
	if raw == nil {
		diagnostics.Add(field, reason(reasonMissing), fmt.Sprintf("`%s`が無い", field))
		return "", false
	}
	text := *raw
	if !lowerHexRe.MatchString(text) {
		diagnostics.Add(field, reason(reasonDigest),
			fmt.Sprintf("%sはlowercase hexでなければならない", field))
		return "", false
	}
	if algorithm == "" {
		return "", false
	}
	if want := digestHexLength[algorithm]; len(text) != want {
		diagnostics.Add(field, reason(reasonDigest),
			fmt.Sprintf("%sは%sで%d文字でなければならない（%d文字）", field, algorithm, want, len(text)))
		return "", false
	}
	return text, true
}

// requireDecimalID は非負decimal stringを検査する（§6.5・§6.6）。
func requireDecimalID(raw *string, field string, diagnostics *Diagnostics) (string, bool) {
	if raw == nil {
		diagnostics.Add(field, reason(reasonMissing), fmt.Sprintf("`%s`が無い", field))
		return "", false
	}
	text := *raw
	if len(text) > NameMaxBytes || !decimalIDRe.MatchString(text) {
		diagnostics.Add(field, reason(reasonText),
			fmt.Sprintf("%sは非負decimal string（leading zeroなし）でなければならない（%q）", field, text))
		return "", false
	}
	return text, true
}

func requirePositiveInt(raw *int64, field string, diagnostics *Diagnostics) (int64, bool) {
	if raw == nil {
		diagnostics.Add(field, reason(reasonMissing), fmt.Sprintf("`%s`が無い", field))
		return 0, false
	}
	if *raw <= 0 {
		diagnostics.Add(field, reason(reasonLimit),
			fmt.Sprintf("%sは正整数でなければならない（%d）", field, *raw))
		return 0, false
	}
	return *raw, true
}

// requireRFC3339 はUTC RFC 3339のstring時刻を検査する（§6.1・§6.6）。
//
// go-tomlのoffset date-time型ではなくstringで書く値である。§6.1が
// 「UTC RFC 3339またはISO 8601 full-date（`YYYY-MM-DD`）のstring」を許し、
// full-dateは`T00:00:00Z`へ正規化する。
func requireRFC3339(raw *string, field string, diagnostics *Diagnostics) (time.Time, bool) {
	if raw == nil {
		diagnostics.Add(field, reason(reasonMissing), fmt.Sprintf("`%s`が無い", field))
		return time.Time{}, false
	}
	text := *raw
	if value, err := time.Parse(time.RFC3339, text); err == nil {
		if !strings.HasSuffix(text, "Z") {
			diagnostics.Add(field, reason(reasonTime),
				fmt.Sprintf("%sはUTC（末尾`Z`）でなければならない（%q）", field, text))
			return time.Time{}, false
		}
		if value.IsZero() {
			diagnostics.Add(field, reason(reasonTime), fmt.Sprintf("%sが未設定の時刻", field))
			return time.Time{}, false
		}
		return value.UTC(), true
	}
	// full-dateは`T00:00:00Z`へ正規化する（§6.1）。
	if value, err := time.Parse(time.DateOnly, text); err == nil {
		return value.UTC(), true
	}
	diagnostics.Add(field, reason(reasonTime),
		fmt.Sprintf("%sがUTC RFC 3339またはfull-dateでない（%q）", field, text))
	return time.Time{}, false
}

// requireFileName はpath区切りを含まないfile名を検査する。
//
// asset nameは§7.1のselectorが`name_regex`で照合し、templateの`{{asset.name}}`
// からdownload先のfile名にもなる。区切りを許すとcache directoryの外へ出る。
func requireFileName(raw *string, field string, diagnostics *Diagnostics) string {
	name := requireText(raw, field, 1, PathComponentMaxBytes, diagnostics)
	if name == "" {
		return ""
	}
	if strings.ContainsAny(name, `/\`) || name == "." || name == ".." {
		diagnostics.Add(field, reason(reasonText),
			fmt.Sprintf("%sは区切りを含まないfile名でなければならない（%q）", field, name))
		return ""
	}
	return name
}

// PathComponentMaxBytes は1 path componentの上限である
// （docs/04-storage-and-data.md §21）。
const PathComponentMaxBytes = 255

// ParseDigestAlgorithm は§6.5・§7.2のdigest algorithmを読む。
//
// schema 1は`sha256`と`sha512`だけを扱う。sourceの`digest_algorithm` fieldから
// 読んだ値もこの2値へ閉じる。上流がalgorithmを増やしたら、hex長の照合より前に
// ここで止める。
func ParseDigestAlgorithm(text string) (DigestAlgorithm, error) {
	switch DigestAlgorithm(text) {
	case AlgorithmSHA256, AlgorithmSHA512:
		return DigestAlgorithm(text), nil
	default:
		return "", fmt.Errorf("digest algorithmは%s|%sだけ（%q）",
			AlgorithmSHA256, AlgorithmSHA512, text)
	}
}

// DigestHexLength はalgorithmごとのhex長を返す。未知algorithmは0を返す。
//
// §6.5が「hex長がalgorithmと一致しない値を拒否する」と定める。長さの正本を
// packageごとに複製しないため、catalog側もこの値を使う。
func DigestHexLength(algorithm DigestAlgorithm) int { return digestHexLength[algorithm] }
