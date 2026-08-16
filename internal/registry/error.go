package registry

import (
	"errors"
	"fmt"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
)

// messageRegistryInvalid はregistry不正のmessage IDである。
const messageRegistryInvalid = "registry.invalid"

// invalidError はregistryが契約に合わないことを表す。
//
// docs/07-registry-and-tools.md §3が「client versionがmin未満/max超過、schema
// 不一致、entry欠落/extra、digest不一致なら`E_REGISTRY_INVALID`」と定める。
// registryはclientへ同梱される信頼の根であり、読めない部分を推測で補わない。
func invalidError(cause error) *domain.Error {
	messageID, _ := domain.ParseMessageID(messageRegistryInvalid)
	return &domain.Error{
		Code:      domain.CodeRegistryInvalid,
		MessageID: messageID,
		Parameters: domain.Parameters{
			"reason": domain.StringScalar(cause.Error()),
		},
		PathRole: domain.RoleRegistry,
		Cause:    fmt.Errorf("registry: %w", cause),
	}
}

// describeDecodeError はstrict decodeの失敗を読める形にする。
//
// unknown keyは「どのkeyが余分か」を出す。registryを直す人が最初に見る情報である。
func describeDecodeError(err error) error {
	var strict *toml.StrictMissingError
	if errors.As(err, &strict) && len(strict.Errors) > 0 {
		key := strict.Errors[0].Key()
		return fmt.Errorf("未知のkey `%s`", strings.Join(key, "."))
	}
	var decode *toml.DecodeError
	if errors.As(err, &decode) {
		row, column := decode.Position()
		return fmt.Errorf("%d行%d列: %s", row, column, decode.Error())
	}
	return err
}
