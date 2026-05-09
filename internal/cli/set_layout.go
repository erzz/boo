package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/erzz/boo/internal/layout"
	"github.com/erzz/boo/internal/project"
)

// newSetLayoutCmd implements `boo set-layout <name> <template>` — re-resolves the named
// template and writes it as the project's saved layout, replacing whatever was there.
// Hand-edits to layout.yaml are lost by design ("give me this template, period").
// The registry's Layout display field is updated; the project is NOT auto-launched.
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
			if err := executeSetLayout(a, name, template); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(c.OutOrStdout(),
				"Project %q is now using layout %q. Next 'boo %s' will apply it.\n",
				name, template, name)
			return nil
		},
	}
}

// executeSetLayout resolves the template, writes the per-project layout snapshot, and updates
// the registry's Layout field — all under the state lock. Used by `boo set-layout` and the
// TUI's SetLayoutIntent. Does not print; callers tailor the success message.
func executeSetLayout(a *app, name, template string) error {
	resolved, err := layout.ResolveTemplate(a.Paths.LayoutsDir, template)
	if err != nil {
		return err
	}
	l := resolved.Layout
	if l.Name == "" {
		l.Name = template
	}
	return a.Paths.WithLock(func() error {
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
	})
}
