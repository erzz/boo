package cli

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/erzz/boo/internal/ghostty"
	"github.com/erzz/boo/internal/layout"
	"github.com/erzz/boo/internal/project"
)

// runRoot dispatches the bare 'boo' / 'boo <name>' invocation.
//
//   - 'boo <name>' switches to that project (focus existing window or open new).
//   - 'boo' (no args) always opens the built-in TUI picker. We deliberately
//     do NOT cwd-detect-and-switch: if the user is already sitting inside a
//     project's Ghostty window, that path would just reopen the window they
//     are typing into. The picker is the one obvious behaviour for "no args".
func runRoot(cmd *cobra.Command, args []string) error {
	a, err := newApp()
	if err != nil {
		return err
	}

	if len(args) == 1 {
		reg, err := project.Load(a.Paths)
		if err != nil {
			return err
		}
		name := args[0]
		if err := project.ValidateName(name); err != nil {
			return err
		}
		p, err := reg.Get(name)
		if err != nil {
			if errors.Is(err, project.ErrNotFound) {
				return fmt.Errorf("project %q not found. Create it with: boo new %s --dir <path>", name, name)
			}
			return err
		}
		return switchToProject(cmd.Context(), a, p)
	}

	// No args: always open the picker.
	return runPicker(cmd.Context(), a, pickerTUI, cmd.OutOrStdout())
}

// switchToProject focuses the project's existing window if alive, otherwise
// opens a fresh one. Cold-starts Ghostty if needed.
//
// On any "Ghostty isn't running" error we attempt EnsureRunning once and retry
// the operation. After cold-start, any saved WindowID is stale by definition
// (process-lifetime IDs).
func switchToProject(ctx context.Context, a *app, p project.Project) error {
	if ctx == nil {
		ctx = context.Background()
	}
	rt, err := project.LoadRuntime(a.Paths, p.Name)
	if err != nil {
		return err
	}

	// 1. Try to focus an existing window if we think one exists.
	if rt.WindowID != "" {
		exists, err := a.Ghostty.WindowExists(ctx, rt.WindowID)
		switch {
		case err == nil && exists:
			if err := a.Ghostty.FocusWindow(ctx, rt.WindowID); err != nil {
				return err
			}
			return updateLaunchTime(a, p.Name, rt)
		case err == nil && !exists:
			// Window died; fall through to reopen with the same Ghostty server.
			rt.WindowID = ""
		case ghostty.IsNotRunning(err):
			// Ghostty itself is gone; cold-start invalidates all stored window IDs.
			if err := a.Ghostty.EnsureRunning(ctx); err != nil {
				return err
			}
			rt.WindowID = ""
		default:
			return err
		}
	}

	// 2. Open a fresh window.
	res, err := openProjectWindow(ctx, a, p)
	if ghostty.IsNotRunning(err) {
		if err := a.Ghostty.EnsureRunning(ctx); err != nil {
			return err
		}
		res, err = openProjectWindow(ctx, a, p)
	}
	if err != nil {
		return err
	}
	rt.WindowID = res.WindowID
	return updateLaunchTime(a, p.Name, rt)
}

func openProjectWindow(ctx context.Context, a *app, p project.Project) (*ghostty.OpenWindowResult, error) {
	l, err := loadOrRegenerateLayout(a, p)
	if err != nil {
		return nil, err
	}
	return a.Ghostty.OpenLayout(ctx, layoutToParams(p.Dir, l))
}

// loadOrRegenerateLayout reads the per-project layout snapshot, falling
// back to re-resolving from the registry's template name and writing a
// fresh snapshot when the snapshot file is missing.
//
// The snapshot is a derived artifact: the registry's `Layout` field is
// the source of truth for "which template this project uses", and
// `layout.ResolveTemplate` is the deterministic builder. So a missing
// snapshot is recoverable — we just regenerate it. This handles:
//
//   - Pre-1.0 users with stale .toml snapshots that got cleaned up.
//   - Anyone who blew away ~/.local/share/boo by accident.
//   - Future schema bumps where the on-disk snapshot version is too old.
//
// Hard errors (parse failures on a present file, missing template) still
// propagate — we only treat "file does not exist" as recoverable.
func loadOrRegenerateLayout(a *app, p project.Project) (layout.Layout, error) {
	l, err := project.LoadLayout(a.Paths, p.Name)
	if err == nil {
		return l, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return layout.Layout{}, err
	}

	// Snapshot missing — regenerate from the template recorded in the registry.
	tplName := p.Layout
	if tplName == "" {
		tplName = "triple" // matches picker.hardcodedFallbackLayout
	}
	resolved, terr := layout.ResolveTemplate(a.Paths.LayoutsDir, tplName)
	if terr != nil {
		// Surface both errors so the user understands why we couldn't
		// auto-recover (snapshot missing AND template no longer
		// resolvable — e.g. user-defined template was deleted).
		return layout.Layout{}, fmt.Errorf("layout snapshot missing for project %q and template %q is unresolvable: %w", p.Name, tplName, terr)
	}
	regenerated := resolved.Layout
	if regenerated.Name == "" {
		regenerated.Name = tplName
	}
	if werr := project.SaveLayout(a.Paths, p.Name, regenerated); werr != nil {
		return layout.Layout{}, fmt.Errorf("regenerate layout snapshot for %q: %w", p.Name, werr)
	}
	slog.Info("regenerated missing layout snapshot",
		"project", p.Name, "template", tplName, "path", a.Paths.ProjectLayoutFile(p.Name))
	return regenerated, nil
}

