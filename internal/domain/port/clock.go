package port

import "time"

// Clock は現在時刻と単調時間を供給する（docs/02-architecture.md §4.1）。
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
