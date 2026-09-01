// Package platform のlink adapterである。
package platform

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain/port"
)

// linkProbePrefix は[LinkManager.Capabilities]が作るprobe directoryのprefixである。
//
// gdtvmが作ったものだと分かる名前にしている。probeが中断で残った場合、利用者と
// `doctor`が由来を判別できる必要がある。
const linkProbePrefix = "gdtvm-linkprobe-"

// linkのproduction adapterが返すsentinel error。
var (
	// ErrLinkUnsupported は対象OSまたはfilesystemがそのlink種別を作れないことを表す。
	ErrLinkUnsupported = errors.New("platform: このlink種別を作成できない")
	// ErrNotLink は対象が存在するがlinkではないことを表す。
	ErrNotLink = errors.New("platform: 対象がlinkではない")
	// ErrUnknownReparse は既知でないreparse pointを見つけたことを表す。
	//
	// docs/09-platform.md §3.2「別root reparse pointなら自動置換せず`doctor`診断
	// とする」。**`LinkNone`を返さない** —— 返すと呼出し側が通常のdirectoryと
	// 区別できず、置換してよいものとして扱ってしまう。
	ErrUnknownReparse = errors.New("platform: 未知のreparse pointである")
	// ErrPathNotAbsolute はpathがabsoluteでないことを表す。
	ErrPathNotAbsolute = errors.New("platform: pathがabsoluteでない")
	// ErrPathNotCanonical はpathが正規形でないことを表す。
	ErrPathNotCanonical = errors.New("platform: pathが正規形でない")
)

// LinkManager はOS APIを直接使う[port.LinkManager]実装である。
//
// docs/09-platform.md §3.2・§3.3・§5.1のlink操作をここで満たす。junctionと
// symlinkの作成・種別判定・target読取り・除去だけを持ち、**どのpathが妥当かの
// 判断は持たない**。data root内かどうかはrootを知るApplication側の検査であり、
// adapterはrootを知らない（docs/02-architecture.md §1）。
//
// 状態を持たないため、複数goroutineから同時に使ってよい。
type LinkManager struct{}

var _ port.LinkManager = (*LinkManager)(nil)

// NewLinkManager はproduction LinkManagerを作る。
func NewLinkManager() *LinkManager { return &LinkManager{} }

// Capabilities はdir上で実際に作成できるlink種別を返す。
//
// **filesystem種別名から推測せず、作って消すprobeで判定する**
// （docs/09-platform.md §3.1「必須能力が欠ける場合はsetup probeで理由付き拒否
// する」）。Windows標準userがsymlinkを作れるかはfilesystemではなくprivilegeと
// Developer Modeで決まるため、種別名からは分からない。
//
// probeが作った実体はすべて[os.Remove]で1件ずつ消す。**[os.RemoveAll]を使わない**
// —— junctionをdirectoryとして辿り、target側の内容を消しうる
// （同§3.2「junction targetを再帰削除しない」）。
func (m *LinkManager) Capabilities(dir string) (port.LinkCapabilities, error) {
	if err := requireSafeAbsolute("dir", dir); err != nil {
		return port.LinkCapabilities{}, err
	}
	root, err := os.MkdirTemp(dir, linkProbePrefix)
	if err != nil {
		return port.LinkCapabilities{}, fmt.Errorf(
			"platform: link能力のprobe directoryを作れない: %w", err)
	}
	// 後片付けは作成の逆順に行う。link自体を先に外さないと、targetを消す段階で
	// linkが宙に浮いた状態になる。
	defer func() { _ = os.Remove(root) }()

	targetDir := filepath.Join(root, "target")
	if err := os.Mkdir(targetDir, 0o700); err != nil {
		return port.LinkCapabilities{}, fmt.Errorf(
			"platform: link能力のprobe targetを作れない: %w", err)
	}
	defer func() { _ = os.Remove(targetDir) }()

	targetFile := filepath.Join(root, "target-file")
	if err := os.WriteFile(targetFile, []byte("gdtvm"), 0o600); err != nil {
		return port.LinkCapabilities{}, fmt.Errorf(
			"platform: link能力のprobe fileを作れない: %w", err)
	}
	defer func() { _ = os.Remove(targetFile) }()

	return port.LinkCapabilities{
		Junction: m.probe(filepath.Join(root, "junction"),
			func(p string) error { return m.CreateJunction(p, targetDir) }),
		Symlink: m.probe(filepath.Join(root, "symlink"),
			func(p string) error { return m.CreateSymlink(p, targetDir, true) }),
		Hardlink: m.probe(filepath.Join(root, "hardlink"),
			func(p string) error { return m.CreateHardlink(p, targetFile) }),
	}, nil
}

// probe は1件のlink種別を作って消し、成功したかを返す。
//
// **作成失敗の理由を区別しない。** [port.LinkCapabilities]は真偽値だけを持ち、
// 「権限が無い」と「filesystemが対応しない」を運べない。理由付きの拒否は
// setup（docs/09-platform.md §3.1）が自分で検査して報告する。
func (m *LinkManager) probe(linkPath string, create func(string) error) bool {
	if err := create(linkPath); err != nil {
		return false
	}
	// 作れたものは必ず消す。残すと次回のprobeが既存entryで失敗し、能力があるのに
	// 無いと判定される。
	_ = os.Remove(linkPath)
	return true
}

