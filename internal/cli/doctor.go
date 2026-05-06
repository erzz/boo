package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	booexec "github.com/sean-erswell-liljefelt/boo/internal/exec"
	"github.com/sean-erswell-liljefelt/boo/internal/doctor"
	"github.com/sean-erswell-liljefelt/boo/internal/ghostty"
)

func newDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check that your environment is set up to run boo",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			runner := booexec.NewReal()
			client := ghostty.New(runner)
			results, worst := doctor.Run(ctx, doctor.AllChecks(client))
			renderResults(cmd.OutOrStdout(), results)
			if worst == doctor.Fail {
				return fmt.Errorf("doctor: one or more checks failed")
			}
			return nil
		},
	}
}

func renderResults(w io.Writer, results []doctor.Result) {
	for _, r := range results {
		fmt.Fprintf(w, "[%s] %s — %s\n", r.Status, r.Name, r.Detail)
		if r.Hint != "" {
			fmt.Fprintf(w, "       hint: %s\n", r.Hint)
		}
	}
}
