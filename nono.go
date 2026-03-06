// Package nono provides Go bindings for the nono capability-based security sandbox.
//
// nono uses Linux Landlock and macOS Seatbelt to restrict what resources a
// process can access. The three primary types are:
//
//   - [CapabilitySet] — builds the set of allowed filesystem paths and network modes.
//   - [QueryContext] — checks whether a given path or network operation would be permitted.
//   - [SandboxState] — serialises and deserialises a CapabilitySet as JSON.
//
// Typical usage:
//
//	caps := nono.New()
//	defer caps.Close()
//	if err := caps.AllowPath("/data", nono.AccessRead); err != nil {
//	    log.Fatal(err)
//	}
//	if err := caps.SetNetworkMode(nono.NetworkBlocked); err != nil {
//	    log.Fatal(err)
//	}
//	if err := nono.Apply(caps); err != nil { // irreversible
//	    log.Fatal(err)
//	}
package nono

// #include "internal/clib/nono.h"
import "C"
import (
	"runtime"
	"strings"
)

// AccessMode controls the type of filesystem access granted.
// uint32 matches the C parameter type (uint32_t) used in the FFI functions.
type AccessMode uint32

const (
	AccessRead      AccessMode = C.NONO_ACCESS_MODE_READ
	AccessWrite     AccessMode = C.NONO_ACCESS_MODE_WRITE
	AccessReadWrite AccessMode = C.NONO_ACCESS_MODE_READ_WRITE
)

// NetworkMode controls outbound network access.
// uint32 matches the C parameter type (uint32_t) used in the FFI functions.
type NetworkMode uint32

const (
	NetworkBlocked   NetworkMode = C.NONO_NETWORK_MODE_BLOCKED
	NetworkAllowAll  NetworkMode = C.NONO_NETWORK_MODE_ALLOW_ALL
	NetworkProxyOnly NetworkMode = C.NONO_NETWORK_MODE_PROXY_ONLY
)

// CapabilitySourceTag identifies where a capability came from.
// int reflects the default C enum signedness; values are always non-negative.
type CapabilitySourceTag int

const (
	SourceUser    CapabilitySourceTag = C.NONO_CAPABILITY_SOURCE_TAG_USER
	SourceGroup   CapabilitySourceTag = C.NONO_CAPABILITY_SOURCE_TAG_GROUP
	SourceSystem  CapabilitySourceTag = C.NONO_CAPABILITY_SOURCE_TAG_SYSTEM
	SourceProfile CapabilitySourceTag = C.NONO_CAPABILITY_SOURCE_TAG_PROFILE
)

// QueryStatus indicates whether a queried operation is allowed or denied.
type QueryStatus int

const (
	QueryAllowed QueryStatus = C.NONO_QUERY_STATUS_ALLOWED
	QueryDenied  QueryStatus = C.NONO_QUERY_STATUS_DENIED
)

// QueryReason explains why a query was allowed or denied.
type QueryReason int

const (
	ReasonGrantedPath        QueryReason = C.NONO_QUERY_REASON_GRANTED_PATH
	ReasonNetworkAllowed     QueryReason = C.NONO_QUERY_REASON_NETWORK_ALLOWED
	ReasonPathNotGranted     QueryReason = C.NONO_QUERY_REASON_PATH_NOT_GRANTED
	ReasonInsufficientAccess QueryReason = C.NONO_QUERY_REASON_INSUFFICIENT_ACCESS
	ReasonNetworkBlocked     QueryReason = C.NONO_QUERY_REASON_NETWORK_BLOCKED
)

// FSCapability describes a single filesystem capability in a [CapabilitySet].
type FSCapability struct {
	OriginalPath string
	ResolvedPath string
	Access       AccessMode
	IsFile       bool
	Source       CapabilitySourceTag
	GroupName    string // non-empty only when Source == SourceGroup
}

