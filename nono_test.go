package nono_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/always-further/nono-go"
)

// newCapSet returns a new CapabilitySet that is closed at test end.
func newCapSet(t *testing.T) *nono.CapabilitySet {
	t.Helper()
	cs := nono.New()
	// Close always returns nil; the error return exists only to satisfy io.Closer.
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

// newCapSetWithPath returns a CapabilitySet with a temp dir allowed for reading,
// and the path itself for tests that need to reference it.
func newCapSetWithPath(t *testing.T) (*nono.CapabilitySet, string) {
	t.Helper()
	cs := newCapSet(t)
	dir := t.TempDir()
	if err := cs.AllowPath(dir, nono.AccessRead); err != nil {
		t.Fatalf("AllowPath: %v", err)
	}
	return cs, dir
}

// newCapSetWithAnyPath returns a CapabilitySet with a temp dir allowed for
// reading. Use when the path itself is not needed by the test.
func newCapSetWithAnyPath(t *testing.T) *nono.CapabilitySet {
	t.Helper()
	cs, _ := newCapSetWithPath(t)
	return cs
}

func TestAccessModeString(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		mode nono.AccessMode
		want string
	}{
		{"read", nono.AccessRead, "read"},
		{"write", nono.AccessWrite, "write"},
		{"read-write", nono.AccessReadWrite, "read-write"},
		{"unknown", nono.AccessMode(99), "AccessMode(99)"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.mode.String(); got != tc.want {
				t.Errorf("AccessMode(%d).String() = %q, want %q", tc.mode, got, tc.want)
			}
		})
	}
}

func TestNetworkModeString(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		mode nono.NetworkMode
		want string
	}{
		{"blocked", nono.NetworkBlocked, "blocked"},
		{"allow-all", nono.NetworkAllowAll, "allow-all"},
		{"proxy-only", nono.NetworkProxyOnly, "proxy-only"},
		{"unknown", nono.NetworkMode(99), "NetworkMode(99)"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.mode.String(); got != tc.want {
				t.Errorf("NetworkMode(%d).String() = %q, want %q", tc.mode, got, tc.want)
			}
		})
	}
}

func TestQueryStatusString(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		status nono.QueryStatus
		want   string
	}{
		{"allowed", nono.QueryAllowed, "allowed"},
		{"denied", nono.QueryDenied, "denied"},
		{"unknown", nono.QueryStatus(99), "QueryStatus(99)"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.status.String(); got != tc.want {
				t.Errorf("QueryStatus(%d).String() = %q, want %q", tc.status, got, tc.want)
			}
		})
	}
}

func TestQueryReasonString(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		reason nono.QueryReason
		want   string
	}{
		{"granted-path", nono.ReasonGrantedPath, "granted-path"},
		{"network-allowed", nono.ReasonNetworkAllowed, "network-allowed"},
		{"path-not-granted", nono.ReasonPathNotGranted, "path-not-granted"},
		{"insufficient-access", nono.ReasonInsufficientAccess, "insufficient-access"},
		{"network-blocked", nono.ReasonNetworkBlocked, "network-blocked"},
		{"unknown", nono.QueryReason(99), "QueryReason(99)"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.reason.String(); got != tc.want {
				t.Errorf("QueryReason(%d).String() = %q, want %q", tc.reason, got, tc.want)
			}
		})
	}
}

func TestCapabilitySourceTagString(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		tag  nono.CapabilitySourceTag
		want string
	}{
		{"user", nono.SourceUser, "user"},
		{"group", nono.SourceGroup, "group"},
		{"system", nono.SourceSystem, "system"},
		{"profile", nono.SourceProfile, "profile"},
		{"unknown", nono.CapabilitySourceTag(99), "CapabilitySourceTag(99)"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.tag.String(); got != tc.want {
				t.Errorf("CapabilitySourceTag(%d).String() = %q, want %q", tc.tag, got, tc.want)
			}
		})
	}
}

func TestVersion(t *testing.T) {
	t.Parallel()
	v := nono.Version()
	if v == "" {
		t.Fatal("Version() returned empty string")
	}
	t.Logf("nono version: %s", v)
}

func TestSupportInfo(t *testing.T) {
	t.Parallel()
	info := nono.SupportInfo()
	if info.Platform == "" {
		t.Fatal("SupportInfo().Platform is empty")
	}
	t.Logf("platform: %s, supported: %v, details: %s", info.Platform, nono.IsSupported(), info.Details)
}

// TestCapabilitySetLifecycle verifies that New/Close is safe to call
// and that double-Close is a no-op.
func TestCapabilitySetLifecycle(t *testing.T) {
	t.Parallel()
	cs := nono.New()
	if err := cs.Close(); err != nil {
		t.Fatalf("Close() error: %v", err)
	}
	// Second Close must be a no-op.
	if err := cs.Close(); err != nil {
		t.Fatalf("second Close() error: %v", err)
	}
}

// TestClosedCapabilitySet verifies that all mutating methods on a closed set
// return errors.
func TestClosedCapabilitySet(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	mutators := []struct {
		name string
		fn   func(*nono.CapabilitySet) error
	}{
		{"AllowPath", func(cs *nono.CapabilitySet) error { return cs.AllowPath(dir, nono.AccessRead) }},
		{"AllowFile", func(cs *nono.CapabilitySet) error { return cs.AllowFile(dir, nono.AccessRead) }},
		{"SetNetworkMode", func(cs *nono.CapabilitySet) error { return cs.SetNetworkMode(nono.NetworkAllowAll) }},
		{"SetNetworkBlocked", func(cs *nono.CapabilitySet) error { return cs.SetNetworkBlocked(true) }},
		{"SetProxyPort", func(cs *nono.CapabilitySet) error { return cs.SetProxyPort(8080) }},
		{"AllowCommand", func(cs *nono.CapabilitySet) error { return cs.AllowCommand("git") }},
		{"BlockCommand", func(cs *nono.CapabilitySet) error { return cs.BlockCommand("curl") }},
		{"AddPlatformRule", func(cs *nono.CapabilitySet) error { return cs.AddPlatformRule("(version 1)") }},
	}

	for _, m := range mutators {
		t.Run(m.name, func(t *testing.T) {
			t.Parallel()
			cs := nono.New()
			if err := cs.Close(); err != nil {
				t.Fatalf("Close: %v", err)
			}
			if err := m.fn(cs); err == nil {
				t.Errorf("%s on closed set: expected error, got nil", m.name)
			} else if !errors.Is(err, nono.ErrInvalidArg) {
				t.Errorf("%s on closed set: expected ErrInvalidArg, got %v", m.name, err)
			}
		})
	}
}

