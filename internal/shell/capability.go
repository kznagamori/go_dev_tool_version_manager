// Package shell はsetup、profile marker、undoを担う。
//
// docs/02-architecture.md §2 の論理領域「internal/shell」に対応する。
//
// 依存範囲: domain、port、storeに依存する。system環境変数とHKLMを変更しない（§8）。
// 許可するinternal importは scripts/ci/check_imports.py の表を正本とし、
// `policy` jobが検査する。
package shell

import (
	"errors"
	"fmt"
	"sort"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain/port"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/store"
)

// capabilityProbePrefix はprobeが作る一時entryのprefixである。
//
// gdtvmが作ったものだと分かる名前にする。probeが中断で残った場合、利用者と
// `doctor`が由来を判別できる必要がある。
const capabilityProbePrefix = ".gdtvm-fsprobe-"

// probeDirPerm はprobeが作るdirectoryのpermissionである。
//
// docs/09-platform.md §2.3「他user所有、world-writable parentを拒否する」。
// 作る側もowner-onlyにする。
const probeDirPerm = 0o700

// probeFilePerm はprobeが作るfileのpermissionである。
const probeFilePerm = 0o600

// capability probeのsentinel error。
var (
	// ErrCapabilityProbe はprobeそのものを実施できないことを表す。
	//
	// **「能力が無い」と「確かめられなかった」を混ぜない。** 前者はfilesystemを
	// 変えれば解決し、後者は権限やdiskの問題である。利用者が取る行動が違う。
	ErrCapabilityProbe = errors.New("shell: filesystem能力を検査できない")
)

// requiredCapabilities はplatformごとの必須capabilityである。
//
// docs/04-storage-and-data.md §16「Windowsはcapabilityに`atomic-replace|
// directory-rename|file-identity|owner-enforcement|junction`を必須とし…Linuxは
// `atomic-replace|directory-rename|file-identity|owner-enforcement|symlink`を
// 必須とする」。表で閉じ、実行時に合成しない。
var requiredCapabilities = map[domain.OS][]store.FilesystemCapability{
	domain.OSWindows: {
		store.CapabilityAtomicReplace,
		store.CapabilityDirectoryRename,
		store.CapabilityFileIdentity,
		store.CapabilityOwnerEnforce,
		store.CapabilityJunction,
	},
	domain.OSLinux: {
		store.CapabilityAtomicReplace,
		store.CapabilityDirectoryRename,
		store.CapabilityFileIdentity,
		store.CapabilityOwnerEnforce,
		store.CapabilitySymlink,
	},
}

// CapabilityProber は対象directoryのfilesystem能力を実測する。
//
// **portへ能力検査の操作を足していない。** docs/02-architecture.md §4.1が
// 「能力検査」を挙げるのは`LinkManager`だけであり、`FileSystem`の操作一覧には
// 無い。残る4値は既存操作（`AtomicWrite`／`Rename`／`RealPath`／`OwnerOf`）で
// 確かめられるため、§4「効果がすべて既存portの背後へ閉じているorchestrationは
// portにしない」に当たる。
type CapabilityProber struct {
	fs    port.FileSystem
	links port.LinkManager
	users port.UserLookup
}

// NewCapabilityProber はCapabilityProberを組み立てる。
func NewCapabilityProber(
	filesystem port.FileSystem, links port.LinkManager, users port.UserLookup,
) (*CapabilityProber, error) {
	switch {
	case filesystem == nil:
		return nil, errors.New("shell: FileSystem portが未設定")
	case links == nil:
		return nil, errors.New("shell: LinkManager portが未設定")
	case users == nil:
		return nil, errors.New("shell: UserLookup portが未設定")
	}
	return &CapabilityProber{fs: filesystem, links: links, users: users}, nil
}

