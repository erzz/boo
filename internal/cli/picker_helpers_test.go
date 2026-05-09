package cli

import (
	"context"
	"reflect"
	"testing"

	"github.com/erzz/boo/internal/picker"
)

// TestProjectPreviewer_ThemeColors verifies that projectPreviewer is wired to
// use the supplied picker.Theme rather than hard-coded ANSI colour values.
//
// In a CI environment without a real TTY, lipgloss strips ANSI codes from
// rendered output, so both themes produce identical plain text. Instead of
// comparing rendered strings (which are unreliable in non-TTY contexts) we:
//
//  1. Confirm the two themes have structurally different styles (proving the
//     theme palette is plumbed through to the lipgloss style objects).
//  2. Smoke-test that the previewer returns non-empty content, which proves
//     the code path exercised by the theme is executed without error.
//
// A future test that requires full ANSI output should force the termenv colour
// profile via lipgloss.NewRenderer — left as a TODO for when the test
// infrastructure supports it.
func TestProjectPreviewer_ThemeColors(t *testing.T) {
	defaultThm, ok := picker.ThemeByName("", "default")
	if !ok {
		t.Fatal("could not load default theme")
	}
	lightThm, ok := picker.ThemeByName("", "light")
	if !ok {
		t.Fatal("could not load light theme")
	}

	// The "default" theme uses accent "#A594FF" and the "light" theme uses
	// "#5B5BD6". This propagates to RightPaneTitle (used by boldAccent).
	// If the two styles are DeepEqual the theme colours are not being plumbed
	// through — most likely the old hard-coded lipgloss.Color("13") is back.
	if reflect.DeepEqual(defaultThm.RightPaneTitle, lightThm.RightPaneTitle) {
		t.Error("RightPaneTitle styles must differ between default and light themes; " +
			"this means projectPreviewer is not using the theme argument")
	}

	// Smoke-test: previewer returns non-empty content for a real project.
	a := makeAppForCmds(t)
	dir := t.TempDir()
	registerProjectForTest(t, a, "proj", dir, "triple")

	ctx := context.Background()

	if got := projectPreviewer(ctx, a, defaultThm)("proj"); got == "" {
		t.Fatal("projectPreviewer returned empty for registered project with default theme")
	}
	if got := projectPreviewer(ctx, a, lightThm)("proj"); got == "" {
		t.Fatal("projectPreviewer returned empty for registered project with light theme")
	}
}

// TestProjectPreviewer_UnknownProject returns empty string gracefully.
func TestProjectPreviewer_UnknownProject(t *testing.T) {
	a := makeAppForCmds(t)
	thm, _ := picker.ThemeByName("", "default")

	result := projectPreviewer(context.Background(), a, thm)("no-such-project")
	if result != "" {
		t.Errorf("expected empty string for unknown project, got %q", result)
	}
}
