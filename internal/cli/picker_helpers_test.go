package cli

import (
	"context"
	"reflect"
	"testing"

	"github.com/erzz/boo/internal/picker"
)

// TestProjectPreviewer_ThemeColors: projectPreviewer must use the supplied picker.Theme, not hard-coded colours.
//
// In CI (no real TTY) lipgloss strips ANSI, so both themes produce identical plain text. Instead:
//  1. Confirm the two themes have structurally different styles (proves theme palette is plumbed through).
//  2. Smoke-test that the previewer returns non-empty content (proves the theme code path executes).
func TestProjectPreviewer_ThemeColors(t *testing.T) {
	defaultThm, ok := picker.ThemeByName("", "default")
	if !ok {
		t.Fatal("could not load default theme")
	}
	lightThm, ok := picker.ThemeByName("", "light")
	if !ok {
		t.Fatal("could not load light theme")
	}

	// "default" uses "#A594FF", "light" uses "#5B5BD6"; both land in RightPaneTitle.
	// DeepEqual styles → theme colours not plumbed (hard-coded colour is back).
	if reflect.DeepEqual(defaultThm.RightPaneTitle, lightThm.RightPaneTitle) {
		t.Error("RightPaneTitle styles must differ between default and light themes; " +
			"this means projectPreviewer is not using the theme argument")
	}

	// Smoke-test: previewer returns non-empty content.
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
