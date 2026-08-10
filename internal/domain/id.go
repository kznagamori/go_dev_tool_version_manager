package domain

import (
	"encoding/hex"
	"fmt"
	"regexp"
)

// idRe は128 bit random IDの正規形である（docs/04-storage-and-data.md §6）。
//
// 同§の表は「operation/install/root ID」を「128 bit randomの32 lowercase hex」と
// 定める。大文字hexやhyphen付きUUID表記は受理しない。
var idRe = regexp.MustCompile(`^[0-9a-f]{32}$`)

// IDLength は128 bit IDの小文字hex表記の長さである。
const IDLength = 32

// IDByteLength はID生成に必要なrandom byte数である。
const IDByteLength = 16

// InvocationID はCLI呼出し1回を識別するIDである（docs/02-architecture.md §5）。
//
// `RequestContext.InvocationID`、CLI JSON envelope、structured log、Planの
// `invocation_id`で同じ値を使う。1回の呼出しの中で複数のoperationが走っても
// 変わらない。
type InvocationID struct {
	hex string
}

// OperationID は変更transaction 1件を識別するIDである。
//
// Plan、lock metadata、structured log、progressの`operation_id`で使う
// （docs/04-storage-and-data.md §15・§18・§19）。読取りoperationも進捗と
// logを紐付けるために持つ。
type OperationID struct {
	hex string
}

// NewInvocationID は128 bitのrandom byte列からInvocationIDを作る。
//
// [port.Random] が返したbyte列を正規形へ変換する唯一の入口である。encodingを
// domain側に置くことで、port実装ごとに大文字hexやUUID表記へぶれないようにする。
func NewInvocationID(raw [IDByteLength]byte) InvocationID {
	return InvocationID{hex: hex.EncodeToString(raw[:])}
}

// NewOperationID は128 bitのrandom byte列からOperationIDを作る。
func NewOperationID(raw [IDByteLength]byte) OperationID {
	return OperationID{hex: hex.EncodeToString(raw[:])}
}

// ParseInvocationID は32桁小文字hexをInvocationIDへ変換する。
func ParseInvocationID(text string) (InvocationID, error) {
	if err := validateID("invocation", text); err != nil {
		return InvocationID{}, err
	}
	return InvocationID{hex: text}, nil
}

// ParseOperationID は32桁小文字hexをOperationIDへ変換する。
func ParseOperationID(text string) (OperationID, error) {
	if err := validateID("operation", text); err != nil {
		return OperationID{}, err
	}
	return OperationID{hex: text}, nil
}

func validateID(kind, text string) error {
	if !idRe.MatchString(text) {
		return fmt.Errorf("domain: %s IDは128 bitの32桁小文字hexだが %q が渡された", kind, text)
	}
	return nil
}

// String は32桁小文字hex表記を返す。
func (i InvocationID) String() string { return i.hex }

// IsZero はParseを通していない値かどうかを返す。
func (i InvocationID) IsZero() bool { return i.hex == "" }

// String は32桁小文字hex表記を返す。
func (o OperationID) String() string { return o.hex }

// IsZero はParseを通していない値かどうかを返す。
func (o OperationID) IsZero() bool { return o.hex == "" }
