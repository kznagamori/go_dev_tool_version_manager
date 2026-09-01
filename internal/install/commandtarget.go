package install

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/definition"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain/port"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/security"
	"github.com/kznagamori/go_dev_tool_version_manager/internal/store"
)

// CommandTargetRequest は`command_targets`の収集入力である。
//
// docs/08-install-runtime.md §7手順4「**required** runtime commandのtargetと
// fixed argsが指すpayload内fileについて、相対path、size、SHA-256を
// `command_targets`へ記録する。payload全fileのmanifestは作らない」。
type CommandTargetRequest struct {
	// Platform はdefinitionの当該platform blockである。
	Platform definition.Platform
	// PayloadDir は検査対象のpayload rootである（staging内）。
	PayloadDir domain.PathValue
	// Roots はcommand targetのrenderに使うrootである。
	//
	// `Payload`は[PayloadDir]と同じ実体を指す。
	Roots RenderRoots
}

// CollectCommandTargets はreceiptへ書く`command_targets`を作る。
//
// docs/04-storage-and-data.md §14「`command_targets`は**required runtime command
// のtargetとfixed argsが参照するpayload内fileだけ**を、payload相対path byte順・
// 一意で持つ」。
//
// **`required=false`のcommandを含めない。** §7手順4が`required`だけを対象とする。
// 任意commandまで記録すると、そのcommandを使わない利用者のpayloadで`doctor`が
// 存在しないfileの破損を報告する。
//
// **payload外を指すentryは記録しない。** command targetは`{{payload}}`配下と
// 決まっている（docs/06-tool-definition.md §10.1）が、fixed argsは
// `{{storage.<id>}}`も取れる。storageは利用者が書き換える領域であり、
// 完全性記録の対象にすると正常な変更を破損として報告する。
func CollectCommandTargets(
	filesystem port.FileSystem, req CommandTargetRequest,
) ([]store.ReceiptCommandTarget, error) {
	if filesystem == nil {
		return nil, errors.New("install: FileSystem portが未設定")
	}
	if req.PayloadDir.IsZero() || req.PayloadDir.Path() == "" {
		return nil, errors.New("install: payload directoryが未設定")
	}

	paths, err := commandTargetPaths(req)
	if err != nil {
		return nil, err
	}
	targets := make([]store.ReceiptCommandTarget, 0, len(paths))
	for _, relative := range paths {
		absolute, joinErr := security.Join(security.JoinRequest{
			Root:       req.PayloadDir,
			Components: strings.Split(relative, "/"),
			Host:       req.Roots.Host,
		})
		if joinErr != nil {
			return nil, fmt.Errorf("install: command target %q: %w", relative, joinErr)
		}
		digest, size, hashErr := hashPayloadFile(filesystem, absolute)
		if hashErr != nil {
			return nil, fmt.Errorf("install: command target %q: %w", relative, hashErr)
		}
		targets = append(targets, store.ReceiptCommandTarget{
			// §14のpathはpayload相対だが、receiptの例は`payload/node.exe`と
			// payload directory名を含む。`payload_path=payload`固定（§14）で
			// あるため、prefixを付けた形が正である。
			Path:   payloadComponent + "/" + relative,
			Size:   size,
			SHA256: digest,
		})
	}
	// §14「payload相対path byte順・一意」。
	sort.Slice(targets, func(i, j int) bool { return targets[i].Path < targets[j].Path })
	return targets, nil
}

// commandTargetPaths は記録対象のpayload相対pathをbyte順・一意で返す。
func commandTargetPaths(req CommandTargetRequest) ([]string, error) {
	seen := make(map[string]struct{})
	var relatives []string
	add := func(value domain.PathValue) error {
		relative, inside, err := payloadRelative(value, req.PayloadDir, req.Roots.Host)
		if err != nil {
			return err
		}
		if !inside {
			// payload外（storage等）は完全性記録の対象にしない。
			return nil
		}
		if _, duplicate := seen[relative]; duplicate {
			return nil
		}
		seen[relative] = struct{}{}
		relatives = append(relatives, relative)
		return nil
	}

	for index := range req.Platform.Runtime.Commands {
		command := req.Platform.Runtime.Commands[index]
		if !command.Required {
			continue
		}
		target, err := RenderPath(command.Target, req.Roots)
		if err != nil {
			return nil, fmt.Errorf("install: command %q のtarget: %w", command.Name, err)
		}
		if addErr := add(target); addErr != nil {
			return nil, addErr
		}
		for _, arg := range command.Args {
			if !strings.Contains(arg, "{{") {
				// literal argはfileを指さない。
				continue
			}
			value, argErr := RenderPath(arg, req.Roots)
			if argErr != nil {
				return nil, fmt.Errorf("install: command %q のargs %q: %w",
					command.Name, arg, argErr)
			}
			if addErr := add(value); addErr != nil {
				return nil, addErr
			}
		}
	}
	sort.Strings(relatives)
	return relatives, nil
}

// payloadRelative はpayload rootからの相対pathを返す。
//
// 第2戻り値がfalseならpayload外を指している。
func payloadRelative(
	value, payload domain.PathValue, host domain.Platform,
) (string, bool, error) {
	if value.IsZero() || value.Path() == "" {
		return "", false, errors.New("install: command targetが未設定")
	}
	if !security.IsContained(payload.Path(), value.Path(), host) {
		return "", false, nil
	}
	separator := security.PathSeparator(host)
	rest := strings.TrimPrefix(value.Path(), payload.Path())
	rest = strings.TrimPrefix(rest, separator)
	if rest == "" {
		// payload root自体はfileではない。
		return "", false, nil
	}
	// receiptのpathはPOSIX slashで持つ（§14の例が`payload/node.exe`）。
	return strings.ReplaceAll(rest, separator, "/"), true, nil
}

// hashPayloadFile は1 fileの内部SHA-256とbyte数を返す。
func hashPayloadFile(
	filesystem port.FileSystem, path domain.PathValue,
) (string, int64, error) {
	reader, err := filesystem.Open(path.Path())
	if err != nil {
		return "", 0, fmt.Errorf("開けない: %w", err)
	}
	defer func() { _ = reader.Close() }()

	digest, size, hashErr := security.InternalStreamDigest(reader, commandTargetMaxBytes)
	if hashErr != nil {
		return "", 0, hashErr
	}
	return digest, size, nil
}

// commandTargetMaxBytes は1 command targetの上限である。
//
// docs/04-storage-and-data.md §21のarchive単一file上限と同じ値を使う。
// command targetは展開済みpayload内のfileであり、その上限を超える実体は
// そもそも展開されない。
const commandTargetMaxBytes = ArchiveFileMaxBytes
