package catalog

import (
	"fmt"
	"net/url"
	"path"
	"regexp"
	"strings"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/definition"
)

// resolvedArtifact は1 version itemのartifact解決結果である（§7.1）。
type resolvedArtifact struct {
	file string
	url  string
	size int64
	// asset は`source=asset`で選ばれたassetである。templateではnilになる。
	asset *Asset
}

// selectAsset は§7.1のselectorでassetをexactly 1件選ぶ。
//
// 「指定条件すべてに一致するassetをexactly 1件要求する。0件はそのversionを
// `installable=false/artifact-not-found`、2件以上はdefinition/source error。
// **source順で選ばない。**」先頭一致で選ぶと、上流が同じ条件のassetを増やした
// ことに気付かないまま別の配布物をinstallしうる。
//
// 0件はnilとnilを返し、呼出し側がunavailableへ倒す。
func selectAsset(selector *definition.ArtifactSelector, assets []Asset) (*Asset, error) {
	if selector == nil {
		return nil, fmt.Errorf("source=assetにselectorが無い")
	}
	nameRe, err := compileSelectorRegex(selector.NameRegex)
	if err != nil {
		return nil, err
	}
	var matched []int
	for index := range assets {
		if matchesSelector(&assets[index], selector, nameRe) {
			matched = append(matched, index)
		}
	}
	switch len(matched) {
	case 0:
		return nil, nil
	case 1:
		return &assets[matched[0]], nil
	default:
		names := make([]string, 0, len(matched))
		for _, index := range matched {
			names = append(names, assets[index].Name)
		}
		return nil, fmt.Errorf("selectorに%d件一致した（%s）", len(matched), strings.Join(names, ", "))
	}
}

func compileSelectorRegex(pattern string) (*regexp.Regexp, error) {
	if pattern == "" {
		return nil, nil
	}
	compiled, err := regexp.Compile(pattern)
	if err != nil {
		// definitionのschema検証を通ったRE2だけが渡る（§7.1）。
		return nil, fmt.Errorf("name_regexをcompileできない: %w", err)
	}
	return compiled, nil
}

// matchesSelector は指定した条件すべてに一致するかを返す。
//
// 未指定の条件は絞り込みに使わない。`os`/`arch`/`libc`は完全一致で比較し、
// sourceのassetが持つ他のfieldでは絞り込まない（§7.1）。
func matchesSelector(asset *Asset, selector *definition.ArtifactSelector, nameRe *regexp.Regexp) bool {
	if nameRe != nil && !nameRe.MatchString(asset.Name) {
		return false
	}
	if selector.OS != "" && asset.OS != selector.OS {
		return false
	}
	if selector.Arch != "" && asset.Arch != selector.Arch {
		return false
	}
	if selector.Libc != "" && asset.Libc != selector.Libc {
		return false
	}
	return true
}

// templateValues はURL/file templateへ渡す値である（§12）。
type templateValues struct {
	version  string
	metadata map[string]string
	asset    *Asset
}

// renderTemplate は§7.1のURL/file templateをrenderする。
//
// 使えるrootは`{{version}}`と宣言済みの`{{metadata.<key>}}`, `{{asset.<field>}}`
// だけである（definitionのschema検証が宣言との突き合わせを済ませている）。
// **template値が欠落したitemをinstallable扱いしない**（§7.1）ため、値が空の
// 変数はerrorにする。
//
// `escape`が真ならURL componentとしてpercent encodeする。file名は`checkFileName`
// がbasename grammarを検査する。
func renderTemplate(text string, values templateValues, escape bool) (string, error) {
	var failure error
	rendered := definition.ReplaceTemplateVars(text, func(token string) string {
		value, err := templateValue(token, values)
		if err != nil {
			if failure == nil {
				failure = err
			}
			return ""
		}
		if escape {
			// path componentとして安全にする。`/`・`?`・`#`が値に入ると、
			// render後のURLが別のpathやqueryを指しうる。
			return url.PathEscape(value)
		}
		return value
	})
	if failure != nil {
		return "", failure
	}
	if len(rendered) > definition.RenderedMaxBytes {
		return "", fmt.Errorf("render結果が%d byteを超える", definition.RenderedMaxBytes)
	}
	return rendered, nil
}

// templateValue は1つのtemplate変数の値を返す。
func templateValue(token string, values templateValues) (string, error) {
	inner := strings.TrimSuffix(strings.TrimPrefix(token, "{{"), "}}")
	switch {
	case inner == "version":
		return requireTemplateValue(token, values.version)
	case strings.HasPrefix(inner, "metadata."):
		key := strings.TrimPrefix(inner, "metadata.")
		value, found := values.metadata[key]
		if !found {
			return "", fmt.Errorf("%sの値がitemに無い", token)
		}
		return requireTemplateValue(token, value)
	case strings.HasPrefix(inner, "asset."):
		if values.asset == nil {
			return "", fmt.Errorf("%sを使うがassetを選んでいない", token)
		}
		value, err := assetFieldValue(values.asset, strings.TrimPrefix(inner, "asset."))
		if err != nil {
			return "", err
		}
		return requireTemplateValue(token, value)
	default:
		// definitionのschema検証が許可rootを閉じている（§12）。
		return "", fmt.Errorf("%sはURL/file templateで使えない", token)
	}
}

func requireTemplateValue(token, value string) (string, error) {
	if value == "" {
		return "", fmt.Errorf("%sの値が空", token)
	}
	return value, nil
}

