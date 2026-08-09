package port

import (
	"context"
	"io"
	"time"
)

// ProcessSpec は子processの起動指定である。
//
// shell文字列ではなくargv配列で受けるのは、docs/10-security.md が要求する
// command injectionのfail closedのためである。shell経由の起動口を作らない。
type ProcessSpec struct {
	// Executable は解決済みのabsolute pathである。PATH探索は呼出側が済ませる。
	Executable string
	// Args はargv[1]以降である。argv[0]は実装がExecutableから決める。
	Args []string
	// Dir はworking directoryである。空は拒否する。暗黙のcwdを使わないためである。
	Dir string
	// Env は完全な環境である。nilは空環境を意味し、親環境の暗黙継承はしない。
	Env map[string]string
	// Stdin はnil可。
	Stdin io.Reader
	// Timeout は0で無制限である。
	Timeout time.Duration
	// PassthroughStdio がtrueなら、stdout/stderrをcaptureせず利用者processへ
	// そのまま渡す。shim経由の実行がこれに当たり、gdtvmは内容を保存もmaskもしない
	// （docs/10-security.md）。
	PassthroughStdio bool
}

// ProcessResult は実行結果である。
type ProcessResult struct {
	ExitCode int
	// Stdout と Stderr はcapture時のみ設定する。組込み上限で打ち切り、
	// secretをmaskした後の値である（docs/10-security.md §9.2）。
	Stdout string
	Stderr string
	// TimedOut はTimeout超過で打ち切った場合にtrueである。
	TimedOut bool
	// Duration は単調時間で測った実行時間である。
	Duration time.Duration
}

// ProcessRunner は子processの起動を抽象化する（docs/02-architecture.md §4.1）。
//
// helper processやbackend daemonを起こす経路を持たない。E2Eの書込み範囲検査は
// このportの記録を証跡にするため（docs/11-quality-and-ci.md §7.2）、
// portを迂回した起動を作らない。
type ProcessRunner interface {
	// Run は子processを実行して終了を待つ。ctxのcancelで打ち切る。
	Run(ctx context.Context, spec ProcessSpec) (ProcessResult, error)
}
