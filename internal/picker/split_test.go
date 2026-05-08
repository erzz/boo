package picker

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

// helper: build a model already sized to the given width.
func sizedModel(width int, items ...Item) *model {
	m := newTestModel(items...)
	m.theme = defaultTheme()
	updated, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: 24})
	return updated.(*model)
}

func TestSplit_NarrowTerminalCollapses(t *testing.T) {
	m := sizedModel(60, Item{Key: "alpha", Title: "alpha", Description: "/x/alpha"})
	if m.splitActive() {
		t.Fatal("split should be inactive at width=60 (below threshold)")
	}
	v := m.View()
	// In narrow mode the list pane still has its own border, but the
	// right pane is suppressed. Distinguishing feature: the right
	// pane's fallback content includes the project's path/directory
	// in label form ("Directory" header + path), and that should NOT
	// appear when no right pane is rendered.
	if strings.Contains(v, "Directory") {
		t.Errorf("narrow view should not render right-pane Directory label, got:\n%s", v)
	}
	// The list itself (single pane) should still render the project.
	if !strings.Contains(v, "alpha") {
		t.Errorf("narrow view missing project title, got:\n%s", v)
	}
}

func TestSplit_WideTerminalRendersBothPanes(t *testing.T) {
	m := sizedModel(120, Item{Key: "alpha", Title: "alpha", Description: "/x/alpha"})
	if !m.splitActive() {
		t.Fatal("split should be active at width=120")
	}
	v := m.View()
	// The list pane renders the project title; the right pane (no
	// PreviewProject callback) renders the fallback summary which
	// includes the project's directory.
	if !strings.Contains(v, "alpha") {
		t.Errorf("expected project name in view, got:\n%s", v)
	}
	if !strings.Contains(v, "/x/alpha") {
		t.Errorf("expected project dir in right pane fallback, got:\n%s", v)
	}
	if !strings.ContainsAny(v, "╭╮╰╯") {
		t.Errorf("expected rounded border in right pane, got:\n%s", v)
	}
}

func TestSplit_RightPaneUsesPreviewCallback(t *testing.T) {
	const sentinel = "PREVIEW-CALLBACK-SENTINEL"
	called := ""
	m := newTestModel(Item{Key: "alpha", Title: "alpha"})
	m.theme = defaultTheme()
	m.previewProject = func(name string) string {
		called = name
		return sentinel
	}
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 24})
	v := updated.View()
	if called != "alpha" {
		t.Errorf("preview callback called with %q, want alpha", called)
	}
	if !strings.Contains(v, sentinel) {
		t.Errorf("right pane should contain preview output %q, got:\n%s", sentinel, v)
	}
}

func TestSplit_NewProjectRowShowsHint(t *testing.T) {
	m := newTestModel() // empty list — but we still need newProjectItem
	// Manually inject the synthetic row the way Run() does.
	m.list.SetItems([]list.Item{newProjectItem{}})
	m.theme = defaultTheme()
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 24})
	v := updated.View()
	if !strings.Contains(v, "+ New project") {
		t.Errorf("right pane should mention + New project, got:\n%s", v)
	}
}
