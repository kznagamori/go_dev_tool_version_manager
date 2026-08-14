package fake

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain/port"
)

func newLocks(t *testing.T) *LockManager {
	t.Helper()
	set := NewSet()
	return set.LockManager
}

func acquire(t *testing.T, manager *LockManager, class port.LockClass, qualifier ...string) port.Lock {
	t.Helper()
	lock, err := manager.Acquire(context.Background(), port.LockRequest{
		Class: class, Qualifier: qualifier,
	})
	if err != nil {
		t.Fatalf("Acquire(%v, %v) = %v", class, qualifier, err)
	}
	return lock
}

// TestLockOrderIsEnforced はdocs/02-architecture.md §12の順序を固定する。
//
// fakeが順序違反を素通りさせると、本番でだけdeadlockするcodeがtestを通る。
func TestLockOrderIsEnforced(t *testing.T) {
	manager := newLocks(t)
	// §12の順で取れる。
	state := acquire(t, manager, port.ClassState)
	catalog := acquire(t, manager, port.ClassCatalog, "node")
	install := acquire(t, manager, port.ClassInstall, "node", "22.18.0", "linux-amd64-glibc")
	storage := acquire(t, manager, port.ClassStorage, "node", "global-packages")
	setup := acquire(t, manager, port.ClassSetup)
	shim := acquire(t, manager, port.ClassShim)
	for _, lock := range []port.Lock{shim, setup, storage, install, catalog, state} {
		if err := lock.Release(); err != nil {
			t.Fatalf("Release = %v", err)
		}
	}

	// 逆順は拒否される。
	manager = newLocks(t)
	acquire(t, manager, port.ClassShim)
	_, err := manager.Acquire(context.Background(), port.LockRequest{Class: port.ClassState})
	if !errors.Is(err, port.ErrLockOrder) {
		t.Errorf("shimの後のstate = %v, want ErrLockOrder", err)
	}

	// 飛び越しは許す（§12は全classを取ることを求めていない）。
	manager = newLocks(t)
	acquire(t, manager, port.ClassState)
	acquire(t, manager, port.ClassShim)
}

// TestLockOrderWithinClassFollowsKey は§12の同一class内の順序を固定する。
//
// catalogはToolID順、installはToolID/version/platform順、storageはToolID/
// storage ID順である。
func TestLockOrderWithinClassFollowsKey(t *testing.T) {
	manager := newLocks(t)
	acquire(t, manager, port.ClassCatalog, "go")
	acquire(t, manager, port.ClassCatalog, "node")

	manager = newLocks(t)
	acquire(t, manager, port.ClassCatalog, "node")
	_, err := manager.Acquire(context.Background(), port.LockRequest{
		Class: port.ClassCatalog, Qualifier: []string{"go"},
	})
	if !errors.Is(err, port.ErrLockOrder) {
		t.Errorf("catalog降順 = %v, want ErrLockOrder", err)
	}

	// installはversionまで含めて順序を持つ。
	manager = newLocks(t)
	acquire(t, manager, port.ClassInstall, "node", "20.0.0", "linux-amd64-glibc")
	acquire(t, manager, port.ClassInstall, "node", "22.18.0", "linux-amd64-glibc")
}

// TestLockRejectsDoubleAcquire は同じlockの二重取得を固定する。
func TestLockRejectsDoubleAcquire(t *testing.T) {
	manager := newLocks(t)
	acquire(t, manager, port.ClassCatalog, "node")
	_, err := manager.Acquire(context.Background(), port.LockRequest{
		Class: port.ClassCatalog, Qualifier: []string{"node"},
	})
	if err == nil {
		t.Error("同じlockの二重取得が通った")
	}
	// 順序違反とは別のerrorである。二重取得は待てば解消しない。
	if errors.Is(err, port.ErrLockTimeout) {
		t.Error("二重取得がtimeoutとして報告された")
	}
}

// TestLockReleaseIsIdempotent は二重解放が成功することを固定する。
//
// 呼出し側は明示解放とdeferの両方を書くため、二重解放をerrorにすると
// 正常経路が失敗する。
func TestLockReleaseIsIdempotent(t *testing.T) {
	manager := newLocks(t)
	lock := acquire(t, manager, port.ClassState)
	if err := lock.Release(); err != nil {
		t.Fatalf("1回目のRelease = %v", err)
	}
	if err := lock.Release(); err != nil {
		t.Errorf("2回目のRelease = %v, want nil", err)
	}
	if len(manager.HeldKeys()) != 0 {
		t.Errorf("解放後もlockが残っている: %v", manager.HeldKeys())
	}
	// 解放後は同じlockを取り直せる。
	acquire(t, manager, port.ClassState)
}

// TestLockContentionTimesOut はdocs/04-storage-and-data.md §19の待ち時間を固定する。
func TestLockContentionTimesOut(t *testing.T) {
	set := NewSet()
	manager := set.LockManager
	key, err := port.LockKey(port.ClassCatalog, []string{"node"})
	if err != nil {
		t.Fatalf("LockKey = %v", err)
	}
	manager.HoldExternal(key)

	_, acquireErr := manager.Acquire(context.Background(), port.LockRequest{
		Class: port.ClassCatalog, Qualifier: []string{"node"}, Wait: 5 * time.Second,
	})
	if !errors.Is(acquireErr, port.ErrLockTimeout) {
		t.Fatalf("競合時のAcquire = %v, want ErrLockTimeout", acquireErr)
	}
	if manager.Waits() == 0 {
		t.Error("待機経路を通っていない")
	}
	// 他processが解放すれば取得できる。timeoutが恒久的な失敗でないことの確認。
	manager.ReleaseExternal(key)
	acquire(t, manager, port.ClassCatalog, "node")
}

