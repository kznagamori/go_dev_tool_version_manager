// Command gdtvm は go_dev_tool_version_manager の CLI である。
//
// gdtvm は multi-call binary であり、起動 basename が "gdtvm" のときだけ CLI として
// 動作し、shim index の公開 command 名なら最小 runtime resolver へ分岐する（08章11.2節）。
// この分岐と各 command の実装は後続フェーズ（W07/W09）で追加する。本ファイルは骨格段階
// の暫定 entry point である。
//
// CLI 薄層は引数解析、型付き request 変換、表示、prompt、終了コード変換だけを担い、
// domain 判断・path 決定・TOML/state 直接操作・network・展開・link・process・環境生成・
// security policy を持たない（02章、10章12節）。
package main

import (
	"fmt"
	"os"
)

func main() {
	// 骨格段階では CLI/shim dispatch を未実装とし、明示的に一般失敗で終了する。
	fmt.Fprintln(os.Stderr, "gdtvm: CLI is not implemented yet (W09)")
	os.Exit(exitGeneral)
}