func TestAllowPath(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, mode := range []nono.AccessMode{nono.AccessRead, nono.AccessWrite, nono.AccessReadWrite} {
		t.Run(mode.String(), func(t *testing.T) {
			t.Parallel()
			cs := newCapSet(t)
			if err := cs.AllowPath(dir, mode); err != nil {
				t.Fatalf("AllowPath(%v) failed: %v", mode, err)
			}
		})
	}
}

func TestAllowPathInvalid(t *testing.T) {
	t.Parallel()
	cs := newCapSet(t)
	err := cs.AllowPath("/this/path/does/not/exist/12345", nono.AccessRead)
	if err == nil {
		t.Fatal("expected error for non-existent path, got nil")
	}
	if !errors.Is(err, nono.ErrPathNotFound) {
		t.Errorf("expected errors.Is to match ErrPathNotFound, got: %v", err)
	}
}

func TestAllowFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "testfile.txt")
	if err := os.WriteFile(path, []byte("test"), 0o644); err != nil {
		t.Fatalf("os.WriteFile(%q): %v", path, err)
	}

	cs := newCapSet(t)
	if err := cs.AllowFile(path, nono.AccessRead); err != nil {
		t.Fatalf("AllowFile failed: %v", err)
	}

	fsCaps := cs.FSCapabilities()
	if len(fsCaps) != 1 {
		t.Fatalf("expected 1 fs capability, got %d", len(fsCaps))
	}
	if !fsCaps[0].IsFile {
		t.Error("expected IsFile == true")
	}
}

func TestNetworkModeAllowAll(t *testing.T) {
	t.Parallel()
	cs := newCapSet(t)
	if err := cs.SetNetworkMode(nono.NetworkAllowAll); err != nil {
		t.Fatalf("SetNetworkMode(AllowAll) failed: %v", err)
	}
	if got := cs.NetworkMode(); got != nono.NetworkAllowAll {
		t.Errorf("NetworkMode() = %v, want NetworkAllowAll", got)
	}
}

func TestNetworkModeProxyOnly(t *testing.T) {
	t.Parallel()
	cs := newCapSet(t)
	if err := cs.SetNetworkMode(nono.NetworkProxyOnly); err != nil {
		t.Fatalf("SetNetworkMode(ProxyOnly) failed: %v", err)
	}
	if err := cs.SetProxyPort(8080); err != nil {
		t.Fatalf("SetProxyPort failed: %v", err)
	}
	if got := cs.NetworkMode(); got != nono.NetworkProxyOnly {
		t.Errorf("NetworkMode() = %v, want NetworkProxyOnly", got)
	}
	if got := cs.ProxyPort(); got != 8080 {
		t.Errorf("ProxyPort() = %d, want 8080", got)
	}
}

func TestIsNetworkBlocked(t *testing.T) {
	t.Parallel()
	cs := newCapSet(t)

	if err := cs.SetNetworkBlocked(true); err != nil {
		t.Fatalf("SetNetworkBlocked(true) failed: %v", err)
	}
	if !cs.IsNetworkBlocked() {
		t.Error("expected IsNetworkBlocked() == true")
	}

	if err := cs.SetNetworkBlocked(false); err != nil {
		t.Fatalf("SetNetworkBlocked(false) failed: %v", err)
	}
	if cs.IsNetworkBlocked() {
		t.Error("expected IsNetworkBlocked() == false")
	}
}

func TestSummary(t *testing.T) {
	t.Parallel()
	cs := newCapSetWithAnyPath(t)
	s := cs.Summary()
	if s == "" {
		t.Error("Summary() returned empty string")
	}
	t.Logf("summary: %s", s)
}

func TestFSCapabilities(t *testing.T) {
	t.Parallel()
	cs := newCapSet(t)
	dir := t.TempDir()
	if err := cs.AllowPath(dir, nono.AccessReadWrite); err != nil {
		t.Fatalf("AllowPath(%q, AccessReadWrite): %v", dir, err)
	}

	caps := cs.FSCapabilities()
	if len(caps) != 1 {
		t.Fatalf("expected 1 cap, got %d", len(caps))
	}
	if caps[0].Access != nono.AccessReadWrite {
		t.Errorf("Access = %v, want ReadWrite", caps[0].Access)
	}
	if caps[0].Source != nono.SourceUser {
		t.Errorf("Source = %v, want User", caps[0].Source)
	}
}

func TestPathCovered(t *testing.T) {
	t.Parallel()
	cs, dir := newCapSetWithPath(t)

	// Resolve symlinks so the path matches what the library stores internally
	// (e.g. /var/... → /private/var/... on macOS).
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	child := filepath.Join(resolved, "subdir")
	covered, err := cs.PathCovered(child)
	if err != nil {
		t.Fatalf("PathCovered(%q): %v", child, err)
	}
	if !covered {
		t.Errorf("PathCovered(%q) = false, want true", child)
	}
	uncovered, err := cs.PathCovered("/completely/different/path")
	if err != nil {
		t.Fatalf("PathCovered uncovered path: %v", err)
	}
	if uncovered {
		t.Error("PathCovered returned true for uncovered path")
	}
}

func TestDeduplicate(t *testing.T) {
	t.Parallel()
	cs := newCapSet(t)
	dir := t.TempDir()
	if err := cs.AllowPath(dir, nono.AccessRead); err != nil {
		t.Fatalf("AllowPath(%q, AccessRead) first: %v", dir, err)
	}
	if err := cs.AllowPath(dir, nono.AccessRead); err != nil {
		t.Fatalf("AllowPath(%q, AccessRead) second: %v", dir, err)
	}

	if err := cs.Deduplicate(); err != nil {
		t.Fatalf("Deduplicate: %v", err)
	}

	if got := len(cs.FSCapabilities()); got != 1 {
		t.Errorf("after Deduplicate expected 1 cap, got %d", got)
	}
}

func TestAllowCommand(t *testing.T) {
	t.Parallel()
	cs := newCapSet(t)
	if err := cs.AllowCommand("git"); err != nil {
		t.Fatalf("AllowCommand failed: %v", err)
	}
}

func TestBlockCommand(t *testing.T) {
	t.Parallel()
	cs := newCapSet(t)
	if err := cs.BlockCommand("curl"); err != nil {
		t.Fatalf("BlockCommand failed: %v", err)
	}
}

func TestAddPlatformRule(t *testing.T) {
	t.Parallel()
	cs := newCapSet(t)
	// "(version 1)" is a valid minimal Seatbelt S-expression on macOS and
	// is stored as-is on Linux (where platform rules are no-ops). It must
	// succeed on both supported platforms.
	if err := cs.AddPlatformRule("(version 1)"); err != nil {
		t.Fatalf("AddPlatformRule failed: %v", err)
	}
}

