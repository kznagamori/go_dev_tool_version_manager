package domain

import (
	"errors"
	"strings"
	"testing"
)

// TestErrorCodeSetIsClosed はdocs/03-cli.md §7の「全34 error code」を固定する。
func TestErrorCodeSetIsClosed(t *testing.T) {
	all := AllErrorCodes()
	if len(all) != ErrorCodeCount {
		t.Fatalf("AllErrorCodes = %d件, want %d件", len(all), ErrorCodeCount)
	}
	if len(exitCodes) != ErrorCodeCount {
		t.Fatalf("exitCodes = %d件, want %d件", len(exitCodes), ErrorCodeCount)
	}

	seen := make(map[ErrorCode]bool, len(all))
	for _, code := range all {
		if seen[code] {
			t.Errorf("AllErrorCodesに %s が重複している", code)
		}
		seen[code] = true
		if !code.IsKnown() {
			t.Errorf("%s がexitCodes表に無い", code)
		}
		if !strings.HasPrefix(string(code), "E_") {
			t.Errorf("%s が `E_` で始まらない", code)
		}
	}
	for code := range exitCodes {
		if !seen[code] {
			t.Errorf("exitCodes表の %s がAllErrorCodesに無い", code)
		}
	}
}

// TestExitCodeMapping はdocs/03-cli.md §7の表と1対1で一致することを確かめる。
//
// 「全34 error codeはexit 1～11のexactly 1件へmapする」という規定を、表の全行を
// 書き下して固定する。実装表をそのまま読み直すのではなく仕様表を独立に持つことで、
// 実装側の写し間違いを検出できる。
func TestExitCodeMapping(t *testing.T) {
	want := map[int][]ErrorCode{
		1:  {CodeInternal},
		2:  {CodeUsage},
		3:  {CodeConfigInvalid, CodeProjectConfigInvalid},
		4:  {CodeToolUnknown, CodeVersionInvalid, CodeVersionNotFound, CodeVersionNotInstalled, CodePlatformUnsupported, CodeUnsupportedShell},
		5:  {CodeNetwork, CodeOffline, CodeCatalogMissing},
		6:  {CodeChecksumMismatch, CodeRegistryInvalid, CodeDefinitionInvalid, CodeArchiveUnsafe, CodeProbeFailed},
		7:  {CodePermission, CodeFilesystem, CodePathConflict, CodePathUnsafe, CodeLinkFailed, CodeShellProfileConflict, CodePathIntegrationFailed},
		8:  {CodeLockTimeout, CodePlanStale, CodeStateConflict, CodeConflict},
		9:  {CodeStateCorrupt, CodeReceiptInvalid},
		10: {CodeCancelled},
		11: {CodeApprovalRequired, CodeApprovalDenied},
	}

	total := 0
	for exit, codes := range want {
		total += len(codes)
		for _, code := range codes {
			if got := code.ExitCode(); got != exit {
				t.Errorf("%s.ExitCode() = %d, want %d", code, got, exit)
			}
		}
	}
	if total != ErrorCodeCount {
		t.Fatalf("仕様表の合計が%d件、want %d件", total, ErrorCodeCount)
	}
}

// TestPlatformErrorCodesAreSubset はdocs/09-platform.md §9の7件が
// §7の閉じた集合に含まれることを確かめる。
//
// docs/02-architecture.md §14は両表の和集合をv0.1の閉じた集合と定める。
// platform側に§7へ無いcodeが増えると終了codeへ写像できなくなる。
func TestPlatformErrorCodesAreSubset(t *testing.T) {
	platform := []ErrorCode{
		CodePlatformUnsupported, CodePermission, CodePathUnsafe, CodeLinkFailed,
		CodeShellProfileConflict, CodePathIntegrationFailed, CodeUnsupportedShell,
	}
	if len(platform) != 7 {
		t.Fatalf("platform code = %d件, want 7件", len(platform))
	}
	for _, code := range platform {
		if !code.IsKnown() {
			t.Errorf("platform code %s がdocs/03-cli.md §7の34件に無い", code)
		}
	}
}