// Probe はdirectoryで確認できたcapabilityをASCII byte順・重複なしで返す。
//
// docs/04-storage-and-data.md §16「`filesystem_capabilities`は§17.1の値を
// ASCII byte順・重複なしで1～7件」。
//
// **filesystem種別名から推測しない**（docs/09-platform.md §3.1「必須能力が
// 欠ける場合はsetup probeで理由付き拒否する」）。ReFS/FAT/network shareで
// 何が欠けるかは種別名からは決まらず、権限やmount optionでも変わる。
//
// probeが作った実体はすべて消す。残すとdata rootに素性の分からないentryが残る。
func (p *CapabilityProber) Probe(dir string, host domain.Platform) ([]store.FilesystemCapability, error) {
	if host.IsZero() {
		return nil, fmt.Errorf("%w: host platformが未設定", ErrCapabilityProbe)
	}
	root, err := p.makeProbeRoot(dir)
	if err != nil {
		return nil, err
	}
	// 後始末は作成の逆順に行う。**RemoveAllを使わない** —— junctionをdirectory
	// として辿り、target側の内容を消しうる（docs/09-platform.md §3.2
	// 「junction targetを再帰削除しない」）。
	defer p.cleanup(root)

	found := make(map[store.FilesystemCapability]bool, store.FilesystemCapabilityCount)
	found[store.CapabilityAtomicReplace] = p.probeAtomicReplace(root)
	found[store.CapabilityDirectoryRename] = p.probeDirectoryRename(root)
	found[store.CapabilityFileIdentity] = p.probeFileIdentity(root)

	owner, err := p.probeOwnerEnforcement(dir)
	if err != nil {
		// 所有者を取得できないこと自体はprobeの失敗である。能力が無いと
		// 断定しない。
		return nil, err
	}
	found[store.CapabilityOwnerEnforce] = owner

	caps, err := p.links.Capabilities(dir)
	if err != nil {
		return nil, fmt.Errorf("%w: link能力を取得できない: %w", ErrCapabilityProbe, err)
	}
	found[store.CapabilityJunction] = caps.Junction
	found[store.CapabilitySymlink] = caps.Symlink
	found[store.CapabilityHardlink] = caps.Hardlink

	return sortedCapabilities(found), nil
}

// makeProbeRoot はprobe用のdirectoryを作る。
func (p *CapabilityProber) makeProbeRoot(dir string) (string, error) {
	if dir == "" {
		return "", fmt.Errorf("%w: 対象directoryが空である", ErrCapabilityProbe)
	}
	root := dir + "/" + capabilityProbePrefix + "root"
	if err := p.fs.MkdirAll(root, probeDirPerm); err != nil {
		return "", fmt.Errorf("%w: probe directoryを作れない: %w", ErrCapabilityProbe, err)
	}
	return root, nil
}

// cleanup はprobeが作った実体を消す。
func (p *CapabilityProber) cleanup(root string) {
	// Walkで拾って1件ずつ消す。作った名前を列挙するより、途中失敗で残った
	// 実体も拾える。
	var entries []string
	_ = p.fs.Walk(root, func(path string, info port.FileInfo) error {
		if path != root {
			entries = append(entries, path)
		}
		return nil
	})
	// 深い順に消す。directoryは空でなければ消せない。
	sort.Sort(sort.Reverse(sort.StringSlice(entries)))
	for _, entry := range entries {
		_ = p.fs.Remove(entry)
	}
	_ = p.fs.Remove(root)
}

// probeAtomicReplace は既存fileを丸ごと置き換えられるかを返す。
//
// docs/04-storage-and-data.md §4のatomic writeは「中断時にpathが旧内容のまま
// 残るか、まったく存在しないかのどちらか」を要求する。実装はtemp書込みと
// renameで成り立つため、**既存fileへの置換が成功することが前提能力**である。
func (p *CapabilityProber) probeAtomicReplace(root string) bool {
	target := root + "/atomic"
	if err := p.fs.AtomicWrite(target, []byte("before"), probeFilePerm); err != nil {
		return false
	}
	// 内容の長さが変わる置換にする。同じ長さだと、部分書込みでも成功して
	// 見えることがある。
	if err := p.fs.AtomicWrite(target, []byte("after-replacement"), probeFilePerm); err != nil {
		return false
	}
	data, err := p.fs.ReadFile(target, int64(len("after-replacement"))+1)
	if err != nil {
		return false
	}
	return string(data) == "after-replacement"
}