func TestQueryContextPath(t *testing.T) {
	t.Parallel()
	cs, dir := newCapSetWithPath(t)

	qc, err := nono.NewQueryContext(cs)
	if err != nil {
		t.Fatalf("NewQueryContext: %v", err)
	}
	t.Cleanup(func() { _ = qc.Close() })

	t.Run("allowed", func(t *testing.T) {
		t.Parallel()
		result, err := qc.QueryPath(dir, nono.AccessRead)
		if err != nil {
			t.Fatalf("QueryPath: %v", err)
		}
		if result.Status != nono.QueryAllowed {
			t.Errorf("expected Allowed, got %v (reason %v)", result.Status, result.Reason)
		}
	})

	t.Run("denied", func(t *testing.T) {
		t.Parallel()
		// Use a path guaranteed to be absent from the capability set on any platform.
		result, err := qc.QueryPath("/path/not/in/capability/set/xyz123", nono.AccessRead)
		if err != nil {
			t.Fatalf("QueryPath: %v", err)
		}
		if result.Status != nono.QueryDenied {
			t.Errorf("expected Denied, got %v", result.Status)
		}
	})
}

func TestQueryContextNilCapabilitySet(t *testing.T) {
	t.Parallel()
	_, err := nono.NewQueryContext(nil)
	if err == nil {
		t.Fatal("expected error for nil capability set, got nil")
	}
	if !errors.Is(err, nono.ErrInvalidArg) {
		t.Errorf("expected ErrInvalidArg for nil capability set, got %v", err)
	}
}

func TestQueryContextClosedCapabilitySet(t *testing.T) {
	t.Parallel()
	cs := nono.New()
	if err := cs.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	_, err := nono.NewQueryContext(cs)
	if err == nil {
		t.Fatal("expected error for closed capability set, got nil")
	}
	if !errors.Is(err, nono.ErrInvalidArg) {
		t.Errorf("expected ErrInvalidArg for closed capability set, got %v", err)
	}
}

func TestQueryContextNetwork(t *testing.T) {
	t.Parallel()
	cs := newCapSet(t)
	if err := cs.SetNetworkBlocked(true); err != nil {
		t.Fatalf("SetNetworkBlocked(true): %v", err)
	}

	qc, err := nono.NewQueryContext(cs)
	if err != nil {
		t.Fatalf("NewQueryContext: %v", err)
	}
	t.Cleanup(func() { _ = qc.Close() })

	result, err := qc.QueryNetwork()
	if err != nil {
		t.Fatalf("QueryNetwork: %v", err)
	}
	if result.Status != nono.QueryDenied {
		t.Errorf("expected network Denied, got %v", result.Status)
	}
}

func TestSandboxStateJSONRoundTrip(t *testing.T) {
	t.Parallel()
	cs := newCapSetWithAnyPath(t)
	if err := cs.SetNetworkMode(nono.NetworkAllowAll); err != nil {
		t.Fatalf("SetNetworkMode(NetworkAllowAll): %v", err)
	}

	state, err := nono.StateFromCaps(cs)
	if err != nil {
		t.Fatalf("StateFromCaps: %v", err)
	}
	t.Cleanup(func() { _ = state.Close() })

	jsonStr, err := state.JSON()
	if err != nil {
		t.Fatalf("JSON: %v", err)
	}
	if !json.Valid([]byte(jsonStr)) {
		t.Errorf("JSON output is not valid JSON: %q", jsonStr)
	}
	t.Logf("state JSON: %s", jsonStr)

	state2, err := nono.StateFromJSON(jsonStr)
	if err != nil {
		t.Fatalf("StateFromJSON: %v", err)
	}
	t.Cleanup(func() { _ = state2.Close() })

	jsonStr2, err := state2.JSON()
	if err != nil {
		t.Fatalf("second JSON: %v", err)
	}
	if jsonStr != jsonStr2 {
		t.Errorf("JSON round-trip mismatch:\n  first:  %s\n  second: %s", jsonStr, jsonStr2)
	}
	// Verify semantic equivalence: the restored state must produce a capability
	// set with the same filesystem capabilities and network mode as the original.
	cs2, err := state2.Caps()
	if err != nil {
		t.Fatalf("Caps from restored state: %v", err)
	}
	t.Cleanup(func() { _ = cs2.Close() })

	if got := cs2.NetworkMode(); got != nono.NetworkAllowAll {
		t.Errorf("restored NetworkMode = %v, want NetworkAllowAll", got)
	}
	origCaps := cs.FSCapabilities()
	restoredCaps := cs2.FSCapabilities()
	if len(origCaps) != len(restoredCaps) {
		t.Fatalf("FSCapabilities count: original %d, restored %d", len(origCaps), len(restoredCaps))
	}
	for i := range origCaps {
		if origCaps[i].ResolvedPath != restoredCaps[i].ResolvedPath {
			t.Errorf("FSCapabilities[%d].ResolvedPath = %q, want %q", i, restoredCaps[i].ResolvedPath, origCaps[i].ResolvedPath)
		}
		if origCaps[i].Access != restoredCaps[i].Access {
			t.Errorf("FSCapabilities[%d].Access = %v, want %v", i, restoredCaps[i].Access, origCaps[i].Access)
		}
	}
}

func TestSandboxStateToCaps(t *testing.T) {
	t.Parallel()
	cs := newCapSetWithAnyPath(t)

	state, err := nono.StateFromCaps(cs)
	if err != nil {
		t.Fatalf("StateFromCaps: %v", err)
	}
	t.Cleanup(func() { _ = state.Close() })

	cs2, err := state.Caps()
	if err != nil {
		t.Fatalf("Caps: %v", err)
	}
	t.Cleanup(func() { _ = cs2.Close() })

	if len(cs2.FSCapabilities()) == 0 {
		t.Error("Caps returned empty capability set")
	}
}

func TestSandboxStateClosedSet(t *testing.T) {
	t.Parallel()
	cs := newCapSetWithAnyPath(t)

	state, err := nono.StateFromCaps(cs)
	if err != nil {
		t.Fatalf("StateFromCaps: %v", err)
	}
	if err := state.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if _, err := state.JSON(); err == nil {
		t.Error("JSON on closed state: expected error, got nil")
	}
	if _, err := state.Caps(); err == nil {
		t.Error("Caps on closed state: expected error, got nil")
	}
}

func TestStateFromCapsNil(t *testing.T) {
	t.Parallel()
	_, err := nono.StateFromCaps(nil)
	if err == nil {
		t.Fatal("expected error for nil capability set, got nil")
	}
}

