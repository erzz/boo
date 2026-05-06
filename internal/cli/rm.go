package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/sean-erswell-liljefelt/boo/internal/project"
)

func newRmCmd() *cobra.Command {
	var purge bool
	cmd := &cobra.Command{
		Use:   "rm <name>",
		Short: "Remove a project from the registry",
		Long: `Remove a project's registration. Per-project layout/state files are
deleted as well.

By default any associated Ghostty window is left open; pass --purge to also
close it.`,
		Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			name := args[0]
			a, err := newApp()
			if err != nil {
				return err
			}
			return a.Paths.WithLock(func() error {
				reg, err := project.Load(a.Paths)
				if err != nil {
					return err
				}
				p, err := reg.Get(name)
				if err != nil {
					if errors.Is(err, project.ErrNotFound) {
						return fmt.Errorf("project %q not found", name)
					}
					return err
				}

				if purge {
					rt, err := project.LoadRuntime(a.Paths, p.Name)
					if err == nil && rt.WindowID != "" {
						if err := a.Ghostty.CloseWindow(c.Context(), rt.WindowID); err != nil {
							fmt.Fprintf(c.ErrOrStderr(), "warning: could not close window %s: %v\n", rt.WindowID, err)
						}
					}
				}

				if err := reg.Remove(p.Name); err != nil {
					return err
				}
				if err := reg.Save(a.Paths); err != nil {
					return err
				}
				if err := project.PurgeProjectDir(a.Paths, p.Name); err != nil {
					// Registry is already updated; report but don't fail.
					fmt.Fprintf(c.ErrOrStderr(), "warning: removed from registry but could not purge state dir: %v\n", err)
				}
				fmt.Fprintf(c.OutOrStdout(), "Removed project %q\n", p.Name)
				return nil
			})
		},
	}
	cmd.Flags().BoolVar(&purge, "purge", false, "also close any associated Ghostty window")
	return cmd
}
