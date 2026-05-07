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

// previewWidth is the target width for layout previews shown by `boo
// layouts`. Wide enough to fit the canonical 1+2 (`triple`) shape with
// readable cells, narrow enough to land cleanly on an 80-col terminal
// after the 2-space indent we add for visual grouping.
const previewWidth = 50

// newLayoutsCmd implements `boo layouts`: list every available layout
// template (built-in + user) with its description and an ASCII preview.
//
// This is the primary discovery surface for the layout system. A user
// who's never edited a YAML file should be able to run `boo layouts`,
// see what shapes ship with boo, and pick one for `boo new --layout`.
//
// Output structure per template:
//
//	<name>                                 [built-in|user]
//	  <path>
//	  <description>
//
//	  <preview>
//
// User templates that shadow built-ins (same name) are listed once,
// with the user version winning. We mark them [user] so it's obvious
// which version the user is seeing.
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

// layoutsJSONEntry is the per-template record emitted by `boo layouts
// --json`. The ASCII preview is omitted because it's purely a TTY
// affordance — JSON consumers that want shape information should
// inspect the layout structure directly via 'boo show' or by reading
// the layout file.
//
// Error is set (and Source/Path/Description left zero) when a user
// template fails to parse. The human listing surfaces these inline
// with [error]; JSON consumers see the same information so a tool
// can warn rather than silently dropping the broken template.
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
			// Mirror the human listing: surface broken
			// templates rather than hiding them. Tools can
			// detect a non-empty error field and flag the
			// template for the user.
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

// runLayouts is the testable core of `boo layouts`. Extracted from
// RunE so we can pass a fake out/Paths in tests without standing up a
// full app instance.
func runLayouts(a *app, out io.Writer) error {
	names, err := layout.ListTemplates(a.Paths.LayoutsDir)
	if err != nil {
		return fmt.Errorf("list layouts: %w", err)
	}
	if len(names) == 0 {
		// Defensive: built-ins are embedded so this should never
		// happen. If it does, something is very wrong with the
		// binary build — say so rather than silently print nothing.
		return fmt.Errorf("no layout templates available (built-ins missing from binary?)")
	}
	sort.Strings(names)

	for i, name := range names {
		if i > 0 {
			_, _ = fmt.Fprintln(out)
		}
		r, err := layout.ResolveTemplate(a.Paths.LayoutsDir, name)
		if err != nil {
			// One bad user template shouldn't kill the whole
			// listing — surface the error inline and keep going.
			// This matches what a user expects: "show me what's
			// here, including what's broken."
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
	// Right-align the source tag to a fixed column so the listing
	// scans cleanly regardless of name length. Truncate names that
	// would push past the tag column rather than disrupt alignment.
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

	// Preview: indent each line by 2 spaces to group it visually
	// with the metadata above.
	preview := layoutpreview.RenderLayout(r.Layout, previewWidth)
	for _, line := range strings.Split(preview, "\n") {
		_, _ = fmt.Fprintf(w, "  %s\n", line)
	}
}
