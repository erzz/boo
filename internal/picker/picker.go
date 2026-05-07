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
	"github.com/charmbracelet/lipgloss"
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

// Result describes what the user did. At most one of Selected and NewProject
// is populated; if neither is set the user cancelled.
type Result struct {
	Selected   string
	NewProject *NewProjectIntent
}

// Cancelled reports whether the user dismissed the UI without choosing.
func (r Result) Cancelled() bool { return r.Selected == "" && r.NewProject == nil }

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
}

// Run shows the TUI and blocks until the user makes a decision.
//
// Renders to stderr so command output redirected via stdout doesn't get
// mangled and the alt-screen restore at exit is clean.
func Run(items []Item, opts Options) (Result, error) {
	listItems := make([]list.Item, 0, len(items)+1)
	for _, it := range items {
		listItems = append(listItems, it)
	}
	if !opts.HideNewProject {
		listItems = append(listItems, newProjectItem{})
	}

	l := list.New(listItems, newDelegate(), 0, 0)
	title := opts.Title
	if title == "" {
		title = "boo — projects"
	}
	l.Title = title
	l.SetShowStatusBar(false)
	l.SetShowHelp(true)
	l.SetFilteringEnabled(true)
	l.Styles.Title = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("13"))
	if !opts.HideNewProject {
		l.AdditionalShortHelpKeys = shortKeys
		l.AdditionalFullHelpKeys = shortKeys
	}

	form := newFormModel(opts.Defaults)
	form.preview = opts.PreviewTemplate
	form.setLayoutNames(opts.LayoutNames)

	scr := screenList
	formOnly := opts.SkipListGoStraightToForm
	// AlreadyRegistered is only meaningful when we were going to land on
	// the form. The interstitial answers the question "you asked to
	// create a new project here, but this dir already has one — switch
	// or continue?". For list-first flows (bare `boo`) the list is
	// exactly what the user wants; short-circuiting to the interstitial
	// would be hostile (forcing them to press 'esc' just to see their
	// own project list).
	switch {
	case formOnly && opts.Defaults.AlreadyRegisteredAs != "":
		scr = screenAlreadyRegistered
	case formOnly:
		scr = screenForm
	}

	m := &model{
		list:                l,
		form:                form,
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
	if mm.intent != nil {
		return Result{NewProject: mm.intent}, nil
	}
	return Result{Selected: mm.selected}, nil
}

// screen identifies the active sub-view.
type screen int

const (
	screenList screen = iota
	screenForm
	screenAlreadyRegistered
)

type model struct {
	list   list.Model
	form   formModel
	screen screen

	formOnly            bool   // when true, esc on form cancels the whole TUI rather than going back to the list
	hideNewProject      bool   // when true, the form/intent path is unreachable; selection-only mode
	alreadyRegisteredAs string // shown on the AlreadyRegistered screen

	selected  string
	intent    *NewProjectIntent
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
					m.selected = v.Key
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
		m.intent = intent
		return m, tea.Quit
	}
	return m, cmd
}

func (m *model) updateAlreadyRegistered(msg tea.Msg) (tea.Model, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok {
		switch km.String() {
		case "s", "enter":
			m.selected = m.alreadyRegisteredAs
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
	out := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("13")).Render(
		"This directory is already registered")
	out += "\n\nProject: " + lipgloss.NewStyle().Bold(true).Render(m.alreadyRegisteredAs)
	out += "\n\n" + lipgloss.NewStyle().Faint(true).Render(
		"[s/enter] switch to it   [c] continue creating new   [esc] cancel")
	return out
}

// styles
var (
	titleStyle    = lipgloss.NewStyle().Bold(true)
	descStyle     = lipgloss.NewStyle().Faint(true)
	selectedTitle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("13"))
	selectedDesc  = lipgloss.NewStyle().Foreground(lipgloss.Color("13"))

	statusRunning = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true)
	statusStopped = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	statusBroken  = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)

	trailingStyle    = lipgloss.NewStyle().Faint(true)
	newProjectStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	newProjectFocus  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("13"))
	newProjectFooter = lipgloss.NewStyle().Faint(true)
)

type itemDelegate struct{}

func newDelegate() itemDelegate { return itemDelegate{} }

func (itemDelegate) Height() int                             { return 2 }
func (itemDelegate) Spacing() int                            { return 0 }
func (itemDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

func (d itemDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	selected := index == m.Index()

	if _, ok := listItem.(newProjectItem); ok {
		cursor := "  "
		titleS := newProjectStyle
		if selected {
			cursor = "▌ "
			titleS = newProjectFocus
		}
		_, _ = fmt.Fprintln(w, cursor+titleS.Render("+ New project"))
		_, _ = fmt.Fprint(w, "    "+newProjectFooter.Render("press enter to register a project"))
		return
	}

	it, ok := listItem.(Item)
	if !ok {
		return
	}
	titleS, descS := titleStyle, descStyle
	if selected {
		titleS, descS = selectedTitle, selectedDesc
	}
	cursor := "  "
	if selected {
		cursor = "▌ "
	}

	first := fmt.Sprintf("%s%s   %s", cursor, titleS.Render(it.Title), renderStatus(it.Status))
	if it.Trailing != "" {
		first += "   " + trailingStyle.Render(it.Trailing)
	}
	second := "    " + descS.Render(it.Description)
	_, _ = fmt.Fprintln(w, first)
	_, _ = fmt.Fprint(w, second)
}

func renderStatus(s string) string {
	switch s {
	case "running":
		return statusRunning.Render("● running")
	case "dir-missing":
		return statusBroken.Render("✖ dir missing")
	case "":
		return ""
	default:
		return statusStopped.Render("○ " + s)
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
