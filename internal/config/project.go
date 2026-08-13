package config

import (
	"sort"
	"strconv"
	"strings"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain/port"
)

// LatestKeyword はCLIが`--latest`で使う語である。
//
// docs/05-configuration.md §4は「値はcatalog正規完全versionだけ。latest、channel、
// range、配列、provider、storage設定を保存しない」と定めるため、project fileへ
// 書かれていたら拒否する。
const LatestKeyword = "latest"

// ProjectConfig は検証済みのproject設定である（docs/05-configuration.md §4）。
type ProjectConfig struct {
	// Selections はtoolごとの選択versionのraw文字列である。
	//
	// versionのgrammarはtool definitionのschemeで決まる（docs/06-tool-definition.md
	// §4）。definitionはP3で入るため、ここでは形と明らかな禁止値だけを見る。
	Selections map[domain.ToolID]string
	// Origin は読んだfileのpathである。
	Origin domain.PathValue
}

// projectFile はproject TOMLのexact key集合である。
type projectFile struct {
	Schema *int64            `toml:"schema"`
	Tools  map[string]string `toml:"tools"`
}

// ParseProjectConfig はproject `.gdtvm.toml`をstrictに解析する。
//
// top-levelは`schema`と`tools`だけで、`schema=1`は必須である（§4）。`tools`の値が
// 文字列以外（配列、table、数値）ならdecode時に型違いとして落ちる。
//
// tool IDがregistryに実在するか、aliasでないか、versionがschemeに合うかは
// registryとdefinitionを要するため、ここでは判定しない。§4が「aliasを保存した
// fileは`E_PROJECT_CONFIG_INVALID`」と定める判定はP4のregistry読込みが行う。
// 本関数はkebab-case grammar（[domain.ParseToolID]）までを見る。
func ParseProjectConfig(data []byte, origin domain.PathValue) (ProjectConfig, *domain.Error) {
	if int64(len(data)) > ConfigFileMaxBytes {
		return ProjectConfig{}, projectError("config.project_file_too_large", map[string]string{
			"limit": strconv.Itoa(ConfigFileMaxBytes),
			"size":  strconv.Itoa(len(data)),
		})
	}

	var file projectFile
	if err := decodeStrict(data, &file); err != nil {
		return ProjectConfig{}, projectError("config.project_parse_failed", map[string]string{
			"detail": err.Error(),
		})
	}
	if file.Schema == nil || *file.Schema != SchemaVersion {
		return ProjectConfig{}, projectError("config.project_schema_invalid", nil)
	}

	selections := make(map[domain.ToolID]string, len(file.Tools))
	for _, rawID := range sortedMapKeys(file.Tools) {
		tool, err := domain.ParseToolID(rawID)
		if err != nil {
			return ProjectConfig{}, projectError("config.project_tool_id_invalid", map[string]string{
				"value": rawID,
			})
		}
		version := file.Tools[rawID]
		if err := validateProjectVersion(version); err != nil {
			return ProjectConfig{}, err
		}
		selections[tool] = version
	}

	return ProjectConfig{Selections: selections, Origin: origin}, nil
}

// validateProjectVersion は完全version以外の保存を拒否する（§4）。
//
// schemeごとのgrammarはdefinitionが決めるため、ここではschemeに依存せず判定できる
// ものだけを見る。空、前後空白、`latest`、範囲・wildcard記号、配列表記が対象である。
func validateProjectVersion(version string) *domain.Error {
	switch {
	case version == "":
		return projectError("config.project_version_empty", nil)
	case strings.TrimSpace(version) != version:
		return projectError("config.project_version_has_space", map[string]string{"value": version})
	case version == LatestKeyword:
		return projectError("config.project_version_latest", nil)
	case strings.ContainsAny(version, "*^~<>= ,|"):
		return projectError("config.project_version_not_exact", map[string]string{"value": version})
	}
	return nil
}

