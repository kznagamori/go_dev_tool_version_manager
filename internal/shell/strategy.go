package shell

import (
	"errors"
	"fmt"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain/port"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/store"
)

// shimProbeName はshim方式のprobeが作る一時linkの名前である。
const shimProbeName = capabilityProbePrefix + "shim"

// currentLinkStrategies はplatformごとのcurrent link方式である。
//
// docs/04-storage-and-data.md §16「Windowsは…`current_link_strategy=junction`、
// Linuxは…`current_link_strategy=symlink`とする」。**選択肢ではなく固定**である。
var currentLinkStrategies = map[domain.OS]store.LinkStrategy{
	domain.OSWindows: store.LinkJunction,
	domain.OSLinux:   store.LinkSymlink,
}

// platformShimStrategies はplatformごとに使うshim方式である。
//
// docs/04-storage-and-data.md §16はWindowsを`hardlink|fallback-resolver`、
// Linuxを`symlink|fallback-resolver`と定める。**`fallback-resolver`は
// ここで選ばない** —— clientへresolver binaryを内蔵する二段構えbuildが要り、
// そのrecipeはarchive生成（P11-04）と同じtaskで決まる（P7-02着手時の利用者判断）。
var platformShimStrategies = map[domain.OS]store.ShimStrategy{
	domain.OSWindows: store.ShimHardlink,
	domain.OSLinux:   store.ShimSymlink,
}

// StrategyRequest はlink方式の決定に要る入力である。
type StrategyRequest struct {
	// ShimDir はshim directoryのabsolute pathである。
	ShimDir string
	// ClientPath はshimの実体となるclient executableのabsolute pathである。
	ClientPath string
	// Host はhost platformである。
	Host domain.Platform
}

// Strategies は決定したlink方式である。
type Strategies struct {
	// CurrentLink はcurrent linkの方式である。
	CurrentLink store.LinkStrategy
	// Shim はshim実体の方式である。
	Shim store.ShimStrategy
	// UsesHardlink はshimにhardlinkを使うかである。
	//
	// docs/04-storage-and-data.md §16「**hardlinkを使う場合だけ**capabilityへ
	// `hardlink`を含める」。capability listの組立てがこれを見る。
	UsesHardlink bool
}

// StrategyDecider はlink方式を実測して決める。
type StrategyDecider struct {
	fs    port.FileSystem
	links port.LinkManager
}

// NewStrategyDecider はStrategyDeciderを組み立てる。
func NewStrategyDecider(filesystem port.FileSystem, links port.LinkManager) (*StrategyDecider, error) {
	switch {
	case filesystem == nil:
		return nil, errors.New("shell: FileSystem portが未設定")
	case links == nil:
		return nil, errors.New("shell: LinkManager portが未設定")
	}
	return &StrategyDecider{fs: filesystem, links: links}, nil
}

// Decide はcurrent linkとshimの方式を決める。
//
// docs/09-platform.md §3.3「**同一volumeで安全にrootを導出できるとき**は
// clientへのhardlinkを優先し」。この条件は「shim directoryからclient pathへ
// hardlinkを張れるか」そのものである。**volume名を比べて推測しない** ——
// 同じvolumeでもmount point、ACL、filesystem機能で結果が変わる。実際に張って
// 確かめる。
//
// 張れない場合、v0.1では`fallback-resolver`を使えないため
// `E_PLATFORM_UNSUPPORTED`とする（docs/04-storage-and-data.md §16「必須
// capabilityを確認できない場合はPlanを作らず`E_PLATFORM_UNSUPPORTED`にする」）。
func (d *StrategyDecider) Decide(req StrategyRequest) (Strategies, *domain.Error) {
	if err := req.validate(); err != nil {
		return Strategies{}, domain.Internal(err)
	}
	currentLink, ok := currentLinkStrategies[req.Host.OS()]
	if !ok {
		return Strategies{}, unsupportedStrategy(fmt.Sprintf(
			"OS %q にcurrent link方式の定義が無い", req.Host.OS()))
	}
	shimStrategy, ok := platformShimStrategies[req.Host.OS()]
	if !ok {
		return Strategies{}, unsupportedStrategy(fmt.Sprintf(
			"OS %q にshim方式の定義が無い", req.Host.OS()))
	}
	if err := d.checkShimStrategy(req, shimStrategy); err != nil {
		return Strategies{}, err
	}
	return Strategies{
		CurrentLink:  currentLink,
		Shim:         shimStrategy,
		UsesHardlink: shimStrategy == store.ShimHardlink,
	}, nil
}

