// Package picker is the Bubble Tea TUI used by `boo`.
//
// It owns nothing project-specific: callers pass in a slice of items and
// receive back a Result describing what the user wants to do next (switch
// to an existing project, create a new one, or cancel). The CLI layer
// performs the actual side effects. This keeps the picker free of
// Ghostty/registry/process dependencies and trivially testable.
package picker

import (
	"fmt"
	"io"
	"log/slog"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/erzz/boo/internal/config"
	"github.com/erzz/boo/internal/layout"
	"github.com/erzz/boo/internal/theme"
)

// Item is one row in the picker.
type Item struct {
	Key         string
	Title       string
	Description string
	Status      string // "running" | "stopped" | "dir-missing" | ""
	Trailing    string
	// Layout is the template name currently assigned to the project.
	// Used by the set-layout sub-screen as the cycler's starting anchor
	// so ←/→ moves relative to "what it is now". Empty = start at index
	// 0 (still functional, just less ergonomic).
	Layout string
}

// FilterValue satisfies bubbles/list.Item.
func (i Item) FilterValue() string { return i.Title + " " + i.Description }

// newProjectItem is the synthetic "+ New project" row injected at the bottom.
type newProjectItem struct{}

func (newProjectItem) FilterValue() string { return "+ New project" }

// Intent is a sealed sum type returned by Run. CLI dispatches via type switch.
// The unexported isIntent() method prevents external packages from adding variants.
type Intent interface{ isIntent() }

// SwitchIntent — user picked an existing project from the list.
type SwitchIntent struct{ Name string }

// DeleteIntent — user confirmed delete (d) or purge (D) on a project row.
// Confirmation happened in TUI; CLI must NOT prompt again.
type DeleteIntent struct {
	Name  string
	Purge bool
}

// KillIntent — user pressed K on a running project. Closes the Ghostty
// window but leaves the project registered. Unlike DeleteIntent{Purge:true},
// no confirm modal is shown: closing a window is reversible (just relaunch),
// whereas deletion is not.
type KillIntent struct{ Name string }

// SetLayoutIntent — user picked a new template on the set-layout
// sub-screen. The CLI re-resolves the template and writes both the
// per-project layout snapshot and the registry's display field, same
// as `boo set-layout <name> <template>`.
type SetLayoutIntent struct {
	Name     string
	Template string
}

// EditIntent — user submitted the edit form. OldName is the pre-edit key.
// CLI decides which field changes are meaningful; no-op edit is fine.
type EditIntent struct {
	OldName     string
	NewName     string
	NewDir      string
	NewTemplate string
}

// NewProjectIntent — user submitted the new-project form. Non-empty From
// means clone into Dir; otherwise Dir is an existing dir to register.
//
// MaterialisedLayout, when non-nil, overrides Template at registration
// time: the CLI writes this layout to the project's own layout.yaml
// instead of resolving Template from the templates dir. It's the
// post-edit output of the in-picker layout editor (`screenLayoutEditor`).
// The registry's `Layout` field still records the original Template
// name so layout regeneration (`loadOrRegenerateLayout`) keeps working.
type NewProjectIntent struct {
	Name               string
	Dir                string
	From               string
	Template           string
	MaterialisedLayout *layout.Layout
}

func (SwitchIntent) isIntent()     {}
func (DeleteIntent) isIntent()     {}
func (KillIntent) isIntent()       {}
func (SetLayoutIntent) isIntent()  {}
func (EditIntent) isIntent()       {}
func (NewProjectIntent) isIntent() {}

// Result describes what the user did. A nil Intent means the user cancelled.
type Result struct {
	Intent Intent
}

// Cancelled reports whether the user dismissed the UI without choosing.
func (r Result) Cancelled() bool { return r.Intent == nil }

// Options configures Run.
type Options struct {
	Title    string
	Defaults FormDefaults
	// SkipListGoStraightToForm opens the form immediately, skipping the
	// project list. Used by `boo new` (no positional args).
	SkipListGoStraightToForm bool
	// HideNewProject suppresses the synthetic "+ New project" row and the
	// 'n'/'+' keybind that opens the form. Used by selection-only callers
	// like `boo delete` where creating a new project from the picker would
	// make no sense.
	HideNewProject bool
	// PreviewTemplate, if set, is called on every Layout field change; returned string
	// is rendered below the form. Callback keeps the picker free of layout-package deps.
	PreviewTemplate func(name string) string
	// LayoutNames, if non-empty, turns the Layout template field into a ←/→ cycler.
	// Empty = free-text input (useful for tests or callers that don't enumerate templates).
	LayoutNames []string
	// Theme selects a named visual theme. Empty or unknown falls back to the built-in default.
	Theme string
	// ThemesDir holds user-authored theme YAML files. Empty = built-ins only.
	ThemesDir string
	// ConfigPath is the path to config.yaml. When set, T persists the new theme to disk.
	// Empty suppresses the T keybind from help and makes theme cycling session-only.
	ConfigPath string
	// PreviewProject is called on cursor movement; returns the right-pane content string.
	// Prefer PreviewProjectFactory when live theme cycling must update the preview style.
	PreviewProject func(name string) string

	// PreviewProjectFactory supersedes PreviewProject. Called once at startup and on each
	// theme cycle to produce a PreviewProject closure styled with the new theme.
	PreviewProjectFactory func(thm Theme) func(name string) string

	// Action callbacks. nil = key disabled and hidden from help.
	//
	// OnDelete returns (warnings, error). Non-nil warnings = success with non-fatal
	// side-effect failures; non-nil error = deletion itself failed (shows error screen).
	// Confirmation/UX lives in the picker; CLI only performs the side effect.
	OnDelete    func(name string, purge bool) (warnings []string, err error)
	OnSetLayout func(name, template string) error
	// OnKill closes the Ghostty window for the named project without
	// touching the registry. Called when the user presses K on a project
	// the picker believes is running (Status == "running"). nil = key
	// disabled and hidden from help.
	OnKill func(name string) error
	// OnEdit applies an edit. Receives original key (oldName) plus desired post-edit values.
	// nil = 'e' key disabled.
	OnEdit func(oldName, newName, newDir, newTemplate string) error

	// OnOpenLayout returns a tea.Cmd that opens the layout file in $EDITOR via
	// tea.ExecProcess. Returning nil disables the 'o' key. Callback keeps the picker
	// free of $EDITOR / exec.Command knowledge.
	OnOpenLayout func(name string) tea.Cmd

	// RefreshItems is called after a successful action. Returns the new item list or an
	// error; on error the picker logs and leaves the existing list in place. nil = no refresh.
	RefreshItems func() ([]Item, error)

	// OnLaunch, if set, runs the project launch as an async tea.Cmd while the picker stays
	// alive. Must emit LaunchFinishedMsg on completion. nil = legacy quit-and-handoff.
	OnLaunch func(name string) tea.Cmd

	// ResolveLayout, if set, is called after the new-project form is submitted (with a
	// non-empty Template) to materialise the chosen template into a *layout.Layout that
	// the in-picker editor can mutate. nil = skip the editor entirely (form submit
	// proceeds straight to NewProjectIntent dispatch, current behaviour). The callback
	// must return a fresh, fully-owned tree on each call — the editor mutates it in
	// place, so a cached/shared instance would leak edits across invocations.
	ResolveLayout func(template string) (*layout.Layout, error)

	// StartupWarning, if non-empty, is shown in the status bar on open.
	StartupWarning string
}

