//go:build linux && amd64

package nono

// #cgo LDFLAGS: -L${SRCDIR}/internal/clib/linux_amd64 -lnono_ffi -ldl -lpthread -lm
import "C"
