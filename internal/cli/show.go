package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/erzz/boo/internal/project"
)

// newShowCmd implements `boo show <name>` — print everything boo knows
// about a single project.
//
// This is the diagnostic primitive that `boo list` is too narrow to
// serve: when a user wants to know "where is this project's layout
// file, when did I last open it, is the window actually alive right
// now", they should be able to ask without opening a TUI or grepping
// `boo list`'s columns.
//
// Output is human-formatted, two-column. A future --json flag (G5)
// will give the same data in machine-readable form.
func newShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <name>",
		Short: "Print everything boo knows about a project",
		Long: `Print the full record for one project: source directory,
template name, runtime status, last-launched timestamp, and the paths
of the layout and runtime state files boo manages for it.

Useful for debugging ("is boo seeing what I think it is?") and as the
fastest way to find a project's layout file for hand-editing.`,
		Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			a, err := newApp()
			if err != nil {
				return err
			}
			name := args[0]
			reg, err := project.Load(a.Paths)
			if err != nil {
				return err
			}
			p, err := reg.Get(name)
			if err != nil {
				return err
			}
			rt, _ := project.LoadRuntime(a.Paths, name)

			// Status calculation mirrors buildPickerItems so a
			// user comparing `boo show X` against `boo list`
			// sees the same word for the same state.
			status := "stopped"
			switch {
			case !dirExists(p.Dir):
				status = "dir-missing"
			case rt.WindowID != "":
				if exists, err := a.Ghostty.WindowExists(c.Context(), rt.WindowID); err == nil && exists {
					status = "running"
				}
			}

			lastLaunched := "never"
			if !rt.LastLaunchedAt.IsZero() {
				lastLaunched = rt.LastLaunchedAt.Local().Format("2006-01-02 15:04:05 MST")
			}
			windowID := rt.WindowID
			if windowID == "" {
				windowID = "(none)"
			}

			rows := []struct{ k, v string }{
				{"name", p.Name},
				{"dir", p.Dir},
				{"layout", p.Layout},
				{"status", status},
				{"window_id", windowID},
				{"last_launched", lastLaunched},
				{"created", p.CreatedAt.Local().Format("2006-01-02 15:04:05 MST")},
				{"layout_file", a.Paths.ProjectLayoutFile(name)},
				{"state_file", a.Paths.ProjectStateFile(name)},
				{"state_dir", a.Paths.ProjectDir(name)},
			}

			keyW := 0
			for _, r := range rows {
				if len(r.k) > keyW {
					keyW = len(r.k)
				}
			}
			out := c.OutOrStdout()
			for _, r := range rows {
				_, _ = fmt.Fprintf(out, "  %-*s  %s\n", keyW, r.k, r.v)
			}
			return nil
		},
	}
}
