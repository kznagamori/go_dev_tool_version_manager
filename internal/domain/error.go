package domain

import (
	"errors"
	"fmt"
	"strings"
)

// MessageIDInternal は想定外の内部失敗へ付けるmessage IDである。
//
// docs/02-architecture.md §14は「想定外の内部失敗だけは公開code `E_INTERNAL`、
// 終了code 1へ変換する」と定めるため、その変換路だけは実装側でIDを固定する必要が
// ある。他のcodeに対応するIDはmessage catalog（docs/04-storage-and-data.md §20)を
// 作るtaskで決める。
const MessageIDInternal = "error.internal"

// nonRetryableCodes はdocs/02-architecture.md §14が`Retryable=true`を禁じる
// exactly 8件である。
//
// checksum、archive/path安全性、registry/definition、state/receiptの破損は、
// 同じ操作を再実行しても同じ結果になるか、再実行そのものが危険である。利用者へ
// 「再実行できる」と伝えないことで、破損状態のまま繰り返させない。
var nonRetryableCodes = map[ErrorCode]struct{}{
	CodeChecksumMismatch:  {},
	CodeArchiveUnsafe:     {},
	CodePathUnsafe:        {},
	CodePathConflict:      {},
	CodeRegistryInvalid:   {},
	CodeDefinitionInvalid: {},
	CodeStateCorrupt:      {},
	CodeReceiptInvalid:    {},
}

// Error は公開境界へ出すtyped errorである（docs/02-architecture.md §14）。
//
// 表示文を持たず、stable code、message ID、scalar parameterだけを返す。文は
// presentation層がmessage catalogから生成する。CLI human、CLI JSON、shimで
// 同じ失敗条件が同じcodeとmessage IDになることを、この型で担保する。
//
// [BuildInfo] と同じく、field組立てとValidateを分けている。errorを作る箇所は
// 例外処理の途中にあり、そこで多段のconstructor errorを扱うと本来のerrorが
// 埋もれるためである。公開境界（CLI adapter、JSON codec）が出力前にValidateする。
type Error struct {
	// Code はstable error codeである。閉じた34件だけを使う。
	Code ErrorCode
	// MessageID は表示文を引くkeyである。
	MessageID MessageID
	// Parameters はmessage templateへ渡すscalarである。secretやpath raw contentを
	// 入れない（docs/10-security.md §9.2）。
	Parameters Parameters
	// Operation は失敗したoperation名である。空を許す。
	Operation string
	// Tool は対象tool。tool非依存の失敗では未設定にする。
	Tool ToolID
	// Version は対象version。version非依存の失敗では未設定にする。
	Version Version
	// PathRole は対象pathのlogical roleである。絶対pathを公開境界へ出さずに
	// 対象を特定できるようにするための項目であり、path文字列そのものは持たない
	// （docs/02-architecture.md §14）。
	PathRole PathRole
	// Retryable は利用者が状態を直したあと再実行できるかを表す。自動retryの
	// 対象という意味ではない。
	Retryable bool
	// Cause はdebug log用の内部errorである。JSON/public messageへ直接
	// serializeしない（§14）。
	Cause error
}

// Internal は想定外の内部失敗を公開可能なErrorへ変換する（§14）。
//
// 分類できない失敗をそのまま公開境界へ出さないための最後の受け皿である。
// causeは保持するがJSONや利用者向けmessageへは出さない。
func Internal(cause error) *Error {
	// 定数からのparseは失敗しない。失敗した場合でもzero MessageIDのままにして
	// Validateで検出できるようにし、error処理の中でpanicさせない。
	messageID, _ := ParseMessageID(MessageIDInternal)
	return &Error{
		Code:      CodeInternal,
		MessageID: messageID,
		Retryable: false,
		Cause:     cause,
	}
}

