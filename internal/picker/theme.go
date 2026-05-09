package picker

import (
	"github.com/charmbracelet/lipgloss"

	booth "github.com/erzz/boo/internal/theme"
)

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

// buildTheme materialises the lipgloss styles for a palette. This is
// the one place we map the data model (theme.Theme) to renderable
// lipgloss styles. Adding a new colour slot to the palette means
// adding one line here and one in defaultTheme YAML — nothing else.
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

// ThemeByName resolves a theme name to a styled Theme. Loading order:
//
//  1. User theme at themesDir/<name>.yaml (if themesDir != "").
//  2. Built-in theme embedded in the binary.
//  3. Built-in `default` theme as ultimate fallback.
//
// On any error (unknown name, malformed file, IO failure) it falls
// back to the default theme silently and returns ok=false. Themes are
// cosmetic, not functional — a typo in `ui.theme` shouldn't prevent
// the picker from launching. Callers that care about the failure
// (notably `boo doctor`) should resolve the theme themselves via the
// `internal/theme` package and inspect the error.
//
// themesDir may be empty, which restricts resolution to built-ins.
// This matches what tests want and avoids forcing every caller to
// thread the path through.
func ThemeByName(themesDir, name string) (Theme, bool) {
	r, err := booth.Resolve(themesDir, name)
	if err != nil {
		return defaultTheme(), false
	}
	return buildTheme(r.Theme), true
}
