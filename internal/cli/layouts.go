package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/erzz/boo/internal/layout"
	"github.com/erzz/boo/internal/layoutpreview"
)

// previewWidth is the ASCII preview width for `boo layouts` — fits triple-pane on 80-col terminals.
const previewWidth = 50

// newLayoutsCmd implements `boo layouts` — list every layout template (built-in + user)
// with its description and an ASCII preview.
func newLayoutsCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "layouts",
		Short: "List available layout templates with previews",
		Long: `List every layout template available to 'boo new --layout', with
its description and an ASCII preview of the resulting window.

Templates come from two places:

  - Built-in templates embedded in the boo binary.
  - User templates in ~/.config/boo/layouts/<name>.yaml.

User templates with the same name as a built-in shadow the built-in;
this command lists each name once and marks the source as [user] when
shadowed.

Use 'boo new --layout <name>' to create a project with one of these
layouts. To create a custom layout, drop a <name>.yaml in
~/.config/boo/layouts/ — see docs/layouts.md for the YAML reference.`,
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, args []string) error {
			a, err := newApp()
			if err != nil {
				return err
			}
			if asJSON {
				return runLayoutsJSON(a, c.OutOrStdout())
			}
			return runLayouts(a, c.OutOrStdout())
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit machine-readable JSON instead of the human listing")
	return cmd
}

// layoutsJSONEntry is the per-template record for `boo layouts --json`.
// ASCII preview omitted (TTY affordance only). Non-empty Error signals a broken user template.
type layoutsJSONEntry struct {
	Name        string `json:"name"`
	Source      string `json:"source,omitempty"` // "builtin" or "user"
	Path        string `json:"path,omitempty"`
	Description string `json:"description,omitempty"`
	Error       string `json:"error,omitempty"`
}

func runLayoutsJSON(a *app, w io.Writer) error {
	names, err := layout.ListTemplates(a.Paths.LayoutsDir)
	if err != nil {
		return fmt.Errorf("list layouts: %w", err)
	}
	sort.Strings(names)
	out := make([]layoutsJSONEntry, 0, len(names))
	for _, name := range names {
		r, err := layout.ResolveTemplate(a.Paths.LayoutsDir, name)
		if err != nil {
			// Surface broken templates rather than hiding them.
			out = append(out, layoutsJSONEntry{
				Name:  name,
				Error: err.Error(),
			})
			continue
		}
		src := "builtin"
		if r.Source == layout.SourceUser {
			src = "user"
		}
		out = append(out, layoutsJSONEntry{
			Name:        r.Layout.Name,
			Source:      src,
			Path:        r.Path,
			Description: r.Description,
		})
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// runLayouts is the testable core of `boo layouts` (extracted from RunE).
func runLayouts(a *app, out io.Writer) error {
	names, err := layout.ListTemplates(a.Paths.LayoutsDir)
	if err != nil {
		return fmt.Errorf("list layouts: %w", err)
	}
	if len(names) == 0 {
		// Defensive: built-ins are embedded; if this fires the binary build is broken.
		return fmt.Errorf("no layout templates available (built-ins missing from binary?)")
	}
	sort.Strings(names)

	for i, name := range names {
		if i > 0 {
			_, _ = fmt.Fprintln(out)
		}
		r, err := layout.ResolveTemplate(a.Paths.LayoutsDir, name)
		if err != nil {
			// Surface error inline so one bad template doesn't kill the whole listing.
			_, _ = fmt.Fprintf(out, "%s    [error]\n  %v\n", name, err)
			continue
		}
		writeLayoutEntry(out, r)
	}
	return nil
}

// writeLayoutEntry writes one template's listing entry to w.
func writeLayoutEntry(w io.Writer, r layout.ResolvedTemplate) {
	source := "[built-in]"
	if r.Source == layout.SourceUser {
		source = "[user]"
	}
	// Right-align source tag to a fixed column; truncate long names rather than break alignment.
	const tagCol = 40
	name := r.Layout.Name
	if name == "" {
		name = "(unnamed)"
	}
	pad := tagCol - len(name)
	if pad < 1 {
		pad = 1
	}
	_, _ = fmt.Fprintf(w, "%s%s%s\n", name, strings.Repeat(" ", pad), source)
	_, _ = fmt.Fprintf(w, "  %s\n", r.Path)

	if r.Description != "" {
		for _, line := range strings.Split(r.Description, "\n") {
			_, _ = fmt.Fprintf(w, "  %s\n", line)
		}
	}
	_, _ = fmt.Fprintln(w)

	// Indent preview 2 spaces to group visually with the metadata above.
	resolved, err := r.Layout.Resolve(0)
	preview := "(preview unavailable)"
	if err == nil {
		preview = layoutpreview.RenderLayout(resolved, previewWidth)
	}
	for _, line := range strings.Split(preview, "\n") {
		_, _ = fmt.Fprintf(w, "  %s\n", line)
	}
}
