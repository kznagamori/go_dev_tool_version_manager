package store

import (
	"strings"
	"testing"
)

// fullPlanJSON は§16の全配列を埋めたPlanである。
//
// §16の構造例はoperation entryを空にしているため、downloads/extracts/probes/
// writes/storageの契約がまったく検査されない。security上もっとも重要な部分
// （外部programの起動と利用者可視の書込み）がそこにあるため、実行可能な形の
// fixtureを別に持つ。
const fullPlanJSON = `{
  "schema": 1,
  "client_version": "devel",
  "invocation_id": "33333333333333333333333333333333",
  "operation_id": "22222222222222222222222222222222",
  "operation": "install",
  "created_at": "2026-08-07T09:00:00Z",
  "summary": {
    "tool_id": "node",
    "version": "22.18.0",
    "platform_id": "linux-amd64-glibc",
    "provider_kind": "official",
    "provider_name": "Node.js project",
    "provider_repository": "",
    "provider_homepage": "https://nodejs.org",
    "provider_license": "MIT",
    "provider_release": "v22.18.0",
    "license_notice": "",
    "channel": "stable",
    "lifecycle": "supported",
    "expected_digest": "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
    "checksum_source": "text-file",
    "warning_count": 0
  },
  "setup": null,
  "inputs": {
    "root_id": "0123456789abcdef0123456789abcdef",
    "config_sha256": "",
    "project_sha256": "",
    "definition_sha256": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
    "catalog_sha256": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
    "registry_sha256": "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
    "selections_revision": 8,
    "setup_revision": 3,
    "receipt_index_revision": 5
  },
  "downloads": [
    {
      "id": "node-archive",
      "provider_kind": "official",
      "provider_name": "Node.js project",
      "provider_repository": "",
      "provider_homepage": "https://nodejs.org",
      "provider_release": "v22.18.0",
      "url": "https://nodejs.org/dist/v22.18.0/node-v22.18.0-linux-x64.tar.gz",
      "file_name": "node-v22.18.0-linux-x64.tar.gz",
      "size": 1,
      "expected_digest": "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
      "checksum_source": "text-file",
      "license": "MIT",
      "adoption_reason_message_id": "",
      "destination": {"role": "download-cache", "path": "/data/gdtvm/cache/downloads/node.tar.gz"}
    }
  ],
  "extracts": [
    {
      "id": "node-extract",
      "source_download_id": "node-archive",
      "format": "tar.gz",
      "strip_components": 1,
      "destination": {"role": "staging", "path": "/data/gdtvm/tmp/staging-1"}
    }
  ],
  "probes": [
    {
      "id": "node-version",
      "runtime_command": "node",
      "executable": {"role": "staging", "path": "/data/gdtvm/tmp/staging-1/bin/node"},
      "version": "22.18.0",
      "source": "https://nodejs.org/dist/v22.18.0/node-v22.18.0-linux-x64.tar.gz",
      "artifact_digest": "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
      "license": "MIT",
      "reason_message_id": "probe.verify_version",
      "args": [
        {"kind": "literal", "value": "--version", "path": null},
        {"kind": "path", "value": "", "path": {"role": "staging", "path": "/data/gdtvm/tmp/probe-1/out"}}
      ],
      "working_directory": {"role": "staging", "path": "/data/gdtvm/tmp/probe-1"},
      "write_paths": [{"role": "staging", "path": "/data/gdtvm/tmp/probe-1"}],
      "stream": "stdout",
      "expect": "version",
      "regex": "^v(?P<version>[0-9]+[.][0-9]+[.][0-9]+)$",
      "expected_version": "22.18.0",
      "expected_root": null,
      "required_paths": [
        {"kind": "file", "path": {"role": "staging", "path": "/data/gdtvm/tmp/staging-1/bin/node"}}
      ],
      "timeout_ms": 30000,
      "required": true
    }
  ],
  "writes": [
    {
      "id": "current-link",
      "action": "symlink",
      "target": {"role": "current-link", "path": "/data/gdtvm/tools/node/current"}
    }
  ],
  "storage": [
    {
      "id": "global-packages",
      "kind": "global-packages",
      "scope": "version",
      "target": {"role": "version-data", "path": "/data/gdtvm/tools/node/versions/22.18.0/version-data/global-packages"},
      "purge": "with-version",
      "action": "create"
    }
  ],
  "warnings": []
}`

