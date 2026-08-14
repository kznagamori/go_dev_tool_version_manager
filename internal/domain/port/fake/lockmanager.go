package fake

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain/port"
)

// OpAcquireLock はLockManager.Acquireの失敗注入名である。
const OpAcquireLock = "lock.Acquire"

// OpReleaseLock はLock.Releaseの失敗注入名である。
//
// 解放の失敗を注入できるようにするのは、docs/02-architecture.md §12の
// 「cancel/timeoutでも取得済みlockを必ず解放する」を、解放が失敗する場合まで
// 含めてtestできるようにするためである。
const OpReleaseLock = "lock.Release"

// LockManager は決定的なin-process fake LockManagerである。
//
// OS lockではなくmapで排他を模すが、**docs/02-architecture.md §12のlock順序は
// 実装と同じく強制する**。順序違反を素通りさせると、本番でだけdeadlockする
// codeがtestを通ってしまう。
//
// 保持中lockと待機の様子をtestから観測できるようにし、process間競合の
// 再現に使う。
type LockManager struct {
	mu       sync.Mutex
	injector *Injector
	// held はこのmanagerが保持しているlockである。key→classで持つ。
	held map[string]port.LockClass
	// owners は他processが保持している想定のlockである。
	//
	// Acquireはここに載っているkeyを待ち、[LockManager.ReleaseExternal]で
	// 解放されるまで取得できない。process間競合をclockを進めずに再現する。
	owners map[string]struct{}
	// waits は待機に入った回数である。timeout経路を通ったことの確認に使う。
	waits int
	// clock はtimeout判定に使う。fake clockを渡すと待ち時間を制御できる。
	clock *Clock
}

// NewLockManager はfake LockManagerを作る。
func NewLockManager(injector *Injector, clock *Clock) *LockManager {
	return &LockManager{
		injector: injector,
		held:     make(map[string]port.LockClass),
		owners:   make(map[string]struct{}),
		clock:    clock,
	}
}

// HoldExternal は他processが保持している状態を作る。
func (m *LockManager) HoldExternal(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.owners[key] = struct{}{}
}

// ReleaseExternal は他processの保持を解く。
func (m *LockManager) ReleaseExternal(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.owners, key)
}

// Waits は待機に入った回数を返す。
func (m *LockManager) Waits() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.waits
}

// HeldKeys は保持中lockのkeyをclass昇順・key昇順で返す。
func (m *LockManager) HeldKeys() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	keys := make([]string, 0, len(m.held))
	for key := range m.held {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		return port.CompareLocks(m.held[keys[i]], keys[i], m.held[keys[j]], keys[j]) < 0
	})
	return keys
}

// Held は保持中lockのclassをclass昇順・key昇順で返す。
func (m *LockManager) Held() []port.LockClass {
	keys := m.HeldKeys()
	m.mu.Lock()
	defer m.mu.Unlock()
	classes := make([]port.LockClass, 0, len(keys))
	for _, key := range keys {
		classes = append(classes, m.held[key])
	}
	return classes
}

// Acquire はlockを取得する。
//
// 順序検査 → 二重取得検査 → 他process待ちの順で判定する。順序違反を最初に
// 見るのは、それが実装の誤りであって競合状態ではないためである。
func (m *LockManager) Acquire(ctx context.Context, request port.LockRequest) (port.Lock, error) {
	if err := m.injector.Check(OpAcquireLock); err != nil {
		return nil, err
	}
	key, err := port.LockKey(request.Class, request.Qualifier)
	if err != nil {
		return nil, err
	}

	m.mu.Lock()
	if err := m.checkOrderLocked(request.Class, key); err != nil {
		m.mu.Unlock()
		return nil, err
	}
	if _, duplicate := m.held[key]; duplicate {
		m.mu.Unlock()
		return nil, fmt.Errorf("fake: lock %q を二重取得した", key)
	}
	_, contended := m.owners[key]
	if !contended {
		m.held[key] = request.Class
		m.mu.Unlock()
		return &fakeLock{manager: m, class: request.Class, key: key}, nil
	}
	m.waits++
	m.mu.Unlock()

	return m.wait(ctx, request, key)
}

// checkOrderLocked は§12のlock順序を強制する。
//
// 保持中のどのlockよりも後ろ（class順、同classならkey順）でなければならない。
// 小さいclassを大きいclassの後に取ると、逆順で取る別のinvocationとdeadlockする。
func (m *LockManager) checkOrderLocked(class port.LockClass, key string) error {
	for heldKey, heldClass := range m.held {
		if port.CompareLocks(heldClass, heldKey, class, key) >= 0 {
			return fmt.Errorf(
				"%w: 保持中の %q(%s) より後に %q(%s) を取得できない",
				port.ErrLockOrder, heldKey, heldClass, key, class)
		}
	}
	return nil
}

// wait は他processの解放を待つ。
//
// fake clockを進めることでtimeoutを再現する。実時間のsleepを入れないのは、
// testの実行時間をclockの進め方で制御できるようにするためである。
func (m *LockManager) wait(ctx context.Context, request port.LockRequest, key string) (port.Lock, error) {
	limit := request.Wait
	if limit <= 0 {
		limit = port.LockWaitDefault
	}
	deadline := m.clock.Now().Add(limit)

	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		m.mu.Lock()
		if _, contended := m.owners[key]; !contended {
			m.held[key] = request.Class
			m.mu.Unlock()
			return &fakeLock{manager: m, class: request.Class, key: key}, nil
		}
		m.mu.Unlock()
		if !m.clock.Now().Before(deadline) {
			return nil, fmt.Errorf("%w: %q を%vで取得できなかった", port.ErrLockTimeout, key, limit)
		}
		// fake clockを1秒進める。呼出し側がHoldExternalを解除しない限り、
		// deadlineへ到達してtimeoutする。
		m.clock.Advance(time.Second)
	}
}

func (m *LockManager) release(key string) error {
	if err := m.injector.Check(OpReleaseLock); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.held, key)
	return nil
}

// fakeLock は取得済みlockである。
type fakeLock struct {
	manager  *LockManager
	class    port.LockClass
	key      string
	released bool
	mu       sync.Mutex
}

func (l *fakeLock) Class() port.LockClass { return l.class }
func (l *fakeLock) Key() string           { return l.key }

// Release はlockを解放する。二重解放は成功として扱う。
//
// 呼出し側はdeferで解放を登録するため、明示解放とdeferの両方が走ることがある。
// 二重解放をerrorにすると、正常経路が失敗する。
func (l *fakeLock) Release() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.released {
		return nil
	}
	if err := l.manager.release(l.key); err != nil {
		return err
	}
	l.released = true
	return nil
}

var (
	_ port.LockManager = (*LockManager)(nil)
	_ port.Lock        = (*fakeLock)(nil)
)
