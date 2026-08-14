package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/pelletier/go-toml/v2"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
)

// SchemaVersion は本packageが作成・読込みするschema revisionである。
//
// docs/04-storage-and-data.md §7の「schema revisionはすべて`1`」と、§22の
// 「v0.1のclientはschema 1だけを作成・読込みする。未知future schemaを推測して
// 読まない」。migration機構はdocs/15-deferred.mdへ延期されている。
const SchemaVersion = 1

// 各fileの上限（docs/04-storage-and-data.md §21）。利用者configで拡大できない。
const (
	// StateFileMaxBytes はsetup/selection等のstate TOML 1 fileの上限である。
	StateFileMaxBytes = 1 << 20
	// LogLineMaxBytes はstructured log 1行の上限である。
	//
	// fields件数の上限は[port.LogFieldsMax]が正本である（P1-04）。
	LogLineMaxBytes = 256 << 10
)

// JSONMaxSafeInteger はJSON整数の上限である（docs/04-storage-and-data.md §7）。
//
// 同§が「byte count/revisionは非負integer、JSONは2^53-1以下」と定める。
// JSON側の実装がdoubleで読む可能性があるため、往復で値が変わらない範囲へ限る。
// TOMLにこの制約は無いが、同じ値が両形式へ現れるため一律に適用する。
const JSONMaxSafeInteger = int64(1)<<53 - 1

// utf8BOM はUTF-8 BOMのbyte列である。§7が「BOMなし」と定めるため入力で拒否する。
var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// relativePathRe はTOML/JSONへ書くPOSIX relative pathのgrammarである。
//
// §7が「TOML/JSONではlogical role＋POSIX relativeを基本」と定める。区切りは`/`
// だけで、絶対path、`.`、`..`、空component、backslashを拒否する。Windowsでも
// 保存形式はPOSIX relativeで統一し、絶対化はruntimeが行う（§14）。
var relativePathRe = regexp.MustCompile(`^[^/\\]+(?:/[^/\\]+)*$`)

// decodeTOML はTOML bytesをstrictにdecodeする。
//
// DisallowUnknownFieldsがunknown keyを、go-toml自身が重複key/table、型違い、
// trailing dataを拒否する。§7の「unknown/duplicate key、型違い、enum外、
// 上限超過、trailing dataを拒否する」のうちenumと上限は各file形式が扱う。
func decodeTOML(data []byte, target any) error {
	if err := checkTextInput(data); err != nil {
		return err
	}
	decoder := toml.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return describeTOMLError(err)
	}
	return nil
}

// describeTOMLError はgo-tomlのerrorを行・列付きの日本語diagnosticへ変換する。
//
// docs/05-configuration.md §1が位置付きの拒否を求める。state fileにも同じ
// 診断を出すのは、破損時に利用者が該当行を特定できるようにするためである。
func describeTOMLError(err error) error {
	var strict *toml.StrictMissingError
	if errors.As(err, &strict) {
		if len(strict.Errors) > 0 {
			first := strict.Errors[0]
			row, column := first.Position()
			return fmt.Errorf("%d行%d列: 未知のkey %q", row, column, first.Key())
		}
		return errors.New("未知のkeyがある")
	}
	var decode *toml.DecodeError
	if errors.As(err, &decode) {
		row, column := decode.Position()
		return fmt.Errorf("%d行%d列: %s", row, column, decode.Error())
	}
	return err
}

// decodeJSON はJSON bytesをstrictにdecodeする。
//
// `encoding/json`は既定でunknown fieldを無視し、重複keyを後勝ちで受理し、
// 1 documentの後ろにあるdataを見ない。§7はそのいずれも拒否するため、
// DisallowUnknownFieldsに加えてtrailing dataと重複keyを別途検査する。
func decodeJSON(data []byte, target any) error {
	if err := checkTextInput(data); err != nil {
		return err
	}
	if err := checkDuplicateJSONKeys(data); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	// 1 documentのあとに値が続かないことを確かめる。§17が「stdoutは完了時の
	// exactly 1 JSON document」と定めており、連結documentを受理しない。
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("JSON documentの後ろにdataが続いている")
		}
		return err
	}
	return nil
}

// checkTextInput はBOMと不正UTF-8を拒否する（§7）。
func checkTextInput(data []byte) error {
	if bytes.HasPrefix(data, utf8BOM) {
		return errors.New("UTF-8 BOMは許可されない")
	}
	if !utf8.Valid(data) {
		return errors.New("正しいUTF-8でない")
	}
	return nil
}

