package registry

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"

	"github.com/pelletier/go-toml/v2"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
)

// SchemaVersion は`registry.toml`のschema版である（docs/07-registry-and-tools.md §3）。
const SchemaVersion = 1

// ToolDefinitionSchema はregistryが収めるtool definitionのschema版である。
//
// clientが読めるのはschema 1だけであり、未知future schemaを推測して読まない
// （docs/04-storage-and-data.md §22）。
const ToolDefinitionSchema = 1

// ManifestFileMaxBytes は`registry.toml`の上限である。
//
// docs/04-storage-and-data.md §21「registry manifest各file 2 MiB」。
const ManifestFileMaxBytes = 2 << 20

// ToolCount は§2・§3が定めるtool entry数である。
//
// 「toolsはID ASCII byte順、exactly 4件」。件数を定数で持つのは、entryを足したり
// 消したりしたときにtestが気付けるようにするためである。
const ToolCount = 4

// lowerHexRe はgdtvm自身が計算するdigestの表記である。
//
// §3が「この`sha256`はgdtvm自身が計算するdigestであり、upstream digestと違い
// algorithm prefixを持たない」と定める。
var lowerHexRe = regexp.MustCompile(`^[0-9a-f]{64}$`)

// ToolEntry は`registry.toml`の`[[tools]]` 1件である。3 key全件必須。
type ToolEntry struct {
	// ID はtool IDである。file basenameと一致する。
	ID domain.ToolID
	// Path はregistry rootからの相対pathである。`tools/<id>.toml`固定。
	Path string
	// SHA256 はdefinition fileのraw bytesのdigestである（prefixなし64 hex）。
	SHA256 string
}

// Manifest は`registry.toml`のtyped表現である（§3）。
type Manifest struct {
	// Schema はmanifest自身のschema版である。
	Schema int
	// ToolDefinitionSchema は収めるdefinitionのschema版である。
	ToolDefinitionSchema int
	// ClientMinVersion はこのregistryを読める最小client versionである。
	ClientMinVersion domain.ClientVersion
	// ClientMaxVersion は上限である。宣言しない場合はzeroになる。
	ClientMaxVersion domain.ClientVersion
	// Tools はID ASCII byte順のexactly 4件である。
	Tools []ToolEntry
}

// manifestFile は§3のexact key集合である。
//
// pointer型にするのは、keyが無いことと空の値を区別するためである。unknown key、
// 重複、型違いはstrict decodeが拒否する。
type manifestFile struct {
	Schema               *int         `toml:"schema"`
	ToolDefinitionSchema *int         `toml:"tool_definition_schema"`
	ClientMinVersion     *string      `toml:"client_min_version"`
	ClientMaxVersion     *string      `toml:"client_max_version"`
	Tools                *[]toolTable `toml:"tools"`
}

type toolTable struct {
	ID     *string `toml:"id"`
	Path   *string `toml:"path"`
	SHA256 *string `toml:"sha256"`
}

// ParseManifest は`registry.toml`を読む（§3）。
//
// unknown key、重複、型違い、entry欠落/extraを拒否する。寛容なfallbackを持たない
// のは、registryがclientへ同梱される信頼の根であり、読めない部分を推測で補うと
// digest検証の前提が崩れるためである。
func ParseManifest(data []byte) (Manifest, *domain.Error) {
	if len(data) > ManifestFileMaxBytes {
		return Manifest{}, invalidError(
			fmt.Errorf("registry.tomlが%d byteを超える（%d byte）", ManifestFileMaxBytes, len(data)))
	}
	var file manifestFile
	decoder := toml.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&file); err != nil {
		return Manifest{}, invalidError(describeDecodeError(err))
	}
	value, err := buildManifest(file)
	if err != nil {
		return Manifest{}, invalidError(err)
	}
	return value, nil
}

func buildManifest(file manifestFile) (Manifest, error) {
	var value Manifest

	schema, err := requireInt(file.Schema, "schema")
	if err != nil {
		return value, err
	}
	if schema != SchemaVersion {
		return value, fmt.Errorf("schemaは%dだけ（%d）", SchemaVersion, schema)
	}
	value.Schema = schema

	definitionSchema, err := requireInt(file.ToolDefinitionSchema, "tool_definition_schema")
	if err != nil {
		return value, err
	}
	if definitionSchema != ToolDefinitionSchema {
		return value, fmt.Errorf(
			"tool_definition_schemaは%dだけ（%d）", ToolDefinitionSchema, definitionSchema)
	}
	value.ToolDefinitionSchema = definitionSchema

	value.ClientMinVersion, err = requireClientVersion(file.ClientMinVersion, "client_min_version")
	if err != nil {
		return value, err
	}
	// maxだけ任意である（§3）。
	if file.ClientMaxVersion != nil {
		value.ClientMaxVersion, err = requireClientVersion(file.ClientMaxVersion, "client_max_version")
		if err != nil {
			return value, err
		}
		// minより小さいmaxは、どのclientも読めないregistryを表す。
		if value.ClientMaxVersion.Compare(value.ClientMinVersion) < 0 {
			return value, fmt.Errorf("client_max_versionがclient_min_versionより小さい")
		}
	}

	value.Tools, err = buildTools(file.Tools)
	if err != nil {
		return value, err
	}
	return value, nil
}

