package port

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
)

// LockClass はlock順序を決める分類である（docs/02-architecture.md §12）。
//
// 同§が「ロック順序を固定し、デッドロックを防ぐ」として6分類を順番に定める。
// 数値の大小がそのまま取得順であり、逆順や飛び越しは許すが、小さいclassを
// 大きいclassの後に取ることを禁じる。
type LockClass uint8

// LockClass のexactly 6値。値は§12の並び順そのものである。
const (
	// ClassState は正本stateのlockである。
	ClassState LockClass = iota + 1
	// ClassCatalog はcatalog cacheのlockである。ToolID順で取る。
	ClassCatalog
	// ClassInstall は導入単位のlockである。ToolID、version、platform順で取る。
	ClassInstall
	// ClassStorage はtyped storageのlockである。ToolID、storage ID順で取る。
	ClassStorage
	// ClassSetup はsetup状態のlockである。
	ClassSetup
	// ClassShim はshim directoryのlockである。
	ClassShim
)

// LockClassCount は§12が定めるlock分類数である。
const LockClassCount = 6

var lockClassNames = map[LockClass]string{
	ClassState: "state", ClassCatalog: "catalog", ClassInstall: "install",
	ClassStorage: "storage", ClassSetup: "setup", ClassShim: "shim",
}

// String は§19のlock metadataへ書くrole名を返す。
func (c LockClass) String() string {
	if name, ok := lockClassNames[c]; ok {
		return name
	}
	return "unknown"
}

// IsValid は定義済みのclassかを返す。
func (c LockClass) IsValid() bool {
	_, ok := lockClassNames[c]
	return ok
}

// ParseLockClass はrole名をclassへ戻す。
func ParseLockClass(text string) (LockClass, error) {
	for class, name := range lockClassNames {
		if name == text {
			return class, nil
		}
	}
	return 0, fmt.Errorf("port: lock role %q は§12の6分類のいずれでもない", text)
}

// LockWaitDefault はlock取得の既定待ち時間である。
//
// docs/04-storage-and-data.md §19の「lock待ち既定30秒、cancel可能」と§21の
// 「lock wait 30秒」。利用者configで拡大できない。
const LockWaitDefault = 30 * time.Second

// LockRequest はlock取得の入力である。
type LockRequest struct {
	// Class は§12のlock分類である。取得順の判定に使う。
	Class LockClass
	// Qualifier は同じclass内で対象を区別する識別子である。
	//
	// §12がcatalogをToolID順、installをToolID/version/platform順、storageを
	// ToolID/storage ID順と定めるため、同一class内にも順序がある。state、setup、
	// shimは対象が1つなので空にする。
	Qualifier []string
	// Operation は変更transactionの識別子である。§19のmetadataへ書く。
	Operation domain.OperationID
	// Wait はlock取得を待つ上限である。zeroなら[LockWaitDefault]を使う。
	Wait time.Duration
}

// Lock は取得済みlockである。
//
// 解放はcancel/timeoutでも必ず行う（§12「cancel/timeoutでも取得済みlockを
// 必ず解放する」）。呼出し側は取得直後にdeferでReleaseを登録する。
type Lock interface {
	// Class は取得したlockの分類である。
	Class() LockClass
	// Key は`<class>`と`Qualifier`から決まるlock識別子である。
	//
	// §19の`locks/<role>.lock`のrole部分にあたる。
	Key() string
	// Release はlockを解放する。二重解放は成功として扱う。
	Release() error
}

// ErrLockOrder はlock順序違反である。
//
// §12の順序を破った取得は、実装の誤りであってrun時の競合状態ではない。
// 待てば解消する状態と区別するため、専用のerrorにする。
var ErrLockOrder = errors.New("port: lock順序違反")

// ErrLockTimeout はlock取得が待ち時間内に成立しなかったことを表す。
//
// 呼出し側は`E_LOCK_TIMEOUT`へ写す（docs/03-cli.md §7）。
var ErrLockTimeout = errors.New("port: lock取得がtimeoutした")

// LockManager はprocess間lockのportである（docs/02-architecture.md §4.1）。
//
// 排他性の正本はOS lock/handleであり、`locks/<role>.lock`のfile contentは
// 診断metadataだけである（§19）。PID不在やfile ageだけでactive lockを
// 破棄しない。
type LockManager interface {
	// Acquire はlockを取得する。
	//
	// ctxのcancelで待機を中断する。待ち時間を超えたら[ErrLockTimeout]、
	// §12の順序を破ったら[ErrLockOrder]を返す。
	Acquire(ctx context.Context, request LockRequest) (Lock, error)
	// Held は現在このprocessが保持しているlockをclass昇順・key昇順で返す。
	//
	// 順序検査と診断のために公開する。呼出し側が自前でlock台帳を持つと、
	// portの実装と食い違ったときに検出できない。
	Held() []LockClass
}
