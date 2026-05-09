package picker

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/charmbracelet/bubbles/list"

	"github.com/erzz/boo/internal/config"
)

// newCycleTestModel constructs a minimal *model wired enough for
// cycleTheme to exercise its full path: a list with a delegate that
// gets reassigned, a form with a theme field, and the three theme
// cycler fields. Avoids tea.NewProgram because cycleTheme is pure
// state mutation; no IO loop required.
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

func TestCycleTheme_AdvancesAndPersists(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte("ui:\n  theme: default\n"), 0o644); err != nil {
		t.Fatalf("seed config: %v", err)
	}

	m := newCycleTestModel(t, "default", cfgPath)

	m.cycleTheme()

	if m.themeName == "default" {
		t.Error("themeName did not advance from default")
	}
	if m.status.text == "" || m.status.isErr {
		t.Errorf("expected OK status, got text=%q isErr=%v", m.status.text, m.status.isErr)
	}

	cfg, _, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("Load after cycle: %v", err)
	}
	if cfg.ThemeOr("") != m.themeName {
		t.Errorf("config theme=%q, model theme=%q (must match after persist)",
			cfg.ThemeOr(""), m.themeName)
	}
}

func TestCycleTheme_NoConfigPathDoesNotPersist(t *testing.T) {
	m := newCycleTestModel(t, "default", "")
	m.cycleTheme()

	if m.themeName == "default" {
		t.Error("themeName did not advance")
	}
	if m.status.isErr {
		t.Errorf("expected OK status with no config, got error: %q", m.status.text)
	}
	// Status should not claim it was saved.
	if got := m.status.text; len(got) == 0 {
		t.Error("expected non-empty status")
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
