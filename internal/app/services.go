package app

import (
	"fmt"
	"strings"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain/port"
)

// Services はApplication Serviceの生成結果である（docs/02-architecture.md §4）。
//
// 検証済みのbuild metadataと外部作用portだけを保持する。値型なので、生成後に
// 呼出し側が元のBuildInfo変数を書き換えてもここへは伝わらない。
//
// §4は`App ApplicationService`と`Runtime RuntimeResolver`の2 fieldを挙げるが、
// その2つの型はそれぞれ§7・§8のoperationと§9のresolverであり、実装はP8-01と
// P8-03である（docs/13-progress.md）。中身の無いinterfaceを先に置くことは
// 「使わないものを将来のために残さない」方針に反するため、P1-03時点では
// 持たない。§18が内部Go APIを同一module内で変更してよいと定めるため、2 fieldは
// 対応するtaskで追加する。
type Services struct {
	build BuildInfo
	ports port.Ports
}

// NewServices は依存を注入してServicesを組み立てる。
//
// docs/02-architecture.md §4に従い、ここで行うのは依存の存在検査とbuild metadata
// 形式検査だけである。filesystem、network、環境変数、時刻のいずれにも触れない。
// 有効rootの決定やconfig/state/registryの読込みは`Initialize`の責務であり、
// constructorで先に触ると、`--home`やmode overrideがrequestで決まる前に
// 別のrootを掴んでしまう。
//
// portが1つでも欠けていればerrorにする。nilのまま組み立てを許すと、欠けたportを
// 最初に使うoperationまで失敗が遅れ、しかもnil pointer panicとして現れるためである。
func NewServices(build BuildInfo, ports port.Ports) (Services, error) {
	if err := build.Validate(); err != nil {
		return Services{}, fmt.Errorf("app: build metadataが不正である: %w", err)
	}
	if missing := ports.Missing(); len(missing) > 0 {
		return Services{}, fmt.Errorf("app: portが注入されていない: %s", strings.Join(missing, ", "))
	}
	return Services{build: build, ports: ports}, nil
}

// BuildInfo は検証済みのbuild metadataのcopyを返す。
//
// docs/02-architecture.md §7の`GetBuildInfo`が、config/state/registryが壊れていても
// binary metadataだけで応答できるようにするための入口である。
func (s Services) BuildInfo() BuildInfo { return s.build }
