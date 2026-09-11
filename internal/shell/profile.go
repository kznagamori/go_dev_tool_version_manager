package shell

import (
	"errors"
	"fmt"
	"strings"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/store"
)

// marker行とblock本文（docs/09-platform.md §5.2）。
//
// marker文字列は仕様が字句として定めるものであり、設定で変えられない。変えると
// 既存profileのblockを自分のものと認識できなくなる。
const (
	// MarkerBegin はblockの開始行である。
	MarkerBegin = "# >>> gdtvm initialize >>>"
	// MarkerEnd はblockの終了行である。
	MarkerEnd = "# <<< gdtvm initialize <<<"
)

// profile統合のsentinel error。
var (
	// ErrUnsupportedShell が示すshellには安全なintegrationが無い。
	//
	// docs/09-platform.md §9の`E_UNSUPPORTED_SHELL`に対応する。
	ErrUnsupportedShell = errors.New("shell: 対象shellに安全なintegrationがない")
	// ErrProfileConflict はmarkerが不完全または複数あることを表す。
	//
	// docs/09-platform.md §9の`E_SHELL_PROFILE_CONFLICT`に対応する。
	ErrProfileConflict = errors.New("shell: profileのmarkerが不完全または複数ある")
	// ErrUnsafeLiteral はescapeできない文字が含まれることを表す。
	ErrUnsafeLiteral = errors.New("shell: shell literalへ安全に埋め込めない")
)

// profilePaths はshellごとの対象fileである（home相対）。
//
// docs/09-platform.md §5.2「bashは`~/.bashrc`、zshは`~/.zshrc`、fishは
// `~/.config/fish/conf.d/gdtvm.fish`を対象とする」。**検出した全profileを
// 一括変更しない**ため、shell 1件につきfile 1件で閉じる。
var profilePaths = map[store.Shell]string{
	store.ShellBash: ".bashrc",
	store.ShellZsh:  ".zshrc",
	store.ShellFish: ".config/fish/conf.d/gdtvm.fish",
}

// ProfilePath はshellの対象profile pathをhomeからの相対で返す。
//
// homeは§2.2のOS lookup値を呼出し側が渡す。**環境変数`HOME`で置換しない**
// （docs/04-storage-and-data.md §1.2）。
func ProfilePath(shell store.Shell, home string) (string, error) {
	relative, ok := profilePaths[shell]
	if !ok {
		return "", fmt.Errorf("%w: %q（対象はbash、zsh、fish）", ErrUnsupportedShell, shell)
	}
	if home == "" {
		return "", errors.New("shell: homeが未設定")
	}
	return strings.TrimSuffix(home, "/") + "/" + relative, nil
}

// RenderBlock はshim directoryを埋め込んだmarker blockを返す。
//
// docs/09-platform.md §5.2の字句をそのまま組み立てる。**command substitutionを
// 生成しない** —— dollar-parenやbackquoteを通すと、profileを読み込んだshellが
// gdtvmの意図しないcommandを実行する。single quoteで囲み、quote自体を
// 各shellの規則でescapeすることで、内容が常にliteralになる。
func RenderBlock(shell store.Shell, shimDir string) (string, error) {
	if shimDir == "" {
		return "", errors.New("shell: shim directoryが未設定")
	}
	quoted, err := quoteLiteral(shell, shimDir)
	if err != nil {
		return "", err
	}
	var body string
	switch shell {
	case store.ShellBash, store.ShellZsh:
		body = "export PATH=" + quoted + `:"$PATH"`
	case store.ShellFish:
		body = "fish_add_path --prepend " + quoted
	default:
		return "", fmt.Errorf("%w: %q", ErrUnsupportedShell, shell)
	}
	return MarkerBegin + "\n" + body + "\n" + MarkerEnd + "\n", nil
}