func TestParseErrorCodeRejectsUnknown(t *testing.T) {
	for _, text := range []string{"", "E_UNKNOWN", "e_internal", "INTERNAL", "E_INTERNAL "} {
		if _, err := ParseErrorCode(text); err == nil {
			t.Errorf("ParseErrorCode(%q) = nil, want error", text)
		}
	}
	if code, err := ParseErrorCode("E_INTERNAL"); err != nil || code != CodeInternal {
		t.Errorf("ParseErrorCode(E_INTERNAL) = %v, %v", code, err)
	}
}

// TestUnknownErrorCodeFallsBackToInternal はfail closedの既定を固定する。
func TestUnknownErrorCodeFallsBackToInternal(t *testing.T) {
	var unknown ErrorCode = "E_NOT_IN_TABLE"
	if got := unknown.ExitCode(); got != 1 {
		t.Errorf("未知codeのExitCode = %d, want 1", got)
	}
	if unknown.IsKnown() {
		t.Error("未知codeがIsKnown()=trueになった")
	}
}

func TestExitConstants(t *testing.T) {
	if ExitSuccess != 0 {
		t.Errorf("ExitSuccess = %d, want 0", ExitSuccess)
	}
	if ExitDoctorUnhealthy != 12 {
		t.Errorf("ExitDoctorUnhealthy = %d, want 12", ExitDoctorUnhealthy)
	}
	// exit 12はerror codeから到達しない（docs/03-cli.md §7）。
	for _, code := range AllErrorCodes() {
		if code.ExitCode() == ExitDoctorUnhealthy {
			t.Errorf("%s がexit 12へmapしている", code)
		}
		if exit := code.ExitCode(); exit < 1 || exit > 11 {
			t.Errorf("%s.ExitCode() = %d, want 1〜11", code, exit)
		}
	}
}

// TestNonRetryableCodesRejectRetryable はdocs/02-architecture.md §14の8件を固定する。
func TestNonRetryableCodesRejectRetryable(t *testing.T) {
	forbidden := NonRetryableCodes()
	if len(forbidden) != 8 {
		t.Fatalf("NonRetryableCodes = %d件, want 8件", len(forbidden))
	}
	messageID, err := ParseMessageID("error.test_case")
	if err != nil {
		t.Fatalf("ParseMessageID = %v", err)
	}

	for _, code := range forbidden {
		if IsRetryableAllowed(code) {
			t.Errorf("IsRetryableAllowed(%s) = true, want false", code)
		}
		bad := &Error{Code: code, MessageID: messageID, Retryable: true}
		if err := bad.Validate(); err == nil {
			t.Errorf("%s のretryable=trueがValidateを通った", code)
		}
		good := &Error{Code: code, MessageID: messageID, Retryable: false}
		if err := good.Validate(); err != nil {
			t.Errorf("%s のretryable=falseがValidateで落ちた: %v", code, err)
		}
	}

	// 8件以外はretryable=trueを許す。
	if !IsRetryableAllowed(CodeNetwork) {
		t.Error("IsRetryableAllowed(E_NETWORK) = false, want true")
	}
	retryable := &Error{Code: CodeNetwork, MessageID: messageID, Retryable: true}
	if err := retryable.Validate(); err != nil {
		t.Errorf("E_NETWORKのretryable=trueがValidateで落ちた: %v", err)
	}
}

func TestErrorValidateReportsEveryDefect(t *testing.T) {
	bad := &Error{
		Code:       "E_NOT_IN_TABLE",
		Parameters: Parameters{"Bad Key": StringScalar("x")},
		PathRole:   PathRole("not-a-role"),
	}
	err := bad.Validate()
	if err == nil {
		t.Fatal("Validate = nil, want error")
	}
	for _, want := range []string{"34件", "message ID", "parameter key", "path_role"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error に %q が含まれない:\n%v", want, err)
		}
	}
}

