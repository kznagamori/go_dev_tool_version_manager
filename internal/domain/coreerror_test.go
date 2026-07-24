package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestNewError_Defaults(t *testing.T) {
	e := NewError(CodeToolUnknown, "error.tool_unknown")
	if e.Code() != CodeToolUnknown {
		t.Errorf("Code = %q", e.Code())
	}
	if e.Category() != CategoryResolution {
		t.Errorf("既定 category = %q, want resolution", e.Category())
	}
	if e.MessageID() != "error.tool_unknown" {
		t.Errorf("MessageID = %q", e.MessageID())
	}
}

func TestNewError_Options(t *testing.T) {
	cause := errors.New("root cause")
	e := NewError(CodeNetwork, "error.network",
		WithCause(cause),
		WithOperationID("op-1"),
		WithArgs(map[string]string{"host": "example.com"}),
		WithContext(ErrorContext{ToolID: "node", Path: "/x"}),
		WithRetryable(true),
		WithRemediations("remedy.retry", "remedy.offline"),
		WithSafeDetails("connection reset"),
	)
	if e.OperationID() != "op-1" {
		t.Errorf("OperationID = %q", e.OperationID())
	}
	if e.Args()["host"] != "example.com" {
		t.Errorf("Args[host] = %q", e.Args()["host"])
	}
	if e.Context().ToolID != "node" || e.Context().Path != "/x" {
		t.Errorf("Context = %+v", e.Context())
	}
	if !e.Retryable() {
		t.Error("Retryable = false")
	}
	if len(e.Remediations()) != 2 {
		t.Errorf("Remediations = %v", e.Remediations())
	}
	if !errors.Is(e, cause) {
		t.Error("errors.Is(e, cause) = false")
	}
}

func TestCoreError_ArgsAreCopied(t *testing.T) {
	src := map[string]string{"k": "v"}
	e := NewError(CodeUsage, "error.usage", WithArgs(src))
	src["k"] = "mutated"
	if e.Args()["k"] != "v" {
		t.Error("WithArgs は呼び出し側 map の変更から独立しているべき")
	}
}

func TestCoreError_Error_NoSecrets(t *testing.T) {
	e := NewError(CodeDigestMismatch, "error.digest", WithSafeDetails("expected!=actual"))
	s := e.Error()
	if !strings.Contains(s, "E_DIGEST_MISMATCH") {
		t.Errorf("Error() は code を含むべき: %q", s)
	}
	if !strings.Contains(s, "error.digest") {
		t.Errorf("Error() は message ID を含むべき: %q", s)
	}
}

func TestCoreError_AsTarget(t *testing.T) {
	err := error(NewError(CodeLocked, "error.locked"))
	var ce *CoreError
	if !errors.As(err, &ce) {
		t.Fatal("errors.As で *CoreError を取り出せるべき")
	}
	if ce.Code() != CodeLocked {
		t.Errorf("As で取り出した Code = %q", ce.Code())
	}
}

func TestDefaultCategory_TimeoutIsNetwork(t *testing.T) {
	// E_TIMEOUT の既定 category は network（04章7節、既定は network=5）。
	if got := defaultCategory(CodeTimeout); got != CategoryNetwork {
		t.Errorf("defaultCategory(E_TIMEOUT) = %q, want network", got)
	}
}

func TestDefaultCategory_AllCodesMapped(t *testing.T) {
	// 全 code が internal 以外の具体 category へ対応すること（未分類の取りこぼし防止）。
	all := []ErrorCode{
		CodeUsage, CodeConfigParse, CodeConfigSchema, CodeHomeNotWritable, CodeRegistryInvalid,
		CodeToolUnknown, CodeCommandAmbiguous, CodePlatformUnsupported, CodeVersionInvalid,
		CodeVersionNotFound, CodeVersionNotInstalled, CodeCatalogMissing, CodeOffline, CodeNetwork,
		CodeDigestMismatch, CodeSignatureInvalid, CodeUpdateFailed, CodePolicyDenied,
		CodeDefinitionUnapproved, CodeArchiveUnsafe, CodeDependencyCycle, CodeDependencyConflict,
		CodeProcessFailed, CodeTimeout, CodeLocked, CodePlanStale, CodeStateCorrupt, CodeLinkFailed,
		CodeCancelled, CodePartial,
	}
	for _, c := range all {
		if got := defaultCategory(c); got == CategoryInternal {
			t.Errorf("defaultCategory(%s) = internal（具体 category を割り当てるべき）", c)
		}
	}
}
