package store

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
)

// tomlKeyRe はTOML行から`key = ...`のkeyを取り出す。
var tomlKeyRe = regexp.MustCompile(`^([a-z_0-9]+) = `)

// jsonKeyRe はJSON行から`"key":`のkeyを取り出す。
var jsonKeyRe = regexp.MustCompile(`^\s*"([a-z_0-9]+)"\s*:`)

// TestReceiptRequiresEveryKey は§14の「全件必須」を1 keyずつ確かめる。
//
// 個別のnegative testはkeyを網羅できず、fieldを増やしたときに検査漏れが
// 黙って通る。仕様の例から1行ずつ落として全件が拒否されることを確かめる。
func TestReceiptRequiresEveryKey(t *testing.T) {
	lines := strings.Split(strings.TrimRight(specReceiptTOML, "\n"), "\n")
	checked := 0
	for index, line := range lines {
		match := tomlKeyRe.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		key := match[1]
		// storageは唯一空を許すarrayである（§14）。table全体を消す形になる
		// `[[storage]]`配下のkeyも、1行だけ落とせばkey欠落として拒否される。
		t.Run(key+"/"+itoa(index), func(t *testing.T) {
			without := append(append([]string(nil), lines[:index]...), lines[index+1:]...)
			source := strings.Join(without, "\n") + "\n"
			if _, err := ParseReceipt([]byte(source)); err == nil {
				t.Errorf("%d行目の %q を落としても通った", index+1, key)
			}
		})
		checked++
	}
	// 例が持つkey数より極端に少なければ、走査自体が壊れている。
	if checked < 40 {
		t.Fatalf("検査したkeyが%d件しかない", checked)
	}
}

// TestCatalogRequiresEveryKey は§15の「top-level/entry keyは例の集合だけ」を
// 1 keyずつ確かめる。
//
// `expires_at`だけはstatic sourceでnullを許すため、欠落しても通る。
func TestCatalogRequiresEveryKey(t *testing.T) {
	lines := strings.Split(strings.TrimRight(specCatalogJSON, "\n"), "\n")
	checked := 0
	for index, line := range lines {
		match := jsonKeyRe.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		key := match[1]
		if key == "items" {
			// itemsを落とすとJSONとして壊れる（直前の行のカンマが残る）。
			// items自体の欠落は空itemsと同義であり、別testで扱う。
			continue
		}
		t.Run(key, func(t *testing.T) {
			without := append(append([]string(nil), lines[:index]...), lines[index+1:]...)
			source := strings.Join(without, "\n") + "\n"
			// 直前行の末尾カンマを消してJSONとして有効に保つ。
			source = fixTrailingComma(source)
			_, err := ParseCatalog(semverRequest(source))
			if key == "expires_at" {
				if err != nil {
					t.Errorf("expires_atの欠落はstatic sourceとして許すべき: %s", describe(err))
				}
				return
			}
			if err == nil {
				t.Errorf("%q を落としても通った", key)
			}
		})
		checked++
	}
	if checked < 20 {
		t.Fatalf("検査したkeyが%d件しかない", checked)
	}
}

// fixTrailingComma はobject/arrayの末尾に残ったカンマを取り除く。
func fixTrailingComma(source string) string {
	for _, pair := range []struct{ from, to string }{
		{",\n}", "\n}"}, {",\n  }", "\n  }"}, {",\n    }", "\n    }"},
		{",\n]", "\n]"}, {",\n  ]", "\n  ]"},
	} {
		source = strings.ReplaceAll(source, pair.from, pair.to)
	}
	return source
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	digits := ""
	for value > 0 {
		digits = string(rune('0'+value%10)) + digits
		value /= 10
	}
	return digits
}

// TestEncodeReceiptRejectsInvalidValue はencode経路の検査を固定する。
//
// state fileと同じく、programが組み立てた不正な値をそのままfileにしない。
func TestEncodeReceiptRejectsInvalidValue(t *testing.T) {
	base, parseErr := ParseReceipt([]byte(specReceiptTOML))
	if parseErr != nil {
		t.Fatalf("ParseReceipt = %s", describe(parseErr))
	}
	if _, err := EncodeReceipt(base); err != nil {
		t.Fatalf("正当な値が落ちた: %s", describe(err))
	}

	tests := []struct {
		name   string
		mutate func(*Receipt)
	}{
		{"payload_pathが他値", func(r *Receipt) { r.PayloadPath = "files" }},
		{"install_idが不正", func(r *Receipt) { r.InstallID = "x" }},
		{"installed_atがzero", func(r *Receipt) { r.InstalledAt = time.Time{} }},
		{"artifactのdigestがzero", func(r *Receipt) { r.Artifact.Digest = domain.Digest{} }},
		{"third-partyが未承認", func(r *Receipt) {
			r.Artifact.ProviderKind = ProviderThirdParty
			r.Artifact.ThirdPartyApproved = false
		}},
		{"commandsが空", func(r *Receipt) { r.Commands = nil }},
		{"probesが空", func(r *Receipt) { r.Probes = nil }},
		{"command_targetsが空", func(r *Receipt) { r.CommandTargets = nil }},
		{"environment_profilesが空", func(r *Receipt) { r.EnvironmentProfiles = nil }},
		{"未定義storageを参照", func(r *Receipt) {
			r.Commands[0].Target = "{{storage.missing}}/node.exe"
		}},
		{"未定義profileを参照", func(r *Receipt) {
			r.Commands[0].EnvironmentProfile = "missing"
		}},
		{"required probeがskipped", func(r *Receipt) {
			r.Probes[0].Required = true
			r.Probes[0].Status = ProbeSkipped
		}},
		{"probe timeoutが範囲外", func(r *Receipt) {
			r.Probes[0].TimeoutMillis = ProbeTimeoutMaxMillis + 1
		}},
		{"storage scopeとpurgeが不整合", func(r *Receipt) {
			r.Storage[0].Purge = PurgeRetain
		}},
		{"command_targetがpayload外", func(r *Receipt) {
			r.CommandTargets[0].Path = "tools/node.exe"
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := cloneReceipt(base)
			test.mutate(&value)
			if _, err := EncodeReceipt(value); err == nil {
				t.Error("EncodeReceipt = nil, want error")
			}
		})
	}
}

