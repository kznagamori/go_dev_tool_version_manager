// Package shim はshim metadata生成、呼出名解決、実体委譲を担う。
//
// docs/02-architecture.md §2 の論理領域「internal/shim」に対応する。
//
// 依存範囲: domain、port、runtimeに依存する。許可するinternal importは
// scripts/ci/check_imports.py の表を正本とし、`policy` jobが検査する。
package shim

import (
	"errors"
	"fmt"
	"strings"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/store"
)

// ClientCommandName はCLIとして起動したときの呼出名である。
//
// docs/08-install-runtime.md §10「起動basenameが`gdtvm`ならCLI」。
const ClientCommandName = "gdtvm"

// ShimDirName はdata root直下のshim directory名である。
//
// docs/04-storage-and-data.md §11「shim pathはdata root相対`shims`固定」。
// 設定で変えられないため定数にする。
const ShimDirName = "shims"

// 呼出名解決のsentinel error。
var (
	// ErrUnknownCommand は呼出名がshim indexに無いことを表す。
	//
	// docs/09-platform.md §3.3「**unknown basenameをCLIとして実行しない**」。
	// CLIへ落とすと、利用者が意図しないcommandがgdtvm本体として動く。
	ErrUnknownCommand = errors.New("shim: 呼出名がshim indexに無い")
	// ErrAmbiguousCommand は呼出名が複数のtoolに対応することを表す。
	//
	// docs/08-install-runtime.md §10手順1「0件/複数件は失敗する」。indexは
	// name一意のはずだが、破損したindexを読んだ場合に片方を選んで進めない。
	ErrAmbiguousCommand = errors.New("shim: 呼出名が複数のtoolに対応する")
	// ErrInvalidCommandName は呼出名が正規化できないことを表す。
	ErrInvalidCommandName = errors.New("shim: 呼出名を正規化できない")
	// ErrNotShimPath は起動pathがshim directory配下でないことを表す。
	ErrNotShimPath = errors.New("shim: 起動pathがshim directory配下でない")
)

// Mode は起動がCLIとshimのどちらかである。
type Mode string

// Mode の値。
const (
	// ModeCLI は`gdtvm`として起動した場合である。
	ModeCLI Mode = "cli"
	// ModeShim は公開commandとして起動した場合である。
	ModeShim Mode = "shim"
)

// NormalizeCommandName は起動pathから呼出名を取り出して正規化する。
//
// docs/08-install-runtime.md §10手順1「呼出basenameを**厳密に**正規化して」。
//
// Windowsは`.exe`を1つだけ落とし、case-insensitiveに畳む。Linuxはbyte一致で
// 扱う。**OSの規則をどちらかへ寄せない** —— Windowsで`GO.EXE`と`go.exe`は同じ
// fileだが、Linuxで`GO`と`go`は別のfileである。片方の規則で両方を扱うと、
// 一方で別commandを同一視し、もう一方で同一commandを取り違える。
func NormalizeCommandName(argv0 string, host domain.Platform) (string, error) {
	if host.IsZero() {
		return "", fmt.Errorf("%w: host platformが未設定", ErrInvalidCommandName)
	}
	_, base := splitPath(strings.TrimSpace(argv0), host)
	if base == "" || base == "." || base == ".." {
		return "", fmt.Errorf(
			"%w: 起動pathからcommand名を取り出せない（argv0=%q）", ErrInvalidCommandName, argv0)
	}
	if host.OS() == domain.OSWindows {
		base = strings.ToLower(base)
		// suffixは1つだけ落とす。`go.exe.exe`はcommand名`go.exe`であり、
		// 繰り返し落とすと別のcommandになる。
		base = strings.TrimSuffix(base, strings.ToLower(host.ExecutableSuffix()))
	}
	if base == "" {
		return "", fmt.Errorf(
			"%w: suffixを除くと空になる（argv0=%q）", ErrInvalidCommandName, argv0)
	}
	// splitPathが取り切れなかった区切りが残っていれば、command名として扱わない。
	// 通常は起こらないが、区切りを含む名前でindexを引くと別entryへ当たりうる。
	if strings.ContainsAny(base, pathSeparators(host)) {
		return "", fmt.Errorf(
			"%w: 呼出名にpath区切りが残る（%q）", ErrInvalidCommandName, base)
	}
	return base, nil
}