// Error はGo側のerror文字列を返す。
//
// Causeを含めない。呼出し側がそのまま利用者へ表示しても内部errorが漏れないように
// するためである。causeは[Error.Unwrap]と構造化logから辿る。
func (e *Error) Error() string {
	var b strings.Builder
	b.WriteString(string(e.Code))
	if !e.MessageID.IsZero() {
		b.WriteString(" (")
		b.WriteString(e.MessageID.String())
		b.WriteString(")")
	}
	if e.Operation != "" {
		b.WriteString(" operation=")
		b.WriteString(e.Operation)
	}
	if !e.Tool.IsZero() {
		b.WriteString(" tool=")
		b.WriteString(e.Tool.String())
	}
	if !e.Version.IsZero() {
		b.WriteString(" version=")
		b.WriteString(e.Version.String())
	}
	if e.PathRole != "" {
		b.WriteString(" path_role=")
		b.WriteString(string(e.PathRole))
	}
	return b.String()
}

// Unwrap は内部errorを返し、errors.Is/errors.Asで辿れるようにする。
func (e *Error) Unwrap() error { return e.Cause }

// ExitCode は対応する終了codeを返す（docs/03-cli.md §7）。
func (e *Error) ExitCode() int { return e.Code.ExitCode() }

// Validate は公開境界へ出す前の不変条件を検査する。
//
// 誤りを1件目で打ち切らず全件返す。errorを組み立てる箇所は例外処理の途中にあり、
// 直しながら何度も失敗を再現させたくないためである。
func (e *Error) Validate() error {
	var errs []error

	if !e.Code.IsKnown() {
		errs = append(errs, fmt.Errorf(
			"domain: error code %q はdocs/03-cli.md §7の34件に含まれない", e.Code))
	}
	if e.MessageID.IsZero() {
		errs = append(errs, errors.New("domain: typed errorのmessage IDが未設定"))
	}
	if err := e.Parameters.Validate(); err != nil {
		errs = append(errs, err)
	}
	if _, forbidden := nonRetryableCodes[e.Code]; forbidden && e.Retryable {
		errs = append(errs, fmt.Errorf(
			"domain: %s はretryable=trueにできない（docs/02-architecture.md §14の8件）", e.Code))
	}
	if e.PathRole != "" {
		if _, ok := pathRoles[e.PathRole]; !ok {
			errs = append(errs, fmt.Errorf(
				"domain: path_role %q は§17.2の22値に含まれない", e.PathRole))
		}
	}

	return errors.Join(errs...)
}

// IsRetryableAllowed は指定codeにretryable=trueを付けてよいかを返す。
func IsRetryableAllowed(code ErrorCode) bool {
	_, forbidden := nonRetryableCodes[code]
	return !forbidden
}

// NonRetryableCodes は`Retryable=true`を禁じるcodeを§14の記載順で返す。
func NonRetryableCodes() []ErrorCode {
	return []ErrorCode{
		CodeChecksumMismatch, CodeArchiveUnsafe, CodePathUnsafe, CodePathConflict,
		CodeRegistryInvalid, CodeDefinitionInvalid, CodeStateCorrupt, CodeReceiptInvalid,
	}
}

// CodeOf はerror chainから最初のtyped error codeを取り出す。
//
// 見つからない場合は`E_INTERNAL`とfalseを返す。未分類のerrorを公開境界へ
// 出さないという§14の要求に対して、呼出し側がfail closedに倒せるようにする。
func CodeOf(err error) (ErrorCode, bool) {
	var typed *Error
	if errors.As(err, &typed) && typed.Code.IsKnown() {
		return typed.Code, true
	}
	return CodeInternal, false
}

// ExitCodeOf はerror chainから終了codeを決める。
//
// typed errorでない、またはcodeが閉じた集合の外なら終了code 1を返す（§14）。
func ExitCodeOf(err error) int {
	if err == nil {
		return ExitSuccess
	}
	code, _ := CodeOf(err)
	return code.ExitCode()
}