func TestParsePlanAcceptsFullExample(t *testing.T) {
	value, err := ParsePlan([]byte(fullPlanJSON))
	if err != nil {
		t.Fatalf("ParsePlan = %s", describe(err))
	}
	if len(value.Downloads) != 1 || value.Downloads[0].ID != "node-archive" {
		t.Fatalf("downloads = %+v", value.Downloads)
	}
	if value.Downloads[0].Destination.Role() != "download-cache" {
		t.Errorf("destination role = %q", value.Downloads[0].Destination.Role())
	}
	if len(value.Extracts) != 1 || value.Extracts[0].SourceDownloadID != "node-archive" {
		t.Errorf("extracts = %+v", value.Extracts)
	}
	if len(value.Probes) != 1 {
		t.Fatalf("probes = %+v", value.Probes)
	}
	probe := value.Probes[0]
	if len(probe.Args) != 2 || probe.Args[0].Kind != ArgLiteral || probe.Args[1].Kind != ArgPath {
		t.Errorf("args = %+v", probe.Args)
	}
	if probe.Args[0].Value != "--version" || probe.Args[1].Value != "" {
		t.Errorf("arg values = %q/%q", probe.Args[0].Value, probe.Args[1].Value)
	}
	if probe.ExpectedRoot != nil {
		t.Error("expected_rootがnullでない")
	}
	if len(probe.RequiredPaths) != 1 || probe.RequiredPaths[0].Kind != RequiredFile {
		t.Errorf("required_paths = %+v", probe.RequiredPaths)
	}
	if len(value.Writes) != 1 || value.Writes[0].Action != WriteSymlink {
		t.Errorf("writes = %+v", value.Writes)
	}
	if len(value.Storage) != 1 || value.Storage[0].Action != StorageCreate {
		t.Errorf("storage = %+v", value.Storage)
	}
}

func TestFullPlanRoundTrip(t *testing.T) {
	value, parseErr := ParsePlan([]byte(fullPlanJSON))
	if parseErr != nil {
		t.Fatalf("ParsePlan = %s", describe(parseErr))
	}
	data, encodeErr := EncodePlan(value)
	if encodeErr != nil {
		t.Fatalf("EncodePlan = %s", describe(encodeErr))
	}
	again, reparseErr := ParsePlan(data)
	if reparseErr != nil {
		t.Fatalf("再parse = %s\n%s", describe(reparseErr), data)
	}
	if len(again.Downloads) != 1 || len(again.Extracts) != 1 || len(again.Probes) != 1 ||
		len(again.Writes) != 1 || len(again.Storage) != 1 {
		t.Fatalf("round tripで件数が変わった: %+v", again)
	}
	if again.Probes[0].ExpectedRoot != nil {
		t.Error("expected_rootのnullが復元されなかった")
	}
	if len(again.Probes[0].Args) != 2 {
		t.Errorf("argsの件数が変わった: %+v", again.Probes[0].Args)
	}
	// 同じ値から同じbyte列が出る。承認前と実行時の突き合わせが単純になる。
	second, secondErr := EncodePlan(value)
	if secondErr != nil {
		t.Fatalf("EncodePlan = %s", describe(secondErr))
	}
	if string(second) != string(data) {
		t.Error("出力が決定的でない")
	}
}

// TestPlanArgContract は§16の`PlanArg`契約を固定する。
func TestPlanArgContract(t *testing.T) {
	rejects := []struct{ name, json string }{
		{"literalにpathがある",
			strings.Replace(fullPlanJSON, `{"kind": "literal", "value": "--version", "path": null}`,
				`{"kind": "literal", "value": "--version", "path": {"role": "staging", "path": "/data/x"}}`, 1)},
		{"literalのvalueが空",
			strings.Replace(fullPlanJSON, `{"kind": "literal", "value": "--version", "path": null}`,
				`{"kind": "literal", "value": "", "path": null}`, 1)},
		{"pathにvalueがある",
			strings.Replace(fullPlanJSON, `{"kind": "path", "value": "", "path":`,
				`{"kind": "path", "value": "x", "path":`, 1)},
		{"pathのpathがnull",
			strings.Replace(fullPlanJSON,
				`{"kind": "path", "value": "", "path": {"role": "staging", "path": "/data/gdtvm/tmp/probe-1/out"}}`,
				`{"kind": "path", "value": "", "path": null}`, 1)},
		{"kindがenum外",
			strings.Replace(fullPlanJSON, `{"kind": "literal", "value": "--version"`,
				`{"kind": "template", "value": "--version"`, 1)},
		{"pathが相対",
			strings.Replace(fullPlanJSON, `"path": "/data/gdtvm/tmp/probe-1/out"`,
				`"path": "probe-1/out"`, 1)},
	}
	for _, test := range rejects {
		t.Run(test.name, func(t *testing.T) {
			if _, err := ParsePlan([]byte(test.json)); err == nil {
				t.Error("ParsePlan = nil, want error")
			}
		})
	}
}

