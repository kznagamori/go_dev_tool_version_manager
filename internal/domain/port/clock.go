package port

import (
	"context"
	"time"
)

// Clock は現在時刻、単調時間、待機を供給する（docs/02-architecture.md §4.1）。
//
// 暗黙の時刻取得を禁止するために存在する。receiptのinstalled_at、catalog cacheの
// 有効期限、timeout判定がwall clockへ直接依存すると、testがhostの時計に左右され
// 決定的に検査できない。
type Clock interface {
	// Now はwall clock時刻を返す。記録用の時刻にだけ使う。
	Now() time.Time

	// Since は単調時間で経過時間を返す。timeoutとrate計算にはwall clockではなく
	// こちらを使う。NTP補正やsummer timeでwall clockが巻き戻っても経過時間が
	// 負にならないようにするためである。
	Since(start Monotonic) time.Duration

	// Monotonic は現在の単調時間点を返す。
	Monotonic() Monotonic

	// Sleep はdだけ待機する。cancelされた場合はcontextのerrorを返す。
	//
	// 待機を時刻取得と同じportへ置くのは、docs/10-security.md §10のretryが要求
	// するbackoff（docs/04-storage-and-data.md §21で1/2/4秒に固定）を決定的に
	// testするためである。実装が`time.Sleep`を直接呼ぶと、backoffのtestが実時間
	// で7秒かかり、回数と間隔を検査できない。
	//
	// dが0以下の場合は待機せず、cancel済みcontextのerrorだけを返す。
	Sleep(ctx context.Context, d time.Duration) error
}

// Monotonic は単調増加する時間点である。wall clockとは別の型にして、
// 記録用時刻と経過時間計測の取り違えをcompile時に防ぐ。
type Monotonic struct {
	// nanos は起点からのnanosecondである。起点の絶対値に意味はなく、
	// 差分だけが意味を持つ。
	nanos int64
}

// NewMonotonic は起点からのnanosecondで単調時間点を作る。
// Clock実装とtestだけが使う。
func NewMonotonic(nanos int64) Monotonic {
	return Monotonic{nanos: nanos}
}

// Nanos は起点からのnanosecondを返す。
func (m Monotonic) Nanos() int64 {
	return m.nanos
}

// Sub は2つの単調時間点の差を返す。
func (m Monotonic) Sub(other Monotonic) time.Duration {
	return time.Duration(m.nanos - other.nanos)
}
