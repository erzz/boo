package picker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/list"
)

// newCycleTestModel constructs a minimal *model wired enough for
// cycleTheme to exercise its full path: a list with a delegate that
// gets reassigned, a form with a theme field, and the theme cycler
// fields. Avoids tea.NewProgram because cycleTheme is pure state
// mutation; no IO loop required.
//
// configPath, when non-empty, is stored on the model so cycleTheme
// will attempt to persist the new theme to disk.
func newCycleTestModel(t *testing.T, themeName, configPath string) *model {
	t.Helper()
	th := defaultTheme()
	l := list.New(nil, newDelegate(th), 0, 0)
	return &model{
		list:       l,
		theme:      th,
		themeName:  themeName,
		configPath: configPath,
		// themesDir empty => built-ins only, which is what we want.
	}
}

func TestCycleTheme_AdvancesTheme(t *testing.T) {
	m := newCycleTestModel(t, "default", "")

	m.cycleTheme()

	if m.themeName == "default" {
		t.Error("themeName did not advance from default")
	}
	if m.status.text == "" || m.status.isErr {
		t.Errorf("expected OK status, got text=%q isErr=%v", m.status.text, m.status.isErr)
	}
}

// TestCycleTheme_AdvancesAndPersists verifies that when a config path is
// provided, cycleTheme writes the new theme name to disk.
func TestCycleTheme_AdvancesAndPersists(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte("ui:\n  theme: default\n"), 0o644); err != nil {
		t.Fatalf("seed config: %v", err)
	}

	m := newCycleTestModel(t, "default", cfgPath)
	m.cycleTheme()

	if m.themeName == "default" {
		t.Error("in-memory themeName did not advance")
	}
	got, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("re-read config: %v", err)
	}
	if !strings.Contains(string(got), "theme: "+m.themeName) {
		t.Errorf("persisted file does not contain theme %q:\n%s", m.themeName, got)
	}
}

// TestCycleTheme_StatusShowsThemeNameOnSuccess verifies that on a successful
// cycle (with or without a config path) the status bar shows just the theme name.
func TestCycleTheme_StatusShowsThemeNameOnSuccess(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte("ui:\n  theme: default\n"), 0o644); err != nil {
		t.Fatalf("seed config: %v", err)
	}

	m := newCycleTestModel(t, "default", cfgPath)
	m.cycleTheme()

	if m.status.isErr {
		t.Fatalf("unexpected error status: %q", m.status.text)
	}
	want := "theme: " + m.themeName
	if m.status.text != want {
		t.Errorf("status = %q, want %q", m.status.text, want)
	}
	// Must NOT mention session-only or persist hints.
	if strings.Contains(m.status.text, "session only") {
		t.Errorf("status unexpectedly mentions 'session only': %q", m.status.text)
	}
}

// TestCycleTheme_StatusSurfacesWriteFailure verifies that when the persist
// write fails, the in-memory theme still advances and the status bar reports
// the failure without marking it as a hard error (isErr=false, it's an OK
// status that includes the failure note).
func TestCycleTheme_StatusSurfacesWriteFailure(t *testing.T) {
	// Point configPath at a directory, so WriteAtomic will fail.
	dir := t.TempDir()
	badPath := filepath.Join(dir, "is-a-dir", "config.yaml")
	// The parent doesn't exist, so any write will fail.

	m := newCycleTestModel(t, "default", badPath)
	m.cycleTheme()

	// In-memory theme must have advanced despite the write failure.
	if m.themeName == "default" {
		t.Error("in-memory themeName did not advance on write failure")
	}
	// Status must mention the failure.
	if !strings.Contains(m.status.text, "session only") {
		t.Errorf("status does not mention 'session only' on write failure: %q", m.status.text)
	}
	if !strings.Contains(m.status.text, "failed to persist") {
		t.Errorf("status does not mention 'failed to persist': %q", m.status.text)
	}
	// The theme name should still be in the status.
	if !strings.Contains(m.status.text, "theme: "+m.themeName) {
		t.Errorf("status does not include theme name %q: %q", m.themeName, m.status.text)
	}
}

func TestCycleTheme_WrapsAround(t *testing.T) {
	// Start from whatever the last theme is in built-in alphabetical
	// order and confirm we wrap to the first.
	m := newCycleTestModel(t, "default", "")

	// Cycle through every built-in. After len(builtins) presses we
	// should be back at the starting theme.
	const maxIters = 20 // safety cap
	startName := m.themeName
	advanced := 0
	for i := 0; i < maxIters; i++ {
		m.cycleTheme()
		advanced++
		if m.themeName == startName {
			break
		}
	}
	if m.themeName != startName {
		t.Errorf("did not wrap to %q after %d cycles (still at %q)",
			startName, advanced, m.themeName)
	}
	if advanced < 2 {
		t.Errorf("wrap happened in %d cycle — need more than one built-in theme", advanced)
	}
}

// ─── Should-fix 4: Theme cycling must refresh preview via factory ─────────────

// TestCycleTheme_InvalidatesPreviewCache verifies that when a
// PreviewProjectFactory is configured, cycleTheme:
//   - calls the factory with the new theme so previewProject is updated, and
//   - clears the previewCache so stale themed strings are not shown.
//
// It also confirms that a non-nil tea.Cmd is returned so the caller can
// dispatch a fresh async preview for the selected item.
func TestCycleTheme_InvalidatesPreviewCache(t *testing.T) {
	th := defaultTheme()
	l := list.New([]list.Item{Item{Key: "proj", Title: "proj"}}, newDelegate(th), 80, 24)

	factoryCalls := 0
	m := &model{
		list:      l,
		theme:     th,
		themeName: "default",
		keys:      defaultKeyMap(),
		// Factory produces a closure that embeds a unique marker so we can
		// verify it was called and that previewProject was updated.
		previewProjectFactory: func(_ Theme) func(string) string {
			factoryCalls++
			return func(name string) string {
				return "factory-preview-v2-" + name
			}
		},
		// Seed previewProject with the "old" closure to detect replacement.
		previewProject: func(name string) string {
			return "factory-preview-v1-" + name
		},
	}
	// Pre-populate the cache with a "stale" entry from the old theme.
	m.previewCache = map[string]string{"proj": "factory-preview-v1-proj"}

	// Cycle the theme. applyTheme should call the factory and clear the cache.
	cmd := m.cycleTheme()

	if m.previewCache != nil {
		t.Error("previewCache must be nil after theme cycle — stale themed previews must be discarded")
	}
	if factoryCalls != 1 {
		t.Errorf("factory called %d times after cycleTheme, want 1", factoryCalls)
	}
	// A new preview must be queued so the right pane refreshes with the new theme.
	if cmd == nil {
		t.Error("cycleTheme must return a non-nil cmd to trigger fresh async preview dispatch")
	}
	// The factory updated previewProject — verify by calling it directly.
	if m.previewProject == nil {
		t.Fatal("previewProject must be non-nil after factory call in applyTheme")
	}
	if got := m.previewProject("proj"); got != "factory-preview-v2-proj" {
		t.Errorf("previewProject after cycle = %q, want %q (factory must have updated the closure)",
			got, "factory-preview-v2-proj")
	}
}
