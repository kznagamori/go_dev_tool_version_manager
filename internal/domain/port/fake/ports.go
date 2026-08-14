package fake

import (
	"time"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain/port"
)

// DefaultNow はfake Clockの既定起点を返す。
//
// 固定値にするのは、時刻がtest結果へ現れる箇所（receiptのinstalled_at、
// catalog cacheの有効期限）をgolden比較できるようにするためである。
//
// package-level varではなくfunctionにしている。time.Timeはconstにできず、varに
// するとtestが起点を書き換えて他のtestへ影響させられるpackage global mutable
// stateになるためである（docs/02-architecture.md §4）。
func DefaultNow() time.Time {
	return time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
}

// Set は9 portのfake一式と、それらが共有する失敗注入器である。
type Set struct {
	Injector      *Injector
	Clock         *Clock
	FileSystem    *FileSystem
	HTTPClient    *HTTPClient
	LinkManager   *LinkManager
	LockManager   *LockManager
	Logger        *Logger
	ProcessRunner *ProcessRunner
	Random        *Random
	UserLookup    *UserLookup
}

// NewSet は9 portのfakeを1つのInjectorで束ねて作る。
//
// Injectorを共有するのは、「downloadが失敗した後にstagingのcleanupが走ったか」
// のようなport横断の順序をひとつの記録で検査できるようにするためである。
func NewSet() *Set {
	injector := NewInjector()
	fsys := NewFileSystem(injector)
	clock := NewClock(DefaultNow())
	return &Set{
		Injector:      injector,
		Clock:         clock,
		FileSystem:    fsys,
		HTTPClient:    NewHTTPClient(injector),
		LinkManager:   NewLinkManager(fsys),
		LockManager:   NewLockManager(injector, clock),
		Logger:        NewLogger(),
		ProcessRunner: NewProcessRunner(injector),
		Random:        NewRandom(injector),
		UserLookup: NewUserLookup(injector, port.UserIdentity{
			Name: "testuser",
			ID:   "1000",
			Home: "/home/testuser",
		}),
	}
}

// Ports は注入用のport.Portsを返す。
func (s *Set) Ports() port.Ports {
	return port.Ports{
		Clock:         s.Clock,
		FileSystem:    s.FileSystem,
		HTTPClient:    s.HTTPClient,
		LinkManager:   s.LinkManager,
		LockManager:   s.LockManager,
		Logger:        s.Logger,
		ProcessRunner: s.ProcessRunner,
		Random:        s.Random,
		UserLookup:    s.UserLookup,
	}
}
