package cli

import (
	"errors"

	"github.com/spf13/cobra"
)

func newPickCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "pick",
		Short: "Open the TUI picker to switch projects",
		RunE: func(_ *cobra.Command, _ []string) error {
			return errors.New("boo pick is not implemented yet (Phase 3)")
		},
	}
}
