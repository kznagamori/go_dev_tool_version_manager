package install

import (
	"errors"
	"fmt"
	"strings"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/definition"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/security"
)

// RenderRoots はtemplate rootの実値である（docs/06-tool-definition.md §12）。
//
// §12の許可rootは`{{version}}`, `{{platform.id}}`, `{{payload}}`, `{{probe_temp}}`,
// `{{storage.<id>}}`、およびversion sourceで宣言したmetadata/assetである。
// metadata/assetはartifact URL/file templateだけで使い、catalog組立て時に解決済み
// なので、install時のpath renderが扱うのは残りの4種類とstorageである。
//
// **未設定のrootは失敗させる。** 空文字列で代替すると、`{{probe_temp}}/x`が
// filesystem rootからの絶対pathになる。
type RenderRoots struct {
	// Version は解決した完全versionである。
	Version string
	// PlatformID はplatform IDである。
	PlatformID string
	// Payload はpayload rootである（role=payload）。
	Payload domain.PathValue
	// ProbeTemp はprobe専用temp directoryである（role=staging）。
	//
	// validation probe内だけで使える（§12）。probe以外の文脈で参照されたら
	// 失敗させる——probe終了後に削除される一時directoryを恒久的な参照先に
	// してしまうためである。zero値なら「この文脈では使えない」を意味する。
	ProbeTemp domain.PathValue
	// Storage は`{{storage.<id>}}`のID別rootである。
	Storage map[string]domain.PathValue
	// Host はpath区切りを決めるplatformである。
	Host domain.Platform
}

// RenderPath はpath contextのtemplateを実pathへ評価する（§12）。
//
// **path templateはrootそのものか、rootの子pathでなければならない。**
// docs/06-tool-definition.md §10.1・§11がliteral prefix/suffixの連結を禁じており、
// 連結を許すとrender後にrootの外を指す値を作れる。したがって形は
// `{{root}}`または`{{root}}/a/b`のどちらかに限る。
//
// 子pathはPOSIX slashで書き、OS separatorへの変換は[security.Join]が行う（§12
// 「logical pathはPOSIX slashで記述し、OS adapterがseparatorへ変換する」）。
//
// **render結果のcanonical containment再検査はここでは行わない。** §12はrender後の
// 再検査を求めるが、Plan作成時点ではpayloadがまだ存在せずrealpathへ解決できない。
// 構成上、結果は必ず宣言rootの子pathになる（[security.Join]がcomponentごとに
// `..`と区切り混在を拒否する）。実書込み時のcanonical検査は
// docs/11-quality-and-ci.md §7.2の記録wrapperが行う。
func RenderPath(text string, roots RenderRoots) (domain.PathValue, error) {
	if text == "" {
		return domain.PathValue{}, errors.New("install: path templateが空")
	}
	name, rest, err := splitTemplateRoot(text)
	if err != nil {
		return domain.PathValue{}, err
	}
	root, err := resolvePathRoot(name, roots)
	if err != nil {
		return domain.PathValue{}, err
	}
	if rest == "" {
		return root, nil
	}

	components := strings.Split(rest, "/")
	value, joinErr := security.Join(security.JoinRequest{
		Root: root, Components: components, Host: roots.Host,
	})
	if joinErr != nil {
		return domain.PathValue{}, fmt.Errorf("install: path template %q: %w", text, joinErr)
	}
	if len(value.Path()) > definition.RenderedMaxBytes {
		return domain.PathValue{}, fmt.Errorf(
			"install: render結果が%d byteを超える（%d byte）",
			definition.RenderedMaxBytes, len(value.Path()))
	}
	return value, nil
}

