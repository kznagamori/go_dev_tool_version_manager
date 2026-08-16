package registry

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
)

// ManifestPath はregistry rootからのmanifest相対pathである。
const ManifestPath = "registry.toml"

// ExactTree はdocs/07-registry-and-tools.md §2が定めるregistry treeである。
//
// 「上記以外のentryをrelease registryへ含めない。helper、key、script、local
// bundle directoryは存在しない。」余分なfileを黙って無視すると、releaseへ意図
// しないものが混ざったことに気付けない。
func ExactTree() []string {
	return []string{
		"registry.toml",
		"tools/dotnet.toml",
		"tools/go.toml",
		"tools/node.toml",
		"tools/python.toml",
		"schemas/tool-definition-v1.json",
		"schemas/registry-v1.json",
		"messages/ja.toml",
		"licenses/python-build-standalone-MPL-2.0.txt",
	}
}

// CheckTree はregistry rootのfile集合が§2のexact treeと一致することを検査する。
//
// `paths`はregistry rootからの相対path（slash区切り）の集合である。過不足の
// どちらもerrorにする。
func CheckTree(paths []string) *domain.Error {
	want := make(map[string]struct{}, len(ExactTree()))
	for _, path := range ExactTree() {
		want[path] = struct{}{}
	}
	got := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		got[path] = struct{}{}
	}

	var missing, extra []string
	for path := range want {
		if _, ok := got[path]; !ok {
			missing = append(missing, path)
		}
	}
	for path := range got {
		if _, ok := want[path]; !ok {
			extra = append(extra, path)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)

	switch {
	case len(missing) > 0 && len(extra) > 0:
		return invalidError(fmt.Errorf("registry treeに欠落 %v と余分 %v がある", missing, extra))
	case len(missing) > 0:
		return invalidError(fmt.Errorf("registry treeに欠落がある: %v", missing))
	case len(extra) > 0:
		return invalidError(fmt.Errorf("registry treeに余分なentryがある: %v", extra))
	}
	return nil
}

// VerifyDefinitionDigest はdefinition fileのdigestがmanifestと一致することを
// 検査する（§3）。
//
// 「digestはraw file bytes」であり、gdtvm自身が計算する内部SHA-256である。
// upstream digestと違いalgorithm prefixを持たない。
func VerifyDefinitionDigest(entry ToolEntry, data []byte) *domain.Error {
	sum := sha256.Sum256(data)
	got := hex.EncodeToString(sum[:])
	if got != entry.SHA256 {
		return invalidError(fmt.Errorf(
			"%s のdigestが一致しない（manifest %s / 実file %s）", entry.Path, entry.SHA256, got))
	}
	return nil
}

// DefinitionDigest はdefinition fileのdigestを計算する。
//
// manifestを生成・更新する側と検証する側で同じ計算を使うために公開する。
func DefinitionDigest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
