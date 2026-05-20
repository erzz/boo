package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/erzz/boo/internal/project"
)

// newEditCmd opens a project's layout snapshot (~/.config/boo/projects/<name>/layout.yaml) in $EDITOR.
// Validation is lazy — a broken file will error on next `boo <name>`, same as `boo config edit`.
func newEditCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "edit <name>",
		Short: "Open a project's layout file in $EDITOR",
		Long: `Open the per-project layout snapshot for <name> in $EDITOR.

The layout file lives under boo's config directory (run 'boo show <name>'
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

			// Confirm project exists — editing a layout for an unknown project writes a file that's never read.
			reg, err := project.Load(a.Paths)
			if err != nil {
				return err
			}
			if _, err := reg.Get(name); err != nil {
				return err
			}
			p, err := reg.Get(name)
			if err != nil {
				return err
			}
			responsive, err := projectUsesResponsiveLayout(a, p)
			if err != nil {
				return err
			}
			if responsive {
				return fmt.Errorf("project %q uses a responsive layout; editing responsive layouts is not supported yet", name)
			}

			path := a.Paths.ProjectLayoutFile(name)
			if _, err := os.Stat(path); err != nil {
				// Layout file missing for a registered project — surface the path so the user can investigate.
				return fmt.Errorf("layout file for project %q not found at %s: %w", name, path, err)
			}
			return openInEditor("", path)
		},
	}
}
