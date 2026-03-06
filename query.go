package nono

// #include "internal/clib/nono.h"
import "C"
import (
	"runtime"
	"sync"
	"unsafe"
)

// errQueryContextClosed is the error message for operations on a closed QueryContext.
const errQueryContextClosed = "query context is closed"

// QueryContext evaluates permission queries against a snapshot of a CapabilitySet.
// The capability set is cloned internally when creating a QueryContext, so
// subsequent modifications to the source CapabilitySet do not affect it.
//
// QueryContext is safe for concurrent use. Multiple goroutines may call
// QueryPath and QueryNetwork simultaneously; only Close requires exclusive access.
type QueryContext struct {
	mu  sync.RWMutex
	ptr *C.struct_NonoQueryContext
}

// NewQueryContext creates a QueryContext from a CapabilitySet.
func NewQueryContext(caps *CapabilitySet) (*QueryContext, error) {
	if caps == nil {
		return nil, staticError(ErrCodeInvalidArg, "nil capability set")
	}
	// RLock is sufficient: nono_query_context_new clones the capability set
	// internally and does not mutate the source.
	caps.mu.RLock()
	defer caps.mu.RUnlock()
	if caps.ptr == nil {
		return nil, staticError(ErrCodeInvalidArg, errCapSetClosed)
	}
	ptr := C.nono_query_context_new(caps.ptr)
	if ptr == nil {
		// nono_query_context_new only returns nil when caps is nil, which we
		// already guard above, so this path indicates an unexpected condition.
		return nil, staticError(ErrCodeUnknown, "failed to create query context")
	}
	qc := &QueryContext{ptr: ptr}
	runtime.SetFinalizer(qc, (*QueryContext).Close)
	return qc, nil
}

// Close frees the underlying C query context immediately.
// It is safe to call Close multiple times; subsequent calls are no-ops.
// Close always returns nil; the error return satisfies [io.Closer].
func (qc *QueryContext) Close() error {
	qc.mu.Lock()
	defer qc.mu.Unlock()
	if qc.ptr != nil {
		C.nono_query_context_free(qc.ptr)
		qc.ptr = nil
		runtime.SetFinalizer(qc, nil)
	}
	return nil
}

// QueryPath checks whether path access with the given mode is permitted.
func (qc *QueryContext) QueryPath(path string, mode AccessMode) (QueryResult, error) {
	if err := checkNUL(path); err != nil {
		return QueryResult{}, err
	}
	qc.mu.RLock()
	defer qc.mu.RUnlock()
	if qc.ptr == nil {
		return QueryResult{}, staticError(ErrCodeInvalidArg, errQueryContextClosed)
	}
	// LockOSThread pins each goroutine to its own OS thread so that
	// nono_last_error() reads the error set by this goroutine's C call,
	// not one from a concurrent caller. Multiple goroutines can safely
	// hold the RLock simultaneously because each is pinned to a distinct
	// OS thread with independent thread-local error state.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))

	var cresult C.NonoQueryResult
	code := C.nono_query_context_query_path(qc.ptr, cpath, C.uint32_t(mode), &cresult)
	if !isOK(C.int(code)) {
		return QueryResult{}, mapError(C.int(code))
	}
	return extractQueryResult(&cresult), nil
}

// QueryNetwork checks whether outbound network access is permitted.
func (qc *QueryContext) QueryNetwork() (QueryResult, error) {
	qc.mu.RLock()
	defer qc.mu.RUnlock()
	if qc.ptr == nil {
		return QueryResult{}, staticError(ErrCodeInvalidArg, errQueryContextClosed)
	}
	// See QueryPath for the LockOSThread + RLock concurrent-caller rationale.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	var cresult C.NonoQueryResult
	code := C.nono_query_context_query_network(qc.ptr, &cresult)
	if !isOK(C.int(code)) {
		return QueryResult{}, mapError(C.int(code))
	}
	return extractQueryResult(&cresult), nil
}

// extractQueryResult converts a C NonoQueryResult to a Go QueryResult.
// All four string fields of NonoQueryResult are heap-allocated and
// caller-owned per the C header; goString (which calls nono_string_free)
// is safe to call on each of them unconditionally.
func extractQueryResult(r *C.NonoQueryResult) QueryResult {
	// Free all caller-owned strings regardless of which fields are populated.
	// goString is nil-safe and calls nono_string_free before returning.
	// C field names do not match Go field names; the mapping is:
	//   r.granted_path -> GrantedPath   (path of the granting capability)
	//   r.access       -> GrantedAccess (access mode string of the granting capability)
	//   r.granted      -> ActualAccess  (actually-granted mode in insufficient-access cases)
	//   r.requested    -> RequestedAccess (requested mode in insufficient-access cases)
	grantedPath := goString(r.granted_path)
	grantedAccess := goString(r.access)
	actualAccess := goString(r.granted)
	requestedAccess := goString(r.requested)
	return QueryResult{
		Status:          QueryStatus(r.status),
		Reason:          QueryReason(r.reason),
		GrantedPath:     grantedPath,
		GrantedAccess:   grantedAccess,
		ActualAccess:    actualAccess,
		RequestedAccess: requestedAccess,
	}
}