// cloneReceipt はmutateがbaseを壊さないようにsliceを複製する。
func cloneReceipt(source Receipt) Receipt {
	clone := source
	clone.Storage = append([]ReceiptStorage(nil), source.Storage...)
	clone.Commands = append([]ReceiptCommand(nil), source.Commands...)
	clone.EnvironmentProfiles = append([]ReceiptEnvironmentProfile(nil), source.EnvironmentProfiles...)
	clone.Probes = append([]ReceiptProbe(nil), source.Probes...)
	clone.CommandTargets = append([]ReceiptCommandTarget(nil), source.CommandTargets...)
	return clone
}

// TestEncodeCatalogRejectsInvalidValue はcatalogのencode検査を固定する。
func TestEncodeCatalogRejectsInvalidValue(t *testing.T) {
	base, parseErr := ParseCatalog(semverRequest(specCatalogJSON))
	if parseErr != nil {
		t.Fatalf("ParseCatalog = %s", describe(parseErr))
	}
	if _, err := EncodeCatalog(base); err != nil {
		t.Fatalf("正当な値が落ちた: %s", describe(err))
	}

	tests := []struct {
		name   string
		mutate func(*Catalog)
	}{
		{"definition_sha256が空", func(c *Catalog) { c.DefinitionSHA256 = "" }},
		{"source_identityが空", func(c *Catalog) { c.SourceIdentity = "" }},
		{"source_identityが非HTTPS", func(c *Catalog) { c.SourceIdentity = "http://example.com/x" }},
		{"fetched_atがzero", func(c *Catalog) { c.FetchedAt = time.Time{} }},
		{"expires_atがfetched_atより前", func(c *Catalog) {
			c.ExpiresAt = c.FetchedAt.Add(-time.Hour)
		}},
		{"installableなのにreasonがある", func(c *Catalog) {
			c.Items[0].UnavailableReason = "catalog.artifact_missing"
		}},
		{"lifecycle_evidenceが空", func(c *Catalog) { c.Items[0].LifecycleEvidence = "" }},
		{"lifecycle_assessed_atがzero", func(c *Catalog) {
			c.Items[0].LifecycleAssessedAt = time.Time{}
		}},
		{"artifact_digestがzero", func(c *Catalog) { c.Items[0].ArtifactDigest = domain.Digest{} }},
		{"artifact_urlが非HTTPS", func(c *Catalog) { c.Items[0].ArtifactURL = "http://example.com/x.zip" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := base
			value.Items = append([]CatalogItem(nil), base.Items...)
			test.mutate(&value)
			if _, err := EncodeCatalog(value); err == nil {
				t.Error("EncodeCatalog = nil, want error")
			}
		})
	}
}

// TestRequireHTTPSURL は§7のURL制約を固定する。
func TestRequireHTTPSURL(t *testing.T) {
	accepts := []string{
		"https://example.com",
		"https://example.com/a/b.zip",
		"https://example.com:8443/a?b=c#d",
		"https://example.com/%E6%97%A5%E6%9C%AC",
	}
	for _, text := range accepts {
		if _, err := requireHTTPSURL("u", text); err != nil {
			t.Errorf("URL %q が落ちた: %v", text, err)
		}
	}
	rejects := []struct {
		name string
		url  string
	}{
		{"空", ""},
		{"HTTP", "http://example.com"},
		{"file", "file:///etc/passwd"},
		{"scheme無し", "example.com/a"},
		{"host無し", "https:///a"},
		{"userinfo", "https://user@example.com"},
		{"userinfo＋password", "https://user:token@example.com"},
		{"8 KiB超過", "https://example.com/" + strings.Repeat("a", URLMaxBytes)},
	}
	for _, test := range rejects {
		t.Run(test.name, func(t *testing.T) {
			if _, err := requireHTTPSURL("u", test.url); err == nil {
				t.Errorf("URL %q が通った", test.url)
			}
		})
	}
}

// TestLooksLikeURL はsource identityのURL判定を固定する。
//
// definition記録の文字列をURLとして検査してしまうと、正当なcatalogを拒否する。
func TestLooksLikeURL(t *testing.T) {
	urls := []string{"https://a/b", "http://a", "ftp://a"}
	for _, text := range urls {
		if !looksLikeURL(text) {
			t.Errorf("%q がURLと判定されない", text)
		}
	}
	others := []string{"definition:static", "static", "", "a:b", "tools/node.toml"}
	for _, text := range others {
		if looksLikeURL(text) {
			t.Errorf("%q がURLと判定された", text)
		}
	}
}
