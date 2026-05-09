package picker

import "github.com/charmbracelet/bubbles/key"

// keyMap is the single source of truth for picker keybindings.
// Centralising them here ensures the `?` help footer can never drift from
// what Update actually handles, and tests can reference binding keys rather than hard-coded strings.
type keyMap struct {
	// List screen — selection / dismissal.
	Select key.Binding // enter
	Quit   key.Binding // q, ctrl+c, esc

	// List screen — actions on the highlighted project.
	New        key.Binding // n, + (also opens form when "+ New project" row picked)
	Edit       key.Binding // e
	OpenLayout key.Binding // o — open layout YAML in $EDITOR
	Delete     key.Binding // d — confirm modal, DeleteIntent{Purge:false}
	Purge      key.Binding // D — same but Purge:true (close window too)
	SetLayout  key.Binding // l — layout cycler sub-screen

	// List screen — global UI controls.
	CycleTheme key.Binding // T — cycle to the next available theme and persist

	// Confirm modal.
	ConfirmYes key.Binding // y, enter
	ConfirmNo  key.Binding // n, esc

	// Form / interstitial screens.
	Cancel  key.Binding // esc — back to list (or quit if formOnly)
	Confirm key.Binding // s/enter on the AlreadyRegistered interstitial
	Switch  key.Binding // c on the AlreadyRegistered interstitial
}

// defaultKeyMap returns the production bindings. Function (not var) so tests get fresh copies.
func defaultKeyMap() keyMap {
	return keyMap{
		Select: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "select"),
		),
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q / ctrl-c", "quit"),
		),
		New: key.NewBinding(
			key.WithKeys("n", "+"),
			key.WithHelp("n", "new project"),
		),
		Edit: key.NewBinding(
			key.WithKeys("e"),
			key.WithHelp("e", "edit"),
		),
		OpenLayout: key.NewBinding(
			key.WithKeys("o"),
			key.WithHelp("o", "open layout file"),
		),
		Delete: key.NewBinding(
			key.WithKeys("d"),
			key.WithHelp("d", "delete"),
		),
		Purge: key.NewBinding(
			key.WithKeys("D"),
			key.WithHelp("D", "delete + close window"),
		),
		SetLayout: key.NewBinding(
			key.WithKeys("l"),
			key.WithHelp("l", "set layout"),
		),
		CycleTheme: key.NewBinding(
			key.WithKeys("T"),
			key.WithHelp("T", "cycle theme"),
		),
		ConfirmYes: key.NewBinding(
			key.WithKeys("y", "enter"),
			key.WithHelp("y/enter", "confirm"),
		),
		ConfirmNo: key.NewBinding(
			key.WithKeys("n", "N", "esc"),
			key.WithHelp("n/esc", "cancel"),
		),
		Cancel: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "back"),
		),
		Confirm: key.NewBinding(
			key.WithKeys("s", "enter"),
			key.WithHelp("s/enter", "switch"),
		),
		Switch: key.NewBinding(
			key.WithKeys("c"),
			key.WithHelp("c", "continue (create new)"),
		),
	}
}

// matches reports whether pressed matches any of b's configured keys.
// Works on string keys (tea.KeyMsg.String()) so tests don't need to construct fake messages.
func matches(b key.Binding, pressed string) bool {
	for _, k := range b.Keys() {
		if k == pressed {
			return true
		}
	}
	return false
}
