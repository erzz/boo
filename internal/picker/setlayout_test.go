package picker

import (
	"strings"
	"testing"
)

// helper: build a model with cycler-mode wired (LayoutNames + a stub
// preview callback + a stub OnSetLayout). Mirrors how the CLI populates
// Options.
func setLayoutModelWithNames(t *testing.T, names []string, items ...Item) (*model, *[]string) {
	t.Helper()
	m := sizedModel(120, items...)
	m.layoutNames = names
	m.previewTemplate = func(name string) string { return "preview:" + name }
	calls := []string{}
	m.onSetLayout = func(name, template string) error {
		calls = append(calls, name+"="+template)
		return nil
	}
	m.refreshItems = func() ([]Item, error) { return items, nil }
	return m, &calls
}

// Pressing 'l' on a project Item opens the set-layout sub-screen
// pre-positioned at the project's current template.
func TestList_LPressOpensSetLayoutAtCurrentTemplate(t *testing.T) {
	m, _ := setLayoutModelWithNames(t,
		[]string{"1x1", "triple", "2x2x2"},
		Item{Key: "alpha", Title: "alpha", Description: "/tmp/a", Layout: "triple"},
	)
	m = pressKey(t, m, "l")

	if m.screen != screenSetLayout {
		t.Fatalf("screen = %v, want screenSetLayout", m.screen)
	}
	if got := m.setLayout.current(); got != "triple" {
		t.Errorf("anchor template = %q, want %q (project's current)", got, "triple")
	}
	v := m.View()
	if !strings.Contains(v, "alpha") {
		t.Errorf("view missing project name:\n%s", v)
	}
	if !strings.Contains(v, "preview:triple") {
		t.Errorf("view missing template preview:\n%s", v)
	}
}

// Pressing 'l' when no LayoutNames were configured (e.g. selection-only
// pickers) is a no-op rather than opening an empty cycler.
func TestList_LPressNoNamesIsNoop(t *testing.T) {
	m := sizedModel(120, Item{Key: "alpha", Title: "alpha", Description: "/tmp/a"})
	// Deliberately do NOT set m.layoutNames.
	m = pressKey(t, m, "l")
	if m.screen != screenList {
		t.Errorf("screen = %v, want screenList (l should be no-op without LayoutNames)", m.screen)
	}
}

// ←/→ (and h/l) cycle through the template list with wraparound.
func TestSetLayout_CycleWrapsBothDirections(t *testing.T) {
	m, _ := setLayoutModelWithNames(t,
		[]string{"a", "b", "c"},
		Item{Key: "p", Title: "p", Description: "/tmp/p", Layout: "a"},
	)
	m = pressKey(t, m, "l")

	m = pressKey(t, m, "right")
	if got := m.setLayout.current(); got != "b" {
		t.Errorf("after →: current = %q, want b", got)
	}
	m = pressKey(t, m, "right")
	m = pressKey(t, m, "right") // wrap
	if got := m.setLayout.current(); got != "a" {
		t.Errorf("after 3×→: current = %q, want a (wrap)", got)
	}
	m = pressKey(t, m, "left") // wrap backwards
	if got := m.setLayout.current(); got != "c" {
		t.Errorf("after ←: current = %q, want c (wrap)", got)
	}
}

// Enter on the cycler invokes the OnSetLayout callback with the
// highlighted template, then returns to the list. No intent is set
// (the action was handled in-loop).
func TestSetLayout_EnterInvokesCallback(t *testing.T) {
	m, calls := setLayoutModelWithNames(t,
		[]string{"a", "b", "c"},
		Item{Key: "alpha", Title: "alpha", Description: "/tmp/a", Layout: "a"},
	)
	m = pressKey(t, m, "l")
	m = pressKey(t, m, "right") // → b
	m = pressKey(t, m, "enter")

	if len(*calls) != 1 || (*calls)[0] != "alpha=b" {
		t.Errorf("OnSetLayout calls = %v, want [alpha=b]", *calls)
	}
	if m.screen != screenList {
		t.Errorf("screen after success = %v, want screenList", m.screen)
	}
	if m.intent != nil {
		t.Errorf("intent = %v, want nil (handled in loop)", m.intent)
	}
}

// Esc returns to the list with no intent set.
func TestSetLayout_EscReturnsToList(t *testing.T) {
	m, _ := setLayoutModelWithNames(t,
		[]string{"a", "b"},
		Item{Key: "p", Title: "p", Description: "/tmp/p", Layout: "a"},
	)
	m = pressKey(t, m, "l")
	m = pressKey(t, m, "esc")
	if m.screen != screenList {
		t.Errorf("screen = %v, want screenList", m.screen)
	}
	if m.intent != nil {
		t.Errorf("intent = %v, want nil", m.intent)
	}
}

// If the project's current template isn't in the names list (e.g. a
// user-defined template that got deleted), the cycler still opens —
// just anchored at index 0 — rather than erroring.
func TestSetLayout_UnknownAnchorFallsBackToZero(t *testing.T) {
	m, _ := setLayoutModelWithNames(t,
		[]string{"a", "b"},
		Item{Key: "p", Title: "p", Description: "/tmp/p", Layout: "missing"},
	)
	m = pressKey(t, m, "l")
	if got := m.setLayout.current(); got != "a" {
		t.Errorf("anchor with unknown template = %q, want a", got)
	}
}
