package picker

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// openLayoutModel returns a sized model with onOpenLayout wired to a
// stub that records the project name it was asked to open and returns
// a no-op tea.Cmd. Mirrors the editModel / modelWithDelete helpers.
func openLayoutModel(t *testing.T, items ...Item) (*model, *[]string) {
	t.Helper()
	m := sizedModel(120, items...)
	calls := []string{}
	m.onOpenLayout = func(name string) tea.Cmd {
		calls = append(calls, name)
		// Return a cmd that immediately fires editorFinishedMsg{nil}
		// so we can drive the post-editor refresh path in one update.
		return func() tea.Msg { return NewEditorFinishedMsg(nil) }
	}
	m.refreshItems = func() []Item { return items }
	return m, &calls
}

// Pressing 'o' on a project Item invokes onOpenLayout with the
// project's key and dispatches the returned tea.Cmd. The picker stays
// on screenList — the editor takes over the terminal until it exits,
// at which point editorFinishedMsg arrives separately.
func TestList_OPressInvokesOnOpenLayout(t *testing.T) {
	m, calls := openLayoutModel(t,
		Item{Key: "alpha", Title: "alpha", Description: "/tmp/a"},
	)
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	mm := updated.(*model)

	if len(*calls) != 1 || (*calls)[0] != "alpha" {
		t.Errorf("onOpenLayout calls = %v, want [alpha]", *calls)
	}
	if cmd == nil {
		t.Error("expected non-nil tea.Cmd from o press")
	}
	if mm.screen != screenList {
		t.Errorf("screen = %v, want screenList (editor handles its own UI)", mm.screen)
	}
}

// 'o' is dead-keyed when OnOpenLayout is nil — same contract as 'd'/'e'.
func TestList_OPressNoCallbackIsNoop(t *testing.T) {
	m := sizedModel(120, Item{Key: "alpha", Title: "alpha", Description: "/tmp/a"})
	// Deliberately no onOpenLayout.
	m = pressKey(t, m, "o")
	if m.screen != screenList {
		t.Errorf("screen = %v, want screenList (o dead-keyed without OnOpenLayout)", m.screen)
	}
}

// 'o' on the synthetic "+ New project" row is a no-op (no Item to open
// the layout file for).
func TestList_OPressOnNewProjectRowIsNoop(t *testing.T) {
	m, calls := openLayoutModel(t) // empty items → only synthetic row visible
	_ = pressKey(t, m, "o")
	if len(*calls) != 0 {
		t.Errorf("onOpenLayout calls = %v, want [] (synthetic row should not trigger)", *calls)
	}
}

// If onOpenLayout returns nil (e.g. because the editor wrapper hit a
// pre-flight failure it surfaced separately), the picker doesn't wedge
// — it just stays on the list.
func TestList_OPressNilCmdIsNoop(t *testing.T) {
	m := sizedModel(120, Item{Key: "alpha", Title: "alpha", Description: "/tmp/a"})
	m.onOpenLayout = func(_ string) tea.Cmd { return nil }
	m.refreshItems = func() []Item { return nil }

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	mm := updated.(*model)
	if cmd != nil {
		t.Errorf("expected nil cmd when onOpenLayout returns nil, got %T", cmd)
	}
	if mm.screen != screenList {
		t.Errorf("screen = %v, want screenList", mm.screen)
	}
}

// editorFinishedMsg{nil} triggers a refresh and stays on screenList.
// This is the success path: the user saved their edits and quit the
// editor; the picker silently picks up any layout-file changes via
// the refresh.
func TestEditorFinishedMsg_SuccessRefreshes(t *testing.T) {
	refreshCalls := 0
	m := sizedModel(120, Item{Key: "alpha", Title: "alpha", Description: "/tmp/a"})
	m.refreshItems = func() []Item {
		refreshCalls++
		return []Item{{Key: "alpha", Title: "alpha", Description: "/tmp/a"}}
	}

	updated, _ := m.Update(NewEditorFinishedMsg(nil))
	mm := updated.(*model)
	if refreshCalls != 1 {
		t.Errorf("RefreshItems called %d times, want 1", refreshCalls)
	}
	if mm.screen != screenList {
		t.Errorf("screen = %v, want screenList", mm.screen)
	}
	if mm.errMsg != "" {
		t.Errorf("errMsg = %q, want empty", mm.errMsg)
	}
}

// editorFinishedMsg{err} lands on screenError with the editor's error
// surfaced. Covers two CLI failure modes:
//   - $EDITOR not set (callback constructs the err itself, never spawns)
//   - editor process failed/crashed (tea.ExecProcess passes the err through)
func TestEditorFinishedMsg_ErrorShowsErrorScreen(t *testing.T) {
	m := sizedModel(120, Item{Key: "alpha", Title: "alpha", Description: "/tmp/a"})
	m.refreshItems = func() []Item { return nil }

	updated, _ := m.Update(NewEditorFinishedMsg(errors.New("vim segfaulted")))
	mm := updated.(*model)
	if mm.screen != screenError {
		t.Fatalf("screen = %v, want screenError", mm.screen)
	}
	if !strings.Contains(mm.errMsg, "vim segfaulted") {
		t.Errorf("errMsg = %q, want to contain 'vim segfaulted'", mm.errMsg)
	}
	// The error message is prefixed with "editor:" so the user knows
	// which subsystem barked.
	if !strings.Contains(mm.errMsg, "editor:") {
		t.Errorf("errMsg = %q, want 'editor:' prefix", mm.errMsg)
	}
}