// RenderText はpath以外のtemplateを文字列へ評価する（§12）。
//
// `expected_version`のように、変数がliteralの任意の位置へ現れてよい文脈で使う。
// pathを作らないため区切りの変換もcontainmentも行わない。
//
// 未知変数を拒否する。§12が「未知変数、再帰展開、function、condition、shell
// evaluationを禁止する」と定めており、素通りさせるとliteralとして残った
// `{{...}}`がそのまま比較対象になる。
func RenderText(text string, roots RenderRoots) (string, error) {
	if text == "" {
		return "", nil
	}
	var failure error
	rendered := definition.ReplaceTemplateVars(text, func(match string) string {
		value, err := resolveScalar(match, roots)
		if err != nil && failure == nil {
			failure = err
		}
		return value
	})
	if failure != nil {
		return "", failure
	}
	if len(rendered) > definition.RenderedMaxBytes {
		return "", fmt.Errorf(
			"install: render結果が%d byteを超える（%d byte）",
			definition.RenderedMaxBytes, len(rendered))
	}
	return rendered, nil
}

// splitTemplateRoot は`{{root}}`と残りの相対pathへ分ける。
//
// 先頭が`{{`でない、または`}}`の後に`/`以外が続く形を拒否する。
// literal prefix/suffixの連結を許さないためである（§10.1・§11）。
func splitTemplateRoot(text string) (string, string, error) {
	if !strings.HasPrefix(text, "{{") {
		return "", "", fmt.Errorf(
			"install: path template %q がrootで始まっていない", text)
	}
	end := strings.Index(text, "}}")
	if end < 0 {
		return "", "", fmt.Errorf("install: path template %q が閉じていない", text)
	}
	name := text[:end+2]
	rest := text[end+2:]
	switch {
	case rest == "":
		return name, "", nil
	case !strings.HasPrefix(rest, "/"):
		// `{{payload}}bin`のような連結を拒否する。
		return "", "", fmt.Errorf(
			"install: path template %q はrootの直後が`/`でない", text)
	}
	rest = strings.TrimPrefix(rest, "/")
	if rest == "" || strings.HasSuffix(rest, "/") {
		return "", "", fmt.Errorf("install: path template %q に空componentがある", text)
	}
	if strings.Contains(rest, "{{") {
		// 子path側へ変数を許すと、rootの外を指す値を後から差し込める。
		return "", "", fmt.Errorf(
			"install: path template %q の子pathへ変数を書けない", text)
	}
	return name, rest, nil
}

// resolvePathRoot はroot名を実pathへ解決する。
func resolvePathRoot(name string, roots RenderRoots) (domain.PathValue, error) {
	switch name {
	case definition.PayloadTemplate:
		if roots.Payload.IsZero() {
			return domain.PathValue{}, errors.New("install: payload rootが未設定")
		}
		return roots.Payload, nil
	case definition.ProbeTempTemplate:
		if roots.ProbeTemp.IsZero() {
			// probe以外の文脈では`{{probe_temp}}`を解決できない（§12）。
			return domain.PathValue{}, errors.New(
				"install: probe temp rootはvalidation probe内だけで使える")
		}
		return roots.ProbeTemp, nil
	}
	if id, ok := definition.StorageTemplateID(name); ok {
		root, declared := roots.Storage[id]
		if !declared {
			return domain.PathValue{}, fmt.Errorf(
				"install: storage ID %q が宣言されていない", id)
		}
		if root.IsZero() {
			// 宣言だけあって値が入っていない場合。root単体のrenderは
			// [security.Join]を通らないため、ここで落とさないとzero値の
			// PathValueがそのまま呼出し側へ渡る。
			return domain.PathValue{}, fmt.Errorf(
				"install: storage ID %q のrootが未設定", id)
		}
		return root, nil
	}
	return domain.PathValue{}, fmt.Errorf("install: 未知のpath root %q", name)
}

// resolveScalar はpath以外のtemplate変数を文字列へ解決する。
func resolveScalar(match string, roots RenderRoots) (string, error) {
	switch match {
	case definition.VersionTemplate:
		if roots.Version == "" {
			return "", errors.New("install: versionが未設定")
		}
		return roots.Version, nil
	case definition.PlatformIDTemplate:
		if roots.PlatformID == "" {
			return "", errors.New("install: platform IDが未設定")
		}
		return roots.PlatformID, nil
	}
	return "", fmt.Errorf("install: 未知のtemplate変数 %q", match)
}
