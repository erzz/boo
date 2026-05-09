package cli

import (
	"github.com/erzz/boo/internal/layout"
	"github.com/erzz/boo/internal/project"
	"github.com/erzz/boo/internal/state"
)

// projectNamesForCompletion returns registered project names for cobra tab-completion.
// All errors are swallowed silently — a noisy error on TAB would break the shell prompt.
// Skips newApp() so a malformed config doesn't disable completion.
func projectNamesForCompletion() []string {
	paths, err := state.Default()
	if err != nil {
		return nil
	}
	reg, err := project.Load(paths)
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(reg.Projects))
	for _, p := range reg.Projects {
		out = append(out, p.Name)
	}
	return out
}

// templateNamesForCompletion returns layout template names for cobra tab-completion.
// Same silent-on-error contract as projectNamesForCompletion.
func templateNamesForCompletion() []string {
	paths, err := state.Default()
	if err != nil {
		return nil
	}
	names, err := layout.ListTemplates(paths.LayoutsDir)
	if err != nil {
		return nil
	}
	return names
}
