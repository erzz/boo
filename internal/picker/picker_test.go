package picker

import (
	"errors"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

func newTestModel(items ...Item) *model {
	listItems := make([]list.Item, len(items))
	for i, it := range items {
		listItems[i] = it
	}
	l := list.New(listItems, newDelegate(defaultTheme()), 80, 24)
	return &model{list: l, keys: defaultKeyMap()}
}

func keyMsg(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "q":
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func TestModel_EnterSelectsCurrentItem(t *testing.T) {
	m := newTestModel(
		Item{Key: "alpha", Title: "alpha"},
		Item{Key: "beta", Title: "beta"},
	)
	// First item is selected by default.
	updated, _ := m.Update(keyMsg("enter"))
	mm := updated.(*model)
	if v, ok := mm.intent.(SwitchIntent); !ok || v.Name != "alpha" {
		t.Fatalf("expected SwitchIntent{alpha}, got %#v", mm.intent)
	}
	if mm.cancelled {
		t.Fatal("should not be cancelled on enter")
	}
}

func TestModel_DownThenEnterSelectsSecond(t *testing.T) {
	m := newTestModel(
		Item{Key: "alpha", Title: "alpha"},
		Item{Key: "beta", Title: "beta"},
	)
	updated, _ := m.Update(keyMsg("down"))
	updated, _ = updated.(*model).Update(keyMsg("enter"))
	if v, ok := updated.(*model).intent.(SwitchIntent); !ok || v.Name != "beta" {
		t.Fatalf("expected SwitchIntent{beta}, got %#v", updated.(*model).intent)
	}
}

func TestModel_QCancels(t *testing.T) {
	m := newTestModel(Item{Key: "alpha", Title: "alpha"})
	updated, _ := m.Update(keyMsg("q"))
	mm := updated.(*model)
	if !mm.cancelled {
		t.Fatal("expected cancelled")
	}
	if mm.intent != nil {
		t.Fatalf("expected nil intent, got %#v", mm.intent)
	}
}

func TestModel_EscCancels(t *testing.T) {
	m := newTestModel(Item{Key: "alpha", Title: "alpha"})
	updated, _ := m.Update(keyMsg("esc"))
	if !updated.(*model).cancelled {
		t.Fatal("expected cancelled")
	}
}

func TestModel_EmptyList_EnterDoesNothing(t *testing.T) {
	m := newTestModel()
	updated, cmd := m.Update(keyMsg("enter"))
	mm := updated.(*model)
	if mm.intent != nil {
		t.Fatalf("expected nil intent, got %#v", mm.intent)
	}
	if mm.cancelled {
		t.Fatal("should not be cancelled")
	}
	// Cmd may be nil; the important thing is no quit was issued with a selection.
	_ = cmd
}

func TestRenderStatus(t *testing.T) {
	// Just smoke: doesn't crash and returns non-empty for known statuses.
	d := newDelegate(defaultTheme())
	for _, s := range []string{"running", "stopped", "dir-missing", "anything-else"} {
		if got := d.renderStatus(s); got == "" {
			t.Errorf("renderStatus(%q) was empty", s)
		}
	}
	if got := d.renderStatus(""); got != "" {
		t.Errorf("renderStatus(\"\") = %q, want empty", got)
	}
}

// Bare `boo` (list-first flows) must NEVER short-circuit to the
// "this directory is already registered" interstitial. The interstitial
// only answers the question "you asked to create a new project here,
// but this dir already has one — switch or continue?", which is
// nonsensical when the user just wants their project list.
//
// `boo new` and `boo save`'s form fallback (formOnly=true) DO want the
// interstitial when a registered dir is detected, so the bare case
// has to be selectively skipped without breaking those flows.
//
// This was a real regression: a previous implementation gated the
// interstitial purely on AlreadyRegisteredAs being non-empty, which
// meant bare `boo` from inside any registered dir hit the interstitial
// before showing the list. Pin the precedence rule so it can't drift.
func TestInitialScreen_BareBooSkipsInterstitial(t *testing.T) {
	tests := []struct {
		name                string
		formOnly            bool
		alreadyRegisteredAs string
		want                screen
	}{
		{
			name:                "bare boo in registered dir lands on the list",
			formOnly:            false,
			alreadyRegisteredAs: "alpha",
			want:                screenList,
		},
		{
			name:                "bare boo in unregistered dir lands on the list",
			formOnly:            false,
			alreadyRegisteredAs: "",
			want:                screenList,
		},
		{
			name:                "boo new in registered dir hits the interstitial",
			formOnly:            true,
			alreadyRegisteredAs: "alpha",
			want:                screenAlreadyRegistered,
		},
		{
			name:                "boo new in unregistered dir lands on the form",
			formOnly:            true,
			alreadyRegisteredAs: "",
			want:                screenForm,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := initialScreen(tt.formOnly, tt.alreadyRegisteredAs); got != tt.want {
				t.Errorf("initialScreen(formOnly=%v, alreadyRegisteredAs=%q) = %v, want %v",
					tt.formOnly, tt.alreadyRegisteredAs, got, tt.want)
			}
		})
	}
}

// Fix 1: RefreshItems error must leave items intact; empty slice must clear.

// TestRefreshList_ErrorPreservesItems verifies that when the RefreshItems
// callback returns an error the existing list contents are kept unchanged.
// This is the critical invariant: a transient registry read failure must
// not wipe out the items the user is currently looking at.
func TestRefreshList_ErrorPreservesItems(t *testing.T) {
	alpha := Item{Key: "alpha", Title: "alpha"}
	beta := Item{Key: "beta", Title: "beta"}
	m := newTestModel(alpha, beta)

	callCount := 0
	m.refreshItems = func() ([]Item, error) {
		callCount++
		return nil, errors.New("registry unavailable")
	}

	m.refreshList()

	if callCount != 1 {
		t.Fatalf("refreshItems called %d times, want 1", callCount)
	}
	// Both original items should still be visible.
	got := m.list.Items()
	if len(got) != 2 {
		t.Fatalf("list has %d items after error refresh, want 2 (items must be preserved)", len(got))
	}
	if it, ok := got[0].(Item); !ok || it.Key != "alpha" {
		t.Errorf("item[0] = %v, want alpha", got[0])
	}
	if it, ok := got[1].(Item); !ok || it.Key != "beta" {
		t.Errorf("item[1] = %v, want beta", got[1])
	}
}

// TestRefreshList_EmptySliceShowsEmptyState verifies that a nil error
// with an empty (or nil) item slice updates the list to empty. This is
// distinct from an error: it is the correct outcome when the last project
// has been deleted.
func TestRefreshList_EmptySliceShowsEmptyState(t *testing.T) {
	alpha := Item{Key: "alpha", Title: "alpha"}
	m := newTestModel(alpha)
	m.hideNewProject = true // suppress the synthetic "+ New project" entry

	m.refreshItems = func() ([]Item, error) {
		return []Item{}, nil // success, but nothing left
	}

	m.refreshList()

	got := m.list.Items()
	if len(got) != 0 {
		t.Fatalf("list has %d items after empty-slice refresh, want 0", len(got))
	}
}

// TestRefreshList_NilSliceSuccessShowsEmptyState verifies that a nil
// slice returned alongside a nil error also clears the list (nil == empty
// in the "no projects remaining" sense).
func TestRefreshList_NilSliceSuccessShowsEmptyState(t *testing.T) {
	alpha := Item{Key: "alpha", Title: "alpha"}
	m := newTestModel(alpha)
	m.hideNewProject = true

	m.refreshItems = func() ([]Item, error) {
		return nil, nil // success, intentionally empty
	}

	m.refreshList()

	got := m.list.Items()
	if len(got) != 0 {
		t.Fatalf("list has %d items after nil-slice-success refresh, want 0", len(got))
	}
}

// Fix 2: delete with purge + window-close failure must surface the warning.

// TestRunIntent_DeletePurge_WindowCloseWarning verifies that when the
// onDelete callback returns a non-empty warning string (window close
// failed) the status bar reflects it rather than claiming success.
func TestRunIntent_DeletePurge_WindowCloseWarning(t *testing.T) {
	alpha := Item{Key: "alpha", Title: "alpha"}
	m := newTestModel(alpha)
	m.onDelete = func(name string, purge bool) (string, error) {
		return "could not close window w1: connection refused", nil
	}
	m.refreshItems = func() ([]Item, error) { return []Item{}, nil }

	updated, _ := m.runIntent(DeleteIntent{Name: "alpha", Purge: true})
	mm := updated.(*model)

	if mm.status.isErr {
		t.Fatal("status should not be an error (deletion succeeded)")
	}
	if mm.status.text == "" {
		t.Fatal("status text should be non-empty")
	}
	const want = "window close failed"
	if !strings.Contains(mm.status.text, want) {
		t.Errorf("status = %q, want it to contain %q", mm.status.text, want)
	}
}

// TestRunIntent_DeletePurge_NoWarning verifies the happy path: when
// purge succeeds without a warning the status says "deleted … and closed window".
func TestRunIntent_DeletePurge_NoWarning(t *testing.T) {
	alpha := Item{Key: "alpha", Title: "alpha"}
	m := newTestModel(alpha)
	m.onDelete = func(name string, purge bool) (string, error) {
		return "", nil // success, no warning
	}
	m.refreshItems = func() ([]Item, error) { return []Item{}, nil }

	updated, _ := m.runIntent(DeleteIntent{Name: "alpha", Purge: true})
	mm := updated.(*model)

	if mm.status.isErr {
		t.Fatal("status should not be an error")
	}
	const want = "and closed window"
	if !strings.Contains(mm.status.text, want) {
		t.Errorf("status = %q, want it to contain %q", mm.status.text, want)
	}
}

// TestRunIntent_Delete_ErrorPreservesItems verifies that a failed delete
// shows an error screen and does NOT call refresh (so items are preserved).
func TestRunIntent_Delete_ErrorPreservesItems(t *testing.T) {
	alpha := Item{Key: "alpha", Title: "alpha"}
	m := newTestModel(alpha)
	refreshCalled := false
	m.onDelete = func(name string, purge bool) (string, error) {
		return "", errors.New("registry locked")
	}
	m.refreshItems = func() ([]Item, error) {
		refreshCalled = true
		return []Item{}, nil
	}

	updated, _ := m.runIntent(DeleteIntent{Name: "alpha", Purge: false})
	mm := updated.(*model)

	if mm.screen != screenError {
		t.Errorf("screen = %v, want screenError", mm.screen)
	}
	if refreshCalled {
		t.Error("refreshItems must not be called when onDelete returns an error")
	}
}
