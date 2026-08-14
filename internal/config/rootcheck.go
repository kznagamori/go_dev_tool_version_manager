package config

import (
	"io/fs"
	"strings"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain/port"
)

// RootCheckRequest はrootのfilesystem安全検査の入力である。
//
// [DecideRoots]がpathを決めたあと、実際にそのrootを使う前に呼ぶ。決定と検査を
// 分けているのは、決定がfilesystemに触れず再現可能でなければならないためである
// （docs/13-progress.md P2-01・P2-03の分担）。
type RootCheckRequest struct {
	// Root は検査対象のrootである。role付きで受け取り、診断へroleを載せる。
	Root domain.PathValue
	// Host はpath規則を決めるplatformである。
	Host domain.Platform
	// User は現在の実userである。owner一致の判定に使う。
	User port.UserIdentity
	// FileSystem はstat、realpath、権限検査に使う。
	FileSystem port.FileSystem
	// UserLookup はowner識別に使う。
	UserLookup port.UserLookup
}

// RootCheckResult は検査を通ったrootの確定情報である。
type RootCheckResult struct {
	// Canonical はrealpathで解決したrootである。以降の書込みはこの値を基準に
	// containmentを判定する（docs/04-storage-and-data.md §6）。
	Canonical domain.PathValue
	// Existed は検査時点でrootが存在したかを表す。falseはsetupが作る対象である。
	Existed bool
}

// ancestorScanMax は親chain検査でさかのぼる最大段数である。
//
// symlink/reparse loopや異常に深いpathで無限loopにしないための打ち切りである。
// docs/04-storage-and-data.md §21のlogical path上限32 KiBに対し、component長の
// 下限が1 byteでも十分に余裕がある値を取る。
const ancestorScanMax = 4096

