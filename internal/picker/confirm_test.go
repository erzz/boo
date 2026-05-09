package picker

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// errFake is a sentinel used by tests that exercise the picker's failure / showError path.
var errFake = errors.New("fake")

// pressKey drives a single key through model.Update.
func pressKey(t *testing.T, m *model, s string) *model {
	t.Helper()
	var msg tea.KeyMsg
	switch s {
	case "enter":
		msg = tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		msg = tea.KeyMsg{Type: tea.KeyEsc}
	default:
		msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
	updated, _ := m.Update(msg)
	return updated.(*model)
}

// modelWithDelete returns a model with a stub delete callback; calls recorded in returned slice.
func modelWithDelete(items ...Item) (*model, *[]string) {
	m := sizedModel(120, items...)
	calls := []string{}
	m.onDelete = func(name string, purge bool) ([]string, error) {
		tag := name
		if purge {
			tag += "+purge"
		}
		calls = append(calls, tag)
		return nil, nil
	}
	m.refreshItems = func() ([]Item, error) { return items, nil } // no-op refresh
	return m, &calls
}

// TestList_DPressOpensDeleteConfirm: 'd' opens the confirm modal with a DeleteIntent for the selected project.
func TestList_DPressOpensDeleteConfirm(t *testing.T) {
	m, _ := modelWithDelete(
		Item{Key: "alpha", Title: "alpha", Description: "/tmp/alpha"},
		Item{Key: "beta", Title: "beta", Description: "/tmp/beta"},
	)
	m = pressKey(t, m, "d")

	if m.screen != screenConfirm {
		t.Fatalf("screen = %v, want screenConfirm", m.screen)
	}
	di, ok := m.confirm.pending.(DeleteIntent)
	if !ok {
		t.Fatalf("pending intent type = %T, want DeleteIntent", m.confirm.pending)
	}
	if di.Name != "alpha" || di.Purge {
		t.Errorf("pending = %+v, want {alpha false}", di)
	}
	v := m.View()
	if !strings.Contains(v, "alpha") {
		t.Errorf("modal view missing project name:\n%s", v)
	}
	if !strings.Contains(v, "Delete project") {
		t.Errorf("modal view missing title:\n%s", v)
	}
}

// TestList_ShiftDPressOpensPurgeConfirm: 'D' opens the modal with Purge:true and "close window" title.
func TestList_ShiftDPressOpensPurgeConfirm(t *testing.T) {
	m, _ := modelWithDelete(
		Item{Key: "alpha", Title: "alpha", Description: "/tmp/alpha"},
	)
	m = pressKey(t, m, "D")
	di, ok := m.confirm.pending.(DeleteIntent)
	if !ok {
		t.Fatalf("pending intent type = %T, want DeleteIntent", m.confirm.pending)
	}
	if !di.Purge {
		t.Error("Purge = false, want true")
	}
	if !strings.Contains(m.View(), "close window") {
		t.Errorf("purge modal missing 'close window' text:\n%s", m.View())
	}
}

// TestConfirm_YesInvokesCallback: confirming the modal calls OnDelete and returns to the list.
func TestConfirm_YesInvokesCallback(t *testing.T) {
	m, calls := modelWithDelete(Item{Key: "alpha", Title: "alpha", Description: "/tmp/alpha"})
	m = pressKey(t, m, "d")
	m = pressKey(t, m, "y")

	if len(*calls) != 1 || (*calls)[0] != "alpha" {
		t.Errorf("OnDelete calls = %v, want [alpha]", *calls)
	}
	if m.screen != screenList {
		t.Errorf("screen after success = %v, want screenList", m.screen)
	}
	if m.intent != nil {
		t.Errorf("intent = %v, want nil (handled in loop)", m.intent)
	}
}

// TestConfirm_YesErrorShowsErrorScreen: OnDelete error → screenError (not silent success).
func TestConfirm_YesErrorShowsErrorScreen(t *testing.T) {
	m := sizedModel(120, Item{Key: "alpha", Title: "alpha", Description: "/tmp/alpha"})
	m.onDelete = func(name string, purge bool) ([]string, error) { return nil, errFake }
	m.refreshItems = func() ([]Item, error) { return nil, nil }

	m = pressKey(t, m, "d")
	m = pressKey(t, m, "y")

	if m.screen != screenError {
		t.Fatalf("screen = %v, want screenError", m.screen)
	}
	if !strings.Contains(m.View(), "fake") {
		t.Errorf("error view missing error text:\n%s", m.View())
	}
	// Any keypress dismisses.
	m = pressKey(t, m, "x")
	if m.screen != screenList {
		t.Errorf("screen after dismiss = %v, want screenList", m.screen)
	}
}

// TestConfirm_NoReturnsToList: cancelling the modal returns to screenList with no intent.
func TestConfirm_NoReturnsToList(t *testing.T) {
	m, _ := modelWithDelete(Item{Key: "alpha", Title: "alpha", Description: "/tmp/alpha"})
	m = pressKey(t, m, "d")
	m = pressKey(t, m, "esc")

	if m.screen != screenList {
		t.Errorf("screen = %v, want screenList", m.screen)
	}
	if m.intent != nil {
		t.Errorf("intent = %v, want nil", m.intent)
	}
	if m.confirm.pending != nil {
		t.Error("modal not cleared on cancel")
	}
}

// TestList_DPressOnNewProjectRowIsNoop: 'd' on "+ New project" row is a no-op.
func TestList_DPressOnNewProjectRowIsNoop(t *testing.T) {
	m, _ := modelWithDelete() // no items → only the synthetic row is selectable
	m = pressKey(t, m, "d")
	if m.screen != screenList {
		t.Errorf("screen = %v, want screenList (d on synthetic row should be no-op)", m.screen)
	}
}

// TestList_DPressNoCallbackIsNoop: 'd' is dead-keyed when OnDelete is nil.
func TestList_DPressNoCallbackIsNoop(t *testing.T) {
	m := sizedModel(120, Item{Key: "alpha", Title: "alpha", Description: "/tmp/alpha"})
	// Deliberately no onDelete.
	m = pressKey(t, m, "d")
	if m.screen != screenList {
		t.Errorf("screen = %v, want screenList (d should be dead-keyed when OnDelete unwired)", m.screen)
	}
	if m.confirm.pending != nil {
		t.Error("confirm modal opened despite missing callback")
	}
}
