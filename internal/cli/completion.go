package cli

import (
	"github.com/erzz/boo/internal/layout"
	"github.com/erzz/boo/internal/project"
	"github.com/erzz/boo/internal/state"
)

// projectNamesForCompletion returns registered project names suitable
// for cobra ValidArgsFunction completion.
//
// All errors are swallowed: completion runs in the user's shell on
// every TAB. A noisy error (or a non-zero exit) would break the shell
// prompt or pollute the candidate list. Returning nil silently makes
// the shell fall back to no completions, which is the right
// degradation.
//
// Note: we deliberately don't go through newApp() here. newApp loads
// config, which can FAIL on malformed YAML — and we don't want a
// broken config to disable shell completion. We resolve paths
// directly via state.Default() and skip config entirely.
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

// templateNamesForCompletion returns layout template names (built-in +
// user) suitable for cobra ValidArgsFunction completion. Same
// silent-on-error contract as projectNamesForCompletion.
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
