package cli

import (
	"context"
	"encoding/json"
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

func newDoctorCmd() *cobra.Command { return newDoctorCmdWithRunner(nil) }

// newDoctorCmdWithRunner is like newDoctorCmd but injects runnerIn for tests. Pass nil for production.
func newDoctorCmdWithRunner(runnerIn booexec.Runner) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check that your environment is set up to run boo",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			runner := runnerIn
			if runner == nil {
				runner = booexec.NewReal()
			}
			client := ghostty.New(runner)
			// Doctor must run even if config is broken — resolve paths directly, not via newApp.
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
			if jsonOut {
				if err := renderResultsJSON(cmd.OutOrStdout(), results); err != nil {
					return err
				}
			} else {
				renderResults(cmd.OutOrStdout(), results)
			}
			if worst == doctor.Fail {
				return fmt.Errorf("doctor: one or more checks failed")
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "output check results as a JSON array")
	return cmd
}

// validateUserThemes returns the names of user theme files in dir that fail to parse.
// Built-ins are skipped (embedded, validated at build time). Doctor uses this to flag
// broken user themes; `boo themes` shows the full parse error.
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
		// Resolve via the same code path the picker uses.
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

// doctorResultJSON is the JSON shape for a single check result in `boo doctor --json`.
type doctorResultJSON struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail"`
	Hint   string `json:"hint,omitempty"`
}

// renderResultsJSON writes check results as a JSON array. Exit-code semantics are the caller's concern.
func renderResultsJSON(w io.Writer, results []doctor.Result) error {
	out := make([]doctorResultJSON, len(results))
	for i, r := range results {
		out[i] = doctorResultJSON{
			Name:   r.Name,
			Status: r.Status.String(),
			Detail: r.Detail,
			Hint:   r.Hint,
		}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}
