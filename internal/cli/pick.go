package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/erzz/boo/internal/picker"
	"github.com/erzz/boo/internal/project"
)

func newPickCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "pick",
		Short: "Open the TUI picker to switch projects",
		Long: `Open a Bubble Tea picker over registered projects. Enter switches to
the highlighted project (focusing its existing window or opening a new one);
q or Esc cancels.`,
		RunE: func(c *cobra.Command, _ []string) error {
			a, err := newApp()
			if err != nil {
				return err
			}
			reg, err := project.Load(a.Paths)
			if err != nil {
				return err
			}
			if len(reg.Projects) == 0 {
				fmt.Fprintln(c.OutOrStdout(), "No projects registered. Run 'boo new <name> --dir <path>' to create one.")
				return nil
			}

			items := buildPickerItems(c.Context(), a, reg.Projects)
			res, err := picker.Run("boo — projects", items)
			if err != nil {
				return err
			}
			if res.Selected == "" {
				// User cancelled. Not an error.
				return nil
			}

			p, err := reg.Get(res.Selected)
			if err != nil {
				return err
			}
			return switchToProject(c.Context(), a, p)
		},
	}
}

// buildPickerItems mirrors the status/last-launched columns from `boo list`
// so users see consistent information across both UIs.
func buildPickerItems(ctx context.Context, a *app, projects []project.Project) []picker.Item {
	if ctx == nil {
		ctx = context.Background()
	}
	out := make([]picker.Item, 0, len(projects))
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
		trailing := ""
		if !rt.LastLaunchedAt.IsZero() {
			trailing = humanAge(rt.LastLaunchedAt)
		}
		out = append(out, picker.Item{
			Key:         p.Name,
			Title:       p.Name,
			Description: p.Dir,
			Status:      status,
			Trailing:    trailing,
		})
	}
	return out
}
