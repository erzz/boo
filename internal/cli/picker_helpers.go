package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"

	"github.com/erzz/boo/internal/layout"
	"github.com/erzz/boo/internal/layoutpreview"
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
			_, _ = fmt.Fprintln(out, "No projects registered. Run 'boo new' to create one.")
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

	// Action callbacks live inside the TUI loop — running them
	// in-process means a successful delete/set-layout returns the user
	// to a refreshed list rather than dropping them back to the shell.
	// Each callback re-Loads the registry under the state lock to
	// avoid racing with other shells.
	onDelete := func(name string, purge bool) error {
		return a.Paths.WithLock(func() error {
			freshReg, err := project.Load(a.Paths)
			if err != nil {
				return err
			}
			p, err := freshReg.Get(name)
			if err != nil {
				return err
			}
			// We're inside the alt-screen, so executeDelete's success
			// line would just flash and disappear. Send its output to
			// io.Discard; the user sees the result via the refreshed
			// list (project gone) instead.
			return executeDelete(ctx, a, freshReg, p, purge, io.Discard, io.Discard)
		})
	}
	onSetLayout := func(name, template string) error {
		return executeSetLayout(a, name, template)
	}
	onEdit := func(oldName, newName, newDir, newTemplate string) error {
		return executeEdit(a, oldName, newName, newDir, newTemplate)
	}
	onOpenLayout := func(name string) tea.Cmd {
		// Editor resolution mirrors `boo edit`: $EDITOR wins, $VISUAL
		// is the fallback. If neither is set we surface the issue
		// inside the picker (via editorFinishedMsg → screenError)
		// rather than silently doing nothing.
		editor := os.Getenv("EDITOR")
		if editor == "" {
			editor = os.Getenv("VISUAL")
		}
		if editor == "" {
			return func() tea.Msg {
				return picker.NewEditorFinishedMsg(
					errors.New("set $EDITOR (or $VISUAL) to edit layout files from the picker"))
			}
		}
		// Confirm the project is still registered + the layout file
		// exists before suspending the alt-screen. A "no such file"
		// after the editor has stolen the terminal would be much
		// more disruptive than reporting it inline.
		freshReg, err := project.Load(a.Paths)
		if err != nil {
			return func() tea.Msg { return picker.NewEditorFinishedMsg(err) }
		}
		if _, err := freshReg.Get(name); err != nil {
			return func() tea.Msg { return picker.NewEditorFinishedMsg(err) }
		}
		path := a.Paths.ProjectLayoutFile(name)
		if _, err := os.Stat(path); err != nil {
			return func() tea.Msg {
				return picker.NewEditorFinishedMsg(
					fmt.Errorf("layout file for project %q not found at %s: %w", name, path, err))
			}
		}
		ed := exec.Command(editor, path)
		// tea.ExecProcess wires Stdin/Stdout/Stderr to the controlling
		// terminal automatically and restores the alt-screen on exit.
		return tea.ExecProcess(ed, func(err error) tea.Msg {
			return picker.NewEditorFinishedMsg(err)
		})
	}
	refresh := func() []picker.Item {
		freshReg, err := project.Load(a.Paths)
		if err != nil {
			// Refresh failures are non-fatal — leave the existing
			// items alone. The user can still navigate; next action
			// will re-Load the registry under the lock anyway.
			return nil
		}
		return buildPickerItems(ctx, a, freshReg.Projects)
	}

	res, err := picker.Run(items, picker.Options{
		Defaults:        defs,
		PreviewTemplate: templatePreviewer(a),
		PreviewProject:  projectPreviewer(ctx, a),
		LayoutNames:     templateNames(a),
		Theme:           a.Config.ThemeOr("default"),
		ThemesDir:       a.Paths.ThemesDir,
		ConfigPath:      a.Paths.ConfigFile,
		OnDelete:        onDelete,
		OnSetLayout:     onSetLayout,
		OnEdit:          onEdit,
		OnOpenLayout:    onOpenLayout,
		RefreshItems:    refresh,
	})
	if err != nil {
		return err
	}
	if res.Cancelled() {
		return nil
	}
	// Only handoff intents (Switch, NewProject) reach this point —
	// mutating actions are handled inside the picker loop. The default
	// arm catches programmer error if the picker ever returns an
	// unexpected type, e.g. after a refactor regression.
	switch v := res.Intent.(type) {
	case picker.NewProjectIntent:
		return runCreateProject(ctx, a, v, out)
	case picker.SwitchIntent:
		// Re-load the registry: the user may have deleted other
		// projects during the picker session, but the one they're
		// switching to should still be present.
		freshReg, err := project.Load(a.Paths)
		if err != nil {
			return err
		}
		p, err := freshReg.Get(v.Name)
		if err != nil {
			return err
		}
		return switchToProject(ctx, a, p)
	default:
		return fmt.Errorf("picker: unexpected handoff intent %T", v)
	}
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
			Layout:      p.Layout,
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
	// fzf needs a real TTY for its UI. If we're not attached to one
	// (script, pipe, CI), fzf will silently hang waiting for keystrokes
	// it can never receive. Fail fast with a clear message instead.
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		return "", errors.New("boo fzf needs a terminal for fzf's UI (stdout isn't a TTY)")
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

