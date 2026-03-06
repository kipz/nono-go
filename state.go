package nono

// #include "internal/clib/nono.h"
import "C"
import (
	"runtime"
	"sync"
	"unsafe"
)

// errSandboxStateClosed is the error message for operations on a closed SandboxState.
const errSandboxStateClosed = "sandbox state is closed"

// SandboxState is a serializable snapshot of a [CapabilitySet].
// Use [StateFromCaps] or [StateFromJSON] to create one.
//
// SandboxState is safe for concurrent use. Multiple goroutines may call
// JSON concurrently; only Close requires exclusive access.
type SandboxState struct {
	mu  sync.RWMutex
	ptr *C.struct_NonoSandboxState
}

// StateFromCaps creates a SandboxState snapshot from a CapabilitySet.
func StateFromCaps(caps *CapabilitySet) (*SandboxState, error) {
	if caps == nil {
		return nil, staticError(ErrCodeInvalidArg, "nil capability set")
	}
	caps.mu.RLock()
	defer caps.mu.RUnlock()
	if caps.ptr == nil {
		return nil, staticError(ErrCodeInvalidArg, errCapSetClosed)
	}
	ptr := C.nono_sandbox_state_from_caps(caps.ptr)
	if ptr == nil {
		// nono_sandbox_state_from_caps only returns nil when caps is nil (guarded
		// above) or on internal allocation failure.
		return nil, staticError(ErrCodeUnknown, "failed to create sandbox state")
	}
	ss := &SandboxState{ptr: ptr}
	runtime.SetFinalizer(ss, (*SandboxState).Close)
	return ss, nil
}

// StateFromJSON deserializes a SandboxState from a JSON string.
// On failure the returned error contains the C library's description.
// The error code is always ErrCodeUnknown because the C function does not
// return a numeric code; use errors.Is(err, nono.ErrUnknown) to match the
// error, and err.(*nono.Error).Message() for the human-readable reason.
//
// The parameter type is string (not []byte) to match the C FFI signature,
// which accepts a null-terminated C string.
func StateFromJSON(data string) (*SandboxState, error) {
	if err := checkNUL(data); err != nil {
		return nil, err
	}
	// TODO: request a NonoErrorCode return from nono_sandbox_state_from_json
	// upstream so callers can distinguish parse errors from I/O failures.
	// LockOSThread ensures nono_last_error() in mapError reads the error set
	// by nono_sandbox_state_from_json on the same OS thread.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	cjson := C.CString(data)
	defer C.free(unsafe.Pointer(cjson))
	ptr := C.nono_sandbox_state_from_json(cjson)
	if ptr == nil {
		// The C function does not return an error code; ErrUnknown is used
		// so callers are not misled into thinking the failure is always a
		// parse error. The human-readable message from nono_last_error()
		// provides the actual reason.
		return nil, mapError(C.int(C.NONO_ERROR_CODE_ERR_UNKNOWN))
	}
	// On success the C library does not set thread-local error state;
	// mapError (which calls nono_clear_error) is only invoked on failure.
	// Any prior error on this thread was already cleared by the previous
	// mapError call, so no explicit nono_clear_error is needed here.
	ss := &SandboxState{ptr: ptr}
	runtime.SetFinalizer(ss, (*SandboxState).Close)
	return ss, nil
}

// Close frees the underlying C sandbox state immediately.
// It is safe to call Close multiple times; subsequent calls are no-ops.
// Close always returns nil; the error return satisfies [io.Closer].
func (ss *SandboxState) Close() error {
	ss.mu.Lock() // exclusive: modifies ptr
	defer ss.mu.Unlock()
	if ss.ptr != nil {
		C.nono_sandbox_state_free(ss.ptr)
		ss.ptr = nil
		runtime.SetFinalizer(ss, nil)
	}
	return nil
}

// JSON serializes the state to a JSON string.
// The return type is string (not []byte) to match the C FFI, which returns a
// null-terminated string that is copied into Go memory by goString.
// On failure, error codes are:
//   - [ErrInvalidArg] if the state has been closed.
//   - [ErrUnknown] for C-originated failures (the C function returns only a
//     pointer; use err.(*Error).Message() for the human-readable reason).
func (ss *SandboxState) JSON() (string, error) {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	if ss.ptr == nil {
		return "", staticError(ErrCodeInvalidArg, errSandboxStateClosed)
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	s := C.nono_sandbox_state_to_json(ss.ptr)
	if s == nil {
		// nono_sandbox_state_to_json returns only a pointer (no error code);
		// use ErrUnknown and rely on nono_last_error for the message.
		return "", mapError(C.int(C.NONO_ERROR_CODE_ERR_UNKNOWN))
	}
	// On success the C library does not set thread-local error state;
	// nono_clear_error is not needed (see StateFromJSON for rationale).
	return goString(s), nil
}

// Caps converts the state back to a CapabilitySet.
// On failure, error codes are:
//   - [ErrInvalidArg] if the state has been closed.
//   - [ErrUnknown] for C-originated failures (the C function returns only a
//     pointer; use err.(*Error).Message() for the human-readable reason).
func (ss *SandboxState) Caps() (*CapabilitySet, error) {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	if ss.ptr == nil {
		return nil, staticError(ErrCodeInvalidArg, errSandboxStateClosed)
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	ptr := C.nono_sandbox_state_to_caps(ss.ptr)
	if ptr == nil {
		// nono_sandbox_state_to_caps returns only a pointer (no error code);
		// use ErrUnknown and rely on nono_last_error for the message.
		return nil, mapError(C.int(C.NONO_ERROR_CODE_ERR_UNKNOWN))
	}
	// On success the C library does not set thread-local error state;
	// nono_clear_error is not needed (see StateFromJSON for rationale).
	cs := &CapabilitySet{ptr: ptr}
	runtime.SetFinalizer(cs, (*CapabilitySet).Close)
	return cs, nil
}
