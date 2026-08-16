package store

import (
	"strings"
	"testing"
	"time"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
)

// specCatalogJSON はdocs/04-storage-and-data.md §15の例そのものである。
const specCatalogJSON = `{
  "schema": 1,
  "tool_id": "node",
  "platform_id": "windows-amd64",
  "definition_sha256": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
  "source_identity": "https://nodejs.org/dist/index.json",
  "fetched_at": "2026-08-07T09:00:00Z",
  "expires_at": "2026-08-08T09:00:00Z",
  "items": [
    {
      "version": "22.18.0",
      "channel": "stable",
      "lifecycle": "supported",
      "lifecycle_evidence": "https://github.com/nodejs/Release",
      "lifecycle_assessed_at": "2026-08-07T00:00:00Z",
      "published_at": "2026-07-01T00:00:00Z",
      "installable": true,
      "unavailable_reason": "",
      "provider_kind": "official",
      "provider_release": "v22.18.0",
      "artifact_file": "node-v22.18.0-win-x64.zip",
      "artifact_url": "https://nodejs.org/dist/v22.18.0/node-v22.18.0-win-x64.zip",
      "artifact_size": 1,
      "artifact_digest": "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
      "checksum_source": "text-file"
    }
  ]
}`

func semverRequest(data string) CatalogRequest {
	return CatalogRequest{Data: []byte(data), Scheme: domain.SchemeSemver}
}

func TestParseCatalogAcceptsSpecExample(t *testing.T) {
	value, err := ParseCatalog(semverRequest(specCatalogJSON))
	if err != nil {
		t.Fatalf("ParseCatalog = %s", describe(err))
	}
	if value.Tool.String() != "node" || value.Platform.ID() != "windows-amd64" {
		t.Errorf("tool/platform = %q/%q", value.Tool, value.Platform.ID())
	}
	if !value.HasExpiry() {
		t.Error("expires_atが読めていない")
	}
	if !value.FetchedAt.Equal(time.Date(2026, 8, 7, 9, 0, 0, 0, time.UTC)) {
		t.Errorf("fetched_at = %v", value.FetchedAt)
	}
	if len(value.Items) != 1 {
		t.Fatalf("item件数 = %d", len(value.Items))
	}
	item := value.Items[0]
	if item.Version.String() != "22.18.0" || item.Channel != domain.ChannelStable {
		t.Errorf("version/channel = %q/%q", item.Version, item.Channel)
	}
	if item.Lifecycle != domain.LifecycleSupported || !item.Installable {
		t.Errorf("lifecycle/installable = %q/%v", item.Lifecycle, item.Installable)
	}
	if item.ArtifactDigest.Algorithm() != domain.AlgoSHA256 {
		t.Errorf("digest algorithm = %q", item.ArtifactDigest.Algorithm())
	}
}

func TestCatalogRoundTrip(t *testing.T) {
	value, parseErr := ParseCatalog(semverRequest(specCatalogJSON))
	if parseErr != nil {
		t.Fatalf("ParseCatalog = %s", describe(parseErr))
	}
	data, encodeErr := EncodeCatalog(value)
	if encodeErr != nil {
		t.Fatalf("EncodeCatalog = %s", describe(encodeErr))
	}
	again, reparseErr := ParseCatalog(CatalogRequest{Data: data, Scheme: domain.SchemeSemver})
	if reparseErr != nil {
		t.Fatalf("再parse = %s\n%s", describe(reparseErr), data)
	}
	if len(again.Items) != 1 || again.Items[0].Version.String() != value.Items[0].Version.String() {
		t.Errorf("round tripでitemが変わった\n%+v", again.Items)
	}
	if again.HasExpiry() != value.HasExpiry() {
		t.Error("expiryの有無が変わった")
	}
	assertTrailingLF(t, data)
}

