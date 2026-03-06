//go:build darwin && arm64

package nono

// #cgo LDFLAGS: -L${SRCDIR}/internal/clib/darwin_arm64 -lnono_ffi -framework Security
import "C"
