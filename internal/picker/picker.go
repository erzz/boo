// Package picker is the Bubble Tea TUI used by `boo`.
//
// It owns nothing project-specific: callers pass in a slice of items and
// receive back a Result describing what the user wants to do next (switch
// to an existing project, create a new one, or cancel). The CLI layer
// performs the actual side effects. This keeps the picker free of
// Ghostty/registry/process dependencies and trivially testable.
package picker

import (
	"fmt"
	"io"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

// Item is one row in the picker.
type Item struct {
	Key         string
	Title       string
	Description string
	Status      string // "running" | "stopped" | "dir-missing" | ""
	Trailing    string
}

// FilterValue satisfies bubbles/list.Item.
func (i Item) FilterValue() string { return i.Title + " " + i.Description }

// newProjectItem is the synthetic "+ New project" row injected at the bottom.
type newProjectItem struct{}

func (newProjectItem) FilterValue() string { return "+ New project" }

// Intent is the sealed sum type of "what the user wants to do next" returned
// by Run. CLI code dispatches on the concrete type via a type switch.
//
// New intents are added as new struct types implementing isIntent(). The
// interface is sealed (unexported method) so external packages can't add
// variants and break the exhaustive switches in CLI dispatch code.
type Intent interface{ isIntent() }

// SwitchIntent — user picked an existing project from the list.
type SwitchIntent struct{ Name string }

// NewProjectIntent — user submitted the new-project form. The CLI turns
// this into the actual registry/clone work.
//
// Exactly one of Dir or From is meaningful at submit time:
//
//   - If From is non-empty, the project will be created by cloning into Dir
//     (Dir defaults to a sibling of cwd named after the repo).
//   - Otherwise Dir is the existing directory to register.
type NewProjectIntent struct {
	Name     string
	Dir      string
	From     string
	Template string
}

func (SwitchIntent) isIntent()     {}
func (NewProjectIntent) isIntent() {}

// Result describes what the user did. A nil Intent means the user cancelled.
type Result struct {
	Intent Intent
}

// Cancelled reports whether the user dismissed the UI without choosing.
func (r Result) Cancelled() bool { return r.Intent == nil }

// Options configures Run.
type Options struct {
	Title    string
	Defaults FormDefaults
	// SkipListGoStraightToForm opens the form immediately, skipping the
	// project list. Used by `boo new` (no positional args).
	SkipListGoStraightToForm bool
	// HideNewProject suppresses the synthetic "+ New project" row and the
	// 'n'/'+' keybind that opens the form. Used by selection-only callers
	// like `boo delete` where creating a new project from the picker would
	// make no sense.
	HideNewProject bool
	// PreviewTemplate, if set, is called whenever the form's "Layout
	// template" field changes; the returned string is rendered below the
	// form as a preview of what the layout will look like. Empty return =
	// no preview shown (e.g. unknown template name while the user types).
	//
	// We pass a callback rather than importing the layout package here so
	// the picker stays free of project-specific dependencies — the same
	// reason Item, FormDefaults etc. don't reference internal/project or
	// internal/ghostty.
	PreviewTemplate func(name string) string
	// LayoutNames, if non-empty, turns the form's "Layout template" field
	// from a free-text input into a left/right cycler over this list. The
	// CLI populates it from layout.ListTemplates so users see exactly the
	// templates they have available (built-ins + any user overrides) and
	// can't typo their way into a validation error after submit.
	//
	// Empty = back to the legacy free-text input. Useful for tests and
	// for any future caller that wants the old behaviour.
	LayoutNames []string
	// Theme selects a named visual theme. Empty or unknown names fall
	// back to the built-in default. The CLI populates this from the
	// `ui.theme` config key.
	Theme string
}

// Run shows the TUI and blocks until the user makes a decision.
//
// Renders to stderr so command output redirected via stdout doesn't get
// mangled and the alt-screen restore at exit is clean.
func Run(items []Item, opts Options) (Result, error) {
	theme, _ := ThemeByName(opts.Theme)

	listItems := make([]list.Item, 0, len(items)+1)
	for _, it := range items {
		listItems = append(listItems, it)
	}
	if !opts.HideNewProject {
		listItems = append(listItems, newProjectItem{})
	}

	l := list.New(listItems, newDelegate(theme), 0, 0)
	title := opts.Title
	if title == "" {
		title = "boo — projects"
	}
	l.Title = title
	l.SetShowStatusBar(false)
	l.SetShowHelp(true)
	l.SetFilteringEnabled(true)
	l.Styles.Title = theme.ListTitle
	if !opts.HideNewProject {
		l.AdditionalShortHelpKeys = shortKeys
		l.AdditionalFullHelpKeys = shortKeys
	}

	form := newFormModel(opts.Defaults, theme)
	form.preview = opts.PreviewTemplate
	form.setLayoutNames(opts.LayoutNames)

	formOnly := opts.SkipListGoStraightToForm
	scr := initialScreen(formOnly, opts.Defaults.AlreadyRegisteredAs)

	m := &model{
		list:                l,
		form:                form,
		theme:               theme,
		screen:              scr,
		formOnly:            formOnly,
		hideNewProject:      opts.HideNewProject,
		alreadyRegisteredAs: opts.Defaults.AlreadyRegisteredAs,
	}
	prog := tea.NewProgram(m, tea.WithAltScreen(), tea.WithOutput(stderr()))
	final, err := prog.Run()
	if err != nil {
		return Result{}, fmt.Errorf("picker: %w", err)
	}
	mm := final.(*model)
	if mm.cancelled {
		return Result{}, nil
	}
	return Result{Intent: mm.intent}, nil
}

// screen identifies the active sub-view.
type screen int

const (
	screenList screen = iota
	screenForm
	screenAlreadyRegistered
)

// initialScreen decides which sub-view to show when the picker starts.
//
// AlreadyRegistered is only meaningful when we were going to land on
// the form. The interstitial answers the question "you asked to create
// a new project here, but this dir already has one — switch or
// continue?". For list-first flows (bare `boo`) the list is exactly
// what the user wants; short-circuiting to the interstitial would be
// hostile (forcing them to press 'esc' just to see their own project
// list).
//
// Pulled out of Run() so the precedence rule is unit-testable without
// spinning up a Bubble Tea program.
func initialScreen(formOnly bool, alreadyRegisteredAs string) screen {
	switch {
	case formOnly && alreadyRegisteredAs != "":
		return screenAlreadyRegistered
	case formOnly:
		return screenForm
	default:
		return screenList
	}
}

type model struct {
	list   list.Model
	form   formModel
	theme  Theme
	screen screen

	formOnly            bool   // when true, esc on form cancels the whole TUI rather than going back to the list
	hideNewProject      bool   // when true, the form/intent path is unreachable; selection-only mode
	alreadyRegisteredAs string // shown on the AlreadyRegistered screen

	intent    Intent // nil + cancelled=false should not happen; nil + cancelled=true means dismissed
	cancelled bool

	width, height int
}

func (m *model) Init() tea.Cmd { return nil }

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if sz, ok := msg.(tea.WindowSizeMsg); ok {
		m.width, m.height = sz.Width, sz.Height
		m.list.SetSize(sz.Width, sz.Height)
		m.form.setSize(sz.Width)
		return m, nil
	}

	switch m.screen {
	case screenAlreadyRegistered:
		return m.updateAlreadyRegistered(msg)
	case screenForm:
		return m.updateForm(msg)
	default:
		return m.updateList(msg)
	}
}

