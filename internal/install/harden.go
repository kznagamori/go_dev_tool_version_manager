package install

import (
	"errors"
	"fmt"
	"io/fs"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain/port"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/security"
)

// HardenPayload はpayload treeを通常利用でread/execute onlyへ正規化する。
//
// docs/08-install-runtime.md §7手順5「payloadを通常利用でread/execute onlyへ
// 正規化する。Windowsは現在userのwrite ACEを除き、Linuxはdirectory 0555、
// executable 0555、その他0444を基本とする」。
//
// **treeの走査をここで行い、entryごとの正規化を[port.FileSystem]へ委ねる。**
// docs/02-architecture.md §4「効果がすべて既存portの背後へ閉じているorchestration
// はportにしない」。走査までportへ入れると、どのentryをどの種別で正規化したのかを
// fakeで確かめられなくなる。OS固有の実現方法（Linuxのmode、Windowsのwrite ACE
// 除去）だけがadapterの責務である。
//
// **containment検査を各entryで行う。** 展開後のtreeを走査するため、symlinkが
// 混ざっていればpayload外を正規化しうる。展開側（[Extractor]）がsymlinkを拒否
// しているが、ここでも確かめる——正規化はpermissionを緩める方向にも使える
// operationであり、対象がpayload内であることを1箇所の検査に頼らない。
//
// docs/10-security.md §5「permission変更だけを完全な防御とみなさない」。
// これはpayload完全性のための一段であり、`command_targets`のdigest照合
// （§13の`doctor`）と併せて機能する。
func HardenPayload(filesystem port.FileSystem, payload domain.PathValue, host domain.Platform) error {
	switch {
	case filesystem == nil:
		return errors.New("install: FileSystem portが未設定")
	case payload.IsZero() || payload.Path() == "":
		return errors.New("install: payload directoryが未設定")
	case payload.Role() != domain.RolePayload:
		return fmt.Errorf("install: payload directoryのroleが%sである", payload.Role())
	case host.IsZero():
		return errors.New("install: host platformが未設定")
	}

	root := payload.Path()
	walkErr := filesystem.Walk(root, func(path string, info port.FileInfo) error {
		// root自身も対象にする。payload directoryが書込み可能なままだと、
		// 中身がread onlyでもentryの追加・削除ができる。
		if path != root && !security.IsContained(root, path, host) {
			return fmt.Errorf("install: payload外のentry %q が現れた", path)
		}
		kind, kindErr := permissionKindOf(info)
		if kindErr != nil {
			return fmt.Errorf("install: %q: %w", path, kindErr)
		}
		if err := filesystem.HardenReadExecute(path, kind); err != nil {
			return fmt.Errorf("install: %q のpermissionを正規化できない: %w", path, err)
		}
		return nil
	})
	if walkErr != nil {
		return walkErr
	}
	return nil
}

// permissionKindOf はentryの正規化種別を決める。
//
// §6「Linux executableのowner executeを保持しsetuid/setgidを除去する」。展開時に
// 保持したowner executeが、ここでの`executable`判定の根拠になる。
//
// **symlinkとその他の特殊entryを拒否する。** §6が展開時にこれらを拒否しており、
// payload内に存在しないはずである。現れた場合は展開後に差し込まれたことを意味し、
// permissionを正規化して先へ進むより失敗させるほうが安全である。
func permissionKindOf(info port.FileInfo) (port.PermissionKind, error) {
	switch {
	case info.IsSymlink:
		// [port.FileInfo.IsSymlink]はWindowsのreparse pointも含む。mode bitで
		// 見るとreparse pointを取りこぼす。
		return 0, errors.New("payload内にsymlink/reparse pointがある")
	case info.Mode&fs.ModeType != 0 && !info.IsDir:
		// device、FIFO、socket等。
		return 0, fmt.Errorf("payload内に通常fileでないentryがある（mode %v）", info.Mode)
	case info.IsDir:
		return port.PermissionDirectory, nil
	case info.Mode&ownerExecute != 0:
		return port.PermissionExecutable, nil
	default:
		return port.PermissionRegular, nil
	}
}

// ownerExecute はowner execute bitである。
//
// §6が保持を求めるのはownerのexecuteであり、group/otherのbitでは判定しない。
const ownerExecute fs.FileMode = 0o100
