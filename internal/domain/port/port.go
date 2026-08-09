// Package port は外部作用の抽象ポートを定義する。
//
// docs/02-architecture.md §1 が「抽象ポートはcore側が所有し、Infrastructureは
// それを実装する」と定めるため、interfaceはcore側であるdomain配下に置く。
// Application ServiceやInfrastructure adapterへ置くと、内側のpackageが外側を
// importすることになりGoのimport cycleを生む。
//
// このpackageはdomain値と標準libraryだけに依存し、OS API、HTTP client、
// CLI、TOML libraryのいずれも参照しない。production adapterとfakeは同じ
// interfaceを実装し、[Ports] 経由でApplication Serviceへ注入する。
//
// docs/02-architecture.md §4.1 は14 portを定義するが、本packageが持つのは
// docs/11-quality-and-ci.md §6 が決定的検査の前提とする6件だけである。
// 残る8件（Registry、Archive、Hash、Lock、Environment、Random、ProgressSink、
// Logger）は最初に必要とするtaskで追加する（docs/13-progress.md P0-03）。
package port

// Ports は外部作用の注入口である。
//
// docs/02-architecture.md §4 に従い、production adapterとfakeを同じfieldへ
// 入れ替えられるようにする。Prompt/terminalはadapter責務のため持たない。
// progress/cancelはrequestごとに渡すため、ここには置かない。
//
// 現在は6 portだけを持つ。field追加時はdocs/13-progress.mdの該当taskと
// docs/02-architecture.md §4.1 を同じ変更で更新する。
type Ports struct {
	Clock         Clock
	FileSystem    FileSystem
	HTTPClient    HTTPClient
	LinkManager   LinkManager
	ProcessRunner ProcessRunner
	UserLookup    UserLookup
}