// TestPlanDownloadContract は§16のdownload契約を固定する。
func TestPlanDownloadContract(t *testing.T) {
	rejects := []struct{ name, json string }{
		{"destinationがdownload-cache/staging以外",
			strings.Replace(fullPlanJSON, `"destination": {"role": "download-cache"`,
				`"destination": {"role": "payload"`, 1)},
		{"urlが非HTTPS",
			strings.Replace(fullPlanJSON, `"url": "https://nodejs.org/dist/`,
				`"url": "http://nodejs.org/dist/`, 1)},
		{"urlにuserinfo",
			strings.Replace(fullPlanJSON, `"url": "https://nodejs.org/dist/`,
				`"url": "https://u:t@nodejs.org/dist/`, 1)},
		{"digestが内部形式",
			strings.Replace(fullPlanJSON,
				`"expected_digest": "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
      "checksum_source": "text-file",
      "license": "MIT"`,
				`"expected_digest": "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
      "checksum_source": "text-file",
      "license": "MIT"`, 1)},
		{"officialなのにadoption reasonがある",
			strings.Replace(fullPlanJSON, `"adoption_reason_message_id": ""`,
				`"adoption_reason_message_id": "download.adopted"`, 1)},
		{"file_nameに区切り",
			strings.Replace(fullPlanJSON, `"file_name": "node-v22.18.0-linux-x64.tar.gz"`,
				`"file_name": "dist/node.tar.gz"`, 1)},
		{"sizeが負", strings.Replace(fullPlanJSON, `"size": 1,`, `"size": -1,`, 1)},
		{"idがkebab-caseでない",
			strings.Replace(fullPlanJSON, `"id": "node-archive"`, `"id": "Node_Archive"`, 1)},
	}
	for _, test := range rejects {
		t.Run(test.name, func(t *testing.T) {
			if _, err := ParsePlan([]byte(test.json)); err == nil {
				t.Error("ParsePlan = nil, want error")
			}
		})
	}

	// third-partyは取得元・license・採用理由を必ず持つ。
	thirdParty := strings.NewReplacer(
		`"provider_kind": "official",
      "provider_name": "Node.js project",
      "provider_repository": "",`,
		`"provider_kind": "third-party",
      "provider_name": "Astral",
      "provider_repository": "https://github.com/astral-sh/x",`,
		`"adoption_reason_message_id": ""`, `"adoption_reason_message_id": "download.adopted"`,
	).Replace(fullPlanJSON)
	if _, err := ParsePlan([]byte(thirdParty)); err != nil {
		t.Fatalf("third-party downloadが落ちた: %s", describe(planErrorOf(thirdParty)))
	}
	noReason := strings.Replace(thirdParty,
		`"adoption_reason_message_id": "download.adopted"`, `"adoption_reason_message_id": ""`, 1)
	if _, err := ParsePlan([]byte(noReason)); err == nil {
		t.Error("third-partyで採用理由が空のdownloadが通った")
	}
	noRepo := strings.Replace(thirdParty,
		`"provider_repository": "https://github.com/astral-sh/x",`, `"provider_repository": "",`, 1)
	if _, err := ParsePlan([]byte(noRepo)); err == nil {
		t.Error("third-partyでrepositoryが空のdownloadが通った")
	}
}

