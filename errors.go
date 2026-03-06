package nono

// #include "internal/clib/nono.h"
import "C"
import "fmt"

// ErrorCode is the numeric code returned by a failing nono FFI call.
// All failure codes are negative; zero indicates success (NONO_ERROR_CODE_OK).
// Codes are not guaranteed to be contiguous — use errors.Is with the named
// Err* sentinels or switch on ErrCode* constants rather than range-checking.
type ErrorCode int

// Error code constants corresponding to the C library's NonoErrorCode enum.
// Values are sourced directly from the C enum to prevent silent divergence
// if the C library renumbers any code.
const (
	ErrCodePathNotFound         ErrorCode = ErrorCode(C.NONO_ERROR_CODE_ERR_PATH_NOT_FOUND)
	ErrCodeExpectedDirectory    ErrorCode = ErrorCode(C.NONO_ERROR_CODE_ERR_EXPECTED_DIRECTORY)
	ErrCodeExpectedFile         ErrorCode = ErrorCode(C.NONO_ERROR_CODE_ERR_EXPECTED_FILE)
	ErrCodePathCanonicalization ErrorCode = ErrorCode(C.NONO_ERROR_CODE_ERR_PATH_CANONICALIZATION)
	ErrCodeNoCapabilities       ErrorCode = ErrorCode(C.NONO_ERROR_CODE_ERR_NO_CAPABILITIES)
	ErrCodeSandboxInit          ErrorCode = ErrorCode(C.NONO_ERROR_CODE_ERR_SANDBOX_INIT)
	ErrCodeUnsupportedPlatform  ErrorCode = ErrorCode(C.NONO_ERROR_CODE_ERR_UNSUPPORTED_PLATFORM)
	ErrCodeBlockedCommand       ErrorCode = ErrorCode(C.NONO_ERROR_CODE_ERR_BLOCKED_COMMAND)
	ErrCodeConfigParse          ErrorCode = ErrorCode(C.NONO_ERROR_CODE_ERR_CONFIG_PARSE)
	ErrCodeProfileParse         ErrorCode = ErrorCode(C.NONO_ERROR_CODE_ERR_PROFILE_PARSE)
	ErrCodeIO                   ErrorCode = ErrorCode(C.NONO_ERROR_CODE_ERR_IO)
	ErrCodeInvalidArg           ErrorCode = ErrorCode(C.NONO_ERROR_CODE_ERR_INVALID_ARG)
	ErrCodeTrustVerification    ErrorCode = ErrorCode(C.NONO_ERROR_CODE_ERR_TRUST_VERIFICATION)
	ErrCodeUnknown              ErrorCode = ErrorCode(C.NONO_ERROR_CODE_ERR_UNKNOWN)
)

// Sentinel errors for use with errors.Is. Example:
//
//	if errors.Is(err, nono.ErrPathNotFound) { ... }
//
// The [Error.Is] method matches by code only, so the message from the C
// library does not need to match the sentinel's message.
// Both fields of [Error] are unexported, so callers cannot mutate sentinel
// values through an errors.As pointer — the error codes are stable for
// the lifetime of the process.
var (
	ErrPathNotFound         error = &Error{code: ErrCodePathNotFound, message: "path not found"}
	ErrExpectedDirectory    error = &Error{code: ErrCodeExpectedDirectory, message: "expected directory"}
	ErrExpectedFile         error = &Error{code: ErrCodeExpectedFile, message: "expected file"}
	ErrPathCanonicalization error = &Error{code: ErrCodePathCanonicalization, message: "path canonicalization failed"}
	ErrNoCapabilities       error = &Error{code: ErrCodeNoCapabilities, message: "no capabilities"}
	ErrSandboxInit          error = &Error{code: ErrCodeSandboxInit, message: "sandbox init failed"}
	ErrUnsupportedPlatform  error = &Error{code: ErrCodeUnsupportedPlatform, message: "unsupported platform"}
	ErrBlockedCommand       error = &Error{code: ErrCodeBlockedCommand, message: "blocked command"}
	ErrConfigParse          error = &Error{code: ErrCodeConfigParse, message: "config parse error"}
	ErrProfileParse         error = &Error{code: ErrCodeProfileParse, message: "profile parse error"}
	ErrIO                   error = &Error{code: ErrCodeIO, message: "i/o error"}
	ErrInvalidArg           error = &Error{code: ErrCodeInvalidArg, message: "invalid argument"}
	ErrTrustVerification    error = &Error{code: ErrCodeTrustVerification, message: "trust verification failed"}
	ErrUnknown              error = &Error{code: ErrCodeUnknown, message: "unknown error"}
)

// Error is returned by nono FFI calls that fail.
// Use [Error.Code] to identify the failure kind; compare against the ErrCode*
// constants or use errors.Is with the named Err* sentinels.
// Use [Error.Message] to obtain the human-readable description.
//
// Both fields are unexported to prevent callers from mutating sentinel error
// values through an errors.As pointer, which would corrupt future errors.Is
// comparisons. Use the Code() and Message() accessor methods to read them.
type Error struct {
	code    ErrorCode
	message string
}

// Code returns the numeric error code.
func (e *Error) Code() ErrorCode { return e.code }

// Message returns the human-readable error description from the C library.
func (e *Error) Message() string { return e.message }

// Error implements the error interface. The string is the C library's
// description. When no message is available the code is included instead.
func (e *Error) Error() string {
	if e.message != "" {
		return e.message
	}
	// Defensive fallback: mapError always populates message from nono_last_error(),
	// and staticError always receives a non-empty literal, so this branch is
	// unreachable through normal code paths. It guards against a zero-value &Error{}.
	return fmt.Sprintf("nono error %d", int(e.code))
}

// Is reports whether e has the same error code as target, enabling
// errors.Is checks with the named sentinel errors:
//
//	errors.Is(err, nono.ErrPathNotFound)
func (e *Error) Is(target error) bool {
	t, ok := target.(*Error)
	if !ok {
		return false
	}
	return e.code == t.code
}

// mapError retrieves the last error message from the C library's thread-local
// state, wraps it with the given numeric code, then clears the thread-local
// state. Call only immediately after a failing FFI call while holding
// runtime.LockOSThread so that nono_last_error reads the error set by that call.
// See the lockOSThread policy block in nono.go for the full invariant.
func mapError(code C.int) error {
	// goString is nil-safe and frees the C string via nono_string_free.
	msg := goString(C.nono_last_error())
	if msg == "" {
		msg = "unknown error"
	}
	// Clear the thread-local error slot so stale messages are not visible to
	// any subsequent nono_last_error call on this OS thread.
	C.nono_clear_error()
	return &Error{code: ErrorCode(code), message: msg}
}

// staticError constructs an Error from a known code and message without
// consulting nono_last_error. Use when the C function returned nil/false
// without setting a thread-local error (e.g. null-pointer guard paths).
func staticError(code ErrorCode, msg string) error {
	return &Error{code: code, message: msg}
}