// buildTools は§3のtool entry契約を検査する。
func buildTools(raw *[]toolTable) ([]ToolEntry, error) {
	if raw == nil {
		return nil, errors.New("`tools`が無い")
	}
	entries := *raw
	if len(entries) != ToolCount {
		return nil, fmt.Errorf("toolsはexactly %d件（%d件）", ToolCount, len(entries))
	}
	values := make([]ToolEntry, 0, len(entries))
	for index := range entries {
		entry, err := buildTool(&entries[index])
		if err != nil {
			return nil, fmt.Errorf("tools[%d]: %w", index, err)
		}
		values = append(values, entry)
	}
	if err := checkToolOrder(values); err != nil {
		return nil, err
	}
	return values, nil
}

func buildTool(table *toolTable) (ToolEntry, error) {
	var value ToolEntry

	idText, err := requireText(table.ID, "id")
	if err != nil {
		return value, err
	}
	id, parseErr := domain.ParseToolID(idText)
	if parseErr != nil {
		return value, parseErr
	}
	value.ID = id

	path, err := requireText(table.Path, "path")
	if err != nil {
		return value, err
	}
	// §3は`tools/<id>.toml`固定と定める。任意pathを許すと、manifestが
	// registry treeの外のfileを指せる。
	want := "tools/" + id.String() + ".toml"
	if path != want {
		return value, fmt.Errorf("pathは%qだけ（%q）", want, path)
	}
	value.Path = path

	digest, err := requireText(table.SHA256, "sha256")
	if err != nil {
		return value, err
	}
	if !lowerHexRe.MatchString(digest) {
		return value, fmt.Errorf("sha256が64文字のlowercase hexでない（%q）", digest)
	}
	value.SHA256 = digest
	return value, nil
}

// checkToolOrder は§3の「ID ASCII byte順、ID一意」を検査する。
//
// 並びを契約にするのは、manifestのdiffがentryの並べ替えで揺れないようにするため
// である。順序が自由だと、同じ内容のregistryが複数のbyte列を持ちうる。
func checkToolOrder(entries []ToolEntry) error {
	ids := make([]string, 0, len(entries))
	for _, entry := range entries {
		ids = append(ids, entry.ID.String())
	}
	// Goの文字列比較はbyte単位のため、tool ID grammar（lowercase英数字とhyphen）
	// では隣接比較がそのままASCII byte順の検査になる。
	for index := 1; index < len(ids); index++ {
		switch {
		case ids[index-1] == ids[index]:
			return fmt.Errorf("tool ID %q が重複している", ids[index])
		case ids[index-1] > ids[index]:
			return fmt.Errorf("toolsがID ASCII byte順でない（%q の後に %q）", ids[index-1], ids[index])
		}
	}
	return nil
}

// CheckClientVersion はclient versionがmin/maxの範囲にあることを検査する（§3）。
//
// 「client versionがmin未満/max超過、schema不一致、entry欠落/extra、digest不一致
// なら`E_REGISTRY_INVALID`」。
func (m Manifest) CheckClientVersion(client domain.ClientVersion) *domain.Error {
	if client.IsZero() {
		return invalidError(errors.New("client versionが未設定"))
	}
	if client.Compare(m.ClientMinVersion) < 0 {
		return invalidError(fmt.Errorf(
			"client version %s がregistryの下限 %s を下回る", client, m.ClientMinVersion))
	}
	if !m.ClientMaxVersion.IsZero() && client.Compare(m.ClientMaxVersion) > 0 {
		return invalidError(fmt.Errorf(
			"client version %s がregistryの上限 %s を超える", client, m.ClientMaxVersion))
	}
	return nil
}

// Entry はtool IDに対応するentryを返す。
func (m Manifest) Entry(id domain.ToolID) (ToolEntry, bool) {
	for _, entry := range m.Tools {
		if entry.ID == id {
			return entry, true
		}
	}
	return ToolEntry{}, false
}

func requireInt(raw *int, key string) (int, error) {
	if raw == nil {
		return 0, fmt.Errorf("`%s`が無い", key)
	}
	return *raw, nil
}

func requireText(raw *string, key string) (string, error) {
	if raw == nil {
		return "", fmt.Errorf("`%s`が無い", key)
	}
	if *raw == "" {
		return "", fmt.Errorf("`%s`が空", key)
	}
	return *raw, nil
}

func requireClientVersion(raw *string, key string) (domain.ClientVersion, error) {
	text, err := requireText(raw, key)
	if err != nil {
		return domain.ClientVersion{}, err
	}
	version, parseErr := domain.ParseClientVersion(text)
	if parseErr != nil {
		return domain.ClientVersion{}, fmt.Errorf("%s: %w", key, parseErr)
	}
	return version, nil
}