// TestPlanExtractContract は§16のextract契約を固定する。
func TestPlanExtractContract(t *testing.T) {
	rejects := []struct{ name, json string }{
		{"source_download_idが存在しない",
			strings.Replace(fullPlanJSON, `"source_download_id": "node-archive"`,
				`"source_download_id": "missing"`, 1)},
		{"formatがenum外",
			strings.Replace(fullPlanJSON, `"format": "tar.gz"`, `"format": "tar.xz"`, 1)},
		{"strip_componentsが2",
			strings.Replace(fullPlanJSON, `"strip_components": 1`, `"strip_components": 2`, 1)},
		{"strip_componentsが負",
			strings.Replace(fullPlanJSON, `"strip_components": 1`, `"strip_components": -1`, 1)},
		{"destinationがstaging以外",
			strings.Replace(fullPlanJSON, `"destination": {"role": "staging", "path": "/data/gdtvm/tmp/staging-1"}`,
				`"destination": {"role": "payload", "path": "/data/gdtvm/tmp/staging-1"}`, 1)},
	}
	for _, test := range rejects {
		t.Run(test.name, func(t *testing.T) {
			if _, err := ParsePlan([]byte(test.json)); err == nil {
				t.Error("ParsePlan = nil, want error")
			}
		})
	}
	if _, err := ParsePlan([]byte(strings.Replace(fullPlanJSON,
		`"strip_components": 1`, `"strip_components": 0`, 1))); err != nil {
		t.Errorf("strip_components=0が落ちた: %v", err)
	}
}

// TestPlanProbeContract は§16のprobe契約を固定する。
//
// §16は「完全version、artifact URL/digest、provider license、理由を空にしない」と
// 定める。子processを起動する前に何を起動するかを完全に示すためである。
func TestPlanProbeContract(t *testing.T) {
	rejects := []struct{ name, json string }{
		{"versionが空",
			strings.Replace(fullPlanJSON, `"version": "22.18.0",
      "source":`, `"version": "",
      "source":`, 1)},
		{"sourceが非HTTPS",
			strings.Replace(fullPlanJSON, `"source": "https://nodejs.org/dist/`,
				`"source": "http://nodejs.org/dist/`, 1)},
		{"licenseが空",
			strings.Replace(fullPlanJSON, `"license": "MIT",
      "reason_message_id"`, `"license": "",
      "reason_message_id"`, 1)},
		{"reason_message_idが空",
			strings.Replace(fullPlanJSON, `"reason_message_id": "probe.verify_version"`,
				`"reason_message_id": ""`, 1)},
		{"reason_message_idがsegment 1件",
			strings.Replace(fullPlanJSON, `"reason_message_id": "probe.verify_version"`,
				`"reason_message_id": "verify"`, 1)},
		{"timeoutが上限超過",
			strings.Replace(fullPlanJSON, `"timeout_ms": 30000`, `"timeout_ms": 120001`, 1)},
		{"timeoutが0",
			strings.Replace(fullPlanJSON, `"timeout_ms": 30000`, `"timeout_ms": 0`, 1)},
		{"expect=versionでregexが空",
			strings.Replace(fullPlanJSON, `"regex": "^v(?P<version>[0-9]+[.][0-9]+[.][0-9]+)$"`,
				`"regex": ""`, 1)},
		{"expect=versionでexpected_rootがある",
			strings.Replace(fullPlanJSON, `"expected_root": null`,
				`"expected_root": {"role": "staging", "path": "/data/x"}`, 1)},
		{"working_directoryが相対",
			strings.Replace(fullPlanJSON, `"working_directory": {"role": "staging", "path": "/data/gdtvm/tmp/probe-1"}`,
				`"working_directory": {"role": "staging", "path": "tmp/probe-1"}`, 1)},
		{"required_pathのkindがenum外",
			strings.Replace(fullPlanJSON, `{"kind": "file", "path": {"role": "staging"`,
				`{"kind": "socket", "path": {"role": "staging"`, 1)},
		{"streamがenum外",
			strings.Replace(fullPlanJSON, `"stream": "stdout"`, `"stream": "both"`, 1)},
	}
	for _, test := range rejects {
		t.Run(test.name, func(t *testing.T) {
			if _, err := ParsePlan([]byte(test.json)); err == nil {
				t.Error("ParsePlan = nil, want error")
			}
		})
	}

	// expect=path-withinはexpected_rootを持つ。
	pathWithin := strings.NewReplacer(
		`"expect": "version"`, `"expect": "path-within"`,
		`"expected_version": "22.18.0"`, `"expected_version": ""`,
		`"expected_root": null`, `"expected_root": {"role": "staging", "path": "/data/gdtvm/tmp/staging-1"}`,
	).Replace(fullPlanJSON)
	if _, err := ParsePlan([]byte(pathWithin)); err != nil {
		t.Errorf("expect=path-withinが落ちた: %s", describe(planErrorOf(pathWithin)))
	}
}

