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
		newLayoutsCmd(),
		newConfigCmd(),
		newThemesCmd(),
		newEditCmd(),
		newSetLayoutCmd(),
		newShowCmd(),
	)

	// Shell completion for project names. Cobra calls this with whatever
	// the user has typed so far ("toComplete") and the prior args. We
	// return all registered project names — cobra filters by prefix.
	//
	// We wire the same function on every command that takes a <name> as
	// its first argument: bare boo (switch), delete, save, edit,
	// set-layout, show. `set-layout` also has a second positional
	// (template) which we wire separately below.
	completeProjectNames := func(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
		// On commands that take exactly one project name, suppress
		// completion past the first arg.
		if len(args) >= 1 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return projectNamesForCompletion(), cobra.ShellCompDirectiveNoFileComp
	}
	completeTemplateNames := func(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
		if len(args) >= 2 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return templateNamesForCompletion(), cobra.ShellCompDirectiveNoFileComp
	}

	root.ValidArgsFunction = completeProjectNames
	for _, sub := range root.Commands() {
		switch sub.Name() {
		case "delete", "save", "edit", "show":
			sub.ValidArgsFunction = completeProjectNames
		case "set-layout":
			sub.ValidArgsFunction = func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
				if len(args) == 0 {
					return projectNamesForCompletion(), cobra.ShellCompDirectiveNoFileComp
				}
				return completeTemplateNames(cmd, args, toComplete)
			}
		case "new":
			// `boo new` takes a name but it's a NEW name — no
			// useful completion candidates. Suppress file
			// completion so the shell doesn't offer files.
			sub.ValidArgsFunction = func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
		}
	}

	return root
}
