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

// isLayoutParseError reports whether err came from layout decoding (YAML
// parse / validate) rather than the underlying filesystem. The layout
// package wraps both with the prefix "layout:", so a substring check is
// sufficient and keeps us from leaking knowledge of the parser internals.
func isLayoutParseError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "layout:")
}

func newSaveCmd() *cobra.Command {
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
			a, err := newApp()
			if err != nil {
				return err
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
					})
					if err != nil {
						return err
					}
					if res.Cancelled() || res.NewProject == nil {
						return nil
					}
					return runCreateProject(c.Context(), a, *res.NewProject, c.OutOrStdout())
				}
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

			// Compare against the previously-saved layout so we only nag the
			// user when something actually changed (and especially when
			// something that boo can't recover is about to be wiped).
			//
			// Error handling here is deliberately narrow:
			//   - missing file (first save) → treat as no previous → silent path.
			//   - parse error (corrupt or hand-edited junk) → warn loudly and
			//     fall through to the full diff/prompt against an empty
			//     previous, so the user can recover by saving over the
			//     corrupt file rather than being blocked by it.
			//   - other read errors (permissions, I/O) → hard error. We are
			//     about to overwrite the same file; if we can't even read
			//     it, refusing to save is safer than silently clobbering.
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

			// Fold prev's invisible-but-stable fields (command, env,
			// initial_input, non-default direction) into the captured
			// layout so common re-saves stop being lossy. The merged
			// value is what we write AND what the diff reflects, so the
			// user sees an honest before/after.
			merged, mergeLost := mergeForSave(prev, newLayout)
			if err := merged.Validate(); err != nil {
				return fmt.Errorf("merged layout failed validation: %w", err)
			}
			diff = diffForSave(prev, merged)
			if len(mergeLost) > 0 {
				// Anything the merge couldn't carry (dropped tabs/splits
				// that held command/env/etc.) gets surfaced as an extra
				// loss reason. Promote the outcome to Lossy if it isn't
				// already, so the user is prompted before we destroy
				// data.
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

// applySaveOutcome handles the user-facing side of an honest-save: render
// the diff (when relevant), prompt for confirmation (unless --force), and
// report whether the caller should proceed with writing the layout.
//
// Three branches mirror diffForSave's outcomes:
//   - Silent     → no output, no prompt, proceed.
//   - Structural → render diff to stdout, prompt unless force.
//   - Lossy      → render diff to STDERR (so --force still leaves a trace
//                  for scripts/CI), prompt unless force.
//
// Returning (false, nil) means the user declined; the caller is responsible
// for printing "aborted" in whatever style fits its surrounding output.
// Errors only come from the prompt's IO; any error means the save is
// aborted and surfaced to the user.
//
// Extracted out of RunE so it can be tested without standing up a full app
// (which would require a Ghostty fake, a temp paths root, and a registry).
// The decision matrix here is the part most worth pinning down with tests;
// the rest of RunE is straight-line plumbing.
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
		// Defensive: an unknown outcome shouldn't reach here, but if it
		// does, prefer "block the save and tell the user" over silently
		// proceeding. This mirrors the rest of save.go's "refuse to
		// overwrite when uncertain" stance.
		return false, fmt.Errorf("internal error: unknown save outcome %v", diff.Outcome)
	}
}

// frontWindowMatch is the result of matching Ghostty's focused window
// against the project registry.
//
// Exactly one of project / unregisteredCwd is populated when the function
// returns nil error AND a window was focused:
//   - project        → focused window belongs to this registered project
//   - unregisteredCwd → focused window exists but no registered project
//                       owns it; cwd is the focused terminal's working dir
//                       (best-effort; may be "" if Describe failed)
//
// When no Ghostty window is focused, the function returns a clean error
// rather than an empty match — there's nothing actionable to do in that
// case for `boo save`.
type frontWindowMatch struct {
	project         *project.Project
	unregisteredCwd string
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

	// 2. WindowID didn't match (Ghostty restarted, window opened outside
	//    boo, etc.) — try to recover by matching the focused terminal's
	//    working directory against registered project dirs. This is the
	//    common case after a Ghostty restart: the runtime files still hold
	//    stale IDs but the user is clearly *in* the project's directory.
	cwd := ""
	if desc, derr := a.Ghostty.DescribeWindow(ctx, id); derr == nil {
		cwd = firstTerminalCwd(desc)
		if cwd != "" {
			if p, err := reg.FindByDir(cwd); err == nil {
				// Refresh the runtime WindowID so the rest of `save` works
				// against this window. Without this update, the caller would
				// hit the "no live window to capture" branch.
				if rt, rerr := project.LoadRuntime(a.Paths, p.Name); rerr == nil {
					rt.WindowID = id
					_ = project.SaveRuntime(a.Paths, p.Name, rt)
				}
				pp := p
				return frontWindowMatch{project: &pp}, nil
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

// capturedToLayout projects a Ghostty DescribedWindow into boo's layout
// vocabulary as a flat tree.
//
// Why flat
// --------
// Ghostty's AppleScript dictionary returns a per-tab list of terminals
// with no information about how they're nested or split. We have no
// way to recover the tree shape from a live capture, so capture always
// emits the canonical flat representation:
//
//   - 1 terminal in a tab → a single leaf as the tab's root.
//   - N terminals       → a right-leaning chain of `row` splits
//     (built by save_merge's buildFlatRoot).
//
// mergeForSave is then responsible for either preserving the previous
// tab's tree shape (when leaf counts match) or accepting this flat
// shape (when they don't, surfacing the loss in the diff).
//
// The conversion rules:
//   - Each captured tab becomes one layout.Tab.
//   - Each terminal becomes one leaf (cwd from terminal.WorkingDirectory,
//     made relative to the project root if it lives under it).
//   - Tab names are preserved when non-empty.
//
// What is NOT captured (use the diff to surface contextually):
//   - command (Ghostty doesn't expose the original launch command)
//   - env    (likewise)
//   - tree shape (Ghostty's API gives a flat list)
//   - initial_input
func capturedToLayout(p project.Project, desc *ghostty.DescribedWindow) (layout.Layout, []string) {
	out := layout.Layout{Name: p.Layout}
	if out.Name == "" {
		out.Name = "triple"
	}

	var warnings []string
	for _, dt := range desc.Tabs {
		// A tab must have at least one terminal for the layout to
		// validate. Defensive: Ghostty shouldn't return a tab with no
		// terminals, but if it does we drop the tab and warn rather
		// than producing an invalid layout.
		if len(dt.Terminals) == 0 {
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

// relativiseCwd returns cwd as a path relative to projectDir if it lives
// under it (cleaner layout files; "." for the root). Otherwise the absolute
// path is returned unchanged. Symlinks are not resolved — Ghostty's reported
// cwd may already be canonicalised.
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