// Run shows the TUI and blocks until the user makes a decision.
//
// Renders to stderr so command output redirected via stdout doesn't get
// mangled and the alt-screen restore at exit is clean.
func Run(items []Item, opts Options) (Result, error) {
	theme, _ := ThemeByName(opts.ThemesDir, opts.Theme)

	listItems := make([]list.Item, 0, len(items)+1)
	for _, it := range items {
		listItems = append(listItems, it)
	}
	if !opts.HideNewProject {
		listItems = append(listItems, newProjectItem{})
	}

	l := list.New(listItems, newDelegate(theme), 0, 0)
	title := opts.Title
	if title == "" {
		title = "boo — projects"
	}
	l.Title = title
	l.SetShowStatusBar(false)
	l.SetShowHelp(true)
	l.SetFilteringEnabled(true)
	l.Styles.Title = theme.ListTitle

	keys := defaultKeyMap()
	// The list's `?` help footer is built from whatever bindings the
	// caller actually wired up. Each binding's presence here mirrors a
	// reachable code path in updateList; if a callback is nil we drop
	// the binding from help entirely rather than advertise a key that
	// will silently no-op.
	helpExtras := func() []key.Binding {
		out := []key.Binding{}
		if !opts.HideNewProject {
			out = append(out, keys.New)
		}
		if opts.OnEdit != nil {
			out = append(out, keys.Edit)
		}
		if opts.OnOpenLayout != nil {
			out = append(out, keys.OpenLayout)
		}
		if opts.OnSetLayout != nil {
			out = append(out, keys.SetLayout)
		}
		if opts.OnDelete != nil {
			out = append(out, keys.Delete, keys.Purge)
		}
		if opts.OnKill != nil {
			out = append(out, keys.Kill)
		}
		if opts.ConfigPath != "" {
			out = append(out, keys.CycleTheme)
		}
		return out
	}
	l.AdditionalShortHelpKeys = helpExtras
	l.AdditionalFullHelpKeys = helpExtras

	form := newFormModel(opts.Defaults, theme)
	form.preview = opts.PreviewTemplate
	form.setLayoutNames(opts.LayoutNames)

	formOnly := opts.SkipListGoStraightToForm
	scr := initialScreen(formOnly, opts.Defaults.AlreadyRegisteredAs)

	// Resolve initial preview function. Factory wins over the bare
	// function so the preview is styled with the correct theme from
	// the first render. Both may be nil (selection-only callers).
	previewFn := opts.PreviewProject
	if opts.PreviewProjectFactory != nil {
		previewFn = opts.PreviewProjectFactory(theme)
	}

	m := &model{
		list:                  l,
		form:                  form,
		theme:                 theme,
		keys:                  keys,
		screen:                scr,
		themeName:             opts.Theme,
		themesDir:             opts.ThemesDir,
		configPath:            opts.ConfigPath,
		formOnly:              formOnly,
		hideNewProject:        opts.HideNewProject,
		alreadyRegisteredAs:   opts.Defaults.AlreadyRegisteredAs,
		previewProject:        previewFn,
		previewProjectFactory: opts.PreviewProjectFactory,
		previewTemplate:       opts.PreviewTemplate,
		layoutNames:           opts.LayoutNames,
		onDelete:              opts.OnDelete,
		onSetLayout:           opts.OnSetLayout,
		onKill:                opts.OnKill,
		onEdit:                opts.OnEdit,
		onOpenLayout:          opts.OnOpenLayout,
		onLaunch:              opts.OnLaunch,
		refreshItems:          opts.RefreshItems,
		resolveLayout:         opts.ResolveLayout,
	}
	if opts.StartupWarning != "" {
		m.status = statusLine{text: opts.StartupWarning, isErr: true}
	}
	prog := tea.NewProgram(m, tea.WithAltScreen(), tea.WithOutput(stderr()))
	final, err := prog.Run()
	if err != nil {
		return Result{}, fmt.Errorf("picker: %w", err)
	}
	mm := final.(*model)
	if mm.cancelled {
		return Result{}, nil
	}
	return Result{Intent: mm.intent}, nil
}

// screen identifies the active sub-view.
type screen int

const (
	screenList screen = iota
	screenForm
	screenAlreadyRegistered
	screenConfirm
	screenSetLayout
	screenLayoutEditor
	screenError
)

// initialScreen decides which sub-view to show when the picker starts.
//
// AlreadyRegistered is only meaningful when we were going to land on
// the form. The interstitial answers the question "you asked to create
// a new project here, but this dir already has one — switch or
// continue?". For list-first flows (bare `boo`) the list is exactly
// what the user wants; short-circuiting to the interstitial would be
// hostile (forcing them to press 'esc' just to see their own project
// list).
//
// Pulled out of Run() so the precedence rule is unit-testable without
// spinning up a Bubble Tea program.
func initialScreen(formOnly bool, alreadyRegisteredAs string) screen {
	switch {
	case formOnly && alreadyRegisteredAs != "":
		return screenAlreadyRegistered
	case formOnly:
		return screenForm
	default:
		return screenList
	}
}

