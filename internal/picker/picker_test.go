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

func TestModel_EscNoOpOnMainList(t *testing.T) {
	// esc was removed from the Quit binding (Fix 2). On the main list
	// screen pressing esc should be a no-op — neither cancelling the
	// picker nor setting any intent.
	m := newTestModel(Item{Key: "alpha", Title: "alpha"})
	updated, _ := m.Update(keyMsg("esc"))
	mm := updated.(*model)
	if mm.cancelled {
		t.Fatal("esc must not cancel on the main list screen (Fix 2: only q / ctrl-c quit)")
	}
	if mm.intent != nil {
		t.Fatalf("esc must not set an intent on the main list screen, got %#v", mm.intent)
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
// onDelete callback returns a non-empty warnings slice (window close
// failed) the status bar reflects it rather than claiming success.
func TestRunIntent_DeletePurge_WindowCloseWarning(t *testing.T) {
	alpha := Item{Key: "alpha", Title: "alpha"}
	m := newTestModel(alpha)
	m.onDelete = func(name string, purge bool) ([]string, error) {
		return []string{"could not close window w1: connection refused"}, nil
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
	// Single warning: status should contain the warning text.
	const want = "could not close"
	if !strings.Contains(mm.status.text, want) {
		t.Errorf("status = %q, want it to contain %q", mm.status.text, want)
	}
}

// TestRunIntent_DeletePurge_NoWarning verifies the happy path: when
// purge succeeds without a warning the status says "deleted … and closed window".
func TestRunIntent_DeletePurge_NoWarning(t *testing.T) {
	alpha := Item{Key: "alpha", Title: "alpha"}
	m := newTestModel(alpha)
	m.onDelete = func(name string, purge bool) ([]string, error) {
		return nil, nil // success, no warnings
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
	m.onDelete = func(name string, purge bool) ([]string, error) {
		return nil, errors.New("registry locked")
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

// Fix 1 (extended): Both window-close AND state-dir purge warnings must be
// surfaced when the picker uses the in-loop delete callback.
//
// This test verifies the critical gap the reviewer identified: the picker
// was previously discarding state-dir purge failures via io.Discard. With
// the []string return type every non-fatal failure is visible.
func TestRunIntent_Delete_MultipleWarnings(t *testing.T) {
	alpha := Item{Key: "alpha", Title: "alpha"}
	m := newTestModel(alpha)
	m.onDelete = func(name string, purge bool) ([]string, error) {
		return []string{
			"could not close window w1: connection refused",
			"removed from registry but could not purge state dir: permission denied",
		}, nil
	}
	m.refreshItems = func() ([]Item, error) { return []Item{}, nil }

	updated, _ := m.runIntent(DeleteIntent{Name: "alpha", Purge: true})
	mm := updated.(*model)

	if mm.status.isErr {
		t.Fatal("status should not be an error (deletion succeeded despite side-effect failures)")
	}
	// Multiple warnings: status must inline the first warning and show a
	// "+N more" suffix so TUI users see at least one concrete message
	// (slog output is not visible inside the alt-screen).
	const wantFirstWarn = "could not close"
	const wantMoreSuffix = "+1 more"
	if !strings.Contains(mm.status.text, wantFirstWarn) {
		t.Errorf("status = %q, want it to contain first warning %q", mm.status.text, wantFirstWarn)
	}
	if !strings.Contains(mm.status.text, wantMoreSuffix) {
		t.Errorf("status = %q, want it to contain %q (so users know there are additional warnings)", mm.status.text, wantMoreSuffix)
	}
}

// ─── Fix 3: Async picker startup ─────────────────────────────────────────────

// TestInit_ReturnsEnrichCmdWhenRefreshSet verifies that Init() returns a
// non-nil tea.Cmd when a RefreshItems callback is configured. The cmd is the
// async enrichment kick-off; nil would mean the picker never enriches items.
func TestInit_ReturnsEnrichCmdWhenRefreshSet(t *testing.T) {
	m := newTestModel(Item{Key: "alpha", Title: "alpha"})
	m.refreshItems = func() ([]Item, error) { return nil, nil }

	cmd := m.Init()
	if cmd == nil {
		t.Fatal("Init() must return a non-nil cmd when refreshItems is set")
	}
}

// TestInit_NilWhenNoRefreshItems verifies that Init() returns nil (no-op) when
// no RefreshItems callback is configured — e.g. in the delete-picker which
// uses pre-enriched items.
func TestInit_NilWhenNoRefreshItems(t *testing.T) {
	m := newTestModel(Item{Key: "alpha", Title: "alpha"})
	// m.refreshItems intentionally nil

	cmd := m.Init()
	if cmd != nil {
		t.Fatal("Init() must return nil when refreshItems is not set")
	}
}

// TestEnrichedItemsMsg_UpdatesItemList verifies that when an enrichedItemsMsg
// arrives the model replaces the current item list with the enriched items.
// This is the core of the async startup enrichment path.
func TestEnrichedItemsMsg_UpdatesItemList(t *testing.T) {
	// Start with a bare item (no Status — as if built by buildBareItems).
	bare := Item{Key: "alpha", Title: "alpha", Status: ""}
	m := newTestModel(bare)
	m.hideNewProject = true

	// Send the enrichedItemsMsg with a status-enriched version.
	enriched := []Item{{Key: "alpha", Title: "alpha", Status: "running"}}
	updated, _ := m.Update(enrichedItemsMsg{items: enriched, err: nil})
	mm := updated.(*model)

	got := mm.list.Items()
	if len(got) != 1 {
		t.Fatalf("expected 1 item after enrichment, got %d", len(got))
	}
	it, ok := got[0].(Item)
	if !ok {
		t.Fatalf("expected Item, got %T", got[0])
	}
	if it.Status != "running" {
		t.Errorf("Status = %q after enrichment, want %q", it.Status, "running")
	}
}

// TestEnrichedItemsMsg_ErrorPreservesItems verifies that an enrichedItemsMsg
// carrying an error leaves the existing item list intact.
func TestEnrichedItemsMsg_ErrorPreservesItems(t *testing.T) {
	alpha := Item{Key: "alpha", Title: "alpha", Status: "stopped"}
	m := newTestModel(alpha)
	m.hideNewProject = true

	updated, _ := m.Update(enrichedItemsMsg{err: errors.New("ghostty unreachable")})
	mm := updated.(*model)

	got := mm.list.Items()
	if len(got) != 1 {
		t.Fatalf("expected original 1 item preserved on error, got %d", len(got))
	}
}

// TestView_DoesNotMutateModel verifies that View() is pure: calling it
// multiple times must not change any model fields (screen, status, etc.).
// This is the Bubble Tea contract: View must be a pure projection of model
// state, never a place where side effects sneak in.
func TestView_DoesNotMutateModel(t *testing.T) {
	m := newTestModel(
		Item{Key: "alpha", Title: "alpha", Status: "stopped"},
		Item{Key: "beta", Title: "beta", Status: "running"},
	)
	// Give the model plausible dimensions so View() takes the common code
	// path (bordered panes + status bar).
	m.width = 120
	m.height = 40

	screenBefore := m.screen
	statusBefore := m.status

	_ = m.View()
	_ = m.View()
	_ = m.View()

	if m.screen != screenBefore {
		t.Errorf("View() mutated screen: %v → %v", screenBefore, m.screen)
	}
	if m.status != statusBefore {
		t.Errorf("View() mutated status: before=%v after=%v", statusBefore, m.status)
	}
}

// ─── Must-fix 1: View() must not invoke the PreviewProject callback ───────────

// TestView_DoesNotInvokePreviewCallback asserts that View() is I/O-free: the
// PreviewProject callback must never be called directly from within View(). The
// preview is dispatched as a tea.Cmd (from startPreview / Init) and applied in
// Update; View() only reads from the previewCache.
//
// This test would have caught the original bug: previewProject was called
// synchronously inside renderItemDetail → viewRightPane → viewList → View().
func TestView_DoesNotInvokePreviewCallback(t *testing.T) {
	m := newTestModel(
		Item{Key: "alpha", Title: "alpha", Status: "stopped"},
	)
	m.width = 120
	m.height = 40

	called := false
	m.previewProject = func(name string) string {
		called = true
		t.Errorf("previewProject was called directly from View() for project %q — View() must be I/O-free", name)
		return "preview"
	}

	// Init() returns a tea.Cmd that would eventually invoke previewProject
	// asynchronously — but we do not execute it here. We just verify that
	// calling View() directly does NOT invoke the callback.
	_ = m.Init()
	_ = m.View()
	_ = m.View()

	if called {
		t.Error("View() invoked the previewProject callback — View() must only read from previewCache")
	}
}

// ─── Must-fix 2: Stale enrichment results must be discarded ──────────────────

// TestEnrichment_OldResultIgnored verifies that when two enrichment rounds
// are started and the older one finishes last, its result is silently dropped
// and the model retains the original (unenriched) state until the newer result
// arrives.
//
// Concrete failure scenario: user deletes a project quickly after launch;
// the post-delete refresh removes it, then the slower startup enrichment
// arrives and reinserts it. This test would fail on the pre-fix code.
func TestEnrichment_OldResultIgnored(t *testing.T) {
	bare := Item{Key: "alpha", Title: "alpha", Status: ""}
	m := newTestModel(bare)
	m.hideNewProject = true
	m.refreshItems = func() ([]Item, error) { return nil, nil }

	// Simulate two in-flight enrichments by calling startEnrich twice.
	// After the second call m.enrichGen == 2.
	_ = m.startEnrich() // gen=1 — slow startup enrichment
	_ = m.startEnrich() // gen=2 — faster post-action refresh

	// Deliver gen=1's result (stale — enrichGen is already 2).
	staleItems := []Item{{Key: "alpha", Title: "alpha", Status: "stale-running"}}
	updated, _ := m.Update(enrichedItemsMsg{items: staleItems, gen: 1})
	mm := updated.(*model)

	got := mm.list.Items()
	if len(got) != 1 {
		t.Fatalf("expected 1 item, got %d", len(got))
	}
	it, ok := got[0].(Item)
	if !ok {
		t.Fatalf("expected Item, got %T", got[0])
	}
	if it.Status == "stale-running" {
		t.Error("stale enrichment (gen=1) was applied even though enrichGen=2 — must be discarded")
	}

	// Now deliver gen=2's result (fresh) — must be applied.
	freshItems := []Item{{Key: "alpha", Title: "alpha", Status: "running"}}
	updated2, _ := mm.Update(enrichedItemsMsg{items: freshItems, gen: 2})
	mm2 := updated2.(*model)

	got2 := mm2.list.Items()
	if len(got2) != 1 {
		t.Fatalf("expected 1 item after fresh enrichment, got %d", len(got2))
	}
	it2, ok := got2[0].(Item)
	if !ok {
		t.Fatalf("expected Item, got %T", got2[0])
	}
	if it2.Status != "running" {
		t.Errorf("fresh enrichment (gen=2) not applied: Status = %q, want %q", it2.Status, "running")
	}
}