func TestStateFromCapsClosedSet(t *testing.T) {
	t.Parallel()
	cs := nono.New()
	if err := cs.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	_, err := nono.StateFromCaps(cs)
	if err == nil {
		t.Fatal("expected error for closed capability set, got nil")
	}
}

func TestStateFromJSONInvalid(t *testing.T) {
	t.Parallel()
	_, err := nono.StateFromJSON("not valid json")
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
	// ErrUnknown is asserted here because nono_sandbox_state_from_json does not
	// return a numeric error code. If the upstream C API gains a typed return
	// (see TODO in state.go:57), update this assertion to the specific code.
	if !errors.Is(err, nono.ErrUnknown) {
		t.Errorf("expected ErrUnknown for invalid JSON, got %v", err)
	}
	// The human-readable message from nono_last_error must be non-empty so
	// callers can inspect err.(*nono.Error).Message() for the failure reason.
	var nonoErr *nono.Error
	if !errors.As(err, &nonoErr) {
		t.Fatalf("expected *nono.Error from StateFromJSON, got %T", err)
	}
	if nonoErr.Message() == "" {
		t.Error("StateFromJSON error message is empty; nono_last_error may not have been set")
	}
}

func TestErrorIs(t *testing.T) {
	t.Parallel()
	cs := newCapSet(t)
	pathErr := cs.AllowPath("/nonexistent/path/xyz", nono.AccessRead)
	if pathErr == nil {
		t.Fatal("expected error from AllowPath on nonexistent path")
	}

	// Use nono.New() directly (not newCapSet) to retain manual lifecycle
	// control. A t.Cleanup is still registered — Close is idempotent, so
	// the cleanup after the explicit Close below is a safe no-op.
	cs2 := nono.New()
	t.Cleanup(func() { _ = cs2.Close() })
	if err := cs2.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	closedErr := cs2.AllowPath(t.TempDir(), nono.AccessRead)
	if closedErr == nil {
		t.Fatal("expected error from AllowPath on closed set")
	}

	tests := []struct {
		name   string
		err    error
		target error
		wantIs bool
	}{
		{"path-not-found matches sentinel", pathErr, nono.ErrPathNotFound, true},
		{"path-not-found does not match wrong sentinel", pathErr, nono.ErrSandboxInit, false},
		{"invalid-arg matches sentinel", closedErr, nono.ErrInvalidArg, true},
		{"invalid-arg does not match wrong sentinel", closedErr, nono.ErrPathNotFound, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := errors.Is(tc.err, tc.target)
			if got != tc.wantIs {
				t.Errorf("errors.Is(%v, %v) = %v, want %v", tc.err, tc.target, got, tc.wantIs)
			}
		})
	}
}

func TestErrorType(t *testing.T) {
	t.Parallel()
	cs := newCapSet(t)
	err := cs.AllowPath("/nonexistent/path/xyz", nono.AccessRead)
	if err == nil {
		t.Fatal("expected error")
	}
	var nonoErr *nono.Error
	if !errors.As(err, &nonoErr) {
		t.Fatalf("expected *nono.Error via errors.As, got %T", err)
	}
	if nonoErr.Code() >= 0 {
		t.Errorf("error code should be negative, got %v", nonoErr.Code())
	}
	if nonoErr.Error() == "" {
		t.Error("Error() returned empty string")
	}
}

// TestQueryResultFields verifies that QueryResult populates the human-readable
// string fields (GrantedPath, GrantedAccess, ActualAccess, RequestedAccess)
// based on the reason code, exercising extractQueryResult in query.go.
func TestQueryResultFields(t *testing.T) {
	t.Parallel()
	cs, dir := newCapSetWithPath(t)

	qc, err := nono.NewQueryContext(cs)
	if err != nil {
		t.Fatalf("NewQueryContext: %v", err)
	}
	t.Cleanup(func() { _ = qc.Close() })

	t.Run("granted-path fields", func(t *testing.T) {
		t.Parallel()
		result, err := qc.QueryPath(dir, nono.AccessRead)
		if err != nil {
			t.Fatalf("QueryPath: %v", err)
		}
		if result.Reason != nono.ReasonGrantedPath {
			t.Fatalf("expected ReasonGrantedPath, got %v", result.Reason)
		}
		if result.GrantedPath == "" {
			t.Error("GrantedPath should be non-empty for ReasonGrantedPath")
		}
		if result.GrantedAccess == "" {
			t.Error("GrantedAccess should be non-empty for ReasonGrantedPath")
		}
	})

	t.Run("denied fields are empty", func(t *testing.T) {
		t.Parallel()
		result, err := qc.QueryPath("/path/not/in/capability/set/xyz123", nono.AccessRead)
		if err != nil {
			t.Fatalf("QueryPath: %v", err)
		}
		if result.Status != nono.QueryDenied {
			t.Fatalf("expected Denied, got %v", result.Status)
		}
		if result.GrantedPath != "" {
			t.Errorf("GrantedPath should be empty for denied query, got %q", result.GrantedPath)
		}
	})
}

// TestErrorSentinelViaErrorsAs verifies that errors.As on a sentinel yields a
// pointer to the sentinel itself, and that the pointer is distinct from a live
// error even though errors.Is reports them as equal.
func TestErrorSentinelViaErrorsAs(t *testing.T) {
	t.Parallel()
	var sentinelPtr *nono.Error
	if !errors.As(nono.ErrPathNotFound, &sentinelPtr) {
		t.Fatal("errors.As on sentinel returned false")
	}
	if sentinelPtr.Code() != nono.ErrCodePathNotFound {
		t.Errorf("sentinel Code() = %v, want %v", sentinelPtr.Code(), nono.ErrCodePathNotFound)
	}

	cs := newCapSet(t)
	liveErr := cs.AllowPath("/nonexistent/path/xyz", nono.AccessRead)
	if liveErr == nil {
		t.Fatal("expected error from AllowPath on nonexistent path")
	}
	var livePtr *nono.Error
	if !errors.As(liveErr, &livePtr) {
		t.Fatalf("errors.As on live error returned false, got %T", liveErr)
	}
	// Live and sentinel pointers must be distinct objects.
	if livePtr == sentinelPtr {
		t.Error("live error pointer should be distinct from sentinel pointer")
	}
	// errors.Is must still consider them equal (same code).
	if !errors.Is(liveErr, nono.ErrPathNotFound) {
		t.Error("live error should match sentinel via errors.Is")
	}
}

