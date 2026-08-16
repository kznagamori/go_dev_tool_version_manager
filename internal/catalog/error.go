package catalog

import (
	"fmt"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
)

// source errorのmessage ID。
const (
	messageSourceInvalid = "catalog.source_invalid"
	messageSourceFetch   = "catalog.source_fetch_failed"
)

// sourceError は上流文書がdefinitionの参照に合わないことを表す。
//
// docs/06-tool-definition.md §6.1が「definitionが参照するfieldの欠落/型違いは
// そのitemを黙ってskipせずsource errorにする」、§6.3が「matchしないitemはsource
// layout違反としてrefreshを失敗させる」と定める。**部分的な結果を返さない。**
//
// codeは`E_DEFINITION_INVALID`とする。仕様はsource error専用のcodeを定めていない
// が、docs/03-cli.md §7の完全性group（exit 6）にsource側の不一致を置ける唯一の
// codeであり、docs/08-install-runtime.md §111が「definition参照不正」を同codeへ
// 割り当てている。上流文書に対してdefinitionのpointer/regexが解決できない状態は
// 同じ性質で、直す先もregistryのdefinitionである。
func sourceError(field string, cause error) *domain.Error {
	return &domain.Error{
		Code:      domain.CodeDefinitionInvalid,
		MessageID: messageID(messageSourceInvalid),
		Parameters: domain.Parameters{
			"field":  domain.StringScalar(field),
			"reason": domain.StringScalar(cause.Error()),
		},
		PathRole: domain.RoleToolDefinition,
		Cause:    fmt.Errorf("version source %s: %w", field, cause),
	}
}

// fetchError は上流文書を取得できなかったことを表す。
//
// codeは`E_NETWORK`とする（docs/03-cli.md §7のnetwork group）。retryとbackoffは
// HTTPClient adapterの責務であり（docs/13-progress.md P5-01）、本packageは1回の
// 要求結果だけを扱う。
func fetchError(url string, cause error) *domain.Error {
	return &domain.Error{
		Code:      domain.CodeNetwork,
		MessageID: messageID(messageSourceFetch),
		Parameters: domain.Parameters{
			// URLはdefinitionが宣言した公開配布元であり、credentialを含まない
			// （docs/06-tool-definition.md §4・§6.1がHTTPSかつcredentialなしを要求する）。
			"url":    domain.StringScalar(url),
			"reason": domain.StringScalar(cause.Error()),
		},
		Retryable: true,
		Cause:     fmt.Errorf("version source取得 %s: %w", url, cause),
	}
}

// messageID はmessage ID文字列をMessageIDへ変換する。
//
// 引数は本package内のconstantだけであり、parseは失敗しない。失敗した場合も
// zero値のまま返し、error処理の途中でpanicさせない（CLAUDE.md §9）。
func messageID(id string) domain.MessageID {
	value, _ := domain.ParseMessageID(id)
	return value
}
