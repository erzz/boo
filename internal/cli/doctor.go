package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/erzz/boo/internal/config"
	"github.com/erzz/boo/internal/doctor"
	booexec "github.com/erzz/boo/internal/exec"
	"github.com/erzz/boo/internal/ghostty"
	"github.com/erzz/boo/internal/state"
	"github.com/erzz/boo/internal/theme"
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
			// Doctor must run even if config is broken — it's the
			// command users will reach for to diagnose that exact
			// situation. Resolve paths directly (not via newApp,
			// which would itself call config.Load and fail early).
			paths, err := state.Default()
			if err != nil {
				return err
			}
			checks := append(doctor.AllChecks(client),
				doctor.ConfigCheck(paths.ConfigFile, func(path string) error {
					_, _, err := config.Load(path)
					return err
				}),
				doctor.ThemesCheck(paths.ThemesDir, validateUserThemes),
			)
			results, worst := doctor.Run(ctx, checks)
			renderResults(cmd.OutOrStdout(), results)
			if worst == doctor.Fail {
				return fmt.Errorf("doctor: one or more checks failed")
			}
			return nil
		},
	}
}

// validateUserThemes returns the names of user theme files in dir
// that fail to parse. Built-ins are skipped — they're embedded and
// validated at build time. We intentionally don't surface the full
// parse error here; `boo themes` already does that. Doctor just
// flags that something is broken so the user knows to look.
func validateUserThemes(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var broken []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".yaml") {
			continue
		}
		stem := strings.TrimSuffix(name, ".yaml")
		// Resolve via the package so we exercise the same code
		// path the picker uses. The validation is just "does
		// Resolve return an error?" — simple and honest.
		if _, err := theme.Resolve(dir, stem); err != nil {
			broken = append(broken, stem)
		}
	}
	return broken, nil
}

func renderResults(w io.Writer, results []doctor.Result) {
	for _, r := range results {
		_, _ = fmt.Fprintf(w, "[%s] %s — %s\n", r.Status, r.Name, r.Detail)
		if r.Hint != "" {
			_, _ = fmt.Fprintf(w, "       hint: %s\n", r.Hint)
		}
	}
}
