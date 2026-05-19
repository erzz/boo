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

// runPicker is the shared entry point for asking the user to pick a project.
// TUI mode also offers a "+ New project" entry that feeds back through runCreateProject.
// Cancellation (Esc / Ctrl-C / no selection) is a no-op, never an error.
func runPicker(ctx context.Context, a *app, mode pickerMode, out io.Writer) error {
	reg, err := project.Load(a.Paths)
	if err != nil {
		return err
	}

	// fzf is selection-only; no "create new" form.
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

	// TUI path. Show the picker even with zero projects so the "+ New project" entry is visible.
	// Initial items are bare (no Ghostty calls); async enrichment fills Status/Trailing.
	items := buildBareItems(reg.Projects)

	// Pre-populate form defaults from cwd, so that hitting `n` from the
	// list lands the user on a form that's ready to submit if they like
	// the suggestions.
	defs, err := buildNewProjectDefaults(a, defaultsFromFlags{})
	if err != nil {
		return err
	}

	// Action callbacks run in-process so successful mutations return the user to a
	// refreshed list rather than dropping them to the shell. Each callback re-loads
	// the registry under the state lock.
	onDelete := func(name string, purge bool) ([]string, error) {
		var warns []string
		err := a.Paths.WithLock(func() error {
			freshReg, err := project.Load(a.Paths)
			if err != nil {
				return err
			}
			p, err := freshReg.Get(name)
			if err != nil {
				return err
			}
			// We're inside the alt-screen — executeDelete writes nothing; non-fatal
			// side-effect failures come back as []string warnings.
			var innerErr error
			warns, innerErr = executeDelete(ctx, a, freshReg, p, purge)
			return innerErr
		})
		return warns, err
	}
	onSetLayout := func(name, template string) error {
		return executeSetLayout(a, name, template)
	}
	// onKill closes the live Ghostty window for the project. It deliberately
	// does NOT take the state lock: CloseWindow only reads the runtime file
	// and shells out to JXA. Returning a non-nil error surfaces on the
	// picker's error screen; success refreshes the list so the "running"
	// pill flips back to "stopped".
	onKill := func(name string) error {
		rt, err := project.LoadRuntime(a.Paths, name)
		if err != nil {
			return fmt.Errorf("read runtime for %q: %w", name, err)
		}
		if rt.WindowID == "" {
			return fmt.Errorf("no recorded Ghostty window for %q", name)
		}
		if err := a.Ghostty.CloseWindow(ctx, rt.WindowID); err != nil {
			return fmt.Errorf("close window %s: %w", rt.WindowID, err)
		}
		return nil
	}
	onEdit := func(oldName, newName, newDir, newTemplate string) error {
		return executeEdit(a, oldName, newName, newDir, newTemplate)
	}
	onOpenLayout := func(name string) tea.Cmd {
		// Editor resolution mirrors `boo edit`: $EDITOR wins, $VISUAL is the fallback.
		// Verify the project and layout file exist before suspending the alt-screen.
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
		ed, err := buildEditorCmd("", path)
		if err != nil {
			return func() tea.Msg { return picker.NewEditorFinishedMsg(err) }
		}
		// tea.ExecProcess wires Stdin/Stdout/Stderr to the controlling terminal and restores
		// the alt-screen on exit.
		return tea.ExecProcess(ed, func(err error) tea.Msg {
			return picker.NewEditorFinishedMsg(err)
		})
	}
	refresh := func() ([]picker.Item, error) {
		freshReg, err := project.Load(a.Paths)
		if err != nil {
			// Non-fatal — picker keeps existing items; next action re-loads under lock.
			return nil, err
		}
		return buildPickerItems(ctx, a, freshReg.Projects), nil
	}

	// onLaunch runs the project launch as a background tea.Cmd so the picker stays alive.
	onLaunch := func(name string) tea.Cmd {
		return func() tea.Msg {
			freshReg, err := project.Load(a.Paths)
			if err != nil {
				return picker.NewLaunchFinishedMsg(name, err)
			}
			p, err := freshReg.Get(name)
			if err != nil {
				return picker.NewLaunchFinishedMsg(name, err)
			}
			return picker.NewLaunchFinishedMsg(name, switchToProject(ctx, a, p))
		}
	}

	// Surface a startup warning when the configured theme couldn't be loaded.
	themeName := a.Config.ThemeOr("default")
	_, themeOK := picker.ThemeByName(a.Paths.ThemesDir, themeName)
	var startupWarning string
	if !themeOK {
		startupWarning = fmt.Sprintf("theme %q not found, using default", themeName)
	}

	res, err := picker.Run(items, picker.Options{
		Defaults:              defs,
		PreviewTemplate:       templatePreviewer(a),
		PreviewProjectFactory: func(thm picker.Theme) func(string) string { return projectPreviewer(ctx, a, thm) },
		LayoutNames:           templateNames(a),
		ResolveLayout:         layoutResolver(a),
		Theme:                 themeName,
		ThemesDir:             a.Paths.ThemesDir,
		ConfigPath:            a.Paths.ConfigFile,
		OnDelete:              onDelete,
		OnSetLayout:           onSetLayout,
		OnKill:                onKill,
		OnEdit:                onEdit,
		OnOpenLayout:          onOpenLayout,
		OnLaunch:              onLaunch,
		StartupWarning:        startupWarning,
		RefreshItems:          refresh,
	})
	if err != nil {
		return err
	}
	if res.Cancelled() {
		return nil
	}
	// Only handoff intents (Switch, NewProject) reach this point.
	switch v := res.Intent.(type) {
	case picker.NewProjectIntent:
		return runCreateProject(ctx, a, v, out)
	case picker.SwitchIntent:
		// Re-load the registry: user may have deleted projects during the session.
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

// buildBareItems builds lightweight []picker.Item from registry data only (no Ghostty calls).
// Status/Trailing are left empty; filled later by async enrichment via RefreshItems.
// Used as the initial item set for the main TUI picker so picker.Run returns immediately.
func buildBareItems(projects []project.Project) []picker.Item {
	out := make([]picker.Item, 0, len(projects))
	for _, p := range projects {
		out = append(out, picker.Item{
			Key:         p.Name,
			Title:       p.Name,
			Description: p.Dir,
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
	// fzf needs a real TTY. Fail fast with a clear message rather than hanging.
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

// runFzf shells out to fzf directly (not via internal/exec Runner). fzf is interactive
// and TTY-bound; Runner's capture-buffer model can't support that. Documented exception
// to the project-wide "all shellouts via Runner" rule. See also editor.go, picker_helpers.go.
func runFzf(ctx context.Context, args []string, stdin string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "fzf", args...)
	cmd.Stdin = strings.NewReader(stdin)
	cmd.Stderr = os.Stderr
	return cmd.Output()
}

// templatePreviewer returns a picker.Options.PreviewTemplate callback that resolves the
// template via the same path boo new uses and renders it via layoutpreview.
// Returns "" on any error so the form silently hides the preview for unknown names.
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

// templateNames returns layout template names visible to boo new (built-ins + user overrides).
// Returns nil on any error so the form falls back to free-text input.
func templateNames(a *app) []string {
	names, err := layout.ListTemplates(a.Paths.LayoutsDir)
	if err != nil {
		return nil
	}
	return names
}

// layoutResolver returns a picker.Options.ResolveLayout callback that
// materialises a layout template into a fresh, owned *layout.Layout the
// in-picker editor can mutate. Each call re-parses the YAML so the editor
// can never leak edits across invocations. Returns (nil, nil) for a blank
// template (caller falls through to the default flow without opening the
// editor); for a non-empty but unresolvable template it returns the
// underlying error and the picker logs + falls through.
func layoutResolver(a *app) func(template string) (*layout.Layout, error) {
	return func(template string) (*layout.Layout, error) {
		template = strings.TrimSpace(template)
		if template == "" {
			return nil, nil
		}
		r, err := layout.ResolveTemplate(a.Paths.LayoutsDir, template)
		if err != nil {
			return nil, err
		}
		l := r.Layout
		if l.Name == "" {
			l.Name = template
		}
		return &l, nil
	}
}

// pickerTheme resolves the active picker theme from the app's config.
// On any error (unknown name, malformed file) it falls back to the default
// theme silently — same behaviour as picker.Run's internal resolution.
func pickerTheme(a *app) picker.Theme {
	thm, _ := picker.ThemeByName(a.Paths.ThemesDir, a.Config.ThemeOr("default"))
	return thm
}

// projectPreviewer returns a picker.Options.PreviewProject callback that renders a
// multi-line project summary: name, dir, layout, status, last-launched, layout preview.
//
// Re-loads the registry on every invocation so in-loop mutations (Edit, SetLayout) are
// reflected immediately in the right pane without staleness.
//
// rightPaneInnerWidth (36) = picker.rightPaneWidth (40) − border (2) − padding (2).
// Must be kept in sync with geometry.go if rightPaneWidth changes.
func projectPreviewer(ctx context.Context, a *app, thm picker.Theme) func(string) string {
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
		// Title row — use theme accent so it matches the list's selected-title.
		b.WriteString(boldAccent(thm, p.Name))
		b.WriteString("\n\n")
		// dir is shortened to fit: $HOME→~ first, then head-ellipsis. "dir  " prefix = 5 cols.
		writeRow(&b, thm, "dir", shortenPath(p.Dir, rightPaneInnerWidth-5))
		writeRow(&b, thm, "layout", p.Layout)
		writeRow(&b, thm, "status", renderStatusFor(thm, status))
		writeRow(&b, thm, "last", lastLaunched)

		// Prefer saved snapshot (layout.yaml) which reflects boo edit/save edits.
		// Fall back to template only when no saved file exists. Other errors render inline.
		var rendered string
		if saved, err := project.LoadLayout(a.Paths, p.Name); err == nil {
			rendered = layoutpreview.RenderLayout(saved, rightPaneInnerWidth)
		} else if errors.Is(err, os.ErrNotExist) {
			if r, err := layout.ResolveTemplate(a.Paths.LayoutsDir, p.Layout); err == nil {
				rendered = layoutpreview.RenderLayout(r.Layout, rightPaneInnerWidth)
			}
		} else {
			rendered = thm.StatusBroken.Render("✖ layout unreadable")
		}
		if rendered != "" {
			b.WriteString("\n")
			b.WriteString(faint(thm, "layout preview"))
			b.WriteString("\n")
			b.WriteString(rendered)
		}
		return b.String()
	}
}

// boldAccent / faint / writeRow / renderStatusFor are tiny theme-aware rendering helpers.
func boldAccent(thm picker.Theme, s string) string {
	return thm.RightPaneTitle.Render(s)
}

func faint(thm picker.Theme, s string) string {
	return thm.RightPaneFaint.Render(s)
}

func writeRow(b *strings.Builder, thm picker.Theme, k, v string) {
	b.WriteString(faint(thm, k))
	b.WriteString("  ")
	b.WriteString(v)
	b.WriteString("\n")
}

func renderStatusFor(thm picker.Theme, s string) string {
	switch s {
	case "running":
		return thm.StatusRunning.Render("● running")
	case "dir-missing":
		return thm.StatusBroken.Render("✖ dir missing")
	default:
		return thm.StatusStopped.Render("○ " + s)
	}
}

// shortenPath collapses $HOME→~ then head-ellipsises to fit maxWidth runes.
// Keeps the tail because the leaf dir is the most identifying part of a path.
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
	// Head-ellipsis: keep last (maxWidth-1) runes, prefix "…".
	r := []rune(p)
	return "…" + string(r[len(r)-(maxWidth-1):])
}
