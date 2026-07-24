package ports

import "github.com/kznagamori/go_dev_tool_version_manager/internal/domain"

// ReleaseIntegrityVerifier は release asset 名・size・SHA-256、archive 内容、埋込み
// registry hash の照合を担う抽象 port である（02章5節, 11章2節）。client release 検証
// 専用であり、上流 tool artifact 署名には用いない。詳細契約は W04 で拡張する。
type ReleaseIntegrityVerifier interface {
	// VerifyArchiveDigest は archive の SHA-256 が expected と一致するか検証する。
	VerifyArchiveDigest(path string, expected domain.Digest) error
	// VerifyRegistryTreeHash は registryRoot の tree hash（07章8節）を計算し expected と
	// 照合する。
	VerifyRegistryTreeHash(registryRoot string, expected domain.Digest) error
}

// SignatureVerifier は上流 tool artifact の PGP-detached/Minisign 署名検証を担う抽象
// port である（02章5節, 06章6.1節）。client release 検証には用いない。詳細契約は W05 で
// 拡張する。
type SignatureVerifier interface {
	// VerifyPGPDetached は data の detached PGP 署名を検証し、鍵の fingerprint を照合する。
	VerifyPGPDetached(dataPath, signaturePath, keyFilePath, expectedFingerprint string) error
	// VerifyMinisign は data の Minisign 署名を trusted public key で検証する。
	VerifyMinisign(dataPath, signaturePath, trustedPublicKey string) error
}
