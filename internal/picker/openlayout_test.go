package picker

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// openLayoutModel returns a sized model with onOpenLayout stubbed to record calls and return editorFinishedMsg.
// Mirrors the editModel / modelWithDelete helpers.
func openLayoutModel(t *testing.T, items ...Item) (*model, *[]string) {
	t.Helper()
	m := sizedModel(120, items...)
	calls := []string{}
	m.onOpenLayout = func(name string) tea.Cmd {
		calls = append(calls, name)
		// Return editorFinishedMsg so the post-editor refresh path is drivable in one update.
		return func() tea.Msg { return NewEditorFinishedMsg(nil) }
	}
	m.refreshItems = func() ([]Item, error) { return items, nil }
	return m, &calls
}

// TestList_OPressInvokesOnOpenLayout: 'o' on a project Item calls onOpenLayout with its key
// and dispatches the returned cmd. Picker stays on screenList (editor handles its own UI).
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

// TestList_OPressNoCallbackIsNoop: 'o' is dead-keyed when OnOpenLayout is nil (same as 'd'/'e').
func TestList_OPressNoCallbackIsNoop(t *testing.T) {
	m := sizedModel(120, Item{Key: "alpha", Title: "alpha", Description: "/tmp/a"})
	// Deliberately no onOpenLayout.
	m = pressKey(t, m, "o")
	if m.screen != screenList {
		t.Errorf("screen = %v, want screenList (o dead-keyed without OnOpenLayout)", m.screen)
	}
}

// TestList_OPressOnNewProjectRowIsNoop: 'o' on the synthetic "+ New project" row is a no-op.
func TestList_OPressOnNewProjectRowIsNoop(t *testing.T) {
	m, calls := openLayoutModel(t) // empty items → only synthetic row visible
	_ = pressKey(t, m, "o")
	if len(*calls) != 0 {
		t.Errorf("onOpenLayout calls = %v, want [] (synthetic row should not trigger)", *calls)
	}
}

// TestList_OPressNilCmdIsNoop: nil cmd from onOpenLayout → picker stays on screenList, no wedge.
func TestList_OPressNilCmdIsNoop(t *testing.T) {
	m := sizedModel(120, Item{Key: "alpha", Title: "alpha", Description: "/tmp/a"})
	m.onOpenLayout = func(_ string) tea.Cmd { return nil }
	m.refreshItems = func() ([]Item, error) { return nil, nil }

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	mm := updated.(*model)
	if cmd != nil {
		t.Errorf("expected nil cmd when onOpenLayout returns nil, got %T", cmd)
	}
	if mm.screen != screenList {
		t.Errorf("screen = %v, want screenList", mm.screen)
	}
}

// TestEditorFinishedMsg_SuccessRefreshes: editorFinishedMsg{nil} triggers refresh, stays on screenList.
func TestEditorFinishedMsg_SuccessRefreshes(t *testing.T) {
	refreshCalls := 0
	m := sizedModel(120, Item{Key: "alpha", Title: "alpha", Description: "/tmp/a"})
	m.refreshItems = func() ([]Item, error) {
		refreshCalls++
		return []Item{{Key: "alpha", Title: "alpha", Description: "/tmp/a"}}, nil
	}

	updated, cmd := m.Update(NewEditorFinishedMsg(nil))
	mm := updated.(*model)
	// startEnrich returns a tea.Cmd that calls RefreshItems async; execute manually to assert synchronously.
	if cmd != nil {
		cmd()
	}
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

// TestEditorFinishedMsg_ErrorShowsErrorScreen: editorFinishedMsg{err} → screenError with error text.
// Covers: $EDITOR not set (err constructed before spawn) and editor process crash (err from tea.ExecProcess).
func TestEditorFinishedMsg_ErrorShowsErrorScreen(t *testing.T) {
	m := sizedModel(120, Item{Key: "alpha", Title: "alpha", Description: "/tmp/a"})
	m.refreshItems = func() ([]Item, error) { return nil, nil }

	updated, _ := m.Update(NewEditorFinishedMsg(errors.New("vim segfaulted")))
	mm := updated.(*model)
	if mm.screen != screenError {
		t.Fatalf("screen = %v, want screenError", mm.screen)
	}
	if !strings.Contains(mm.errMsg, "vim segfaulted") {
		t.Errorf("errMsg = %q, want to contain 'vim segfaulted'", mm.errMsg)
	}
	// errMsg must carry "editor:" prefix so the user knows which subsystem barked.
	if !strings.Contains(mm.errMsg, "editor:") {
		t.Errorf("errMsg = %q, want 'editor:' prefix", mm.errMsg)
	}
}