// checkShimStrategy は選んだshim方式で実際にlinkを張れるか確かめる。
//
// **作って消す。** 残すとshim directoryに素性の分からないentryが残り、次回の
// probeが既存entryで失敗する。
func (d *StrategyDecider) checkShimStrategy(
	req StrategyRequest, strategy store.ShimStrategy,
) *domain.Error {
	if err := d.fs.MkdirAll(req.ShimDir, probeDirPerm); err != nil {
		return &domain.Error{
			Code:      domain.CodeFilesystem,
			Retryable: true,
			PathRole:  domain.RoleShim,
			Cause:     fmt.Errorf("shell: shim directoryを作れない: %w", err),
		}
	}
	probePath := req.ShimDir + "/" + shimProbeName
	var err error
	switch strategy {
	case store.ShimHardlink:
		err = d.links.CreateHardlink(probePath, req.ClientPath)
	case store.ShimSymlink:
		err = d.links.CreateSymlink(probePath, req.ClientPath, true)
	default:
		return unsupportedStrategy(fmt.Sprintf("shim方式 %q を選べない", strategy))
	}
	if err != nil {
		return unsupportedStrategy(fmt.Sprintf(
			"shim directoryからclientへ%sを張れない（%v）", strategy, err))
	}
	_ = d.links.RemoveLink(probePath)
	return nil
}

// validate はStrategy要求の前提を確かめる。
func (r StrategyRequest) validate() error {
	switch {
	case r.Host.IsZero():
		return errors.New("shell: host platformが未設定")
	case r.ShimDir == "":
		return errors.New("shell: shim directoryが未設定")
	case r.ClientPath == "":
		return errors.New("shell: client pathが未設定")
	}
	return nil
}

// unsupportedStrategy は対象外platformのerrorを作る。
//
// docs/09-platform.md §9の`E_PLATFORM_UNSUPPORTED`である。filesystemを変える
// までretryしても解消しないためRetryableにしない。
func unsupportedStrategy(reason string) *domain.Error {
	return &domain.Error{
		Code:  domain.CodePlatformUnsupported,
		Cause: fmt.Errorf("shell: %s", reason),
	}
}

// SelectCapabilities は§16が求める形でcapability listを整える。
//
// docs/04-storage-and-data.md §16「hardlinkを使う場合だけcapabilityへ
// `hardlink`を含める」。**実測できた能力をそのまま載せない** —— 使わない
// 能力を載せると、Planの`filesystem_capabilities`が「setupが依存する能力」
// ではなく「たまたま使えた能力」の一覧になる。
func SelectCapabilities(
	probed []store.FilesystemCapability, strategies Strategies, host domain.Platform,
) ([]store.FilesystemCapability, *domain.Error) {
	if err := CheckRequired(probed, host); err != nil {
		return nil, err
	}
	required, ok := requiredCapabilities[host.OS()]
	if !ok {
		return nil, unsupportedStrategy(fmt.Sprintf(
			"OS %q に必須capabilityの定義が無い", host.OS()))
	}
	selected := make(map[store.FilesystemCapability]bool, len(required)+1)
	for _, capability := range required {
		selected[capability] = true
	}
	if strategies.UsesHardlink {
		present := false
		for _, capability := range probed {
			if capability == store.CapabilityHardlink {
				present = true
				break
			}
		}
		if !present {
			// 方式としてhardlinkを選んだのに能力が無い。矛盾した状態で
			// Planを作らない。
			return nil, unsupportedStrategy(
				"shim方式にhardlinkを選んだがfilesystemがhardlinkを作れない")
		}
		selected[store.CapabilityHardlink] = true
	}
	return sortedCapabilities(selected), nil
}