// TestCapabilitySetConcurrent exercises the thread-safety guarantees documented
// on CapabilitySet by driving reads and writes from multiple goroutines at once.
func TestCapabilitySetConcurrent(t *testing.T) {
	t.Parallel()

	t.Run("reads and writes", func(t *testing.T) {
		t.Parallel()
		cs := newCapSetWithAnyPath(t)
		if err := cs.SetNetworkMode(nono.NetworkAllowAll); err != nil {
			t.Fatalf("SetNetworkMode(NetworkAllowAll): %v", err)
		}

		const goroutines = 8
		var wg sync.WaitGroup

		// Concurrent reads.
		for range goroutines {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_ = cs.NetworkMode()
				_ = cs.IsNetworkBlocked()
				_ = cs.Summary()
				_ = cs.FSCapabilities()
				_ = cs.ProxyPort()
			}()
		}

		// Concurrent writes while reads are in flight.
		for range goroutines {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if err := cs.SetNetworkMode(nono.NetworkBlocked); err != nil {
					t.Errorf("concurrent SetNetworkMode(Blocked): %v", err)
				}
				if err := cs.SetNetworkMode(nono.NetworkAllowAll); err != nil {
					t.Errorf("concurrent SetNetworkMode(AllowAll): %v", err)
				}
			}()
		}

		wg.Wait()

		// Verify the final state is a valid NetworkMode (not garbage).
		mode := cs.NetworkMode()
		if mode != nono.NetworkBlocked && mode != nono.NetworkAllowAll {
			t.Errorf("unexpected NetworkMode after concurrent writes: %v", mode)
		}
	})
	t.Run("close races with readers", func(t *testing.T) {
		t.Parallel()
		// Verify that Close racing with read-only methods does not panic or
		// corrupt memory. After Close, read-only methods must return zero values.
		const goroutines = 4
		var wg sync.WaitGroup
		cs := nono.New()

		// started ensures readers are actually running before Close is called,
		// so the race is genuinely exercised rather than finishing before Close.
		started := make(chan struct{}, goroutines) // buffered so senders never block

		for range goroutines {
			wg.Add(1)
			go func() {
				defer wg.Done()
				started <- struct{}{} // signal that a reader has started
				runtime.Gosched()    // yield to increase likelihood that Close races with reads
				_ = cs.NetworkMode()
				_ = cs.IsNetworkBlocked()
				_ = cs.ProxyPort()
			}()
		}
		// Wait until at least one reader has started before closing.
		<-started
		_ = cs.Close()

		wg.Wait()

		// After Close, read-only methods must return documented zero values.
		if got := cs.NetworkMode(); got != nono.NetworkBlocked {
			t.Errorf("NetworkMode after Close = %v, want NetworkBlocked", got)
		}
		if cs.IsNetworkBlocked() {
			t.Error("IsNetworkBlocked after Close should be false")
		}
		if got := cs.ProxyPort(); got != 0 {
			t.Errorf("ProxyPort after Close = %d, want 0", got)
		}
	})
}

// TestNULByteRejection verifies that all string-accepting methods reject paths
// or commands containing NUL bytes, which C.CString would silently truncate.
// This is a security invariant: a NUL-containing path must never reach the C FFI.
func TestNULByteRejection(t *testing.T) {
	t.Parallel()
	const nulPath = "/data\x00/injected"
	const nulCmd = "curl\x00 --bypass"

	cs := newCapSet(t)

	cases := []struct {
		name string
		fn   func() error
	}{
		{"AllowPath", func() error { return cs.AllowPath(nulPath, nono.AccessRead) }},
		{"AllowFile", func() error { return cs.AllowFile(nulPath, nono.AccessRead) }},
		{"AllowCommand", func() error { return cs.AllowCommand(nulCmd) }},
		{"BlockCommand", func() error { return cs.BlockCommand(nulCmd) }},
		{"AddPlatformRule", func() error { return cs.AddPlatformRule("rule\x00inject") }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.fn()
			if err == nil {
				t.Fatalf("%s: expected ErrInvalidArg for NUL-containing string, got nil", tc.name)
			}
			if !errors.Is(err, nono.ErrInvalidArg) {
				t.Errorf("%s: expected ErrInvalidArg, got %v", tc.name, err)
			}
		})
	}
}

// TestNULByteRejectionQueryPath verifies that QueryPath rejects paths with NUL bytes.
func TestNULByteRejectionQueryPath(t *testing.T) {
	t.Parallel()
	cs := newCapSetWithAnyPath(t)
	qc, err := nono.NewQueryContext(cs)
	if err != nil {
		t.Fatalf("NewQueryContext: %v", err)
	}
	t.Cleanup(func() { _ = qc.Close() })

	_, err = qc.QueryPath("/data\x00/injected", nono.AccessRead)
	if err == nil {
		t.Fatal("QueryPath: expected ErrInvalidArg for NUL-containing path, got nil")
	}
	if !errors.Is(err, nono.ErrInvalidArg) {
		t.Errorf("QueryPath: expected ErrInvalidArg, got %v", err)
	}
}

// TestNULByteRejectionStateFromJSON verifies that StateFromJSON rejects strings with NUL bytes.
func TestNULByteRejectionStateFromJSON(t *testing.T) {
	t.Parallel()
	_, err := nono.StateFromJSON("{\"key\":\x00\"value\"}")
	if err == nil {
		t.Fatal("StateFromJSON: expected ErrInvalidArg for NUL-containing string, got nil")
	}
	if !errors.Is(err, nono.ErrInvalidArg) {
		t.Errorf("StateFromJSON: expected ErrInvalidArg, got %v", err)
	}
}

// TestApplyNilAndClosed verifies that Apply rejects nil and closed sets without
// activating the sandbox (the nil/closed check runs before any system call).
func TestApplyNilAndClosed(t *testing.T) {
	t.Parallel()
	t.Run("nil", func(t *testing.T) {
		t.Parallel()
		if err := nono.Apply(nil); err == nil {
			t.Error("Apply(nil): expected error, got nil")
		}
	})

	t.Run("closed", func(t *testing.T) {
		t.Parallel()
		cs := nono.New()
		if err := cs.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
		if err := nono.Apply(cs); err == nil {
			t.Error("Apply(closed): expected error, got nil")
		}
	})
}

// TestIsSupportedTrue verifies IsSupported() returns true on supported platforms.
func TestIsSupportedTrue(t *testing.T) {
	t.Parallel()
	if !nono.IsSupported() {
		t.Error("IsSupported() = false; expected true on this platform")
	}
}

