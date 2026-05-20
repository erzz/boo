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
	"golang.org/x/term"

	"github.com/erzz/boo/internal/ghostty"
	"github.com/erzz/boo/internal/layout"
	"github.com/erzz/boo/internal/project"
)

var terminalColsFn = terminalCols

// runRoot dispatches bare 'boo' / 'boo <name>'. With a name argument switches to
// that project; with no args always opens the TUI picker (no cwd-detect-and-switch,
// which would reopen the window the user is already typing into).
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

// switchToProject focuses the project's existing window if alive, otherwise opens a fresh one.
// Cold-starts Ghostty if needed. On cold-start, any saved WindowID is stale (process-lifetime).
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
	resolved, err := resolveLaunchLayout(l)
	if err != nil {
		return nil, err
	}
	return a.Ghostty.OpenLayout(ctx, layoutToParams(p.Dir, resolved))
}

// loadOrRegenerateLayout reads the per-project layout snapshot, regenerating it from the
// template when the file is missing (recoverable: the registry's Layout field is source of
// truth). Hard errors on present-but-unreadable files still propagate.
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
		// Surface both errors: snapshot missing AND template unresolvable.
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

func resolveLaunchLayout(l layout.Layout) (layout.Layout, error) {
	if !l.IsResponsive() {
		return l, nil
	}
	cols := terminalColsFn()
	return l.Resolve(cols)
}

func terminalCols() int {
	for _, fd := range []int{int(os.Stdout.Fd()), int(os.Stderr.Fd()), int(os.Stdin.Fd())} {
		if !term.IsTerminal(fd) {
			continue
		}
		cols, _, err := term.GetSize(fd)
		if err == nil && cols > 0 {
			return cols
		}
	}
	return 0
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

// splitToParams converts a layout.Split tree into the ghostty.LayoutSplit tree the JXA
// walker consumes. Resolves leaf cwd against the project root.
//
// Both `command` and `initial_input` are folded into cfg.initialInput — the one Ghostty
// knob that types into the surface after launching the default shell. cfg.command makes
// Ghostty hard-code /bin/bash --noprofile --norc, stripping PATH and $SHELL and exiting
// on completion — not what users expect from "run nvim in this pane". So command gets a
// trailing newline so it executes; initial_input is appended without one (user reviews
// and presses Enter themselves).
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
		Size:      s.Size,
	}
}

// mergeCommandAndInput folds command + initial_input into a single keystroke payload.
//   - command alone → "<command>\n" (executes, then prompt)
//   - initial_input alone → "<input>" (typed but not auto-executed; user presses Enter)
//   - both → "<command>\n<input>"
//   - neither → ""
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
// "" or "." → project root; absolute paths pass through; relative paths are joined.
func resolveLayoutCwd(projectDir, layoutCwd string) string {
	if layoutCwd == "" || layoutCwd == "." {
		return projectDir
	}
	if filepath.IsAbs(layoutCwd) {
		return filepath.Clean(layoutCwd)
	}
	return filepath.Clean(filepath.Join(projectDir, layoutCwd))
}