// TestPlanWriteRegistryValue は§17.2のlocator例外がwritesにも適用されることを固定する。
func TestPlanWriteRegistryValue(t *testing.T) {
	registry := strings.Replace(fullPlanJSON,
		`{
      "id": "current-link",
      "action": "symlink",
      "target": {"role": "current-link", "path": "/data/gdtvm/tools/node/current"}
    }`,
		`{
      "id": "user-path",
      "action": "registry-value",
      "target": {"role": "config", "path": "HKCU\\Environment\\Path"}
    }`, 1)
	if _, err := ParsePlan([]byte(registry)); err != nil {
		t.Fatalf("registry-value writeが落ちた: %s", describe(planErrorOf(registry)))
	}
	// locatorはregistry-valueだけの例外である。他actionでは絶対pathが要る。
	wrongAction := strings.Replace(registry, `"action": "registry-value"`, `"action": "replace"`, 1)
	if _, err := ParsePlan([]byte(wrongAction)); err == nil {
		t.Error("action=replaceでlocatorが通った")
	}
	wrongRole := strings.Replace(registry, `"target": {"role": "config"`, `"target": {"role": "state"`, 1)
	if _, err := ParsePlan([]byte(wrongRole)); err == nil {
		t.Error("registry-valueのroleがconfig以外なのに通った")
	}
}

// TestPlanStorageContract は§16のstorage契約を固定する。
func TestPlanStorageContract(t *testing.T) {
	rejects := []struct{ name, json string }{
		{"actionがenum外",
			strings.Replace(fullPlanJSON, `"action": "create"`, `"action": "delete"`, 1)},
		{"scopeとpurgeが不整合",
			strings.Replace(fullPlanJSON, `"purge": "with-version"`, `"purge": "retain"`, 1)},
		{"targetが相対",
			strings.Replace(fullPlanJSON,
				`"path": "/data/gdtvm/tools/node/versions/22.18.0/version-data/global-packages"`,
				`"path": "version-data/global-packages"`, 1)},
		{"kindがenum外",
			strings.Replace(fullPlanJSON, `"kind": "global-packages"`, `"kind": "misc"`, 1)},
	}
	for _, test := range rejects {
		t.Run(test.name, func(t *testing.T) {
			if _, err := ParsePlan([]byte(test.json)); err == nil {
				t.Error("ParsePlan = nil, want error")
			}
		})
	}
}

// TestPlanIDsAreUniqueAcrossKinds は§16の「IDはPlan内で種類をまたいで一意」を固定する。
func TestPlanIDsAreUniqueAcrossKinds(t *testing.T) {
	collide := strings.Replace(fullPlanJSON, `"id": "current-link"`, `"id": "node-archive"`, 1)
	if _, err := ParsePlan([]byte(collide)); err == nil {
		t.Error("downloadとwriteでID重複が通った")
	}
	collideProbe := strings.Replace(fullPlanJSON, `"id": "node-version"`, `"id": "node-extract"`, 1)
	if _, err := ParsePlan([]byte(collideProbe)); err == nil {
		t.Error("extractとprobeでID重複が通った")
	}
}

// TestPlanArraysAreSorted は§16の「各配列はIDのASCII byte順」を固定する。
func TestPlanArraysAreSorted(t *testing.T) {
	second := `,
    {
      "id": "aaa-write",
      "action": "create",
      "target": {"role": "project-file", "path": "/work/.gdtvm.toml"}
    }`
	unsorted := strings.Replace(fullPlanJSON,
		`"target": {"role": "current-link", "path": "/data/gdtvm/tools/node/current"}
    }
  ],`,
		`"target": {"role": "current-link", "path": "/data/gdtvm/tools/node/current"}
    }`+second+`
  ],`, 1)
	if _, err := ParsePlan([]byte(unsorted)); err == nil {
		t.Error("ID降順のwritesが通った")
	}
	// encodeは整列して出す。同じ内容から同じdocumentが出るために必要。
	value, parseErr := ParsePlan([]byte(fullPlanJSON))
	if parseErr != nil {
		t.Fatalf("ParsePlan = %s", describe(parseErr))
	}
	value.Writes = append(value.Writes, PlanWrite{
		ID: "aaa-write", Action: WriteCreate, Target: value.Writes[0].Target,
	})
	data, encodeErr := EncodePlan(value)
	if encodeErr != nil {
		t.Fatalf("EncodePlan = %s", describe(encodeErr))
	}
	if strings.Index(string(data), `"aaa-write"`) > strings.Index(string(data), `"current-link"`) {
		t.Errorf("writesがID順で出ていない\n%s", data)
	}
}