func sortedMapKeys(source map[string]string) []string {
	keys := make([]string, 0, len(source))
	for key := range source {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// VCSMarkers はGit worktree rootの目印である（docs/05-configuration.md §4.1）。
//
// 同§は「`.git` directory/fileの両方を認識する」と定める。submoduleやworktreeでは
// `.git`がfileになるため、directoryだけを見ると境界を越えて探索してしまう。
const VCSMarker = ".git"

// ProjectSearchRequest はproject file探索の入力である（docs/05-configuration.md §4.1）。
type ProjectSearchRequest struct {
	// ExplicitFile は`--project-file`である。空は未指定を表す。
	ExplicitFile string
	// Disabled は`--no-project`である。
	Disabled bool
	// StartDir はcanonical current directoryである。呼出し側がport経由で
	// realpathへ解決してから渡す。
	StartDir string
	// Host はpath区切りとcase規則を決めるplatformである。
	Host domain.Platform
	// Filename は探すfile名である。schema 1では`.gdtvm.toml`固定。
	Filename string
	// StopAtVCSRoot は最寄りGit worktree rootで停止するかを表す。
	// CLI `--project-search-beyond-vcs-root`はその実行だけfalse相当にする。
	StopAtVCSRoot bool
	// FileSystem は存在検査に使う。
	FileSystem port.FileSystem
}

// ProjectSearchResult は探索結果である。
type ProjectSearchResult struct {
	// Found は1件見つかったかどうかを表す。
	Found bool
	// File は見つかったfileのpathである。Foundがfalseなら空pathのrole付き値になる。
	File domain.PathValue
	// StoppedAtVCSRoot はGit worktree rootで打ち切ったかを表す。`doctor`が
	// 「上位にfileがあるのに使われない」理由を説明できるようにする。
	StoppedAtVCSRoot bool
}

// FindProjectFile はproject fileを探索する（docs/05-configuration.md §4.1）。
//
// 手順は同§のとおり、`--project-file`があればそのfileだけ、`--no-project`なら
// 探索なし、そうでなければcanonical current directoryから親へ辿り、既定では
// 最寄りGit worktree rootで停止し、最初に見つけた1件だけを使う。複数mergeしない。
func FindProjectFile(req ProjectSearchRequest) (ProjectSearchResult, *domain.Error) {
	empty, err := newPath(domain.RoleProjectFile, "")
	if err != nil {
		return ProjectSearchResult{}, err
	}

	if req.Disabled {
		// `--no-project`は`--project-file`より強い。両方を与えた場合に
		// 「探索しない」と「このfileを読む」が両立しないためである。
		if req.ExplicitFile != "" {
			return ProjectSearchResult{}, usageError("config.project_file_with_no_project", nil)
		}
		return ProjectSearchResult{File: empty}, nil
	}

	if req.ExplicitFile != "" {
		return explicitProjectFile(req)
	}

	if req.FileSystem == nil {
		return ProjectSearchResult{}, usageError("config.filesystem_missing", nil)
	}
	if req.StartDir == "" || req.Host.IsZero() || req.Filename == "" {
		return ProjectSearchResult{}, usageError("config.project_search_request_incomplete", nil)
	}

	return walkUpwards(req, empty)
}

func explicitProjectFile(req ProjectSearchRequest) (ProjectSearchResult, *domain.Error) {
	if req.Host.IsZero() {
		return ProjectSearchResult{}, usageError("config.host_platform_missing", nil)
	}
	if !isAbsolutePath(req.ExplicitFile, pathSeparator(req.Host)) {
		return ProjectSearchResult{}, usageError("config.project_file_not_absolute", map[string]string{
			"value": req.ExplicitFile,
		})
	}
	file, err := newPath(domain.RoleProjectFile, req.ExplicitFile)
	if err != nil {
		return ProjectSearchResult{}, err
	}
	return ProjectSearchResult{Found: true, File: file}, nil
}

func walkUpwards(req ProjectSearchRequest, empty domain.PathValue) (ProjectSearchResult, *domain.Error) {
	separator := pathSeparator(req.Host)
	current := strings.TrimRight(req.StartDir, separator)

	for {
		candidate := joinPath(separator, current, req.Filename)
		exists, err := pathExists(req.FileSystem, candidate)
		if err != nil {
			return ProjectSearchResult{}, err
		}
		if exists {
			file, pathErr := newPath(domain.RoleProjectFile, candidate)
			if pathErr != nil {
				return ProjectSearchResult{}, pathErr
			}
			return ProjectSearchResult{Found: true, File: file}, nil
		}

		if req.StopAtVCSRoot {
			// project fileより先にVCS markerを見るのではなく、同じdirectoryを
			// 「file → marker」の順で見る。worktree root直下のproject fileは
			// 境界の内側にあり、使うのが正しいためである。
			marker := joinPath(separator, current, VCSMarker)
			isRoot, markerErr := pathExists(req.FileSystem, marker)
			if markerErr != nil {
				return ProjectSearchResult{}, markerErr
			}
			if isRoot {
				return ProjectSearchResult{File: empty, StoppedAtVCSRoot: true}, nil
			}
		}

		parent := parentDir(current, separator)
		if parent == "" || parent == current {
			return ProjectSearchResult{File: empty}, nil
		}
		current = parent
	}
}

// pathExists はfilesystem上の存在を返す。
//
// 存在しない場合とpermission errorを区別する。docs/05-configuration.md §4.1が
// 「symlink loop、permission error、競合caseを明確に失敗させる」と定めるため、
// 読めなかったことを「無かった」として黙って通さない。
func pathExists(filesystem port.FileSystem, path string) (bool, *domain.Error) {
	_, err := filesystem.Stat(path)
	if err == nil {
		return true, nil
	}
	if isNotExist(err) {
		return false, nil
	}
	return false, filesystemErrorWithCause("config.project_search_stat_failed", path, err)
}

// parentDir はseparatorに応じた親directoryを返す。rootに達したら空を返す。
func parentDir(path, separator string) string {
	trimmed := strings.TrimRight(path, separator)
	if isFilesystemRoot(trimmed, separator) {
		return ""
	}
	index := strings.LastIndex(trimmed, separator)
	if index < 0 {
		return ""
	}
	if separator == "/" {
		if index == 0 {
			return "/"
		}
		return trimmed[:index]
	}
	parent := trimmed[:index]
	// `C:` まで戻ったらdrive rootを表す`C:\`へ正規化する。
	if len(parent) == 2 && isDriveLetter(parent[0]) && parent[1] == ':' {
		return parent + `\`
	}
	if parent == "" {
		return ""
	}
	return parent
}
