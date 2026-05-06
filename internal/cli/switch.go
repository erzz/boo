package cli

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/sean-erswell-liljefelt/boo/internal/ghostty"
	"github.com/sean-erswell-liljefelt/boo/internal/project"
)

// runRoot dispatches the bare 'boo' / 'boo <name>' invocation.
//
//   - 'boo <name>' switches to that project (focus existing window or open new).
//   - 'boo' (no args) detects the current cwd and switches to the project
//     registered at that exact path, if any.
func runRoot(cmd *cobra.Command, args []string) error {
	a, err := newApp()
	if err != nil {
		return err
	}
	reg, err := project.Load(a.Paths)
	if err != nil {
		return err
	}

	var name string
	if len(args) == 1 {
		name = args[0]
		if err := project.ValidateName(name); err != nil {
			return err
		}
	} else {
		dir, err := resolveDir("")
		if err != nil {
			return err
		}
		p, err := reg.FindByDir(dir)
		if err != nil {
			return fmt.Errorf("not inside a registered project (%s).\nUse 'boo new <name> --dir .' to register, or 'boo list' to see known projects", dir)
		}
		name = p.Name
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
	// Phase 1 honours only the primary split of the first tab. Multi-tab and
	// split rendering land in Phase 2.
	primary := l.Tabs[0].Splits[0]
	return a.Ghostty.OpenWindow(ctx, ghostty.OpenWindowParams{
		WorkingDirectory: resolveLayoutCwd(p.Dir, primary.Cwd),
		Command:          primary.Command,
		Env:              primary.Env,
	})
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