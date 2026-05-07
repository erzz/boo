package picker

import "github.com/charmbracelet/lipgloss"

// Theme is the resolved set of styles the picker uses to render itself.
//
// Until now styles were scattered as module-level `var (...)` blocks. This
// type centralises them so we can:
//
//   - Swap themes at runtime (Options.Theme + ThemeByName).
//   - Add new named themes without touching every render call site.
//   - Test that a theme has all required fields populated.
//
// All fields are required — a zero-value Theme will render empty/invisible
// text. Construct themes via the named constructors (defaultTheme, etc.)
// rather than building one from scratch.
type Theme struct {
	// List item styles
	Title         lipgloss.Style
	Desc          lipgloss.Style
	SelectedTitle lipgloss.Style
	SelectedDesc  lipgloss.Style

	// Status pill styles (running/stopped/broken)
	StatusRunning lipgloss.Style
	StatusStopped lipgloss.Style
	StatusBroken  lipgloss.Style

	// Trailing metadata + the synthetic "+ New project" row
	Trailing         lipgloss.Style
	NewProject       lipgloss.Style
	NewProjectFocus  lipgloss.Style
	NewProjectFooter lipgloss.Style

	// List title (top of the bubbles/list view)
	ListTitle lipgloss.Style

	// "Already registered" interstitial
	AlreadyRegisteredTitle lipgloss.Style
	AlreadyRegisteredName  lipgloss.Style
	AlreadyRegisteredHelp  lipgloss.Style

	// Form
	FormTitle            lipgloss.Style
	FormLabel            lipgloss.Style
	FormLabelFocus       lipgloss.Style
	FormInfo             lipgloss.Style
	FormErr              lipgloss.Style
	FormHelp             lipgloss.Style
	FormCyclerArrow      lipgloss.Style
	FormCyclerArrowFocus lipgloss.Style
}

// defaultTheme is the only built-in theme today. The palette and weights
// match the original module-level styles exactly so Phase 1 is a pure
// refactor — no visible change.
func defaultTheme() Theme {
	const (
		accent  = lipgloss.Color("13") // magenta — selection / focus
		info    = lipgloss.Color("12") // blue — "+ New project"
		ok      = lipgloss.Color("10") // green — running
		warn    = lipgloss.Color("9")  // red — broken / errors
		stopped = lipgloss.Color("8")  // grey — stopped
	)
	bold := lipgloss.NewStyle().Bold(true)
	faint := lipgloss.NewStyle().Faint(true)

	return Theme{
		Title:         bold,
		Desc:          faint,
		SelectedTitle: bold.Foreground(accent),
		SelectedDesc:  lipgloss.NewStyle().Foreground(accent),

		StatusRunning: bold.Foreground(ok),
		StatusStopped: lipgloss.NewStyle().Foreground(stopped),
		StatusBroken:  bold.Foreground(warn),

		Trailing:         faint,
		NewProject:       bold.Foreground(info),
		NewProjectFocus:  bold.Foreground(accent),
		NewProjectFooter: faint,

		ListTitle: bold.Foreground(accent),

		AlreadyRegisteredTitle: bold.Foreground(accent),
		AlreadyRegisteredName:  bold,
		AlreadyRegisteredHelp:  faint,

		FormTitle:            bold.Foreground(accent),
		FormLabel:            faint,
		FormLabelFocus:       bold.Foreground(accent),
		FormInfo:             faint.Italic(true),
		FormErr:              bold.Foreground(warn),
		FormHelp:             faint,
		FormCyclerArrow:      faint,
		FormCyclerArrowFocus: bold.Foreground(accent),
	}
}

// ThemeByName resolves a theme name to a Theme. Unknown names fall back
// to the default theme silently — themes are cosmetic, not functional, so
// a typo in config shouldn't break the picker. Callers that care about
// validation can compare the returned theme against defaultTheme() or
// inspect the bool.
func ThemeByName(name string) (Theme, bool) {
	switch name {
	case "", "default":
		return defaultTheme(), true
	default:
		return defaultTheme(), false
	}
}