// layoutToParams projects a layout.Layout into the JSON shape OpenLayout
// expects, resolving every leaf's cwd against the project root.
func layoutToParams(projectDir string, l layout.Layout) ghostty.OpenLayoutParams {
	tabs := make([]ghostty.LayoutTab, len(l.Tabs))
	for i, t := range l.Tabs {
		tabs[i] = ghostty.LayoutTab{
			Name: t.Name,
			Root: splitToParams(projectDir, t.Root),
		}
	}
	return ghostty.OpenLayoutParams{Tabs: tabs}
}

// splitToParams recursively converts a layout.Split tree into the
// ghostty.LayoutSplit tree the JXA walker consumes. Leaves get their cwd
// resolved against the project root; interior nodes carry direction +
// recursively-converted children only.
//
// The user-facing layout schema has both `command` and `initial_input`
// fields, but Ghostty exposes only one knob we want to use:
// `cfg.initialInput`, which Ghostty types into the surface AFTER it
// has launched the user's default shell normally. The other knob,
// `cfg.command`, makes Ghostty hard-code `/bin/bash --noprofile --norc
// -c "exec -l <cmd>"` — that strips PATH, ignores $SHELL, and exits
// the surface when the command exits. None of those are what users
// expect from "run nvim in this pane".
//
// So we fold both fields into a single string that gets typed into the
// shell: the command (with a trailing newline so it executes) followed
// by the initial input (typed but not auto-executed, as before). This
// gives users their default shell, full PATH, and a prompt to return
// to when the command exits — exactly like opening Ghostty manually
// and typing the command.
//
// The field is still named Command in the YAML for clarity (users
// think of it as "the command to run", not "keystrokes to inject").
func splitToParams(projectDir string, s layout.Split) ghostty.LayoutSplit {
	if s.IsLeaf() {
		return ghostty.LayoutSplit{
			WorkingDirectory: resolveLayoutCwd(projectDir, s.Cwd),
			InitialInput:     mergeCommandAndInput(s.Command, s.InitialInput),
			Env:              s.Env,
		}
	}
	children := make([]ghostty.LayoutSplit, len(s.Children))
	for i, c := range s.Children {
		children[i] = splitToParams(projectDir, c)
	}
	return ghostty.LayoutSplit{
		Direction: s.Direction,
		Children:  children,
	}
}

// mergeCommandAndInput composes the leaf's command and initial_input
// into the single keystroke payload Ghostty's `cfg.initialInput` will
// type into the freshly-spawned shell.
//
//   - command alone:        "<command>\n"        (executes, then prompt)
//   - initial_input alone:  "<input>"            (typed at prompt, NOT executed)
//   - both:                 "<command>\n<input>" (command runs, then input is
//     typed at the prompt the
//     command leaves behind, or
//     piped into a long-running
//     program if there is one)
//   - neither:              ""                   (no input)
//
// The asymmetric handling (newline after command, none after input) is
// deliberate: a command without a newline wouldn't execute, but raw
// initial_input is meant to leave the user at a typed-but-not-pressed
// state so they can review and hit Enter (or edit) themselves.
func mergeCommandAndInput(command, input string) string {
	switch {
	case command == "" && input == "":
		return ""
	case command == "":
		return input
	case input == "":
		return command + "\n"
	default:
		return command + "\n" + input
	}
}

func updateLaunchTime(a *app, name string, rt project.Runtime) error {
	rt.LastLaunchedAt = time.Now().UTC()
	return project.SaveRuntime(a.Paths, name, rt)
}

// resolveLayoutCwd resolves a layout-declared cwd against the project root.
// "" or "." map to the project root; absolute paths pass through unchanged;
// relative paths are joined and cleaned.
//
// Phase 1 policy: relative paths that escape the project root with `..` are
// allowed and pass through; we trust authored layouts. Revisit if user-supplied
// layouts get sandboxing requirements.
func resolveLayoutCwd(projectDir, layoutCwd string) string {
	if layoutCwd == "" || layoutCwd == "." {
		return projectDir
	}
	if filepath.IsAbs(layoutCwd) {
		return filepath.Clean(layoutCwd)
	}
	return filepath.Clean(filepath.Join(projectDir, layoutCwd))
}