// checkDuplicateJSONKeys は同一object内の重複keyを拒否する。
//
// `encoding/json`は重複keyを後勝ちで受理するため、tokenを走査して自前で
// 検出する。後勝ちを許すと、同じ文書が実装ごとに別の値へ読めてしまう。
func checkDuplicateJSONKeys(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	// object階層ごとに出現済みkeyを持つ。arrayの層はnilを積んで階層だけ合わせる。
	var stack []map[string]struct{}
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		delim, isDelim := token.(json.Delim)
		switch {
		case isDelim && delim == '{':
			stack = append(stack, map[string]struct{}{})
		case isDelim && delim == '[':
			stack = append(stack, nil)
		case isDelim && (delim == '}' || delim == ']'):
			stack = stack[:len(stack)-1]
		default:
			// object直下のstring tokenはkeyかvalueのどちらか。json.Decoderは
			// keyとvalueを交互に返すため、seenへの登録で区別できる。
			key, isString := token.(string)
			if !isString || len(stack) == 0 || stack[len(stack)-1] == nil {
				continue
			}
			seen := stack[len(stack)-1]
			if _, duplicate := seen[key]; duplicate {
				return fmt.Errorf("重複するJSON key %q", key)
			}
			seen[key] = struct{}{}
			// value側を読み飛ばす。valueがstringのときにkeyと誤認しないため。
			if err := skipJSONValue(decoder, &stack); err != nil {
				return err
			}
		}
	}
}

// skipJSONValue はkey直後のvalueを1件読み飛ばす。
//
// 複合値の場合はstackへ積んで階層を合わせ、走査本体が閉じdelimで戻す。
func skipJSONValue(decoder *json.Decoder, stack *[]map[string]struct{}) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	if delim, isDelim := token.(json.Delim); isDelim {
		switch delim {
		case '{':
			*stack = append(*stack, map[string]struct{}{})
		case '[':
			*stack = append(*stack, nil)
		}
	}
	return nil
}

// encodeTOML はtyped valueをTOML bytesへ変換し、末尾LFを1つに揃える（§7）。
func encodeTOML(value any) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := toml.NewEncoder(&buffer)
	encoder.SetIndentTables(false)
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}
	return normalizeTrailingLF(buffer.Bytes()), nil
}

// encodeJSON はtyped valueをJSON bytesへ変換し、末尾LFを1つに揃える（§7）。
//
// HTML escapeを切るのは、`<`や`&`を含むURLが`<`へ化けるとupstreamの
// URLと文字列一致しなくなるためである。
func encodeJSON(value any) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}
	return normalizeTrailingLF(buffer.Bytes()), nil
}

// normalizeTrailingLF は末尾のLFをちょうど1つにする（§7）。
func normalizeTrailingLF(data []byte) []byte {
	trimmed := bytes.TrimRight(data, "\n")
	return append(trimmed, '\n')
}

// requireSchema はschema revisionが1であることを確かめる（§7・§22）。
func requireSchema(field string, value *int64) error {
	if value == nil {
		return fmt.Errorf("%sが無い", field)
	}
	if *value != SchemaVersion {
		return fmt.Errorf("%sは%dだけを読める（%d）", field, SchemaVersion, *value)
	}
	return nil
}

// requireSize はfile sizeが上限内であることを確かめる（§21）。
func requireSize(kind string, data []byte, max int) error {
	if len(data) > max {
		return fmt.Errorf("%sが%d byteを超える（%d byte）", kind, max, len(data))
	}
	return nil
}

// parseTimestamp はUTC RFC 3339・秒精度以上・offset `Z`のtimestampを読む（§7）。
//
// offsetを`Z`に限るのは、同じ時刻が`+09:00`と`Z`の2通りで書かれると、byte列の
// 比較でstateの同一性を判定できなくなるためである。
func parseTimestamp(field, text string) (time.Time, error) {
	if text == "" {
		return time.Time{}, fmt.Errorf("%sが空", field)
	}
	if !strings.HasSuffix(text, "Z") {
		return time.Time{}, fmt.Errorf("%sはoffset `Z` のUTCでなければならない（%q）", field, text)
	}
	value, err := time.Parse(time.RFC3339, text)
	if err != nil {
		return time.Time{}, fmt.Errorf("%sがRFC 3339でない（%q）", field, text)
	}
	// Goのzero timeは`0001-01-01T00:00:00Z`へserializeされ、RFC 3339としては
	// 妥当に見える。未設定のtime.Timeを書き出すとこの値になるため、読書きの
	// 両方で拒否する。そうしないと「時刻を入れ忘れた」stateが西暦1年の
	// timestampとして黙って保存される。
	if value.IsZero() {
		return time.Time{}, fmt.Errorf("%sが未設定（%q）", field, text)
	}
	return value.UTC(), nil
}

