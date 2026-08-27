package app

import (
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/security"
)

// Scope は1 operationが行ってよい外部作用の許可集合である。
//
// docs/02-architecture.md §8手順5「Execute中のdownload/extract/probeがPlanの列挙と
// 一致し、全書込みがdata root、distribution root、宣言済みintegration対象、
// project fileの中にあり、任意helper/backend processを起動しないこと」を、
// 実行時に判定できる形へ落としたものである。
//
// **[domain.Plan]の代替ではない。** Planはdocs/04-storage-and-data.mdの完全な
// typed内容を持ち、承認・表示・fingerprintまで担う。ここが持つのは、port呼出しを
// 通す／通さないの判定に要る最小限だけである。Planを組み立てる側（P6）が、
// `writes[]`・`probes[]`・artifact URLからこのScopeを作る。
type Scope struct {
	// roots は書込みを許すlogical rootである。role付きで持つ。
	//
	// docs/11-quality-and-ci.md §7.2「判定はdocs/04-storage-and-data.md §17.2の
	// `path_role`で行う」。pathだけで持つと、同じabsolute pathが別roleで
	// 現れたときにどのrootとして許したのかが分からなくなる。
	roots []domain.PathValue
	// processes は起動を許すprocessである。
	processes []AllowedProcess
	// downloads は取得を許すURLである。正規化して保持する。
	downloads []string
	// host はpath比較のcase規則を決めるplatformである。
	host domain.Platform
}

// AllowedProcess は起動を許す1 processである。
//
// docs/11-quality-and-ci.md §7.2「変更operationで起動した全probe processがPlan
// `probes[]`のexecutable/argv/cwd/write pathと一致し、任意helper/backend
// processがない」。write pathは[Scope.roots]が別途縛るため、ここは起動そのものを
// 決める3つを持つ。
type AllowedProcess struct {
	// Executable は解決済みのabsolute pathである。
	Executable string
	// Args はargv[1]以降である。**完全一致で照合する。**
	//
	// prefix一致にすると、宣言したargsの後ろへ任意の引数を足して別の動作を
	// させられる。probeは`--version`のような固定argvしか使わない。
	Args []string
	// Dir はworking directoryである。
	Dir string
}

// ScopeRequest はScopeの構築入力である。
type ScopeRequest struct {
	// Roots は書込みを許すlogical rootである。1件以上必要である。
	Roots []domain.PathValue
	// Processes は起動を許すprocessである。0件なら1つも起動できない。
	Processes []AllowedProcess
	// Downloads は取得を許すURLである。0件なら1件もdownloadできない。
	Downloads []string
	// Host はpath比較のcase規則を決めるplatformである。
	Host domain.Platform
}

// NewScope はScopeを組み立てる。
//
// **空のScopeを許さない。** rootが0件のScopeはどの書込みも拒否するため一見安全に
// 見えるが、実際には「rootを渡し忘れた」ことと区別できない。呼出し側が縛る対象を
// 明示していることを構築時に要求する。
func NewScope(req ScopeRequest) (*Scope, error) {
	if req.Host.IsZero() {
		return nil, errors.New("app: host platformが未設定")
	}
	if len(req.Roots) == 0 {
		return nil, errors.New("app: 書込みを許すrootが1件も無い")
	}
	scope := &Scope{host: req.Host}
	for index, root := range req.Roots {
		if root.IsZero() {
			return nil, fmt.Errorf("app: roots[%d]のroleが未設定", index)
		}
		if root.Path() == "" {
			return nil, fmt.Errorf("app: roots[%d]（role %s）のpathが空", index, root.Role())
		}
		scope.roots = append(scope.roots, root)
	}
	for index, process := range req.Processes {
		if process.Executable == "" {
			return nil, fmt.Errorf("app: processes[%d]のExecutableが空", index)
		}
		if process.Dir == "" {
			return nil, fmt.Errorf("app: processes[%d]のDirが空", index)
		}
		scope.processes = append(scope.processes, AllowedProcess{
			Executable: process.Executable,
			Args:       append([]string(nil), process.Args...),
			Dir:        process.Dir,
		})
	}
	for index, raw := range req.Downloads {
		normalized, err := normalizeDownloadURL(raw)
		if err != nil {
			return nil, fmt.Errorf("app: downloads[%d]: %w", index, err)
		}
		scope.downloads = append(scope.downloads, normalized)
	}
	return scope, nil
}

// AllowsWrite は解決済みpathが許可rootのいずれかに含まれるかを返す。
//
// 含まれる場合は、どのrootとして許したかのroleも返す。書込み記録へ残すためで、
// docs/11-quality-and-ci.md §7.2の封じ込め検査はroleで判定する。
//
// **解決済みpathを渡す。** 解決前のpathで比べると、symlink/reparse pointを経由した
// 逸脱を見逃す（docs/10-security.md §6）。
func (s *Scope) AllowsWrite(resolved string) (domain.PathRole, bool) {
	if s == nil || resolved == "" {
		return "", false
	}
	// 最も深いrootを採る。data rootの下にdistribution rootがあるような入れ子で、
	// 宣言順に依存して別のroleを記録しないためである。
	var (
		best      domain.PathRole
		bestDepth = -1
		found     bool
	)
	for _, root := range s.roots {
		if !security.IsContained(root.Path(), resolved, s.host) {
			continue
		}
		if depth := pathDepth(root.Path(), s.host); depth > bestDepth {
			best, bestDepth, found = root.Role(), depth, true
		}
	}
	return best, found
}