type model struct {
	list   list.Model
	form   formModel
	theme  Theme
	keys   keyMap
	screen screen

	// Theme cycler state. themeName tracks the active theme so `T`
	// knows where in the cycle to advance from. themesDir lets
	// cycleTheme reload the full theme list on each keypress.
	// configPath, when non-empty, causes cycleTheme to persist the
	// selected theme back to disk after every cycle.
	themeName  string
	themesDir  string
	configPath string

	formOnly            bool   // when true, esc on form cancels the whole TUI rather than going back to the list
	hideNewProject      bool   // when true, the form/intent path is unreachable; selection-only mode
	alreadyRegisteredAs string // shown on the AlreadyRegistered screen
	previewProject      func(name string) string
	// previewProjectFactory, when non-nil, is used by applyTheme to
	// re-create previewProject with the new theme on each theme cycle.
	// This ensures the right pane stays styled with the live theme.
	previewProjectFactory func(thm Theme) func(name string) string
	previewTemplate       func(name string) string // shared with form; reused by setLayout sub-screen
	layoutNames           []string                 // shared with form; reused by setLayout sub-screen

	// Async preview state. previewCache stores the most recently
	// computed preview string per project name. previewGen is
	// incremented on every startPreview call so out-of-order
	// previewReadyMsgs (from rapid cursor movement) are discarded.
	previewCache map[string]string
	previewGen   uint64

	// enrichGen is incremented on every startEnrich call. enrichedItemsMsgs
	// whose gen is older than enrichGen are discarded so a slow startup
	// enrichment can never overwrite a fresher post-action refresh.
	enrichGen uint64

	// Action callbacks (see Options docs). nil = action key is disabled.
	//
	// Successful callbacks return nil error; the picker then calls
	// RefreshItems and re-renders the list. Failed callbacks return an
	// error which the picker shows on a transient error screen until
	// the user dismisses it with any key.
	onDelete      func(name string, purge bool) (warnings []string, err error)
	onSetLayout   func(name, template string) error
	onKill        func(name string) error
	onEdit        func(oldName, newName, newDir, newTemplate string) error
	onOpenLayout  func(name string) tea.Cmd
	onLaunch      func(name string) tea.Cmd
	refreshItems  func() ([]Item, error)
	resolveLayout func(template string) (*layout.Layout, error)

	intent    Intent // nil + cancelled=false should not happen; nil + cancelled=true means dismissed
	cancelled bool

	// confirm is the active modal, valid iff screen == screenConfirm.
	confirm confirmModel

	// setLayout is the set-layout sub-screen state, valid iff
	// screen == screenSetLayout.
	setLayout setLayoutModel

	// layoutEditor is the new-project layout editor sub-screen state,
	// valid iff screen == screenLayoutEditor. Reached after form submit
	// when ResolveLayout is wired and the form's Template is non-empty.
	layoutEditor layoutEditorModel

	// pendingNewProject holds the form-submitted intent while the user
	// is in the layout editor sub-screen. On editor confirm we attach
	// the materialised layout (or not) and dispatch via runIntent.
	// Non-nil iff screen == screenLayoutEditor.
	pendingNewProject *NewProjectIntent

	// errMsg is the message rendered on screenError. Cleared on
	// dismissal.
	errMsg string

	// status holds the most recent action's outcome for the bottom
	// status bar. Empty status.text => render the idle hint instead.
	// Set by setStatus(...) after every in-loop action; never cleared
	// (the previous outcome is more informative than nothing).
	status statusLine

	width, height int
}

// statusLine is the state behind the bottom status bar. Kept as a
// struct so the renderer can pick the right theme style based on the
// outcome without re-parsing the message.
type statusLine struct {
	text  string // human-readable summary, e.g. "deleted alpha"
	isErr bool   // true => render in StatusBarError style
}

// splitThreshold is the minimum terminal width for the split-pane layout.
// Below 90 cols the list pane gets cramped; 90 keeps the squeeze range in
// single-pane mode where overflow is harmless. (Was 80, then 70, raised to 90.)
const splitThreshold = 90

// splitMinHeight is the minimum terminal height for split mode. Below 24 rows the
// right-pane content clips ugly; drop to single-pane where vertical space helps more.
const splitMinHeight = 24

// rightPaneWidth is the column budget for the right pane in split mode.
// 40 balances "enough for a layout preview" (~25 cols) vs "not squeezing the list".
const rightPaneWidth = 40

// Layout overheads used by WindowSize math.
const (
	statusBarHeight     = 1  // vertical lines reserved for the bottom status bar
	listBorderOverhead  = 2  // top + bottom border of the list pane
	listPaneInnerInset  = 4  // list pane: left/right borders (2) + Padding(0,1) (2)
	listMinInnerWidth   = 20 // floor for list inner content width
	brandStripReserved  = 4  // rows for the brand strip (3) + gap (1) above the list
	brandStripMinHeight = 12 // inner list-pane height below which we suppress the strip
)

// editorFinishedMsg is dispatched by OnOpenLayout after tea.ExecProcess returns.
// Public alias so CLI-side code can dispatch the right message without the unexported name.
type EditorFinishedMsg = editorFinishedMsg

type editorFinishedMsg struct {
	err error
}

// NewEditorFinishedMsg constructs an editorFinishedMsg for CLI-side tea.ExecProcess wrappers.
func NewEditorFinishedMsg(err error) tea.Msg {
	return editorFinishedMsg{err: err}
}

// launchFinishedMsg is dispatched when an OnLaunch cmd completes.
type launchFinishedMsg struct {
	name string
	err  error
}

// LaunchFinishedMsg is the exported alias for CLI-side OnLaunch callbacks.
type LaunchFinishedMsg = launchFinishedMsg

// NewLaunchFinishedMsg constructs a launchFinishedMsg. Pass nil err on success.
func NewLaunchFinishedMsg(name string, err error) tea.Msg {
	return launchFinishedMsg{name: name, err: err}
}

// brandStripActive reports whether the list pane is tall enough for the decorative strip.
func (m *model) brandStripActive(innerListHeight int) bool {
	return innerListHeight >= brandStripMinHeight
}

// usableInnerHeight returns the per-pane content height (rows inside bordered box,
// excluding borders and status bar). The -1 safety margin guards against terminals
// that report height inclusive of chrome they reserve outside the alt-screen.
func (m *model) usableInnerHeight() int {
	h := m.height - statusBarHeight - listBorderOverhead - 1
	if h < 1 {
		h = 1
	}
	return h
}

