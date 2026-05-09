package picker

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestStatusBar_IdleShowsHint: idle status bar shows "press ? for help", not empty string.
func TestStatusBar_IdleShowsHint(t *testing.T) {
	m := sizedModel(120, Item{Key: "alpha", Title: "alpha", Description: "/x/alpha"})
	v := m.View()
	if !strings.Contains(v, "press ? for help") {
		t.Errorf("idle status bar missing hint, got:\n%s", v)
	}
}

// TestStatusBar_DeleteSuccessSetsStatus: successful delete writes "deleted <name>" to status bar.
func TestStatusBar_DeleteSuccessSetsStatus(t *testing.T) {
	m, _ := modelWithDelete(Item{Key: "alpha", Title: "alpha", Description: "/x/alpha"})
	m = pressKey(t, m, "d")
	m = pressKey(t, m, "y")
	if m.status.text != "deleted alpha" {
		t.Errorf("status.text = %q, want 'deleted alpha'", m.status.text)
	}
	if m.status.isErr {
		t.Error("status.isErr = true, want false (success)")
	}
	if !strings.Contains(m.View(), "deleted alpha") {
		t.Errorf("View missing status text, got:\n%s", m.View())
	}
}

// TestStatusBar_PurgeSuccessReflectsWindowClose: purge delete shows "closed window" in status.
func TestStatusBar_PurgeSuccessReflectsWindowClose(t *testing.T) {
	m, _ := modelWithDelete(Item{Key: "alpha", Title: "alpha", Description: "/x/alpha"})
	m = pressKey(t, m, "D")
	m = pressKey(t, m, "y")
	if !strings.Contains(m.status.text, "closed window") {
		t.Errorf("purge status missing 'closed window', got %q", m.status.text)
	}
}

// TestStatusBar_FailureSetsErrorStatus: failed action → screenError + isErr=true; persists after dismiss.
func TestStatusBar_FailureSetsErrorStatus(t *testing.T) {
	m := sizedModel(120, Item{Key: "alpha", Title: "alpha", Description: "/x/alpha"})
	m.onDelete = func(_ string, _ bool) ([]string, error) { return nil, errFake }
	m.refreshItems = func() ([]Item, error) { return nil, nil }

	m = pressKey(t, m, "d")
	m = pressKey(t, m, "y")
	// Error screen is up; status is also primed.
	if !m.status.isErr {
		t.Error("status.isErr = false after failed delete, want true")
	}
	if !strings.Contains(m.status.text, "fake") {
		t.Errorf("status.text = %q, expected to contain 'fake'", m.status.text)
	}
	// Dismiss the error screen — status bar should still reflect the failure.
	m = pressKey(t, m, "x")
	if m.screen != screenList {
		t.Errorf("screen after dismiss = %v, want screenList", m.screen)
	}
	if !m.status.isErr {
		t.Error("status.isErr cleared after error dismissal, should persist")
	}
	if !strings.Contains(m.View(), "fake") {
		t.Errorf("View after dismissal missing failure text, got:\n%s", m.View())
	}
}

// TestStatusBar_EditorSuccessSetsStatus: editorFinishedMsg with nil error sets status to "layout file saved".
func TestStatusBar_EditorSuccessSetsStatus(t *testing.T) {
	m := sizedModel(120, Item{Key: "alpha", Title: "alpha", Description: "/x/alpha"})
	m.refreshItems = func() ([]Item, error) {
		return []Item{{Key: "alpha", Title: "alpha", Description: "/x/alpha"}}, nil
	}
	updated, _ := m.Update(NewEditorFinishedMsg(nil))
	mm := updated.(*model)
	if mm.status.text != "layout file saved" {
		t.Errorf("status.text = %q, want 'layout file saved'", mm.status.text)
	}
	if mm.status.isErr {
		t.Error("status.isErr = true, want false (success)")
	}
}

// TestList_RowsAreSingleLine: Description (path) must not appear in the list (right-pane only). Confirms Height()=1.
func TestList_RowsAreSingleLine(t *testing.T) {
	m := sizedModel(60, // narrow → right pane suppressed, list-only view
		Item{Key: "alpha", Title: "alpha", Description: "/x/SECRET-PATH-MARKER"},
	)
	v := m.View()
	if strings.Contains(v, "SECRET-PATH-MARKER") {
		t.Errorf("list row still rendering Description (path); should be right-pane only, got:\n%s", v)
	}
	if !strings.Contains(v, "alpha") {
		t.Errorf("list missing project title, got:\n%s", v)
	}
}

// TestSplit_LowerThreshold: width=90 activates split mode.
// Threshold raised from 70 after real-world 83-col panes showed content overflow.
func TestSplit_LowerThreshold(t *testing.T) {
	m := sizedModel(90, Item{Key: "alpha", Title: "alpha"})
	if !m.splitActive() {
		t.Errorf("split should be active at width=90 (current threshold)")
	}
}

// TestSplit_BelowThresholdInactive: width=89 (one below) is non-split.
func TestSplit_BelowThresholdInactive(t *testing.T) {
	m := sizedModel(89, Item{Key: "alpha", Title: "alpha"})
	if m.splitActive() {
		t.Errorf("split should be inactive at width=89 (one below threshold)")
	}
}

// TestSplit_ShortTerminalCollapses: a wide-but-short pane (e.g. boo
// launched into a Ghostty pane that's split horizontally so the height
// is constrained) should drop to single-pane. The right-pane content
// needs ~20 rows to render without ugly clipping.
func TestSplit_ShortTerminalCollapses(t *testing.T) {
	m := newTestModel(Item{Key: "alpha", Title: "alpha"})
	m.theme = defaultTheme()
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 20})
	mm := updated.(*model)
	if mm.splitActive() {
		t.Errorf("split should be inactive at height=20 (below splitMinHeight=24)")
	}
}

// TestSplit_HeightAtThreshold: height=24 is the floor for split mode.
func TestSplit_HeightAtThreshold(t *testing.T) {	m := newTestModel(Item{Key: "alpha", Title: "alpha"})
	m.theme = defaultTheme()
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 24})
	mm := updated.(*model)
	if !mm.splitActive() {
		t.Errorf("split should be active at width=120, height=24 (both at/above thresholds)")
	}
}

// TestSplit_HeightOneBelowThresholdInactive: height=23 (one below splitMinHeight) suppresses split mode.
func TestSplit_HeightOneBelowThresholdInactive(t *testing.T) {
	m := newTestModel(Item{Key: "alpha", Title: "alpha"})
	m.theme = defaultTheme()
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 23})
	mm := updated.(*model)
	if mm.splitActive() {
		t.Errorf("split should be inactive at height=23 (one below splitMinHeight=24)")
	}
}

// TestStatusBar_LeavesRoomForList: at H=24 the list inner area is 24-1-2=21 (statusBarHeight + listBorderOverhead).
// Smoke-checks that viewList doesn't crash and the math doesn't produce negative space.
func TestStatusBar_LeavesRoomForList(t *testing.T) {
	m := sizedModel(120, Item{Key: "alpha", Title: "alpha"})
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 24})
	mm := updated.(*model)
	if mm.height != 24 {
		t.Errorf("model.height = %d, want 24", mm.height)
	}
	// Smoke-check: non-empty render at H=24 means the math doesn't wrap into negative territory.
	if v := mm.View(); v == "" {
		t.Error("View() returned empty at H=24, status bar math may have starved the list")
	}
}