// QueryResult holds the outcome of a permission query.
// The C NonoQueryResult struct uses abbreviated field names that differ from
// the Go field names; see extractQueryResult in query.go for the mapping.
type QueryResult struct {
	// Status is whether the operation is allowed or denied.
	Status QueryStatus
	// Reason is the specific reason for the status.
	Reason QueryReason
	// GrantedPath is the capability path that grants access.
	// Non-empty only when Reason == ReasonGrantedPath.
	GrantedPath string
	// GrantedAccess is the access mode string of the granting capability.
	// Non-empty only when Reason == ReasonGrantedPath.
	// Typed as string because the C library returns a human-readable mode name.
	GrantedAccess string
	// ActualAccess is the access mode that was actually granted.
	// Non-empty only when Reason == ReasonInsufficientAccess.
	ActualAccess string
	// RequestedAccess is the access mode that was requested but not granted.
	// Non-empty only when Reason == ReasonInsufficientAccess.
	RequestedAccess string
}

// PlatformInfo describes sandboxing support on the current platform.
type PlatformInfo struct {
	Platform string
	Details  string
}

// goString takes ownership of a C string returned by the nono FFI, copies
// it to a Go string, and frees the C memory. Safe to call with nil.
func goString(s *C.char) string {
	if s == nil {
		return ""
	}
	defer C.nono_string_free(s)
	return C.GoString(s)
}

// isOK reports whether a C error code indicates success.
func isOK(code C.int) bool { return code == 0 }

// checkNUL returns ErrInvalidArg if s contains a NUL byte. C.CString silently
// truncates at the first NUL byte; for a security library this must be an error
// rather than silent data loss.
func checkNUL(s string) error {
	if strings.ContainsRune(s, 0) {
		return staticError(ErrCodeInvalidArg, "string contains NUL byte")
	}
	return nil
}

// lockOSThread policy: methods that call a C FFI function AND subsequently
// call mapError (which calls nono_last_error) must bracket the C call with
// runtime.LockOSThread/UnlockOSThread. This prevents the goroutine from being
// rescheduled to a different OS thread between the FFI call that sets the
// thread-local error and the nono_last_error call that reads it.
//
// Read-only methods that return values directly from C (NetworkMode, ProxyPort,
// IsNetworkBlocked, Summary, PathCovered, FSCapabilities) do NOT call
// nono_last_error and therefore do NOT need LockOSThread.

// Apply activates the sandbox for the current process using the given
// capabilities. This is irreversible — once applied, the process and all
// children can only access resources allowed by caps.
//
// On success caps is closed: the capability set is consumed by the kernel and
// any subsequent mutations would have no effect on the active sandbox.
func Apply(caps *CapabilitySet) error {
	if caps == nil {
		return staticError(ErrCodeInvalidArg, "nil capability set")
	}
	caps.mu.Lock()
	defer caps.mu.Unlock()
	if caps.ptr == nil {
		return staticError(ErrCodeInvalidArg, errCapSetClosed)
	}
	// LockOSThread ensures nono_last_error() in mapError reads the thread-local
	// error set by nono_sandbox_apply on the same OS thread.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	code := C.nono_sandbox_apply(caps.ptr)
	if !isOK(C.int(code)) {
		return mapError(C.int(code))
	}
	// Free the capability set: the sandbox is now live and further mutations
	// would have no effect. The C signature takes `const *caps`, confirming
	// that nono_sandbox_apply borrows without taking ownership, so we must
	// free explicitly. Nilling the pointer prevents further Go-side mutations
	// and clears the finalizer to avoid a double-free via GC.
	C.nono_capability_set_free(caps.ptr)
	caps.ptr = nil
	runtime.SetFinalizer(caps, nil)
	return nil
}

// IsSupported reports whether sandboxing is available on the current platform.
func IsSupported() bool {
	return bool(C.nono_sandbox_is_supported())
}

// SupportInfo returns detailed platform sandboxing support information.
// When only a boolean is needed, prefer the cheaper [IsSupported] function,
// which avoids the two C-to-Go string copies (platform, details) that
// SupportInfo performs.
func SupportInfo() PlatformInfo {
	info := C.nono_sandbox_support_info()
	// The C header documents info.platform and info.details as caller-owned
	// heap strings that must be freed with nono_string_free. goString handles
	// that ownership transfer correctly.
	return PlatformInfo{
		Platform: goString(info.platform),
		Details:  goString(info.details),
	}
}

// Version returns the nono library version string.
func Version() string {
	return goString(C.nono_version())
}
