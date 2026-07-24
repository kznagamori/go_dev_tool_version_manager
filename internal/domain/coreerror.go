package domain

import "strings"

// ErrorCode は Application Service 境界の安定したエラーコードである（10章9節）。
// GUI/CLI はこの値で分岐し、翻訳文を parse しない。CLI 終了コードへの対応は
// cmd/gdtvm が行う（04章7節）。
type ErrorCode string

// 10章9節の最低限の stable code。追加時は 04章7節の終了コード表と ExitCode の
// mapping を同じ変更で更新する。
const (
	CodeUsage                ErrorCode = "E_USAGE"                 // CLI usage/構文エラー
	CodeConfigParse          ErrorCode = "E_CONFIG_PARSE"          // 設定 TOML の parse 失敗
	CodeConfigSchema         ErrorCode = "E_CONFIG_SCHEMA"         // 設定/definition schema 違反
	CodeHomeNotWritable      ErrorCode = "E_HOME_NOT_WRITABLE"     // 管理 root へ書込み不可
	CodeRegistryInvalid      ErrorCode = "E_REGISTRY_INVALID"      // 同梱 registry 欠落/不正/非互換
	CodeToolUnknown          ErrorCode = "E_TOOL_UNKNOWN"          // 未知の tool ID
	CodeCommandAmbiguous     ErrorCode = "E_COMMAND_AMBIGUOUS"     // shim command の所有 tool が複数
	CodePlatformUnsupported  ErrorCode = "E_PLATFORM_UNSUPPORTED"  // 現在 platform 非対応
	CodeVersionInvalid       ErrorCode = "E_VERSION_INVALID"       // version 文字列が不正
	CodeVersionNotFound      ErrorCode = "E_VERSION_NOT_FOUND"     // catalog に該当版なし
	CodeVersionNotInstalled  ErrorCode = "E_VERSION_NOT_INSTALLED" // 該当版が未導入
	CodeCatalogMissing       ErrorCode = "E_CATALOG_MISSING"       // 検証済み catalog cache なし
	CodeOffline              ErrorCode = "E_OFFLINE"               // offline 指定でネットワーク不可
	CodeNetwork              ErrorCode = "E_NETWORK"               // ネットワーク失敗
	CodeDigestMismatch       ErrorCode = "E_DIGEST_MISMATCH"       // digest 不一致
	CodeSignatureInvalid     ErrorCode = "E_SIGNATURE_INVALID"     // 上流署名が不正
	CodeUpdateFailed         ErrorCode = "E_UPDATE_FAILED"         // self-update 失敗
	CodePolicyDenied         ErrorCode = "E_POLICY_DENIED"         // security policy により拒否
	CodeDefinitionUnapproved ErrorCode = "E_DEFINITION_UNAPPROVED" // local definition 未承認
	CodeArchiveUnsafe        ErrorCode = "E_ARCHIVE_UNSAFE"        // archive 安全検査違反
	CodeDependencyCycle      ErrorCode = "E_DEPENDENCY_CYCLE"      // 依存が循環
	CodeDependencyConflict   ErrorCode = "E_DEPENDENCY_CONFLICT"   // 依存/conflict 競合
	CodeProcessFailed        ErrorCode = "E_PROCESS_FAILED"        // 外部 process/hook 失敗
	CodeTimeout              ErrorCode = "E_TIMEOUT"               // timeout（文脈で network/process）
	CodeLocked               ErrorCode = "E_LOCKED"                // lock 競合/待機 timeout
	CodePlanStale            ErrorCode = "E_PLAN_STALE"            // Plan fingerprint が変化
	CodeStateCorrupt         ErrorCode = "E_STATE_CORRUPT"         // state 破損
	CodeLinkFailed           ErrorCode = "E_LINK_FAILED"           // link 作成/権限/filesystem 失敗
	CodeCancelled            ErrorCode = "E_CANCELLED"             // 操作 cancel
	CodePartial              ErrorCode = "E_PARTIAL"               // 部分成功
)

// Category は CoreError の粗い分類である（10章9節）。GUI の分岐補助と、E_TIMEOUT の
// 終了コード決定（network 起因 5 / process 起因 8、04章7節）に用いる。
type Category string

// Category enum の値。
const (
	CategoryUsage      Category = "usage"
	CategoryConfig     Category = "config"
	CategoryRegistry   Category = "registry"
	CategoryResolution Category = "resolution"
	CategoryDependency Category = "dependency"
	CategoryNetwork    Category = "network"
	CategorySecurity   Category = "security"
	CategoryFilesystem Category = "filesystem"
	CategoryProcess    Category = "process"
	CategoryLock       Category = "lock"
	CategoryUpdate     Category = "update"
	CategoryState      Category = "state"
	CategoryCancel     Category = "cancel"
	CategoryPartial    Category = "partial"
	CategoryInternal   Category = "internal"
)