// TestErrorStringOmitsCause は内部errorが表示文字列へ漏れないことを固定する。
//
// docs/02-architecture.md §14は「`Cause`はdebug log用でJSON/public messageへ
// 直接serializeしない」と定める。Error()を利用者へ出しても漏れないようにする。
func TestErrorStringOmitsCause(t *testing.T) {
	const secret = "postgres://user:hunter2@db.internal/gdtvm"
	messageID, err := ParseMessageID("error.state_corrupt")
	if err != nil {
		t.Fatalf("ParseMessageID = %v", err)
	}
	tool, err := ParseToolID("node")
	if err != nil {
		t.Fatalf("ParseToolID = %v", err)
	}
	typed := &Error{
		Code:      CodeStateCorrupt,
		MessageID: messageID,
		Operation: "install",
		Tool:      tool,
		PathRole:  RoleState,
		Cause:     errors.New(secret),
	}

	text := typed.Error()
	if strings.Contains(text, secret) || strings.Contains(text, "hunter2") {
		t.Fatalf("Error()にcauseが含まれた: %q", text)
	}
	for _, want := range []string{"E_STATE_CORRUPT", "error.state_corrupt", "install", "node", "state"} {
		if !strings.Contains(text, want) {
			t.Errorf("Error() = %q に %q が含まれない", text, want)
		}
	}
	if !errors.Is(typed, typed.Cause) {
		t.Error("Unwrapでcauseを辿れない")
	}
}

func TestInternalConversion(t *testing.T) {
	cause := errors.New("予期しない失敗")
	typed := Internal(cause)

	if typed.Code != CodeInternal {
		t.Errorf("Code = %s, want %s", typed.Code, CodeInternal)
	}
	if typed.ExitCode() != 1 {
		t.Errorf("ExitCode = %d, want 1", typed.ExitCode())
	}
	if typed.Retryable {
		t.Error("Retryable = true, want false")
	}
	if typed.MessageID.String() != MessageIDInternal {
		t.Errorf("MessageID = %q, want %q", typed.MessageID, MessageIDInternal)
	}
	if err := typed.Validate(); err != nil {
		t.Errorf("Validate = %v, want nil", err)
	}
	if !errors.Is(typed, cause) {
		t.Error("Unwrapでcauseを辿れない")
	}
}

func TestCodeOfAndExitCodeOf(t *testing.T) {
	messageID, err := ParseMessageID("error.network")
	if err != nil {
		t.Fatalf("ParseMessageID = %v", err)
	}
	typed := &Error{Code: CodeNetwork, MessageID: messageID}

	tests := []struct {
		name     string
		err      error
		wantCode ErrorCode
		wantOK   bool
		wantExit int
	}{
		{"nil", nil, CodeInternal, false, ExitSuccess},
		{"typed", typed, CodeNetwork, true, 5},
		{"wrapped", errors.Join(errors.New("外側"), typed), CodeNetwork, true, 5},
		{"plain", errors.New("素のerror"), CodeInternal, false, 1},
		{"未知code", &Error{Code: "E_NOT_IN_TABLE"}, CodeInternal, false, 1},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			code, ok := CodeOf(test.err)
			if test.err == nil {
				// nilはCodeOfの対象外。ExitCodeOfだけを見る。
				if got := ExitCodeOf(test.err); got != test.wantExit {
					t.Errorf("ExitCodeOf = %d, want %d", got, test.wantExit)
				}
				return
			}
			if code != test.wantCode || ok != test.wantOK {
				t.Errorf("CodeOf = %v, %v, want %v, %v", code, ok, test.wantCode, test.wantOK)
			}
			if got := ExitCodeOf(test.err); got != test.wantExit {
				t.Errorf("ExitCodeOf = %d, want %d", got, test.wantExit)
			}
		})
	}
}