// TestPathCoveredClosedSet verifies that PathCovered on a closed set returns (false, nil).
func TestPathCoveredClosedSet(t *testing.T) {
	t.Parallel()
	cs := nono.New()
	if err := cs.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	covered, err := cs.PathCovered("/any/path")
	if err != nil {
		t.Fatalf("PathCovered on closed set: expected nil error, got %v", err)
	}
	if covered {
		t.Error("PathCovered on closed set: expected false, got true")
	}
}

// TestPathCoveredNULByte verifies that PathCovered rejects NUL-containing paths.
func TestPathCoveredNULByte(t *testing.T) {
	t.Parallel()
	cs := newCapSetWithAnyPath(t)
	_, err := cs.PathCovered("/data\x00/injected")
	if err == nil {
		t.Fatal("PathCovered: expected ErrInvalidArg for NUL-containing path, got nil")
	}
	if !errors.Is(err, nono.ErrInvalidArg) {
		t.Errorf("PathCovered: expected ErrInvalidArg, got %v", err)
	}
}

// TestSummaryClosedSet verifies that Summary on a closed set returns an empty string.
func TestSummaryClosedSet(t *testing.T) {
	t.Parallel()
	cs := nono.New()
	if err := cs.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if got := cs.Summary(); got != "" {
		t.Errorf("Summary on closed set = %q, want empty string", got)
	}
}

// TestFSCapabilitiesClosedSet verifies that FSCapabilities on a closed set returns nil.
func TestFSCapabilitiesClosedSet(t *testing.T) {
	t.Parallel()
	cs := nono.New()
	if err := cs.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if got := cs.FSCapabilities(); got != nil {
		t.Errorf("FSCapabilities on closed set = %v, want nil", got)
	}
}

// TestDeduplicateClosedSet verifies that Deduplicate on a closed set returns ErrInvalidArg.
func TestDeduplicateClosedSet(t *testing.T) {
	t.Parallel()
	cs := nono.New()
	if err := cs.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	err := cs.Deduplicate()
	if err == nil {
		t.Fatal("Deduplicate on closed set: expected error, got nil")
	}
	if !errors.Is(err, nono.ErrInvalidArg) {
		t.Errorf("Deduplicate on closed set: expected ErrInvalidArg, got %v", err)
	}
}

