package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/erzz/boo/internal/ghostty"
	"github.com/erzz/boo/internal/layout"
	"github.com/erzz/boo/internal/picker"
	"github.com/erzz/boo/internal/project"
)

// isLayoutParseError reports whether err came from layout decoding.
// The layout package wraps parse/validate errors with a "layout:" prefix.
func isLayoutParseError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "layout:")
}

func newSaveCmd() *cobra.Command { return newSaveCmdWithApp(nil) }

// newSaveCmdWithApp is like newSaveCmd but uses appIn instead of calling
// newApp() inside RunE.  Pass nil for production behaviour.
// Used by tests to inject a fake *app.
func newSaveCmdWithApp(appIn *app) *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "save [name]",
		Short: "Capture the live Ghostty layout for a project",
		Long: `Snapshot the current state of a project's Ghostty window — its tabs and
the working directory of every terminal — and write it back as the project's
saved layout. Run after rearranging windows interactively to make the new
shape persistent.

With no name argument, boo identifies the project by looking up the focused
Ghostty window and matching its ID against registered projects:

  - matched   → captures the layout for that project (this is the common case)
  - unmatched → falls through to the new-project TUI form, pre-populated
                from the focused window's working directory. Treat the
                no-args form as "save the project I'm in, or register the
                thing I'm in if it isn't a project yet."
  - no Ghostty window focused → clean error suggesting 'boo save <name>'.

What you'll see:

  - If nothing meaningful changed since the last save, boo writes the file
    and exits silently — idempotent saves don't nag.
  - If the shape changed (tabs/splits added or removed) but no information
    was lost, boo prints a side-by-side ASCII diff of just the changed
    tabs and asks to confirm.
  - Invisible-but-stable fields from the previous layout (command, env,
    initial_input, custom split direction) are carried forward by position
    when the captured shape lines up. So re-saving after rearranging
    splits with the same cwd is non-lossy and silent.
  - If splits or tabs were CLOSED that held command / env / initial_input
    / custom direction, that data CANNOT be carried forward. boo marks
    those cells with a trailing '!' in the diff, lists them under
    "Unrecoverable on next save", and asks to confirm.

Use --force to skip the confirmation in either case. The lossy diff is
still printed to stderr under --force so audit logs show what was lost.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			a := appIn
			if a == nil {
				var err error
				a, err = newApp()
				if err != nil {
					return err
				}
			}
			reg, err := project.Load(a.Paths)
			if err != nil {
				return err
			}

			// Resolve which project to save.
			var p project.Project
			if len(args) == 1 {
				name := args[0]
				if err := project.ValidateName(name); err != nil {
					return err
				}
				p, err = reg.Get(name)
				if err != nil {
					if errors.Is(err, project.ErrNotFound) {
						return fmt.Errorf("project %q not found", name)
					}
					return err
				}
			} else {
				match, err := matchFrontWindow(c.Context(), a, reg)
				if err != nil {
					return err
				}
				switch {
				case match.project != nil:
					p = *match.project
					if match.recoveredWindowID != "" {
						if rt, err := project.LoadRuntime(a.Paths, p.Name); err == nil {
							rt.WindowID = match.recoveredWindowID
							_ = project.SaveRuntime(a.Paths, p.Name, rt)
						}
					}
					_, _ = fmt.Fprintf(c.OutOrStdout(), "Detected project %q from focused Ghostty window.\n", p.Name)
				case match.unregisteredCwd != "":
					// Front Ghostty window exists but no registered project owns
					// it. Treat as "register what I'm in" and hand over to the
					// new-project flow seeded with the window's cwd.
					_, _ = fmt.Fprintf(c.OutOrStdout(), "Focused Ghostty window isn't a registered project — opening the new-project form.\n")
					defs, err := buildNewProjectDefaults(a, defaultsFromFlags{dir: match.unregisteredCwd})
					if err != nil {
						return err
					}
					res, err := picker.Run(nil, picker.Options{
						Title:                    "boo — register current window",
						Defaults:                 defs,
						SkipListGoStraightToForm: true,
						HideNewProject:           true,
						PreviewTemplate:          templatePreviewer(a),
						LayoutNames:              templateNames(a),
						Theme:                    a.Config.ThemeOr("default"),
						ThemesDir:                a.Paths.ThemesDir,
						ConfigPath:               a.Paths.ConfigFile,
					})
					if err != nil {
						return err
					}
					if res.Cancelled() {
						return nil
					}
					npi, ok := res.Intent.(picker.NewProjectIntent)
					if !ok {
						return nil
					}
					return runCreateProject(c.Context(), a, npi, c.OutOrStdout())
				}
			}

			responsive, err := projectUsesResponsiveLayout(a, p)
			if err != nil {
				return err
			}
			if responsive {
				return fmt.Errorf("project %q uses a responsive layout; 'boo save' does not support responsive layouts yet", p.Name)
			}

			rt, err := project.LoadRuntime(a.Paths, p.Name)
			if err != nil {
				return err
			}
			if rt.WindowID == "" {
				return fmt.Errorf("project %q has no live window to capture (run 'boo %s' first)", p.Name, p.Name)
			}
			exists, err := a.Ghostty.WindowExists(c.Context(), rt.WindowID)
			if err != nil {
				return err
			}
			if !exists {
				return fmt.Errorf("project %q's window %s is no longer alive (run 'boo %s' to reopen, then save)", p.Name, rt.WindowID, p.Name)
			}

			desc, err := a.Ghostty.DescribeWindow(c.Context(), rt.WindowID)
			if err != nil {
				return err
			}
			newLayout, warnings := capturedToLayout(p, desc)
			if err := newLayout.Validate(); err != nil {
				return fmt.Errorf("captured layout failed validation: %w", err)
			}

			tabs := len(newLayout.Tabs)
			leaves := 0
			for _, t := range newLayout.Tabs {
				leaves += len(collectLeaves(t.Root))
			}
			_, _ = fmt.Fprintf(c.OutOrStdout(), "Captured %d tab(s), %d pane(s) from window %s\n", tabs, leaves, rt.WindowID)
			for _, w := range warnings {
				_, _ = fmt.Fprintf(c.ErrOrStderr(), "warning: %s\n", w)
			}

			// Compare against the previously-saved layout.
			// Error handling:
			//   - missing file (first save) → treat as no previous → silent path.
			//   - parse error (corrupt file) → warn and fall through so user can recover by saving over it.
			//   - other read errors (permissions) → hard error; refusing to overwrite is safer.
			var prev layout.Layout
			loaded, perr := project.LoadLayout(a.Paths, p.Name)
			switch {
			case perr == nil:
				prev = loaded
			case errors.Is(perr, fs.ErrNotExist):
				// First save — nothing to compare against.
			case isLayoutParseError(perr):
				_, _ = fmt.Fprintf(c.ErrOrStderr(), "warning: previous layout file is unreadable (%v) — proceeding as a fresh save.\n", perr)
			default:
				return fmt.Errorf("read previous layout (refusing to overwrite): %w", perr)
			}
			diff := diffForSave(prev, newLayout)

			// Fold prev's invisible fields (command, env, initial_input, direction) into the
			// captured layout. The merged value is what we write AND what the diff reflects.
			merged, mergeLost := mergeForSave(prev, newLayout)
			if err := merged.Validate(); err != nil {
				return fmt.Errorf("merged layout failed validation: %w", err)
			}
			diff = diffForSave(prev, merged)
			if len(mergeLost) > 0 {
				// Promote to Lossy if merge dropped tabs/splits with command/env/etc.
				diff.LossReasons = append(diff.LossReasons, mergeLost...)
				if diff.Outcome != OutcomeLossy {
					diff.Outcome = OutcomeLossy
				}
			}

			proceed, err := applySaveOutcome(diff, force, c.InOrStdin(), c.OutOrStdout(), c.ErrOrStderr())
			if err != nil {
				return err
			}
			if !proceed {
				_, _ = fmt.Fprintln(c.OutOrStdout(), "aborted")
				return nil
			}

			if err := project.SaveLayout(a.Paths, p.Name, merged); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(c.OutOrStdout(), "Saved layout for %q.\n", p.Name)
			return nil
		},
	}
	cmd.Flags().BoolVarP(&force, "force", "f", false, "skip confirmation prompts (the lossy diff is still printed to stderr)")
	return cmd
}

func projectUsesResponsiveLayout(a *app, p project.Project) (bool, error) {
	saved, err := project.LoadLayout(a.Paths, p.Name)
	switch {
	case err == nil:
		return saved.IsResponsive(), nil
	case errors.Is(err, fs.ErrNotExist), isLayoutParseError(err):
		return resolveResponsiveFromTemplate(a, p, err)
	default:
		return false, fmt.Errorf("read previous layout: %w", err)
	}
}

func resolveResponsiveFromTemplate(a *app, p project.Project, snapshotErr error) (bool, error) {
	if p.Layout == "" {
		return false, nil
	}
	resolved, err := layout.ResolveTemplate(a.Paths.LayoutsDir, p.Layout)
	if err != nil {
		return false, fmt.Errorf("resolve layout template %q after previous layout read failed (%v): %w", p.Layout, snapshotErr, err)
	}
	return resolved.Layout.IsResponsive(), nil
}

// applySaveOutcome renders the diff and optionally prompts for confirmation.
// Returns (true, nil) to proceed, (false, nil) if the user declined.
// Extracted for testability — the decision matrix is the part most worth pinning down.
func applySaveOutcome(diff SaveDiff, force bool, in io.Reader, out, errOut io.Writer) (bool, error) {
	switch diff.Outcome {
	case OutcomeSilent:
		return true, nil
	case OutcomeStructural:
		renderDiff(diff, out)
		if force {
			return true, nil
		}
		return confirm(in, out, "Apply this change?")
	case OutcomeLossy:
		renderDiff(diff, errOut)
		if force {
			return true, nil
		}
		return confirm(in, out, "Save anyway and lose the unrecoverable data above?")
	default:
		// Unknown outcome: block the save rather than silently proceeding.
		return false, fmt.Errorf("internal error: unknown save outcome %v", diff.Outcome)
	}
}

// frontWindowMatch is the result of matching Ghostty's focused window against the registry.
// Exactly one of project / unregisteredCwd is populated on nil error when a window is focused.
// When no window is focused, returns a clean error (nothing actionable for `boo save`).
type frontWindowMatch struct {
	project           *project.Project
	recoveredWindowID string
	unregisteredCwd   string
}

func matchFrontWindow(ctx context.Context, a *app, reg *project.Registry) (frontWindowMatch, error) {
	id, err := a.Ghostty.FrontWindowID(ctx)
	if err != nil {
		return frontWindowMatch{}, fmt.Errorf("could not determine focused Ghostty window: %w", err)
	}
	if id == "" {
		return frontWindowMatch{}, errors.New("no focused Ghostty window detected. Run 'boo save <name>' to specify which project to save")
	}

	// 1. WindowID match — strongest signal. Same Ghostty process lifetime.
	for _, p := range reg.Projects {
		rt, err := project.LoadRuntime(a.Paths, p.Name)
		if err != nil {
			continue
		}
		if rt.WindowID == id {
			pp := p
			return frontWindowMatch{project: &pp}, nil
		}
	}

	// 2. WindowID didn't match (Ghostty restarted, window opened outside boo) —
	//    try to recover by matching the focused terminal's cwd against registered dirs.
	cwd := ""
	if desc, derr := a.Ghostty.DescribeWindow(ctx, id); derr == nil {
		cwd = firstTerminalCwd(desc)
		if cwd != "" {
			if p, err := reg.FindByDir(cwd); err == nil {
				pp := p
				return frontWindowMatch{project: &pp, recoveredWindowID: id}, nil
			}
		}
	}

	// 3. Genuinely unregistered. Fall through to the new-project form,
	//    seeded with the focused window's cwd (best-effort).
	return frontWindowMatch{unregisteredCwd: cwd}, nil
}

// firstTerminalCwd returns the working directory of the first terminal in
// the first tab of a described window, or "" if there are none.
func firstTerminalCwd(d *ghostty.DescribedWindow) string {
	if d == nil {
		return ""
	}
	for _, t := range d.Tabs {
		for _, term := range t.Terminals {
			if term.WorkingDirectory != "" {
				return term.WorkingDirectory
			}
		}
	}
	return ""
}

// capturedToLayout projects a Ghostty DescribedWindow into boo's layout vocabulary.
//
// Ghostty's AppleScript API returns only a flat terminal list per tab — no
// split tree, no command, no env. Capture always emits a flat representation:
// 1 terminal → single leaf; N terminals → right-leaning row chain.
// mergeForSave then either restores the previous tree shape (leaf counts match)
// or accepts the flat shape (counts differ, surfacing the loss in the diff).
func capturedToLayout(p project.Project, desc *ghostty.DescribedWindow) (layout.Layout, []string) {
	out := layout.Layout{Name: p.Layout}
	if out.Name == "" {
		out.Name = "triple"
	}

	var warnings []string
	for _, dt := range desc.Tabs {
		if len(dt.Terminals) == 0 {
			// Ghostty shouldn't return a tab with no terminals, but drop and warn defensively.
			warnings = append(warnings, fmt.Sprintf("tab %q had no terminals; dropped", dt.Name))
			continue
		}
		leaves := make([]layout.Split, len(dt.Terminals))
		for i, term := range dt.Terminals {
			leaves[i] = layout.Split{Cwd: relativiseCwd(p.Dir, term.WorkingDirectory)}
		}
		out.Tabs = append(out.Tabs, layout.Tab{
			Name: dt.Name,
			Root: buildFlatRoot(leaves),
		})
	}
	return out, warnings
}

// relativiseCwd returns cwd relative to projectDir if it lives under it
// ("." for the root). Otherwise returns the absolute path unchanged.
func relativiseCwd(projectDir, cwd string) string {
	if cwd == "" {
		return "."
	}
	cleanProj := filepath.Clean(projectDir)
	cleanCwd := filepath.Clean(cwd)
	if cleanCwd == cleanProj {
		return "."
	}
	rel, err := filepath.Rel(cleanProj, cleanCwd)
	if err != nil || strings.HasPrefix(rel, "..") {
		return cleanCwd
	}
	return rel
}

// confirm prompts the user for y/N. Empty answer or anything not starting
// with y/Y is treated as no.
func confirm(in io.Reader, out io.Writer, prompt string) (bool, error) {
	if _, err := fmt.Fprintf(out, "%s [y/N] ", prompt); err != nil {
		return false, err
	}
	buf := make([]byte, 16)
	n, _ := in.Read(buf) // EOF / empty answer → no
	answer := strings.TrimSpace(string(buf[:n]))
	return strings.HasPrefix(strings.ToLower(answer), "y"), nil
}
