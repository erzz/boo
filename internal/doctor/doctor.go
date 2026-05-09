// Package doctor runs environment sanity checks for boo.
//
// Checks are evaluated in order. Some checks short-circuit later ones (e.g.
// if Ghostty is not installed there is no point probing it). Each check
// returns a Result; the worst Status across all checks becomes the overall
// outcome.
package doctor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/erzz/boo/internal/ghostty"
)

// Status represents the outcome of a single check.
type Status int

const (
	OK Status = iota
	Skip
	Warn
	Fail
)

func (s Status) String() string {
	switch s {
	case OK:
		return "OK"
	case Skip:
		return "SKIP"
	case Warn:
		return "WARN"
	case Fail:
		return "FAIL"
	default:
		return "?"
	}
}

// Result is what a Check produces.
type Result struct {
	Name   string
	Status Status
	Detail string
	Hint   string
}

// supportedGhosttyMin is the minimum Ghostty version we've tested against.
// Older versions may work but the JXA API surface boo relies on first
// stabilized in 1.3.x.
const supportedGhosttyMin = "1.3.0"

// CheckFunc takes the running set of results so far and returns the next
// result. This lets later checks short-circuit based on earlier outcomes.
type CheckFunc func(ctx context.Context, prior []Result) Result

// AllChecks returns the default set of checks in display order.
func AllChecks(client *ghostty.Client) []CheckFunc {
	return []CheckFunc{
		checkPlatform,
		checkGhosttyInstalled,
		ghosttyRunningCheck(client),
		ghosttyVersionRangeCheck(),
		ghosttyAutomationCheck(client),
		checkFzfOptional,
	}
}

// ConfigCheck returns a check that loads the user config at path and
// reports whether it parses cleanly.
//
//   - File missing → SKIP ("using factory defaults").
//   - File parses → OK.
//   - File present but malformed → FAIL with the parse error and a
//     pointer to `boo config edit` for the obvious next step.
//
// Wired separately from AllChecks because the doctor package shouldn't
// depend on internal/config (it's a leaf package); the CLI assembles
// the loader and passes a CheckFunc in.
func ConfigCheck(path string, load func(path string) error) CheckFunc {
	return func(_ context.Context, _ []Result) Result {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return Result{
				Name:   "config",
				Status: Skip,
				Detail: "no config.yaml — using factory defaults",
				Hint:   "run 'boo config edit' to create one",
			}
		}
		if err := load(path); err != nil {
			return Result{
				Name:   "config",
				Status: Fail,
				Detail: err.Error(),
				Hint:   "fix the YAML or run 'boo config edit' to inspect",
			}
		}
		return Result{Name: "config", Status: OK, Detail: path}
	}
}

// ThemesCheck reports whether each user theme in themesDir parses
// cleanly. Themes are cosmetic, so broken themes are WARN rather than
// FAIL — boo still launches, falling back to the default theme.
//
// Like ConfigCheck, this is wired separately so the doctor package
// stays free of an internal/theme dependency. The CLI passes in a
// list-and-validate function.
//
//   - themesDir doesn't exist → SKIP ("no user themes").
//   - All user themes parse → OK.
//   - One or more themes fail to parse → WARN with the first error
//     surfaced; `boo themes` shows the full list inline with [error].
func ThemesCheck(themesDir string, validate func(dir string) (broken []string, err error)) CheckFunc {
	return func(_ context.Context, _ []Result) Result {
		if _, err := os.Stat(themesDir); os.IsNotExist(err) {
			return Result{
				Name:   "themes",
				Status: Skip,
				Detail: "no user themes — using built-ins",
			}
		}
		broken, err := validate(themesDir)
		if err != nil {
			return Result{
				Name:   "themes",
				Status: Warn,
				Detail: "could not list user themes: " + err.Error(),
				Hint:   "run 'boo themes' to inspect",
			}
		}
		if len(broken) == 0 {
			return Result{Name: "themes", Status: OK, Detail: themesDir}
		}
		return Result{
			Name:   "themes",
			Status: Warn,
			Detail: fmt.Sprintf("%d broken theme(s): %s", len(broken), strings.Join(broken, ", ")),
			Hint:   "run 'boo themes' to see the parse errors; the picker falls back to the default theme",
		}
	}
}

// Run executes all checks in order and returns the results plus the worst
// non-Skip status.
func Run(ctx context.Context, checks []CheckFunc) ([]Result, Status) {
	results := make([]Result, 0, len(checks))
	worst := OK
	for _, c := range checks {
		r := c(ctx, results)
		if r.Status > worst && r.Status != Skip {
			worst = r.Status
		}
		results = append(results, r)
	}
	return results, worst
}

func checkPlatform(_ context.Context, _ []Result) Result {
	if runtimeGOOS() != "darwin" {
		return Result{
			Name:   "platform",
			Status: Fail,
			Detail: fmt.Sprintf("boo currently supports macOS only (detected %s)", runtimeGOOS()),
			Hint:   "Linux/Windows support is not yet implemented.",
		}
	}
	return Result{Name: "platform", Status: OK, Detail: "macOS"}
}

func checkGhosttyInstalled(_ context.Context, _ []Result) Result {
	if path, err := exec.LookPath("ghostty"); err == nil {
		return Result{Name: "ghostty installed", Status: OK, Detail: path}
	}
	if _, err := os.Stat("/Applications/Ghostty.app"); err == nil {
		return Result{
			Name:   "ghostty installed",
			Status: OK,
			Detail: "/Applications/Ghostty.app (not on PATH)",
			Hint:   "Add Ghostty's bin to PATH to use the ghostty CLI directly.",
		}
	}
	return Result{
		Name:   "ghostty installed",
		Status: Fail,
		Detail: "Ghostty not found on PATH or in /Applications",
		Hint:   "Install from https://ghostty.org",
	}
}

