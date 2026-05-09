package picker

import (
	"github.com/charmbracelet/lipgloss"

	booth "github.com/erzz/boo/internal/theme"
)

// Theme is the resolved set of lipgloss styles the picker renders with.
// All fields are required; construct via named constructors (defaultTheme, etc.).
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

	// Right pane (split view)
	RightPaneBorder lipgloss.Style
	RightPaneTitle  lipgloss.Style
	RightPaneLabel  lipgloss.Style
	RightPaneValue  lipgloss.Style
	RightPaneFaint  lipgloss.Style

	// Left pane (list) border. Mirrors RightPaneBorder so the split
	// view reads as two equally-weighted panes rather than one boxed
	// pane next to a bare list.
	ListPaneBorder lipgloss.Style

	// Status bar (bottom of the list screen). Renders the most recent
	// action's outcome, e.g. "deleted alpha · ok" or "edit failed:
	// name reserved".
	StatusBar      lipgloss.Style // base / "ok" outcome
	StatusBarError lipgloss.Style // failed-action outcome (red emphasis)
	StatusBarFaint lipgloss.Style // idle / no-action-yet hint text
}

// buildTheme materialises lipgloss styles from a palette. The single mapping point from
// internal/theme data model to renderable styles.
func buildTheme(p booth.Theme) Theme {
	accent := lipgloss.Color(p.Colors.Accent)
	info := lipgloss.Color(p.Colors.Info)
	border := lipgloss.Color(p.Colors.Border)
	ok := lipgloss.Color(p.Colors.OK)
	warn := lipgloss.Color(p.Colors.Warn)
	stopped := lipgloss.Color(p.Colors.Stopped)

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

		RightPaneBorder: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(border).
			Padding(0, 1),
		RightPaneTitle: bold.Foreground(accent),
		RightPaneLabel: faint,
		RightPaneValue: lipgloss.NewStyle(),
		RightPaneFaint: faint,

		ListPaneBorder: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(border).
			Padding(0, 1),

		StatusBar:      lipgloss.NewStyle().Foreground(ok),
		StatusBarError: bold.Foreground(warn),
		StatusBarFaint: faint,
	}
}

// defaultTheme returns the lipgloss styles built from the embedded
// `default` theme. Used as the safe fallback when ThemeByName can't
// resolve a user-requested theme.
func defaultTheme() Theme {
	return buildTheme(booth.MustDefault())
}

// ThemeByName resolves a theme by name (user file in themesDir, then built-in, then default).
// On any error falls back to the default theme silently and returns ok=false.
// Themes are cosmetic — a typo in ui.theme must not prevent the picker from launching.
func ThemeByName(themesDir, name string) (Theme, bool) {
	r, err := booth.Resolve(themesDir, name)
	if err != nil {
		return defaultTheme(), false
	}
	return buildTheme(r.Theme), true
}
