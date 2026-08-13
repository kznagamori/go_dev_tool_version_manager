package config

import (
	"errors"
	"io/fs"
	"testing"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain/port"
)

func projectOrigin(t *testing.T, path string) domain.PathValue {
	t.Helper()
	value, err := domain.NewPathValue(domain.RoleProjectFile, path)
	if err != nil {
		t.Fatalf("NewPathValue = %v", err)
	}
	return value
}

// TestParseProjectConfigAcceptsSpecSample はdocs/05-configuration.md §4の例を固定する。
func TestParseProjectConfigAcceptsSpecSample(t *testing.T) {
	body := "schema = 1\n\n[tools]\ngo = \"1.26.5\"\nnode = \"22.18.0\"\npython = \"3.13.5\"\n"

	got, err := ParseProjectConfig([]byte(body), projectOrigin(t, "/repo/.gdtvm.toml"))
	if err != nil {
		t.Fatalf("ParseProjectConfig = %v, want nil", err)
	}
	if len(got.Selections) != 3 {
		t.Fatalf("Selections = %d件, want 3件", len(got.Selections))
	}
	for id, want := range map[string]string{"go": "1.26.5", "node": "22.18.0", "python": "3.13.5"} {
		tool, parseErr := domain.ParseToolID(id)
		if parseErr != nil {
			t.Fatalf("ParseToolID(%q) = %v", id, parseErr)
		}
		if got.Selections[tool] != want {
			t.Errorf("Selections[%s] = %q, want %q", id, got.Selections[tool], want)
		}
	}
	if got.Origin.Path() != "/repo/.gdtvm.toml" || got.Origin.Role() != domain.RoleProjectFile {
		t.Errorf("Origin = %+v", got.Origin)
	}
}

// TestParseProjectConfigAcceptsToolsOnlySchema は`tools`が無いfileを許すことを見る。
func TestParseProjectConfigAcceptsToolsOnlySchema(t *testing.T) {
	got, err := ParseProjectConfig([]byte("schema = 1\n"), projectOrigin(t, "/repo/.gdtvm.toml"))
	if err != nil {
		t.Fatalf("ParseProjectConfig = %v, want nil", err)
	}
	if len(got.Selections) != 0 {
		t.Errorf("Selections = %d件, want 0件", len(got.Selections))
	}
}

// TestParseProjectConfigRejectsStrictViolations はdocs/05-configuration.md §4の
// 「top-levelは`schema`, `tools`だけ」「値はcatalog正規完全versionだけ」を固定する。
func TestParseProjectConfigRejectsStrictViolations(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"schemaが無い", "[tools]\ngo = \"1.26.5\"\n"},
		{"schemaが1以外", "schema = 2\n"},
		{"空file", ""},
		{"unknown top-level key", "schema = 1\ndisabled = [\"go\"]\n"},
		{"unknown table", "schema = 1\n[providers]\ngo = \"official\"\n"},
		{"重複key", "schema = 1\n[tools]\ngo = \"1.26.5\"\ngo = \"1.26.4\"\n"},
		{"tool IDが大文字", "schema = 1\n[tools]\nGo = \"1.26.5\"\n"},
		{"tool IDがunderscore", "schema = 1\n[tools]\ndotnet_sdk = \"9.0.100\"\n"},
		{"tool IDが空", "schema = 1\n[tools]\n\"\" = \"1.26.5\"\n"},
		{"値が配列", "schema = 1\n[tools]\ngo = [\"1.26.5\"]\n"},
		{"値がtable", "schema = 1\n[tools.go]\nversion = \"1.26.5\"\n"},
		{"値が数値", "schema = 1\n[tools]\ngo = 1\n"},
		{"値が空", "schema = 1\n[tools]\ngo = \"\"\n"},
		{"値がlatest", "schema = 1\n[tools]\ngo = \"latest\"\n"},
		{"値がrange", "schema = 1\n[tools]\ngo = \">=1.26.0\"\n"},
		{"値がcaret range", "schema = 1\n[tools]\ngo = \"^1.26\"\n"},
		{"値がwildcard", "schema = 1\n[tools]\ngo = \"1.26.*\"\n"},
		{"値に前後空白", "schema = 1\n[tools]\ngo = \" 1.26.5\"\n"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := ParseProjectConfig([]byte(test.body), projectOrigin(t, "/repo/.gdtvm.toml"))
			if err == nil {
				t.Fatal("ParseProjectConfig = nil, want error")
			}
			// §7の終了code表はproject設定を`E_PROJECT_CONFIG_INVALID`とする。
			if err.Code != domain.CodeProjectConfigInvalid {
				t.Errorf("Code = %s, want %s", err.Code, domain.CodeProjectConfigInvalid)
			}
			if validateErr := err.Validate(); validateErr != nil {
				t.Errorf("typed errorがValidateで落ちた: %v", validateErr)
			}
		})
	}
}