// probeDirectoryRename は中身のあるdirectoryをrenameできるかを返す。
//
// docs/08-install-runtime.md §7手順7のcommitがversion directoryのatomic rename
// に依存する。**空のdirectoryで確かめない** —— 空だけ通るfilesystemがある。
func (p *CapabilityProber) probeDirectoryRename(root string) bool {
	source := root + "/rename-src"
	if err := p.fs.MkdirAll(source, probeDirPerm); err != nil {
		return false
	}
	if err := p.fs.AtomicWrite(source+"/payload", []byte("payload"), probeFilePerm); err != nil {
		return false
	}
	destination := root + "/rename-dst"
	if err := p.fs.Rename(source, destination); err != nil {
		return false
	}
	info, err := p.fs.Stat(destination + "/payload")
	return err == nil && !info.IsDir
}

// probeFileIdentity は2つのpathが同じfileかどうかを区別できるかを返す。
//
// docs/09-platform.md §2.3「data rootとdistribution rootのidentityをstateへ
// 保存し、別rootのstate/linkを混在させない」。identityを区別できないfilesystem
// では、別rootのstateを同じものと見なす。
func (p *CapabilityProber) probeFileIdentity(root string) bool {
	left := root + "/identity-a"
	right := root + "/identity-b"
	if err := p.fs.AtomicWrite(left, []byte("a"), probeFilePerm); err != nil {
		return false
	}
	if err := p.fs.AtomicWrite(right, []byte("b"), probeFilePerm); err != nil {
		return false
	}
	leftReal, err := p.fs.RealPath(left)
	if err != nil {
		return false
	}
	rightReal, err := p.fs.RealPath(right)
	if err != nil {
		return false
	}
	if leftReal == rightReal {
		// 別のfileが同じcanonical pathになる。case foldingや8.3名の畳み込みで
		// 起こりうる。区別できない。
		return false
	}
	// 同じfileを2度解決したら同じ結果になること。揺れるならidentityとして
	// 使えない。
	leftAgain, err := p.fs.RealPath(left)
	return err == nil && leftAgain == leftReal
}

// probeOwnerEnforcement は対象directoryが現在userの所有かを返す。
//
// docs/09-platform.md §2.3「filesystem root、network share、他user所有、
// world-writable parent…を拒否する」。所有者を持たないfilesystem（FAT等）では
// [port.UserLookup.OwnerOf]が空を返し、所有を根拠にできない。
func (p *CapabilityProber) probeOwnerEnforcement(dir string) (bool, error) {
	current, err := p.users.Current()
	if err != nil {
		return false, fmt.Errorf("%w: 現在userを取得できない: %w", ErrCapabilityProbe, err)
	}
	owner, err := p.users.OwnerOf(dir)
	if err != nil {
		return false, fmt.Errorf("%w: 所有者を取得できない: %w", ErrCapabilityProbe, err)
	}
	if owner == "" || current.ID == "" {
		// 所有者の概念が無い。**「一致した」と見なさない。**
		return false, nil
	}
	return owner == current.ID, nil
}

// sortedCapabilities は確認できたcapabilityをASCII byte順で返す。
func sortedCapabilities(found map[store.FilesystemCapability]bool) []store.FilesystemCapability {
	result := make([]store.FilesystemCapability, 0, len(found))
	for capability, ok := range found {
		if ok {
			result = append(result, capability)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

// CheckRequired は必須capabilityが揃っているかを検査する。
//
// docs/04-storage-and-data.md §16「必須capabilityを確認できない場合はPlanを
// 作らず`E_PLATFORM_UNSUPPORTED`にする」。
//
// **足りないcapabilityを名指しする。** 「対象外」だけでは利用者が何を直せば
// よいか分からない（docs/09-platform.md §9「errorには…利用者が選べる安全な
// 代替を含める」）。
func CheckRequired(
	capabilities []store.FilesystemCapability, host domain.Platform,
) *domain.Error {
	required, ok := requiredCapabilities[host.OS()]
	if !ok {
		return &domain.Error{
			Code:  domain.CodePlatformUnsupported,
			Cause: fmt.Errorf("shell: OS %q に必須capabilityの定義が無い", host.OS()),
		}
	}
	present := make(map[store.FilesystemCapability]bool, len(capabilities))
	for _, capability := range capabilities {
		present[capability] = true
	}
	missing := make([]string, 0, len(required))
	for _, capability := range required {
		if !present[capability] {
			missing = append(missing, string(capability))
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return &domain.Error{
		Code: domain.CodePlatformUnsupported,
		Cause: fmt.Errorf(
			"shell: このfilesystemは%sに必須の能力を欠く（不足: %v）", host.OS(), missing),
	}
}
