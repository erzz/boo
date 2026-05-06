package exec

import "bytes"

// Tiny wrappers so runner.go doesn't pull in bytes directly at the top level —
// keeps the runner file focused on the Runner interface.

type bytesBuffer = bytes.Buffer

func bytesReader(b []byte) *bytes.Reader { return bytes.NewReader(b) }
