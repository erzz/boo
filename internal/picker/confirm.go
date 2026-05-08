package picker

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// confirmModel is a generic yes/no modal used by destructive actions.
// It carries a title, body, and the intent to emit if the user
// confirms. The owner (model.Update) handles the y/n keys via the
// shared keymap and turns "yes" into mm.intent + tea.Quit.
//
// We keep this as a plain data struct (not a tea.Model) because the
// surrounding model already has the message loop; nesting a second
// Update would make the cancel/back routing harder than it's worth for
// a 6-line modal.
type confirmModel struct {
	title string
	body  string
	// pending is the intent to set on the parent model when the user
	// confirms. nil + active modal is a programmer error.
	pending Intent
}

// view renders the modal centred-ish using lipgloss. We don't actually
// overlay on top of the list view (lipgloss doesn't support true
// overlays without tea.WithAltScreen tricks); instead we render the
// modal as the whole screen while screenConfirm is active. Lazygit and
// k9s both do the same and it reads as a "modal" because the
// surrounding chrome disappears.
func (c confirmModel) view(t Theme, _ int, _ int) string {
	var b strings.Builder
	b.WriteString(t.RightPaneTitle.Render(c.title))
	b.WriteString("\n\n")
	b.WriteString(c.body)
	b.WriteString("\n\n")
	b.WriteString(t.RightPaneFaint.Render("[y/enter] confirm   [n/esc] cancel"))

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Padding(1, 2).
		Render(b.String())
	return box
}

// formatDeleteBody renders the per-project body text for a delete
// confirmation. Spelled out here so the in-CLI prompt and the in-TUI
// modal stay in sync about what gets touched.
func formatDeleteBody(name, dir string, purge bool) string {
	body := fmt.Sprintf("Project: %s\nDirectory: %s\n\n"+
		"The registry entry and per-project state will be removed.\n"+
		"The source directory will NOT be touched.", name, dir)
	if purge {
		body += "\n\nThe associated Ghostty window will also be closed."
	}
	return body
}