func TestParseProjectConfigRejectsOversizedFile(t *testing.T) {
	head := "schema = 1\n"
	over := head + string(make([]byte, ConfigFileMaxBytes))

	_, err := ParseProjectConfig([]byte(over), projectOrigin(t, "/repo/.gdtvm.toml"))
	if err == nil {
		t.Fatal("上限超過が通った")
	}
	if err.Code != domain.CodeProjectConfigInvalid {
		t.Errorf("Code = %s", err.Code)
	}
}

// searchFS は存在するpathの集合だけを持つtest用FileSystemである。
//
// port.FileSystemを埋め込んで未使用methodを補う。探索はStatだけを使うため、
// 他のmethodが呼ばれたらnil panicで気付ける。黙って成功させるより望ましい。
type searchFS struct {
	port.FileSystem
	exist map[string]bool
	fail  map[string]error
}

func newSearchFS(paths ...string) *searchFS {
	exist := make(map[string]bool, len(paths))
	for _, path := range paths {
		exist[path] = true
	}
	return &searchFS{exist: exist, fail: make(map[string]error)}
}

func (s *searchFS) Stat(path string) (port.FileInfo, error) {
	if err, ok := s.fail[path]; ok {
		return port.FileInfo{}, err
	}
	if s.exist[path] {
		return port.FileInfo{}, nil
	}
	return port.FileInfo{}, fs.ErrNotExist
}

// TestFindProjectFileWalksUpwards はdocs/05-configuration.md §4.1の探索順を固定する。
func TestFindProjectFileWalksUpwards(t *testing.T) {
	filesystem := newSearchFS("/repo/.gdtvm.toml")

	got, err := FindProjectFile(ProjectSearchRequest{
		StartDir: "/repo/app/cmd", Host: linuxHost(t), Filename: ProjectFileName,
		StopAtVCSRoot: false, FileSystem: filesystem,
	})
	if err != nil {
		t.Fatalf("FindProjectFile = %v, want nil", err)
	}
	if !got.Found || got.File.Path() != "/repo/.gdtvm.toml" {
		t.Errorf("File = %+v, want /repo/.gdtvm.toml", got)
	}
	if got.File.Role() != domain.RoleProjectFile {
		t.Errorf("role = %q", got.File.Role())
	}
}

// TestFindProjectFileUsesNearestOnly は「最初に見つけた1件だけ」を固定する。
func TestFindProjectFileUsesNearestOnly(t *testing.T) {
	filesystem := newSearchFS("/repo/.gdtvm.toml", "/repo/app/.gdtvm.toml")

	got, err := FindProjectFile(ProjectSearchRequest{
		StartDir: "/repo/app/cmd", Host: linuxHost(t), Filename: ProjectFileName,
		FileSystem: filesystem,
	})
	if err != nil {
		t.Fatalf("FindProjectFile = %v, want nil", err)
	}
	if got.File.Path() != "/repo/app/.gdtvm.toml" {
		t.Errorf("File = %q, want /repo/app/.gdtvm.toml", got.File.Path())
	}
}

