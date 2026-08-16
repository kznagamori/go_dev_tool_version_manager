package definition

import (
	"fmt"
	"regexp"
	"strings"
)

// §12の許可root。`{{storage.<id>}}`と`{{metadata.<key>}}`と`{{asset.<field>}}`だけ
// 引数を取る。
const (
	VersionTemplate    = "{{version}}"
	PlatformIDTemplate = "{{platform.id}}"
	PayloadTemplate    = "{{payload}}"
	ProbeTempTemplate  = "{{probe_temp}}"
)

// RenderedMaxBytes はrender結果の上限である（§12、docs/04-storage-and-data.md §21）。
const RenderedMaxBytes = 32 << 10

// templateRe はtemplate変数の出現を切り出す。
//
// `{{`から次の`}}`までを1件とする。未知変数を検出するために、中身を限定せず
// すべての出現を拾ってから許可集合と突き合わせる。限定した正規表現で拾うと、
// 一致しなかった`{{...}}`がliteralとして素通りする。
var templateRe = regexp.MustCompile(`\{\{[^{}]*\}\}`)

// storageTemplateRe は`{{storage.<id>}}`のIDを取り出す。
var storageTemplateRe = regexp.MustCompile(`^\{\{storage\.([a-z][a-z0-9]*(?:-[a-z0-9]+)*)\}\}$`)

// metadataTemplateRe は`{{metadata.<key>}}`のkeyを取り出す。
var metadataTemplateRe = regexp.MustCompile(`^\{\{metadata\.([a-z][a-z0-9_]*)\}\}$`)

// assetTemplateRe は`{{asset.<field>}}`のfieldを取り出す。
var assetTemplateRe = regexp.MustCompile(`^\{\{asset\.([a-z][a-z0-9_]*)\}\}$`)

// templateScope は文脈ごとの許可rootである（§12）。
//
// 文脈で許可rootが変わるため、bool 1つずつではなくscopeとして持つ。
// `{{probe_temp}}`はvalidation probe内だけであり（§12）、commandが使えると
// probe終了後に削除される一時directoryを恒久的な参照先にしてしまう。
type templateScope struct {
	// name は診断へ出す文脈名である。
	name string
	// version は`{{version}}`を許すか。
	version bool
	// platformID は`{{platform.id}}`を許すか。
	platformID bool
	// payload は`{{payload}}`を許すか。
	payload bool
	// probeTemp は`{{probe_temp}}`を許すか。
	probeTemp bool
	// storage は`{{storage.<id>}}`を許すか。
	storage bool
	// metadata は`{{metadata.<key>}}`を許すか。
	metadata bool
	// asset は`{{asset.<field>}}`を許すか。
	asset bool
}

// §12の文脈別scope。
var (
	// artifactScope は§7.1のURL/file templateである。
	//
	// 「URL/file templateは`{{version}}`と宣言済み`{{metadata.<key>}}`,
	// `{{asset.<field>}}`だけ」。
	artifactScope = templateScope{name: "artifact template", version: true, metadata: true, asset: true}
	// commandScope は§10.1のcommand target/argsと§10.2のenvironment値である。
	commandScope = templateScope{name: "command", payload: true, storage: true}
	// probeArgScope は§11のprobe argsとrequired_pathsである。
	probeArgScope = templateScope{name: "probe", payload: true, probeTemp: true, storage: true}
	// expectedVersionScope は§11の`expected_version`である。
	expectedVersionScope = templateScope{name: "expected_version", version: true}
)

// templateContext はtemplate検査に要る宣言済み集合である。
//
// storage ID、metadata key、asset fieldは同じplatformの宣言に依存する。
// 宣言していないrootを参照するtemplateは、render時に値が無くinstallできない。
type templateContext struct {
	storageIDs   map[string]struct{}
	metadataKeys map[string]struct{}
	assetFields  map[AssetField]struct{}
}

// checkRoot は1件のtemplate変数が文脈で許されるかを確かめる。
func (c templateContext) checkRoot(token string, scope templateScope) error {
	switch token {
	case VersionTemplate:
		if scope.version {
			return nil
		}
	case PlatformIDTemplate:
		if scope.platformID {
			return nil
		}
	case PayloadTemplate:
		if scope.payload {
			return nil
		}
	case ProbeTempTemplate:
		if scope.probeTemp {
			return nil
		}
	}
	if match := storageTemplateRe.FindStringSubmatch(token); match != nil {
		if !scope.storage {
			return fmt.Errorf("%sでは %s を使えない", scope.name, token)
		}
		if _, declared := c.storageIDs[match[1]]; !declared {
			return fmt.Errorf("storage ID %q がこのplatformに宣言されていない", match[1])
		}
		return nil
	}
	if match := metadataTemplateRe.FindStringSubmatch(token); match != nil {
		if !scope.metadata {
			return fmt.Errorf("%sでは %s を使えない", scope.name, token)
		}
		if _, declared := c.metadataKeys[match[1]]; !declared {
			return fmt.Errorf("metadata key %q が`metadata_fields`に宣言されていない", match[1])
		}
		return nil
	}
	if match := assetTemplateRe.FindStringSubmatch(token); match != nil {
		if !scope.asset {
			return fmt.Errorf("%sでは %s を使えない", scope.name, token)
		}
		field, err := parseAssetField(match[1])
		if err != nil {
			return err
		}
		if _, declared := c.assetFields[field]; !declared {
			return fmt.Errorf("asset field %q が`asset_fields`に宣言されていない", match[1])
		}
		return nil
	}
	return fmt.Errorf("%sで使えないtemplate変数 %s", scope.name, token)
}