// pathSeparators はhost platformがpath区切りとして受ける文字である。
//
// Windows APIは`\`と`/`のどちらも区切りとして受ける。drive指定の`:`も
// component境界であり、`C:go`はdrive相対pathでcommand名ではない。
func pathSeparators(host domain.Platform) string {
	if host.OS() == domain.OSWindows {
		return `/\:`
	}
	return "/"
}

// splitPath はhost platformの規則でpathをdirectoryとbasenameへ分ける。
//
// **[filepath.Dir]／[filepath.Base]を使わない。** どちらも実行中OSの区切りだけを
// 見るため、Linuxで動かすとWindowsのpath全体を1つの名前として扱う。shimは自分の
// host上で動くのでproductionでは一致するが、規則を実行環境に委ねると**両OS分の
// 規則を1か所で確かめられない**。host platformを受け取る関数が実行環境で
// 挙動を変えるのは、引数の意味とも食い違う。
func splitPath(path string, host domain.Platform) (string, string) {
	index := strings.LastIndexAny(path, pathSeparators(host))
	if index < 0 {
		return "", path
	}
	dir, base := path[:index], path[index+1:]
	// `/go`や`C:\go`のdirはseparatorを含んだ`/`・`C:\`である。落とすと
	// 相対pathという別の意味になる。
	if dir == "" || (host.OS() == domain.OSWindows && strings.HasSuffix(dir, ":")) {
		dir = path[:index+1]
	}
	return dir, base
}

// isPathRoot はそれ以上親を持たないpathかどうかを返す。
func isPathRoot(path string, host domain.Platform) bool {
	if path == "" {
		return true
	}
	_, base := splitPath(path, host)
	return base == ""
}

// ResolveMode は呼出名がCLIとshimのどちらかを返す。
//
// docs/08-install-runtime.md §10「起動basenameが`gdtvm`ならCLI、shim indexの
// 公開commandならCLI/network初期化前にruntimeへ分岐する」。
func ResolveMode(commandName string) Mode {
	if commandName == ClientCommandName {
		return ModeCLI
	}
	return ModeShim
}

// ResolveCommand は呼出名からshim indexの所有toolを1件決める。
//
// docs/08-install-runtime.md §10手順1「shim indexの所有toolを**1件**に決める。
// 0件/複数件は失敗する」。
//
// **見つからない呼出名をCLIとして扱わない**（docs/09-platform.md §3.3）。
// PATH探索もしない（同§10「targetを一般PATHから再探索せず」）。
func ResolveCommand(index store.ShimIndex, commandName string) (domain.ToolID, error) {
	var found domain.ToolID
	matches := 0
	for _, command := range index.Commands {
		if command.Name != commandName {
			continue
		}
		matches++
		if matches > 1 {
			return domain.ToolID{}, fmt.Errorf(
				"%w: %q（%d件）", ErrAmbiguousCommand, commandName, matches)
		}
		if command.ToolID.IsZero() {
			// codecを通していないindexである。所有toolを決められない状態で
			// 起動しない。
			return domain.ToolID{}, fmt.Errorf(
				"shim: shim indexのtool_idが空である（command=%q）", commandName)
		}
		found = command.ToolID
	}
	if matches == 0 {
		return domain.ToolID{}, fmt.Errorf("%w: %q", ErrUnknownCommand, commandName)
	}
	return found, nil
}