// TestLockDefaultWaitIs30Seconds は§21の「lock wait 30秒」を固定する。
func TestLockDefaultWaitIs30Seconds(t *testing.T) {
	if port.LockWaitDefault != 30*time.Second {
		t.Fatalf("LockWaitDefault = %v, want 30s", port.LockWaitDefault)
	}
	set := NewSet()
	manager := set.LockManager
	key, _ := port.LockKey(port.ClassCatalog, []string{"node"})
	manager.HoldExternal(key)
	start := set.Clock.Now()

	// Waitを指定しなければ既定30秒まで待つ。
	_, err := manager.Acquire(context.Background(), port.LockRequest{
		Class: port.ClassCatalog, Qualifier: []string{"node"},
	})
	if !errors.Is(err, port.ErrLockTimeout) {
		t.Fatalf("Acquire = %v, want ErrLockTimeout", err)
	}
	elapsed := set.Clock.Now().Sub(start)
	if elapsed < port.LockWaitDefault {
		t.Errorf("待ち時間 = %v, want >= %v", elapsed, port.LockWaitDefault)
	}
}

// TestLockAcquireHonorsCancel は§19の「cancel可能」を固定する。
func TestLockAcquireHonorsCancel(t *testing.T) {
	set := NewSet()
	manager := set.LockManager
	key, _ := port.LockKey(port.ClassCatalog, []string{"node"})
	manager.HoldExternal(key)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := manager.Acquire(ctx, port.LockRequest{
		Class: port.ClassCatalog, Qualifier: []string{"node"},
	})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("cancel済みctxのAcquire = %v, want context.Canceled", err)
	}
	// cancelはtimeoutと区別される。利用者の中断とlock競合は別の事象である。
	if errors.Is(err, port.ErrLockTimeout) {
		t.Error("cancelがtimeoutとして報告された")
	}
}

// TestLockHeldReportsOrder は保持中lockがclass昇順・key昇順で返ることを固定する。
func TestLockHeldReportsOrder(t *testing.T) {
	manager := newLocks(t)
	acquire(t, manager, port.ClassState)
	acquire(t, manager, port.ClassCatalog, "go")
	acquire(t, manager, port.ClassCatalog, "node")
	acquire(t, manager, port.ClassShim)

	keys := manager.HeldKeys()
	want := []string{"state", "catalog~go", "catalog~node", "shim"}
	if len(keys) != len(want) {
		t.Fatalf("HeldKeys = %v, want %v", keys, want)
	}
	for index, key := range want {
		if keys[index] != key {
			t.Errorf("HeldKeys[%d] = %q, want %q", index, keys[index], key)
		}
	}
	classes := manager.Held()
	if len(classes) != len(want) || classes[0] != port.ClassState || classes[3] != port.ClassShim {
		t.Errorf("Held = %v", classes)
	}
}

// TestLockFailureInjection は取得・解放の失敗注入が効くことを固定する。
func TestLockFailureInjection(t *testing.T) {
	set := NewSet()
	set.Injector.FailOnce(OpAcquireLock, errors.New("acquire失敗"))
	if _, err := set.LockManager.Acquire(context.Background(), port.LockRequest{
		Class: port.ClassState,
	}); err == nil {
		t.Error("Acquireの失敗注入が効いていない")
	}

	set = NewSet()
	lock := acquire(t, set.LockManager, port.ClassState)
	set.Injector.FailOnce(OpReleaseLock, errors.New("release失敗"))
	if err := lock.Release(); err == nil {
		t.Error("Releaseの失敗注入が効いていない")
	}
	// 解放に失敗したlockは保持中のままである。失敗を握りつぶして解放済みに
	// すると、実際には残っているlockを解放済みと誤認する。
	if len(set.LockManager.HeldKeys()) != 1 {
		t.Errorf("解放失敗後のHeldKeys = %v, want 1件", set.LockManager.HeldKeys())
	}
}

// TestLockRejectsInvalidRequest は不正なrequestを固定する。
func TestLockRejectsInvalidRequest(t *testing.T) {
	manager := newLocks(t)
	tests := []struct {
		name    string
		request port.LockRequest
	}{
		{"class未設定", port.LockRequest{}},
		{"class範囲外", port.LockRequest{Class: port.LockClass(99)}},
		{"qualifier必須classで空", port.LockRequest{Class: port.ClassInstall}},
		{"qualifier不要classで指定", port.LockRequest{Class: port.ClassState, Qualifier: []string{"x"}}},
		{"qualifierが不正", port.LockRequest{Class: port.ClassCatalog, Qualifier: []string{"a/b"}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := manager.Acquire(context.Background(), test.request); err == nil {
				t.Error("Acquire = nil, want error")
			}
		})
	}
}
