package picker

import (
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
	return &model{list: l}
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