// checkSubstitution は§7.1のURL/file templateを検査する。
//
// 変数はliteralの中の任意の位置に現れてよい。Node.jsの
// `node-v{{version}}-win-x64.zip`のように、変数の前後にliteralが付く。
func (c templateContext) checkSubstitution(field, value string, scope templateScope) error {
	if len(value) > RenderedMaxBytes {
		return fmt.Errorf("%sが%d byteを超える（%d byte）", field, RenderedMaxBytes, len(value))
	}
	for _, token := range templateRe.FindAllString(value, -1) {
		if err := c.checkRoot(token, scope); err != nil {
			return fmt.Errorf("%s: %w", field, err)
		}
	}
	// 対応の取れない`{`と`}`は未知構文である。`{{`だけを許し、conditionや
	// functionの記法を素通りさせない（§12）。
	if rest := templateRe.ReplaceAllString(value, ""); strings.ContainsAny(rest, "{}") {
		return fmt.Errorf("%sに対応の取れない波括弧がある（%q）", field, value)
	}
	return nil
}

// checkPathTemplate は§10・§11のpath templateを検査する。
//
// entry全体が許可rootそのものか、rootの子pathでなければならない。
// **path templateへliteral prefix/suffixを連結しない**（§10.1・§11）。
// 連結を許すと`{{payload}}../etc`のように、render後にrootの外を指す値を
// 作れてしまう。
func (c templateContext) checkPathTemplate(field, value string, scope templateScope) error {
	if len(value) > RenderedMaxBytes {
		return fmt.Errorf("%sが%d byteを超える（%d byte）", field, RenderedMaxBytes, len(value))
	}
	if !strings.HasPrefix(value, "{{") {
		return fmt.Errorf("%sはtemplate rootで始まらなければならない（%q）", field, value)
	}
	end := strings.Index(value, "}}")
	if end < 0 {
		return fmt.Errorf("%sのtemplate変数が閉じていない（%q）", field, value)
	}
	root, child := value[:end+2], value[end+2:]
	if err := c.checkRoot(root, scope); err != nil {
		return fmt.Errorf("%s: %w", field, err)
	}
	return checkChildPath(field, child)
}

// checkChildPath はtemplate rootに続く子pathを検査する。
//
// 空か`/`始まりのPOSIX relative pathだけを許す。§12が「logical pathはPOSIX
// slashで記述し、OS adapterがseparatorへ変換する」と定める。
func checkChildPath(field, child string) error {
	if child == "" {
		return nil
	}
	if !strings.HasPrefix(child, "/") {
		// `{{payload}}bin`のような連結を拒否する。render後の境界が曖昧になり、
		// rootの外を指す値と区別できない。
		return fmt.Errorf("%sのtemplate rootの直後は`/`でなければならない（%q）", field, child)
	}
	if strings.Contains(child, "{{") {
		// 子pathへの再帰展開を禁止する（§12）。
		return fmt.Errorf("%sの子pathにtemplate変数がある（%q）", field, child)
	}
	if strings.ContainsAny(child, `\`) {
		return fmt.Errorf("%sの子pathにbackslashがある（%q）", field, child)
	}
	for _, component := range strings.Split(strings.TrimPrefix(child, "/"), "/") {
		switch {
		case component == "":
			return fmt.Errorf("%sの子pathに空componentがある（%q）", field, child)
		case component == "." || component == "..":
			return fmt.Errorf("%sの子pathに相対参照がある（%q）", field, child)
		case len(component) > PathComponentMaxBytes:
			return fmt.Errorf("%sの子path componentが%d byteを超える", field, PathComponentMaxBytes)
		}
	}
	return nil
}

// isPathTemplate は値がtemplate rootで始まるかを返す。
//
// §10.1と§11のargsは「literalまたはentry全体がpath template」である。
// どちらとして検査するかをこの判定で分ける。
func isPathTemplate(value string) bool { return strings.HasPrefix(value, "{{") }

// checkLiteralArg はtemplateでないargvのliteralを検査する。
//
// §10.1が「shell文字列、環境展開、command substitutionは禁止」と定める。
// literalの中にtemplate変数が現れる形も拒否する。entry全体がtemplateか、
// まったくtemplateを含まないliteralかの二択にするためである。
func checkLiteralArg(field, value string) error {
	switch {
	case value == "":
		return fmt.Errorf("%sが空", field)
	case len(value) > RenderedMaxBytes:
		return fmt.Errorf("%sが%d byteを超える（%d byte）", field, RenderedMaxBytes, len(value))
	case strings.ContainsAny(value, "{}"):
		return fmt.Errorf("%sのliteralに波括弧がある（%q）", field, value)
	case strings.ContainsRune(value, 0):
		return fmt.Errorf("%sにNULがある", field)
	}
	return nil
}