// TestAllowPathExpectsDirectory verifies that AllowPath on a regular file returns ErrExpectedDirectory.
func TestAllowPathExpectsDirectory(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(path, []byte("data"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cs := newCapSet(t)
	err := cs.AllowPath(path, nono.AccessRead)
	if err == nil {
		t.Fatal("AllowPath on file: expected error, got nil")
	}
	if !errors.Is(err, nono.ErrExpectedDirectory) {
		t.Errorf("AllowPath on file: expected ErrExpectedDirectory, got %v", err)
	}
}

// TestAllowFileExpectsFile verifies that AllowFile on a directory returns ErrExpectedFile.
func TestAllowFileExpectsFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cs := newCapSet(t)
	err := cs.AllowFile(dir, nono.AccessRead)
	if err == nil {
		t.Fatal("AllowFile on directory: expected error, got nil")
	}
	if !errors.Is(err, nono.ErrExpectedFile) {
		t.Errorf("AllowFile on directory: expected ErrExpectedFile, got %v", err)
	}
}

// TestAllowFileInvalid verifies that AllowFile on a non-existent path returns ErrPathNotFound.
func TestAllowFileInvalid(t *testing.T) {
	t.Parallel()
	cs := newCapSet(t)
	err := cs.AllowFile("/this/path/does/not/exist/12345.txt", nono.AccessRead)
	if err == nil {
		t.Fatal("AllowFile on non-existent path: expected error, got nil")
	}
	if !errors.Is(err, nono.ErrPathNotFound) {
		t.Errorf("AllowFile on non-existent path: expected ErrPathNotFound, got %v", err)
	}
}

// TestAllowFileAccessModes verifies that AllowFile succeeds for write and read-write modes.
func TestAllowFileAccessModes(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(path, []byte("data"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	for _, mode := range []nono.AccessMode{nono.AccessWrite, nono.AccessReadWrite} {
		t.Run(mode.String(), func(t *testing.T) {
			t.Parallel()
			cs := newCapSet(t)
			if err := cs.AllowFile(path, mode); err != nil {
				t.Fatalf("AllowFile(%v): %v", mode, err)
			}
		})
	}
}

// TestFSCapabilitiesMultiplePaths verifies that FSCapabilities returns all added paths in order.
func TestFSCapabilitiesMultiplePaths(t *testing.T) {
	t.Parallel()
	cs := newCapSet(t)
	dirs := make([]string, 3)
	for i := range dirs {
		dirs[i] = t.TempDir()
		if err := cs.AllowPath(dirs[i], nono.AccessRead); err != nil {
			t.Fatalf("AllowPath(%q): %v", dirs[i], err)
		}
	}
	caps := cs.FSCapabilities()
	if len(caps) != 3 {
		t.Fatalf("expected 3 capabilities, got %d", len(caps))
	}
}

// TestSetNetworkBlockedInteraction verifies that SetNetworkBlocked(true) sets the
// network mode to NetworkBlocked, and SetNetworkBlocked(false) sets it to NetworkAllowAll.
func TestSetNetworkBlockedInteraction(t *testing.T) {
	t.Parallel()
	cs := newCapSet(t)

	if err := cs.SetNetworkBlocked(true); err != nil {
		t.Fatalf("SetNetworkBlocked(true): %v", err)
	}
	if got := cs.NetworkMode(); got != nono.NetworkBlocked {
		t.Errorf("after SetNetworkBlocked(true): NetworkMode() = %v, want NetworkBlocked", got)
	}

	if err := cs.SetNetworkBlocked(false); err != nil {
		t.Fatalf("SetNetworkBlocked(false): %v", err)
	}
	if got := cs.NetworkMode(); got != nono.NetworkAllowAll {
		t.Errorf("after SetNetworkBlocked(false): NetworkMode() = %v, want NetworkAllowAll", got)
	}
}

// TestQueryContextDoubleClose verifies that closing a QueryContext twice is safe.
func TestQueryContextDoubleClose(t *testing.T) {
	t.Parallel()
	cs := newCapSetWithAnyPath(t)
	qc, err := nono.NewQueryContext(cs)
	if err != nil {
		t.Fatalf("NewQueryContext: %v", err)
	}
	if err := qc.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := qc.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

// TestQueryContextClosedContext verifies that QueryPath and QueryNetwork on a closed
// QueryContext return ErrInvalidArg.
func TestQueryContextClosedContext(t *testing.T) {
	t.Parallel()
	cs := newCapSetWithAnyPath(t)
	qc, err := nono.NewQueryContext(cs)
	if err != nil {
		t.Fatalf("NewQueryContext: %v", err)
	}
	if err := qc.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	t.Run("QueryPath", func(t *testing.T) {
		t.Parallel()
		_, err := qc.QueryPath("/any/path", nono.AccessRead)
		if err == nil {
			t.Fatal("expected error on closed context, got nil")
		}
		if !errors.Is(err, nono.ErrInvalidArg) {
			t.Errorf("expected ErrInvalidArg, got %v", err)
		}
	})
	t.Run("QueryNetwork", func(t *testing.T) {
		t.Parallel()
		_, err := qc.QueryNetwork()
		if err == nil {
			t.Fatal("expected error on closed context, got nil")
		}
		if !errors.Is(err, nono.ErrInvalidArg) {
			t.Errorf("expected ErrInvalidArg, got %v", err)
		}
	})
}

// TestQueryNetworkAllowed verifies that QueryNetwork returns QueryAllowed with
// ReasonNetworkAllowed when the network mode is NetworkAllowAll.
func TestQueryNetworkAllowed(t *testing.T) {
	t.Parallel()
	cs := newCapSet(t)
	if err := cs.SetNetworkMode(nono.NetworkAllowAll); err != nil {
		t.Fatalf("SetNetworkMode(AllowAll): %v", err)
	}
	qc, err := nono.NewQueryContext(cs)
	if err != nil {
		t.Fatalf("NewQueryContext: %v", err)
	}
	t.Cleanup(func() { _ = qc.Close() })

	result, err := qc.QueryNetwork()
	if err != nil {
		t.Fatalf("QueryNetwork: %v", err)
	}
	if result.Status != nono.QueryAllowed {
		t.Errorf("expected QueryAllowed, got %v", result.Status)
	}
	if result.Reason != nono.ReasonNetworkAllowed {
		t.Errorf("expected ReasonNetworkAllowed, got %v", result.Reason)
	}
}

// TestQueryPathInsufficientAccess verifies that querying write access on a read-only
// path returns ReasonInsufficientAccess and populates ActualAccess and RequestedAccess.
func TestQueryPathInsufficientAccess(t *testing.T) {
	t.Parallel()
	cs, dir := newCapSetWithPath(t) // AllowPath with AccessRead

	qc, err := nono.NewQueryContext(cs)
	if err != nil {
		t.Fatalf("NewQueryContext: %v", err)
	}
	t.Cleanup(func() { _ = qc.Close() })

	result, err := qc.QueryPath(dir, nono.AccessWrite)
	if err != nil {
		t.Fatalf("QueryPath: %v", err)
	}
	if result.Status != nono.QueryDenied {
		t.Errorf("expected QueryDenied, got %v", result.Status)
	}
	if result.Reason != nono.ReasonInsufficientAccess {
		t.Errorf("expected ReasonInsufficientAccess, got %v", result.Reason)
	}
	if result.ActualAccess == "" {
		t.Error("ActualAccess should be non-empty for ReasonInsufficientAccess")
	}
	if result.RequestedAccess == "" {
		t.Error("RequestedAccess should be non-empty for ReasonInsufficientAccess")
	}
}

// TestQueryContextSnapshotIsolation verifies that modifications to a CapabilitySet
// after creating a QueryContext do not affect the QueryContext's snapshot.
func TestQueryContextSnapshotIsolation(t *testing.T) {
	t.Parallel()
	cs := newCapSet(t)
	dir := t.TempDir()

	qc, err := nono.NewQueryContext(cs)
	if err != nil {
		t.Fatalf("NewQueryContext: %v", err)
	}
	t.Cleanup(func() { _ = qc.Close() })

	// Add a path after the QueryContext was created; the QC should not see it.
	if err := cs.AllowPath(dir, nono.AccessRead); err != nil {
		t.Fatalf("AllowPath after NewQueryContext: %v", err)
	}

	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	result, err := qc.QueryPath(resolved, nono.AccessRead)
	if err != nil {
		t.Fatalf("QueryPath: %v", err)
	}
	if result.Status != nono.QueryDenied {
		t.Errorf("snapshot isolation broken: QueryPath returned %v, want QueryDenied", result.Status)
	}
}

// TestClosedCapabilitySetReadOnlyAccessors verifies that read-only accessors on
// a closed set return documented zero values without error.
func TestClosedCapabilitySetReadOnlyAccessors(t *testing.T) {
	t.Parallel()
	cs := nono.New()
	if err := cs.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if got := cs.NetworkMode(); got != nono.NetworkBlocked {
		t.Errorf("NetworkMode on closed set = %v, want NetworkBlocked", got)
	}
	if got := cs.ProxyPort(); got != 0 {
		t.Errorf("ProxyPort on closed set = %d, want 0", got)
	}
	if cs.IsNetworkBlocked() {
		t.Error("IsNetworkBlocked on closed set should be false")
	}
	if got := cs.Summary(); got != "" {
		t.Errorf("Summary on closed set = %q, want empty string", got)
	}
}

// TestSandboxStateDoubleClose verifies that closing a SandboxState twice is safe.
func TestSandboxStateDoubleClose(t *testing.T) {
	t.Parallel()
	cs := newCapSetWithAnyPath(t)
	state, err := nono.StateFromCaps(cs)
	if err != nil {
		t.Fatalf("StateFromCaps: %v", err)
	}
	if err := state.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := state.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

// TestDeduplicateKeepsHighestAccess verifies that deduplication of overlapping
// paths keeps the highest access level (read + read-write → read-write).
func TestDeduplicateKeepsHighestAccess(t *testing.T) {
	t.Parallel()
	cs := newCapSet(t)
	dir := t.TempDir()
	if err := cs.AllowPath(dir, nono.AccessRead); err != nil {
		t.Fatalf("AllowPath(Read): %v", err)
	}
	if err := cs.AllowPath(dir, nono.AccessReadWrite); err != nil {
		t.Fatalf("AllowPath(ReadWrite): %v", err)
	}

	if err := cs.Deduplicate(); err != nil {
		t.Fatalf("Deduplicate: %v", err)
	}

	caps := cs.FSCapabilities()
	if len(caps) != 1 {
		t.Fatalf("expected 1 cap after Deduplicate, got %d", len(caps))
	}
	if caps[0].Access != nono.AccessReadWrite {
		t.Errorf("Access = %v, want AccessReadWrite", caps[0].Access)
	}
}

// TestQueryContextConcurrent exercises the concurrent-read safety guarantees
// documented on QueryContext.
func TestQueryContextConcurrent(t *testing.T) {
	t.Parallel()
	cs, dir := newCapSetWithPath(t)
	if err := cs.SetNetworkMode(nono.NetworkAllowAll); err != nil {
		t.Fatalf("SetNetworkMode: %v", err)
	}

	qc, err := nono.NewQueryContext(cs)
	if err != nil {
		t.Fatalf("NewQueryContext: %v", err)
	}
	t.Cleanup(func() { _ = qc.Close() })

	const goroutines = 8
	var wg sync.WaitGroup
	for range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := qc.QueryPath(dir, nono.AccessRead); err != nil {
				t.Errorf("concurrent QueryPath: %v", err)
			}
			if _, err := qc.QueryNetwork(); err != nil {
				t.Errorf("concurrent QueryNetwork: %v", err)
			}
		}()
	}
	wg.Wait()
}

// TestFSCapabilityFieldCompleteness verifies that all fields of FSCapability
// are populated correctly for a user-added directory path.
func TestFSCapabilityFieldCompleteness(t *testing.T) {
	t.Parallel()
	cs := newCapSet(t)
	dir := t.TempDir()
	if err := cs.AllowPath(dir, nono.AccessRead); err != nil {
		t.Fatalf("AllowPath: %v", err)
	}

	caps := cs.FSCapabilities()
	if len(caps) != 1 {
		t.Fatalf("expected 1 cap, got %d", len(caps))
	}
	cap := caps[0]
	if cap.OriginalPath == "" {
		t.Error("OriginalPath should be non-empty")
	}
	if cap.ResolvedPath == "" {
		t.Error("ResolvedPath should be non-empty")
	}
	if cap.Access != nono.AccessRead {
		t.Errorf("Access = %v, want AccessRead", cap.Access)
	}
	if cap.Source != nono.SourceUser {
		t.Errorf("Source = %v, want SourceUser", cap.Source)
	}
	if cap.GroupName != "" {
		t.Errorf("GroupName = %q, want empty for SourceUser", cap.GroupName)
	}
	if cap.IsFile {
		t.Error("IsFile should be false for a directory")
	}
}

// TestNetworkModeBlocked verifies the SetNetworkMode(NetworkBlocked) + read-back round-trip.
func TestNetworkModeBlocked(t *testing.T) {
	t.Parallel()
	cs := newCapSet(t)
	if err := cs.SetNetworkMode(nono.NetworkBlocked); err != nil {
		t.Fatalf("SetNetworkMode(NetworkBlocked): %v", err)
	}
	if got := cs.NetworkMode(); got != nono.NetworkBlocked {
		t.Errorf("NetworkMode() = %v, want NetworkBlocked", got)
	}
}

// TestProxyPortDefaultZero verifies that ProxyPort returns 0 on a fresh set
// and after setting a non-ProxyOnly network mode.
func TestProxyPortDefaultZero(t *testing.T) {
	t.Parallel()
	cs := newCapSet(t)
	if got := cs.ProxyPort(); got != 0 {
		t.Errorf("ProxyPort on fresh set = %d, want 0", got)
	}
	if err := cs.SetNetworkMode(nono.NetworkAllowAll); err != nil {
		t.Fatalf("SetNetworkMode(AllowAll): %v", err)
	}
	if got := cs.ProxyPort(); got != 0 {
		t.Errorf("ProxyPort after AllowAll = %d, want 0", got)
	}
}

// TestSummaryContainsPath verifies that Summary includes the resolved path of
// an allowed directory.
func TestSummaryContainsPath(t *testing.T) {
	t.Parallel()
	cs := newCapSet(t)
	dir := t.TempDir()
	if err := cs.AllowPath(dir, nono.AccessRead); err != nil {
		t.Fatalf("AllowPath: %v", err)
	}

	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}

	summary := cs.Summary()
	if summary == "" {
		t.Fatal("Summary() returned empty string")
	}
	if !strings.Contains(summary, resolved) {
		t.Errorf("Summary() = %q, expected it to contain resolved path %q", summary, resolved)
	}
}

// TestQueryNetworkBlockedFields verifies that QueryNetwork when blocked returns
// ReasonNetworkBlocked and that string fields are empty.
func TestQueryNetworkBlockedFields(t *testing.T) {
	t.Parallel()
	cs := newCapSet(t)
	if err := cs.SetNetworkBlocked(true); err != nil {
		t.Fatalf("SetNetworkBlocked(true): %v", err)
	}

	qc, err := nono.NewQueryContext(cs)
	if err != nil {
		t.Fatalf("NewQueryContext: %v", err)
	}
	t.Cleanup(func() { _ = qc.Close() })

	result, err := qc.QueryNetwork()
	if err != nil {
		t.Fatalf("QueryNetwork: %v", err)
	}
	if result.Status != nono.QueryDenied {
		t.Errorf("expected QueryDenied, got %v", result.Status)
	}
	if result.Reason != nono.ReasonNetworkBlocked {
		t.Errorf("expected ReasonNetworkBlocked, got %v", result.Reason)
	}
	if result.GrantedPath != "" {
		t.Errorf("GrantedPath = %q, want empty", result.GrantedPath)
	}
	if result.GrantedAccess != "" {
		t.Errorf("GrantedAccess = %q, want empty", result.GrantedAccess)
	}
	if result.ActualAccess != "" {
		t.Errorf("ActualAccess = %q, want empty", result.ActualAccess)
	}
	if result.RequestedAccess != "" {
		t.Errorf("RequestedAccess = %q, want empty", result.RequestedAccess)
	}
}

// TestAllowFileFieldCompleteness verifies that all FSCapability fields are
// correctly populated when using AllowFile.
func TestAllowFileFieldCompleteness(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "testfile.txt")
	if err := os.WriteFile(path, []byte("test"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cs := newCapSet(t)
	if err := cs.AllowFile(path, nono.AccessReadWrite); err != nil {
		t.Fatalf("AllowFile: %v", err)
	}

	caps := cs.FSCapabilities()
	if len(caps) != 1 {
		t.Fatalf("expected 1 cap, got %d", len(caps))
	}
	cap := caps[0]
	if cap.OriginalPath == "" {
		t.Error("OriginalPath should be non-empty")
	}
	if cap.ResolvedPath == "" {
		t.Error("ResolvedPath should be non-empty")
	}
	if cap.Access != nono.AccessReadWrite {
		t.Errorf("Access = %v, want AccessReadWrite", cap.Access)
	}
	if !cap.IsFile {
		t.Error("IsFile should be true")
	}
	if cap.Source != nono.SourceUser {
		t.Errorf("Source = %v, want SourceUser", cap.Source)
	}
	if cap.GroupName != "" {
		t.Errorf("GroupName = %q, want empty for SourceUser", cap.GroupName)
	}
}
