package config

import (
	"errors"
	"io/fs"
	"strings"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain/port"
)

// ModeSource はmodeがどこで決まったかを表す（docs/04-storage-and-data.md §1）。
//
// 同§の優先順位「CLIの一時`--mode`、有効なsetup state、導入経路の既定」を
// そのまま3値にしたものである。`doctor`がmodeの由来を説明できるようにするため、
// 決まった値だけでなく由来も返す。
type ModeSource string

// ModeSource のexactly 3値。
const (
	ModeSourceOverride       ModeSource = "override"
	ModeSourceSetupState     ModeSource = "setup-state"
	ModeSourceInstallDefault ModeSource = "install-default"
)

// ModeRequest はmode決定の入力である。
//
// OverrideとSetupStateはpointerで「与えられなかった」を表す。空のdomain.Modeで
// 代用すると、未指定と不正値を区別できない。
type ModeRequest struct {
	// Override はCLIの一時`--mode`である。
	Override *domain.Mode
	// SetupState は有効なsetup stateが持つmodeである。
	SetupState *domain.Mode
	// InstallDefault は導入経路の既定である。手動archiveは`portable`、
	// `install.ps1`/`install.sh`は`user`（§1）。
	InstallDefault domain.Mode
}

// ModeDecision はmode決定の結果である。
type ModeDecision struct {
	Mode   domain.Mode
	Source ModeSource
}

// DecideMode はdocs/04-storage-and-data.md §1の優先順位でmodeを決める。
//
// global `gdtvm.toml`はmode keyを持たないため、設定fileは入力に含めない。
func DecideMode(req ModeRequest) (ModeDecision, *domain.Error) {
	switch {
	case req.Override != nil:
		if err := validateMode(*req.Override, "--mode"); err != nil {
			return ModeDecision{}, err
		}
		return ModeDecision{Mode: *req.Override, Source: ModeSourceOverride}, nil
	case req.SetupState != nil:
		if err := validateMode(*req.SetupState, "setup state"); err != nil {
			return ModeDecision{}, err
		}
		return ModeDecision{Mode: *req.SetupState, Source: ModeSourceSetupState}, nil
	default:
		if err := validateMode(req.InstallDefault, "導入経路の既定"); err != nil {
			return ModeDecision{}, err
		}
		return ModeDecision{Mode: req.InstallDefault, Source: ModeSourceInstallDefault}, nil
	}
}

func validateMode(mode domain.Mode, origin string) *domain.Error {
	if _, err := domain.ParseMode(string(mode)); err != nil {
		return usageError("config.mode_invalid", map[string]string{
			"origin": origin,
			"mode":   string(mode),
		})
	}
	return nil
}

// RootRequest はroot決定の入力である。
//
// 実際のfilesystem検査は行わないため、port.FileSystemを受け取らない。owner、
// reparse point、unsafe filesystemの検査はP2-03が担当する
// （docs/13-progress.md）。ここで決めるのはpathそのものである。
type RootRequest struct {
	// Mode は[DecideMode]が決めたmodeである。
	Mode domain.Mode
	// Host はclientが動いているplatformである。§1.2のuser data rootがOSで
	// 異なるため必要になる。
	Host domain.Platform
	// ExecutableDir は`gdtvm[.exe]`が存在するcanonical directoryである。
	// 呼出し側がport経由でrealpathへ解決してから渡す。
	ExecutableDir string
	// User はOS user lookupの結果である。
	User port.UserIdentity
	// HomeOverride はCLIの`--home`である。空は未指定を表す。
	HomeOverride string
	// ConfiguredUserDataRoot はglobal `gdtvm.toml`の`paths.user_data_root`である。
	// user modeだけで意味を持ち、空はOS既定を表す（docs/05-configuration.md §3）。
	ConfiguredUserDataRoot string
}

// Roots はroot決定の結果である。
//
// pathはすべてrole付きで返す。role未定義のpathを公開境界へ出さないという
// docs/04-storage-and-data.md §17.2の要求を、戻り値の型で守る。
type Roots struct {
	Mode ModeDecisionRoots
	// DataRoot はtool、state、cacheを置くrootである。
	DataRoot domain.PathValue
	// DistributionRoot はclient、registry、任意configを置くrootである。
	DistributionRoot domain.PathValue
	// ConfigFile はglobal `gdtvm.toml`の位置である。存在は保証しない。
	ConfigFile domain.PathValue
}

// ModeDecisionRoots はRoots内でmodeとその由来を保持する。
type ModeDecisionRoots struct {
	Mode domain.Mode
	// HomeOverridden は`--home`でdata rootを上書きしたかを表す。
	//
	// trueのとき、永続shell/PATH変更とshimの永続作成を行ってはならない
	// （docs/04-storage-and-data.md §1・§1.2）。
	HomeOverridden bool
}

// windowsUserDataDir はWindowsのLocalAppData直下に置くdirectory名である（§1.2）。
const windowsUserDataDir = "gdtvm"

// linuxUserDataDir はLinuxのhome直下に置く相対pathである（§1.2）。
const linuxUserDataDir = ".local/share/gdtvm"

