package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/erzz/boo/internal/theme"
)

// newThemesCmd implements `boo themes` and its subcommands.
//
// Mirrors the shape of `boo config` and `boo layouts`: a parent command
// with show/path/init subcommands. The bare `boo themes` is an alias
// for `boo themes list`, the same way `boo config` is an alias for
// `boo config show`.
//
// Themes are cosmetic and discovery-driven. The user model is:
//
//   - Run `boo themes` to see what's available (built-in + user).
//   - Pick one in `~/.config/boo/config.yaml` via `ui.theme: <name>`.
//   - To customise: `boo themes init <name>` writes a starter file
//     to ~/.config/boo/themes/<name>.yaml that the user edits.
//
// We deliberately don't auto-seed any files on first run. Users explicitly
// opt in with `themes init` when they want to customise.
func newThemesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "themes",
		Short: "List, inspect, and create visual themes",
		Long: `List every theme available to 'ui.theme' in config.yaml, with its
description and a colour swatch. Themes come from two places:

  - Built-in themes embedded in the boo binary.
  - User themes in ~/.config/boo/themes/<name>.yaml.

User themes with the same name as a built-in shadow the built-in.

Subcommands:

  boo themes              # alias for 'boo themes list'
  boo themes list         # list available themes
  boo themes show <name>  # print a theme's YAML
  boo themes path         # print the user themes directory
  boo themes init <name>  # write a starter theme file you can edit

Activate a theme by setting 'ui.theme: <name>' in
~/.config/boo/config.yaml.`,
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			a, err := newApp()
			if err != nil {
				return err
			}
			return runThemesList(a, c.OutOrStdout(), false)
		},
	}

	cmd.AddCommand(newThemesListCmd())
	cmd.AddCommand(newThemesShowCmd())
	cmd.AddCommand(newThemesPathCmd())
	cmd.AddCommand(newThemesInitCmd())
	return cmd
}

func newThemesListCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List available themes (built-in + user)",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			a, err := newApp()
			if err != nil {
				return err
			}
			return runThemesList(a, c.OutOrStdout(), asJSON)
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit machine-readable JSON instead of the human listing")
	return cmd
}

func newThemesShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <name>",
		Short: "Print a theme's raw YAML",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			a, err := newApp()
			if err != nil {
				return err
			}
			return runThemesShow(a, c.OutOrStdout(), args[0])
		},
	}
}

func newThemesPathCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "path",
		Short: "Print the user themes directory",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			a, err := newApp()
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintln(c.OutOrStdout(), a.Paths.ThemesDir)
			return nil
		},
	}
}

func newThemesInitCmd() *cobra.Command {
	var force bool
	var from string
	cmd := &cobra.Command{
		Use:   "init <name>",
		Short: "Write a starter theme file you can edit",
		Long: `Write a new theme YAML to ~/.config/boo/themes/<name>.yaml.

By default the new file is seeded from the built-in 'default' theme,
so you have a complete palette to edit rather than a stub. Use
--from <theme> to seed from a different built-in (e.g. when more
built-ins ship later).

Refuses to overwrite an existing file unless --force is given.`,
		Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			a, err := newApp()
			if err != nil {
				return err
			}
			seed := from
			if seed == "" {
				seed = "default"
			}
			return runThemesInit(a, c.OutOrStdout(), args[0], seed, force)
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "overwrite an existing theme file")
	cmd.Flags().StringVar(&from, "from", "", "built-in theme to seed from (default: 'default')")
	return cmd
}

// themesJSONEntry mirrors layoutsJSONEntry for symmetry with `boo
// layouts --json`. Tools can detect a non-empty Error field and flag
// the theme rather than silently dropping it from the listing.
type themesJSONEntry struct {
	Name        string `json:"name"`
	Source      string `json:"source,omitempty"`
	Path        string `json:"path,omitempty"`
	Description string `json:"description,omitempty"`
	Active      bool   `json:"active,omitempty"`
	Error       string `json:"error,omitempty"`
}

func runThemesList(a *app, out io.Writer, asJSON bool) error {
	names, err := theme.List(a.Paths.ThemesDir)
	if err != nil {
		return fmt.Errorf("list themes: %w", err)
	}
	if len(names) == 0 {
		// Defensive: built-ins are embedded so this should never
		// happen. If it does, the binary is corrupt.
		return fmt.Errorf("no themes available (built-ins missing from binary?)")
	}
	sort.Strings(names)

	active := a.Config.ThemeOr("default")

	if asJSON {
		entries := make([]themesJSONEntry, 0, len(names))
		for _, name := range names {
			entry := themesJSONEntry{Name: name, Active: name == active}
			r, err := theme.Resolve(a.Paths.ThemesDir, name)
			if err != nil {
				entry.Error = err.Error()
				entries = append(entries, entry)
				continue
			}
			entry.Source = string(r.Source)
			entry.Path = r.Path
			entry.Description = r.Theme.Description
			entries = append(entries, entry)
		}
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(entries)
	}

	for i, name := range names {
		if i > 0 {
			_, _ = fmt.Fprintln(out)
		}
		r, err := theme.Resolve(a.Paths.ThemesDir, name)
		if err != nil {
			_, _ = fmt.Fprintf(out, "%s    [error]\n  %v\n", name, err)
			continue
		}
		writeThemeEntry(out, r, name == active)
	}
	return nil
}