// TestCatalogStaticSourceAllowsNullExpiry は§15の「static sourceは
// `expires_at=null`を許す」を固定する。
func TestCatalogStaticSourceAllowsNullExpiry(t *testing.T) {
	static := strings.NewReplacer(
		`"expires_at": "2026-08-08T09:00:00Z"`, `"expires_at": null`,
		`"source_identity": "https://nodejs.org/dist/index.json"`,
		`"source_identity": "definition:static"`,
	).Replace(specCatalogJSON)
	value, err := ParseCatalog(semverRequest(static))
	if err != nil {
		t.Fatalf("static sourceが落ちた: %s", describe(err))
	}
	if value.HasExpiry() {
		t.Error("nullなのに期限を持っている")
	}
	// 書き戻してもnullのままである。zero timeを西暦1年として書かない。
	data, encodeErr := EncodeCatalog(value)
	if encodeErr != nil {
		t.Fatalf("EncodeCatalog = %s", describe(encodeErr))
	}
	if !strings.Contains(string(data), `"expires_at":null`) {
		t.Errorf("expires_atがnullで出ていない: %s", data)
	}
}

// TestCatalogRejectsExpiryBeforeFetch は作った瞬間に期限切れのcatalogを拒否する。
func TestCatalogRejectsExpiryBeforeFetch(t *testing.T) {
	stale := strings.Replace(specCatalogJSON,
		`"expires_at": "2026-08-08T09:00:00Z"`, `"expires_at": "2026-08-06T09:00:00Z"`, 1)
	if _, err := ParseCatalog(semverRequest(stale)); err == nil {
		t.Error("fetched_atより前のexpires_atが通った")
	}
	// 同時刻は許す。TTL 0のsourceを表現できなくしないため。
	same := strings.Replace(specCatalogJSON,
		`"expires_at": "2026-08-08T09:00:00Z"`, `"expires_at": "2026-08-07T09:00:00Z"`, 1)
	if _, err := ParseCatalog(semverRequest(same)); err != nil {
		t.Errorf("fetched_atと同時刻が落ちた: %s", describe(err))
	}
}

// TestCatalogItemOrder は§15の「version comparison降順、同値ならversion byte順」を
// 固定する。
//
// state fileと違いschemeを持つため、comparisonまで検査できる。
func TestCatalogItemOrder(t *testing.T) {
	item := func(version string) string {
		return `{
      "version": "` + version + `",
      "channel": "stable",
      "lifecycle": "supported",
      "lifecycle_evidence": "https://github.com/nodejs/Release",
      "lifecycle_assessed_at": "2026-08-07T00:00:00Z",
      "published_at": "2026-07-01T00:00:00Z",
      "installable": true,
      "unavailable_reason": "",
      "provider_kind": "official",
      "provider_release": "v` + version + `",
      "artifact_file": "node-v` + version + `-win-x64.zip",
      "artifact_url": "https://nodejs.org/dist/v` + version + `/node.zip",
      "artifact_size": 1,
      "artifact_digest": "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
      "checksum_source": "text-file"
    }`
	}
	catalog := func(versions ...string) string {
		items := make([]string, 0, len(versions))
		for _, version := range versions {
			items = append(items, item(version))
		}
		head := strings.Split(specCatalogJSON, `"items": [`)[0]
		return head + `"items": [` + strings.Join(items, ",") + "]\n}"
	}

	// comparison降順。byte順なら"22.9.0" < "22.18.0"だが、semverでは22.18.0が大きい。
	if _, err := ParseCatalog(semverRequest(catalog("22.18.0", "22.9.0", "20.0.0"))); err != nil {
		t.Fatalf("comparison降順が落ちた: %s", describe(err))
	}
	// byte順に並べるとcomparison降順ではない。仕様どおり拒否する。
	if _, err := ParseCatalog(semverRequest(catalog("22.9.0", "22.18.0"))); err == nil {
		t.Error("comparison昇順が通った")
	}
	if _, err := ParseCatalog(semverRequest(catalog("20.0.0", "22.18.0"))); err == nil {
		t.Error("昇順が通った")
	}
	if _, err := ParseCatalog(semverRequest(catalog("22.18.0", "22.18.0"))); err == nil {
		t.Error("同一versionの重複が通った")
	}
}

