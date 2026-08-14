package store

import (
	"fmt"
	"regexp"
	"strings"
)

// PayloadTemplate はpayload rootを指すtemplate rootである。
//
// docs/06-tool-definition.md §12が許可rootを定め、
// docs/04-storage-and-data.md §14がreceiptで使える部分集合を定める。
const PayloadTemplate = "{{payload}}"

// ProbeTempTemplate はprobeごとのowner-only一時directoryを指す。
//
// docs/06-tool-definition.md §12が「`{{probe_temp}}`はvalidation probe内だけ」と
// 定める。commandやstorage pathで使うと、削除される一時directoryを恒久的な
// 参照先にしてしまう。
const ProbeTempTemplate = "{{probe_temp}}"

// storageTemplateRe は`{{storage.<id>}}`とその子pathを取り出す。
//
// idのgrammarはdocs/06-tool-definition.md §3のkebab-caseに合わせる。
var storageTemplateRe = regexp.MustCompile(`^\{\{storage\.([a-z0-9]+(?:-[a-z0-9]+)*)\}\}`)

// templateRootRe は任意の`{{...}}`を検出する。
//
// 許可rootの判定に先立って「templateを含むか」を見るために使う。未知変数を
// literalとして通さないためであり、docs/06-tool-definition.md §12の
// 「未知変数、再帰展開、function、condition、shell evaluationを禁止する」に対応する。
var templateRootRe = regexp.MustCompile(`\{\{[^}]*\}\}`)

// templateScope はtemplateを書ける場所ごとの許可rootである。
type templateScope struct {
	// name は診断に出す場所の名前である。
	name string
	// allowProbeTemp は`{{probe_temp}}`を許すかである。probeだけがtrueになる。
	allowProbeTemp bool
}

// commandScope はcommand target、fixed args、storage path、environment値の範囲である。
//
// docs/04-storage-and-data.md §14が「target/fixed args/path/setで許すtemplateは
// `{{payload}}`とreceipt内に存在する`{{storage.<id>}}`およびその子pathだけで、
// metadata/version/staging/outputや再帰展開は禁止する」と定める。
var commandScope = templateScope{name: "command", allowProbeTemp: false}

// probeScope はprobe args、required_paths、expected_rootの範囲である。
//
// docs/06-tool-definition.md §11が「argsはliteralに加えて、entry全体として
// `{{payload}}`, `{{probe_temp}}`, `{{storage.<id>}}`とその子pathを使える」と
// 定める。commandと違い`{{probe_temp}}`を許す。
var probeScope = templateScope{name: "probe", allowProbeTemp: true}

// validateTemplate はtemplateが許可rootで始まり、未知変数を含まないことを確かめる。
//
// storageIDsはreceipt内に存在するstorage IDの集合である。receiptに無いIDを
// 参照するtemplateは、renderできないかrender先が管理外になる。
//
// literalだけの値も許す。docs/06-tool-definition.md §11の「argsはliteralに加えて」
// のとおり、templateでない値は通常の文字列として扱う。ただしliteral中に
// `{{...}}`があれば未知変数として拒否する。
func validateTemplate(field, value string, scope templateScope, storageIDs map[string]struct{}) error {
	if value == "" {
		return fmt.Errorf("%sが空", field)
	}

	rest, matched := matchTemplateRoot(value, scope, storageIDs)
	if !matched {
		// rootに一致しない場合はliteralである。`{{`が残っていれば未知変数。
		if found := templateRootRe.FindString(value); found != "" {
			return fmt.Errorf("%sに許可されないtemplate %q がある（%q）", field, found, value)
		}
		return nil
	}
	// docs/06-tool-definition.md §11は「path templateへのliteral prefix/suffix連結を
	// 拒否」と定める。root直後は`/`区切りの子pathだけを許す。
	if rest == "" {
		return nil
	}
	if !strings.HasPrefix(rest, "/") {
		return fmt.Errorf("%sのtemplate rootへliteralを連結できない（%q）", field, value)
	}
	child := strings.TrimPrefix(rest, "/")
	if found := templateRootRe.FindString(child); found != "" {
		// 再帰展開の禁止（§12）。子pathの中でさらにtemplateを使わせない。
		return fmt.Errorf("%sのtemplate子pathにtemplate %q がある（%q）", field, found, value)
	}
	if _, err := requireRelativePath(field+"の子path", child); err != nil {
		return err
	}
	return nil
}

// matchTemplateRoot は先頭の許可rootを消費し、残りと一致有無を返す。
func matchTemplateRoot(
	value string, scope templateScope, storageIDs map[string]struct{},
) (string, bool) {
	if strings.HasPrefix(value, PayloadTemplate) {
		return strings.TrimPrefix(value, PayloadTemplate), true
	}
	if scope.allowProbeTemp && strings.HasPrefix(value, ProbeTempTemplate) {
		return strings.TrimPrefix(value, ProbeTempTemplate), true
	}
	if match := storageTemplateRe.FindStringSubmatch(value); match != nil {
		if _, known := storageIDs[match[1]]; known {
			return value[len(match[0]):], true
		}
	}
	return "", false
}

// validateStorageTemplateExists はstorage参照が未定義IDでないことを確かめる。
//
// [validateTemplate]は未知のstorage IDをliteralとして扱えず、`{{`を含むため
// 「許可されないtemplate」として拒否する。診断を分かりやすくするため、
// storage参照の形をしている場合は専用のmessageを出す。
func validateStorageTemplateExists(field, value string, storageIDs map[string]struct{}) error {
	match := storageTemplateRe.FindStringSubmatch(value)
	if match == nil {
		return nil
	}
	if _, known := storageIDs[match[1]]; !known {
		return fmt.Errorf("%sがreceiptに無いstorage %q を参照している", field, match[1])
	}
	return nil
}