// quoteLiteral は文字列を各shellのsingle quote規則でescapeする。
//
// POSIX shell（bash/zsh）のsingle quote内は**いかなるescapeも効かない**。
// そのため、single quoteそのものは「quoteを閉じる→backslashでescapeした
// quote→quoteを開き直す」の4文字へ置き換える。
//
// fishのsingle quote内はbackslashとsingle quoteだけがescape可能であり、
// その2文字をbackslashで前置する。
//
// **どちらの規則も、囲みの外へ出る文字を作らない。** 出せると、そこから先が
// commandとして解釈される。
func quoteLiteral(shell store.Shell, value string) (string, error) {
	if strings.ContainsRune(value, 0) {
		// NULはshell fileへ書けない。書くと読み込み側の挙動がshellごとに変わる。
		return "", fmt.Errorf("%w: NULを含む", ErrUnsafeLiteral)
	}
	if strings.ContainsAny(value, "\n\r") {
		// 改行はblockの行構造を壊し、markerの対応が取れなくなる。
		return "", fmt.Errorf("%w: 改行を含む", ErrUnsafeLiteral)
	}
	switch shell {
	case store.ShellBash, store.ShellZsh:
		return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'", nil
	case store.ShellFish:
		escaped := strings.ReplaceAll(value, `\`, `\\`)
		escaped = strings.ReplaceAll(escaped, "'", `\'`)
		return "'" + escaped + "'", nil
	default:
		return "", fmt.Errorf("%w: %q", ErrUnsupportedShell, shell)
	}
}

// ProfileState はprofile file内のmarker blockの状態である。
type ProfileState string

// ProfileState の値。
const (
	// ProfileAbsent はmarkerが1件も無い状態である。
	ProfileAbsent ProfileState = "absent"
	// ProfileMatches は求めるblockと完全に一致する1件がある状態である。
	ProfileMatches ProfileState = "matches"
	// ProfileDiffers はblockが1件あるが内容が違う状態である。
	ProfileDiffers ProfileState = "differs"
	// ProfileConflict はmarkerが不完全または複数ある状態である。
	//
	// docs/09-platform.md §5.2「不完全/複数なら**自動修正せず診断する**」。
	ProfileConflict ProfileState = "conflict"
)

// ProfileInspection はprofileを読んだ結果である。
type ProfileInspection struct {
	// State はmarker blockの状態である。
	State ProfileState
	// BeginCount は開始markerの出現数である。
	BeginCount int
	// EndCount は終了markerの出現数である。
	EndCount int
	// Reason はConflictのときの理由である。
	Reason string
}

// InspectProfile はprofileの内容から求めるblockとの関係を判定する。
//
// **利用者fileをsource/evaluateしない**（docs/09-platform.md §5.2）。行の
// 一致だけで判定する。shellに読ませて判断すると、profileに書かれた任意の
// commandをgdtvmが実行することになる。
func InspectProfile(content, want string) ProfileInspection {
	lines := strings.Split(content, "\n")
	var begins, ends []int
	for i, line := range lines {
		switch strings.TrimRight(line, " \t\r") {
		case MarkerBegin:
			begins = append(begins, i)
		case MarkerEnd:
			ends = append(ends, i)
		}
	}
	inspection := ProfileInspection{BeginCount: len(begins), EndCount: len(ends)}

	switch {
	case len(begins) == 0 && len(ends) == 0:
		inspection.State = ProfileAbsent
		return inspection
	case len(begins) != 1 || len(ends) != 1:
		inspection.State = ProfileConflict
		inspection.Reason = fmt.Sprintf(
			"markerが開始%d件・終了%d件である（各1件であるべき）", len(begins), len(ends))
		return inspection
	case ends[0] < begins[0]:
		inspection.State = ProfileConflict
		inspection.Reason = "終了markerが開始markerより前にある"
		return inspection
	}

	block := strings.Join(lines[begins[0]:ends[0]+1], "\n") + "\n"
	if block == want {
		inspection.State = ProfileMatches
		return inspection
	}
	inspection.State = ProfileDiffers
	return inspection
}

// ApplyBlock はprofileの内容へblockを反映した結果を返す。
//
// docs/09-platform.md §5.2「marker 0件なら末尾へ追加、完全一致1件ならno-op、
// 不完全/複数なら自動修正せず診断する」。
//
// **内容を組み立てるだけで書き込まない。** 書込みはbackup→temp→flush→atomic
// replaceの順で行う必要があり（同§5.2）、その順序は[RunTransaction]の段階として
// 呼出し側が持つ。ここを純粋な変換にしておくと、marker判定と行組立てを
// 実fileなしで網羅できる。
func ApplyBlock(content, want string) (updated string, changed bool, err error) {
	inspection := InspectProfile(content, want)
	switch inspection.State {
	case ProfileMatches:
		// 既に一致している。書き直すとmtimeだけが変わり、利用者の差分に
		// 意味のない変更が出る。
		return content, false, nil
	case ProfileConflict:
		return "", false, fmt.Errorf("%w: %s", ErrProfileConflict, inspection.Reason)
	case ProfileDiffers:
		return replaceBlock(content, want), true, nil
	default:
		return appendBlock(content, want), true, nil
	}
}

// RemoveBlock はprofileの内容からblockを取り除いた結果を返す。
//
// docs/09-platform.md §7「`setup --remove`はsetup stateで所有を証明できる
// PATH entryまたはmarkerだけを除去する」。**markerの外は1行も触らない。**
func RemoveBlock(content string) (updated string, changed bool, err error) {
	inspection := InspectProfile(content, "")
	switch inspection.State {
	case ProfileAbsent:
		return content, false, nil
	case ProfileConflict:
		// 所有を証明できない状態では消さない。利用者が手で書いたblockを
		// 巻き込む。
		return "", false, fmt.Errorf("%w: %s", ErrProfileConflict, inspection.Reason)
	}
	return cutBlock(content), true, nil
}

// appendBlock は末尾へblockを足す。
//
// 直前が空行でなければ1行空ける。既存の最終行へ連結すると、その行の意味が
// 変わる。
func appendBlock(content, block string) string {
	if content == "" {
		return block
	}
	var builder strings.Builder
	builder.WriteString(content)
	if !strings.HasSuffix(content, "\n") {
		builder.WriteString("\n")
	}
	if !strings.HasSuffix(content, "\n\n") {
		builder.WriteString("\n")
	}
	builder.WriteString(block)
	return builder.String()
}

// replaceBlock は既存blockを新しい内容へ差し替える。
func replaceBlock(content, block string) string {
	before, after := splitAroundBlock(content)
	return before + block + after
}

// cutBlock は既存blockを取り除く。
func cutBlock(content string) string {
	before, after := splitAroundBlock(content)
	if before == "" {
		return after
	}
	if after == "" {
		// blockが末尾だった。直前に入れた空行も一緒に戻す。
		return strings.TrimRight(before, "\n") + "\n"
	}
	return before + after
}

// splitAroundBlock はblockの前後をそれぞれ返す。
//
// 呼出し側が[InspectProfile]でmarkerが各1件・順序が正しいことを確かめてから
// 呼ぶ。
func splitAroundBlock(content string) (string, string) {
	lines := strings.Split(content, "\n")
	begin, end := -1, -1
	for i, line := range lines {
		switch strings.TrimRight(line, " \t\r") {
		case MarkerBegin:
			begin = i
		case MarkerEnd:
			end = i
		}
	}
	if begin < 0 || end < begin {
		return content, ""
	}
	before := ""
	if begin > 0 {
		before = strings.Join(lines[:begin], "\n") + "\n"
	}
	after := ""
	if end+1 < len(lines) {
		after = strings.Join(lines[end+1:], "\n")
	}
	return before, after
}

// UnsupportedShellError は§9の`E_UNSUPPORTED_SHELL`を作る。
//
// docs/09-platform.md §5.2「対象shellがbash/zsh/fishのいずれでもない場合は
// `E_UNSUPPORTED_SHELL`とし、**`none`を案内する**」。案内をerrorへ含めるのは
// §9「errorには…利用者が選べる安全な代替を含める」による。
func UnsupportedShellError(name string) *domain.Error {
	return &domain.Error{
		Code: domain.CodeUnsupportedShell,
		Cause: fmt.Errorf(
			"shell: %q には安全なshell profile統合がない。"+
				"対象はbash、zsh、fishである。%s を選べばprofileを変更しない",
			name, store.PathIntegrationNone),
	}
}

// ProfileConflictError は§9の`E_SHELL_PROFILE_CONFLICT`を作る。
func ProfileConflictError(path string, inspection ProfileInspection) *domain.Error {
	return &domain.Error{
		Code:     domain.CodeShellProfileConflict,
		PathRole: domain.RoleConfig,
		Cause: fmt.Errorf(
			"shell: %s のmarkerを自動修正しない（%s）。"+
				"markerを手で1組に直すか、%s を選べばprofileを変更しない",
			path, inspection.Reason, store.PathIntegrationNone),
	}
}