// Identity は起動したbinaryとその出所である。
//
// docs/02-architecture.md §9の`InvocationRequest`が「argv0、module path」を持つ
// のは、**両者が一致しない**ためである。Linuxのshimはclientへのrelative symlink
// であり、argv0はshim path、module pathは辿った先のclient本体を指す。Windowsの
// hardlinkでは同じfileの別名なので両者が一致する。
//
// どちらか一方だけでは足りない。argv0がなければどのcommandとして呼ばれたか
// 分からず、module pathがなければ動いているclientの同一性を確かめられない。
type Identity struct {
	// Mode はCLIとshimのどちらとして起動したかである。
	Mode Mode
	// CommandName は正規化した呼出名である。
	CommandName string
	// Argv0 は呼出しに使われたpathである。
	Argv0 string
	// ModulePath は実際に動いているbinaryのpathである。
	ModulePath string
	// ShimDir はargv0が置かれていたshim directoryである。ModeShimでだけ埋まる。
	ShimDir string
	// DataRoot はshim directoryの親である。ModeShimでだけ埋まる。
	//
	// docs/04-storage-and-data.md §11「shim pathはdata root相対`shims`固定」を
	// 逆に辿る。shimはこのrootの`state/shim-index.toml`を読む。
	DataRoot string
}

// Identify は起動pathと実体pathからIdentityを組み立てる。
//
// **network、prompt、filesystem探索を行わない純粋な計算である**
// （docs/02-architecture.md §9「Resolverはnetwork、prompt、repair、definition
// 再downloadを行わない」）。hot pathであり、実体の確認は呼出し側が行う。
//
// `argv0`がshim directory配下でない場合、ModeShimなら[ErrNotShimPath]を返す。
// data rootを決められないままstateを読みに行くと、別rootのstateを混ぜる
// （docs/09-platform.md §2.3「別rootのstate/linkを混在させない」）。
func Identify(argv0, modulePath string, host domain.Platform) (Identity, error) {
	commandName, err := NormalizeCommandName(argv0, host)
	if err != nil {
		return Identity{}, err
	}
	identity := Identity{
		Mode:        ResolveMode(commandName),
		CommandName: commandName,
		Argv0:       argv0,
		ModulePath:  modulePath,
	}
	if identity.Mode == ModeCLI {
		// CLIはdata rootをconfig locator（internal/config）が決める。shim path
		// から逆算しない —— `--home`やmodeでrootが変わるためである。
		return identity, nil
	}
	shimDir, _ := splitPath(argv0, host)
	dataRoot, shimBase := splitPath(shimDir, host)
	if !sameDirName(shimBase, ShimDirName, host) {
		return Identity{}, fmt.Errorf(
			"%w: %q の親が%qでない", ErrNotShimPath, argv0, ShimDirName)
	}
	if isPathRoot(dataRoot, host) {
		// `shims`がfilesystem root直下に置かれている。docs/09-platform.md §2.3は
		// filesystem root自体をdata rootとして拒否する。
		return Identity{}, fmt.Errorf(
			"%w: %q からdata rootを決められない（親がfilesystem rootである）",
			ErrNotShimPath, argv0)
	}
	identity.ShimDir = shimDir
	identity.DataRoot = dataRoot
	return identity, nil
}

// sameDirName はdirectory名がhost platformの規則で一致するかを返す。
//
// Windowsはcase-insensitiveであり、`SHIMS`と`shims`は同じdirectoryである。
// Linuxはbyte一致で扱う。
func sameDirName(left, right string, host domain.Platform) bool {
	if host.OS() == domain.OSWindows {
		return strings.EqualFold(left, right)
	}
	return left == right
}

// ShimFileName は公開commandのshim file名を返す。
//
// docs/09-platform.md §3.3「公開commandは`shims/<command>.exe`」、§5.1「公開
// command shimも`shims/`から」。suffixはplatformが決める。
func ShimFileName(commandName string, host domain.Platform) string {
	return commandName + host.ExecutableSuffix()
}
