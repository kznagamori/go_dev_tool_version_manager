package events

import (
	"context"
	"time"
)

// EventType は event 種別である（10章7節）。progress の中間 event は coalesce できるが、
// 開始・終了・warning・approval を落とさない。schema の完全な event 契約は W08-03 で拡張する。
type EventType string

// 必須 event type（10章7節）。
const (
	TypeOperationStarted   EventType = "operation-started"
	TypeOperationCompleted EventType = "operation-completed"
	TypeOperationFailed    EventType = "operation-failed"
	TypePlanCreated        EventType = "plan-created"
	TypeApprovalRequired   EventType = "approval-required"
	TypeApprovalResolved   EventType = "approval-resolved"
	TypeDownloadStarted    EventType = "download-started"
	TypeDownloadProgress   EventType = "download-progress"
	TypeDownloadCompleted  EventType = "download-completed"
	TypeVerificationStart  EventType = "verification-started"
	TypeVerificationDone   EventType = "verification-completed"
	TypeStepStarted        EventType = "step-started"
	TypeStepOutput         EventType = "step-output"
	TypeStepCompleted      EventType = "step-completed"
	TypeInstallCommitted   EventType = "install-committed"
	TypeSelectionChanged   EventType = "selection-changed"
	TypeRegistryChanged    EventType = "registry-changed"
	TypeWarning            EventType = "warning"
	TypeDiagnostic         EventType = "diagnostic"
	TypeCleanupWarning     EventType = "cleanup-warning"
)

// Event は operation の進捗・警告・監査を表す（10章7節）。翻訳文・ANSI・秘密値を含めず、
// message_id と非秘密 args で UI へ渡す。
type Event struct {
	Schema            int
	ID                string
	OperationID       string
	ParentOperationID string
	Timestamp         time.Time
	Sequence          int64
	Type              EventType
	MessageID         string
	Args              map[string]string
	// Data は type 固有 payload（進捗の bytes/items 等）。秘密値を含めない。
	Data any
}

// EventSink は event を受け取る抽象 port である（02章5節の UI port を分離したもの）。
// sink が遅い場合 progress 中間 event は coalesce できるが、開始・終了・warning・approval を
// 落としてはならない。
type EventSink interface {
	// Emit は event を送出する。実装はブロッキングを避ける。
	Emit(event Event)
}

// ApprovalKind は承認の種類である（10章8節）。
type ApprovalKind string

// ApprovalKind の値。
const (
	ApprovalNormal          ApprovalKind = "normal"
	ApprovalThirdParty      ApprovalKind = "third-party"
	ApprovalUnverified      ApprovalKind = "unverified"
	ApprovalLocalDefinition ApprovalKind = "local-definition"
	ApprovalShellHook       ApprovalKind = "shell-hook"
	ApprovalProfileChange   ApprovalKind = "profile-change"
	ApprovalDestructive     ApprovalKind = "destructive"
)

// ApprovalDecision は承認結果である。既定は常に deny（10章8節）。
type ApprovalDecision string

// ApprovalDecision の値。
const (
	DecisionAllow ApprovalDecision = "allow"
	DecisionDeny  ApprovalDecision = "deny"
)

// ApprovalEvidence は承認画面に示す非秘密の根拠である（10章8節、11章）。
type ApprovalEvidence struct {
	URLs     []string
	Licenses []string
	Hashes   []string
	Commands []string
	Paths    []string
}

// ApprovalRequest は承認要求である（10章8節）。default answer は常に deny。
type ApprovalRequest struct {
	ID             string
	Kind           ApprovalKind
	TitleMessageID string
	MessageID      string
	Args           map[string]string
	Evidence       ApprovalEvidence
	// CanAssumeYes は --yes で承認可能かどうか（kind と policy に依存、10章8節の表）。
	CanAssumeYes bool
}

// ApprovalProvider は確認を要求する抽象 port である（02章5節）。非対話モードでは
// policy に基づき即答し、入力待ちしない。既定回答は deny。
type ApprovalProvider interface {
	// Resolve は承認要求を解決して decision を返す。context cancel に応答する。
	Resolve(ctx context.Context, req ApprovalRequest) (ApprovalDecision, error)
}
