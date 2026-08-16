package catalog

import (
	"context"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/definition"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain/port"
)

// ChecksumTextMaxBytes はchecksum textの上限である。
//
// docs/04-storage-and-data.md §21「checksum text 2 MiB」。
const ChecksumTextMaxBytes = 2 << 20

// resolveAssetFieldDigest は§7.2の`asset-field`でdigestを決める。
//
// 「sourceにalgorithm fieldがあればその値と`algorithm`が完全一致。なければ
// definitionの`algorithm`必須」。解決したdigestは`<algorithm>:<hex>`へ正規化し、
// hex長がalgorithmと一致しない値を拒否する（§6.5）。
func resolveAssetFieldDigest(
	checksum definition.ArtifactChecksum, asset *Asset,
) (domain.Digest, error) {
	if asset == nil {
		return domain.Digest{}, fmt.Errorf("checksum kind=asset-fieldだがassetを選んでいない")
	}
	algorithm := checksum.Algorithm
	if asset.DigestAlgorithm != "" {
		// sourceがalgorithmを持つ場合、definitionの宣言と食い違えばsource error
		// にする。片方だけを信じると、hex長は合うが別algorithmのdigestを通しうる。
		if algorithm != "" && algorithm != asset.DigestAlgorithm {
			return domain.Digest{}, fmt.Errorf(
				"sourceのdigest_algorithm %q がdefinitionの %q と一致しない",
				asset.DigestAlgorithm, algorithm)
		}
		algorithm = asset.DigestAlgorithm
	}
	if algorithm == "" {
		return domain.Digest{}, fmt.Errorf("digest algorithmを決められない")
	}
	return normalizeDigest(algorithm, asset.Digest)
}

// normalizeDigest はalgorithmとlowercase hexを`<algorithm>:<hex>`へ正規化する。
func normalizeDigest(algorithm definition.DigestAlgorithm, hex string) (domain.Digest, error) {
	if hex == "" {
		return domain.Digest{}, fmt.Errorf("digestが空")
	}
	want := definition.DigestHexLength(algorithm)
	if want == 0 {
		return domain.Digest{}, fmt.Errorf("未知のdigest algorithm %q", algorithm)
	}
	if len(hex) != want {
		return domain.Digest{}, fmt.Errorf(
			"digestのhex長が%sと一致しない（%d文字、期待%d）", algorithm, len(hex), want)
	}
	digest, err := domain.ParseUpstreamDigest(string(algorithm) + ":" + hex)
	if err != nil {
		return domain.Digest{}, err
	}
	return digest, nil
}

// FetchChecksumText はchecksum text fileを取得する（§7.2）。
//
// file最大2 MiB。retryはHTTPClient adapterの責務である（P5-01）。
func FetchChecksumText(
	ctx context.Context, client port.HTTPClient, checksumURL string,
) (string, *domain.Error) {
	if client == nil {
		return "", domain.Internal(fmt.Errorf("catalog: HTTPClientが未注入"))
	}
	response, err := client.Get(ctx, port.HTTPRequest{
		URL:          checksumURL,
		MaxRedirects: RedirectMax,
		MaxBodyBytes: ChecksumTextMaxBytes,
	})
	if err != nil {
		return "", fetchError(checksumURL, err)
	}
	defer func() {
		if response.Body != nil {
			_ = response.Body.Close()
		}
	}()
	if response.StatusCode != 200 {
		return "", fetchError(checksumURL, fmt.Errorf("status %d", response.StatusCode))
	}
	if !strings.HasPrefix(response.FinalURL, "https://") {
		return "", fetchError(checksumURL,
			fmt.Errorf("redirect後のURLがHTTPSでない（%q）", response.FinalURL))
	}
	if response.Body == nil {
		return "", fetchError(checksumURL, fmt.Errorf("応答bodyが無い"))
	}
	body, readErr := io.ReadAll(io.LimitReader(response.Body, ChecksumTextMaxBytes+1))
	if readErr != nil {
		return "", fetchError(checksumURL, readErr)
	}
	if len(body) > ChecksumTextMaxBytes {
		return "", fetchError(checksumURL,
			fmt.Errorf("checksum textが%d byteを超える", ChecksumTextMaxBytes))
	}
	return string(body), nil
}

