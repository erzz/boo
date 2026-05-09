package picker

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// confirmModel is a generic yes/no modal for destructive actions.
// Not a tea.Model — the surrounding model owns the message loop; nesting a second
// Update would complicate cancel/back routing for a 6-line modal.
type confirmModel struct {
	title string
	body  string
	// pending is the intent to set on the parent model when the user
	// confirms. nil + active modal is a programmer error.
	pending Intent
}

// view renders the modal as the full screen (same approach as lazygit/k9s — surrounding chrome
// disappears while the modal is active, reading as a true modal without alt-screen tricks).
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

// formatDeleteBody renders the delete confirmation body for both the CLI prompt and TUI modal.
func formatDeleteBody(name, dir string, purge bool) string {
	body := fmt.Sprintf("Project: %s\nDirectory: %s\n\n"+
		"The registry entry and per-project state will be removed.\n"+
		"The source directory will NOT be touched.", name, dir)
	if purge {
		body += "\n\nThe associated Ghostty window will also be closed."
	}
	return body
}