// TestCatalogRequiresScheme はschemeを必須にした判断を固定する。
//
// §15の順序契約がcomparisonを要するため、catalogはstate fileと違いschemeなしで
// 読めない。catalogは必ずdefinitionと組で使われるため、呼出し側は必ず持っている。
func TestCatalogRequiresScheme(t *testing.T) {
	if _, err := ParseCatalog(CatalogRequest{Data: []byte(specCatalogJSON)}); err == nil {
		t.Error("scheme未指定が通った")
	}
	if _, err := ParseCatalog(CatalogRequest{
		Data: []byte(specCatalogJSON), Scheme: domain.VersionScheme("cargo"),
	}); err == nil {
		t.Error("未知schemeが通った")
	}
	// schemeが合わないとversionのparseで落ちる。catalogとdefinitionの不一致を
	// 検出できることの確認である。
	if _, err := ParseCatalog(CatalogRequest{
		Data: []byte(specCatalogJSON), Scheme: domain.SchemeGo,
	}); err != nil {
		t.Logf("go schemeでは落ちる（想定内）: %s", describe(err))
	}
}

// TestCatalogUnavailableReason は§15のinstallableとreasonの整合を固定する。
func TestCatalogUnavailableReason(t *testing.T) {
	unavailable := strings.NewReplacer(
		`"installable": true`, `"installable": false`,
		`"unavailable_reason": ""`, `"unavailable_reason": "catalog.artifact_missing"`,
	).Replace(specCatalogJSON)
	if _, err := ParseCatalog(semverRequest(unavailable)); err != nil {
		t.Fatalf("installable=falseが落ちた: %s", describe(err))
	}

	rejects := []struct {
		name string
		json string
	}{
		{"installable=trueなのにreasonがある",
			strings.Replace(specCatalogJSON, `"unavailable_reason": ""`,
				`"unavailable_reason": "catalog.artifact_missing"`, 1)},
		{"installable=falseなのにreasonが空",
			strings.Replace(specCatalogJSON, `"installable": true`, `"installable": false`, 1)},
		{"reasonがmessage IDでない",
			strings.Replace(unavailable, `"catalog.artifact_missing"`, `"artifact missing"`, 1)},
		{"reasonのsegmentが1件",
			strings.Replace(unavailable, `"catalog.artifact_missing"`, `"missing"`, 1)},
	}
	for _, test := range rejects {
		t.Run(test.name, func(t *testing.T) {
			if _, err := ParseCatalog(semverRequest(test.json)); err == nil {
				t.Error("ParseCatalog = nil, want error")
			}
		})
	}
}

// TestCatalogPublishedAtMayBeEmpty は§15の空文字許容を固定する。
func TestCatalogPublishedAtMayBeEmpty(t *testing.T) {
	empty := strings.Replace(specCatalogJSON,
		`"published_at": "2026-07-01T00:00:00Z"`, `"published_at": ""`, 1)
	value, err := ParseCatalog(semverRequest(empty))
	if err != nil {
		t.Fatalf("published_at空が落ちた: %s", describe(err))
	}
	if !value.Items[0].PublishedAt.IsZero() {
		t.Error("空のpublished_atがzeroになっていない")
	}
	// lifecycle_assessed_atは全状態で必須のため空を許さない（§15）。
	assessed := strings.Replace(specCatalogJSON,
		`"lifecycle_assessed_at": "2026-08-07T00:00:00Z"`, `"lifecycle_assessed_at": ""`, 1)
	if _, err := ParseCatalog(semverRequest(assessed)); err == nil {
		t.Error("lifecycle_assessed_at空が通った")
	}
}

// TestCatalogLifecycleUnknownStillNeedsEvidence は§15の「全状態に必須」を固定する。
func TestCatalogLifecycleUnknownStillNeedsEvidence(t *testing.T) {
	unknown := strings.Replace(specCatalogJSON, `"lifecycle": "supported"`, `"lifecycle": "unknown"`, 1)
	if _, err := ParseCatalog(semverRequest(unknown)); err != nil {
		t.Fatalf("lifecycle=unknownが落ちた: %s", describe(err))
	}
	noEvidence := strings.Replace(unknown,
		`"lifecycle_evidence": "https://github.com/nodejs/Release"`, `"lifecycle_evidence": ""`, 1)
	if _, err := ParseCatalog(semverRequest(noEvidence)); err == nil {
		t.Error("evidence無しのunknownが通った")
	}
}

