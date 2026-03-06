# nono-go

[![CI](https://github.com/always-further/nono-go/actions/workflows/ci.yml/badge.svg)](https://github.com/always-further/nono-go/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/always-further/nono-go.svg)](https://pkg.go.dev/github.com/always-further/nono-go)

Go CGo bindings for the [nono](https://github.com/always-further/nono) capability-based security sandbox.

nono applies an irreversible, least-privilege sandbox to the current process using Linux Landlock (Linux) or Seatbelt/`sandbox_init` (macOS). You declare the paths and network modes the process needs; nono enforces them at the kernel level.

## Platform support

| OS    | Arch  | Library bundled? |
|-------|-------|------------------|
| macOS | arm64 | yes              |
| macOS | amd64 | build required  |
| Linux | amd64 | build required  |
| Linux | arm64 | build required  |

darwin/arm64 works out of the box. All other platforms require building the native library first (see [Building native libraries](#building-native-libraries)).

## Prerequisites

- Go 1.24+
- A C toolchain (`gcc` or `clang`) for CGo
- For building missing native libs: Rust stable toolchain + `cargo`, and `cargo cross` for cross-compilation

## Installation

```
go get github.com/always-further/nono-go
```

## Building native libraries

`scripts/build-libs.sh` clones the upstream nono repository, cross-compiles `libnono_ffi.a` for all four targets, and copies the results into `internal/clib/`.

```sh
# Clone nono automatically and build all targets
./scripts/build-libs.sh

# Use an existing nono checkout
./scripts/build-libs.sh --nono-src /path/to/nono
```

To build a single target manually:

```sh
cargo build --release \
  --manifest-path /path/to/nono/Cargo.toml \
  -p nono-ffi \
  --target x86_64-unknown-linux-gnu

cp /path/to/nono/target/x86_64-unknown-linux-gnu/release/libnono_ffi.a \
  internal/clib/linux_amd64/libnono_ffi.a
```

## Testing

```sh
go test -v ./...
go vet ./...
staticcheck ./...   # go install honnef.co/go/tools/cmd/staticcheck@latest
```

## Usage

### Apply a sandbox

```go
caps := nono.New()
defer caps.Close()

if err := caps.AllowPath("/home/user/data", nono.AccessRead); err != nil {
    log.Fatal(err)
}
if err := caps.AllowPath("/tmp", nono.AccessReadWrite); err != nil {
    log.Fatal(err)
}
if err := caps.SetNetworkMode(nono.NetworkBlocked); err != nil {
    log.Fatal(err)
}

// Irreversible — applies to this process and all children.
if err := nono.Apply(caps); err != nil {
    log.Fatal(err)
}
```

### Query permissions without applying

`QueryContext` lets you check what a capability set would allow before (or instead of) applying it. The capability set is cloned internally, so later changes to `caps` don't affect the query context.

```go
caps := nono.New()
if err := caps.AllowPath("/home/user/data", nono.AccessRead); err != nil {
    log.Fatal(err)
}
if err := caps.SetNetworkMode(nono.NetworkAllowAll); err != nil {
    log.Fatal(err)
}

qc, err := nono.NewQueryContext(caps)
if err != nil {
    log.Fatal(err)
}
defer qc.Close()

result, err := qc.QueryPath("/home/user/data/file.txt", nono.AccessRead)
if err != nil {
    log.Fatal(err)
}
fmt.Println(result.Status) // nono.QueryAllowed

netResult, err := qc.QueryNetwork()
if err != nil {
    log.Fatal(err)
}
fmt.Println(netResult.Status) // nono.QueryAllowed
```

### Serialize and deserialize state

`SandboxState` provides a JSON-serializable snapshot of a `CapabilitySet`, useful for persisting or transmitting sandbox configuration.

```go
caps := nono.New()
if err := caps.AllowPath("/data", nono.AccessReadWrite); err != nil {
    log.Fatal(err)
}
if err := caps.SetNetworkMode(nono.NetworkBlocked); err != nil {
    log.Fatal(err)
}

state, err := nono.StateFromCaps(caps)
if err != nil {
    log.Fatal(err)
}
defer state.Close()

jsonStr, err := state.ToJSON()
if err != nil {
    log.Fatal(err)
}

// Later: restore from JSON
restored, err := nono.StateFromJSON(jsonStr)
if err != nil {
    log.Fatal(err)
}
defer restored.Close()

caps2, err := restored.ToCaps()
if err != nil {
    log.Fatal(err)
}
defer caps2.Close()
```

## Error handling

All failing operations return `*nono.Error`. Use `errors.Is` with a sentinel accessor to test for specific failure kinds:

```go
err := caps.AllowPath("/nonexistent", nono.AccessRead)
if errors.Is(err, nono.ErrPathNotFound()) {
    // path does not exist
}
```

Named sentinel accessors (each is a function — note the `()`): `ErrPathNotFound()`, `ErrExpectedDirectory()`, `ErrExpectedFile()`, `ErrPathCanonicalization()`, `ErrNoCapabilities()`, `ErrSandboxInit()`, `ErrUnsupportedPlatform()`, `ErrBlockedCommand()`, `ErrConfigParse()`, `ErrProfileParse()`, `ErrIO()`, `ErrInvalidArg()`, `ErrTrustVerification()`, `ErrUnknown()`.

## macOS path canonicalization

On macOS, paths under `/var` (including those returned by `os.TempDir` and `t.TempDir()`) are symlinks to `/private/var`. nono canonicalizes paths, so the resolved capability will be under `/private/var`. When checking `PathCovered`, resolve symlinks first:

```go
dir, _ := filepath.EvalSymlinks(t.TempDir())
covered, err := caps.PathCovered(filepath.Join(dir, "file.txt"))
```