// assetFieldValue はasset fieldの値を文字列で返す。
func assetFieldValue(asset *Asset, field string) (string, error) {
	switch definition.AssetField(field) {
	case definition.AssetName:
		return asset.Name, nil
	case definition.AssetURL:
		return asset.URL, nil
	case definition.AssetDigest:
		return asset.Digest, nil
	case definition.AssetDigestAlgorithm:
		return string(asset.DigestAlgorithm), nil
	case definition.AssetOS:
		return asset.OS, nil
	case definition.AssetArch:
		return asset.Arch, nil
	case definition.AssetLibc:
		return asset.Libc, nil
	case definition.AssetPublishedAt:
		return asset.PublishedAt, nil
	case definition.AssetReleaseTag:
		return asset.ReleaseTag, nil
	case definition.AssetReleaseURL:
		return asset.ReleaseURL, nil
	case definition.AssetReleaseID:
		return asset.ReleaseID, nil
	case definition.AssetID:
		return asset.AssetID, nil
	default:
		// sizeはtemplateへ使わない。§6.5で唯一integerのfieldであり、
		// 文字列化の表記をここで決めるとcatalogとreceiptで揺れる。
		return "", fmt.Errorf("asset field %q はtemplateで使えない", field)
	}
}

// checkArtifactURL はrender後のartifact URLを検査する。
func checkArtifactURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("artifact URLとして読めない（%q）", raw)
	}
	switch {
	case parsed.Scheme != "https":
		return fmt.Errorf("artifact URLがHTTPSでない（%q）", raw)
	case parsed.User != nil:
		return fmt.Errorf("artifact URLがcredentialを含む")
	case parsed.Host == "":
		return fmt.Errorf("artifact URLにhostが無い（%q）", raw)
	}
	return nil
}

// checkFileName はrender後のfile名がbasenameであることを検査する（§7.1）。
//
// path区切り、`.`と`..`、絶対path、NULを拒否する。artifact fileはdownload先の
// basenameになるため、区切りを含む値を通すと`payload`の外へ書ける。
func checkFileName(name string) error {
	switch {
	case name == "":
		return fmt.Errorf("artifact file名が空")
	case name == "." || name == "..":
		return fmt.Errorf("artifact file名が%qである", name)
	case strings.ContainsAny(name, `/\`):
		return fmt.Errorf("artifact file名がpath区切りを含む（%q）", name)
	case strings.ContainsRune(name, 0):
		return fmt.Errorf("artifact file名がNULを含む")
	case len(name) > definition.PathComponentMaxBytes:
		return fmt.Errorf("artifact file名が%d byteを超える", definition.PathComponentMaxBytes)
	}
	return nil
}

// resolveArtifact は1 version itemのartifactを決める（§7.1）。
//
// 選べない場合は(nil, nil)を返し、呼出し側が`installable=false`にする。
func resolveArtifact(
	artifact definition.Artifact, values templateValues, assets []Asset,
) (*resolvedArtifact, error) {
	if artifact.Source == definition.SourceAsset {
		asset, err := selectAsset(artifact.Selector, assets)
		if err != nil {
			return nil, err
		}
		if asset == nil {
			return nil, nil
		}
		artifactURL, file, err := assetURLAndFile(artifact, values, asset)
		if err != nil {
			return nil, err
		}
		if urlErr := checkArtifactURL(artifactURL); urlErr != nil {
			return nil, urlErr
		}
		if nameErr := checkFileName(file); nameErr != nil {
			return nil, nameErr
		}
		size := asset.Size
		if artifact.Size > 0 {
			// §7.1は`size`をexpected sizeとする。definitionが宣言していれば
			// そちらを使い、download応答との一致はP7が検査する。
			size = artifact.Size
		}
		return &resolvedArtifact{file: file, url: artifactURL, size: size, asset: asset}, nil
	}

	// source=templateはrender後のURL/fileを使う。
	values.asset = nil
	artifactURL, err := renderTemplate(artifact.URL, values, true)
	if err != nil {
		return nil, fmt.Errorf("artifact.url: %w", err)
	}
	if urlErr := checkArtifactURL(artifactURL); urlErr != nil {
		return nil, urlErr
	}
	file, err := renderTemplate(artifact.File, values, false)
	if err != nil {
		return nil, fmt.Errorf("artifact.file: %w", err)
	}
	if nameErr := checkFileName(file); nameErr != nil {
		return nil, nameErr
	}
	return &resolvedArtifact{file: file, url: artifactURL, size: artifact.Size}, nil
}

// assetURLAndFile は`source=asset`のURLとfile名を決める（§7.1）。
//
// 「`url`/`file`は空なら選択assetの`url`/`name`を使い、非空なら選択assetを
// `{{asset.<field>}}`で参照できるtemplateとしてrenderする。」upstreamがasset
// listにdownload URLを載せず、file名からURLを組み立てる配布元（Go）に使う。
func assetURLAndFile(
	artifact definition.Artifact, values templateValues, asset *Asset,
) (string, string, error) {
	values.asset = asset
	if artifact.URL == "" {
		if asset.URL == "" || asset.Name == "" {
			return "", "", fmt.Errorf("選択assetの`url`または`name`が空")
		}
		return asset.URL, asset.Name, nil
	}
	// definitionのschema検証が`url`と`file`を組で要求する。片方だけのtemplateは
	// URLとfile名の出所が食い違う。
	artifactURL, err := renderTemplate(artifact.URL, values, true)
	if err != nil {
		return "", "", fmt.Errorf("artifact.url: %w", err)
	}
	file, err := renderTemplate(artifact.File, values, false)
	if err != nil {
		return "", "", fmt.Errorf("artifact.file: %w", err)
	}
	return artifactURL, file, nil
}

// artifactBasename はURLのbasenameを返す。checksum text-fileの照合に使う。
func artifactBasename(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return path.Base(parsed.Path)
}
