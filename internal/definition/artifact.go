package definition

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// ArtifactSource は§7.1のartifact取得方式である。
type ArtifactSource string

// ArtifactSource の値。
const (
	// SourceTemplate はrender後のURL/fileを使う。
	SourceTemplate ArtifactSource = "template"
	// SourceAsset はversion itemのassetsからselectorで1件選ぶ。
	SourceAsset ArtifactSource = "asset"
)

// ArchiveFormat は§7.1のarchive形式である。schema 1はこの2形式だけを扱う。
type ArchiveFormat string

// ArchiveFormat の値。
const (
	FormatZip   ArchiveFormat = "zip"
	FormatTarGz ArchiveFormat = "tar.gz"
)

// ChecksumKind は§7.2のchecksum取得方式である。
type ChecksumKind string

// ChecksumKind の値。
const (
	// ChecksumAssetField はsource assetのdigestを使う。
	ChecksumAssetField ChecksumKind = "asset-field"
	// ChecksumTextFile はURLのUTF-8 textから1行を得る。
	ChecksumTextFile ChecksumKind = "text-file"
)

// LineFormatSHA256 は§7.2がschema 1で許す唯一のline formatである。
const LineFormatSHA256 = "sha256-space-filename"

// ArtifactIDPrimary は§7.1の固定artifact IDである。primary 1件だけ。
const ArtifactIDPrimary = "primary"

// Artifact は§7の`[platforms.artifact]`である。
type Artifact struct {
	// Source は取得方式である。
	Source ArtifactSource
	// URL はtemplate sourceのURL templateである。asset sourceでは空。
	URL string
	// File はtemplate sourceのfile名templateである。asset sourceでは空。
	File string
	// Format はarchive形式である。
	Format ArchiveFormat
	// Size は0がunknown、正値がexpected sizeである。
	Size int64
	// Selector はasset sourceの選択条件である。
	Selector *ArtifactSelector
	// Checksum はdigestの取得方法である。
	Checksum ArtifactChecksum
	// RedirectHosts はartifact/checksum URLへ共通の追加許可hostである。
	RedirectHosts []string
}

// ArtifactSelector は§7.1のasset選択条件である。
//
// 指定条件すべてに一致するassetをexactly 1件要求する。
type ArtifactSelector struct {
	NameRegex string
	OS        string
	Arch      string
	Libc      string
}

// ArtifactChecksum は§7.2のchecksum契約である。
type ArtifactChecksum struct {
	Kind ChecksumKind
	// URL は`text-file`の取得先である。`asset-field`では空。
	URL string
	// LineFormat は`text-file`の行形式である。`asset-field`では空。
	LineFormat string
	// Algorithm は`asset-field`でsourceがalgorithm fieldを持たない場合に要る。
	Algorithm DigestAlgorithm
}

type artifactTable struct {
	ID            *string        `toml:"id"`
	Source        *string        `toml:"source"`
	URL           *string        `toml:"url"`
	File          *string        `toml:"file"`
	Format        *string        `toml:"format"`
	Size          *int64         `toml:"size"`
	Selector      *selectorTable `toml:"selector"`
	Checksum      *checksumTable `toml:"checksum"`
	RedirectHosts *[]string      `toml:"redirect_hosts"`
}

type selectorTable struct {
	NameRegex *string `toml:"name_regex"`
	OS        *string `toml:"os"`
	Arch      *string `toml:"arch"`
	Libc      *string `toml:"libc"`
}

type checksumTable struct {
	Kind       *string `toml:"kind"`
	URL        *string `toml:"url"`
	LineFormat *string `toml:"line_format"`
	Algorithm  *string `toml:"algorithm"`
}

