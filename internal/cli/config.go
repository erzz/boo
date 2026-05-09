package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/spf13/cobra"

	"github.com/erzz/boo/internal/state"
)

// newConfigCmd builds the `boo config` subcommand tree.
// The bare `boo config` is an alias for `boo config show`.
func newConfigCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Inspect and edit boo's global config",
		Long: `Inspect and edit boo's global config (~/.config/boo/config.yaml).

  boo config        # alias for 'boo config show'
  boo config show   # print effective config + the source of each value
  boo config path   # print the config file path (whether or not it exists)
  boo config edit   # open the config file in $EDITOR (creating it if needed)`,
		RunE: func(c *cobra.Command, args []string) error {
			if asJSON {
				return runConfigShowJSON(c.OutOrStdout())
			}
			return runConfigShow(c.OutOrStdout())
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "(with bare 'boo config' or 'show') emit JSON")
	cmd.AddCommand(
		newConfigShowCmd(),
		newConfigPathCmd(),
		newConfigEditCmd(),
	)
	return cmd
}

func newConfigShowCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "show",
		Short: "Print the effective config and where each value came from",
		RunE: func(c *cobra.Command, _ []string) error {
			if asJSON {
				return runConfigShowJSON(c.OutOrStdout())
			}
			return runConfigShow(c.OutOrStdout())
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit JSON instead of the human table")
	return cmd
}

func newConfigPathCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "path",
		Short: "Print the config file path",
		// NOTE: does NOT call newApp() so a malformed config doesn't block the one command
		// the user would run to find and fix it.
		RunE: func(c *cobra.Command, _ []string) error {
			p, err := state.Default()
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintln(c.OutOrStdout(), p.ConfigFile)
			return nil
		},
	}
}

func newConfigEditCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "edit",
		Short: "Open the config file in $EDITOR",
		Long: `Open ~/.config/boo/config.yaml in $EDITOR.

If the file doesn't exist yet, an empty file is created first so the
editor has something to open. Edits are not validated until boo next
loads the file — if you save broken YAML, the next boo command will
error.

$EDITOR is required. $VISUAL is honoured if $EDITOR is unset.`,
		// NOTE: does NOT call newApp() so a malformed config doesn't block the recovery command.
		RunE: func(c *cobra.Command, _ []string) error {
			p, err := state.Default()
			if err != nil {
				return err
			}
			if err := os.MkdirAll(p.ConfigDir, 0o755); err != nil {
				return fmt.Errorf("create config dir %s: %w", p.ConfigDir, err)
			}
			path := p.ConfigFile
			if _, err := os.Stat(path); os.IsNotExist(err) {
				if err := os.WriteFile(path, nil, 0o644); err != nil {
					return fmt.Errorf("create %s: %w", path, err)
				}
			}
			return openInEditor("", path)
		},
	}
}

// runConfigShow prints the effective config as key/value pairs with source annotations.
// Output is human-readable only — not a YAML doc (source annotations make round-tripping fragile).
func runConfigShow(w io.Writer) error {
	a, err := newApp()
	if err != nil {
		return err
	}

	rows := []struct{ key, value, source string }{
		{"default_layout", a.Config.DefaultLayoutOr(""), a.ConfigSrcs["default_layout"]},
		{"projects_dir", a.Config.ProjectsDirOr(""), a.ConfigSrcs["projects_dir"]},
		{"git.default_remote", a.Config.GitDefaultRemoteOr(""), a.ConfigSrcs["git.default_remote"]},
		{"ui.theme", a.Config.ThemeOr(""), a.ConfigSrcs["ui.theme"]},
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].key < rows[j].key })

	keyW, valW := 0, 0
	for _, r := range rows {
		if len(r.key) > keyW {
			keyW = len(r.key)
		}
		if len(r.value) > valW {
			valW = len(r.value)
		}
	}

	_, _ = fmt.Fprintf(w, "Config file: %s\n\n", a.Paths.ConfigFile)
	for _, r := range rows {
		val := r.value
		if val == "" {
			val = "(unset)"
		}
		_, _ = fmt.Fprintf(w, "  %-*s  %-*s  [%s]\n", keyW, r.key, valW, val, r.source)
	}
	return nil
}

// configJSONField pairs a config value with its source ("factory" or file path).
type configJSONField struct {
	Value  string `json:"value"`
	Source string `json:"source"`
}

// configJSONOut is the wire shape of `boo config show --json`. Keys use the same dotted paths as the human renderer.
type configJSONOut struct {
	ConfigFile string                     `json:"config_file"`
	Values     map[string]configJSONField `json:"values"`
}

func runConfigShowJSON(w io.Writer) error {
	a, err := newApp()
	if err != nil {
		return err
	}
	out := configJSONOut{
		ConfigFile: a.Paths.ConfigFile,
		Values: map[string]configJSONField{
			"default_layout":     {Value: a.Config.DefaultLayoutOr(""), Source: a.ConfigSrcs["default_layout"]},
			"projects_dir":       {Value: a.Config.ProjectsDirOr(""), Source: a.ConfigSrcs["projects_dir"]},
			"git.default_remote": {Value: a.Config.GitDefaultRemoteOr(""), Source: a.ConfigSrcs["git.default_remote"]},
			"ui.theme":           {Value: a.Config.ThemeOr(""), Source: a.ConfigSrcs["ui.theme"]},
		},
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}