// splitActive reports whether the current width is wide enough to render
// the split-pane layout. False = render just the left pane (list).
func (m *model) splitActive() bool {
	return m.width >= splitThreshold && m.height >= splitMinHeight
}

func (m *model) Init() tea.Cmd { return tea.Batch(m.startEnrich(), m.startPreview()) }

// previewReadyMsg carries the result of an async preview computation.
// It is emitted by the tea.Cmd returned from startPreview and applied
// in Update. gen matches the m.previewGen value at dispatch time so
// stale results from rapid cursor movement are silently discarded.
type previewReadyMsg struct {
	name    string
	preview string
	gen     uint64
}

// enrichedItemsMsg carries the result of an async item-enrichment run.
// It is sent both by Init (initial startup enrichment) and by startEnrich
// (post-action refresh). The model applies it in Update, which runs on the
// Bubble Tea dispatch loop — no locking required.
// gen matches the m.enrichGen value at dispatch time so out-of-order
// results (e.g. a slow startup enrichment arriving after a post-delete
// refresh) are silently discarded.
type enrichedItemsMsg struct {
	items []Item
	err   error
	gen   uint64
}

// startEnrich returns a tea.Cmd that calls RefreshItems asynchronously and sends
// an enrichedItemsMsg. Increments enrichGen so stale in-flight results are discarded.
// Returns nil when RefreshItems is nil (safe no-op for tea.Batch).
func (m *model) startEnrich() tea.Cmd {
	if m.refreshItems == nil {
		return nil
	}
	m.enrichGen++
	gen := m.enrichGen
	fn := m.refreshItems
	return func() tea.Msg {
		items, err := fn()
		return enrichedItemsMsg{items: items, err: err, gen: gen}
	}
}

// startPreview returns a tea.Cmd that invokes PreviewProject asynchronously for the
// selected item and sends a previewReadyMsg. Increments previewGen so stale results
// from rapid cursor movement are discarded. Returns nil when no callback is configured.
func (m *model) startPreview() tea.Cmd {
	if m.previewProject == nil {
		return nil
	}
	it, ok := m.list.SelectedItem().(Item)
	if !ok {
		return nil
	}
	m.previewGen++
	gen := m.previewGen
	name := it.Key
	fn := m.previewProject
	return func() tea.Msg {
		preview := fn(name)
		return previewReadyMsg{name: name, preview: preview, gen: gen}
	}
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if sz, ok := msg.(tea.WindowSizeMsg); ok {
		m.width, m.height = sz.Width, sz.Height
		// Reserve 1 line for the status bar, 2 for list-pane border, and 1 safety margin
		// for terminals that report height inclusive of chrome outside the alt-screen.
		listHeight := m.usableInnerHeight()
		// In split mode the right pane + gutter take rightPaneWidth+1 cols.
		listWidth := sz.Width
		if m.splitActive() {
			listWidth = sz.Width - rightPaneWidth - 1
		}
		listWidth -= listPaneInnerInset
		if listWidth < listMinInnerWidth {
			listWidth = listMinInnerWidth
		}
		// If the brand strip will be rendered above the list, the
		// bubbles list itself gets correspondingly fewer rows.
		if m.brandStripActive(listHeight) {
			listHeight -= brandStripReserved
		}
		m.list.SetSize(listWidth, listHeight)
		m.form.setSize(sz.Width)
		return m, nil
	}

	// editorFinishedMsg: alt-screen already restored by the runtime; refresh data
	// and surface any spawn error. Handled here (not per-screen) for robustness.
	if fin, ok := msg.(editorFinishedMsg); ok {
		if fin.err != nil {
			return m.showError(fmt.Sprintf("editor: %v", fin.err)), nil
		}
		m.setStatusOK("layout file saved")
		return m, m.startEnrich()
	}

	// previewReadyMsg: cache result; discard if gen is stale (overtaken by newer cursor move).
	if msg, ok := msg.(previewReadyMsg); ok {
		if msg.gen >= m.previewGen {
			if m.previewCache == nil {
				m.previewCache = make(map[string]string)
			}
			m.previewCache[msg.name] = msg.preview
		}
		return m, nil
	}

	// enrichedItemsMsg: discard stale results (gen < enrichGen); apply new items on success.
	if msg, ok := msg.(enrichedItemsMsg); ok {
		if msg.gen < m.enrichGen {
			// Stale: a newer enrichment supersedes this one.
			return m, nil
		}
		if msg.err != nil {
			slog.Warn("picker: refresh failed, keeping existing items", "err", msg.err)
			return m, nil
		}
		listItems := make([]list.Item, 0, len(msg.items)+1)
		for _, it := range msg.items {
			listItems = append(listItems, it)
		}
		if !m.hideNewProject {
			listItems = append(listItems, newProjectItem{})
		}
		m.list.SetItems(listItems)
		// Invalidate preview cache: enrichment updates Status/Trailing that projectPreviewer reflects.
		m.previewCache = nil
		return m, m.startPreview()
	}

	// launchFinishedMsg: update status bar and re-enrich so the status pill reflects running state.
	if lm, ok := msg.(launchFinishedMsg); ok {
		if lm.err != nil {
			m.setStatusErr(fmt.Sprintf("launch failed: %v", lm.err))
		} else {
			m.setStatusOK(fmt.Sprintf("launched %s", lm.name))
		}
		return m, m.startEnrich()
	}

	switch m.screen {
	case screenAlreadyRegistered:
		return m.updateAlreadyRegistered(msg)
	case screenForm:
		return m.updateForm(msg)
	case screenConfirm:
		return m.updateConfirm(msg)
	case screenSetLayout:
		return m.updateSetLayout(msg)
	case screenLayoutEditor:
		return m.updateLayoutEditor(msg)
	case screenError:
		return m.updateError(msg)
	default:
		return m.updateList(msg)
	}
}