// templatePreviewer returns a callback suitable for picker.Options.PreviewTemplate.
// It resolves the template name through the same path `boo new` will use,
// then renders it via internal/layoutpreview. Empty result on any error so
// the form silently hides the preview while the user is mid-typing an
// unknown name — surfacing a stack trace inside a TUI form would be hostile.
//
// previewWidth (50) matches `boo layouts` so what the user sees in the form
// is byte-identical to what they'd see from the command line.
func templatePreviewer(a *app) func(string) string {
	const previewWidth = 50
	return func(name string) string {
		name = strings.TrimSpace(name)
		if name == "" {
			return ""
		}
		r, err := layout.ResolveTemplate(a.Paths.LayoutsDir, name)
		if err != nil {
			return ""
		}
		return layoutpreview.RenderLayout(r.Layout, previewWidth)
	}
}

// templateNames returns the list of layout template names visible to
// `boo new` (built-ins + any user overrides), suitable for
// picker.Options.LayoutNames so the TUI can render the Layout field
// as a left/right cycler instead of a free-text input.
//
// On any error reading the user templates dir we silently return nil,
// which falls back to the legacy free-text input. The form preview
// already silently hides on resolve errors, so the user can still
// type a known name and see it work. We never want a transient I/O
// error to make `boo new`'s TUI unusable.
func templateNames(a *app) []string {
	names, err := layout.ListTemplates(a.Paths.LayoutsDir)
	if err != nil {
		return nil
	}
	return names
}

// projectPreviewer returns a callback suitable for
// picker.Options.PreviewProject. It renders a multi-line summary of one
// project: name, dir, layout template, status, last-launched, and an
// ASCII layout preview.
//
// The closure captures ctx + a (paths/ghostty) but **re-loads the
// registry on every invocation**. This is intentional: in-loop actions
// like Edit and SetLayout mutate the registry on disk, and a stale
// captured *Registry would render outdated dir/layout in the right pane
// after a successful action. Registry load is a small TOML read — cheap
// enough to do per repaint. Mirrors templatePreviewer's "always read
// the source of truth" stance.
//
// rightPaneInnerWidth (36) matches picker.rightPaneWidth (40) minus the
// border (2) and padding (2). Hardcoded for now; if the right pane
// becomes dynamic-width we'll thread the width through this callback.
func projectPreviewer(ctx context.Context, a *app) func(string) string {
	const rightPaneInnerWidth = 36
	if ctx == nil {
		ctx = context.Background()
	}
	return func(name string) string {
		reg, err := project.Load(a.Paths)
		if err != nil {
			return ""
		}
		p, err := reg.Get(name)
		if err != nil {
			return ""
		}
		rt, _ := project.LoadRuntime(a.Paths, name)

		status := "stopped"
		switch {
		case !dirExists(p.Dir):
			status = "dir-missing"
		case rt.WindowID != "":
			if exists, err := a.Ghostty.WindowExists(ctx, rt.WindowID); err == nil && exists {
				status = "running"
			}
		}

		lastLaunched := "never"
		if !rt.LastLaunchedAt.IsZero() {
			lastLaunched = humanAge(rt.LastLaunchedAt)
		}

		var b strings.Builder
		// Title row
		b.WriteString(boldAccent(p.Name))
		b.WriteString("\n\n")
		// Two-column key/value rows. Keys are faint; values plain.
		// dir is shortened to fit the pane: $HOME → ~ first, then a
		// head-ellipsis if it's still too long. The "dir  " prefix
		// (faint key + 2 spaces) eats 5 cols, leaving rightPaneInnerWidth-5
		// for the value itself.
		writeRow(&b, "dir", shortenPath(p.Dir, rightPaneInnerWidth-5))
		writeRow(&b, "layout", p.Layout)
		writeRow(&b, "status", renderStatusFor(status))
		writeRow(&b, "last", lastLaunched)

		// Layout preview underneath.
		if r, err := layout.ResolveTemplate(a.Paths.LayoutsDir, p.Layout); err == nil {
			rendered := layoutpreview.RenderLayout(r.Layout, rightPaneInnerWidth)
			if rendered != "" {
				b.WriteString("\n")
				b.WriteString(faint("layout preview"))
				b.WriteString("\n")
				b.WriteString(rendered)
			}
		}
		return b.String()
	}
}

// boldAccent / faint / writeRow / renderStatusFor are tiny rendering
// helpers shared by projectPreviewer. Kept in this file rather than
// pushed into picker so the picker package stays project-agnostic; the
// styling is intentionally simple (lipgloss styles applied here, not
// looked up from picker.Theme) to avoid coupling CLI render code to the
// theme primitive's internals.
func boldAccent(s string) string {
	return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("13")).Render(s)
}

func faint(s string) string {
	return lipgloss.NewStyle().Faint(true).Render(s)
}

func writeRow(b *strings.Builder, k, v string) {
	b.WriteString(faint(k))
	b.WriteString("  ")
	b.WriteString(v)
	b.WriteString("\n")
}

func renderStatusFor(s string) string {
	switch s {
	case "running":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true).Render("● running")
	case "dir-missing":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true).Render("✖ dir missing")
	default:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("○ " + s)
	}
}

// shortenPath collapses $HOME → ~ and head-ellipsises the result so it
// fits in maxWidth runes. Returns p unchanged if maxWidth <= 0 or if
// the (possibly home-collapsed) path already fits. Falls back to a
// "…<tail>" form when truncation is needed; we keep the *tail* because
// the leaf directory is the most identifying part of a project path.
func shortenPath(p string, maxWidth int) string {
	if maxWidth <= 0 {
		return p
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		if p == home {
			p = "~"
		} else if strings.HasPrefix(p, home+string(os.PathSeparator)) {
			p = "~" + p[len(home):]
		}
	}
	if len([]rune(p)) <= maxWidth {
		return p
	}
	// Head-ellipsis: keep the last (maxWidth-1) runes, prefix "…".
	r := []rune(p)
	return "…" + string(r[len(r)-(maxWidth-1):])
}
