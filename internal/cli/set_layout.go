package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/erzz/boo/internal/layout"
	"github.com/erzz/boo/internal/project"
)

// newSetLayoutCmd implements `boo set-layout <name> <template>` —
// re-resolve the named template and write it as the project's saved
// layout, replacing whatever was there.
//
// This is the missing partner of `boo new --layout`: it lets users
// switch a project to a different template without delete+recreate.
// Authored a new template? Point an existing project at it. Want to
// reset a hand-edited layout to a clean built-in? Run set-layout with
// the template name and your edits are gone.
//
// Caveats:
//   - Any hand-edits to layout.yaml are lost. There is no diff/confirm
//     step; this is a destructive replace by design (matches the user
//     intent "give me this template's layout, period").
//   - The registry's Layout field (display only) is updated to the new
//     template name so `boo list` shows the right thing.
//   - We do NOT auto-launch the project after switching. The next
//     `boo <name>` will pick up the new layout. Auto-launching could
//     close panes the user is working in, which is the kind of
//     surprise we'd rather avoid.
func newSetLayoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set-layout <name> <template>",
		Short: "Switch a project to a different layout template",
		Long: `Re-point an existing project at a different layout template.

The named template is resolved (user templates in ~/.config/boo/layouts/
shadow built-ins) and written as the project's new layout, replacing
whatever was there. Any hand-edits to the project's layout.yaml are lost.

The new layout is NOT applied to the running Ghostty window — the next
'boo <name>' will pick it up.

Run 'boo layouts' to see available templates.`,
		Args: cobra.ExactArgs(2),
		RunE: func(c *cobra.Command, args []string) error {
			a, err := newApp()
			if err != nil {
				return err
			}
			name, template := args[0], args[1]

			// Resolve the template up front so a typo doesn't
			// half-way through the registry write. ResolveTemplate
			// returns a clear "not found" error.
			resolved, err := layout.ResolveTemplate(a.Paths.LayoutsDir, template)
			if err != nil {
				return err
			}
			l := resolved.Layout
			if l.Name == "" {
				l.Name = template
			}

			if err := a.Paths.WithLock(func() error {
				reg, err := project.Load(a.Paths)
				if err != nil {
					return err
				}
				p, err := reg.Get(name)
				if err != nil {
					return err
				}
				if err := project.SaveLayout(a.Paths, name, l); err != nil {
					return err
				}
				p.Layout = l.Name
				if err := reg.Update(p); err != nil {
					return err
				}
				return reg.Save(a.Paths)
			}); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(c.OutOrStdout(),
				"Project %q is now using layout %q. Next 'boo %s' will apply it.\n",
				name, l.Name, name)
			return nil
		},
	}
}
