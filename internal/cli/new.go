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

Or clone from a git URL. The repo name (last URL segment, .git stripped)
is used as the directory name unless --into is given:
  boo new projA --from https://github.com/me/projA.git
  boo new projA --from https://github.com/me/projA.git --into ~/code/projA

--from and --dir are mutually exclusive.`,
		Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			name := args[0]
			if err := project.ValidateName(name); err != nil {
				return err
			}
			if intoDir != "" && fromURL == "" {
				return errors.New("--into requires --from")
			}
			if fromURL == "" && existing == "" {
				return errors.New("either --dir or --from is required")
			}
			if fromURL != "" && existing != "" {
				return errors.New("--dir and --from are mutually exclusive")
			}

			a, err := newApp()
			if err != nil {
				return err
			}

			// Resolve layout up-front so failure here doesn't leave half-state.
			// User templates in $XDG_CONFIG_HOME/boo/layouts/ shadow built-ins.
			resolved, err := layout.ResolveTemplate(a.Paths.LayoutsDir, layoutName)
			if err != nil {
				return err
			}
			l := resolved.Layout
			if l.Name == "" {
				l.Name = layoutName
			}
			if l.Name == "" {
				l.Name = "default"
			}

			// Resolve the project directory.
			//
			// For --dir we just clean the path. For --from we either clone into
			// an explicit --into, or derive a destination from the URL relative
			// to the user's current working directory (matches `git clone`'s
			// own default).
			//
			// Cloning happens *before* the registry lock: clones can be slow,
			// and we don't want to block other boo invocations on network IO.
			// Pre-checks (name/dir collisions) are best-effort here and
			// re-checked under the lock below.
			var dir string
			if fromURL != "" {
				dir, err = resolveCloneDestination(intoDir, fromURL)
				if err != nil {
					return err
				}
				if err := preCheckCollisions(a, name, dir); err != nil {
					return err
				}
				fmt.Fprintf(c.OutOrStdout(), "Cloning %s into %s ...\n", fromURL, dir)
				cloned, err := a.Git.Clone(c.Context(), fromURL, dir)
				if err != nil {
					return err
				}
				dir = cloned
			} else {
				dir, err = resolveDir(existing)
				if err != nil {
					return err
				}
			}

			// Hold the lock across the read-modify-write window.
			return a.Paths.WithLock(func() error {
				reg, err := project.Load(a.Paths)
				if err != nil {
					return err
				}
				if reg.Has(name) {
					if fromURL != "" {
						return fmt.Errorf("project %q already registered (the clone at %s was kept; remove it manually if unwanted)", name, dir)
					}
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
	cmd.Flags().StringVar(&fromURL, "from", "", "git URL to clone from")
	cmd.Flags().StringVar(&intoDir, "into", "", "directory to clone into (with --from); defaults to repo name in cwd")
	cmd.Flags().StringVar(&existing, "dir", "", "existing directory to register")
	cmd.Flags().StringVar(&layoutName, "layout", "default", "layout template to use")
	return cmd
}

// preCheckCollisions surfaces obvious name/dir collisions before we kick off
// a (potentially slow) clone. The same checks are repeated under the lock
// later so this is purely a UX improvement, not a correctness guarantee.
func preCheckCollisions(a *app, name, dir string) error {
	reg, err := project.Load(a.Paths)
	if err != nil {
		// If the registry can't even be read, let the lock-time check produce
		// the real error.
		return nil //nolint:nilerr // intentional: best-effort pre-check
	}
	if reg.Has(name) {
		return fmt.Errorf("project %q already registered (use 'boo rm %s' first)", name, name)
	}
	if reg.HasDir(dir) {
		existing, _ := reg.FindByDir(dir)
		return fmt.Errorf("directory %s is already registered as project %q", dir, existing.Name)
	}
	return nil
}