// TestFindProjectFileStopsAtVCSRoot は§4.1「最寄りGit worktree rootで停止」を固定する。
//
// `.git`はdirectoryとfileの両方を認識する（§4.1）。判定を存在検査だけで行うため、
// submoduleやworktreeで`.git`がfileになっていても同じく境界として扱う。
func TestFindProjectFileStopsAtVCSRoot(t *testing.T) {
	filesystem := newSearchFS("/repo/.gdtvm.toml", "/repo/app/.git")

	got, err := FindProjectFile(ProjectSearchRequest{
		StartDir: "/repo/app/cmd", Host: linuxHost(t), Filename: ProjectFileName,
		StopAtVCSRoot: true, FileSystem: filesystem,
	})
	if err != nil {
		t.Fatalf("FindProjectFile = %v, want nil", err)
	}
	if got.Found {
		t.Errorf("worktree rootを越えて %q を使った", got.File.Path())
	}
	if !got.StoppedAtVCSRoot {
		t.Error("StoppedAtVCSRoot = false, want true")
	}

	// `stop_at_vcs_root=false`ならfilesystem rootまで探索する（§3.3）。
	beyond, err := FindProjectFile(ProjectSearchRequest{
		StartDir: "/repo/app/cmd", Host: linuxHost(t), Filename: ProjectFileName,
		StopAtVCSRoot: false, FileSystem: filesystem,
	})
	if err != nil {
		t.Fatalf("FindProjectFile = %v, want nil", err)
	}
	if !beyond.Found || beyond.File.Path() != "/repo/.gdtvm.toml" {
		t.Errorf("File = %+v, want /repo/.gdtvm.toml", beyond)
	}
}

// TestFindProjectFileUsesFileAtVCSRoot はworktree root直下のfileを使うことを固定する。
//
// 境界の内側にあるfileは有効である。marker検出をfile検出より先に行うと使えなくなる。
func TestFindProjectFileUsesFileAtVCSRoot(t *testing.T) {
	filesystem := newSearchFS("/repo/.gdtvm.toml", "/repo/.git")

	got, err := FindProjectFile(ProjectSearchRequest{
		StartDir: "/repo/app", Host: linuxHost(t), Filename: ProjectFileName,
		StopAtVCSRoot: true, FileSystem: filesystem,
	})
	if err != nil {
		t.Fatalf("FindProjectFile = %v, want nil", err)
	}
	if !got.Found || got.File.Path() != "/repo/.gdtvm.toml" {
		t.Errorf("File = %+v, want /repo/.gdtvm.toml", got)
	}
}

func TestFindProjectFileWindowsWalksToDriveRoot(t *testing.T) {
	filesystem := newSearchFS(`D:\repo\.gdtvm.toml`)

	got, err := FindProjectFile(ProjectSearchRequest{
		StartDir: `D:\repo\app\cmd`, Host: windowsHost(t), Filename: ProjectFileName,
		FileSystem: filesystem,
	})
	if err != nil {
		t.Fatalf("FindProjectFile = %v, want nil", err)
	}
	if !got.Found || got.File.Path() != `D:\repo\.gdtvm.toml` {
		t.Errorf("File = %+v", got)
	}

	// 見つからない場合はdrive rootで打ち切る。無限loopにしない。
	missing, err := FindProjectFile(ProjectSearchRequest{
		StartDir: `D:\other\dir`, Host: windowsHost(t), Filename: ProjectFileName,
		FileSystem: newSearchFS(),
	})
	if err != nil {
		t.Fatalf("FindProjectFile = %v, want nil", err)
	}
	if missing.Found {
		t.Errorf("見つからないはずが %q を返した", missing.File.Path())
	}
}

