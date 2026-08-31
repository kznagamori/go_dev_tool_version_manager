package app

import (
	"errors"
	"fmt"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/store"
)

// PlanScopeRequest はPlanから[Scope]を導くための入力である。
//
// **root群をPlanだけから作れない。** docs/04-storage-and-data.md §16は
// 「`writes[]`は利用者可視の変更だけを列挙する。staging、download cache、state、
// receipt、index、shim、storageなどdata root内部の書込みはPlanへ列挙しない」と
// 定める。`writes[]`だけをrootにすると、列挙されない内部書込みがすべて拒否される。
//
// docs/02-architecture.md §8手順5が許す範囲は「data root、distribution root、
// 宣言済みintegration対象、project file」である。前2者はsetup stateとconfigが
// 持つ値で、Planの外から渡す。
type PlanScopeRequest struct {
	// Plan は対象のPlanである。
	Plan store.Plan
	// DataRoot はdata rootである（role=data-root）。
	DataRoot domain.PathValue
	// DistributionRoot はdistribution rootである（role=distribution-root）。
	DistributionRoot domain.PathValue
	// Host はpath比較のcase規則を決めるplatformである。
	Host domain.Platform
}

// ScopeFromPlan はPlanが許す作用だけのScopeを組み立てる。
//
// docs/11-quality-and-ci.md §7.2と[02-architecture.md](../../docs/02-architecture.md)
// §8手順5の「Execute中のdownload/extract/probeがPlanの列挙と一致し、全書込みが
// data root、distribution root、宣言済みintegration対象、project fileの中にあり、
// **任意helper/backend processを起動しない**こと」をallowlistとして表す。
//
// Planに列挙されていない作用は、ここで作ったScopeが一律に拒否する。
func ScopeFromPlan(req PlanScopeRequest) (*Scope, error) {
	if req.Host.IsZero() {
		return nil, errors.New("app: host platformが未設定")
	}
	roots, err := planScopeRoots(req)
	if err != nil {
		return nil, err
	}
	processes, err := planScopeProcesses(req.Plan)
	if err != nil {
		return nil, err
	}
	return NewScope(ScopeRequest{
		Roots:     roots,
		Processes: processes,
		Downloads: planScopeDownloads(req.Plan),
		Host:      req.Host,
	})
}

// planScopeRoots は§8手順5の封じ込め範囲を集める。
func planScopeRoots(req PlanScopeRequest) ([]domain.PathValue, error) {
	if req.DataRoot.IsZero() {
		return nil, errors.New("app: data rootが未設定")
	}
	if req.DataRoot.Role() != domain.RoleDataRoot {
		return nil, fmt.Errorf("app: data rootのroleが%sである", req.DataRoot.Role())
	}
	roots := []domain.PathValue{req.DataRoot}

	// distribution rootはportable modeでdata rootと同じ場所になりうるが、
	// user modeでは別になる。zeroなら渡し忘れと区別できないため必須にする。
	if req.DistributionRoot.IsZero() {
		return nil, errors.New("app: distribution rootが未設定")
	}
	if req.DistributionRoot.Role() != domain.RoleDistributionRoot {
		return nil, fmt.Errorf(
			"app: distribution rootのroleが%sである", req.DistributionRoot.Role())
	}
	roots = append(roots, req.DistributionRoot)

	// `writes[]`が挙げるのはdata root外の利用者可視な対象である。
	// integration対象とproject file、current linkがここに入る。
	for index, write := range req.Plan.Writes {
		if write.Target.IsZero() {
			return nil, fmt.Errorf("app: writes[%d]のtargetが未設定", index)
		}
		if write.Action == store.WriteRegistryValue {
			// §17.2「Windows user PATHのregistry valueはfilesystem pathでは
			// ないが変更対象の識別が必要なため…`path`はexact locator
			// `HKCU\Environment\Path`とする」。filesystem書込みのrootとして
			// 扱うと、その文字列で始まるpathを許すことになる。registry値の
			// 変更はfilesystem writeを通らないため、Scopeの対象にしない。
			continue
		}
		roots = append(roots, write.Target)
	}
	return roots, nil
}

// planScopeProcesses は§16のprobeを起動許可へ変換する。
//
// **Planに無いprocessを1つも許さない。** §8手順5の「任意helper/backend processを
// 起動しない」は、許可listがprobeと完全に一致することで表す。
func planScopeProcesses(plan store.Plan) ([]AllowedProcess, error) {
	if len(plan.Probes) == 0 {
		return nil, nil
	}
	processes := make([]AllowedProcess, 0, len(plan.Probes))
	for index, probe := range plan.Probes {
		if probe.Executable.IsZero() || probe.Executable.Path() == "" {
			return nil, fmt.Errorf("app: probes[%d]のexecutableが未設定", index)
		}
		args := make([]string, 0, len(probe.Args))
		for argIndex, arg := range probe.Args {
			value, err := planArgValue(arg)
			if err != nil {
				return nil, fmt.Errorf("app: probes[%d].args[%d]: %w", index, argIndex, err)
			}
			args = append(args, value)
		}
		processes = append(processes, AllowedProcess{
			Executable: probe.Executable.Path(),
			Args:       args,
			Dir:        probe.WorkingDirectory.Path(),
		})
	}
	return processes, nil
}

// planArgValue は§16の`PlanArg`をargv 1要素へ戻す。
//
// 「`kind=literal`では`value`をそのままargv 1要素とし`path=null`、`kind=path`では
// `value`を空、`path`を非空の`PathValue`とし、そのnative pathをargv 1要素とする」。
func planArgValue(arg store.PlanArg) (string, error) {
	switch arg.Kind {
	case store.ArgLiteral:
		if !arg.Path.IsZero() {
			return "", errors.New("kind=literalにpathがある")
		}
		return arg.Value, nil
	case store.ArgPath:
		if arg.Value != "" {
			return "", errors.New("kind=pathにvalueがある")
		}
		if arg.Path.IsZero() || arg.Path.Path() == "" {
			return "", errors.New("kind=pathのpathが空")
		}
		return arg.Path.Path(), nil
	default:
		return "", fmt.Errorf("未知のarg kind %q", arg.Kind)
	}
}

// planScopeDownloads は§16のdownload URLを取得許可へ変換する。
func planScopeDownloads(plan store.Plan) []string {
	if len(plan.Downloads) == 0 {
		return nil
	}
	urls := make([]string, 0, len(plan.Downloads))
	for _, download := range plan.Downloads {
		urls = append(urls, download.URL)
	}
	return urls
}
