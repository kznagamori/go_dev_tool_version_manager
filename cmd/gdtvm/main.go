// Command gdtvm は開発tool版managerのCLIである。
//
// この層に許すのはcommand table、help、raw入力からdomain型へのparse、
// Application Service呼出し、Plan表示/prompt、Result/Error/progress表示、
// exit code変換だけである（docs/02-architecture.md §16）。domain判断、path決定、
// TOML/state直接操作、network、展開、link、process、環境生成、security policyを
// ここへ置かない。
//
// P1-01時点ではpackage骨格だけであり、9 commandはP8-04で実装する
// （docs/13-progress.md P8-04）。
package main

import (
	"fmt"
	"io"
	"os"
)

// skeletonExitCode は骨格段階の終了codeである。
//
// 正式なexit code体系はP1-04で定めるため、ここでは汎用の失敗として1を返す
// （docs/13-progress.md P1-04）。未実装を成功として返さないことだけを保証する。
const skeletonExitCode = 1

func main() {
	os.Exit(run(os.Stderr))
}

// run はCLIの入口を関数として切り出したものである。
//
// main本体を薄く保ち、exit codeをprocess終了と切り離してtestできるようにする。
// P8-04でcommand tableとApplication Service呼出しへ置き換える。
func run(stderr io.Writer) int {
	fmt.Fprintln(stderr, "gdtvm: CLI はまだ実装されていない (docs/13-progress.md P8-04)")
	return skeletonExitCode
}