// writeThemeEntry writes one theme's listing entry. The format mirrors
// `boo layouts` (name + tag + path + description + body) but the body
// is a colour swatch rather than an ASCII layout preview.
func writeThemeEntry(w io.Writer, r theme.Resolved, active bool) {
	source := "[built-in]"
	if r.Source == theme.SourceUser {
		source = "[user]"
	}
	if active {
		source = "[active] " + source
	}

	const tagCol = 40
	name := r.Theme.Name
	if name == "" {
		name = "(unnamed)"
	}
	pad := tagCol - len(name)
	if pad < 1 {
		pad = 1
	}
	_, _ = fmt.Fprintf(w, "%s%s%s\n", name, strings.Repeat(" ", pad), source)
	_, _ = fmt.Fprintf(w, "  %s\n", r.Path)

	if r.Theme.Description != "" {
		for _, line := range strings.Split(r.Theme.Description, "\n") {
			_, _ = fmt.Fprintf(w, "  %s\n", line)
		}
	}
	_, _ = fmt.Fprintln(w)

	// Coloured swatch. Each slot renders as a small filled block in
	// the slot's actual colour, followed by the colour value as text
	// for copy-paste. We render lipgloss colours here only on the
	// human listing branch — the JSON branch above stays plain so
	// downstream tools see clean values without ANSI noise.
	for _, slot := range []struct {
		name, val string
	}{
		{"accent ", r.Theme.Colors.Accent},
		{"info   ", r.Theme.Colors.Info},
		{"border ", r.Theme.Colors.Border},
		{"ok     ", r.Theme.Colors.OK},
		{"warn   ", r.Theme.Colors.Warn},
		{"stopped", r.Theme.Colors.Stopped},
	} {
		swatch := renderSwatch(slot.val)
		_, _ = fmt.Fprintf(w, "  %s  %s  %s\n", slot.name, swatch, slot.val)
	}
}

// renderSwatch returns a small filled block coloured to match value.
// Empty value yields whitespace of the same width so swatches stay
// column-aligned regardless of which slots a theme overrides.
func renderSwatch(value string) string {
	const block = "███"
	if value == "" {
		return strings.Repeat(" ", len(block))
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(value)).Render(block)
}

func runThemesShow(a *app, out io.Writer, name string) error {
	// Prefer user theme if present — `show` should reflect what the
	// user would see, not the built-in version they may have shadowed.
	if a.Paths.ThemesDir != "" {
		path := filepath.Join(a.Paths.ThemesDir, name+".yaml")
		if data, err := os.ReadFile(path); err == nil {
			_, _ = out.Write(data)
			if len(data) > 0 && data[len(data)-1] != '\n' {
				_, _ = fmt.Fprintln(out)
			}
			return nil
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("read user theme: %w", err)
		}
	}
	data, err := theme.BuiltinYAML(name)
	if err != nil {
		return fmt.Errorf("theme %q not found", name)
	}
	_, _ = out.Write(data)
	if len(data) > 0 && data[len(data)-1] != '\n' {
		_, _ = fmt.Fprintln(out)
	}
	return nil
}

func runThemesInit(a *app, out io.Writer, name, seed string, force bool) error {
	if a.Paths.ThemesDir == "" {
		return fmt.Errorf("themes directory not configured")
	}
	dst := filepath.Join(a.Paths.ThemesDir, name+".yaml")
	if !force {
		if _, err := os.Stat(dst); err == nil {
			return fmt.Errorf("%s already exists (use --force to overwrite)", dst)
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("stat %s: %w", dst, err)
		}
	}

	data, err := theme.BuiltinYAML(seed)
	if err != nil {
		return fmt.Errorf("seed theme %q not found", seed)
	}
	// Rewrite the seed's `name:` line to match the file we're
	// writing. Without this, every starter file ships with
	// `name: default` which makes `boo themes` list it twice
	// — once as the built-in, once as a user override that
	// happens to point at the same name.
	data = retargetThemeName(data, name)
	if err := os.MkdirAll(a.Paths.ThemesDir, 0o755); err != nil {
		return fmt.Errorf("create themes dir: %w", err)
	}
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		return fmt.Errorf("write theme: %w", err)
	}
	_, _ = fmt.Fprintf(out, "wrote %s\n", dst)
	_, _ = fmt.Fprintln(out, "edit the colour values, then activate with 'ui.theme: "+name+"' in config.yaml")
	return nil
}

// retargetThemeName rewrites a single top-level `name:` line in YAML
// bytes to a new value. We do this with a line-level scan rather than
// YAML round-trip so the seeded file preserves the source comments
// (which carry the schema reference for users editing the file).
//
// If no top-level `name:` line is present, the bytes are returned
// unchanged — Resolve will fill in the filename stem.
func retargetThemeName(data []byte, newName string) []byte {
	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		// Match exactly `name: ...` at column 0 — skip indented
		// `name:` fields under nested keys.
		if strings.HasPrefix(line, "name:") {
			lines[i] = "name: " + newName
			break
		}
	}
	return []byte(strings.Join(lines, "\n"))
}
