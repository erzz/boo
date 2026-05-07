// Package cli wires up the cobra command tree for boo.
package cli

import (
	"log/slog"
	"os"

	"github.com/spf13/cobra"
)

// NewRoot returns the root cobra command. All subcommands are registered here.
func NewRoot() *cobra.Command {
	var verbose bool

	root := &cobra.Command{
		Use:   "boo [project]",
		Short: "Project launcher for Ghostty",
		Long: `boo is a project launcher for the Ghostty terminal emulator.

Switch between project-scoped Ghostty windows by name. Each project remembers
its layout (windows, tabs, splits, working directories, startup commands).

Run 'boo' with no arguments to open the interactive picker. Use
'boo <name>' to switch directly to a known project.

Run 'boo doctor' to verify your environment.`,
		SilenceUsage: true,
		PersistentPreRun: func(_ *cobra.Command, _ []string) {
			level := slog.LevelInfo
			if verbose {
				level = slog.LevelDebug
			}
			h := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})
			slog.SetDefault(slog.New(h))
		},
		RunE: runRoot,
		Args: cobra.MaximumNArgs(1),
	}

	root.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose logging")

	root.AddCommand(
		newDoctorCmd(),
		newNewCmd(),
		newListCmd(),
		newFzfCmd(),
		newDeleteCmd(),
		newSaveCmd(),
	)

	return root
}
