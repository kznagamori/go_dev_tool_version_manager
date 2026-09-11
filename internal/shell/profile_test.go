package shell

import (
	"errors"
	"strings"
	"testing"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/store"
)

// profile統合はfileを読み書きせず、内容の変換だけを行う純粋な処理である。
// marker判定とescapeを実fileなしで網羅できる。

func TestProfilePathMatchesSpec(t *testing.T) {
	t.Parallel()
	// docs/09-platform.md §5.2「bashは`~/.bashrc`、zshは`~/.zshrc`、fishは
	// `~/.config/fish/conf.d/gdtvm.fish`を対象とする」。
	cases := map[store.Shell]string{
		store.ShellBash: "/home/u/.bashrc",
		store.ShellZsh:  "/home/u/.zshrc",
		store.ShellFish: "/home/u/.config/fish/conf.d/gdtvm.fish",
	}
	for shell, want := range cases {
		got, err := ProfilePath(shell, "/home/u")
		if err != nil {
			t.Fatalf("ProfilePath(%q): %v", shell, err)
		}
		if got != want {
			t.Errorf("ProfilePath(%q) = %q, want %q", shell, got, want)
		}
	}
	// 末尾separatorがあっても二重にしない。
	got, err := ProfilePath(store.ShellBash, "/home/u/")
	if err != nil {
		t.Fatalf("ProfilePath: %v", err)
	}
	if got != "/home/u/.bashrc" {
		t.Errorf("ProfilePath = %q, want %q", got, "/home/u/.bashrc")
	}
	if _, err := ProfilePath("tcsh", "/home/u"); !errors.Is(err, ErrUnsupportedShell) {
		t.Errorf("error = %v, want ErrUnsupportedShell", err)
	}
	if _, err := ProfilePath(store.ShellBash, ""); err == nil {
		t.Error("homeが空でも通った")
	}
}

func TestRenderBlockMatchesSpecLiterally(t *testing.T) {
	t.Parallel()
	// docs/09-platform.md §5.2の字句そのもの。marker文字列や本文が変わると、
	// 既存profileのblockを自分のものと認識できなくなる。
	posix, err := RenderBlock(store.ShellBash, "/root/shims")
	if err != nil {
		t.Fatalf("RenderBlock: %v", err)
	}
	wantPosix := "# >>> gdtvm initialize >>>\n" +
		"export PATH='/root/shims':\"$PATH\"\n" +
		"# <<< gdtvm initialize <<<\n"
	if posix != wantPosix {
		t.Errorf("bash block = %q, want %q", posix, wantPosix)
	}
	// zshはbashと同じ本文である。
	zsh, err := RenderBlock(store.ShellZsh, "/root/shims")
	if err != nil {
		t.Fatalf("RenderBlock(zsh): %v", err)
	}
	if zsh != wantPosix {
		t.Errorf("zsh block = %q, want %q", zsh, wantPosix)
	}

	fish, err := RenderBlock(store.ShellFish, "/root/shims")
	if err != nil {
		t.Fatalf("RenderBlock(fish): %v", err)
	}
	wantFish := "# >>> gdtvm initialize >>>\n" +
		"fish_add_path --prepend '/root/shims'\n" +
		"# <<< gdtvm initialize <<<\n"
	if fish != wantFish {
		t.Errorf("fish block = %q, want %q", fish, wantFish)
	}
}

