package catalog

import (
	"context"
	"strings"
	"testing"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/definition"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain/port/fake"
)

const checksumURL = "https://nodejs.org/dist/v22.18.0/SHASUMS256.txt"

// TestParseChecksumTextAcceptsLineFormat は§7.2の`sha256-space-filename`を固定する。
//
// 「`<64 hex><1個以上ASCII space><optional '*'><basename>`を受け」る。
func TestParseChecksumTextAcceptsLineFormat(t *testing.T) {
	cases := []struct{ name, text string }{
		{"space 2個", digest64 + "  node.zip\n"},
		{"space 1個", digest64 + " node.zip\n"},
		{"binary mode", digest64 + "  *node.zip\n"},
		{"CRLF", digest64 + "  node.zip\r\n"},
		{"末尾改行なし", digest64 + "  node.zip"},
		{"空行を挟む", "\n" + digest64 + "  node.zip\n\n"},
		{"他fileの行が並ぶ", otherDigest64 + "  other.zip\n" + digest64 + "  node.zip\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			digest, err := ParseChecksumText(c.text, "node.zip")
			if err != nil {
				t.Fatalf("ParseChecksumText = %v", err)
			}
			if digest.Upstream() != "sha256:"+digest64 {
				t.Fatalf("digest = %q", digest.Upstream())
			}
		})
	}
}

// TestParseChecksumTextRejectsBadInput は§7.2の拒否条件を固定する。
//
// 「BOM、NUL、path、duplicate、別algorithmを拒否する。」
func TestParseChecksumTextRejectsBadInput(t *testing.T) {
	cases := []struct{ name, text, basename string }{
		{"BOM", "\uFEFF" + digest64 + "  node.zip\n", "node.zip"},
		{"NUL", digest64 + "  node.zip\x00\n", "node.zip"},
		{"duplicate", digest64 + "  node.zip\n" + otherDigest64 + "  node.zip\n", "node.zip"},
		// SHA-512のhexは128文字である。別algorithmのfileを黙って読み飛ばさない。
		{"別algorithm", strings.Repeat("a", 128) + "  node.zip\n", "node.zip"},
		{"hexが短い", strings.Repeat("a", 63) + "  node.zip\n", "node.zip"},
		{"hexが大文字", strings.ToUpper(digest64) + "  node.zip\n", "node.zip"},
		{"hexが非hex", strings.Repeat("g", 64) + "  node.zip\n", "node.zip"},
		{"file名にpath", digest64 + "  dist/node.zip\n", "node.zip"},
		{"file名にbackslash", digest64 + `  dist\node.zip` + "\n", "node.zip"},
		{"区切りがtab", digest64 + "\tnode.zip\n", "node.zip"},
		{"区切りが無い", digest64 + "\n", "node.zip"},
		{"対象の行が無い", otherDigest64 + "  other.zip\n", "node.zip"},
		{"basenameが空", digest64 + "  node.zip\n", ""},
		{"file名が空", digest64 + "  \n", "node.zip"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := ParseChecksumText(c.text, c.basename); err == nil {
				t.Fatal("ParseChecksumTextが成功した")
			}
		})
	}
}

