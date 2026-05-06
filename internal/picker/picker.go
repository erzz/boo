// Package picker is the Bubble Tea TUI used by `boo pick`.
//
// It owns nothing project-specific: callers pass in a slice of items and
// receive back the chosen one (or nothing, if the user cancelled). The CLI
// layer does the switch; this keeps the picker free of Ghostty/process
// dependencies and trivially testable.
package picker

import (
	"fmt"
	"io"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Item is one row in the picker. Title is shown in bold; Description appears
// dimmed underneath. Status is a short word ("running", "stopped",
// "dir-missing") rendered as a badge.
type Item struct {
	Key         string // unique identifier returned to the caller (typically the project name)
	Title       string
	Description string
	Status      string
	Trailing    string // small right-aligned hint, e.g. "2h ago"
}

// satisfy bubbles/list.Item
func (i Item) FilterValue() string { return i.Title + " " + i.Description }

// Result is what Run returns. Selected is empty when the user cancelled.
type Result struct {
	Selected string // matches Item.Key
}

// Run shows the picker and blocks until the user selects an item or cancels.
//
// Renders to stderr so command output redirected via stdout (`boo pick > x`)
// doesn't get mangled, and so the alt-screen restore at exit is clean even if
// stdout is a pipe.
func Run(title string, items []Item) (Result, error) {
	listItems := make([]list.Item, len(items))
	for i, it := range items {
		listItems[i] = it
	}
	l := list.New(listItems, newDelegate(), 0, 0)
	l.Title = title
	l.SetShowStatusBar(false)
	l.SetShowHelp(true)
	l.SetFilteringEnabled(true)
	l.Styles.Title = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("13"))

	m := model{list: l}
	prog := tea.NewProgram(&m, tea.WithAltScreen(), tea.WithOutput(stderr()))
	final, err := prog.Run()
	if err != nil {
		return Result{}, fmt.Errorf("picker: %w", err)
	}
	mm := final.(*model)
	if mm.cancelled || mm.selected == "" {
		return Result{}, nil
	}
	return Result{Selected: mm.selected}, nil
}

// model is the Bubble Tea model. Held as a pointer because the program needs
// to read the final state (selection) after Run returns.
type model struct {
	list      list.Model
	selected  string
	cancelled bool
	width     int
	height    int
}

func (m *model) Init() tea.Cmd { return nil }

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		// Leave a couple of lines for title + help.
		m.list.SetSize(msg.Width, msg.Height)
		return m, nil

	case tea.KeyMsg:
		// While the filter input is focused, only Esc/Enter should escape it;
		// everything else (including q) is text input. The list model handles
		// this internally — we only intercept when the filter is *not* active.
		if m.list.FilterState() == list.Filtering {
			break
		}
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			m.cancelled = true
			return m, tea.Quit
		case "enter":
			if it, ok := m.list.SelectedItem().(Item); ok {
				m.selected = it.Key
				return m, tea.Quit
			}
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m *model) View() string { return m.list.View() }

// styles —— intentionally restrained. boo's TUI is a launcher, not a dashboard.
var (
	titleStyle    = lipgloss.NewStyle().Bold(true)
	descStyle     = lipgloss.NewStyle().Faint(true)
	selectedTitle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("13"))
	selectedDesc  = lipgloss.NewStyle().Foreground(lipgloss.Color("13"))

	statusRunning = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true) // green
	statusStopped = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))             // grey
	statusBroken  = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)  // red

	trailingStyle = lipgloss.NewStyle().Faint(true)
)

// itemDelegate renders each row. We roll our own (instead of the bubbles
// default) so we get a single-line presentation with status badge + trailing
// timestamp.
type itemDelegate struct{}

func newDelegate() itemDelegate { return itemDelegate{} }

func (itemDelegate) Height() int                             { return 2 }
func (itemDelegate) Spacing() int                            { return 0 }
func (itemDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

func (d itemDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	it, ok := listItem.(Item)
	if !ok {
		return
	}
	selected := index == m.Index()
	titleS, descS := titleStyle, descStyle
	if selected {
		titleS, descS = selectedTitle, selectedDesc
	}
	status := renderStatus(it.Status)

	cursor := "  "
	if selected {
		cursor = "▌ "
	}

	first := fmt.Sprintf("%s%s   %s", cursor, titleS.Render(it.Title), status)
	if it.Trailing != "" {
		first += "   " + trailingStyle.Render(it.Trailing)
	}
	second := "    " + descS.Render(it.Description)
	fmt.Fprintln(w, first)
	fmt.Fprint(w, second)
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
