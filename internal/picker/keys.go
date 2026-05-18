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

	// Layout editor sub-screen.
	LayoutEditCycleNext  key.Binding // tab — cycle to next leaf / divider
	LayoutEditCyclePrev  key.Binding // shift+tab — cycle to previous leaf / divider
	LayoutEditApply      key.Binding // ctrl+s — apply customisation, dispatch intent (enter would conflict with textinput)
	LayoutEditBack       key.Binding // esc — discard edits, return to the form
	LayoutEditToggleMode key.Binding // ctrl+l — toggle between leaf (commands) and divider (sizes) mode
	LayoutEditSizeIncr   key.Binding // + / = — increase first child's share by 5% (divider mode)
	LayoutEditSizeDecr   key.Binding // - / _ — decrease first child's share by 5% (divider mode)
	LayoutEditSizeReset  key.Binding // 0 — reset divider to "split evenly" (divider mode)
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
		LayoutEditCycleNext: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "next pane"),
		),
		LayoutEditCyclePrev: key.NewBinding(
			key.WithKeys("shift+tab"),
			key.WithHelp("shift+tab", "prev pane"),
		),
		LayoutEditApply: key.NewBinding(
			key.WithKeys("ctrl+s"),
			key.WithHelp("ctrl+s", "apply customisation"),
		),
		LayoutEditBack: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "back to form"),
		),
		LayoutEditToggleMode: key.NewBinding(
			key.WithKeys("ctrl+l"),
			key.WithHelp("ctrl+l", "toggle leaf/divider mode"),
		),
		LayoutEditSizeIncr: key.NewBinding(
			// "=" is the same physical key as "+" on US layouts and avoids a
			// shift requirement; non-US layouts that produce "+" without shift
			// also work via the canonical "+" entry.
			key.WithKeys("+", "="),
			key.WithHelp("+", "grow first child"),
		),
		LayoutEditSizeDecr: key.NewBinding(
			// "_" is shift+"-" on US; both produce useful keysyms across layouts.
			key.WithKeys("-", "_"),
			key.WithHelp("-", "shrink first child"),
		),
		LayoutEditSizeReset: key.NewBinding(
			key.WithKeys("0"),
			key.WithHelp("0", "split evenly"),
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