// defaultCategory は code の既定 category を返す。E_TIMEOUT は文脈依存のため呼び出し
// 側が network/process を明示することを想定し、既定では network を返す。
func defaultCategory(code ErrorCode) Category {
	switch code {
	case CodeUsage:
		return CategoryUsage
	case CodeConfigParse, CodeConfigSchema:
		return CategoryConfig
	case CodeRegistryInvalid, CodeDefinitionUnapproved:
		return CategoryRegistry
	case CodeToolUnknown, CodeCommandAmbiguous, CodePlatformUnsupported,
		CodeVersionInvalid, CodeVersionNotFound, CodeVersionNotInstalled:
		return CategoryResolution
	case CodeDependencyCycle, CodeDependencyConflict:
		return CategoryDependency
	case CodeCatalogMissing, CodeOffline, CodeNetwork:
		return CategoryNetwork
	case CodeTimeout:
		return CategoryNetwork
	case CodeDigestMismatch, CodeSignatureInvalid, CodePolicyDenied, CodeArchiveUnsafe:
		return CategorySecurity
	case CodeHomeNotWritable, CodeLinkFailed:
		return CategoryFilesystem
	case CodeProcessFailed:
		return CategoryProcess
	case CodeLocked:
		return CategoryLock
	case CodeUpdateFailed:
		return CategoryUpdate
	case CodeStateCorrupt, CodePlanStale:
		return CategoryState
	case CodeCancelled:
		return CategoryCancel
	case CodePartial:
		return CategoryPartial
	default:
		return CategoryInternal
	}
}

// ErrorContext は CoreError に付随する非秘密の対象情報である（10章9節）。
type ErrorContext struct {
	ToolID     string
	InstallKey string
	Path       string
	StepID     string
}

// CoreError は Application Service 境界の型付きエラーである（10章9節）。翻訳文・ANSI・
// 秘密値を含めず、message_id と非秘密の args で UI へ渡す。cause chain は内部診断用。
type CoreError struct {
	code         ErrorCode
	category     Category
	messageID    string
	args         map[string]string
	operationID  string
	ctx          ErrorContext
	cause        error
	retryable    bool
	remediations []string
	safeDetails  string
}

// ErrorOption は NewError の可変オプションである。
type ErrorOption func(*CoreError)

// NewError は code と message ID から CoreError を生成する。category は既定で code から
// 導出し、WithCategory で上書きできる。
func NewError(code ErrorCode, messageID string, opts ...ErrorOption) *CoreError {
	e := &CoreError{
		code:      code,
		category:  defaultCategory(code),
		messageID: messageID,
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// WithCategory は category を上書きする。E_TIMEOUT の network/process 区別に用いる。
func WithCategory(c Category) ErrorOption { return func(e *CoreError) { e.category = c } }

// WithCause は内部診断用の原因エラーを設定する。
func WithCause(cause error) ErrorOption { return func(e *CoreError) { e.cause = cause } }

// WithOperationID は関連付ける operation ID を設定する。
func WithOperationID(id string) ErrorOption { return func(e *CoreError) { e.operationID = id } }

// WithArgs は message の非秘密 structured args を設定する。
func WithArgs(args map[string]string) ErrorOption {
	return func(e *CoreError) {
		e.args = make(map[string]string, len(args))
		for k, v := range args {
			e.args[k] = v
		}
	}
}

// WithContext は対象 tool/install/path/step の非秘密情報を設定する。
func WithContext(ctx ErrorContext) ErrorOption { return func(e *CoreError) { e.ctx = ctx } }

// WithRetryable は再試行可能フラグを設定する。
func WithRetryable(r bool) ErrorOption { return func(e *CoreError) { e.retryable = r } }

// WithRemediations は修正手順の message ID 群を設定する。
func WithRemediations(ids ...string) ErrorOption {
	return func(e *CoreError) { e.remediations = append([]string(nil), ids...) }
}

// WithSafeDetails は log/表示に安全な補足文字列を設定する（秘密値を含めない）。
func WithSafeDetails(d string) ErrorOption { return func(e *CoreError) { e.safeDetails = d } }

// Code は安定したエラーコードを返す。
func (e *CoreError) Code() ErrorCode { return e.code }

// Category は分類を返す。
func (e *CoreError) Category() Category { return e.category }

// MessageID は UI 翻訳用の message ID を返す。
func (e *CoreError) MessageID() string { return e.messageID }

// Args は非秘密 structured args を返す（呼び出し側は変更しないこと）。
func (e *CoreError) Args() map[string]string { return e.args }

// OperationID は関連 operation ID を返す。
func (e *CoreError) OperationID() string { return e.operationID }

// Context は対象情報を返す。
func (e *CoreError) Context() ErrorContext { return e.ctx }

// Retryable は再試行可能かどうかを返す。
func (e *CoreError) Retryable() bool { return e.retryable }

// Remediations は修正手順 message ID 群を返す。
func (e *CoreError) Remediations() []string { return e.remediations }

// SafeDetails は安全な補足文字列を返す。
func (e *CoreError) SafeDetails() string { return e.safeDetails }

// Error は秘密値を含まない要約文字列を返す。人向け表示は i18n 層が message ID から
// 生成するため、本文字列は診断用途とする。
func (e *CoreError) Error() string {
	var b strings.Builder
	b.WriteString(string(e.code))
	if e.messageID != "" {
		b.WriteString(" (")
		b.WriteString(e.messageID)
		b.WriteString(")")
	}
	if e.safeDetails != "" {
		b.WriteString(": ")
		b.WriteString(e.safeDetails)
	}
	return b.String()
}

// Unwrap は cause chain を返し、errors.Is/As と連携する。
func (e *CoreError) Unwrap() error { return e.cause }