func TestRenderBlockNeverEscapesTheQuoting(t *testing.T) {
	t.Parallel()
	// **command substitutionを生成しない**（docs/09-platform.md §5.2）。
	// 囲みの外へ出る文字を作ると、そこから先がcommandとして解釈される。
	//
	// **往復では確かめられない。** escapeを解く側を自分で書くと、
	// 「escapeを一切しない」実装でも解く側が何も戻さず往復が成立してしまう。
	// shell自身のlexerを模して**描画結果を走査**し、literalがどこで終わるかを見る。
	hostile := []string{
		`/root/'; rm -rf ~; echo '`,
		"/root/$(id)",
		"/root/`id`",
		`/root/${HOME}`,
		`/root/back\slash`,
		`/root/"double"`,
		"/root/semi;colon",
		"/root/pipe|char",
		`/root/'`,
		`/root/\`,
		`/root/\'`,
	}
	for _, shimDir := range hostile {
		for _, shell := range []store.Shell{store.ShellBash, store.ShellZsh, store.ShellFish} {
			block, err := RenderBlock(shell, shimDir)
			if err != nil {
				t.Fatalf("RenderBlock(%q, %q): %v", shell, shimDir, err)
			}
			body := blockBody(t, block)
			prefix, suffix := bodyAffixes(shell)
			if !strings.HasPrefix(body, prefix) {
				t.Fatalf("%s: 本文の形が違う: %q", shell, body)
			}
			rest := strings.TrimPrefix(body, prefix)

			literal, after, ok := scanSingleQuoted(shell, rest)
			if !ok {
				t.Fatalf("%s: %q でliteralが閉じていない: %q", shell, shimDir, body)
			}
			// literalが元の値に戻ること。
			if literal != shimDir {
				t.Errorf("%s: %q を埋めて %q が戻った（body=%q）", shell, shimDir, literal, body)
			}
			// **literalの外に残るのは、意図したsuffixだけである。**
			// escapeが効いていなければ、ここへ利用者の文字列の続きが現れる。
			if after != suffix {
				t.Errorf("%s: %q でliteralの外へ %q が漏れた（body=%q）",
					shell, shimDir, after, body)
			}
		}
	}
}

// blockBody はblockの本文1行を返す。
func blockBody(t *testing.T, block string) string {
	t.Helper()
	lines := strings.Split(strings.TrimSuffix(block, "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("blockが3行でない: %q", block)
	}
	if lines[0] != MarkerBegin || lines[2] != MarkerEnd {
		t.Fatalf("markerが一致しない: %q", block)
	}
	return lines[1]
}

// bodyAffixes は本文のliteral前後に来る固定文字列を返す。
func bodyAffixes(shell store.Shell) (string, string) {
	if shell == store.ShellFish {
		return "fish_add_path --prepend ", ""
	}
	return "export PATH=", `:"$PATH"`
}

// scanSingleQuoted はshellのlexerを模してsingle quoted literalを1つ読む。
//
// 戻り値はliteralの中身、literalの直後に残る文字列、閉じられたかである。
// **production側の実装を参照しない。** 参照すると、実装の誤りをそのまま
// 正解として扱うことになる。
func scanSingleQuoted(shell store.Shell, input string) (string, string, bool) {
	if !strings.HasPrefix(input, "'") {
		return "", input, false
	}
	var literal strings.Builder
	i := 1
	for i < len(input) {
		c := input[i]
		if shell == store.ShellFish && c == '\\' && i+1 < len(input) {
			// fishのsingle quote内はbackslashとsingle quoteだけがescapeできる。
			next := input[i+1]
			if next == '\\' || next == '\'' {
				literal.WriteByte(next)
				i += 2
				continue
			}
		}
		if c == '\'' {
			if shell != store.ShellFish && strings.HasPrefix(input[i:], `'\''`) {
				// POSIXは閉じる→escapeしたquote→開き直すの4文字で
				// single quote 1文字になる。
				literal.WriteByte('\'')
				i += 4
				continue
			}
			// ここでliteralが閉じる。
			return literal.String(), input[i+1:], true
		}
		literal.WriteByte(c)
		i++
	}
	return literal.String(), "", false
}

func TestRenderBlockRejectsUnsafeLiteral(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		// 改行はblockの行構造を壊し、markerの対応が取れなくなる。
		"改行":  "/root/a\nb",
		"復帰":  "/root/a\rb",
		"NUL": "/root/a\x00b",
	}
	for name, shimDir := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			for _, shell := range []store.Shell{store.ShellBash, store.ShellFish} {
				if _, err := RenderBlock(shell, shimDir); !errors.Is(err, ErrUnsafeLiteral) {
					t.Errorf("%s: error = %v, want ErrUnsafeLiteral", shell, err)
				}
			}
		})
	}
	if _, err := RenderBlock(store.ShellBash, ""); err == nil {
		t.Error("shim directoryが空でも通った")
	}
	if _, err := RenderBlock("tcsh", "/root/shims"); !errors.Is(err, ErrUnsupportedShell) {
		t.Error("未対応shellが通った")
	}
}