// previousFailed returns true if any prior check named with one of the given
// names produced a Fail result. Used to short-circuit dependent checks.
func previousFailed(prior []Result, names ...string) bool {
	for _, r := range prior {
		if r.Status != Fail {
			continue
		}
		for _, n := range names {
			if r.Name == n {
				return true
			}
		}
	}
	return false
}

// previousResult returns the result for a named prior check, if any.
func previousResult(prior []Result, name string) (Result, bool) {
	for _, r := range prior {
		if r.Name == name {
			return r, true
		}
	}
	return Result{}, false
}

func ghosttyRunningCheck(client *ghostty.Client) CheckFunc {
	return func(ctx context.Context, prior []Result) Result {
		if previousFailed(prior, "ghostty installed") {
			return Result{Name: "ghostty running", Status: Skip, Detail: "skipped (Ghostty not installed)"}
		}
		_, err := client.Version(ctx)
		if err == nil {
			return Result{Name: "ghostty running", Status: OK, Detail: "responding to AppleScript"}
		}
		msg := err.Error()
		if isGhosttyNotRunning(msg) {
			return Result{
				Name:   "ghostty running",
				Status: Warn,
				Detail: "Ghostty is installed but not running",
				Hint:   "Launch Ghostty once; boo will then drive it via AppleScript.",
			}
		}
		// Anything else: surface the raw error and let downstream checks decide.
		return Result{
			Name:   "ghostty running",
			Status: Fail,
			Detail: msg,
			Hint:   "Run `osascript -l JavaScript -e 'Application(\"Ghostty\").version()'` to reproduce.",
		}
	}
}

func ghosttyVersionRangeCheck() CheckFunc {
	return func(_ context.Context, prior []Result) Result {
		runR, ok := previousResult(prior, "ghostty running")
		if !ok || runR.Status == Fail || runR.Status == Skip {
			return Result{Name: "ghostty version", Status: Skip, Detail: "skipped (Ghostty not responsive)"}
		}
		if runR.Status == Warn {
			// Not running yet; we can't check version.
			return Result{Name: "ghostty version", Status: Skip, Detail: "skipped (Ghostty not running)"}
		}
		// We have an OK responsive result, but the version string is in its Detail
		// only as "responding to AppleScript". Re-derive — in the future the running
		// check should propagate the version. For now we use the same client via a
		// closure factory; here we just trust the previous OK and parse its detail
		// is not feasible. Leave as a TODO marker by reporting OK with a hint.
		// TODO(boo): plumb version string from the running check into this one.
		return Result{
			Name:   "ghostty version",
			Status: OK,
			Detail: "version compatibility check pending plumbing",
			Hint:   fmt.Sprintf("boo is tested against Ghostty >= %s.", supportedGhosttyMin),
		}
	}
}

func ghosttyAutomationCheck(client *ghostty.Client) CheckFunc {
	return func(ctx context.Context, prior []Result) Result {
		runR, ok := previousResult(prior, "ghostty running")
		if !ok || runR.Status != OK {
			return Result{Name: "automation permission", Status: Skip, Detail: "skipped (Ghostty not responsive)"}
		}
		// Probe a write action by counting windows. The 'count' standard command
		// requires the same Automation permission as creating windows but doesn't
		// open any UI. If permission is denied we get a -1743 error.
		err := client.ProbeAutomation(ctx)
		if err == nil {
			return Result{Name: "automation permission", Status: OK, Detail: "granted"}
		}
		msg := err.Error()
		if isAutomationDenied(msg) {
			return Result{
				Name:   "automation permission",
				Status: Fail,
				Detail: "boo cannot send write actions to Ghostty",
				Hint:   "Grant your terminal access in System Settings → Privacy & Security → Automation → <your terminal> → Ghostty.",
			}
		}
		return Result{
			Name:   "automation permission",
			Status: Warn,
			Detail: "automation probe failed: " + msg,
			Hint:   "boo may still work; try `boo new` and watch for permission prompts.",
		}
	}
}

func isGhosttyNotRunning(msg string) bool {
	// JXA returns "Application can't be found" when Ghostty isn't running and
	// AppleScript can't even resolve the bundle. Other shapes: "isn't running".
	return strings.Contains(msg, "isn't running") ||
		strings.Contains(msg, "Application can't be found")
}

// checkFzfOptional reports whether fzf is available. fzf is optional —
// `boo pick` works fine without it — so a missing fzf is Skip, not Warn.
func checkFzfOptional(_ context.Context, _ []Result) Result {
	if path, err := exec.LookPath("fzf"); err == nil {
		return Result{Name: "fzf (optional)", Status: OK, Detail: path}
	}
	return Result{
		Name:   "fzf (optional)",
		Status: Skip,
		Detail: "fzf not found on PATH",
		Hint:   "Install fzf to use 'boo pick --fzf'. The built-in picker works without it.",
	}
}

func isAutomationDenied(msg string) bool {
	// macOS Automation denial surfaces as error -1743 ("Not authorized to send
	// Apple events to ...") or text containing "Not authorised" / "Not authorized".
	return strings.Contains(msg, "-1743") ||
		strings.Contains(strings.ToLower(msg), "not authorised") ||
		strings.Contains(strings.ToLower(msg), "not authorized")
}
