package shim

import (
	"errors"
	"fmt"
	"io/fs"
	"sort"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain/port"
)

// shimDirPerm はshim directoryのpermissionである。
//
// docs/09-platform.md §2.3「他user所有、world-writable parentを拒否する」。
// 作る側もowner-onlyにする。
const shimDirPerm = 0o700

// Strategy はshim実体の作り方である。
//
// docs/04-storage-and-data.md §16の`shim_strategy`に対応する。`fallback-resolver`
// は**本packageでは扱わない** —— clientへresolver binaryを内蔵する二段構えbuildが
// 要り、そのrecipeはarchive生成（P11-04）と同じtaskで決まるためである
// （docs/13-progress.md P7-02の利用者判断）。
type Strategy string

// Strategy の値。
const (
	// StrategyHardlink はclientへのhardlinkである（Windows、docs/09-platform.md §3.3）。
	StrategyHardlink Strategy = "hardlink"
	// StrategySymlink はclientへのrelative symlinkである（Linux、同§5.1）。
	StrategySymlink Strategy = "symlink"
)

// deploy関連のsentinel error。
var (
	// ErrForeignShim はshim pathに想定外の実体があることを表す。
	ErrForeignShim = errors.New("shim: shim pathに想定外の実体がある")
	// ErrStrategyUnavailable は選んだ方式をそのfilesystemで使えないことを表す。
	ErrStrategyUnavailable = errors.New("shim: この方式でshimを作れない")
)

// Action は1件のshimに対して行った（または行う）操作である。
type Action string

// Action の値。
const (
	// ActionNoop は既に一致しており何もしなかったことを表す。
	//
	// docs/09-platform.md §7「`setup`は冪等とする。…既に一致する項目をno-opとして
	// 報告する」。
	ActionNoop Action = "noop"
	// ActionCreate は新しく作ったことを表す。
	ActionCreate Action = "create"
	// ActionReplace は古いshimを作り直したことを表す。
	ActionReplace Action = "replace"
)

// Result は1件のshimの配置結果である。
type Result struct {
	// CommandName は公開command名である。
	CommandName string
	// Path はshim fileのpathである。
	Path string
	// Action は行った操作である。
	Action Action
}

// DeployRequest はshim配置の入力である。
type DeployRequest struct {
	// ShimDir はshim directoryのabsolute pathである。
	ShimDir string
	// ClientPath はshimの実体となるclient executableのabsolute pathである。
	ClientPath string
	// Commands は配置する公開command名である。順序は問わない。
	Commands []string
	// Host はhost platformである。
	Host domain.Platform
	// Strategy はshim実体の作り方である。
	Strategy Strategy
}

// Deployer は公開command shimを配置する。
//
// **tool executableをcopyしない**（docs/09-platform.md §3.3・§5.1）。shimは常に
// clientへのlinkであり、tool本体でもclientの複製でもない。
type Deployer struct {
	fs    port.FileSystem
	links port.LinkManager
}

// NewDeployer はDeployerを組み立てる。
func NewDeployer(filesystem port.FileSystem, links port.LinkManager) (*Deployer, error) {
	switch {
	case filesystem == nil:
		return nil, errors.New("shim: FileSystem portが未設定")
	case links == nil:
		return nil, errors.New("shim: LinkManager portが未設定")
	}
	return &Deployer{fs: filesystem, links: links}, nil
}

// Deploy は公開command shimを配置し、command名順の結果を返す。
//
// **冪等である**（docs/09-platform.md §7）。既にclientを指すshimがあればno-opと
// して報告し、作り直さない。作り直すと、実行中のshimが指すfileを差し替える。
//
// **想定外の実体を黙って置き換えない。** shim pathに通常のfileやdirectoryが
// あった場合は[ErrForeignShim]で止める。利用者が置いたものを消しうるためで、
// 同§3.2が「自動置換せず`doctor`診断とする」と定めるのと同じ扱いである。
func (d *Deployer) Deploy(req DeployRequest) ([]Result, error) {
	if err := req.validate(); err != nil {
		return nil, err
	}
	if err := d.fs.MkdirAll(req.ShimDir, shimDirPerm); err != nil {
		return nil, fmt.Errorf("shim: shim directoryを作れない: %w", err)
	}

	// command名順に処理する。**結果の順序を入力順に依存させない** —— setupの
	// 報告と`doctor`の出力が実行ごとに入れ替わると、利用者が差分を読めない。
	commands := append([]string(nil), req.Commands...)
	sort.Strings(commands)

	results := make([]Result, 0, len(commands))
	for _, command := range commands {
		result, err := d.deployOne(req, command)
		if err != nil {
			// 途中で止める。残りを続けると、失敗したcommandだけが古いまま
			// PATHに載った状態になる。
			return results, err
		}
		results = append(results, result)
	}
	return results, nil
}

