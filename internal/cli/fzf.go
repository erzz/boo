package cli

import (
	"github.com/spf13/cobra"
)

// newFzfCmd is the explicit entry point for fzf-based selection. Equivalent
// to running bare `boo` outside of any registered project directory, except
// it always uses fzf instead of the built-in TUI picker.
func newFzfCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "fzf",
		Short: "Pick a project with fzf and switch to it",
		Long: `Pipe the project list into fzf for selection. fzf must be on $PATH.

This is the same flow as running 'boo' with no arguments outside a registered
project directory, except it forces fzf instead of the built-in TUI picker.

Cancelling fzf (Esc / Ctrl-C / no match) is a no-op.`,
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			a, err := newApp()
			if err != nil {
				return err
			}
			return runPicker(c.Context(), a, pickerFzf, c.OutOrStdout())
		},
	}
}