// buildArtifact は§7の`artifact`を検証する（§13-6）。
func buildArtifact(
	table *artifactTable, field string, context templateContext,
	source VersionSource, diagnostics *Diagnostics,
) Artifact {
	var value Artifact
	if table == nil {
		diagnostics.Add(field, reason(reasonMissing), "`[platforms.artifact]`が無い")
		return value
	}
	// id=`primary`固定。1件だけであることはTOMLのtable構造が保証する。
	if id := requireText(table.ID, field+".id", 1, NameMaxBytes, diagnostics); id != "" &&
		id != ArtifactIDPrimary {
		diagnostics.Add(field+".id", reason(reasonEnum),
			fmt.Sprintf("artifact idは%qだけ（%q）", ArtifactIDPrimary, id))
	}
	value.Source = buildArtifactSource(table.Source, field, diagnostics)
	if format, ok := requireEnumText(
		table.Format, field+".format", parseArchiveFormat, diagnostics); ok {
		value.Format = format
	}
	if size, ok := requireNonNegativeInt(table.Size, field+".size", diagnostics); ok {
		value.Size = size
	}
	value.RedirectHosts = buildRedirectHosts(table.RedirectHosts, field+".redirect_hosts", diagnostics)

	buildArtifactBySource(table, field, context, value.Source, &value, diagnostics)
	value.Checksum = buildChecksum(table.Checksum, field+".checksum", value.Source, source, diagnostics)
	return value
}

// buildArtifactBySource は§7.1の`source`別契約を検査する。
//
// templateはURL/fileが必須でselectorを持てない。assetはselectorが必須で、
// URL/fileは空でも非空でもよい。**空なら選択assetの`url`/`name`を使い、非空なら
// 選択assetを`{{asset.<field>}}`で参照できるtemplateとしてrenderする**（§7.1）。
// upstreamがasset listにdownload URLを載せず、file名からURLを組み立てる配布元
// （Go）があるためで、artifactの同一性はどちらの場合もselectorが決める。
//
// source=templateへselectorを許さないのは、取得先が2通りに決まる定義を
// 受理しないためである。
func buildArtifactBySource(
	table *artifactTable, field string, context templateContext,
	source ArtifactSource, value *Artifact, diagnostics *Diagnostics,
) {
	switch source {
	case SourceTemplate:
		value.URL = buildArtifactURL(table.URL, field+".url", context, diagnostics)
		value.File = buildArtifactFile(table.File, field+".file", context, diagnostics)
		if table.Selector != nil {
			diagnostics.Add(field+".selector", reason(reasonConditional),
				"source=templateでは`selector`を書けない")
		}
	case SourceAsset:
		// 空は「選択assetの値を使う」を表すため、必須検査を通さずに素通しする。
		// 非空のときだけtemplateとして検査する。
		if table.URL != nil && *table.URL != "" {
			value.URL = buildArtifactURL(table.URL, field+".url", context, diagnostics)
		}
		if table.File != nil && *table.File != "" {
			value.File = buildArtifactFile(table.File, field+".file", context, diagnostics)
		}
		// 片方だけのtemplateはURLとfile名の出所が食い違う。§7.1はどちらも
		// 「空なら選択asset」と定めるため、組で宣言させる。
		if (value.URL == "") != (value.File == "") {
			diagnostics.Add(field+".url", reason(reasonConditional),
				"source=assetの`url`と`file`は両方空にするか両方templateにする")
		}
		value.Selector = buildSelector(table.Selector, field+".selector", diagnostics)
	}
}

func buildArtifactSource(raw *string, field string, diagnostics *Diagnostics) ArtifactSource {
	value, _ := requireEnumText(raw, field+".source", func(text string) (ArtifactSource, error) {
		switch ArtifactSource(text) {
		case SourceTemplate, SourceAsset:
			return ArtifactSource(text), nil
		default:
			return "", fmt.Errorf("sourceは%s|%sだけ（%q）", SourceTemplate, SourceAsset, text)
		}
	}, diagnostics)
	return value
}

func parseArchiveFormat(text string) (ArchiveFormat, error) {
	switch ArchiveFormat(text) {
	case FormatZip, FormatTarGz:
		return ArchiveFormat(text), nil
	default:
		return "", fmt.Errorf("formatは%s|%sだけ（%q）", FormatZip, FormatTarGz, text)
	}
}

// buildArtifactURL はURL templateを検査する（§7.1）。
//
// render前の値はtemplate変数を含むためURLとして解釈できない。変数を除いた
// 骨格がHTTPS URLであることと、変数が許可rootであることを分けて見る。
func buildArtifactURL(
	raw *string, field string, context templateContext, diagnostics *Diagnostics,
) string {
	if raw == nil {
		diagnostics.Add(field, reason(reasonMissing), fmt.Sprintf("`%s`が無い", field))
		return ""
	}
	if err := context.checkSubstitution(field, *raw, artifactScope); err != nil {
		diagnostics.Add(field, reason(reasonTemplate), err.Error())
		return ""
	}
	if err := checkTemplateURLSkeleton(*raw, field); err != nil {
		diagnostics.Add(field, reason(reasonURL), err.Error())
		return ""
	}
	return *raw
}