// deployOne は1件のshimを配置する。
func (d *Deployer) deployOne(req DeployRequest, command string) (Result, error) {
	path := joinPath(req.ShimDir, ShimFileName(command, req.Host), req.Host)
	result := Result{CommandName: command, Path: path}

	existing, err := d.inspect(path, req)
	if err != nil {
		return result, err
	}
	if existing == ActionNoop {
		result.Action = ActionNoop
		return result, nil
	}
	if existing == ActionReplace {
		// 既存linkを外してから作る。上書きで作れる保証がOSごとに違うため、
		// 「外す→作る」の順に固定する（docs/09-platform.md §3.2と同じ形）。
		if err := d.links.RemoveLink(path); err != nil {
			return result, fmt.Errorf("shim: 古いshimを外せない（%s）: %w", path, err)
		}
	}
	if err := d.create(path, req); err != nil {
		return result, err
	}
	result.Action = existing
	return result, nil
}

// inspect はshim pathの現状から行うべき操作を決める。
//
// 戻り値はActionCreate（無い）、ActionNoop（既に一致）、ActionReplace（linkだが
// 別のtargetを指す）のいずれかである。
func (d *Deployer) inspect(path string, req DeployRequest) (Action, error) {
	kind, err := d.links.Kind(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return ActionCreate, nil
		}
		// 種別を判定できないpathを置き換えない。未知のreparse pointが該当し、
		// 判定できないまま消すと利用者の実体を壊す。
		return "", fmt.Errorf("shim: shim pathの種別を判定できない（%s）: %w", path, err)
	}

	wantKind := port.LinkHardlink
	if req.Strategy == StrategySymlink {
		wantKind = port.LinkSymlink
	}
	if kind == port.LinkNone {
		// 通常のfileまたはdirectoryである。利用者が置いたものを消さない。
		return "", fmt.Errorf("%w: %s はlinkではない", ErrForeignShim, path)
	}
	if kind != wantKind {
		// 方式が変わった場合（setup再実行でstrategyが変わる）は作り直す。
		// link同士の置換であり、実体を消さない。
		return ActionReplace, nil
	}
	same, err := d.pointsAtClient(path, req)
	if err != nil {
		return "", err
	}
	if same {
		return ActionNoop, nil
	}
	return ActionReplace, nil
}

// pointsAtClient は既存shimがclientを指しているかを返す。
func (d *Deployer) pointsAtClient(path string, req DeployRequest) (bool, error) {
	if req.Strategy == StrategySymlink {
		target, err := d.links.ReadLink(path)
		if err != nil {
			return false, fmt.Errorf("shim: shimのtargetを読めない（%s）: %w", path, err)
		}
		// 保存値はrelativeである（docs/09-platform.md §5.1）。shim directoryを
		// 基準に解決してから比べる。絶対化せずに比べると、正しいshimを毎回
		// 作り直す。
		shimDir, _ := splitPath(path, req.Host)
		resolved := cleanPath(joinPath(shimDir, target, req.Host), req.Host)
		return resolved == cleanPath(req.ClientPath, req.Host), nil
	}
	// hardlinkはtargetを保存しない。同じ実体かどうかはfile identityで比べる。
	return d.sameFile(path, req.ClientPath)
}

