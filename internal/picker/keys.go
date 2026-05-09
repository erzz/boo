package picker

import "github.com/charmbracelet/bubbles/key"

// keyMap is the single source of truth for picker keybindings.
//
// Centralising the bindings here (rather than scattering string literals
// through Update functions) gives us three things:
//
//   - One place to add a new keybind. Phase 5+ adds d/e/l/o/s/D; each
//     one is a single line here plus a case in the switch.
//   - Automatic, accurate `?` help. bubbles/list reads bindings from
//     ShortHelp/FullHelp; with the bindings declared here the help
//     output can never drift from what Update actually handles.
//   - Testability. Tests can reference k.New.Keys() instead of hard-coding
//     "n" / "+" and stay correct if we re-bind a key later.
//
// The map is constructed once per Run() (cheap; key.Binding is just a
// struct of strings) and lives on the model so per-screen Update
// methods can reach it without globals.
type keyMap struct {
	// List screen — selection / dismissal.
	Select key.Binding // enter
	Quit   key.Binding // q, ctrl+c, esc

	// List screen — actions on the highlighted project. Wired in
	// Phase 5+; declared now so the help footer renders the full
	// vocabulary as soon as each handler lands.
	New        key.Binding // n, + (also opens form when "+ New project" row picked)
	Edit       key.Binding // e — open form pre-populated for editing the highlighted project
	OpenLayout key.Binding // o — open the project's layout YAML in $EDITOR (TUI suspended)
	Delete     key.Binding // d — open confirm modal, emit DeleteIntent{Purge:false}
	Purge      key.Binding // D — same as Delete but with Purge:true (close window too)
	SetLayout  key.Binding // l — open layout cycler sub-screen, emit SetLayoutIntent

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

// defaultKeyMap returns the production bindings. Kept as a function
// (rather than a package-level var) so tests can construct fresh copies
// without worrying about cross-test mutation.
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

// matches reports whether a pressed key string matches any of the
// binding's configured keys. Thin wrapper around key.Matches that
// works on string keys (what tea.KeyMsg.String() returns) rather than
// requiring a tea.KeyMsg, so test code can drive the same logic
// without constructing fake messages.
func matches(b key.Binding, pressed string) bool {
	for _, k := range b.Keys() {
		if k == pressed {
			return true
		}
	}
	return false
}
