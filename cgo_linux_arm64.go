//go:build linux && arm64

package nono

// #cgo LDFLAGS: -L${SRCDIR}/internal/clib/linux_arm64 -lnono_ffi -ldl -lpthread -lm
import "C"