// sameFile は2つのpathが同じ実体かどうかを返す。
//
// hardlinkは「同じfileの別名」であり、辿るべきtargetを持たない。
// [port.FileSystem.RealPath]でcanonical pathへ揃えて比べる。
func (d *Deployer) sameFile(left, right string) (bool, error) {
	leftReal, err := d.fs.RealPath(left)
	if err != nil {
		return false, fmt.Errorf("shim: shimのcanonical pathを取れない（%s）: %w", left, err)
	}
	rightReal, err := d.fs.RealPath(right)
	if err != nil {
		return false, fmt.Errorf("shim: clientのcanonical pathを取れない（%s）: %w", right, err)
	}
	if leftReal == rightReal {
		// 同じpathを2回見ただけである。hardlinkはcanonical pathが別のままなので、
		// これに当たるのはshim pathがclient自身のときだけである。
		return true, nil
	}
	leftInfo, err := d.fs.Stat(left)
	if err != nil {
		return false, fmt.Errorf("shim: shimを読めない（%s）: %w", left, err)
	}
	rightInfo, err := d.fs.Stat(right)
	if err != nil {
		return false, fmt.Errorf("shim: clientを読めない（%s）: %w", right, err)
	}
	// [port.FileInfo]はinode/file indexを持たない。size・mode・mtimeが揃えば
	// 同じ実体と見なす。**取り違えても不一致側へ倒れる** —— 一致と誤れば
	// 作り直しを1回省くだけだが、不一致と誤ってもlinkを張り直すだけで済む。
	return leftInfo.Size == rightInfo.Size &&
		leftInfo.Mode == rightInfo.Mode &&
		leftInfo.ModTime.Equal(rightInfo.ModTime), nil
}

// create はshim実体を作る。
func (d *Deployer) create(path string, req DeployRequest) error {
	switch req.Strategy {
	case StrategyHardlink:
		if err := d.links.CreateHardlink(path, req.ClientPath); err != nil {
			return fmt.Errorf("%w: hardlink（%s）: %w", ErrStrategyUnavailable, path, err)
		}
	case StrategySymlink:
		// relativeで作る。root全体を移してもshimが壊れないようにするため
		// である（docs/09-platform.md §5.1）。
		if err := d.links.CreateSymlink(path, req.ClientPath, true); err != nil {
			return fmt.Errorf("%w: symlink（%s）: %w", ErrStrategyUnavailable, path, err)
		}
	default:
		return fmt.Errorf("shim: 未知のstrategy %q", req.Strategy)
	}
	return nil
}

// validate はDeploy要求の前提を確かめる。
func (r DeployRequest) validate() error {
	switch {
	case r.Host.IsZero():
		return errors.New("shim: host platformが未設定")
	case !isAbsolutePath(r.ShimDir, r.Host):
		return fmt.Errorf("shim: shim directoryがabsoluteでない（%q）", r.ShimDir)
	case !isAbsolutePath(r.ClientPath, r.Host):
		return fmt.Errorf("shim: client pathがabsoluteでない（%q）", r.ClientPath)
	case r.Strategy != StrategyHardlink && r.Strategy != StrategySymlink:
		return fmt.Errorf("shim: strategyが%s|%sでない（%q）",
			StrategyHardlink, StrategySymlink, r.Strategy)
	}
	// platformが定める方式と食い違う組合せを通さない。docs/04-storage-and-data.md
	// §16はWindowsを`hardlink|fallback-resolver`、Linuxを`symlink|fallback-resolver`
	// と定める。
	if r.Host.OS() == domain.OSWindows && r.Strategy == StrategySymlink {
		return fmt.Errorf("shim: Windowsのshim方式にsymlinkを選べない")
	}
	if r.Host.OS() == domain.OSLinux && r.Strategy == StrategyHardlink {
		return fmt.Errorf("shim: Linuxのshim方式にhardlinkを選べない")
	}
	seen := make(map[string]bool, len(r.Commands))
	for _, command := range r.Commands {
		if command == "" {
			return errors.New("shim: 空のcommand名がある")
		}
		if seen[command] {
			// 同じcommandを2回作ると、2回目が1回目を置き換える。宣言の誤りを
			// 黙って畳まない。
			return fmt.Errorf("shim: command名が重複している（%q）", command)
		}
		seen[command] = true
	}
	return nil
}
