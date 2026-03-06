package nono

// #include "internal/clib/nono.h"
import "C"
import (
	"runtime"
	"sync"
	"unsafe"
)

// errCapSetClosed is the error message for operations on a closed CapabilitySet.
const errCapSetClosed = "capability set is closed"

// CapabilitySet holds a set of permissions to be applied to the sandbox.
// Use [New] to create one, and [Apply] to activate it.
// Call [CapabilitySet.Close] when done if you want immediate cleanup;
// otherwise the finalizer will release memory when GC runs.
//
// CapabilitySet is safe for concurrent use. Close may be called concurrently
// with other methods; subsequent calls on a closed set return an error.
// Read-only methods (NetworkMode, ProxyPort, IsNetworkBlocked, Summary,
// PathCovered, FSCapabilities) may execute concurrently with each other.
type CapabilitySet struct {
	mu  sync.RWMutex
	ptr *C.struct_NonoCapabilitySet
}

// New creates a new empty CapabilitySet. It panics on allocation failure
// (equivalent to the runtime panicking on out-of-memory).
func New() *CapabilitySet {
	ptr := C.nono_capability_set_new()
	if ptr == nil {
		// The C library documents this as never returning nil;
		// if it does, treat it like an OOM condition.
		panic("nono: nono_capability_set_new returned nil (out of memory?)")
	}
	cs := &CapabilitySet{ptr: ptr}
	runtime.SetFinalizer(cs, (*CapabilitySet).Close)
	return cs
}

// Close frees the underlying C capability set immediately.
// It is safe to call Close multiple times; subsequent calls are no-ops.
// Close always returns nil; the error return satisfies [io.Closer].
func (cs *CapabilitySet) Close() error {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if cs.ptr != nil {
		C.nono_capability_set_free(cs.ptr)
		cs.ptr = nil
		// Clear the finalizer so the GC does not attempt a double-free.
		// When Close is called as a GC finalizer this is a safe no-op;
		// when called explicitly it removes the pending finalizer.
		runtime.SetFinalizer(cs, nil)
	}
	return nil
}

// AllowPath grants directory access at the given path with the specified mode.
func (cs *CapabilitySet) AllowPath(path string, mode AccessMode) error {
	if err := checkNUL(path); err != nil {
		return err
	}
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if cs.ptr == nil {
		return staticError(ErrCodeInvalidArg, errCapSetClosed)
	}
	// LockOSThread pins the goroutine so that nono_last_error() in mapError
	// reads the thread-local error set by the FFI call on the same OS thread.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))
	code := C.nono_capability_set_allow_path(cs.ptr, cpath, C.uint32_t(mode))
	if !isOK(C.int(code)) {
		return mapError(C.int(code))
	}
	return nil
}

// AllowFile grants single-file access at the given path with the specified mode.
func (cs *CapabilitySet) AllowFile(path string, mode AccessMode) error {
	if err := checkNUL(path); err != nil {
		return err
	}
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if cs.ptr == nil {
		return staticError(ErrCodeInvalidArg, errCapSetClosed)
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))
	code := C.nono_capability_set_allow_file(cs.ptr, cpath, C.uint32_t(mode))
	if !isOK(C.int(code)) {
		return mapError(C.int(code))
	}
	return nil
}

// SetNetworkBlocked sets whether outbound network access is blocked.
//
// Deprecated: use [SetNetworkMode] instead, which provides access to all
// network modes. SetNetworkBlocked(true) is equivalent to
// SetNetworkMode(NetworkBlocked); SetNetworkBlocked(false) is equivalent to
// SetNetworkMode(NetworkAllowAll). The last call between SetNetworkBlocked
// and SetNetworkMode wins.
func (cs *CapabilitySet) SetNetworkBlocked(blocked bool) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if cs.ptr == nil {
		return staticError(ErrCodeInvalidArg, errCapSetClosed)
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	code := C.nono_capability_set_set_network_blocked(cs.ptr, C.bool(blocked))
	if !isOK(C.int(code)) {
		return mapError(C.int(code))
	}
	return nil
}

// SetNetworkMode sets the network mode. Use [SetProxyPort] to configure
// the port when using [NetworkProxyOnly].
// SetNetworkMode and the deprecated [SetNetworkBlocked] both control the
// same underlying network mode field; the last call wins.
func (cs *CapabilitySet) SetNetworkMode(mode NetworkMode) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if cs.ptr == nil {
		return staticError(ErrCodeInvalidArg, errCapSetClosed)
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	code := C.nono_capability_set_set_network_mode(cs.ptr, C.uint32_t(mode))
	if !isOK(C.int(code)) {
		return mapError(C.int(code))
	}
	return nil
}

// NetworkMode returns the current network mode.
// Returns [NetworkBlocked] if the set is closed.
func (cs *CapabilitySet) NetworkMode() NetworkMode {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	if cs.ptr == nil {
		return NetworkBlocked
	}
	return NetworkMode(C.nono_capability_set_network_mode(cs.ptr))
}

// SetProxyPort sets the proxy port for [NetworkProxyOnly] mode.
func (cs *CapabilitySet) SetProxyPort(port uint16) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if cs.ptr == nil {
		return staticError(ErrCodeInvalidArg, errCapSetClosed)
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	code := C.nono_capability_set_set_proxy_port(cs.ptr, C.uint16_t(port))
	if !isOK(C.int(code)) {
		return mapError(C.int(code))
	}
	return nil
}

// ProxyPort returns the proxy port (meaningful only in [NetworkProxyOnly] mode).
// Returns 0 if the set is closed.
func (cs *CapabilitySet) ProxyPort() uint16 {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	if cs.ptr == nil {
		return 0
	}
	return uint16(C.nono_capability_set_proxy_port(cs.ptr))
}