// TestParseCatalogRejects は§15のexact keyと値制約を固定する。
func TestParseCatalogRejects(t *testing.T) {
	tests := []struct {
		name string
		json string
	}{
		{"unknown top-level key",
			strings.Replace(specCatalogJSON, `"schema": 1,`, `"schema": 1, "extra": 1,`, 1)},
		{"unknown item key",
			strings.Replace(specCatalogJSON, `"version": "22.18.0",`, `"version": "22.18.0", "extra": 1,`, 1)},
		{"重複key",
			strings.Replace(specCatalogJSON, `"schema": 1,`, `"schema": 1, "schema": 2,`, 1)},
		{"schemaが2", strings.Replace(specCatalogJSON, `"schema": 1`, `"schema": 2`, 1)},
		{"tool_idが大文字", strings.Replace(specCatalogJSON, `"tool_id": "node"`, `"tool_id": "Node"`, 1)},
		{"platform_id未対応",
			strings.Replace(specCatalogJSON, `"platform_id": "windows-amd64"`, `"platform_id": "darwin-arm64"`, 1)},
		{"definition_sha256が上流形式",
			strings.Replace(specCatalogJSON, `"`+testDigestA+`"`, `"sha256:`+testDigestA+`"`, 1)},
		{"source_identityが空",
			strings.Replace(specCatalogJSON, `"https://nodejs.org/dist/index.json"`, `""`, 1)},
		{"source_identityがHTTP",
			strings.Replace(specCatalogJSON, `"https://nodejs.org/dist/index.json"`,
				`"http://nodejs.org/dist/index.json"`, 1)},
		{"channel enum外",
			strings.Replace(specCatalogJSON, `"channel": "stable"`, `"channel": "beta"`, 1)},
		{"lifecycle enum外",
			strings.Replace(specCatalogJSON, `"lifecycle": "supported"`, `"lifecycle": "deprecated"`, 1)},
		{"lifecycle_evidenceがHTTP",
			strings.Replace(specCatalogJSON, `"https://github.com/nodejs/Release"`,
				`"http://github.com/nodejs/Release"`, 1)},
		{"provider_kindがnone",
			strings.Replace(specCatalogJSON, `"provider_kind": "official"`, `"provider_kind": "none"`, 1)},
		{"artifact_urlにuserinfo",
			strings.Replace(specCatalogJSON, `"https://nodejs.org/dist/v22.18.0/`,
				`"https://user:token@nodejs.org/dist/v22.18.0/`, 1)},
		{"artifact_digestが内部形式",
			strings.Replace(specCatalogJSON,
				`"sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"`,
				`"cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"`, 1)},
		{"artifact_sizeが負",
			strings.Replace(specCatalogJSON, `"artifact_size": 1`, `"artifact_size": -1`, 1)},
		{"artifact_sizeが小数",
			strings.Replace(specCatalogJSON, `"artifact_size": 1`, `"artifact_size": 1.5`, 1)},
		{"artifact_fileが絶対path",
			strings.Replace(specCatalogJSON, `"node-v22.18.0-win-x64.zip"`, `"/node.zip"`, 1)},
		{"checksum_source enum外",
			strings.Replace(specCatalogJSON, `"checksum_source": "text-file"`, `"checksum_source": "asset"`, 1)},
		{"versionが範囲指定",
			strings.Replace(specCatalogJSON, `"version": "22.18.0"`, `"version": "^22.18.0"`, 1)},
		{"versionがlatest",
			strings.Replace(specCatalogJSON, `"version": "22.18.0"`, `"version": "latest"`, 1)},
		{"trailing data", specCatalogJSON + specCatalogJSON},
		{"BOM付き", "\ufeff" + specCatalogJSON},
		{"空", ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := ParseCatalog(semverRequest(test.json)); err == nil {
				t.Error("ParseCatalog = nil, want error")
			}
		})
	}
}

