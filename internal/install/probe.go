package install

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"regexp"
	"time"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain/port"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/security"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/store"
)

// ProbeRequest はPlanのprobeを実行するための入力である。
type ProbeRequest struct {
	// Plan は承認済みのPlanである。実行するprobeはこの列挙がすべてである。
	Plan store.Plan
	// Host はpath規則を決めるplatformである。
	Host domain.Platform
}

// ProbeOutcome は1件のprobeの結果である。
type ProbeOutcome struct {
	// ID はPlanのprobe IDである。
	ID string
	// Status はreceiptへ書く実行結果である。
	Status store.ProbeStatus
	// ReportedVersion はprobeが報告したversionである。非対象なら空。
	ReportedVersion string
	// FinishedAt はprobeが終わった時刻である。
	FinishedAt time.Time
}

// ProbeRunner はPlanのvalidation probeを実行する。
//
// docs/08-install-runtime.md §7手順2「**sanitized最小環境で、probe専用の
// owner-only temp directoryをcwdとして**required probeを実行する」。
type ProbeRunner struct {
	fs     port.FileSystem
	runner port.ProcessRunner
	clock  port.Clock
}

// NewProbeRunner はProbeRunnerを作る。
func NewProbeRunner(
	filesystem port.FileSystem, runner port.ProcessRunner, clock port.Clock,
) (*ProbeRunner, error) {
	switch {
	case filesystem == nil:
		return nil, errors.New("install: FileSystem portが未設定")
	case runner == nil:
		return nil, errors.New("install: ProcessRunner portが未設定")
	case clock == nil:
		return nil, errors.New("install: Clock portが未設定")
	}
	return &ProbeRunner{fs: filesystem, runner: runner, clock: clock}, nil
}

// Run はPlanのprobeを順に実行する。
//
// docs/06-tool-definition.md §11「probeごとに空のowner-only probe tempを作り、
// **成功/失敗/cancel後にengineが削除する**」。probe tempの作成と削除をここで行う。
//
// **required probeが失敗したらそこで止める。** §11「required probe failureは
// commit前にinstall全体を失敗させる」。後続を走らせても結果は捨てるため、
// 利用者を待たせるだけである。
func (p *ProbeRunner) Run(ctx context.Context, req ProbeRequest) ([]ProbeOutcome, *domain.Error) {
	if req.Host.IsZero() {
		return nil, domain.Internal(errors.New("install: host platformが未設定"))
	}
	if len(req.Plan.Probes) == 0 {
		return nil, nil
	}

	outcomes := make([]ProbeOutcome, 0, len(req.Plan.Probes))
	for index := range req.Plan.Probes {
		// §2「cancelはdownload、checksum取得、展開entry、probeの境界で確認する」。
		if ctxErr := ctx.Err(); ctxErr != nil {
			return outcomes, cancelledError(ctxErr)
		}
		outcome, err := p.runOne(ctx, req, req.Plan.Probes[index])
		if err != nil {
			return outcomes, err
		}
		outcomes = append(outcomes, outcome)
	}
	return outcomes, nil
}

