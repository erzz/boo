package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/erzz/boo/internal/picker"
	"github.com/erzz/boo/internal/project"
)

func newDeleteCmd() *cobra.Command { return newDeleteCmdWithApp(nil) }

// newDeleteCmdWithApp is like newDeleteCmd but accepts a pre-built *app for testing.
func newDeleteCmdWithApp(appIn *app) *cobra.Command {
	var (
		purge bool
		force bool
	)
	cmd := &cobra.Command{
		Use:   "delete [name]",
		Short: "Delete a project from boo",
		Long: `Delete a project's registration. Per-project layout/state files are also
removed from boo's state directory.

With no name argument, opens the TUI picker so you can choose which project
to delete. The picker is selection-only here — the "+ New project" entry is
suppressed.

The project's source directory on disk is NEVER touched. Pass --purge to also
close any open Ghostty window for the project.

You will be asked to confirm; pass --force to skip the prompt.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			a := appIn
			if a == nil {
				var err error
				a, err = newApp()
				if err != nil {
					return err
				}
			}

			var name string
			if len(args) == 1 {
				name = args[0]
			} else {
				picked, err := pickProjectForDelete(c.Context(), a)
				if err != nil {
					return err
				}
				if picked == "" {
					// User cancelled the picker — treat as a no-op.
					return nil
				}
				name = picked
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

				if !force {
					ok, err := confirmDelete(c.InOrStdin(), c.OutOrStdout(), p.Name, p.Dir, purge)
					if err != nil {
						return err
					}
					if !ok {
						_, _ = fmt.Fprintln(c.OutOrStdout(), "Aborted.")
						return nil
					}
				}

				warns, err := executeDelete(c.Context(), a, reg, p, purge)
				if err != nil {
					return err
				}
				_, _ = fmt.Fprintf(c.OutOrStdout(), "Deleted project %q (source dir %s left untouched)\n", p.Name, p.Dir)
				for _, w := range warns {
					_, _ = fmt.Fprintf(c.ErrOrStderr(), "warning: %s\n", w)
				}
				return nil
			})
		},
	}
	cmd.Flags().BoolVar(&purge, "purge", false, "also close any associated Ghostty window")
	cmd.Flags().BoolVar(&force, "force", false, "skip the confirmation prompt")
	return cmd
}

// executeDelete performs the side-effect half of `boo delete`: optional window-close,
// registry removal, and state-dir purge. Returns (warnings, error). Non-nil warnings
// = deletion succeeded but non-fatal side effects failed. Non-nil error = deletion failed.
// Does not print; callers decide how to surface results. Callers MUST hold the state lock.
func executeDelete(ctx context.Context, a *app, reg *project.Registry, p project.Project, purge bool) ([]string, error) {
	var warnings []string
	if purge {
		rt, err := project.LoadRuntime(a.Paths, p.Name)
		if err == nil && rt.WindowID != "" {
			if err := a.Ghostty.CloseWindow(ctx, rt.WindowID); err != nil {
				warnings = append(warnings, fmt.Sprintf("could not close window %s: %v", rt.WindowID, err))
			}
		}
	}

	if err := reg.Remove(p.Name); err != nil {
		return nil, err
	}
	if err := reg.Save(a.Paths); err != nil {
		return nil, err
	}
	if err := project.PurgeProjectDir(a.Paths, p.Name); err != nil {
		// Registry is already updated; report but don't fail.
		warnings = append(warnings, fmt.Sprintf("removed from registry but could not purge state dir: %v", err))
	}
	return warnings, nil
}

// pickProjectForDelete shows the TUI picker in selection-only mode. Returns "" on cancel.
func pickProjectForDelete(ctx context.Context, a *app) (string, error) {
	reg, err := project.Load(a.Paths)
	if err != nil {
		return "", err
	}
	if len(reg.Projects) == 0 {
		return "", errors.New("no projects registered — nothing to delete")
	}
	items := buildPickerItems(ctx, a, reg.Projects)
	res, err := picker.Run(items, picker.Options{
		Title:          "boo — delete project",
		HideNewProject: true,
		PreviewProjectFactory: func(thm picker.Theme) func(string) string {
			return projectPreviewer(ctx, a, thm)
		},
		Theme:          a.Config.ThemeOr("default"),
		ThemesDir:      a.Paths.ThemesDir,
		ConfigPath:     a.Paths.ConfigFile,
	})
	if err != nil {
		return "", err
	}
	if res.Cancelled() {
		return "", nil
	}
	// In selection-only mode, enter (SwitchIntent) and d/D (DeleteIntent) both mean "this one".
	switch v := res.Intent.(type) {
	case picker.SwitchIntent:
		return v.Name, nil
	case picker.DeleteIntent:
		return v.Name, nil
	default:
		return "", nil
	}
}

// confirmDelete asks the user to confirm a delete. Only y/yes counts as yes.
func confirmDelete(in io.Reader, out io.Writer, name, dir string, purge bool) (bool, error) {
	_, _ = fmt.Fprintf(out, "Delete project %q?\n", name)
	_, _ = fmt.Fprintf(out, "  registry entry + state will be removed\n")
	_, _ = fmt.Fprintf(out, "  source dir %s will NOT be touched\n", dir)
	if purge {
		_, _ = fmt.Fprintf(out, "  any open Ghostty window will be closed\n")
	}
	_, _ = fmt.Fprintf(out, "Type 'y' to confirm: ")

	r := bufio.NewReader(in)
	line, err := r.ReadString('\n')
	if err != nil && err != io.EOF {
		return false, err
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes", nil
}
