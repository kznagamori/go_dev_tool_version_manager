package main

import (
	"errors"
	"testing"

	"github.com/kznagamori/go_dev_tool_version_manager/internal/domain"
)

func TestExitCode_NilAndNonCore(t *testing.T) {
	if got := exitCode(nil); got != exitSuccess {
		t.Errorf("exitCode(nil) = %d, want %d", got, exitSuccess)
	}
	if got := exitCode(errors.New("plain")); got != exitGeneral {
		t.Errorf("exitCode(plain error) = %d, want %d", got, exitGeneral)
	}
}

func TestExitCode_ByCode(t *testing.T) {
	cases := map[domain.ErrorCode]int{
		domain.CodeUsage:                exitUsage,
		domain.CodeConfigParse:          exitConfig,
		domain.CodeConfigSchema:         exitConfig,
		domain.CodeRegistryInvalid:      exitConfig,
		domain.CodeDependencyCycle:      exitConfig,
		domain.CodeToolUnknown:          exitNotFound,
		domain.CodeVersionInvalid:       exitNotFound,
		domain.CodeVersionNotFound:      exitNotFound,
		domain.CodeVersionNotInstalled:  exitNotFound,
		domain.CodePlatformUnsupported:  exitNotFound,
		domain.CodeCatalogMissing:       exitNetwork,
		domain.CodeOffline:              exitNetwork,
		domain.CodeNetwork:              exitNetwork,
		domain.CodeDigestMismatch:       exitSecurity,
		domain.CodeSignatureInvalid:     exitSecurity,
		domain.CodePolicyDenied:         exitSecurity,
		domain.CodeArchiveUnsafe:        exitSecurity,
		domain.CodeDefinitionUnapproved: exitSecurity,
		domain.CodeHomeNotWritable:      exitFilesystem,
		domain.CodeLinkFailed:           exitFilesystem,
		domain.CodeProcessFailed:        exitProcess,
		domain.CodeLocked:               exitLock,
		domain.CodeCancelled:            exitCancelled,
		domain.CodePartial:              exitPartial,
		domain.CodeStateCorrupt:         exitGeneral,
		domain.CodePlanStale:            exitGeneral,
		domain.CodeCommandAmbiguous:     exitGeneral,
		domain.CodeUpdateFailed:         exitGeneral,
		domain.CodeDependencyConflict:   exitGeneral,
	}
	for code, want := range cases {
		err := domain.NewError(code, "msg")
		if got := exitCode(err); got != want {
			t.Errorf("exitCode(%s) = %d, want %d", code, got, want)
		}
	}
}

func TestExitCode_TimeoutContext(t *testing.T) {
	// E_TIMEOUT は network 起因で 5、process 起因で 8（04章7節）。
	netTimeout := domain.NewError(domain.CodeTimeout, "msg") // 既定 category=network
	if got := exitCode(netTimeout); got != exitNetwork {
		t.Errorf("network timeout = %d, want %d", got, exitNetwork)
	}
	procTimeout := domain.NewError(domain.CodeTimeout, "msg", domain.WithCategory(domain.CategoryProcess))
	if got := exitCode(procTimeout); got != exitProcess {
		t.Errorf("process timeout = %d, want %d", got, exitProcess)
	}
}

func TestExitCode_WrappedCoreError(t *testing.T) {
	// cause chain 越しでも errors.As で CoreError を見つけて対応させる。
	wrapped := errors.Join(errors.New("context"), domain.NewError(domain.CodeOffline, "msg"))
	if got := exitCode(wrapped); got != exitNetwork {
		t.Errorf("wrapped offline = %d, want %d", got, exitNetwork)
	}
}

func TestExitDoctorErrorConstant(t *testing.T) {
	// doctor の code 12 は E_* を持たず CLI が別途返す（04章7節）。定数の値を固定する。
	if exitDoctorError != 12 {
		t.Errorf("exitDoctorError = %d, want 12", exitDoctorError)
	}
}
