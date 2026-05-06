//go:build integration
// +build integration

package ghostty

import (
	"os"
	"testing"
)

// guardAgainstSelfTermination refuses to run integration tests when the test
// process is itself running inside a Ghostty window. Rationale: these tests
// open and close real Ghostty windows via JXA. If the test runner happens to
// be hosted in Ghostty (e.g. an editor session running `make test-int`), a
// bug in close-window targeting — or an accidental `closeWindow` on the
// frontmost window — can take the test runner's own window down with it,
// killing the parent shell, the editor, and the OpenCode session.
//
// This actually happened. The previous OpenCode session in this project
// died when this test suite ran inside Ghostty.
//
// To explicitly opt in (for example, on a fresh tmux pane outside Ghostty,
// or when iterating on the JXA itself), set BOO_ALLOW_GHOSTTY_INTEGRATION=1.
func guardAgainstSelfTermination(t *testing.T) {
	t.Helper()
	if os.Getenv("BOO_ALLOW_GHOSTTY_INTEGRATION") == "1" {
		return
	}
	if isInsideGhostty() {
		t.Skip("refusing to run Ghostty integration tests from inside a Ghostty window " +
			"(they open and close real windows, which has previously taken down the host shell). " +
			"Run from a non-Ghostty terminal, or set BOO_ALLOW_GHOSTTY_INTEGRATION=1 to override.")
	}
}

// isInsideGhostty wraps the package-level detector for use by the test guard.
func isInsideGhostty() bool { return insideGhostty() }
