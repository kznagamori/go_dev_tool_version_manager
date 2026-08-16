package definition

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// schemaJSONPath はregistry rootからのschema JSONのpathである
// （docs/07-registry-and-tools.md §2）。
const schemaJSONPath = "../../registry/schemas/tool-definition-v1.json"

// loadSchemaJSON はschema JSONを読む。
func loadSchemaJSON(t *testing.T) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Clean(schemaJSONPath))
	if err != nil {
		t.Fatalf("schema JSONを読めない: %v", err)
	}
	var value map[string]any
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatalf("schema JSONがJSONとして不正: %v", err)
	}
	return value
}

// TestSchemaJSONIdentity は`$id`が§2の`schema_id`と一致することを固定する。
//
// 食い違うと、definitionが宣言するschemaと同梱schemaが別物になる。
func TestSchemaJSONIdentity(t *testing.T) {
	value := loadSchemaJSON(t)
	if id, _ := value["$id"].(string); id != SchemaID {
		t.Errorf("$id = %q, want %q", id, SchemaID)
	}
	properties := nested(t, value, "properties")
	schemaProperty := asObject(t, properties["schema"])
	if constant, ok := schemaProperty["const"].(float64); !ok || int(constant) != SchemaVersion {
		t.Errorf("properties.schema.const = %v, want %d", schemaProperty["const"], SchemaVersion)
	}
	schemaIDProperty := asObject(t, properties["schema_id"])
	if constant, _ := schemaIDProperty["const"].(string); constant != SchemaID {
		t.Errorf("properties.schema_id.const = %q, want %q", constant, SchemaID)
	}
}

// TestSchemaJSONIsAuxiliary はschema JSONが補助成果物である旨を持つことを固定する。
//
// docs/07-registry-and-tools.md §5が「schema JSONはTOML parser/semantic
// validatorの補助成果物であり、JSON Schemaだけで適合を宣言しない」と定める。
// 読む人がJSON Schemaを唯一の正本と誤解しないよう、file自身へ書いておく。
func TestSchemaJSONIsAuxiliary(t *testing.T) {
	value := loadSchemaJSON(t)
	description, _ := value["description"].(string)
	for _, want := range []string{"補助成果物", "internal/definition"} {
		if !strings.Contains(description, want) {
			t.Errorf("descriptionに %q が無い: %q", want, description)
		}
	}
}

// TestSchemaJSONKeySetsMatchValidator はschema JSONの許可keyがGo側と一致する
// ことを固定する。
//
// 片方だけにkeyを足すと、TOML parserが通す定義をschema JSONが落とす（または
// その逆）状態になる。key集合の同期をtestで担保する。
func TestSchemaJSONKeySetsMatchValidator(t *testing.T) {
	value := loadSchemaJSON(t)
	defs := nested(t, value, "$defs")

	tests := []struct {
		name string
		got  []string
		want []string
	}{
		{"top-level", propertyNames(t, value), []string{"platforms", "schema", "schema_id", "tool"}},
		{"tool", defPropertyNames(t, defs, "tool"), []string{
			"aliases", "description", "homepage", "id", "license", "name", "version_scheme"}},
		{"platforms", defPropertyNames(t, defs, "platform"), []string{
			"arch", "artifact", "artifact_kind", "id", "install", "libc", "license_notice",
			"os", "provider", "runtime", "storage", "validation", "version_source"}},
		{"provider", defPropertyNames(t, defs, "provider"), []string{
			"adoption_reason", "homepage", "license", "name", "repository"}},
		{"version_source", defPropertyNames(t, defs, "versionSource"), sortedCopy(sourceKeyOrder)},
		{"artifact", defPropertyNames(t, defs, "artifact"), []string{
			"checksum", "file", "format", "id", "redirect_hosts", "selector", "size", "source", "url"}},
		{"storage", defPropertyNames(t, defs, "storage"), []string{
			"id", "kind", "path", "purge", "scope"}},
		{"command", defPropertyNames(t, defs, "command"), []string{
			"args", "environment_profile", "name", "passthrough_signals",
			"required", "target", "working_directory"}},
		{"environment", defPropertyNames(t, defs, "environmentProfile"), []string{
			"id", "override_allowed", "path_append", "path_prepend", "set", "shell_export", "unset"}},
		{"probe", defPropertyNames(t, defs, "probe"), []string{
			"args", "expect", "expected_root", "expected_version", "id", "regex",
			"required", "required_paths", "runtime_command", "stream", "timeout"}},
		{"static_versions", defPropertyNames(t, defs, "staticVersion"), []string{
			"assets", "channel", "lifecycle", "lifecycle_assessed_at",
			"lifecycle_evidence", "published_at", "version"}},
		{"lifecycle_overrides", defPropertyNames(t, defs, "lifecycleOverride"), []string{
			"assessed_at", "evidence", "status", "version"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertStringSet(t, test.got, test.want)
		})
	}

	// asset fieldは§6.5の13値と`static_versions.assets`の全件必須が同じ集合である。
	assetNames := defPropertyNames(t, defs, "staticAsset")
	want := make([]string, 0, AssetFieldCount)
	for _, field := range assetFieldOrder {
		want = append(want, string(field))
	}
	assertStringSet(t, assetNames, sortedCopy(want))

	// `asset_fields`のkeyも同じ13値である。
	sourceDef := asObject(t, defs["versionSource"])
	sourceProps := asObject(t, sourceDef["properties"])
	assetFieldsDef := asObject(t, sourceProps["asset_fields"])
	assertStringSet(t, propertyNames(t, assetFieldsDef), sortedCopy(want))
}