// AllowsProcess は起動指定が許可listのいずれかと完全一致するかを返す。
func (s *Scope) AllowsProcess(executable string, args []string, dir string) bool {
	if s == nil {
		return false
	}
	for _, allowed := range s.processes {
		if allowed.Executable != executable || allowed.Dir != dir {
			continue
		}
		if len(allowed.Args) != len(args) {
			continue
		}
		same := true
		for index := range args {
			if allowed.Args[index] != args[index] {
				same = false
				break
			}
		}
		if same {
			return true
		}
	}
	return false
}

// AllowsDownload は取得先が許可listに含まれるかを返す。
func (s *Scope) AllowsDownload(raw string) bool {
	if s == nil {
		return false
	}
	normalized, err := normalizeDownloadURL(raw)
	if err != nil {
		return false
	}
	for _, allowed := range s.downloads {
		if allowed == normalized {
			return true
		}
	}
	return false
}

// normalizeDownloadURL は照合用にURLを正規化する。
//
// schemeとhostだけをlower caseにする。RFC 3986がこの2つをcase非依存と定める
// 一方、pathとqueryはcase依存であり、まとめてlower caseにすると別のartifactを
// 同一視してしまう。fragmentはserverへ送られないため落とす。
func normalizeDownloadURL(raw string) (string, error) {
	if raw == "" {
		return "", errors.New("URLが空")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		// 生URLをerrorへ載せない（docs/10-security.md §9.2）。
		return "", errors.New("URLとして解析できない")
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", errors.New("scheme/hostを持たないURLである")
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Host = strings.ToLower(parsed.Host)
	parsed.Fragment = ""
	parsed.RawFragment = ""
	return parsed.String(), nil
}

// pathDepth はpathのcomponent数を返す。入れ子rootの深さ比較に使う。
func pathDepth(path string, host domain.Platform) int {
	return len(splitComponents(path, host))
}

// splitComponents はpathをcomponent列へ分ける。
//
// Windowsは`\`と`/`の両方を区切りとして扱うため、両方で分ける
// （[security.IsContained]と同じ規則）。片方だけで分けると、同じ位置を指す
// pathのcomponent数がずれる。
func splitComponents(path string, host domain.Platform) []string {
	separators := "/"
	if host.OS() == domain.OSWindows {
		separators = `/\`
	}
	return strings.FieldsFunc(path, func(r rune) bool {
		return strings.ContainsRune(separators, r)
	})
}

// joinComponents はcomponent列を元のpathの前置き（root部）ごと組み直す。
//
// `original`から先頭のroot表記（Linuxの`/`、WindowsのUNC prefix等）を保つため、
// componentを繋いだ結果ではなく、originalの中で該当componentが終わる位置までを
// 切り出す。文字列を組み直すと`/`始まりが落ちてrelative pathになる。
func joinComponents(original string, components []string, separator string) string {
	if len(components) == 0 {
		return separator
	}
	// componentは重複しうる（`/a/b/a`）。末尾から探すと別の出現に当たるため、
	// 先頭から順に位置を進める。
	offset := 0
	for _, component := range components {
		index := strings.Index(original[offset:], component)
		if index < 0 {
			return original
		}
		offset += index + len(component)
	}
	return original[:offset]
}

// WriteAction は記録する書込み種別である（docs/04-storage-and-data.md §16のPlan
// `writes[].action`のうち、port経由で観測できるもの）。
type WriteAction string

// WriteAction の値。
const (
	// WriteCreate はfile/directoryの作成または内容置換である。
	WriteCreate WriteAction = "create"
	// WriteMove はrenameである。
	WriteMove WriteAction = "move"
	// WriteRemove は削除である。
	WriteRemove WriteAction = "remove"
	// WritePermission はpermission変更である。
	WritePermission WriteAction = "permission"
)

// WriteRecord は1件の書込み記録である。
//
// docs/11-quality-and-ci.md §7.2が「port経由でしか外部作用が起きない構造のため、
// この記録は実質的な全書込みの証跡になる」と定める、その証跡の1件である。
type WriteRecord struct {
	// Action は書込み種別である。
	Action WriteAction
	// Role は許可rootのどれとして通したかである。
	Role domain.PathRole
	// Path は解決済みのabsolute pathである。
	Path string
}

// ProcessRecord は1件のprocess起動記録である。
type ProcessRecord struct {
	Executable string
	Args       []string
	Dir        string
	// EnvNames は渡した環境変数名をsortして持つ。**値は持たない。**
	//
	// docs/10-security.md §9.2が「環境変数の全量dumpを出さず、definitionが
	// 宣言したkeyの有無だけを示す」と定める。証跡へ値を残すとそこがsecretの
	// 保管場所になる。
	EnvNames []string
}

// DownloadRecord は1件のdownload記録である。
type DownloadRecord struct {
	// URL はmask済みである（docs/10-security.md §9.2）。
	URL string
}

// Records は1 operationが行った外部作用の記録である。
type Records struct {
	Writes    []WriteRecord
	Processes []ProcessRecord
	Downloads []DownloadRecord
}

// envNames は環境変数名だけをsortして返す。
func envNames(env map[string]string) []string {
	if len(env) == 0 {
		return nil
	}
	names := make([]string, 0, len(env))
	for name := range env {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
