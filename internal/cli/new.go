package cli

import (
	"errors"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/erzz/boo/internal/layout"
	"github.com/erzz/boo/internal/project"
)

func newNewCmd() *cobra.Command {
	var (
		fromURL    string
		intoDir    string
		existing   string
		layoutName string
	)
	cmd := &cobra.Command{
		Use:   "new <name>",
		Short: "Register a new project",
		Long: `Register a project so 'boo <name>' can launch it.

Use --dir to point at an existing directory:
  boo new projA --dir ~/code/projA

Cloning from a git URL is planned for Phase 2 (--from / --into are accepted
but not yet implemented).`,
		Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			name := args[0]
			if err := project.ValidateName(name); err != nil {
				return err
			}
			if fromURL != "" || intoDir != "" {
				return errors.New("--from / --into are not implemented yet (Phase 2). Use --dir for now.")
			}
			if existing == "" {
				return errors.New("--dir is required (interactive prompts come in Phase 2)")
			}
			dir, err := resolveDir(existing)
			if err != nil {
				return err
			}

			a, err := newApp()
			if err != nil {
				return err
			}

			// Resolve layout up-front so failure here doesn't leave half-state.
			var l layout.Layout
			switch layoutName {
			case "", "default":
				l = layout.Default()
			default:
				return fmt.Errorf("layout %q not found (Phase 1 ships only the built-in 'default' layout; user-defined layouts come in Phase 2)", layoutName)
			}
			l.Name = layoutName
			if l.Name == "" {
				l.Name = "default"
			}

			// Hold the lock across the read-modify-write window.
			return a.Paths.WithLock(func() error {
				reg, err := project.Load(a.Paths)
				if err != nil {
					return err
				}
				if reg.Has(name) {
					return fmt.Errorf("project %q already registered (use 'boo rm %s' first)", name, name)
				}
				if reg.HasDir(dir) {
					existing, _ := reg.FindByDir(dir)
					return fmt.Errorf("directory %s is already registered as project %q", dir, existing.Name)
				}

				// Save layout first; if registry save fails afterwards we clean up.
				if err := project.SaveLayout(a.Paths, name, l); err != nil {
					return err
				}
				if err := reg.Add(project.Project{
					Name:      name,
					Dir:       dir,
					Layout:    l.Name,
					CreatedAt: time.Now().UTC(),
				}); err != nil {
					_ = project.PurgeProjectDir(a.Paths, name)
					return err
				}
				if err := reg.Save(a.Paths); err != nil {
					_ = project.PurgeProjectDir(a.Paths, name)
					return err
				}
				fmt.Fprintf(c.OutOrStdout(), "Registered %q at %s (layout: %s)\n", name, dir, l.Name)
				fmt.Fprintf(c.OutOrStdout(), "Run 'boo %s' to launch it.\n", name)
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&fromURL, "from", "", "git URL to clone from (Phase 2)")
	cmd.Flags().StringVar(&intoDir, "into", "", "directory to clone into, with --from (Phase 2)")
	cmd.Flags().StringVar(&existing, "dir", "", "existing directory to register")
	cmd.Flags().StringVar(&layoutName, "layout", "default", "layout template to use")
	return cmd
}
