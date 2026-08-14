package store

import (
	"fmt"
	"time"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain/port"
)

// LockDirectoryName はdata root相対のlock directoryである。
//
// docs/04-storage-and-data.md §19が保存先を`locks/<role>.lock`と定める。
const LockDirectoryName = "locks"

// LockFileSuffix はlock fileの拡張子である（§19）。
const LockFileSuffix = ".lock"

// LockMetadata は`locks/<role>.lock`のtyped表現である（§19）。
//
// 同§が「OS lockを正本とし、file contentは診断metadataだけ」と定める。
// **排他性をこのfileで判定しない。** PID不在やfile ageだけでactive lockを
// 破棄しないのも同じ理由である（docs/02-architecture.md §12）。
type LockMetadata struct {
	// LockID はこの取得を一意に識別する128 bit hexである。
	LockID string
	// Role は§12のlock分類と対象から決まるlock識別子である。
	Role string
	// Operation は変更transactionの識別子である。
	Operation domain.OperationID
	// PID はlockを取得したprocessのIDである。診断表示にだけ使う。
	PID int64
	// CreatedAt はlockを取得した時刻である。
	CreatedAt time.Time
}

// lockMetadataJSON は§19のexact key集合である。
type lockMetadataJSON struct {
	Schema      *int64  `json:"schema"`
	LockID      *string `json:"lock_id"`
	Role        *string `json:"role"`
	OperationID *string `json:"operation_id"`
	PID         *int64  `json:"pid"`
	CreatedAt   *string `json:"created_at"`
}

// ParseLockMetadata はlock fileを読む（§19）。
func ParseLockMetadata(data []byte) (LockMetadata, *domain.Error) {
	var file lockMetadataJSON
	if err := decodeJSON(data, &file); err != nil {
		return LockMetadata{}, lockError(err)
	}
	value, err := buildLockMetadata(file)
	if err != nil {
		return LockMetadata{}, lockError(err)
	}
	return value, nil
}

// EncodeLockMetadata はlock fileを書き出す（§19）。
func EncodeLockMetadata(value LockMetadata) ([]byte, *domain.Error) {
	file := lockMetadataJSONOf(value)
	if _, err := buildLockMetadata(file); err != nil {
		return nil, lockError(err)
	}
	data, encodeErr := encodeJSON(file)
	if encodeErr != nil {
		return nil, lockError(encodeErr)
	}
	return data, nil
}

// lockError はlock metadataの破損を表すtyped errorを作る。
//
// lock fileはgdtvm自身が書く診断metadataであり、読めないことはstateの破損では
// なく内部誤りである。加えて、**読めなくてもlockの排他性は失われない**（OS lock
// が正本）ため、`E_STATE_CORRUPT`にして操作全体を止める扱いにしない。
func lockError(cause error) *domain.Error {
	return typedError(domain.CodeInternal, "lock.metadata_invalid", domain.RoleState, cause)
}

func buildLockMetadata(file lockMetadataJSON) (LockMetadata, error) {
	var value LockMetadata
	if err := requireSchema("schema", file.Schema); err != nil {
		return value, err
	}
	lockID, err := requireIDField("lock_id", file.LockID)
	if err != nil {
		return value, err
	}
	role, err := requirePresent("role", file.Role)
	if err != nil {
		return value, err
	}
	if err := checkLockRole(role); err != nil {
		return value, err
	}
	operationText, err := requirePresent("operation_id", file.OperationID)
	if err != nil {
		return value, err
	}
	operation, err := domain.ParseOperationID(operationText)
	if err != nil {
		return value, fmt.Errorf("operation_id: %w", err)
	}
	if file.PID == nil {
		return value, fmt.Errorf("pidが無い")
	}
	// PIDは診断表示にだけ使う。0以下は正当なprocess IDでない。
	if *file.PID <= 0 {
		return value, fmt.Errorf("pidが正のintegerでない（%d）", *file.PID)
	}
	if _, err := requireNonNegative("pid", *file.PID); err != nil {
		return value, err
	}
	createdAt, err := requireTimestampField("created_at", file.CreatedAt)
	if err != nil {
		return value, err
	}
	return LockMetadata{
		LockID: lockID, Role: role, Operation: operation,
		PID: *file.PID, CreatedAt: createdAt,
	}, nil
}

// checkLockRole はroleが§12の6分類から組み立てられた形かを確かめる。
//
// §19が「role grammarとlock順は§5と[02-architecture.md]§12」と定める。
// roleの先頭segmentがclass名であることと、[port.LockKey]が作る形と一致する
// ことを検査する。未知のclassのlock fileを診断へそのまま出すと、存在しない
// lockを保持しているように見える。
func checkLockRole(role string) error {
	class, qualifier, err := splitLockRole(role)
	if err != nil {
		return err
	}
	rebuilt, err := port.LockKey(class, qualifier)
	if err != nil {
		return fmt.Errorf("role %q: %w", role, err)
	}
	if rebuilt != role {
		return fmt.Errorf("roleが正規形でない（%q、正規形は%q）", role, rebuilt)
	}
	return nil
}

func splitLockRole(role string) (port.LockClass, []string, error) {
	if role == "" {
		return 0, nil, fmt.Errorf("roleが空")
	}
	parts := port.SplitLockKey(role)
	class, err := port.ParseLockClass(parts[0])
	if err != nil {
		return 0, nil, fmt.Errorf("role %q: %w", role, err)
	}
	return class, parts[1:], nil
}

func lockMetadataJSONOf(value LockMetadata) lockMetadataJSON {
	schema := int64(SchemaVersion)
	lockID := value.LockID
	role := value.Role
	operation := value.Operation.String()
	pid := value.PID
	createdAt := formatTimestamp(value.CreatedAt)
	return lockMetadataJSON{
		Schema: &schema, LockID: &lockID, Role: &role,
		OperationID: &operation, PID: &pid, CreatedAt: &createdAt,
	}
}