// CreateJunction はWindowsのdirectory junctionを作る。
//
// Linuxでは[ErrLinkUnsupported]を返す。docs/09-platform.md §5.1がLinuxの
// currentをrelative symlinkと定めており、junctionに相当する実体が無い。
func (m *LinkManager) CreateJunction(linkPath, targetDir string) error {
	if err := requireSafeAbsolute("linkPath", linkPath); err != nil {
		return err
	}
	if err := requireSafeAbsolute("targetDir", targetDir); err != nil {
		return err
	}
	// targetがdirectoryであることを先に確かめる。junctionはdirectoryにしか
	// 張れず、後から失敗すると空のlinkPathが残る。
	info, err := os.Lstat(targetDir)
	if err != nil {
		return fmt.Errorf("platform: junction targetを読めない: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("platform: junction targetがdirectoryでない: %s", targetDir)
	}
	return createJunction(linkPath, targetDir)
}

// CreateSymlink はsymbolic linkを作る。
//
// `relative`がtrueなら`linkPath`のdirectoryからの相対targetで作る
// （docs/09-platform.md §5.1「同じdata root内payloadへのrelative symlink」）。
// root全体を移動しても壊れないようにするためである。
//
// **targetは常にabsoluteで受け取る。** relative targetを受け取ると、何を基準と
// した相対なのかをadapterが決めることになり、暗黙のcurrent directory依存を
// 生む（docs/02-architecture.md §4）。相対形はここで計算する。
func (m *LinkManager) CreateSymlink(linkPath, target string, relative bool) error {
	if err := requireSafeAbsolute("linkPath", linkPath); err != nil {
		return err
	}
	if err := requireSafeAbsolute("target", target); err != nil {
		return err
	}
	stored := target
	if relative {
		rel, err := filepath.Rel(filepath.Dir(linkPath), target)
		if err != nil {
			// Windowsでvolumeが違うと相対形が存在しない。absoluteへ黙って
			// 落とすと、rootを移動した瞬間に壊れるlinkができる。
			return fmt.Errorf("platform: relative targetを計算できない: %w", err)
		}
		stored = rel
	}
	return os.Symlink(stored, linkPath)
}

// CreateHardlink はhard linkを作る。
//
// **targetがregular fileであることを[os.Lstat]で確かめる。** docs/09-platform.md
// §3.3はWindowsの公開command shimを「clientへのhardlink」と定めており、link先が
// さらにlinkだと、shimがどのexecutableを起動するかがそのlinkの状態で変わる。
func (m *LinkManager) CreateHardlink(linkPath, targetFile string) error {
	if err := requireSafeAbsolute("linkPath", linkPath); err != nil {
		return err
	}
	if err := requireSafeAbsolute("targetFile", targetFile); err != nil {
		return err
	}
	info, err := os.Lstat(targetFile)
	if err != nil {
		return fmt.Errorf("platform: hardlink targetを読めない: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf(
			"platform: hardlink targetがregular fileでない: %s (%s)", targetFile, info.Mode())
	}
	return os.Link(targetFile, linkPath)
}

// Kind はpath自体のlink種別を返す。辿った先ではなくpath自体を見る。
//
// reparse pointを先に判定し、次にlink countを見る。順序を逆にすると、Windowsで
// reparse point属性を持つdirectoryをlink countだけで通常entryと判定しうる。
func (m *LinkManager) Kind(path string) (port.LinkKind, error) {
	if err := requireSafeAbsolute("path", path); err != nil {
		return port.LinkNone, err
	}
	return linkKind(path)
}

// ReadLink はlinkが指すtargetを解決せずに返す。
//
// relative symlinkのtargetは保存されたまま返す。絶対化すると、保存値と一致するか
// を呼出し側が確かめられなくなる（docs/09-platform.md §5.1「absolute targetを
// 拒否する」の検査が成立しない）。
func (m *LinkManager) ReadLink(path string) (string, error) {
	if err := requireSafeAbsolute("path", path); err != nil {
		return "", err
	}
	return readLinkTarget(path)
}

// RemoveLink はlinkだけを外す。
//
// **link先の実体を消さない。** currentの張り替えでtool本体を消す事故を防ぐため、
// [os.Remove]だけを使い[os.RemoveAll]を使わない。junctionに対する
// [os.Remove]はreparse point自体を外し、target directoryへ触れない。
//
// linkでないpathは[ErrNotLink]で拒否する。呼出し側の取り違えで通常のdirectoryを
// 消さないためである。
func (m *LinkManager) RemoveLink(path string) error {
	if err := requireSafeAbsolute("path", path); err != nil {
		return err
	}
	kind, err := linkKind(path)
	if err != nil {
		return err
	}
	if kind == port.LinkNone {
		return fmt.Errorf("%w: %s", ErrNotLink, path)
	}
	return os.Remove(path)
}

// requireSafeAbsolute はOS APIへ渡す前にpathを検査する。
//
// absoluteでないpathは暗黙のcurrent directoryに依存する（docs/02-architecture.md
// §4「暗黙working directory…を使わない」）。正規形でないpathを黙って畳むと、
// 途中componentがlinkの場合に畳んだ結果が実体と食い違う。**どちらも直さずに
// 拒否する。**
func requireSafeAbsolute(name, p string) error {
	if p == "" {
		return fmt.Errorf("%w: %sが空である", ErrPathNotAbsolute, name)
	}
	if !filepath.IsAbs(p) {
		return fmt.Errorf("%w: %s=%q", ErrPathNotAbsolute, name, p)
	}
	if cleaned := filepath.Clean(p); cleaned != p {
		return fmt.Errorf("%w: %s=%q（正規形は%q）", ErrPathNotCanonical, name, p, cleaned)
	}
	return nil
}
