package doctor

import "runtime"

// runtimeGOOS is a tiny indirection to keep tests honest.
func runtimeGOOS() string { return runtime.GOOS }
