package store

import (
	"encoding/base64"
	"fmt"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
)

// ParseSetupBackup は`state/backups/setup-<backup-id>.toml`を読む（§10）。
//
// raw bytesはbase64で保存する。Windowsはregistry valueのUTF-16LEそのもの、
// profileはfile raw bytesであり、どちらもTOML stringとして安全に表現できない
// byte列を含みうるためである。
func ParseSetupBackup(data []byte) (SetupBackup, *domain.Error) {
	var file backupFile
	if err := loadStateTOML(data, &file); err != nil {
		return SetupBackup{}, stateError("state.backup_invalid", domain.RoleStateBackup, err)
	}
	value, err := buildSetupBackup(file)
	if err != nil {
		return SetupBackup{}, stateError("state.backup_invalid", domain.RoleStateBackup, err)
	}
	return value, nil
}

func buildSetupBackup(file backupFile) (SetupBackup, error) {
	var value SetupBackup
	if err := requireSchema("schema", file.Schema); err != nil {
		return value, err
	}
	backupID, err := requireIDField("backup_id", file.BackupID)
	if err != nil {
		return value, err
	}
	rootID, err := requireIDField("root_id", file.RootID)
	if err != nil {
		return value, err
	}
	kind, err := requireEnum("kind", file.Kind, backupKinds)
	if err != nil {
		return value, err
	}
	createdAt, err := requireTimestampField("created_at", file.CreatedAt)
	if err != nil {
		return value, err
	}
	target, err := requirePresent("target", file.Target)
	if err != nil {
		return value, err
	}
	if target == "" {
		return value, fmt.Errorf("targetが空")
	}
	existed, err := requireBool("existed", file.Existed)
	if err != nil {
		return value, err
	}
	valueType, err := requirePresent("value_type", file.ValueType)
	if err != nil {
		return value, err
	}
	// value typeはregistry valueだけが持つ。shell profileはfileであり型が無い。
	if kind == BackupShellProfile {
		if err := requireEmpty("value_type", valueType); err != nil {
			return value, err
		}
	}
	encoded, err := requirePresent("raw_bytes_base64", file.RawBytesBase64)
	if err != nil {
		return value, err
	}
	digest, err := requireDigestField("sha256", file.SHA256)
	if err != nil {
		return value, err
	}
	raw, err := decodeBackupRaw(encoded)
	if err != nil {
		return value, err
	}
	if err := checkBackupExistence(existed, raw, digest, valueType, kind); err != nil {
		return value, err
	}
	return SetupBackup{
		BackupID: backupID, RootID: rootID, Kind: kind, CreatedAt: createdAt,
		Target: target, Existed: existed, ValueType: valueType, Raw: raw, SHA256: digest,
	}, nil
}

// decodeBackupRaw はbase64をdecodeし、decode後のsize上限を適用する（§10）。
//
// 上限をdecode後に見るのは、圧縮やencodingで小さく見えるinputが展開後に
// memoryを食う経路を塞ぐためである（§21のfail closed方針）。
func decodeBackupRaw(encoded string) ([]byte, error) {
	if encoded == "" {
		return nil, nil
	}
	// StdEncodingはpaddingを要求する。RawStdEncodingを許すと同じbyte列に
	// 2通りの表現ができ、digestとfile内容の対応が1対1でなくなる。
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("raw_bytes_base64がbase64でない: %w", err)
	}
	if len(raw) > BackupRawMaxBytes {
		return nil, fmt.Errorf(
			"raw_bytes_base64のdecode後が%d byteを超える（%d byte）", BackupRawMaxBytes, len(raw))
	}
	return raw, nil
}

// checkBackupExistence は不存在の表現が1通りであることを確かめる（§10）。
//
// 同§が「不存在は`existed=false`, raw空, digest 64 zero」と定める。存在するのに
// rawが空、あるいは不存在なのにrawがあるbackupを受理すると、rollbackが
// 何を書き戻すべきか決まらない。
func checkBackupExistence(existed bool, raw []byte, digest, valueType string, kind BackupKind) error {
	if existed {
		if digest == zeroDigestHex {
			return fmt.Errorf("existed=trueのbackupのsha256が64 zeroである")
		}
		if kind == BackupWindowsUserPath && valueType == "" {
			return fmt.Errorf("existed=trueのwindows-user-path backupにvalue_typeが無い")
		}
		return nil
	}
	if len(raw) != 0 {
		return fmt.Errorf("existed=falseのbackupにraw bytesがある")
	}
	if digest != zeroDigestHex {
		return fmt.Errorf("existed=falseのbackupのsha256は64 zeroでなければならない")
	}
	if err := requireEmpty("value_type", valueType); err != nil {
		return err
	}
	return nil
}

// EncodeSetupBackup はsetup backupを書き出す（§10）。
//
// 呼出し側はowner-onlyのpermissionで保存し、latest 1世代だけを保持する。
// log/JSON/reportへrawを出さない（§10）。
func EncodeSetupBackup(value SetupBackup) ([]byte, *domain.Error) {
	file := backupFileOf(value)
	if _, err := buildSetupBackup(file); err != nil {
		return nil, stateError("state.backup_invalid", domain.RoleStateBackup, err)
	}
	data, err := encodeTOML(file)
	if err != nil {
		return nil, stateError("state.backup_invalid", domain.RoleStateBackup, err)
	}
	return data, nil
}

func backupFileOf(value SetupBackup) backupFile {
	schema := int64(SchemaVersion)
	kind := string(value.Kind)
	createdAt := formatTimestamp(value.CreatedAt)
	encoded := base64.StdEncoding.EncodeToString(value.Raw)
	return backupFile{
		Schema: &schema, BackupID: &value.BackupID, RootID: &value.RootID,
		Kind: &kind, CreatedAt: &createdAt, Target: &value.Target,
		Existed: &value.Existed, ValueType: &value.ValueType,
		RawBytesBase64: &encoded, SHA256: &value.SHA256,
	}
}
