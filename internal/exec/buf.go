package exec

import "bytes"

type bytesBuffer = bytes.Buffer

func bytesReader(b []byte) *bytes.Reader { return bytes.NewReader(b) }
