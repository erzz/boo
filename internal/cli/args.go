package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// exactArgsWithNames returns a cobra positional-args validator that mirrors
// cobra.ExactArgs but produces a much more helpful error message naming each
// expected argument. Cobra's default ("accepts 1 arg(s), received 0") tells
// the user nothing about *what* the missing argument represents.
//
// Example:
//
//	exactArgsWithNames("name") // expects 1 arg called "name"
//
// On failure the user sees:
//
//	Error: 'boo new' requires exactly 1 argument: <name>
//	Run 'boo new --help' for details.
func exactArgsWithNames(names ...string) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) == len(names) {
			return nil
		}
		return missingArgsError(cmd, names, args)
	}
}

func missingArgsError(cmd *cobra.Command, expected []string, got []string) error {
	placeholders := make([]string, len(expected))
	for i, n := range expected {
		placeholders[i] = "<" + n + ">"
	}
	verb := "arguments"
	if len(expected) == 1 {
		verb = "argument"
	}
	return fmt.Errorf(
		"'%s' requires exactly %d %s: %s (got %d).\nRun '%s --help' for details",
		cmd.CommandPath(),
		len(expected),
		verb,
		joinSpaced(placeholders),
		len(got),
		cmd.CommandPath(),
	)
}

func joinSpaced(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += " "
		}
		out += p
	}
	return out
}