// checkTemplateURLSkeleton はtemplate変数を除いた骨格を検査する。
//
// 変数を固定の代替文字列へ置き換えてからURLとして読む。scheme、host、
// userinfoは変数の外にあり、置換後でも判定できる。
func checkTemplateURLSkeleton(value, field string) error {
	skeleton := templateRe.ReplaceAllString(value, "x")
	if err := checkHTTPSURL(skeleton, field, urlEndpoint); err != nil {
		return err
	}
	parsed, err := url.Parse(skeleton)
	if err != nil {
		return fmt.Errorf("%sがURLとして解釈できない", field)
	}
	// hostへ変数を埋めることを禁じる。上流の値でhostが変わると、§7.1の
	// redirect許可hostの比較が定義時に決まらない。
	if strings.Contains(value, "{{") {
		hostEnd := strings.Index(value, parsed.Host)
		if hostEnd < 0 || strings.Contains(value[:hostEnd+len(parsed.Host)], "{{") {
			return fmt.Errorf("%sのhost部分にtemplate変数を書けない", field)
		}
	}
	return nil
}

// buildArtifactFile はfile名templateを検査する（§7.1）。
//
// 「fileはbasename grammarを検査する」。区切りを許すとdownload先がcache
// directoryの外へ出る。
func buildArtifactFile(
	raw *string, field string, context templateContext, diagnostics *Diagnostics,
) string {
	if raw == nil {
		diagnostics.Add(field, reason(reasonMissing), fmt.Sprintf("`%s`が無い", field))
		return ""
	}
	if err := context.checkSubstitution(field, *raw, artifactScope); err != nil {
		diagnostics.Add(field, reason(reasonTemplate), err.Error())
		return ""
	}
	value := *raw
	switch {
	case value == "":
		diagnostics.Add(field, reason(reasonText), fmt.Sprintf("%sが空", field))
		return ""
	case strings.ContainsAny(value, `/\`):
		diagnostics.Add(field, reason(reasonText),
			fmt.Sprintf("%sは区切りを含まないfile名でなければならない（%q）", field, value))
		return ""
	case strings.HasPrefix(value, "."):
		// render後に`.`や`..`になる形を先に落とす。
		diagnostics.Add(field, reason(reasonText), fmt.Sprintf("%sがdotで始まる（%q）", field, value))
		return ""
	}
	return value
}

// buildSelector は§7.1のselectorを検査する。
func buildSelector(table *selectorTable, field string, diagnostics *Diagnostics) *ArtifactSelector {
	if table == nil {
		diagnostics.Add(field, reason(reasonMissing), "source=assetでは`[platforms.artifact.selector]`が必須")
		return nil
	}
	value := &ArtifactSelector{}
	if table.NameRegex != nil {
		// selectorの`name_regex`はexactly 1件のassetを選ぶ条件である。§7.1は
		// named captureを要求しないため、RE2として妥当であることだけを見る。
		if err := checkRegexSyntax(*table.NameRegex, field+".name_regex"); err != nil {
			diagnostics.Add(field+".name_regex", reason(reasonRegex), err.Error())
		} else {
			value.NameRegex = *table.NameRegex
		}
	}
	if table.OS != nil {
		if os, err := parseOS(*table.OS); err != nil {
			diagnostics.Add(field+".os", reason(reasonEnum), err.Error())
		} else {
			value.OS = string(os)
		}
	}
	if table.Arch != nil {
		if arch, err := parseArch(*table.Arch); err != nil {
			diagnostics.Add(field+".arch", reason(reasonEnum), err.Error())
		} else {
			value.Arch = string(arch)
		}
	}
	if table.Libc != nil {
		if libc, err := parseLibc(*table.Libc); err != nil {
			diagnostics.Add(field+".libc", reason(reasonEnum), err.Error())
		} else {
			value.Libc = string(libc)
		}
	}
	// 条件が1件も無いselectorは全assetに一致し、exactly 1件を選べない。
	if table.NameRegex == nil && table.OS == nil && table.Arch == nil && table.Libc == nil {
		diagnostics.Add(field, reason(reasonConditional), "selectorに条件が1件も無い")
		return nil
	}
	return value
}

// buildChecksum は§7.2のchecksum契約を検査する。
//
// v0.1はchecksumを公開しないartifactを扱わないため、checksum tableは必須である
// （§7.2、[15-deferred.md](../../docs/15-deferred.md) D-06）。
func buildChecksum(
	table *checksumTable, field string, artifactSource ArtifactSource,
	source VersionSource, diagnostics *Diagnostics,
) ArtifactChecksum {
	var value ArtifactChecksum
	if table == nil {
		diagnostics.Add(field, reason(reasonMissing), "`[platforms.artifact.checksum]`が必須")
		return value
	}
	kind, ok := requireEnumText(table.Kind, field+".kind", func(text string) (ChecksumKind, error) {
		switch ChecksumKind(text) {
		case ChecksumAssetField, ChecksumTextFile:
			return ChecksumKind(text), nil
		default:
			return "", fmt.Errorf("checksum kindは%s|%sだけ（%q）",
				ChecksumAssetField, ChecksumTextFile, text)
		}
	}, diagnostics)
	if !ok {
		return value
	}
	value.Kind = kind

	switch kind {
	case ChecksumAssetField:
		buildAssetFieldChecksum(table, field, artifactSource, source, &value, diagnostics)
	case ChecksumTextFile:
		buildTextFileChecksum(table, field, &value, diagnostics)
	}
	return value
}

// buildAssetFieldChecksum は`asset-field`の契約を検査する（§7.2）。
//
// 「sourceにalgorithm fieldがあればその値と`algorithm`が完全一致。なければ
// definitionの`algorithm`必須」。sourceのalgorithm fieldの有無は
// `asset_fields`の`digest_algorithm`宣言と、static sourceのasset自身で決まる。
func buildAssetFieldChecksum(
	table *checksumTable, field string, artifactSource ArtifactSource,
	source VersionSource, value *ArtifactChecksum, diagnostics *Diagnostics,
) {
	if table.URL != nil {
		diagnostics.Add(field+".url", reason(reasonConditional), "kind=asset-fieldでは`url`を書けない")
	}
	if table.LineFormat != nil {
		diagnostics.Add(field+".line_format", reason(reasonConditional),
			"kind=asset-fieldでは`line_format`を書けない")
	}
	// assetのdigestを使うため、artifact自体もassetから選ぶ必要がある。
	if artifactSource == SourceTemplate {
		diagnostics.Add(field+".kind", reason(reasonConditional),
			"source=templateではkind=asset-fieldを使えない")
	}
	if table.Algorithm != nil {
		algorithm, ok := requireDigestAlgorithm(table.Algorithm, field+".algorithm", diagnostics)
		if !ok {
			return
		}
		if sourceHasDigestAlgorithm(source) {
			// sourceがalgorithm fieldを持つ場合、definitionの値は照合用である。
			// staticはasset自身が`digest_algorithm`を持つため、片方だけを見て
			// 食い違いを見逃さないよう両方を保持する。
			value.Algorithm = algorithm
			return
		}
		value.Algorithm = algorithm
		return
	}
	if !sourceHasDigestAlgorithm(source) {
		diagnostics.Add(field+".algorithm", reason(reasonConditional),
			"sourceが`digest_algorithm`を持たないため`algorithm`が必須")
	}
}

// sourceHasDigestAlgorithm はsourceがalgorithm fieldを持つかを返す。
func sourceHasDigestAlgorithm(source VersionSource) bool {
	if source.Kind == SourceStatic {
		// static assetは`digest_algorithm`を全件必須で持つ（§6.6）。
		return len(source.StaticVersions) > 0
	}
	_, declared := source.AssetFields[AssetDigestAlgorithm]
	return declared
}

// buildTextFileChecksum は`text-file`の契約を検査する（§7.2）。
func buildTextFileChecksum(
	table *checksumTable, field string, value *ArtifactChecksum, diagnostics *Diagnostics,
) {
	// checksum URLもtemplateを取る。Node.jsの`SHASUMS256.txt`が
	// `dist/v{{version}}/`配下にある。
	if table.URL == nil {
		diagnostics.Add(field+".url", reason(reasonMissing), "kind=text-fileでは`url`が必須")
	} else {
		context := templateContext{}
		if err := context.checkSubstitution(field+".url", *table.URL, artifactScope); err != nil {
			diagnostics.Add(field+".url", reason(reasonTemplate), err.Error())
		} else if err := checkTemplateURLSkeleton(*table.URL, field+".url"); err != nil {
			diagnostics.Add(field+".url", reason(reasonURL), err.Error())
		} else {
			value.URL = *table.URL
		}
	}
	if table.LineFormat == nil {
		diagnostics.Add(field+".line_format", reason(reasonMissing),
			"kind=text-fileでは`line_format`が必須")
	} else if *table.LineFormat != LineFormatSHA256 {
		diagnostics.Add(field+".line_format", reason(reasonEnum),
			fmt.Sprintf("line_formatは%qだけ（%q）", LineFormatSHA256, *table.LineFormat))
	} else {
		value.LineFormat = *table.LineFormat
	}
	// 「`text-file`は`line_format`がalgorithmを含むため`algorithm`を書かない」。
	if table.Algorithm != nil {
		diagnostics.Add(field+".algorithm", reason(reasonConditional),
			"kind=text-fileでは`algorithm`を書けない")
	}
}

// buildRedirectHosts は§7.1の`redirect_hosts`を検査する。
//
// 「各値はASCII lowercase完全hostでwildcard不可」。wildcardを許すと、
// redirect先を最終URLから動的にallowlist化するのと変わらなくなる。
func buildRedirectHosts(raw *[]string, field string, diagnostics *Diagnostics) []string {
	if raw == nil {
		return nil
	}
	hosts := *raw
	if len(hosts) > ArrayMax {
		diagnostics.Add(field, reason(reasonLimit),
			fmt.Sprintf("redirect_hostsが%d件を超える（%d件）", ArrayMax, len(hosts)))
		return nil
	}
	for index, host := range hosts {
		if err := checkHostname(host); err != nil {
			diagnostics.Add(fmt.Sprintf("%s[%d]", field, index), reason(reasonText), err.Error())
			return nil
		}
	}
	if err := requireUniqueIdentifiers("redirect_hosts", hosts); err != nil {
		diagnostics.Add(field, reason(reasonDuplicate), err.Error())
		return nil
	}
	return append([]string{}, hosts...)
}

// hostnameRe はASCII lowercaseの完全hostである。wildcardとportを許さない。
var hostnameRe = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]*[a-z0-9])?)*$`)

func checkHostname(host string) error {
	switch {
	case host == "":
		return fmt.Errorf("hostが空")
	case len(host) > 253:
		return fmt.Errorf("host %q が253 byteを超える", host)
	case strings.Contains(host, "*"):
		return fmt.Errorf("host %q にwildcardを使えない", host)
	case !hostnameRe.MatchString(host):
		return fmt.Errorf("host %q がASCII lowercaseの完全hostでない", host)
	}
	return nil
}

func requireNonNegativeInt(raw *int64, field string, diagnostics *Diagnostics) (int64, bool) {
	if raw == nil {
		diagnostics.Add(field, reason(reasonMissing), fmt.Sprintf("`%s`が無い", field))
		return 0, false
	}
	if *raw < 0 {
		diagnostics.Add(field, reason(reasonLimit),
			fmt.Sprintf("%sは非負でなければならない（%d）", field, *raw))
		return 0, false
	}
	return *raw, true
}

// checkRegexSyntax はRE2としての妥当性と長さだけを検査する。
func checkRegexSyntax(pattern, field string) error {
	if pattern == "" {
		return fmt.Errorf("%sが空", field)
	}
	if len(pattern) > RegexMaxBytes {
		return fmt.Errorf("%sが%d byteを超える（%d byte）", field, RegexMaxBytes, len(pattern))
	}
	if _, err := regexp.Compile(pattern); err != nil {
		return fmt.Errorf("%sがRE2として不正（%v）", field, err)
	}
	return nil
}
