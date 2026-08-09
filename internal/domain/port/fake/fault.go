// Package fake はport interfaceの決定的な実装とfailure injection基盤を提供する。
//
// docs/11-quality-and-ci.md §6 は clock/HTTP/process/filesystem/link/user lookup を
// fakeへ差し替えた決定的検査を要求し、§8 は download失敗、probe失敗、disk full、
// commit直前中断のfailure injectionを要求する。両方をここで満たす。
//
// production codeからimportしてはならない。CIの`policy` jobが検査する。
package fake

import (
	"fmt"
	"sort"
	"sync"
)

// Fault は特定の操作へ注入する失敗である。
type Fault struct {
	// Op は操作名である。fake側がCheckへ渡す識別子と一致させる。
	Op string
	// Err は返すerrorである。nilは「この回は成功させる」を意味し、
	// N回目だけ失敗させる指定に使う。
	Err error
	// Times は残り発火回数である。0以下は無制限を意味する。
	Times int
}

// Injector は操作名ごとに失敗を注入し、呼出回数を数える。
//
// 「3回目のwriteでdisk full」のような順序依存の失敗を、実際のdisk状態に
// 依存せず決定的に再現するために持つ。並行testからも使うためmutexで保護する。
type Injector struct {
	mu     sync.Mutex
	queues map[string][]Fault
	calls  map[string]int
}

// NewInjector は空のInjectorを作る。
func NewInjector() *Injector {
	return &Injector{
		queues: make(map[string][]Fault),
		calls:  make(map[string]int),
	}
}

// Fail はopのN回目以降でerrを返すよう登録する。
//
// skip回だけ成功させてから失敗させる。skipが0なら次の呼出から失敗する。
// timesが0以下なら以後ずっと失敗する。
func (i *Injector) Fail(op string, skip int, times int, err error) {
	if err == nil {
		panic("fake: Fail にnil errorを渡した。成功させたい回数はskipで指定する")
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	if skip > 0 {
		i.queues[op] = append(i.queues[op], Fault{Op: op, Err: nil, Times: skip})
	}
	i.queues[op] = append(i.queues[op], Fault{Op: op, Err: err, Times: times})
}

// FailOnce はopの次の1回だけerrを返す。
func (i *Injector) FailOnce(op string, err error) {
	i.Fail(op, 0, 1, err)
}

// Check はopの呼出を記録し、注入された失敗があればそのerrorを返す。
// fakeの各操作は実処理の前にこれを呼ぶ。
func (i *Injector) Check(op string) error {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.calls[op]++

	queue := i.queues[op]
	for len(queue) > 0 {
		head := &queue[0]
		if head.Times > 0 {
			head.Times--
			if head.Times == 0 {
				queue = queue[1:]
			}
			i.queues[op] = queue
			return head.Err
		}
		// Times <= 0 は無制限。取り除かない。
		i.queues[op] = queue
		return head.Err
	}
	i.queues[op] = queue
	return nil
}

// Calls はopの呼出回数を返す。
func (i *Injector) Calls(op string) int {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.calls[op]
}

// Operations は呼出のあった操作名をsortして返す。
// 「任意helper processが起動していないこと」のような不在検査に使う。
func (i *Injector) Operations() []string {
	i.mu.Lock()
	defer i.mu.Unlock()
	ops := make([]string, 0, len(i.calls))
	for op := range i.calls {
		ops = append(ops, op)
	}
	sort.Strings(ops)
	return ops
}

// Pending は消化されずに残っている注入を返す。
//
// testの最後に空であることを確認するために使う。残っていれば、意図した経路が
// 実行されなかったということであり、testが素通りしている。
func (i *Injector) Pending() []string {
	i.mu.Lock()
	defer i.mu.Unlock()
	var pending []string
	for op, queue := range i.queues {
		for _, fault := range queue {
			if fault.Err != nil {
				pending = append(pending, fmt.Sprintf("%s(%v)", op, fault.Err))
			}
		}
	}
	sort.Strings(pending)
	return pending
}