// ConfigFileName はglobal設定fileの名前である（docs/05-configuration.md §2）。
const ConfigFileName = "gdtvm.toml"

// DecideRoots はdocs/04-storage-and-data.md §1.1・§1.2に従いrootを決める。
//
// distribution rootはmodeによらず`gdtvm[.exe]`が存在するdirectoryとする。
// docs/05-configuration.md §2が「active distribution rootの`gdtvm[.exe]`と同じ
// directoryにある`gdtvm.toml`だけをglobal設定として読む」と定めており、
// bootstrapはそのdirectoryが`<data-root>/distribution/current`になるように
// 配置する。実行中のbinaryの位置を正としない実装にすると、どのbinaryが動いて
// いるかとどのconfigを読むかがずれる。
func DecideRoots(req RootRequest) (Roots, *domain.Error) {
	if req.ExecutableDir == "" {
		return Roots{}, usageError("config.executable_dir_missing", nil)
	}
	if req.Host.IsZero() {
		return Roots{}, usageError("config.host_platform_missing", nil)
	}

	distribution := req.ExecutableDir
	separator := pathSeparator(req.Host)

	dataRoot, err := decideDataRoot(req, separator, distribution)
	if err != nil {
		return Roots{}, err
	}

	return newRoots(req, dataRoot, distribution, separator)
}

func decideDataRoot(req RootRequest, separator string, distribution string) (string, *domain.Error) {
	switch req.Mode {
	case domain.ModePortable:
		// §1「`portable`と`--home`は排他」。portableのdata rootはdistribution
		// rootそのものであり、上書きを許すとclientと管理領域が分離してしまう。
		if req.HomeOverride != "" {
			return "", usageError("config.home_with_portable", nil)
		}
		return distribution, nil

	case domain.ModeUser:
		if req.HomeOverride != "" {
			if err := validateHomeOverride(req.HomeOverride, distribution, separator); err != nil {
				return "", err
			}
			return req.HomeOverride, nil
		}
		if req.ConfiguredUserDataRoot != "" {
			if !isAbsolutePath(req.ConfiguredUserDataRoot, separator) {
				return "", configError("config.user_data_root_not_absolute", nil)
			}
			return req.ConfiguredUserDataRoot, nil
		}
		return osDefaultUserDataRoot(req, separator)

	default:
		return "", usageError("config.mode_invalid", map[string]string{
			"mode": string(req.Mode),
		})
	}
}

// osDefaultUserDataRoot は§1.2が定めるOS既定のuser data rootを返す。
func osDefaultUserDataRoot(req RootRequest, separator string) (string, *domain.Error) {
	if req.Host.OS() == "windows" {
		if req.User.AppDataLocal == "" {
			return "", filesystemError("config.local_app_data_unavailable", nil)
		}
		return joinPath(separator, req.User.AppDataLocal, windowsUserDataDir), nil
	}
	if req.User.Home == "" {
		return "", filesystemError("config.account_home_unavailable", nil)
	}
	return joinPath(separator, req.User.Home, linuxUserDataDir), nil
}

// validateHomeOverride は`--home`が受け入れ可能かを検査する（§1.2）。
//
// 同§は「既存または現在userが作成可能なabsolute directoryだけを許す。filesystem
// root、distribution rootそのもの、他user所有、network share、symlink/reparse
// loopを拒否する」と定める。このうちfilesystem操作を要しない3件をここで拒否し、
// owner、network share、reparse loopはP2-03のfilesystem検査が扱う。
func validateHomeOverride(home, distribution, separator string) *domain.Error {
	switch {
	case !isAbsolutePath(home, separator):
		return usageError("config.home_not_absolute", map[string]string{"home": home})
	case isFilesystemRoot(home, separator):
		return usageError("config.home_is_filesystem_root", map[string]string{"home": home})
	case samePath(home, distribution, separator):
		return usageError("config.home_is_distribution_root", map[string]string{"home": home})
	}
	return nil
}

func newRoots(req RootRequest, dataRoot, distribution, separator string) (Roots, *domain.Error) {
	data, err := newPath(domain.RoleDataRoot, dataRoot)
	if err != nil {
		return Roots{}, err
	}
	dist, err := newPath(domain.RoleDistributionRoot, distribution)
	if err != nil {
		return Roots{}, err
	}
	config, err := newPath(domain.RoleConfig, joinPath(separator, distribution, ConfigFileName))
	if err != nil {
		return Roots{}, err
	}
	return Roots{
		Mode: ModeDecisionRoots{
			Mode:           req.Mode,
			HomeOverridden: req.HomeOverride != "",
		},
		DataRoot:         data,
		DistributionRoot: dist,
		ConfigFile:       config,
	}, nil
}

func newPath(role domain.PathRole, path string) (domain.PathValue, *domain.Error) {
	value, err := domain.NewPathValue(role, path)
	if err != nil {
		return domain.PathValue{}, internalError(err)
	}
	return value, nil
}

