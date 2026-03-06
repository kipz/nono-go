//go:build !((darwin || linux) && (amd64 || arm64))

// This file is compiled only on platforms NOT in the supported set:
// darwin/arm64, darwin/amd64, linux/amd64, linux/arm64.
// The type mismatch below produces a compile error with a message that
// explains which platforms are supported.
package nono

// This assignment is intentionally a type error; the string literal becomes the
// compiler's diagnostic message, guiding developers to a supported platform.
var _ int = "nono: unsupported platform - rebuild with GOOS=(darwin|linux) GOARCH=(amd64|arm64)"
