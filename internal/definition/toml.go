package definition

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/pelletier/go-toml/v2"
)

// utf8BOM はUTF-8 BOMのbyte列である。
//
// docs/06-tool-definition.md §1が「UTF-8 BOMなしTOML 1.0」と定める。go-tomlは
// BOMを許すため、入力側で拒否する。
var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// decodeError は位置を保持したdecode失敗である。
//
// §13が「errorはdefinition relative path、line/column、field path、stable reason
// codeを返す」と定めるため、go-tomlのerrorを行・列へ分解して持つ。
type decodeError struct {
	line   int
	column int
	field  string
	detail string
}

func (e *decodeError) Error() string {
	if e.line > 0 {
		return fmt.Sprintf("%d行%d列: %s", e.line, e.column, e.detail)
	}
	return e.detail
}

// decodeFile はdefinition TOMLをstrictにdecodeする。
//
// DisallowUnknownFieldsがunknown key/tableを、go-toml自身が重複key/table、
// 型違い、trailing dataを拒否する（§1）。enum、上限、条件付きkeyは呼出し側の
// semantic検証が扱う。
//
// **`internal/store`のcodecを流用しない。** 同packageはstate/receipt/catalogの
// 永続表現を担当し、definitionはregistryが配る入力である。片方の都合で共通関数を
// 変えると、もう片方の受理範囲が黙って動く。[internal/config]も同じ理由で自前の
// strict decodeを持つ。
func decodeFile(data []byte, target any) *decodeError {
	if err := checkInput(data); err != nil {
		return &decodeError{detail: err.Error()}
	}
	decoder := toml.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return describeDecodeError(err)
	}
	return nil
}

// checkInput はdecode前のbyte列を検査する。
func checkInput(data []byte) error {
	switch {
	case len(data) == 0:
		return errors.New("definition fileが空")
	case len(data) > FileMaxBytes:
		return fmt.Errorf(
			"definition fileが%d byteを超える（%d byte）", FileMaxBytes, len(data))
	case bytes.HasPrefix(data, utf8BOM):
		return errors.New("definition fileがUTF-8 BOMで始まる")
	case !utf8.Valid(data):
		return errors.New("definition fileが正しいUTF-8でない")
	}
	return nil
}

// describeDecodeError はgo-tomlのerrorを位置とkeyへ分解する。
func describeDecodeError(err error) *decodeError {
	var strict *toml.StrictMissingError
	if errors.As(err, &strict) && len(strict.Errors) > 0 {
		first := strict.Errors[0]
		line, column := first.Position()
		// Keyはtable階層を分解したsliceで返る。§13のfield pathへ載せるため
		// dotで連結して1つのpathへ戻す。
		key := strings.Join(first.Key(), ".")
		return &decodeError{
			line: line, column: column, field: key,
			detail: fmt.Sprintf("未知のkey %q", key),
		}
	}
	var decode *toml.DecodeError
	if errors.As(err, &decode) {
		line, column := decode.Position()
		return &decodeError{line: line, column: column, detail: decode.Error()}
	}
	return &decodeError{detail: err.Error()}
}
