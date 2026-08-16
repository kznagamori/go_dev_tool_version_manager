package registry

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// schemaJSONPath はregistry rootからのmanifest schema JSONのpathである
// （docs/07-registry-and-tools.md §2）。
const schemaJSONPath = registryDir + "/schemas/registry-v1.json"

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

// TestSchemaJSONIsAuxiliary はschema JSONが補助成果物である旨を持つことを固定する。
//
// docs/07-registry-and-tools.md §5が「schema JSONはTOML parser/semantic
// validatorの補助成果物であり、JSON Schemaだけで適合を宣言しない」と定める。
// JSON Schemaはclient version範囲、digest照合、§2のexact tree検査を表せないため、
// 読む人が正本と誤解しないようfile自身へ書いておく。
func TestSchemaJSONIsAuxiliary(t *testing.T) {
	value := loadSchemaJSON(t)
	description, _ := value["description"].(string)
	for _, want := range []string{"補助成果物", "internal/registry"} {
		if !strings.Contains(description, want) {
			t.Errorf("descriptionに %q が無い: %q", want, description)
		}
	}
}

// TestSchemaJSONConstantsMatchParser は固定値がGo側と一致することを固定する。
//
// 片方だけを変えると、strict parserが通すmanifestをschema JSONが落とす（または
// その逆）状態になる。
func TestSchemaJSONConstantsMatchParser(t *testing.T) {
	value := loadSchemaJSON(t)
	properties := nested(t, value, "properties")

	for _, c := range []struct {
		key  string
		want int
	}{
		{"schema", SchemaVersion},
		{"tool_definition_schema", ToolDefinitionSchema},
	} {
		node := asObject(t, properties[c.key])
		constant, ok := node["const"].(float64)
		if !ok {
			t.Errorf("properties.%s.const が無い: %v", c.key, node)
			continue
		}
		if int(constant) != c.want {
			t.Errorf("properties.%s.const = %d, want %d", c.key, int(constant), c.want)
		}
	}

	// toolsはexactly 4件である（§3）。JSON Schemaでは min/max の両方で表す。
	tools := asObject(t, properties["tools"])
	for _, field := range []string{"minItems", "maxItems"} {
		got, ok := tools[field].(float64)
		if !ok {
			t.Errorf("tools.%s が無い: %v", field, tools)
			continue
		}
		if int(got) != ToolCount {
			t.Errorf("tools.%s = %d, want %d", field, int(got), ToolCount)
		}
	}
}

// TestSchemaJSONPatternsMatchParser は正規表現がGo側と一致することを固定する。
func TestSchemaJSONPatternsMatchParser(t *testing.T) {
	value := loadSchemaJSON(t)
	defs := nested(t, value, "$defs")

	digest := asObject(t, asObject(t, asObject(t, defs["toolEntry"])["properties"])["sha256"])
	if got, _ := digest["pattern"].(string); got != lowerHexRe.String() {
		t.Errorf("sha256.pattern = %q, want %q", got, lowerHexRe.String())
	}
}

// TestSchemaJSONKeySetsMatchParser はkey集合とrequiredがGo側と一致することを
// 固定する。
//
// §3は「top-level keyは`schema`, `tool_definition_schema`, `client_min_version`,
// `client_max_version`, `tools`。前4件のうちmaxだけ任意。tool entryは`id`,
// `path`, `sha256`だけで全件必須」と定める。
func TestSchemaJSONKeySetsMatchParser(t *testing.T) {
	value := loadSchemaJSON(t)
	defs := nested(t, value, "$defs")
	entry := asObject(t, defs["toolEntry"])

	assertStringSet(t, propertyNames(t, value), []string{
		"client_max_version", "client_min_version", "schema", "tool_definition_schema", "tools"})
	assertStringSet(t, requiredNames(t, value), []string{
		"client_min_version", "schema", "tool_definition_schema", "tools"})

	assertStringSet(t, propertyNames(t, entry), []string{"id", "path", "sha256"})
	assertStringSet(t, requiredNames(t, entry), []string{"id", "path", "sha256"})
}

// TestSchemaJSONRejectsUnknownKeys はすべてのobjectがadditionalPropertiesを
// 閉じていることを固定する。
//
// 1か所でも開いていると、§3の「unknown keyを拒否する」がschema JSON側で崩れる。
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

// --- helper ---

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

func requiredNames(t *testing.T, node map[string]any) []string {
	t.Helper()
	raw, ok := node["required"].([]any)
	if !ok {
		t.Fatalf("requiredが無い: %#v", node["required"])
	}
	values := make([]string, 0, len(raw))
	for _, item := range raw {
		text, isText := item.(string)
		if !isText {
			t.Fatalf("requiredの要素がstringでない: %#v", item)
		}
		values = append(values, text)
	}
	return values
}

func assertStringSet(t *testing.T, got, want []string) {
	t.Helper()
	gotSorted := append([]string{}, got...)
	wantSorted := append([]string{}, want...)
	sort.Strings(gotSorted)
	sort.Strings(wantSorted)
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
