package fake

import (
	"sync"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain/port"
)

// UserLookup操作名。
const (
	OpUserCurrent = "user.Current"
	OpUserOwnerOf = "user.OwnerOf"
)

// UserLookup は決定的なport.UserLookupである。
type UserLookup struct {
	mu       sync.Mutex
	identity port.UserIdentity
	owners   map[string]string
	injector *Injector
}

var _ port.UserLookup = (*UserLookup)(nil)

// NewUserLookup は指定identityを返すUserLookupを作る。
func NewUserLookup(injector *Injector, identity port.UserIdentity) *UserLookup {
	if injector == nil {
		injector = NewInjector()
	}
	return &UserLookup{
		identity: identity,
		owners:   make(map[string]string),
		injector: injector,
	}
}

// Injector は失敗注入器を返す。
func (u *UserLookup) Injector() *Injector { return u.injector }

// SetOwner はpathの所有者を登録する。
func (u *UserLookup) SetOwner(path, ownerID string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.owners[clean(path)] = ownerID
}

// Current は実行中の実userを返す。
func (u *UserLookup) Current() (port.UserIdentity, error) {
	if err := u.injector.Check(OpUserCurrent); err != nil {
		return port.UserIdentity{}, err
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.identity, nil
}

// OwnerOf はpathの所有者IDを返す。
//
// 未登録pathは実行中userの所有として扱う。所有者検査のtestで、
// 「他userのfile」だけを明示登録すれば足りるようにするためである。
func (u *UserLookup) OwnerOf(path string) (string, error) {
	if err := u.injector.Check(OpUserOwnerOf); err != nil {
		return "", err
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	if owner, ok := u.owners[clean(path)]; ok {
		return owner, nil
	}
	return u.identity.ID, nil
}
