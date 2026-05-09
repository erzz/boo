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

// TestInitialScreen_BareBooSkipsInterstitial: regression — bare `boo` must NEVER hit the
// "dir already registered" interstitial. Only formOnly=true flows (boo new / boo save fallback)
// want the interstitial. Previous impl gated it purely on AlreadyRegisteredAs being non-empty.
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

// Fix 1: error leaves items intact; empty slice clears.

// TestRefreshList_ErrorPreservesItems: transient registry read failure must not wipe displayed items.
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

// TestRefreshList_EmptySliceShowsEmptyState: nil error + empty slice → list clears (last project deleted).
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

// TestRefreshList_NilSliceSuccessShowsEmptyState: nil slice + nil error also clears (nil == empty).
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

// TestRunIntent_DeletePurge_WindowCloseWarning: non-fatal window-close failure must show in status bar.
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

// TestRunIntent_DeletePurge_NoWarning: happy path — purge success says "deleted … and closed window".
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

// TestRunIntent_Delete_ErrorPreservesItems: failed delete shows error screen and does NOT refresh.
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

// Fix 1 (extended): both window-close AND state-dir purge warnings must be surfaced.
// Regression: picker previously discarded state-dir purge failures via io.Discard.
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

// Fix 3: async picker startup.

// TestInit_ReturnsEnrichCmdWhenRefreshSet: Init() must return non-nil cmd when refreshItems is set.
func TestInit_ReturnsEnrichCmdWhenRefreshSet(t *testing.T) {
	m := newTestModel(Item{Key: "alpha", Title: "alpha"})
	m.refreshItems = func() ([]Item, error) { return nil, nil }

	cmd := m.Init()
	if cmd == nil {
		t.Fatal("Init() must return a non-nil cmd when refreshItems is set")
	}
}

// TestInit_NilWhenNoRefreshItems: Init() returns nil (no-op) when no RefreshItems callback is set.
func TestInit_NilWhenNoRefreshItems(t *testing.T) {
	m := newTestModel(Item{Key: "alpha", Title: "alpha"})
	// m.refreshItems intentionally nil

	cmd := m.Init()
	if cmd != nil {
		t.Fatal("Init() must return nil when refreshItems is not set")
	}
}

// TestEnrichedItemsMsg_UpdatesItemList: enrichedItemsMsg replaces current items with enriched ones.
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

// TestEnrichedItemsMsg_ErrorPreservesItems: enrichedItemsMsg carrying an error leaves existing items intact.
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

// TestView_DoesNotMutateModel: View() must be pure (Bubble Tea contract). Calling it N times
// must not change screen, status, or any other model field.
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

// Must-fix 1: View() must not invoke the PreviewProject callback.
// Regression: previewProject was called synchronously inside renderItemDetail → viewRightPane → View().
// Preview must be dispatched as a tea.Cmd and applied in Update; View() only reads previewCache.
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

	// Init() dispatches previewProject asynchronously — we do NOT run it. Just verify View() doesn't invoke it.
	_ = m.Init()
	_ = m.View()
	_ = m.View()

	if called {
		t.Error("View() invoked the previewProject callback — View() must only read from previewCache")
	}
}

// Must-fix 2: stale enrichment results must be discarded.
// Regression: user deletes a project quickly after launch; post-delete refresh removes it;
// slower startup enrichment arrives and reinserts it.
func TestEnrichment_OldResultIgnored(t *testing.T) {
	bare := Item{Key: "alpha", Title: "alpha", Status: ""}
	m := newTestModel(bare)
	m.hideNewProject = true
	m.refreshItems = func() ([]Item, error) { return nil, nil }

	// Simulate two in-flight enrichments; gen=2 is the current one.
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
