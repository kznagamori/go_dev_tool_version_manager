package definition

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
)

// Diagnostic は1件のdefinition検証errorである（docs/06-tool-definition.md §13）。
//
// 同§が「errorはdefinition relative path、line/column、field path、stable reason
// codeを返す」と定める。表示文は持たない。文はmessage catalogがReasonから作る
// （[02-architecture.md](../../docs/02-architecture.md) §14）。
type Diagnostic struct {
	// Path はregistry rootからのrelative pathである（`tools/node.toml`）。
	//
	// 利用者のhome pathを含まないため、そのまま診断へ出せる
	// （docs/10-security.md §9.2）。
	Path string
	// Line はTOMLの行番号である。位置が分からない場合は0にする。
	Line int
	// Column はTOMLの列番号である。位置が分からない場合は0にする。
	Column int
	// Field はfield pathである（`platforms[0].provider.license`）。
	Field string
	// Reason はstable reason codeである。
	//
	// message IDをそのまま使う。別のcode体系を作ると、同じ失敗がcode表と
	// message catalogの2か所で別々に管理されることになる。
	Reason domain.MessageID
	// Detail は開発者向けの内部説明である。
	//
	// 公開message textではない。log/決定記録で原因を追うために持つ。
	Detail string
}

// String は診断1行の内部表現を返す。log/testの比較に使う。
func (d Diagnostic) String() string {
	var b strings.Builder
	b.WriteString(d.Path)
	if d.Line > 0 {
		b.WriteString(":")
		b.WriteString(strconv.Itoa(d.Line))
		if d.Column > 0 {
			b.WriteString(":")
			b.WriteString(strconv.Itoa(d.Column))
		}
	}
	if d.Field != "" {
		b.WriteString(" ")
		b.WriteString(d.Field)
	}
	b.WriteString(" ")
	b.WriteString(d.Reason.String())
	if d.Detail != "" {
		b.WriteString(": ")
		b.WriteString(d.Detail)
	}
	return b.String()
}

// Diagnostics はdefinition検証errorの集約である（§13）。
//
// 同§が「複数errorを集約しても上限100件で停止する」と定める。1件目で止めると
// registry更新のたびに1件ずつしか直せず、上限なしだと壊れたfileが診断出力を
// 埋める。
type Diagnostics struct {
	path      string
	items     []Diagnostic
	truncated bool
}

// NewDiagnostics は対象fileのrelative pathを固定した集約を作る。
func NewDiagnostics(path string) *Diagnostics {
	return &Diagnostics{path: path}
}

// Add は診断を1件足す。上限に達していればfalseを返す。
//
// 戻り値がfalseの間も呼出し側は検査を続けてよい。件数を数え続けても意味が
// 無いため、上限到達後は保持しない。
func (d *Diagnostics) Add(field string, reason domain.MessageID, detail string) bool {
	return d.addAt(field, 0, 0, reason, detail)
}

// AddAt は位置付きの診断を1件足す。
func (d *Diagnostics) AddAt(field string, line, column int, reason domain.MessageID, detail string) bool {
	return d.addAt(field, line, column, reason, detail)
}

func (d *Diagnostics) addAt(field string, line, column int, reason domain.MessageID, detail string) bool {
	if len(d.items) >= DiagnosticMax {
		d.truncated = true
		return false
	}
	d.items = append(d.items, Diagnostic{
		Path: d.path, Line: line, Column: column,
		Field: field, Reason: reason, Detail: detail,
	})
	return true
}

// Len は保持している診断の件数を返す。
func (d *Diagnostics) Len() int { return len(d.items) }

// Truncated は上限で打ち切ったかを返す。
func (d *Diagnostics) Truncated() bool { return d.truncated }

// Items は保持している診断を検出順で返す。
//
// 呼出し側の書換えが集約へ波及しないようcopyを返す（CLAUDE.md §6）。
func (d *Diagnostics) Items() []Diagnostic {
	return append([]Diagnostic(nil), d.items...)
}

// Err は集約結果をtyped errorへ変換する。診断が無ければnilを返す。
//
// codeは`E_DEFINITION_INVALID`固定である（docs/03-cli.md §7）。parametersへは
// 件数と先頭1件のfield pathだけを載せる。全件を載せるとparameterがscalar mapで
// なくなり、§7のmessage契約から外れる。
func (d *Diagnostics) Err() *domain.Error {
	if len(d.items) == 0 {
		return nil
	}
	first := d.items[0]
	messageID, err := domain.ParseMessageID("definition.invalid")
	if err != nil {
		return domain.Internal(err)
	}
	parameters := domain.Parameters{
		"path":  domain.StringScalar(d.path),
		"count": domain.IntScalar(int64(len(d.items))),
		"field": domain.StringScalar(first.Field),
		"first": domain.StringScalar(first.Reason.String()),
	}
	if d.truncated {
		parameters["truncated"] = domain.BoolScalar(true)
	}
	return &domain.Error{
		Code:       domain.CodeDefinitionInvalid,
		MessageID:  messageID,
		Parameters: parameters,
		PathRole:   domain.RoleToolDefinition,
		Cause:      fmt.Errorf("definition検証で%d件のerror: %s", len(d.items), first),
	}
}

// reason はmessage ID文字列をMessageIDへ変換する。
//
// 引数は本package内のconstantだけであり、parseは失敗しない。失敗した場合は
// zero値のまま返し、[Diagnostics.Err]がcatalog lookupの失敗として扱えるように
// する。error処理の途中でpanicさせない（CLAUDE.md §9）。
func reason(id string) domain.MessageID {
	messageID, _ := domain.ParseMessageID(id)
	return messageID
}