func TestInspectProfileClassifiesMarkerState(t *testing.T) {
	t.Parallel()
	want, err := RenderBlock(store.ShellBash, "/root/shims")
	if err != nil {
		t.Fatalf("RenderBlock: %v", err)
	}
	other, err := RenderBlock(store.ShellBash, "/old/shims")
	if err != nil {
		t.Fatalf("RenderBlock: %v", err)
	}

	cases := map[string]struct {
		content string
		state   ProfileState
	}{
		"空":        {content: "", state: ProfileAbsent},
		"markerなし": {content: "export FOO=1\n", state: ProfileAbsent},
		"完全一致":     {content: "export FOO=1\n\n" + want, state: ProfileMatches},
		"内容が違う":    {content: "export FOO=1\n\n" + other, state: ProfileDiffers},
		"開始だけ":     {content: MarkerBegin + "\nexport PATH=x\n", state: ProfileConflict},
		"終了だけ":     {content: "export PATH=x\n" + MarkerEnd + "\n", state: ProfileConflict},
		"2組":       {content: want + "\n" + want, state: ProfileConflict},
		"順序が逆":     {content: MarkerEnd + "\nexport PATH=x\n" + MarkerBegin + "\n", state: ProfileConflict},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got := InspectProfile(tc.content, want)
			if got.State != tc.state {
				t.Errorf("State = %q, want %q", got.State, tc.state)
			}
			// **不完全/複数は自動修正せず診断する**（docs/09-platform.md §5.2）。
			// 理由が空だと利用者は何を直せばよいか分からない。
			if got.State == ProfileConflict && got.Reason == "" {
				t.Error("ConflictなのにReasonが空")
			}
		})
	}
}

func TestApplyBlockFollowsMarkerRules(t *testing.T) {
	t.Parallel()
	want, err := RenderBlock(store.ShellBash, "/root/shims")
	if err != nil {
		t.Fatalf("RenderBlock: %v", err)
	}

	t.Run("marker0件なら末尾へ追加", func(t *testing.T) {
		t.Parallel()
		original := "export FOO=1\n"
		got, changed, err := ApplyBlock(original, want)
		if err != nil {
			t.Fatalf("ApplyBlock: %v", err)
		}
		if !changed {
			t.Error("changed = false")
		}
		// **既存の行を1行も変えない。**
		if !strings.HasPrefix(got, original) {
			t.Errorf("既存の内容が保たれていない: %q", got)
		}
		if !strings.HasSuffix(got, want) {
			t.Errorf("末尾へblockが付いていない: %q", got)
		}
	})

	t.Run("改行で終わらないfileへ追加", func(t *testing.T) {
		t.Parallel()
		got, _, err := ApplyBlock("export FOO=1", want)
		if err != nil {
			t.Fatalf("ApplyBlock: %v", err)
		}
		// 既存の最終行へ連結すると、その行の意味が変わる。
		if !strings.HasPrefix(got, "export FOO=1\n") {
			t.Errorf("最終行へ連結した: %q", got)
		}
		if !strings.HasSuffix(got, want) {
			t.Errorf("blockが付いていない: %q", got)
		}
	})

	t.Run("完全一致1件ならno-op", func(t *testing.T) {
		t.Parallel()
		original := "export FOO=1\n\n" + want
		got, changed, err := ApplyBlock(original, want)
		if err != nil {
			t.Fatalf("ApplyBlock: %v", err)
		}
		// 書き直すとmtimeだけが変わり、利用者の差分に意味のない変更が出る。
		if changed {
			t.Error("一致しているのにchanged = true")
		}
		if got != original {
			t.Errorf("no-opなのに内容が変わった: %q", got)
		}
	})

	t.Run("内容が違えば差し替える", func(t *testing.T) {
		t.Parallel()
		old, err := RenderBlock(store.ShellBash, "/old/shims")
		if err != nil {
			t.Fatalf("RenderBlock: %v", err)
		}
		original := "export FOO=1\n\n" + old + "export BAR=2\n"
		got, changed, err := ApplyBlock(original, want)
		if err != nil {
			t.Fatalf("ApplyBlock: %v", err)
		}
		if !changed {
			t.Error("changed = false")
		}
		if strings.Contains(got, "/old/shims") {
			t.Errorf("古いblockが残っている: %q", got)
		}
		// **markerの外は1行も触らない。**
		if !strings.Contains(got, "export FOO=1\n") || !strings.Contains(got, "export BAR=2\n") {
			t.Errorf("marker外の行が失われた: %q", got)
		}
	})

	t.Run("不完全なら自動修正しない", func(t *testing.T) {
		t.Parallel()
		_, _, err := ApplyBlock(MarkerBegin+"\nexport PATH=x\n", want)
		if !errors.Is(err, ErrProfileConflict) {
			t.Fatalf("error = %v, want ErrProfileConflict", err)
		}
	})
}

