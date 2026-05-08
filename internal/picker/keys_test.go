package picker

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// The keymap is the single source of truth for picker keybindings.
// These tests pin the contract so future re-bindings can't silently
// disagree with what the Update functions accept.

func TestKeyMap_AllBindingsHaveKeys(t *testing.T) {
	k := defaultKeyMap()
	for name, b := range map[string]struct{ keys []string }{
		"Select":  {k.Select.Keys()},
		"Quit":    {k.Quit.Keys()},
		"New":     {k.New.Keys()},
		"Cancel":  {k.Cancel.Keys()},
		"Confirm": {k.Confirm.Keys()},
		"Switch":  {k.Switch.Keys()},
	} {
		if len(b.keys) == 0 {
			t.Errorf("%s binding has no keys", name)
		}
	}
}

func TestMatches_HitsAllConfiguredKeys(t *testing.T) {
	k := defaultKeyMap()
	// Every key listed for Quit must match.
	for _, key := range k.Quit.Keys() {
		if !matches(k.Quit, key) {
			t.Errorf("matches(Quit, %q) = false, want true", key)
		}
	}
	if matches(k.Quit, "z") {
		t.Error("matches(Quit, \"z\") = true, want false")
	}
}

// AlreadyRegistered footer is derived from the keymap so the on-screen
// help can never claim a binding that updateAlreadyRegistered doesn't
// actually handle.
func TestAlreadyRegisteredFooter_DerivedFromKeyMap(t *testing.T) {
	m := sizedModel(120) // wide enough not to matter
	m.screen = screenAlreadyRegistered
	m.alreadyRegisteredAs = "demo"
	v := m.View()
	if !strings.Contains(v, "switch") {
		t.Errorf("footer missing 'switch' help text:\n%s", v)
	}
	if !strings.Contains(v, "continue") {
		t.Errorf("footer missing 'continue' help text:\n%s", v)
	}
}

// Pressing the configured Quit key on the AlreadyRegistered screen
// cancels — sanity check that updateAlreadyRegistered actually consults
// the keymap (not stale string literals).
func TestAlreadyRegistered_QuitKeyCancels(t *testing.T) {
	m := sizedModel(120)
	m.screen = screenAlreadyRegistered
	m.alreadyRegisteredAs = "demo"
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	mm := updated.(*model)
	if !mm.cancelled {
		t.Error("expected cancelled after q on AlreadyRegistered screen")
	}
}