// TestSchemaJSONEnumsMatchValidator はenum値がGo側と一致することを固定する。
func TestSchemaJSONEnumsMatchValidator(t *testing.T) {
	value := loadSchemaJSON(t)
	defs := nested(t, value, "$defs")

	tests := []struct {
		name string
		got  []string
		want []string
	}{
		{"version_scheme", defEnum(t, defs, "tool", "version_scheme"),
			[]string{"go", "python", "semver"}},
		{"artifact_kind", defEnum(t, defs, "platform", "artifact_kind"),
			[]string{string(KindOfficial), string(KindThirdParty)}},
		{"version_source.kind", defEnum(t, defs, "versionSource", "kind"),
			[]string{string(SourceJSON), string(SourceJSONIndex), string(SourceStatic)}},
		{"artifact.source", defEnum(t, defs, "artifact", "source"),
			[]string{string(SourceAsset), string(SourceTemplate)}},
		{"artifact.format", defEnum(t, defs, "artifact", "format"),
			[]string{string(FormatTarGz), string(FormatZip)}},
		{"storage.kind", defEnum(t, defs, "storage", "kind"), sortedCopy([]string{
			string(StorageConfig), string(StorageContentCache), string(StorageBuildCache),
			string(StorageGlobalBin), string(StorageGlobalPackages), string(StorageRuntimeData)})},
		{"storage.scope", defEnum(t, defs, "storage", "scope"),
			[]string{string(ScopeTool), string(ScopeVersion)}},
		{"storage.purge", defEnum(t, defs, "storage", "purge"), sortedCopy([]string{
			string(StorageRetain), string(StorageExplicit), string(StorageWithVersion)})},
		{"command.working_directory", defEnum(t, defs, "command", "working_directory"),
			[]string{string(WorkingInherit), string(WorkingPayload)}},
		{"probe.stream", defEnum(t, defs, "probe", "stream"), sortedCopy([]string{
			string(StreamStdout), string(StreamStderr), string(StreamCombined)})},
		{"probe.expect", defEnum(t, defs, "probe", "expect"), sortedCopy([]string{
			string(ExpectVersion), string(ExpectSuccess), string(ExpectPathWithin)})},
		{"static.channel", defEnum(t, defs, "staticVersion", "channel"),
			[]string{string(ChannelPrerelease), string(ChannelStable)}},
		{"static.lifecycle", defEnum(t, defs, "staticVersion", "lifecycle"), sortedCopy([]string{
			string(LifecycleSupported), string(LifecycleEOL), string(LifecycleUnknown)})},
		// §6.4はoverride statusを2値へ限る。lifecycleの3値と混同しない。
		{"override.status", defEnum(t, defs, "lifecycleOverride", "status"),
			[]string{string(LifecycleEOL), string(LifecycleSupported)}},
		{"digest_algorithm", defEnum(t, defs, "staticAsset", "digest_algorithm"),
			[]string{string(AlgorithmSHA256), string(AlgorithmSHA512)}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertStringSet(t, test.got, test.want)
		})
	}
}