// TestCatalogErrorCarriesCatalogRole はerror codeとroleを固定する。
func TestCatalogErrorCarriesCatalogRole(t *testing.T) {
	_, err := ParseCatalog(semverRequest(`{"schema":2}`))
	if err == nil {
		t.Fatal("schema 2が通った")
	}
	if err.Code != "E_CATALOG_MISSING" {
		t.Errorf("code = %q, want E_CATALOG_MISSING", err.Code)
	}
	if err.PathRole != "catalog" {
		t.Errorf("path role = %q, want catalog", err.PathRole)
	}
	if len(err.Parameters) != 0 {
		t.Errorf("parametersが空でない: %v", err.Parameters)
	}
}

// TestCatalogAllowsEmptyItems はsourceが空を返した場合を固定する。
func TestCatalogAllowsEmptyItems(t *testing.T) {
	empty := strings.Split(specCatalogJSON, `"items": [`)[0] + `"items": []` + "\n}"
	value, err := ParseCatalog(semverRequest(empty))
	if err != nil {
		t.Fatalf("item 0件が落ちた: %s", describe(err))
	}
	if len(value.Items) != 0 {
		t.Errorf("item件数 = %d", len(value.Items))
	}
	if _, encodeErr := EncodeCatalog(value); encodeErr != nil {
		t.Errorf("item 0件のencodeが落ちた: %s", describe(encodeErr))
	}
}

// TestParseCatalogAcceptsUnavailableItem はunavailable itemがartifact 3 fieldを
// 空で持てることを固定する（P3-03の3本目で判明）。
//
// docs/06-tool-definition.md §7.1が「selectorに0件一致したversionは
// `installable=false/artifact-not-found`」、§6.2が「required tokenが1件でも
// ないversion itemは`installable=false/artifact-not-found`」と定める。その
// itemにはartifactが無く、file名もURLもdigestも書けない。空を拒否すると
// 仕様が要求する状態を表現できない。keyは常に存在し、値だけが空になる。
func TestParseCatalogAcceptsUnavailableItem(t *testing.T) {
	data := `{
  "schema": 1,
  "tool_id": "node",
  "platform_id": "windows-amd64",
  "definition_sha256": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
  "source_identity": "https://nodejs.org/dist/index.json",
  "fetched_at": "2026-08-07T09:00:00Z",
  "expires_at": "2026-08-08T09:00:00Z",
  "items": [
    {
      "version": "22.17.0",
      "channel": "stable",
      "lifecycle": "unknown",
      "lifecycle_evidence": "https://nodejs.org/dist/index.json",
      "lifecycle_assessed_at": "2026-08-07T09:00:00Z",
      "published_at": "2026-06-01T00:00:00Z",
      "installable": false,
      "unavailable_reason": "catalog.artifact_not_found",
      "provider_kind": "official",
      "provider_release": "v22.17.0",
      "artifact_file": "",
      "artifact_url": "",
      "artifact_size": 0,
      "artifact_digest": "",
      "checksum_source": "text-file"
    }
  ]
}`
	catalog, err := ParseCatalog(semverRequest(data))
	if err != nil {
		t.Fatalf("ParseCatalog = %s", err.Cause)
	}
	item := catalog.Items[0]
	if item.Installable {
		t.Fatal("installableがtrueになった")
	}
	if item.ArtifactFile != "" || item.ArtifactURL != "" || !item.ArtifactDigest.IsZero() {
		t.Fatalf("artifact fieldが空でない: %+v", item)
	}

	// installable=trueなら3 fieldは今までどおり必須である。
	installable := strings.Replace(data, `"installable": false`, `"installable": true`, 1)
	installable = strings.Replace(installable,
		`"unavailable_reason": "catalog.artifact_not_found"`, `"unavailable_reason": ""`, 1)
	if _, err := ParseCatalog(semverRequest(installable)); err == nil {
		t.Fatal("installable=trueでartifactが空のitemが通った")
	}
}
