package cli

import (
	"context"
	"errors"
	"fmt"
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
	l, err := project.LoadLayout(a.Paths, p.Name)
	if err != nil {
		return nil, err
	}
	return a.Ghostty.OpenLayout(ctx, layoutToParams(p.Dir, l))
}

// layoutToParams projects a layout.Layout into the JSON shape OpenLayout
// expects, resolving every split's cwd against the project root.
func layoutToParams(projectDir string, l layout.Layout) ghostty.OpenLayoutParams {
	tabs := make([]ghostty.LayoutTab, len(l.Tabs))
	for i, t := range l.Tabs {
		splits := make([]ghostty.LayoutSplit, len(t.Splits))
		for j, s := range t.Splits {
			splits[j] = ghostty.LayoutSplit{
				Direction:        s.Direction,
				WorkingDirectory: resolveLayoutCwd(projectDir, s.Cwd),
				Command:          s.Command,
				InitialInput:     s.InitialInput,
				Env:              s.Env,
			}
		}
		tabs[i] = ghostty.LayoutTab{Name: t.Name, Splits: splits}
	}
	return ghostty.OpenLayoutParams{Tabs: tabs}
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