func (m *model) updateList(msg tea.Msg) (tea.Model, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok {
		// While the filter input is focused, pass everything through.
		if m.list.FilterState() != list.Filtering {
			switch km.String() {
			case "q", "ctrl+c", "esc":
				m.cancelled = true
				return m, tea.Quit
			case "n", "+":
				if m.hideNewProject {
					break
				}
				m.screen = screenForm
				return m, nil
			case "enter":
				switch v := m.list.SelectedItem().(type) {
				case Item:
					m.intent = SwitchIntent{Name: v.Key}
					return m, tea.Quit
				case newProjectItem:
					if m.hideNewProject {
						break
					}
					m.screen = screenForm
					return m, nil
				}
			}
		}
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m *model) updateForm(msg tea.Msg) (tea.Model, tea.Cmd) {
	cmd, submitted, intent, cancelled := m.form.update(msg)
	if cancelled {
		if m.formOnly {
			m.cancelled = true
			return m, tea.Quit
		}
		m.screen = screenList
		return m, nil
	}
	if submitted {
		m.intent = *intent
		return m, tea.Quit
	}
	return m, cmd
}

func (m *model) updateAlreadyRegistered(msg tea.Msg) (tea.Model, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok {
		switch km.String() {
		case "s", "enter":
			m.intent = SwitchIntent{Name: m.alreadyRegisteredAs}
			return m, tea.Quit
		case "c":
			m.screen = screenForm
			return m, nil
		case "esc", "q", "ctrl+c":
			m.cancelled = true
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m *model) View() string {
	switch m.screen {
	case screenAlreadyRegistered:
		return m.viewAlreadyRegistered()
	case screenForm:
		return m.form.view()
	default:
		return m.list.View()
	}
}

func (m *model) viewAlreadyRegistered() string {
	out := m.theme.AlreadyRegisteredTitle.Render(
		"This directory is already registered")
	out += "\n\nProject: " + m.theme.AlreadyRegisteredName.Render(m.alreadyRegisteredAs)
	out += "\n\n" + m.theme.AlreadyRegisteredHelp.Render(
		"[s/enter] switch to it   [c] continue creating new   [esc] cancel")
	return out
}

type itemDelegate struct {
	theme Theme
}

func newDelegate(t Theme) itemDelegate { return itemDelegate{theme: t} }

func (itemDelegate) Height() int                             { return 2 }
func (itemDelegate) Spacing() int                            { return 0 }
func (itemDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

func (d itemDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	selected := index == m.Index()

	if _, ok := listItem.(newProjectItem); ok {
		cursor := "  "
		titleS := d.theme.NewProject
		if selected {
			cursor = "▌ "
			titleS = d.theme.NewProjectFocus
		}
		_, _ = fmt.Fprintln(w, cursor+titleS.Render("+ New project"))
		_, _ = fmt.Fprint(w, "    "+d.theme.NewProjectFooter.Render("press enter to register a project"))
		return
	}

	it, ok := listItem.(Item)
	if !ok {
		return
	}
	titleS, descS := d.theme.Title, d.theme.Desc
	if selected {
		titleS, descS = d.theme.SelectedTitle, d.theme.SelectedDesc
	}
	cursor := "  "
	if selected {
		cursor = "▌ "
	}

	first := fmt.Sprintf("%s%s   %s", cursor, titleS.Render(it.Title), d.renderStatus(it.Status))
	if it.Trailing != "" {
		first += "   " + d.theme.Trailing.Render(it.Trailing)
	}
	second := "    " + descS.Render(it.Description)
	_, _ = fmt.Fprintln(w, first)
	_, _ = fmt.Fprint(w, second)
}

func (d itemDelegate) renderStatus(s string) string {
	switch s {
	case "running":
		return d.theme.StatusRunning.Render("● running")
	case "dir-missing":
		return d.theme.StatusBroken.Render("✖ dir missing")
	case "":
		return ""
	default:
		return d.theme.StatusStopped.Render("○ " + s)
	}
}

func shortKeys() []key.Binding {
	return []key.Binding{
		key.NewBinding(
			key.WithKeys("n", "+"),
			key.WithHelp("n", "new project"),
		),
	}
}