// pathSeparator はhost OSのpath区切りを返す。
//
// runtime.GOOSではなく引数のplatformで決める。どちらのrunnerからでも両OSの規則を
// testできるようにするためである（CLAUDE.md §5「WindowsとLinuxを同時に開発する」）。
func pathSeparator(host domain.Platform) string {
	if host.OS() == "windows" {
		return `\`
	}
	return "/"
}

// joinPath はseparatorで要素を連結する。
//
// path/filepathはhostの区切りを使うため、Linux runnerからWindowsのpathを
// 組み立てられない。区切りを引数で受けることで両OS分を同じcodeで扱う。
func joinPath(separator string, parts ...string) string {
	normalized := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.Trim(strings.ReplaceAll(part, "/", separator), separator)
		if part != "" {
			normalized = append(normalized, part)
		}
	}
	joined := strings.Join(normalized, separator)
	if separator == "/" {
		return "/" + joined
	}
	return joined
}

// isAbsolutePath はseparatorに応じてabsolute pathかを判定する。
//
// Windowsは`C:\`のdrive付きと`\\server\share`のUNCを絶対とみなす。相対pathを
// 受け入れると、cwdによってrootが変わり再現性が失われる。
func isAbsolutePath(path, separator string) bool {
	if path == "" {
		return false
	}
	if separator == "/" {
		return strings.HasPrefix(path, "/")
	}
	if strings.HasPrefix(path, `\\`) {
		return true
	}
	return len(path) >= 3 && isDriveLetter(path[0]) && path[1] == ':' && path[2] == '\\'
}

func isDriveLetter(c byte) bool {
	return ('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z')
}

// isFilesystemRoot はpathがfilesystem rootそのものかを判定する（§1.1・§1.2）。
func isFilesystemRoot(path, separator string) bool {
	trimmed := strings.TrimRight(path, separator)
	if separator == "/" {
		return trimmed == ""
	}
	// `C:` まで削られたらdrive rootである。UNCの`\\server\share`は共有の
	// 入口であり、rootと同じ扱いにする。
	if len(trimmed) == 2 && isDriveLetter(trimmed[0]) && trimmed[1] == ':' {
		return true
	}
	if strings.HasPrefix(path, `\\`) {
		rest := strings.Trim(path[2:], `\`)
		return strings.Count(rest, `\`) <= 1
	}
	return false
}

// samePath は末尾区切りとcase規則を無視して同じpathかを判定する。
//
// Windowsのpathはcase insensitiveのため小文字化して比べる。Linuxはcase
// sensitiveなのでそのまま比べる。
func samePath(left, right, separator string) bool {
	left = strings.TrimRight(left, separator)
	right = strings.TrimRight(right, separator)
	if separator == `\` {
		return strings.EqualFold(left, right)
	}
	return left == right
}

func usageError(messageID string, params map[string]string) *domain.Error {
	return newTypedError(domain.CodeUsage, messageID, params)
}

func configError(messageID string, params map[string]string) *domain.Error {
	return newTypedError(domain.CodeConfigInvalid, messageID, params)
}

// projectError はproject fileに起因する失敗のtyped errorを作る。
//
// docs/03-cli.md §7はglobal設定の`E_CONFIG_INVALID`とproject設定の
// `E_PROJECT_CONFIG_INVALID`を別codeにしている。どちらのfileが悪いかを利用者が
// 区別できるようにするためであり、まとめない。
func projectError(messageID string, params map[string]string) *domain.Error {
	return newTypedError(domain.CodeProjectConfigInvalid, messageID, params)
}

func filesystemError(messageID string, params map[string]string) *domain.Error {
	return newTypedError(domain.CodeFilesystem, messageID, params)
}

func internalError(cause error) *domain.Error {
	typed := domain.Internal(cause)
	return typed
}

func newTypedError(code domain.ErrorCode, messageID string, params map[string]string) *domain.Error {
	id, err := domain.ParseMessageID(messageID)
	if err != nil {
		return domain.Internal(errors.New("config: message IDがgrammarに合わない"))
	}
	typed := &domain.Error{Code: code, MessageID: id, Operation: "initialize"}
	if len(params) > 0 {
		typed.Parameters = make(domain.Parameters, len(params))
		for key, value := range params {
			typed.Parameters[key] = domain.StringScalar(value)
		}
	}
	return typed
}

// isNotExist はerrorが「存在しない」を表すかを返す。
//
// port実装はfs.ErrNotExistを包んで返す契約のため（internal/domain/port）、
// errors.Isで判定する。
func isNotExist(err error) bool { return errors.Is(err, fs.ErrNotExist) }

// filesystemErrorWithCause はpath付きのfilesystem errorを作る。
//
// pathはrole付きで持つが、message parameterへは入れない。個人pathを公開境界へ
// 出さないためである（docs/10-security.md §9.2）。
func filesystemErrorWithCause(messageID, path string, cause error) *domain.Error {
	typed := newTypedError(domain.CodeFilesystem, messageID, nil)
	if value, err := domain.NewPathValue(domain.RoleProjectFile, path); err == nil {
		typed.PathRole = value.Role()
	}
	typed.Cause = cause
	return typed
}