// CheckRoot はrootが管理領域として安全かを検査する。
//
// docs/09-platform.md §2.3が「filesystem root、network share、他user所有、
// world-writable parent、symlink/reparse loop、現在userが作成・renameできないrootを
// 拒否する」と定める。[DecideRoots]と[ParseGlobalConfig]がfilesystem操作なしで
// 判定した分（絶対path、filesystem root、distribution rootそのもの）に加えて、
// ここでは実体を見ないと分からない条件を検査する。
//
// rootが存在しない場合は、作成できる見込みがあるかを最寄りの既存祖先で判定する。
// setupはrootを作るのが仕事であり、存在しないことを理由に拒否すると初回setupが
// できなくなる。
func CheckRoot(req RootCheckRequest) (RootCheckResult, *domain.Error) {
	if req.Root.IsZero() || req.Root.Path() == "" {
		return RootCheckResult{}, usageError("config.root_path_missing", nil)
	}
	if req.Host.IsZero() {
		return RootCheckResult{}, usageError("config.host_platform_missing", nil)
	}
	if req.FileSystem == nil || req.UserLookup == nil {
		return RootCheckResult{}, usageError("config.root_check_ports_missing", nil)
	}

	separator := pathSeparator(req.Host)
	path := req.Root.Path()

	if isFilesystemRoot(path, separator) {
		return RootCheckResult{}, pathUnsafeError("config.root_is_filesystem_root", req.Root)
	}
	// UNC pathはnetwork shareである。§2.3が拒否対象に挙げており、
	// file identityとatomic replaceの保証が得られない。
	if separator == `\` && strings.HasPrefix(path, `\\`) {
		return RootCheckResult{}, pathUnsafeError("config.root_is_network_share", req.Root)
	}

	info, statErr := req.FileSystem.Stat(path)
	switch {
	case statErr == nil:
		return checkExistingRoot(req, info)
	case isNotExist(statErr):
		return checkCreatableRoot(req, separator)
	default:
		return RootCheckResult{}, filesystemErrorWithRole("config.root_stat_failed", req.Root, statErr)
	}
}

// checkExistingRoot は既存rootの種別、owner、permission、canonical pathを検査する。
func checkExistingRoot(req RootCheckRequest, info port.FileInfo) (RootCheckResult, *domain.Error) {
	if !info.IsDir {
		return RootCheckResult{}, pathUnsafeError("config.root_not_directory", req.Root)
	}
	// rootそのものがsymlink/reparse pointなら拒否する。解決先を後から差し替えると
	// containment検査を通したまま管理外へ書けるためである
	// （docs/10-security.md §6「各open/move/delete境界で再検査する」）。
	if info.IsSymlink {
		return RootCheckResult{}, pathUnsafeError("config.root_is_link", req.Root)
	}

	if err := checkOwnerAndMode(req, req.Root.Path(), info); err != nil {
		return RootCheckResult{}, err
	}

	canonical, realErr := req.FileSystem.RealPath(req.Root.Path())
	if realErr != nil {
		return RootCheckResult{}, filesystemErrorWithRole("config.root_realpath_failed", req.Root, realErr)
	}
	if err := checkAncestors(req, canonical); err != nil {
		return RootCheckResult{}, err
	}

	value, pathErr := newPath(req.Root.Role(), canonical)
	if pathErr != nil {
		return RootCheckResult{}, pathErr
	}
	return RootCheckResult{Canonical: value, Existed: true}, nil
}

// checkCreatableRoot はrootが未作成の場合に、最寄りの既存祖先が安全かを検査する。
func checkCreatableRoot(req RootCheckRequest, separator string) (RootCheckResult, *domain.Error) {
	current := parentDir(strings.TrimRight(req.Root.Path(), separator), separator)
	for depth := 0; depth < ancestorScanMax; depth++ {
		if current == "" {
			return RootCheckResult{}, pathUnsafeError("config.root_has_no_existing_ancestor", req.Root)
		}
		info, statErr := req.FileSystem.Stat(current)
		if statErr != nil {
			if isNotExist(statErr) {
				current = parentDir(current, separator)
				continue
			}
			return RootCheckResult{}, filesystemErrorWithRole("config.root_stat_failed", req.Root, statErr)
		}
		if !info.IsDir {
			return RootCheckResult{}, pathUnsafeError("config.root_parent_not_directory", req.Root)
		}
		// 最寄りの既存祖先が現在userのもので、他userが書けないなら、そこへrootを
		// 作れる見込みがあると判断する。実際の作成可否はsetupがmkdirで確かめる。
		if err := checkOwnerAndMode(req, current, info); err != nil {
			return RootCheckResult{}, err
		}
		if err := checkAncestors(req, current); err != nil {
			return RootCheckResult{}, err
		}
		value, pathErr := newPath(req.Root.Role(), req.Root.Path())
		if pathErr != nil {
			return RootCheckResult{}, pathErr
		}
		return RootCheckResult{Canonical: value, Existed: false}, nil
	}
	return RootCheckResult{}, pathUnsafeError("config.root_ancestor_scan_exhausted", req.Root)
}

// checkAncestors は既存親chainに不安全な要素が無いかを検査する（§2.3）。
//
// 上位directoryが他user所有やworld-writableだと、rootそのものを差し替えられる。
// rootだけを見ても安全とは言えない。
func checkAncestors(req RootCheckRequest, canonical string) *domain.Error {
	separator := pathSeparator(req.Host)
	current := parentDir(canonical, separator)

	for depth := 0; depth < ancestorScanMax; depth++ {
		if current == "" {
			return nil
		}
		info, statErr := req.FileSystem.Stat(current)
		if statErr != nil {
			if isNotExist(statErr) {
				return nil
			}
			return filesystemErrorWithRole("config.root_ancestor_stat_failed", req.Root, statErr)
		}
		if err := checkOwnerAndMode(req, current, info); err != nil {
			return err
		}
		next := parentDir(current, separator)
		if next == current {
			return nil
		}
		current = next
	}
	return pathUnsafeError("config.root_ancestor_scan_exhausted", req.Root)
}

// checkOwnerAndMode は1 directoryのownerとpermissionを検査する。
//
// docs/10-security.md §6は「Windowsは現在user所有かつ他user書込み不可のACL、
// Linuxは現在UID所有かつgroup/other書込み不可を基本とする。root、Administrator、
// SYSTEM所有の既存rootを一般userが黙って採用しない」と定める。
//
// mode bitの判定はLinuxだけで行う。WindowsのACLは`fs.FileMode`へ写らず、
// mode bitを見ても他user書込み可否を判定できない。Windowsでのworld-writable相当の
// 判定はACLを読むport実装の責務であり、ここではowner一致だけを見る。
func checkOwnerAndMode(req RootCheckRequest, path string, info port.FileInfo) *domain.Error {
	owner, ownerErr := req.UserLookup.OwnerOf(path)
	if ownerErr != nil {
		return filesystemErrorWithRole("config.root_owner_lookup_failed", req.Root, ownerErr)
	}
	if req.User.ID == "" {
		return usageError("config.current_user_unknown", nil)
	}
	if owner != req.User.ID {
		return permissionError("config.root_owned_by_other_user", req.Root)
	}
	if req.Host.OS() == "windows" {
		return nil
	}
	if info.Mode.Perm()&otherWritableMask != 0 {
		return permissionError("config.root_group_or_world_writable", req.Root)
	}
	return nil
}

// otherWritableMask はgroupとotherの書込みbitである。
//
// sticky bit付きの`/tmp`のような構成も安全とは扱わない。§6が「group/other書込み
// 不可を基本とする」と定めており、例外をここで作らない。
const otherWritableMask fs.FileMode = 0o022

func pathUnsafeError(messageID string, root domain.PathValue) *domain.Error {
	typed := newTypedError(domain.CodePathUnsafe, messageID, nil)
	typed.PathRole = root.Role()
	return typed
}

func permissionError(messageID string, root domain.PathValue) *domain.Error {
	typed := newTypedError(domain.CodePermission, messageID, nil)
	typed.PathRole = root.Role()
	return typed
}

func filesystemErrorWithRole(messageID string, root domain.PathValue, cause error) *domain.Error {
	typed := newTypedError(domain.CodeFilesystem, messageID, nil)
	typed.PathRole = root.Role()
	typed.Cause = cause
	return typed
}
