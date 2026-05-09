package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

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
// Output is human-formatted, two-column by default. Pass --json for
// machine-readable JSON (snake_case keys, RFC 3339 timestamps).
func newShowCmd() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "show <name>",
		Short: "Print everything boo knows about a project",
		Long: `Print the full record for one project: source directory,
template name, runtime status, last-launched timestamp, and the paths
of the layout and runtime state files boo manages for it.

Useful for debugging ("is boo seeing what I think it is?") and as the
fastest way to find a project's layout file for hand-editing.

Use --json to emit a single JSON object suitable for scripting.`,
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

			if jsonOut {
				return renderShowJSON(c.OutOrStdout(), p, rt, status, a)
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
	cmd.Flags().BoolVar(&jsonOut, "json", false, "output as JSON")
	return cmd
}

// showOutputJSON is the JSON shape for `boo show --json`. Uses snake_case
// to match the project.Runtime JSON convention (window_id, last_launched_at).
type showOutputJSON struct {
	Name           string     `json:"name"`
	Dir            string     `json:"dir"`
	Layout         string     `json:"layout"`
	Status         string     `json:"status"`
	WindowID       string     `json:"window_id,omitempty"`
	LastLaunchedAt *time.Time `json:"last_launched_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	LayoutFile     string     `json:"layout_file"`
	StateFile      string     `json:"state_file"`
	StateDir       string     `json:"state_dir"`
}

func renderShowJSON(w io.Writer, p project.Project, rt project.Runtime, status string, a *app) error {
	out := showOutputJSON{
		Name:       p.Name,
		Dir:        p.Dir,
		Layout:     p.Layout,
		Status:     status,
		WindowID:   rt.WindowID,
		CreatedAt:  p.CreatedAt,
		LayoutFile: a.Paths.ProjectLayoutFile(p.Name),
		StateFile:  a.Paths.ProjectStateFile(p.Name),
		StateDir:   a.Paths.ProjectDir(p.Name),
	}
	if !rt.LastLaunchedAt.IsZero() {
		t := rt.LastLaunchedAt
		out.LastLaunchedAt = &t
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}