// AllowCommand adds a command to the allow list (overrides block lists).
func (cs *CapabilitySet) AllowCommand(cmd string) error {
	if err := checkNUL(cmd); err != nil {
		return err
	}
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if cs.ptr == nil {
		return staticError(ErrCodeInvalidArg, errCapSetClosed)
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	ccmd := C.CString(cmd)
	defer C.free(unsafe.Pointer(ccmd))
	code := C.nono_capability_set_allow_command(cs.ptr, ccmd)
	if !isOK(C.int(code)) {
		return mapError(C.int(code))
	}
	return nil
}

// BlockCommand adds a command to the block list.
func (cs *CapabilitySet) BlockCommand(cmd string) error {
	if err := checkNUL(cmd); err != nil {
		return err
	}
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if cs.ptr == nil {
		return staticError(ErrCodeInvalidArg, errCapSetClosed)
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	ccmd := C.CString(cmd)
	defer C.free(unsafe.Pointer(ccmd))
	code := C.nono_capability_set_block_command(cs.ptr, ccmd)
	if !isOK(C.int(code)) {
		return mapError(C.int(code))
	}
	return nil
}

// AddPlatformRule adds a raw platform-specific sandbox rule.
// On macOS this is a Seatbelt S-expression; on Linux it is ignored.
func (cs *CapabilitySet) AddPlatformRule(rule string) error {
	if err := checkNUL(rule); err != nil {
		return err
	}
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if cs.ptr == nil {
		return staticError(ErrCodeInvalidArg, errCapSetClosed)
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	crule := C.CString(rule)
	defer C.free(unsafe.Pointer(crule))
	code := C.nono_capability_set_add_platform_rule(cs.ptr, crule)
	if !isOK(C.int(code)) {
		return mapError(C.int(code))
	}
	return nil
}

// Deduplicate removes redundant filesystem capabilities, keeping the
// highest access level for overlapping paths.
// Returns an error only if the set is closed; for open sets, the operation
// always succeeds (the underlying C function returns void and cannot fail).
func (cs *CapabilitySet) Deduplicate() error {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if cs.ptr == nil {
		return staticError(ErrCodeInvalidArg, errCapSetClosed)
	}
	C.nono_capability_set_deduplicate(cs.ptr)
	return nil
}

// PathCovered reports whether path is covered by an existing directory capability.
// Returns (false, ErrInvalidArg) if path contains a NUL byte (which would be
// silently truncated by C.CString, making the check meaningless).
// Returns (false, nil) if the set is closed.
func (cs *CapabilitySet) PathCovered(path string) (bool, error) {
	if err := checkNUL(path); err != nil {
		return false, err
	}
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	if cs.ptr == nil {
		return false, nil
	}
	// No LockOSThread needed: nono_capability_set_path_covered returns a bool
	// directly and does not set thread-local error state.
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))
	return bool(C.nono_capability_set_path_covered(cs.ptr, cpath)), nil
}

// IsNetworkBlocked reports whether outbound network access is blocked.
// Returns false if the set is closed.
//
// Note: although [SetNetworkBlocked] is deprecated in favour of [SetNetworkMode],
// this read-only accessor is retained because NetworkMode() == NetworkBlocked
// is less readable in simple boolean contexts.
func (cs *CapabilitySet) IsNetworkBlocked() bool {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	if cs.ptr == nil {
		return false
	}
	return bool(C.nono_capability_set_is_network_blocked(cs.ptr))
}

// Summary returns a plain-text summary of the capability set.
// Returns an empty string if the set is closed.
func (cs *CapabilitySet) Summary() string {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	if cs.ptr == nil {
		return ""
	}
	return goString(C.nono_capability_set_summary(cs.ptr))
}

// FSCapabilities returns all filesystem capabilities in the set.
// Returns nil if the set is closed. Entries whose access mode is reported
// as invalid by the C library (NONO_ACCESS_MODE_INVALID) are skipped.
//
// Warning: this method holds the internal read lock across O(N × fields)
// sequential CGo calls — one per field per entry — because the C API has no
// bulk accessor. All write operations (AllowPath, AllowFile, Close, etc.) are
// blocked for the full duration of the iteration. On large capability sets this
// may cause write starvation; call [CapabilitySet.Deduplicate] beforehand to
// minimize N.
//
// TODO(perf): replace with a single CGo call when a bulk C accessor is available upstream.
func (cs *CapabilitySet) FSCapabilities() []FSCapability {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	if cs.ptr == nil {
		return nil
	}
	count := int(C.nono_capability_set_fs_count(cs.ptr))
	caps := make([]FSCapability, 0, count)
	for i := 0; i < count; i++ {
		idx := C.uintptr_t(i)
		access := C.nono_capability_set_fs_access(cs.ptr, idx)
		if access == C.NONO_ACCESS_MODE_INVALID {
			// Index is within the bounds returned by fs_count moments ago
			// and the mutex prevents concurrent modifications, so this
			// path is unreachable in practice. Skip defensively.
			continue
		}
		caps = append(caps, FSCapability{
			OriginalPath: goString(C.nono_capability_set_fs_original(cs.ptr, idx)),
			ResolvedPath: goString(C.nono_capability_set_fs_resolved(cs.ptr, idx)),
			Access:       AccessMode(access),
			IsFile:       bool(C.nono_capability_set_fs_is_file(cs.ptr, idx)),
			Source:       CapabilitySourceTag(C.nono_capability_set_fs_source_tag(cs.ptr, idx)),
			GroupName:    goString(C.nono_capability_set_fs_source_group_name(cs.ptr, idx)),
		})
	}
	return caps
}
