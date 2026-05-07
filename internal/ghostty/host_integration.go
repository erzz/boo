//go:build integration
// +build integration

package ghostty

import "os"

// insideGhostty reports whether the current process is hosted in a Ghostty
// terminal. See detectGhosttyHost for the signals. Only compiled under the
// integration build tag because that's the only consumer (the integration
// test guard refuses to run when launched from inside the Ghostty window
// the test would otherwise stomp on).
func insideGhostty() bool { return detectGhosttyHost(os.Getenv) }
