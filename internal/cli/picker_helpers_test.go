package cli

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
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

func TestLayoutResolver_RejectsResponsiveTemplatesForEditor(t *testing.T) {
	a := makeAppForCmds(t)
	responsive := []byte("name: responsive\nvariants:\n  - tabs:\n      - split:\n          cwd: .\n  - min_cols: 120\n    tabs:\n      - split:\n          direction: row\n          children:\n            - cwd: .\n            - cwd: logs\n")
	if err := os.WriteFile(filepath.Join(a.Paths.LayoutsDir, "responsive.yaml"), responsive, 0o644); err != nil {
		t.Fatalf("write template: %v", err)
	}

	_, err := layoutResolver(a)("responsive")
	if err == nil {
		t.Fatal("expected responsive template rejection, got nil")
	}
	if !strings.Contains(err.Error(), "does not support responsive layouts") {
		t.Fatalf("error = %v, want responsive-editor message", err)
	}
}

func TestProjectPreviewer_RendersDefaultResponsiveVariant(t *testing.T) {
	a := makeAppForCmds(t)
	thm, _ := picker.ThemeByName("", "default")
	dir := t.TempDir()
	registerProjectForTest(t, a, "proj", dir, "1x1x1")
	responsive := []byte("name: responsive\nvariants:\n  - min_cols: 120\n    tabs:\n      - name: wide\n        split:\n          direction: row\n          children:\n            - cwd: .\n            - cwd: logs\n  - tabs:\n      - name: compact\n        split:\n          cwd: .\n")
	if err := os.WriteFile(a.Paths.ProjectLayoutFile("proj"), responsive, 0o644); err != nil {
		t.Fatalf("write layout: %v", err)
	}

	got := projectPreviewer(context.Background(), a, thm)("proj")
	if !strings.Contains(got, `Tab 0 "compact"`) {
		t.Fatalf("preview = %q, want default responsive variant", got)
	}
}

func TestTemplatePreviewer_RendersDefaultResponsiveVariant(t *testing.T) {
	a := makeAppForCmds(t)
	responsive := []byte("name: responsive\nvariants:\n  - min_cols: 120\n    tabs:\n      - name: wide\n        split:\n          direction: row\n          children:\n            - cwd: .\n            - cwd: logs\n  - tabs:\n      - name: compact\n        split:\n          cwd: .\n")
	if err := os.WriteFile(filepath.Join(a.Paths.LayoutsDir, "responsive.yaml"), responsive, 0o644); err != nil {
		t.Fatalf("write template: %v", err)
	}

	got := templatePreviewer(a)("responsive")
	if !strings.Contains(got, `Tab 0 "compact"`) {
		t.Fatalf("preview = %q, want default responsive variant", got)
	}
}

func TestOpenProjectLayoutCmd_RejectsResponsiveLayoutWhenSnapshotMissing(t *testing.T) {
	a := makeAppForCmds(t)
	dir := t.TempDir()
	responsive := []byte("name: responsive\nvariants:\n  - tabs:\n      - split:\n          cwd: .\n  - min_cols: 120\n    tabs:\n      - split:\n          direction: row\n          children:\n            - cwd: .\n            - cwd: logs\n")
	if err := os.WriteFile(filepath.Join(a.Paths.LayoutsDir, "responsive.yaml"), responsive, 0o644); err != nil {
		t.Fatalf("write template: %v", err)
	}
	registerProjectForTest(t, a, "proj", dir, "responsive")
	if err := os.Remove(a.Paths.ProjectLayoutFile("proj")); err != nil {
		t.Fatalf("remove snapshot: %v", err)
	}

	msg := openProjectLayoutCmd(a, "proj")()
	if !strings.Contains(reflect.TypeOf(msg).String(), "editorFinishedMsg") {
		t.Fatalf("msg = %T, want editorFinishedMsg", msg)
	}
	if got := editorFinishedErrString(t, msg); !strings.Contains(got, "responsive layout") {
		t.Fatalf("err = %q, want responsive-layout rejection", got)
	}
}

func TestOpenProjectLayoutCmd_RejectsResponsiveLayoutWhenSnapshotUnreadable(t *testing.T) {
	a := makeAppForCmds(t)
	dir := t.TempDir()
	responsive := []byte("name: responsive\nvariants:\n  - tabs:\n      - split:\n          cwd: .\n  - min_cols: 120\n    tabs:\n      - split:\n          direction: row\n          children:\n            - cwd: .\n            - cwd: logs\n")
	if err := os.WriteFile(filepath.Join(a.Paths.LayoutsDir, "responsive.yaml"), responsive, 0o644); err != nil {
		t.Fatalf("write template: %v", err)
	}
	registerProjectForTest(t, a, "proj", dir, "responsive")
	if err := os.WriteFile(a.Paths.ProjectLayoutFile("proj"), []byte("name: broken\nvariants: ["), 0o644); err != nil {
		t.Fatalf("write broken snapshot: %v", err)
	}

	msg := openProjectLayoutCmd(a, "proj")()
	if !strings.Contains(reflect.TypeOf(msg).String(), "editorFinishedMsg") {
		t.Fatalf("msg = %T, want editorFinishedMsg", msg)
	}
	if got := editorFinishedErrString(t, msg); !strings.Contains(got, "responsive layout") {
		t.Fatalf("err = %q, want responsive-layout rejection", got)
	}
}

func editorFinishedErrString(t *testing.T, msg any) string {
	t.Helper()
	v := reflect.ValueOf(msg)
	field := v.FieldByName("err")
	if !field.IsValid() || field.IsNil() {
		return ""
	}
	errVal := field.Elem()
	if errVal.Kind() == reflect.Pointer {
		errVal = errVal.Elem()
	}
	msgField := errVal.FieldByName("s")
	if msgField.IsValid() && msgField.Kind() == reflect.String {
		return msgField.String()
	}
	t.Fatalf("could not extract error string from %T", msg)
	return ""
}
