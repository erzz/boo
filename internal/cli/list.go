package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/sean-erswell-liljefelt/boo/internal/project"
)

func newListCmd() *cobra.Command {
	return &cobra.Command{
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
			if len(reg.Projects) == 0 {
				fmt.Fprintln(c.OutOrStdout(), "No projects registered. Run 'boo new <name> --dir <path>' to create one.")
				return nil
			}
			renderList(c.Context(), c.OutOrStdout(), a, reg.Projects)
			return nil
		},
	}
}

func renderList(ctx context.Context, w io.Writer, a *app, projects []project.Project) {
	if ctx == nil {
		ctx = context.Background()
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tDIR\tSTATUS\tLAST LAUNCHED")
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
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", p.Name, p.Dir, status, last)
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