// ParseChecksumText は`sha256-space-filename`形式からdigestを取り出す（§7.2）。
//
// `<64 hex><1個以上ASCII space><optional '*'><basename>`を受け、BOM、NUL、path、
// duplicate、別algorithmを拒否する。**対象basenameのexact 1行だけを採る。**
// 複数行が同じbasenameを名乗る場合、どちらが正しいかを決められない。
func ParseChecksumText(text, basename string) (domain.Digest, error) {
	// UTF-8 BOM（U+FEFF）。source fileへBOM自体を書くとGoが読めないため定数で持つ。
	const utf8BOM = "\uFEFF"
	if strings.HasPrefix(text, utf8BOM) {
		return domain.Digest{}, fmt.Errorf("checksum textがBOMで始まる")
	}
	if strings.ContainsRune(text, 0) {
		return domain.Digest{}, fmt.Errorf("checksum textがNULを含む")
	}
	if !utf8.ValidString(text) {
		return domain.Digest{}, fmt.Errorf("checksum textが正しいUTF-8でない")
	}
	if basename == "" {
		return domain.Digest{}, fmt.Errorf("照合するbasenameが空")
	}

	var found string
	for number, line := range strings.Split(text, "\n") {
		line = strings.TrimSuffix(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		hex, name, err := parseChecksumLine(line)
		if err != nil {
			return domain.Digest{}, fmt.Errorf("checksum text %d行目: %w", number+1, err)
		}
		if name != basename {
			continue
		}
		if found != "" {
			return domain.Digest{}, fmt.Errorf("checksum textに %q が複数行ある", basename)
		}
		found = hex
	}
	if found == "" {
		return domain.Digest{}, fmt.Errorf("checksum textに %q の行が無い", basename)
	}
	// `line_format`がalgorithmを含むため、`sha256`で確定する（§7.2）。
	return normalizeDigest(definition.AlgorithmSHA256, found)
}

// parseChecksumLine は1行を`<hex><space+>[*]<basename>`として読む。
func parseChecksumLine(line string) (string, string, error) {
	space := strings.IndexAny(line, " \t")
	if space < 0 {
		return "", "", fmt.Errorf("hexとfile名の区切りが無い")
	}
	if strings.ContainsRune(line[:space], '\t') {
		return "", "", fmt.Errorf("hex部にtabがある")
	}
	hex := line[:space]
	// `line_format = sha256-space-filename`はSHA-256だけを許す。長さが違う行は
	// 別algorithmのfileであり、黙って読み飛ばさない（§7.2）。
	if len(hex) != definition.DigestHexLength(definition.AlgorithmSHA256) {
		return "", "", fmt.Errorf("hexが64文字でない（%d文字）", len(hex))
	}
	if !isLowerHex(hex) {
		return "", "", fmt.Errorf("hexがlowercase hexでない")
	}
	rest := line[space:]
	trimmed := strings.TrimLeft(rest, " ")
	if trimmed == rest {
		// 区切りがtabだけの行を弾く。line formatはASCII spaceを要求する。
		return "", "", fmt.Errorf("区切りがASCII spaceでない")
	}
	// binary modeの`*`は1個だけ許す。
	trimmed = strings.TrimPrefix(trimmed, "*")
	if trimmed == "" {
		return "", "", fmt.Errorf("file名が空")
	}
	if strings.ContainsAny(trimmed, `/\`) {
		return "", "", fmt.Errorf("file名がpathを含む（%q）", trimmed)
	}
	if strings.ContainsAny(trimmed, " \t") {
		return "", "", fmt.Errorf("file名が空白を含む（%q）", trimmed)
	}
	return hex, trimmed, nil
}

func isLowerHex(text string) bool {
	for index := 0; index < len(text); index++ {
		char := text[index]
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}
	return len(text) > 0
}