// formatTimestamp はtimestampを§7の表現へ変換する。
func formatTimestamp(value time.Time) string {
	return value.UTC().Format(time.RFC3339)
}

// parseInternalDigest はgdtvm自身が計算した64 lowercase hexを読む（§7）。
func parseInternalDigest(field, text string) (domain.Digest, error) {
	digest, err := domain.ParseInternalDigest(text)
	if err != nil {
		return domain.Digest{}, fmt.Errorf("%s: %w", field, err)
	}
	return digest, nil
}

// zeroDigestHex は「不存在」を表す64 zeroである。
//
// §9のintegration identityと§10のbackupが、対象が存在しない場合に
// `digestは64 zero`と定める。空文字列にしないのは、fieldの型を
// 全状態で同じに保つためである。
const zeroDigestHex = "0000000000000000000000000000000000000000000000000000000000000000"

// requireRevision は非負かつJSON安全範囲のrevision/countを確かめる（§7）。
func requireRevision(field string, value *int64) (int64, error) {
	if value == nil {
		return 0, fmt.Errorf("%sが無い", field)
	}
	return requireNonNegative(field, *value)
}

// requireNonNegative は非負かつJSON安全範囲のintegerを確かめる（§7）。
func requireNonNegative(field string, value int64) (int64, error) {
	if value < 0 {
		return 0, fmt.Errorf("%sは非負integerでなければならない（%d）", field, value)
	}
	if value > JSONMaxSafeInteger {
		return 0, fmt.Errorf("%sが2^53-1を超える（%d）", field, value)
	}
	return value, nil
}

// requireRelativePath はPOSIX relative pathであることを確かめる（§7）。
func requireRelativePath(field, text string) (string, error) {
	if text == "" {
		return "", fmt.Errorf("%sが空", field)
	}
	if !relativePathRe.MatchString(text) {
		return "", fmt.Errorf("%sはPOSIX relative pathでなければならない（%q）", field, text)
	}
	for _, component := range strings.Split(text, "/") {
		if component == "." || component == ".." {
			return "", fmt.Errorf("%sに相対参照 %q が含まれる", field, component)
		}
	}
	return text, nil
}

// requirePresent は必須stringが与えられていることを確かめる。
//
// 全fieldをpointerで受けるのは、TOML/JSONの「keyが無い」と「空文字列」を
// 区別するためである。§8以降が「全件必須」と定めるfileでは、keyの欠落を
// 空文字列として黙って通してはならない。
func requirePresent(field string, value *string) (string, error) {
	if value == nil {
		return "", fmt.Errorf("%sが無い", field)
	}
	return *value, nil
}

// requireBool は必須boolが与えられていることを確かめる。
func requireBool(field string, value *bool) (bool, error) {
	if value == nil {
		return false, fmt.Errorf("%sが無い", field)
	}
	return *value, nil
}

// requireEmpty は「非該当なら空」と定められたfieldが空であることを確かめる。
func requireEmpty(field, text string) error {
	if text != "" {
		return fmt.Errorf("%sは空でなければならない（%q）", field, text)
	}
	return nil
}

// stateError はstate/receipt/catalogの破損を表すtyped errorを作る。
//
// docs/03-cli.md §7の終了code表に従い、永続stateの破損は`E_STATE_CORRUPT`と
// する。実pathをparametersへ入れず、roleだけを伝える
// （docs/10-security.md §9.2）。
func stateError(messageID string, role domain.PathRole, cause error) *domain.Error {
	return typedError(domain.CodeStateCorrupt, messageID, role, cause)
}

// typedError はrole付きtyped errorを作る共通経路である。
func typedError(code domain.ErrorCode, messageID string, role domain.PathRole, cause error) *domain.Error {
	id, err := domain.ParseMessageID(messageID)
	if err != nil {
		return domain.Internal(err)
	}
	return &domain.Error{
		Code:      code,
		MessageID: id,
		PathRole:  role,
		Cause:     cause,
	}
}
