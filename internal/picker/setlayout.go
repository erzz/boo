package picker

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// setLayoutModel is the sub-screen reached by pressing 'l' on a project row.
// Duplicates the form's layout cycler rather than extracting it — the formModel cycler
// is intertwined with three other inputs. One ~30-line copy is cheaper than the refactor.
type setLayoutModel struct {
	projectName string
	names       []string // template names; never empty when this screen is active
	idx         int
	preview     func(name string) string // optional ASCII preview callback
}

// newSetLayoutModel constructs the sub-screen pre-positioned at currentTemplate (or index 0 if not found).
func newSetLayoutModel(projectName, currentTemplate string, names []string, preview func(string) string) setLayoutModel {
	idx := 0
	for i, n := range names {
		if n == currentTemplate {
			idx = i
			break
		}
	}
	return setLayoutModel{
		projectName: projectName,
		names:       names,
		idx:         idx,
		preview:     preview,
	}
}

// cycle moves the cursor by delta (-1 or +1), wrapping. Returns by value — Update reassigns m.setLayout.
func (s setLayoutModel) cycle(delta int) setLayoutModel {
	if len(s.names) == 0 {
		return s
	}
	s.idx = (s.idx + delta + len(s.names)) % len(s.names)
	return s
}

// current returns the template name at the cursor. Empty when the
// names list is empty (defensive — shouldn't happen at runtime).
func (s setLayoutModel) current() string {
	if s.idx < 0 || s.idx >= len(s.names) {
		return ""
	}
	return s.names[s.idx]
}

// view renders the cycler row, an ASCII preview of the highlighted
// template (if a preview callback was wired), and a keybind footer.
func (s setLayoutModel) view(t Theme) string {
	var b strings.Builder
	b.WriteString(t.RightPaneTitle.Render(fmt.Sprintf("Set layout — %s", s.projectName)))
	b.WriteString("\n\n")

	// Cycler row: ◀ name ▶
	left := t.RightPaneFaint.Render("◀")
	right := t.RightPaneFaint.Render("▶")
	name := t.RightPaneValue.Render(s.current())
	fmt.Fprintf(&b, "  %s  %s  %s\n\n", left, name, right)

	if s.preview != nil {
		if rendered := s.preview(s.current()); rendered != "" {
			b.WriteString(rendered)
			b.WriteString("\n\n")
		}
	}

	b.WriteString(t.RightPaneFaint.Render("[←/→ or h/l] cycle   [enter] apply   [esc] cancel"))

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Padding(1, 2).
		Render(b.String())
}

// updateSetLayout handles input on the set-layout sub-screen.
// Lives on *model (not setLayoutModel) so it can mutate m.intent on confirm and m.screen on cancel.
func (m *model) updateSetLayout(msg tea.Msg) (tea.Model, tea.Cmd) {
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	pressed := km.String()
	switch pressed {
	case "left", "h":
		m.setLayout = m.setLayout.cycle(-1)
		return m, nil
	case "right", "l":
		m.setLayout = m.setLayout.cycle(+1)
		return m, nil
	case "enter":
		intent := SetLayoutIntent{
			Name:     m.setLayout.projectName,
			Template: m.setLayout.current(),
		}
		m.setLayout = setLayoutModel{}
		return m.runIntent(intent)
	}
	if matches(m.keys.ConfirmNo, pressed) {
		m.setLayout = setLayoutModel{}
		m.screen = screenList
		return m, nil
	}
	return m, nil
}
