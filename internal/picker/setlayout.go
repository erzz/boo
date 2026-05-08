package picker

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// setLayoutModel is the sub-screen reached by pressing 'l' on a project
// row in the list. It cycles through the available layout templates and
// emits a SetLayoutIntent on enter.
//
// Mirrors the form's layout cycler but stripped down — we don't need
// name/dir/from inputs here, just template selection. The cycler logic
// is duplicated rather than extracted from form.go because (a) the
// formModel cycler is intertwined with three other inputs and a
// preview pane, and (b) one ~30-line copy is cheaper than a refactor
// that would touch every form_test.go assertion. If a third caller
// shows up we extract.
type setLayoutModel struct {
	projectName string
	names       []string // template names; never empty when this screen is active
	idx         int
	preview     func(name string) string // optional ASCII preview callback
}

// newSetLayoutModel constructs the sub-screen pre-positioned at the
// project's current template (so ←/→ cycles relative to "what it is
// now" rather than always starting at index 0).
//
// If currentTemplate isn't in names, we start at index 0 — the user can
// still cycle to any template, we just don't have a meaningful anchor.
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

// cycle moves the cursor by delta (-1 or +1), wrapping. Returns the
// receiver by value because setLayoutModel is a value type and Update
// idiomatically reassigns m.setLayout = m.setLayout.cycle(±1).
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
//
// Lives on *model (not setLayoutModel) so it can mutate m.intent on
// confirm and m.screen on cancel — the same pattern updateConfirm uses.
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