func TestFindProjectFileExplicitAndDisabled(t *testing.T) {
	// `--project-file`はそのfileだけを使い、探索しない。
	got, err := FindProjectFile(ProjectSearchRequest{
		ExplicitFile: "/elsewhere/custom.toml", Host: linuxHost(t),
	})
	if err != nil {
		t.Fatalf("FindProjectFile = %v, want nil", err)
	}
	if !got.Found || got.File.Path() != "/elsewhere/custom.toml" {
		t.Errorf("File = %+v", got)
	}

	// `--no-project`は探索なし。
	disabled, err := FindProjectFile(ProjectSearchRequest{Disabled: true})
	if err != nil {
		t.Fatalf("FindProjectFile = %v, want nil", err)
	}
	if disabled.Found {
		t.Errorf("--no-projectで %q を返した", disabled.File.Path())
	}

	// 両方を与えたら`E_USAGE`。「探索しない」と「このfileを読む」は両立しない。
	if _, bothErr := FindProjectFile(ProjectSearchRequest{
		ExplicitFile: "/elsewhere/custom.toml", Disabled: true, Host: linuxHost(t),
	}); bothErr == nil {
		t.Error("--project-fileと--no-projectの併用が通った")
	} else if bothErr.Code != domain.CodeUsage {
		t.Errorf("Code = %s, want %s", bothErr.Code, domain.CodeUsage)
	}

	// 相対pathの`--project-file`は拒否する。cwdでfileが変わると再現しない。
	if _, relErr := FindProjectFile(ProjectSearchRequest{
		ExplicitFile: "custom.toml", Host: linuxHost(t),
	}); relErr == nil {
		t.Error("相対pathの--project-fileが通った")
	}
}

// TestFindProjectFileFailsOnStatError は§4.1「permission errorを明確に失敗させる」を固定する。
func TestFindProjectFileFailsOnStatError(t *testing.T) {
	filesystem := newSearchFS()
	filesystem.fail["/repo/app/.gdtvm.toml"] = errors.New("permission denied")

	_, err := FindProjectFile(ProjectSearchRequest{
		StartDir: "/repo/app", Host: linuxHost(t), Filename: ProjectFileName,
		FileSystem: filesystem,
	})
	if err == nil {
		t.Fatal("permission errorが「無かった」として通された")
	}
	if err.Code != domain.CodeFilesystem {
		t.Errorf("Code = %s, want %s", err.Code, domain.CodeFilesystem)
	}
	// 個人pathを公開境界へ出さず、roleだけを伝える（docs/10-security.md §9.2）。
	if err.PathRole != domain.RoleProjectFile {
		t.Errorf("PathRole = %q, want %q", err.PathRole, domain.RoleProjectFile)
	}
	for _, param := range err.Parameters {
		if text, ok := param.Str(); ok && text == "/repo/app/.gdtvm.toml" {
			t.Error("parametersへ実pathが入っている")
		}
	}
}

func TestFindProjectFileRejectsIncompleteRequest(t *testing.T) {
	tests := []struct {
		name    string
		request ProjectSearchRequest
	}{
		{"FileSystem未設定", ProjectSearchRequest{StartDir: "/repo", Host: linuxHost(t), Filename: ProjectFileName}},
		{"StartDirが空", ProjectSearchRequest{Host: linuxHost(t), Filename: ProjectFileName, FileSystem: newSearchFS()}},
		{"Host未設定", ProjectSearchRequest{StartDir: "/repo", Filename: ProjectFileName, FileSystem: newSearchFS()}},
		{"Filenameが空", ProjectSearchRequest{StartDir: "/repo", Host: linuxHost(t), FileSystem: newSearchFS()}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := FindProjectFile(test.request)
			if err == nil {
				t.Fatal("FindProjectFile = nil, want error")
			}
			if err.Code != domain.CodeUsage {
				t.Errorf("Code = %s, want %s", err.Code, domain.CodeUsage)
			}
		})
	}
}
