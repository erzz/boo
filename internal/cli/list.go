package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/erzz/boo/internal/project"
)

func newListCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List registered projects",
		RunE: func(c *cobra.Command, _ []string) error {
			a, err := newApp()
			if err != nil {
				return err
			}
			reg, err := project.Load(a.Paths)
			if err != nil {
				return err
			}
			if asJSON {
				return renderListJSON(c.Context(), c.OutOrStdout(), a, reg.Projects)
			}
			if len(reg.Projects) == 0 {
				_, _ = fmt.Fprintln(c.OutOrStdout(), "No projects registered. Run 'boo new <name> --dir <path>' to create one.")
				return nil
			}
			renderList(c.Context(), c.OutOrStdout(), a, reg.Projects)
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit machine-readable JSON instead of the human table")
	return cmd
}

// listJSONEntry is the per-project record emitted by `boo list --json`.
// Explicit type decouples the wire format from internal storage so changes to project.Project don't silently break scripts.
type listJSONEntry struct {
	Name           string    `json:"name"`
	Dir            string    `json:"dir"`
	Layout         string    `json:"layout"`
	Status         string    `json:"status"`
	WindowID       string    `json:"window_id,omitempty"`
	LastLaunchedAt time.Time `json:"last_launched_at,omitempty"`
	CreatedAt      time.Time `json:"created_at,omitempty"`
}

// renderListJSON prints the project list as a JSON array. Empty registries emit `[]`, not null.
func renderListJSON(ctx context.Context, w io.Writer, a *app, projects []project.Project) error {
	if ctx == nil {
		ctx = context.Background()
	}
	out := make([]listJSONEntry, 0, len(projects))
	for _, p := range projects {
		rt, _ := project.LoadRuntime(a.Paths, p.Name)
		status := "stopped"
		switch {
		case !dirExists(p.Dir):
			status = "dir-missing"
		case rt.WindowID != "":
			if exists, err := a.Ghostty.WindowExists(ctx, rt.WindowID); err == nil && exists {
				status = "running"
			}
		}
		out = append(out, listJSONEntry{
			Name:           p.Name,
			Dir:            p.Dir,
			Layout:         p.Layout,
			Status:         status,
			WindowID:       rt.WindowID,
			LastLaunchedAt: rt.LastLaunchedAt,
			CreatedAt:      p.CreatedAt,
		})
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func renderList(ctx context.Context, w io.Writer, a *app, projects []project.Project) {
	if ctx == nil {
		ctx = context.Background()
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "NAME\tDIR\tSTATUS\tLAST LAUNCHED")
	for _, p := range projects {
		rt, _ := project.LoadRuntime(a.Paths, p.Name) // missing → zero
		status := "stopped"
		switch {
		case !dirExists(p.Dir):
			status = "dir-missing"
		case rt.WindowID != "":
			if exists, err := a.Ghostty.WindowExists(ctx, rt.WindowID); err == nil && exists {
				status = "running"
			}
		}
		last := "—"
		if !rt.LastLaunchedAt.IsZero() {
			last = humanAge(rt.LastLaunchedAt)
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", p.Name, p.Dir, status, last)
	}
	_ = tw.Flush()
}

func dirExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

func humanAge(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}