func (m *model) updateList(msg tea.Msg) (tea.Model, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok {
		// While the filter input is focused, pass everything through.
		if m.list.FilterState() != list.Filtering {
			pressed := km.String()
			switch {
			case matches(m.keys.Quit, pressed):
				m.cancelled = true
				return m, tea.Quit
			case matches(m.keys.New, pressed):
				if m.hideNewProject {
					break
				}
				m.screen = screenForm
				return m, nil
			case matches(m.keys.Edit, pressed):
				if m.onEdit == nil {
					break
				}
				if it, ok := m.list.SelectedItem().(Item); ok {
					m.openEditForm(it)
					return m, nil
				}
			case matches(m.keys.OpenLayout, pressed):
				if m.onOpenLayout == nil {
					break
				}
				if it, ok := m.list.SelectedItem().(Item); ok {
					cmd := m.onOpenLayout(it.Key)
					if cmd == nil {
						break
					}
					// tea.ExecProcess suspends the alt-screen for the editor and resumes it after.
					return m, cmd
				}
			case matches(m.keys.Delete, pressed):
				if m.onDelete == nil {
					break
				}
				if it, ok := m.list.SelectedItem().(Item); ok {
					m.openDeleteConfirm(it, false)
					return m, nil
				}
			case matches(m.keys.Purge, pressed):
				if m.onDelete == nil {
					break
				}
				if it, ok := m.list.SelectedItem().(Item); ok {
					m.openDeleteConfirm(it, true)
					return m, nil
				}
			case matches(m.keys.Kill, pressed):
				if m.onKill == nil {
					break
				}
				it, ok := m.list.SelectedItem().(Item)
				if !ok {
					break
				}
				// Gated on Status == "running": pressing K on a stopped
				// project would either error from CloseWindow or no-op
				// silently. Both are user-hostile; a faint status hint
				// makes the affordance discoverable without surprise.
				if it.Status != "running" {
					m.setStatusErr(fmt.Sprintf("%s is not running", it.Key))
					return m, nil
				}
				return m.runIntent(KillIntent{Name: it.Key})
			case matches(m.keys.SetLayout, pressed):
				if m.onSetLayout == nil {
					break
				}
				if it, ok := m.list.SelectedItem().(Item); ok {
					if m.openSetLayout(it) {
						return m, nil
					}
				}
			case matches(m.keys.CycleTheme, pressed):
				return m, m.cycleTheme()
			case matches(m.keys.Select, pressed):
				switch v := m.list.SelectedItem().(type) {
				case Item:
					if m.onLaunch != nil {
						// Stay-alive launch: picker remains open while the project window opens.
						m.status = statusLine{text: fmt.Sprintf("launching %s…", v.Key)}
						return m, m.onLaunch(v.Key)
					}
					// Legacy quit-and-handoff path (selection-only callers).
					m.intent = SwitchIntent{Name: v.Key}
					return m, tea.Quit
				case newProjectItem:
					if m.hideNewProject {
						break
					}
					m.screen = screenForm
					return m, nil
				}
			}
		}
	}
	// Track selection before delegating to detect cursor movement and dispatch a fresh preview.
	prevKey := ""
	if it, ok := m.list.SelectedItem().(Item); ok {
		prevKey = it.Key
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	// If the selection changed, dispatch an async preview for the new item.
	newKey := ""
	if it, ok := m.list.SelectedItem().(Item); ok {
		newKey = it.Key
	}
	if newKey != "" && newKey != prevKey {
		cmd = tea.Batch(cmd, m.startPreview())
	}
	return m, cmd
}

// openEditForm switches to the form pre-populated with the project's current values
// in edit mode. Mutates inputs in place so the cycler index, layout names, and preview
// callback stay wired up.
func (m *model) openEditForm(it Item) {
	m.form.inputs[fieldName].SetValue(it.Key)
	m.form.inputs[fieldDir].SetValue(it.Description)
	m.form.inputs[fieldFrom].SetValue("") // hidden in edit mode but reset for cleanliness
	if it.Layout != "" {
		m.form.inputs[fieldTemplate].SetValue(it.Layout)
		// Re-anchor the cycler at the project's current template so ←/→ cycles from "what it is now".
		for i, n := range m.form.layoutNames {
			if n == it.Layout {
				m.form.layoutIdx = i
				break
			}
		}
	}
	m.form.err = ""
	m.form.focus = fieldName
	for i := range m.form.inputs {
		m.form.inputs[i].Blur()
	}
	m.form.inputs[fieldName].Focus()
	m.form.setEditMode(true, it.Key)
	m.screen = screenForm
}

// openDeleteConfirm transitions to the confirm modal for the given item.
func (m *model) openDeleteConfirm(it Item, purge bool) {
	title := "Delete project?"
	if purge {
		title = "Delete project and close window?"
	}
	m.confirm = confirmModel{
		title:   title,
		body:    formatDeleteBody(it.Title, it.Description, purge),
		pending: DeleteIntent{Name: it.Key, Purge: purge},
	}
	m.screen = screenConfirm
}

// openSetLayout transitions to the set-layout sub-screen for the given project.
// Returns false (no screen switch) when no template names are available.
func (m *model) openSetLayout(it Item) bool {
	if len(m.layoutNames) == 0 {
		return false
	}
	m.setLayout = newSetLayoutModel(it.Key, it.Layout, m.layoutNames, m.previewTemplate)
	m.screen = screenSetLayout
	return true
}

// openLayoutEditor stashes the form-submitted intent and transitions to the
// layout editor sub-screen with the resolved layout. The editor's ctrl+s/esc
// handlers fish the intent back out, attach (or skip) the materialised layout,
// and finally call runIntent.
func (m *model) openLayoutEditor(intent NewProjectIntent, lay *layout.Layout) {
	pending := intent
	m.pendingNewProject = &pending
	m.layoutEditor = newLayoutEditorModel(intent.Name, intent.Template, lay)
	m.screen = screenLayoutEditor
}

// updateConfirm handles y/n on the active confirm modal. Confirm dispatches
// the pending intent through runIntent; cancel drops back to the list.
func (m *model) updateConfirm(msg tea.Msg) (tea.Model, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok {
		pressed := km.String()
		switch {
		case matches(m.keys.ConfirmYes, pressed):
			if m.confirm.pending == nil {
				// Defensive: should be impossible if openDeleteConfirm is the only entry point.
				m.screen = screenList
				return m, nil
			}
			// Dispatch the pending intent through the in-TUI action runner.
			pending := m.confirm.pending
			m.confirm = confirmModel{}
			return m.runIntent(pending)
		case matches(m.keys.ConfirmNo, pressed):
			m.confirm = confirmModel{}
			m.screen = screenList
			return m, nil
		}
	}
	return m, nil
}

// runIntent executes a mutating intent via configured action callbacks, then refreshes
// the list. SwitchIntent and NewProjectIntent exit the TUI (CLI takes over the terminal);
// all others run in-process and return to the list.
func (m *model) runIntent(in Intent) (tea.Model, tea.Cmd) {
	switch v := in.(type) {
	case SwitchIntent, NewProjectIntent:
		// Quit-and-handoff: CLI executes after Run returns.
		m.intent = in
		return m, tea.Quit
	case DeleteIntent:
		if m.onDelete == nil {
			// No callback — shouldn't be reachable (d/D gated on onDelete being set).
			m.screen = screenList
			return m, nil
		}
		warns, err := m.onDelete(v.Name, v.Purge)
		if err != nil {
			return m.showError(fmt.Sprintf("delete %q: %v", v.Name, err)), nil
		}
		switch {
		case len(warns) > 1:
			// Multiple side-effect failures: log all, show first + count in TUI.
			for _, w := range warns {
				slog.Warn("picker: delete warning", "project", v.Name, "warning", w)
			}
			m.setStatusOK(fmt.Sprintf("deleted %q (%s; +%d more)", v.Name, warns[0], len(warns)-1))
		case len(warns) == 1:
			slog.Warn("picker: delete warning", "project", v.Name, "warning", warns[0])
			m.setStatusOK(fmt.Sprintf("deleted %q (%s)", v.Name, warns[0]))
		case v.Purge:
			m.setStatusOK(fmt.Sprintf("deleted %s and closed window", v.Name))
		default:
			m.setStatusOK(fmt.Sprintf("deleted %s", v.Name))
		}
		m.screen = screenList
		return m, m.startEnrich()
	case KillIntent:
		if m.onKill == nil {
			m.screen = screenList
			return m, nil
		}
		if err := m.onKill(v.Name); err != nil {
			return m.showError(fmt.Sprintf("kill %q: %v", v.Name, err)), nil
		}
		m.setStatusOK(fmt.Sprintf("closed window for %s", v.Name))
		m.screen = screenList
		return m, m.startEnrich()
	case SetLayoutIntent:
		if m.onSetLayout == nil {
			m.screen = screenList
			return m, nil
		}
		if err := m.onSetLayout(v.Name, v.Template); err != nil {
			return m.showError(fmt.Sprintf("set layout for %q to %q: %v", v.Name, v.Template, err)), nil
		}
		m.setStatusOK(fmt.Sprintf("set layout for %s to %s", v.Name, v.Template))
		// Invalidate cached preview: layout template changed, preview would show stale data.
		delete(m.previewCache, v.Name)
		m.screen = screenList
		return m, m.startEnrich()
	case EditIntent:
		if m.onEdit == nil {
			m.screen = screenList
			return m, nil
		}
		if err := m.onEdit(v.OldName, v.NewName, v.NewDir, v.NewTemplate); err != nil {
			return m.showError(fmt.Sprintf("edit %q: %v", v.OldName, err)), nil
		}
		// Status describes what changed concisely; if the rename
		// happened, lead with old→new, otherwise just the project name.
		if v.OldName != v.NewName {
			m.setStatusOK(fmt.Sprintf("edited %s → %s", v.OldName, v.NewName))
		} else {
			m.setStatusOK(fmt.Sprintf("edited %s", v.NewName))
		}
		// Invalidate cached previews for both names (name, dir, or template may have changed).
		delete(m.previewCache, v.OldName)
		delete(m.previewCache, v.NewName)
		// Reset form to new-project mode so the next 'n' doesn't show leftover edit state.
		m.form.setEditMode(false, "")
		m.screen = screenList
		return m, m.startEnrich()
	default:
		// Unknown intent — sealed interface forbids external implementations; defensive only.
		m.screen = screenList
		return m, nil
	}
}

// refreshList rebuilds the list from the RefreshItems callback. On error, keeps
// existing items. On success with nil/empty slice, transitions to empty-state view.
func (m *model) refreshList() {
	if m.refreshItems == nil {
		return
	}
	items, err := m.refreshItems()
	if err != nil {
		slog.Warn("picker: refresh failed, keeping existing items", "err", err)
		return
	}
	listItems := make([]list.Item, 0, len(items)+1)
	for _, it := range items {
		listItems = append(listItems, it)
	}
	if !m.hideNewProject {
		listItems = append(listItems, newProjectItem{})
	}
	m.list.SetItems(listItems)
}

// showError transitions to screenError and mirrors the failure into the status bar
// so it persists after the user dismisses the error screen.
func (m *model) showError(msg string) *model {
	m.errMsg = msg
	m.screen = screenError
	m.setStatusErr(msg)
	return m
}

// setStatusOK records a successful action's outcome for the status bar.
func (m *model) setStatusOK(msg string) {
	m.status = statusLine{text: msg, isErr: false}
}

// setStatusErr records a failed action's outcome for the status bar.
func (m *model) setStatusErr(msg string) {
	m.status = statusLine{text: msg, isErr: true}
}

// cycleTheme advances to the next theme alphabetically, applies it live, and persists
// it to disk when configPath is set. On load failure the cycle still advances.
// On persist failure the in-memory switch takes effect and the status bar surfaces the error.
func (m *model) cycleTheme() tea.Cmd {
	names, err := theme.List(m.themesDir)
	if err != nil || len(names) == 0 {
		m.setStatusErr(fmt.Sprintf("themes: %v", err))
		return nil
	}

	// Default to -1 so an unknown current name advances to names[0] rather than wrapping past the end.
	idx := -1
	for i, n := range names {
		if n == m.themeName {
			idx = i
			break
		}
	}
	next := names[(idx+1)%len(names)]

	t, ok := ThemeByName(m.themesDir, next)
	if !ok {
		// theme.List returned a name ThemeByName couldn't load — skip and report.
		m.setStatusErr(fmt.Sprintf("theme %q failed to load", next))
		return nil
	}

	m.applyTheme(next, t)
	previewCmd := m.startPreview()

	// Persist to disk when a config path is available.
	if m.configPath != "" {
		if perr := config.SetUITheme(m.configPath, next); perr != nil {
			slog.Warn("picker: failed to persist theme", "theme", next, "err", perr)
			m.setStatusOK(fmt.Sprintf("theme: %s (session only — failed to persist: %v)", next, perr))
			return previewCmd
		}
	}
	m.setStatusOK(fmt.Sprintf("theme: %s", next))
	return previewCmd
}

// applyTheme swaps the live theme on every surface that caches lipgloss styles.
// When PreviewProjectFactory is configured, re-creates the previewProject closure
// with the new theme and clears the preview cache.
func (m *model) applyTheme(name string, t Theme) {
	m.theme = t
	m.themeName = name
	// Rebuild list delegate so row styles pick up the new palette.
	m.list.SetDelegate(newDelegate(t))
	m.list.Styles.Title = t.ListTitle
	m.form.theme = t
	// Re-create preview closure and clear cache so stale styled strings are not shown.
	if m.previewProjectFactory != nil {
		m.previewProject = m.previewProjectFactory(t)
		m.previewCache = nil
	}
}

// updateError dismisses the error screen on any keypress (deliberate — any key prevents
// a reflexive keystroke from accidentally triggering another action).
func (m *model) updateError(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := msg.(tea.KeyMsg); ok {
		m.errMsg = ""
		m.screen = screenList
	}
	return m, nil
}

// viewError renders the error modal.
func (m *model) viewError() string {
	body := m.theme.RightPaneTitle.Render("Error") + "\n\n" +
		m.errMsg + "\n\n" +
		m.theme.RightPaneFaint.Render("Press any key to dismiss.")
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Padding(1, 2).
		Render(body)
}

func (m *model) updateForm(msg tea.Msg) (tea.Model, tea.Cmd) {
	cmd, submitted, intent, cancelled := m.form.update(msg)
	if cancelled {
		if m.formOnly {
			m.cancelled = true
			return m, tea.Quit
		}
		// Edit mode exits cleanly back to the list. Reset the flag so
		// a subsequent 'n' opens a fresh new-project form.
		if m.form.editMode {
			m.form.setEditMode(false, "")
		}
		m.screen = screenList
		return m, nil
	}
	if submitted {
		// New-project submits with a non-empty template AND a wired ResolveLayout
		// detour through the layout editor sub-screen instead of dispatching
		// straight away. Edit intents and template-less / unresolved cases keep
		// the original behaviour. Resolution failure is logged and falls through
		// — the editor is a polish path, not load-bearing for project creation.
		if np, ok := intent.(NewProjectIntent); ok && np.Template != "" && m.resolveLayout != nil {
			lay, err := m.resolveLayout(np.Template)
			if err != nil {
				slog.Warn("picker: resolve layout for editor failed, skipping editor",
					"template", np.Template, "err", err)
			} else if lay != nil {
				m.openLayoutEditor(np, lay)
				return m, nil
			}
		}
		// Dispatch through runIntent: edit intents run in-loop; NewProjectIntent exits the TUI.
		return m.runIntent(intent)
	}
	return m, cmd
}

func (m *model) updateAlreadyRegistered(msg tea.Msg) (tea.Model, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok {
		pressed := km.String()
		switch {
		case matches(m.keys.Confirm, pressed):
			m.intent = SwitchIntent{Name: m.alreadyRegisteredAs}
			return m, tea.Quit
		case matches(m.keys.Switch, pressed):
			m.screen = screenForm
			return m, nil
		case matches(m.keys.Quit, pressed), matches(m.keys.Cancel, pressed):
			// Both Quit (q/ctrl+c) and Cancel (esc) dismiss this screen.
			m.cancelled = true
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m *model) View() string {
	var raw string
	switch m.screen {
	case screenAlreadyRegistered:
		raw = m.viewAlreadyRegistered()
	case screenForm:
		raw = m.form.view()
	case screenConfirm:
		raw = m.confirm.view(m.theme, m.width, m.height)
	case screenSetLayout:
		raw = m.setLayout.view(m.theme)
	case screenLayoutEditor:
		raw = m.layoutEditor.view(m.theme, m.width)
	case screenError:
		raw = m.viewError()
	default:
		raw = m.viewList()
	}
	// Hard cap to the terminal viewport. lipgloss auto-wrap can add rows into bordered
	// content beyond per-pane sizing; MaxHeight/MaxWidth are the last line of defence.
	if m.width > 0 && m.height > 0 {
		return lipgloss.NewStyle().MaxHeight(m.height).MaxWidth(m.width).Render(raw)
	}
	return raw
}

// viewList renders the project-list screen. Composition:
//
//	┌──────────────────┐  ┌────────────┐
//	│ list (bordered)  │  │ right pane │   ← split mode only
//	└──────────────────┘  └────────────┘
//	status bar (1 line)
//
// In non-split mode the right pane is omitted but the list border and
// status bar still render — the visual frame is consistent regardless
// of terminal width.
func (m *model) viewList() string {
	innerListWidth := m.width - listPaneInnerInset
	if m.splitActive() {
		innerListWidth = m.width - rightPaneWidth - 1 - listPaneInnerInset
	}
	if innerListWidth < listMinInnerWidth {
		innerListWidth = listMinInnerWidth
	}
	innerHeight := m.usableInnerHeight()
	listView := m.list.View()
	if m.brandStripActive(innerHeight) {
		// 3-row strip + 1-row gap = brandStripReserved rows above the list.
		listView = m.theme.RightPaneFaint.Render(brandStrip) + "\n\n" + listView
	}
	// lipgloss Width(N): N is frame-minus-border; add back padding cols for content area match.
	listBoxed := m.theme.ListPaneBorder.
		Width(innerListWidth + listPanePaddingCols).
		Height(innerHeight).
		Render(listView)

	var panes string
	if m.splitActive() {
		panes = lipgloss.JoinHorizontal(lipgloss.Top, listBoxed, " ", m.viewRightPane())
	} else {
		panes = listBoxed
	}
	return panes + "\n" + m.viewStatusBar()
}

// viewStatusBar renders the bottom status line. Hard-truncated to m.width so a long
// error message can never wrap and steal a row from the panes (statusBarHeight=1).
func (m *model) viewStatusBar() string {
	const idleHint = "press ? for help"
	var raw, prefix string
	style := m.theme.StatusBarFaint
	switch {
	case m.status.text == "":
		raw = idleHint
	case m.status.isErr:
		prefix = "✖ "
		raw = m.status.text
		style = m.theme.StatusBarError
	default:
		prefix = "✓ "
		raw = m.status.text
		style = m.theme.StatusBar
	}
	return style.Render(truncateToWidth(prefix+raw, m.width))
}

// truncateToWidth clips s to at most width visible runes, replacing the last with "…".
// Returns s unchanged when width<=0 (WindowSizeMsg not yet received).
func truncateToWidth(s string, width int) string {
	if width <= 0 {
		return s
	}
	runes := []rune(s)
	if len(runes) <= width {
		return s
	}
	if width <= 1 {
		return string(runes[:width])
	}
	// Replace the last rune with "…" to signal truncation.
	return string(runes[:width-1]) + "…"
}

// viewRightPane renders the context-sensitive right pane. Content depends on the
// selected item: Item → project detail; newProjectItem → hint; empty → placeholder.
// Content is hard-clipped to innerHeight before being handed to lipgloss.
func (m *model) viewRightPane() string {
	// rightPaneWidth - rightPaneBorderCols gives content+padding width for lipgloss Width().
	innerHeight := m.usableInnerHeight()
	// Right pane gets +borderCorrectionPx height to match the list pane's
	// apparent rendered height at runtime. See geometry.go for why.
	border := m.theme.RightPaneBorder.Width(rightPaneWidth - rightPaneBorderCols).Height(innerHeight + borderCorrectionPx)

	switch v := m.list.SelectedItem().(type) {
	case Item:
		return border.Render(clipToHeight(m.renderItemDetail(v, rightPaneInnerWidth), innerHeight))
	case newProjectItem:
		// Empty state: show brand mascot. Once the user has a project, drop to a plain prompt.
		hasProjects := false
		for _, it := range m.list.Items() {
			if _, ok := it.(Item); ok {
				hasProjects = true
				break
			}
		}
		var body string
		if hasProjects {
			body = m.theme.RightPaneTitle.Render("+ New project") + "\n\n" +
				m.theme.RightPaneFaint.Render("Press enter (or n / +) to register a project from the current directory or clone a fresh repo.")
		} else {
			body = m.theme.RightPaneFaint.Render(brandHero) + "\n\n" +
				m.theme.RightPaneTitle.Render(brandTagline) + "\n\n" +
				m.theme.RightPaneFaint.Render("No projects yet. Press enter (or n / +) to register your first one.")
		}
		return border.Render(clipToHeight(body, innerHeight))
	default:
		// Empty list — no items at all.
		return border.Render(clipToHeight(m.theme.RightPaneFaint.Render("No projects yet.\n\nPress n to register your first one."), innerHeight))
	}
}

// clipToHeight truncates s to at most height lines, replacing the last with "…".
// Returns s unchanged when height<=0.
func clipToHeight(s string, height int) string {
	if height <= 0 {
		return s
	}
	lines := strings.Split(s, "\n")
	if len(lines) <= height {
		return s
	}
	if height == 1 {
		return "…"
	}
	clipped := lines[:height]
	clipped[height-1] = "…"
	return strings.Join(clipped, "\n")
}

// renderItemDetail builds the right-pane content for a project Item.
// Reads from the async preview cache; renders "loading…" on a cache miss.
// Falls back to Item fields when no PreviewProject callback is wired.
func (m *model) renderItemDetail(it Item, _ int) string {
	if m.previewProject != nil {
		// nil map read is safe in Go — returns ("", false).
		if cached, hit := m.previewCache[it.Key]; hit {
			if cached != "" {
				return cached
			}
			// Callback returned empty: fall through to the Item fallback.
		} else {
			// Not yet in cache: async cmd is in-flight.
			return m.theme.RightPaneFaint.Render("loading…")
		}
	}
	// Minimal fallback from the Item itself.
	var b strings.Builder
	b.WriteString(m.theme.RightPaneTitle.Render(it.Title))
	b.WriteString("\n\n")
	if it.Description != "" {
		b.WriteString(m.theme.RightPaneLabel.Render("Directory") + "\n")
		b.WriteString(m.theme.RightPaneValue.Render(it.Description) + "\n\n")
	}
	if it.Status != "" {
		b.WriteString(m.theme.RightPaneLabel.Render("Status") + "\n")
		b.WriteString((itemDelegate{theme: m.theme}).renderStatus(it.Status) + "\n")
	}
	return b.String()
}

func (m *model) viewAlreadyRegistered() string {
	out := m.theme.AlreadyRegisteredTitle.Render(
		"This directory is already registered")
	out += "\n\nProject: " + m.theme.AlreadyRegisteredName.Render(m.alreadyRegisteredAs)
	// Derive the footer text from the keymap so it can't drift from updateAlreadyRegistered.
	footer := fmt.Sprintf("[%s] %s   [%s] %s   [esc] cancel",
		m.keys.Confirm.Help().Key, m.keys.Confirm.Help().Desc,
		m.keys.Switch.Help().Key, m.keys.Switch.Help().Desc)
	out += "\n\n" + m.theme.AlreadyRegisteredHelp.Render(footer)
	return out
}

type itemDelegate struct {
	theme Theme
}

func newDelegate(t Theme) itemDelegate { return itemDelegate{theme: t} }

// Height is 1: one line per project. Right pane shows directory and metadata in detail.
func (itemDelegate) Height() int                             { return 1 }
func (itemDelegate) Spacing() int                            { return 0 }
func (itemDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

func (d itemDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	selected := index == m.Index()

	if _, ok := listItem.(newProjectItem); ok {
		cursor := brandCursorInactive
		titleS := d.theme.NewProject
		if selected {
			cursor = brandCursor
			titleS = d.theme.NewProjectFocus
		}
		_, _ = fmt.Fprint(w, cursor+titleS.Render("+ New project"))
		return
	}

	it, ok := listItem.(Item)
	if !ok {
		return
	}
	titleS := d.theme.Title
	if selected {
		titleS = d.theme.SelectedTitle
	}
	cursor := brandCursorInactive
	if selected {
		cursor = brandCursor
	}

	// Single-line layout: cursor + title + status pill + optional trailing (last-launched).
	line := fmt.Sprintf("%s%s   %s", cursor, titleS.Render(it.Title), d.renderStatus(it.Status))
	if it.Trailing != "" {
		line += "   " + d.theme.Trailing.Render(it.Trailing)
	}
	_, _ = fmt.Fprint(w, line)
}

func (d itemDelegate) renderStatus(s string) string {
	switch s {
	case "running":
		return d.theme.StatusRunning.Render("● running")
	case "dir-missing":
		return d.theme.StatusBroken.Render("✖ dir missing")
	case "":
		return ""
	default:
		return d.theme.StatusStopped.Render("○ " + s)
	}
}