// TestSchemaJSONLimitsMatchValidator は上限がGo側と一致することを固定する。
func TestSchemaJSONLimitsMatchValidator(t *testing.T) {
	value := loadSchemaJSON(t)
	defs := nested(t, value, "$defs")
	platformDef := asObject(t, defs["platform"])
	platformProps := asObject(t, platformDef["properties"])

	tests := []struct {
		name  string
		got   any
		want  int
		field string
	}{
		{"platforms", nested(t, value, "properties")["platforms"], PlatformMax, "maxItems"},
		{"aliases", defProperty(t, defs, "tool", "aliases"), AliasMax, "maxItems"},
		{"storage", platformProps["storage"], StorageMax, "maxItems"},
		{"max_documents", defProperty(t, defs, "versionSource", "max_documents"),
			MaxDocumentsLimit, "maximum"},
		{"max_items", defProperty(t, defs, "versionSource", "max_items"),
			MaxItemsLimit, "maximum"},
		{"static_versions", defProperty(t, defs, "versionSource", "static_versions"),
			StaticVersionMax, "maxItems"},
		{"lifecycle_overrides", defProperty(t, defs, "versionSource", "lifecycle_overrides"),
			OverrideMax, "maxItems"},
		{"assets", defProperty(t, defs, "staticVersion", "assets"), StaticAssetMax, "maxItems"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			node := asObject(t, test.got)
			got, ok := node[test.field].(float64)
			if !ok {
				t.Fatalf("%s が無い: %v", test.field, node)
			}
			if int(got) != test.want {
				t.Errorf("%s = %d, want %d", test.field, int(got), test.want)
			}
		})
	}

	// runtime/validationの上限。
	runtimeDef := asObject(t, platformProps["runtime"])
	runtimeProps := asObject(t, runtimeDef["properties"])
	for _, entry := range []struct {
		name string
		node any
		want int
	}{
		{"commands", runtimeProps["commands"], CommandMax},
		{"environment", runtimeProps["environment"], ProfileMax},
	} {
		node := asObject(t, entry.node)
		if got, _ := node["maxItems"].(float64); int(got) != entry.want {
			t.Errorf("%s maxItems = %v, want %d", entry.name, node["maxItems"], entry.want)
		}
	}
	validationDef := asObject(t, platformProps["validation"])
	probesDef := asObject(t, asObject(t, validationDef["properties"])["probes"])
	if got, _ := probesDef["maxItems"].(float64); int(got) != ProbeMax {
		t.Errorf("probes maxItems = %v, want %d", probesDef["maxItems"], ProbeMax)
	}
}

// TestSchemaJSONRejectsUnknownKeys はすべてのobjectがadditionalPropertiesを
// 閉じていることを固定する。
//
// 1か所でも開いていると、§1の「unknown key/tableを拒否する」がschema JSON側で
// 崩れる。
func TestSchemaJSONRejectsUnknownKeys(t *testing.T) {
	value := loadSchemaJSON(t)
	open := make([]string, 0)
	walkSchema(value, "", func(path string, node map[string]any) {
		if _, hasProperties := node["properties"]; !hasProperties {
			return
		}
		additional, present := node["additionalProperties"]
		if !present {
			open = append(open, path+" (additionalProperties未指定)")
			return
		}
		if allowed, isBool := additional.(bool); !isBool || allowed {
			open = append(open, path)
		}
	})
	if len(open) > 0 {
		sort.Strings(open)
		t.Errorf("additionalPropertiesが閉じていないobject: %s", strings.Join(open, ", "))
	}
}

func walkSchema(node any, path string, visit func(string, map[string]any)) {
	switch typed := node.(type) {
	case map[string]any:
		visit(path, typed)
		for _, key := range sortedMapKeys(typed) {
			walkSchema(typed[key], path+"/"+key, visit)
		}
	case []any:
		for index, item := range typed {
			walkSchema(item, fmt.Sprintf("%s[%d]", path, index), visit)
		}
	}
}

func sortedMapKeys(source map[string]any) []string {
	keys := make([]string, 0, len(source))
	for key := range source {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func nested(t *testing.T, node map[string]any, key string) map[string]any {
	t.Helper()
	return asObject(t, node[key])
}

func asObject(t *testing.T, node any) map[string]any {
	t.Helper()
	value, ok := node.(map[string]any)
	if !ok {
		t.Fatalf("objectでない: %#v", node)
	}
	return value
}

func propertyNames(t *testing.T, node map[string]any) []string {
	t.Helper()
	return sortedMapKeys(nested(t, node, "properties"))
}

func defPropertyNames(t *testing.T, defs map[string]any, name string) []string {
	t.Helper()
	return propertyNames(t, asObject(t, defs[name]))
}

func defProperty(t *testing.T, defs map[string]any, name, property string) any {
	t.Helper()
	return nested(t, asObject(t, defs[name]), "properties")[property]
}

func defEnum(t *testing.T, defs map[string]any, name, property string) []string {
	t.Helper()
	node := asObject(t, defProperty(t, defs, name, property))
	raw, ok := node["enum"].([]any)
	if !ok {
		t.Fatalf("%s.%s にenumが無い", name, property)
	}
	values := make([]string, 0, len(raw))
	for _, entry := range raw {
		text, isText := entry.(string)
		if !isText {
			t.Fatalf("%s.%s のenumがstringでない: %#v", name, property, entry)
		}
		values = append(values, text)
	}
	sort.Strings(values)
	return values
}

func sortedCopy(source []string) []string {
	values := append([]string{}, source...)
	sort.Strings(values)
	return values
}

func assertStringSet(t *testing.T, got, want []string) {
	t.Helper()
	gotSorted, wantSorted := sortedCopy(got), sortedCopy(want)
	if len(gotSorted) != len(wantSorted) {
		t.Fatalf("件数 = %d, want %d\ngot  = %v\nwant = %v",
			len(gotSorted), len(wantSorted), gotSorted, wantSorted)
	}
	for index := range gotSorted {
		if gotSorted[index] != wantSorted[index] {
			t.Fatalf("集合が一致しない\ngot  = %v\nwant = %v", gotSorted, wantSorted)
		}
	}
}
