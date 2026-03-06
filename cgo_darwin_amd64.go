//go:build darwin && amd64

package nono

// #cgo LDFLAGS: -L${SRCDIR}/internal/clib/darwin_amd64 -lnono_ffi -framework Security
import "C"
