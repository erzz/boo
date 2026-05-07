package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/erzz/boo/internal/picker"
	"github.com/erzz/boo/internal/project"
)

// pickerMode describes which selection UI to use when boo needs to ask the
// user "which project?".
type pickerMode int

const (
	pickerTUI pickerMode = iota
	pickerFzf
)

// runPicker is the shared entry point for any code path that wants to ask
// the user to pick a project. Used by:
//
//   - bare `boo` (with no project name)
//   - `boo fzf`
//
// In TUI mode the picker also offers a "+ New project" entry that returns
// a NewProject intent; we then feed that back through runCreateProject.
//
// Cancellation (Esc / Ctrl-C / no selection) is treated as a no-op, never
// an error.
func runPicker(ctx context.Context, a *app, mode pickerMode, out io.Writer) error {
	reg, err := project.Load(a.Paths)
	if err != nil {
		return err
	}

	// fzf is selection-only and doesn't host a "create new" form. If the
	// registry is empty, tell the user and bail; otherwise hand off.
	if mode == pickerFzf {
		if len(reg.Projects) == 0 {
			fmt.Fprintln(out, "No projects registered. Run 'boo new' to create one.")
			return nil
		}
		items := buildPickerItems(ctx, a, reg.Projects)
		selected, err := pickViaFzf(ctx, items)
		if err != nil {
			return err
		}
		if selected == "" {
			return nil
		}
		p, err := reg.Get(selected)
		if err != nil {
			return err
		}
		return switchToProject(ctx, a, p)
	}

	// TUI path. Always show the picker — even with zero projects, the
	// "+ New project" entry is visible and that's the user's next move.
	items := buildPickerItems(ctx, a, reg.Projects)

	// Pre-populate form defaults from cwd, so that hitting `n` from the
	// list lands the user on a form that's ready to submit if they like
	// the suggestions.
	defs, err := buildNewProjectDefaults(a, defaultsFromFlags{})
	if err != nil {
		return err
	}

	res, err := picker.Run(items, picker.Options{
		Defaults: defs,
	})
	if err != nil {
		return err
	}
	if res.Cancelled() {
		return nil
	}
	if res.NewProject != nil {
		return runCreateProject(ctx, a, *res.NewProject, out)
	}
	p, err := reg.Get(res.Selected)
	if err != nil {
		return err
	}
	return switchToProject(ctx, a, p)
}

// buildPickerItems mirrors the status/last-launched columns from `boo list`
// so users see consistent information across the TUI and fzf paths.
func buildPickerItems(ctx context.Context, a *app, projects []project.Project) []picker.Item {
	if ctx == nil {
		ctx = context.Background()
	}
	out := make([]picker.Item, 0, len(projects))
	for _, p := range projects {
		rt, _ := project.LoadRuntime(a.Paths, p.Name)
		status := "stopped"
		switch {
		case !dirExists(p.Dir):
			status = "dir-missing"
		case rt.WindowID != "":
			if exists, err := a.Ghostty.WindowExists(ctx, rt.WindowID); err == nil && exists {
				status = "running"
			}
		}
		trailing := ""
		if !rt.LastLaunchedAt.IsZero() {
			trailing = humanAge(rt.LastLaunchedAt)
		}
		out = append(out, picker.Item{
			Key:         p.Name,
			Title:       p.Name,
			Description: p.Dir,
			Status:      status,
			Trailing:    trailing,
		})
	}
	return out
}

// pickViaFzf shells out to fzf for selection. The line format is:
//
//	<name>\t<status> · <dir> · <trailing>
//
// fzf returns the full line; we recover the project name by taking everything
// before the first tab. Returns "" if the user cancelled.
func pickViaFzf(ctx context.Context, items []picker.Item) (string, error) {
	if _, err := exec.LookPath("fzf"); err != nil {
		return "", errors.New("fzf is not on $PATH (install fzf, or run 'boo' for the built-in picker)")
	}

	var input strings.Builder
	for _, it := range items {
		display := it.Title
		extras := []string{}
		if it.Status != "" {
			extras = append(extras, it.Status)
		}
		if it.Description != "" {
			extras = append(extras, it.Description)
		}
		if it.Trailing != "" {
			extras = append(extras, it.Trailing)
		}
		if len(extras) > 0 {
			display = display + "\t" + strings.Join(extras, " · ")
		}
		input.WriteString(display)
		input.WriteByte('\n')
	}

	args := []string{
		"--ansi",
		"--prompt=boo > ",
		"--header=enter to switch · esc to cancel",
		"--delimiter=\t",
		"--with-nth=1..",
		"--no-multi",
	}
	stdout, err := runFzf(ctx, args, input.String())
	if err != nil {
		// Distinguish cancel (exit 130) and no-match (exit 1) from real errors.
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			switch ee.ExitCode() {
			case 1, 130:
				return "", nil
			}
		}
		return "", fmt.Errorf("fzf: %w", err)
	}
	line := strings.TrimRight(string(stdout), "\n")
	if line == "" {
		return "", nil
	}
	if idx := strings.IndexByte(line, '\t'); idx > 0 {
		return line[:idx], nil
	}
	return line, nil
}

// runFzf shells out to fzf directly rather than going through internal/exec's
// Runner. fzf is interactive and TTY-bound; the Runner abstraction is built
// around captured-buffer stdout/stderr, which doesn't model an interactive
// TUI. Documented exception to the project-wide "all shellouts via Runner"
// rule.
func runFzf(ctx context.Context, args []string, stdin string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "fzf", args...)
	cmd.Stdin = strings.NewReader(stdin)
	cmd.Stderr = os.Stderr
	return cmd.Output()
}
