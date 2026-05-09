package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/erzz/boo/internal/project"
)

// newEditCmd opens a project's per-project layout snapshot
// (~/.local/share/boo/projects/<name>/layout.yaml) in $EDITOR.
//
// This is the power-user counterpart to `boo set-layout`: rather than
// switching to a different template, the user can fine-tune the saved
// shape directly. Validation happens lazily — the next `boo <name>`
// will fail if the user saved broken YAML, mirroring `boo config edit`'s
// behaviour.
func newEditCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "edit <name>",
		Short: "Open a project's layout file in $EDITOR",
		Long: `Open the per-project layout snapshot for <name> in $EDITOR.

The layout file lives under boo's data directory (run 'boo show <name>'
to see its exact path). Edits are not validated until boo next loads
the file — if you save broken YAML, the next 'boo <name>' will error.

$EDITOR is required. $VISUAL is honoured if $EDITOR is unset.`,
		Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			a, err := newApp()
			if err != nil {
				return err
			}
			name := args[0]

			// Confirm the project exists. Editing a layout for a
			// project boo doesn't know about would write a file
			// that's never read — silently useless. Better to
			// surface "no such project" up front.
			reg, err := project.Load(a.Paths)
			if err != nil {
				return err
			}
			if _, err := reg.Get(name); err != nil {
				return err
			}

			path := a.Paths.ProjectLayoutFile(name)
			if _, err := os.Stat(path); err != nil {
				// The layout file should exist for any registered
				// project — registration writes it. If it's
				// missing, something has gone wrong; surface
				// the path so the user can investigate.
				return fmt.Errorf("layout file for project %q not found at %s: %w", name, path, err)
			}
			return openInEditor("", path)
		},
	}
}