// TestFetchChecksumText はchecksum textの取得を固定する。
func TestFetchChecksumText(t *testing.T) {
	t.Run("成功", func(t *testing.T) {
		client := fake.NewHTTPClient(nil)
		client.Stub(checksumURL, fake.HTTPStub{
			StatusCode: 200, Body: []byte(digest64 + "  node.zip\n"),
		})
		text, err := FetchChecksumText(context.Background(), client, checksumURL)
		if err != nil {
			t.Fatalf("FetchChecksumText = %s", describeErr(err))
		}
		if !strings.Contains(text, digest64) {
			t.Fatalf("text = %q", text)
		}
	})

	t.Run("404はnetwork error", func(t *testing.T) {
		client := fake.NewHTTPClient(nil)
		client.Stub(checksumURL, fake.HTTPStub{StatusCode: 404})
		_, err := FetchChecksumText(context.Background(), client, checksumURL)
		if err == nil || err.Code != domain.CodeNetwork {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("client未注入", func(t *testing.T) {
		if _, err := FetchChecksumText(context.Background(), nil, checksumURL); err == nil {
			t.Fatal("HTTPClientなしで成功した")
		}
	})
}

// TestResolveAssetFieldDigest は§7.2の`asset-field`契約を固定する。
//
// 「sourceにalgorithm fieldがあればその値と`algorithm`が完全一致。なければ
// definitionの`algorithm`必須。」
func TestResolveAssetFieldDigest(t *testing.T) {
	sha512Hex := strings.Repeat("a", 128)
	cases := []struct {
		name      string
		algorithm definition.DigestAlgorithm
		asset     *Asset
		want      string
		wantErr   bool
	}{
		{
			"definitionのalgorithmを使う", definition.AlgorithmSHA256,
			&Asset{Digest: digest64}, "sha256:" + digest64, false,
		},
		{
			"sourceのalgorithmを使う", "",
			&Asset{Digest: sha512Hex, DigestAlgorithm: definition.AlgorithmSHA512},
			"sha512:" + sha512Hex, false,
		},
		{
			"両方が一致", definition.AlgorithmSHA256,
			&Asset{Digest: digest64, DigestAlgorithm: definition.AlgorithmSHA256},
			"sha256:" + digest64, false,
		},
		{
			"両方が食い違う", definition.AlgorithmSHA256,
			&Asset{Digest: sha512Hex, DigestAlgorithm: definition.AlgorithmSHA512}, "", true,
		},
		{"algorithmを決められない", "", &Asset{Digest: digest64}, "", true},
		// hex長がalgorithmと一致しない値を拒否する（§6.5）。
		{"hex長が合わない", definition.AlgorithmSHA512, &Asset{Digest: digest64}, "", true},
		{"digestが空", definition.AlgorithmSHA256, &Asset{}, "", true},
		{"assetが無い", definition.AlgorithmSHA256, nil, "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			digest, err := resolveAssetFieldDigest(
				definition.ArtifactChecksum{
					Kind: definition.ChecksumAssetField, Algorithm: c.algorithm,
				}, c.asset)
			if c.wantErr {
				if err == nil {
					t.Fatal("resolveAssetFieldDigestが成功した")
				}
				return
			}
			if err != nil {
				t.Fatalf("= %v", err)
			}
			if digest.Upstream() != c.want {
				t.Fatalf("= %q, want %q", digest.Upstream(), c.want)
			}
		})
	}
}

// TestRenderTemplate は§7.1のURL/file templateを固定する。
func TestRenderTemplate(t *testing.T) {
	values := templateValues{
		version:  "22.18.0",
		metadata: map[string]string{"channel": "release"},
		asset:    &Asset{Name: "node.zip", ReleaseTag: "v22.18.0"},
	}
	t.Run("置換", func(t *testing.T) {
		got, err := renderTemplate(
			"https://example.invalid/{{version}}/{{metadata.channel}}/{{asset.name}}", values, true)
		if err != nil {
			t.Fatalf("= %v", err)
		}
		want := "https://example.invalid/22.18.0/release/node.zip"
		if got != want {
			t.Fatalf("= %q, want %q", got, want)
		}
	})

	t.Run("URL componentをpercent encodeする", func(t *testing.T) {
		// 値に`/`が入るとrender後のURLが別のpathを指しうる。
		escaped := templateValues{version: "1.0/../etc"}
		got, err := renderTemplate("https://example.invalid/{{version}}/tool.zip", escaped, true)
		if err != nil {
			t.Fatalf("= %v", err)
		}
		if strings.Contains(got, "/../") {
			t.Fatalf("= %q（escapeされていない）", got)
		}
	})

	t.Run("file名はescapeしない", func(t *testing.T) {
		got, err := renderTemplate("node-v{{version}}-win-x64.zip", values, false)
		if err != nil {
			t.Fatalf("= %v", err)
		}
		if got != "node-v22.18.0-win-x64.zip" {
			t.Fatalf("= %q", got)
		}
	})

	rejected := []struct{ name, text string }{
		{"未宣言のmetadata", "https://example.invalid/{{metadata.missing}}/a.zip"},
		{"使えないroot", "https://example.invalid/{{payload}}/a.zip"},
		{"sizeはtemplateで使えない", "https://example.invalid/{{asset.size}}/a.zip"},
	}
	for _, c := range rejected {
		t.Run(c.name, func(t *testing.T) {
			if _, err := renderTemplate(c.text, values, true); err == nil {
				t.Fatal("renderTemplateが成功した")
			}
		})
	}

	t.Run("値が空のitemをinstallable扱いしない", func(t *testing.T) {
		empty := templateValues{version: "", metadata: map[string]string{"channel": ""}}
		if _, err := renderTemplate("https://example.invalid/{{version}}/a.zip", empty, true); err == nil {
			t.Fatal("空の値が通った")
		}
		if _, err := renderTemplate("https://example.invalid/{{metadata.channel}}/a.zip", empty, true); err == nil {
			t.Fatal("空のmetadataが通った")
		}
	})

	t.Run("assetを選んでいない", func(t *testing.T) {
		noAsset := templateValues{version: "1.0.0"}
		if _, err := renderTemplate("https://example.invalid/{{asset.name}}", noAsset, true); err == nil {
			t.Fatal("assetなしで通った")
		}
	})
}

// TestCheckFileNameRejectsPaths はrender後のfile名がbasenameであることを固定する。
//
// artifact fileはdownload先のbasenameになるため、区切りを含む値を通すと
// `payload`の外へ書ける。
func TestCheckFileNameRejectsPaths(t *testing.T) {
	rejected := []string{"", ".", "..", "a/b.zip", `a\b.zip`, "a\x00.zip",
		strings.Repeat("a", definition.PathComponentMaxBytes+1)}
	for _, name := range rejected {
		if err := checkFileName(name); err == nil {
			t.Errorf("checkFileName(%q) が成功した", name)
		}
	}
	if err := checkFileName("node-v22.18.0-win-x64.zip"); err != nil {
		t.Errorf("正当なbasenameが拒否された: %v", err)
	}
}

// TestCheckArtifactURLRejectsUnsafe はrender後のartifact URLを固定する。
func TestCheckArtifactURLRejectsUnsafe(t *testing.T) {
	rejected := []string{
		"http://example.invalid/a.zip",
		"https://user:pass@example.invalid/a.zip",
		"https:///a.zip",
		"ftp://example.invalid/a.zip",
		"/a.zip",
	}
	for _, raw := range rejected {
		if err := checkArtifactURL(raw); err == nil {
			t.Errorf("checkArtifactURL(%q) が成功した", raw)
		}
	}
	if err := checkArtifactURL("https://example.invalid/a.zip"); err != nil {
		t.Errorf("正当なURLが拒否された: %v", err)
	}
}

// TestBuildAssetsRejectsTypeViolations は§6.5のasset field型契約を固定する。
//
// 「値はstring、sizeだけ非負integer。IDもprecision lossを避けるためdecimal
// stringとして扱う。」数値をstringへ暗黙変換しない。
func TestBuildAssetsRejectsTypeViolations(t *testing.T) {
	source := goAssetSource()
	cases := []struct{ name, document string }{
		{"nameが数値", `{"files":[{"filename":1,"url":"https://a.invalid/x","size":1,` +
			`"sha256":"` + digest64 + `","os":"windows","arch":"amd64"}]}`},
		{"sizeがstring", `{"files":[{"filename":"x","url":"https://a.invalid/x","size":"1",` +
			`"sha256":"` + digest64 + `","os":"windows","arch":"amd64"}]}`},
		{"sizeが負", `{"files":[{"filename":"x","url":"https://a.invalid/x","size":-1,` +
			`"sha256":"` + digest64 + `","os":"windows","arch":"amd64"}]}`},
		{"assets_pointerが配列でない", `{"files":{"filename":"x"}}`},
		{"参照fieldの欠落", `{"files":[{"filename":"x"}]}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := buildAssets(source, mustDecode(t, c.document)); err == nil {
				t.Fatal("buildAssetsが成功した")
			}
		})
	}

	t.Run("digest_algorithmのenum", func(t *testing.T) {
		withAlgorithm := goAssetSource()
		withAlgorithm.AssetFields[definition.AssetDigestAlgorithm] = "/algo"
		document := mustDecode(t, `{"files":[{"filename":"x","url":"https://a.invalid/x",`+
			`"size":1,"sha256":"`+digest64+`","algo":"md5","os":"windows","arch":"amd64"}]}`)
		if _, err := buildAssets(withAlgorithm, document); err == nil {
			t.Fatal("未知のdigest algorithmが通った")
		}
	})
}

// TestHasRequiredTokens は§6.2のtoken判定を固定する。
func TestHasRequiredTokens(t *testing.T) {
	source := nodeStyleSource()
	source.RequiredTokensPointer = definition.DeclaredPointer("/files")
	source.RequiredTokens = []string{"win-x64-zip"}

	t.Run("充足", func(t *testing.T) {
		ok, err := hasRequiredTokens(source, mustDecode(t, `{"files":["win-x64-zip","linux-x64"]}`))
		if err != nil || !ok {
			t.Fatalf("= %v, %v", ok, err)
		}
	})
	t.Run("不足", func(t *testing.T) {
		ok, err := hasRequiredTokens(source, mustDecode(t, `{"files":["linux-x64"]}`))
		if err != nil {
			t.Fatalf("= %v", err)
		}
		if ok {
			t.Fatal("不足がtrueになった")
		}
	})
	t.Run("pointerが解決できないのはsource error", func(t *testing.T) {
		if _, err := hasRequiredTokens(source, mustDecode(t, `{"other":[]}`)); err == nil {
			t.Fatal("欠落が通った")
		}
	})
	t.Run("非stringの要素", func(t *testing.T) {
		if _, err := hasRequiredTokens(source, mustDecode(t, `{"files":[1]}`)); err == nil {
			t.Fatal("非stringが通った")
		}
	})
	t.Run("重複", func(t *testing.T) {
		document := mustDecode(t, `{"files":["win-x64-zip","win-x64-zip"]}`)
		if _, err := hasRequiredTokens(source, document); err == nil {
			t.Fatal("重複が通った（§6.2は一意string arrayを要求する）")
		}
	})
	t.Run("未宣言なら常に充足", func(t *testing.T) {
		ok, err := hasRequiredTokens(nodeStyleSource(), mustDecode(t, `{}`))
		if err != nil || !ok {
			t.Fatalf("= %v, %v", ok, err)
		}
	})
}