// runOne は1件のprobeを実行する。
func (p *ProbeRunner) runOne(
	ctx context.Context, req ProbeRequest, probe store.PlanProbe,
) (ProbeOutcome, *domain.Error) {
	temp := probe.WorkingDirectory
	if temp.IsZero() || temp.Path() == "" {
		return ProbeOutcome{}, domain.Internal(fmt.Errorf(
			"install: probe %q のcwdが未設定", probe.ID))
	}
	// §11「probeごとに**空の**owner-only probe tempを作り」。既存があれば
	// 消してから作る——前回の中断が残した内容がprobe結果を変えうる。
	if err := p.fs.RemoveAll(temp.Path()); err != nil {
		return ProbeOutcome{}, stagingError(fmt.Errorf(
			"install: probe %q のtempを掃除できない: %w", probe.ID, err))
	}
	if err := p.fs.MkdirAll(temp.Path(), probeTempPerm); err != nil {
		return ProbeOutcome{}, stagingError(fmt.Errorf(
			"install: probe %q のtempを作れない: %w", probe.ID, err))
	}
	// **成功/失敗/cancelのいずれでも削除する**（§11）。
	defer func() { _ = p.fs.RemoveAll(temp.Path()) }()

	args := make([]string, 0, len(probe.Args))
	for argIndex, arg := range probe.Args {
		value, err := planArgValue(arg)
		if err != nil {
			return ProbeOutcome{}, domain.Internal(fmt.Errorf(
				"install: probe %q のargs[%d]: %w", probe.ID, argIndex, err))
		}
		args = append(args, value)
	}

	result, runErr := p.runner.Run(ctx, port.ProcessSpec{
		Executable: probe.Executable.Path(),
		Args:       args,
		Dir:        temp.Path(),
		// **sanitized最小環境**（§7手順2、docs/10-security.md §7）。nilは空環境で
		// あり、親環境を継承しない。OSが起動に要求する最小変数（Windowsの
		// `SystemRoot`）だけをprocess adapterが補う。
		Env:     nil,
		Timeout: time.Duration(probe.TimeoutMillis) * time.Millisecond,
	})
	finishedAt := p.clock.Now().UTC()
	if runErr != nil {
		// §7「permission/OS起動失敗は対応するplatform/filesystem error」。
		return ProbeOutcome{}, probeStartError(fmt.Errorf(
			"install: probe %q を起動できない: %w", probe.ID, runErr))
	}

	if err := p.evaluate(probe, result); err != nil {
		return ProbeOutcome{}, err
	}
	reported, err := p.reportedVersion(probe, result, req.Host)
	if err != nil {
		return ProbeOutcome{}, err
	}
	if err := p.checkRequiredPaths(probe); err != nil {
		return ProbeOutcome{}, err
	}
	return ProbeOutcome{
		ID:              probe.ID,
		Status:          store.ProbePassed,
		ReportedVersion: reported,
		FinishedAt:      finishedAt,
	}, nil
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

// evaluate は起動結果そのものを検査する。
//
// §7「required probeの起動後nonzero、timeout、output上限、version/root/path/
// 能力不一致は`E_PROBE_FAILED`」。
func (p *ProbeRunner) evaluate(probe store.PlanProbe, result port.ProcessResult) *domain.Error {
	switch {
	case result.TimedOut:
		return probeFailed(probe, fmt.Errorf(
			"timeout（%d ms）", probe.TimeoutMillis), result)
	case result.ExitCode != 0:
		return probeFailed(probe, fmt.Errorf("exit code %d", result.ExitCode), result)
	}
	return nil
}

// reportedVersion は§11のexpectごとの判定を行い、報告versionを返す。
func (p *ProbeRunner) reportedVersion(
	probe store.PlanProbe, result port.ProcessResult, host domain.Platform,
) (string, *domain.Error) {
	output := probeOutput(probe, result)

	switch probe.Expect {
	case store.ExpectSuccess:
		// exit code 0だけを要求する。regexは任意で、あれば一致も要求する。
		if probe.Regex == "" {
			return "", nil
		}
		matched, err := matchProbe(probe, output)
		if err != nil {
			return "", err
		}
		return matched, nil

	case store.ExpectVersion:
		matched, err := matchProbe(probe, output)
		if err != nil {
			return "", err
		}
		// §11「regexで取り出したversionが`{{version}}`と一致することを要求する」。
		// Planの`expected_version`はrender済みの完全versionである。
		if matched != probe.ExpectedVersion {
			return "", probeFailed(probe, fmt.Errorf(
				"報告version %q が期待 %q と一致しない", matched, probe.ExpectedVersion),
				result)
		}
		return matched, nil

	case store.ExpectPathWithin:
		matched, err := matchProbe(probe, output)
		if err != nil {
			return "", err
		}
		if probe.ExpectedRoot == nil || probe.ExpectedRoot.Path() == "" {
			return "", domain.Internal(fmt.Errorf(
				"install: probe %q のexpected_rootが未設定", probe.ID))
		}
		// §11「regexで取り出したpathが指定root内にあることを要求する」。
		if !security.IsContained(probe.ExpectedRoot.Path(), matched, host) {
			return "", probeFailed(probe, fmt.Errorf(
				"報告path %q がroot %q の外にある", matched, probe.ExpectedRoot.Path()),
				result)
		}
		return "", nil

	default:
		return "", domain.Internal(fmt.Errorf(
			"install: probe %q の未知のexpect %q", probe.ID, probe.Expect))
	}
}

// probeOutput は§11のstreamに従って読む出力を選ぶ。
func probeOutput(probe store.PlanProbe, result port.ProcessResult) string {
	switch probe.Stream {
	case store.StreamStdout:
		return result.Stdout
	case store.StreamStderr:
		return result.Stderr
	default:
		return result.Stdout + result.Stderr
	}
}

// matchProbe はregexの1つ目のcapture groupを取り出す。
func matchProbe(probe store.PlanProbe, output string) (string, *domain.Error) {
	if probe.Regex == "" {
		return "", domain.Internal(fmt.Errorf(
			"install: probe %q のregexが未設定", probe.ID))
	}
	// §11のregexはRE2である。Goの`regexp`がRE2であり、catastrophic backtrackingを
	// 起こさない。
	compiled, err := regexp.Compile(probe.Regex)
	if err != nil {
		// definitionが持ち込んだpatternの誤りである。
		return "", definitionError(fmt.Errorf(
			"install: probe %q のregexを解釈できない: %w", probe.ID, err))
	}
	match := compiled.FindStringSubmatch(output)
	if match == nil {
		return "", probeFailed(probe, errors.New("regexが出力と一致しない"),
			port.ProcessResult{})
	}
	if len(match) < 2 {
		return "", definitionError(fmt.Errorf(
			"install: probe %q のregexにcapture groupが無い", probe.ID))
	}
	return match[1], nil
}

// checkRequiredPaths は§11の`required_paths`を確かめる。
//
// 「probe成功直後に指定種別の存在を要求する」。
func (p *ProbeRunner) checkRequiredPaths(probe store.PlanProbe) *domain.Error {
	for _, required := range probe.RequiredPaths {
		info, err := p.fs.Stat(required.Path.Path())
		if err != nil {
			return probeFailed(probe, fmt.Errorf(
				"required path %q が無い: %w", required.Path.Path(), err),
				port.ProcessResult{})
		}
		switch required.Kind {
		case store.RequiredFile:
			if info.IsDir {
				return probeFailed(probe, fmt.Errorf(
					"required path %q がfileでない", required.Path.Path()),
					port.ProcessResult{})
			}
		case store.RequiredDirectory:
			if !info.IsDir {
				return probeFailed(probe, fmt.Errorf(
					"required path %q がdirectoryでない", required.Path.Path()),
					port.ProcessResult{})
			}
		default:
			return domain.Internal(fmt.Errorf(
				"install: probe %q の未知のrequired path kind %q", probe.ID, required.Kind))
		}
	}
	return nil
}

// probeFailed は`E_PROBE_FAILED`を作る。
//
// §7「probe stderr末尾はmask/上限後だけhuman errorへ含める」。[port.ProcessResult]の
// Stderrはadapterがmaskと上限適用を済ませた値であり、そのまま載せてよい。
func probeFailed(probe store.PlanProbe, cause error, result port.ProcessResult) *domain.Error {
	wrapped := fmt.Errorf("install: probe %q が失敗した: %w", probe.ID, cause)
	if result.Stderr != "" {
		wrapped = fmt.Errorf("%w（stderr: %s）", wrapped, result.Stderr)
	}
	return &domain.Error{Code: domain.CodeProbeFailed, Cause: wrapped}
}

// definitionError は`E_DEFINITION_INVALID`を作る。
//
// §7「実行file欠落やdefinition参照不正は`E_DEFINITION_INVALID`」。
func definitionError(cause error) *domain.Error {
	return &domain.Error{Code: domain.CodeDefinitionInvalid, Cause: cause}
}

// stagingError はstaging領域のfilesystem失敗を表す。
//
// download cacheを指す[filesystemError]と別にするのは、公開境界へ出す
// `PathRole`が違うためである（docs/02-architecture.md §14は実pathを出さず
// roleで対象を特定させる）。
func stagingError(cause error) *domain.Error {
	return &domain.Error{
		Code:      domain.CodeFilesystem,
		Retryable: true,
		PathRole:  domain.RoleStaging,
		Cause:     cause,
	}
}

// probeStartError はprobeのOS起動失敗を表す。
//
// §7「permission/OS起動失敗は対応するplatform/filesystem errorとし」。
// 起動できない原因の多くはpermissionであり、利用者が直せる。
func probeStartError(cause error) *domain.Error {
	return &domain.Error{Code: domain.CodePermission, Retryable: true, Cause: cause}
}

// probeTempPerm はprobe tempのpermissionである。
//
// docs/06-tool-definition.md §11「**owner-only** probe temp」。他userから
// 読み書きできると、probeの入出力を差し替えられる。
const probeTempPerm fs.FileMode = 0o700