func TestApplyBlockRoundTripsWithRemove(t *testing.T) {
	t.Parallel()
	want, err := RenderBlock(store.ShellFish, "/root/shims")
	if err != nil {
		t.Fatalf("RenderBlock: %v", err)
	}
	cases := map[string]string{
		"通常":    "set -x FOO 1\n",
		"空":     "",
		"改行なし":  "set -x FOO 1",
		"末尾が空行": "set -x FOO 1\n\n",
	}
	for name, original := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			applied, changed, err := ApplyBlock(original, want)
			if err != nil {
				t.Fatalf("ApplyBlock: %v", err)
			}
			if !changed {
				t.Fatal("追加したのにchanged = false")
			}
			removed, changed, err := RemoveBlock(applied)
			if err != nil {
				t.Fatalf("RemoveBlock: %v", err)
			}
			if !changed {
				t.Fatal("除去したのにchanged = false")
			}
			// **除去後に利用者の行が残ること。** 往復で内容が痩せると、
			// `setup --remove`が利用者の設定を削ったことになる。
			for _, line := range strings.Split(strings.TrimSuffix(original, "\n"), "\n") {
				if line == "" {
					continue
				}
				if !strings.Contains(removed, line) {
					t.Errorf("利用者の行 %q が失われた: %q", line, removed)
				}
			}
			if strings.Contains(removed, MarkerBegin) || strings.Contains(removed, MarkerEnd) {
				t.Errorf("markerが残っている: %q", removed)
			}
		})
	}
}

func TestRemoveBlockLeavesForeignContentAlone(t *testing.T) {
	t.Parallel()
	t.Run("markerが無ければno-op", func(t *testing.T) {
		t.Parallel()
		original := "export FOO=1\n"
		got, changed, err := RemoveBlock(original)
		if err != nil {
			t.Fatalf("RemoveBlock: %v", err)
		}
		if changed || got != original {
			t.Errorf("markerが無いのに変更した: %q", got)
		}
	})
	t.Run("不完全なら消さない", func(t *testing.T) {
		t.Parallel()
		// 所有を証明できない状態では消さない。利用者が手で書いたblockを巻き込む。
		_, _, err := RemoveBlock(MarkerBegin + "\nexport PATH=x\n")
		if !errors.Is(err, ErrProfileConflict) {
			t.Fatalf("error = %v, want ErrProfileConflict", err)
		}
	})
}

func TestErrorsCarryTheSafeAlternative(t *testing.T) {
	t.Parallel()
	// docs/09-platform.md §5.2「`E_UNSUPPORTED_SHELL`とし、**`none`を案内する**」、
	// §9「errorには…利用者が選べる安全な代替を含める」。
	unsupported := UnsupportedShellError("tcsh")
	if unsupported.Code != domain.CodeUnsupportedShell {
		t.Errorf("code = %q, want %q", unsupported.Code, domain.CodeUnsupportedShell)
	}
	if unsupported.Cause == nil || !strings.Contains(unsupported.Cause.Error(), string(store.PathIntegrationNone)) {
		t.Errorf("cause = %v, noneの案内を含むべき", unsupported.Cause)
	}
	if !strings.Contains(unsupported.Cause.Error(), "tcsh") {
		t.Errorf("cause = %v, 対象shellを名指しすべき", unsupported.Cause)
	}

	conflict := ProfileConflictError("/home/u/.bashrc",
		ProfileInspection{State: ProfileConflict, Reason: "markerが開始2件・終了1件である"})
	if conflict.Code != domain.CodeShellProfileConflict {
		t.Errorf("code = %q, want %q", conflict.Code, domain.CodeShellProfileConflict)
	}
	if conflict.Cause == nil {
		t.Fatal("Causeが無い")
	}
	for _, want := range []string{"/home/u/.bashrc", "開始2件", string(store.PathIntegrationNone)} {
		if !strings.Contains(conflict.Cause.Error(), want) {
			t.Errorf("cause = %v, %q を含むべき", conflict.Cause, want)
		}
	}
}
