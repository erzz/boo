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
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/erzz/boo/internal/config"
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

// Intent is the sealed sum type of "what the user wants to do next" returned
// by Run. CLI code dispatches on the concrete type via a type switch.
//
// New intents are added as new struct types implementing isIntent(). The
// interface is sealed (unexported method) so external packages can't add
// variants and break the exhaustive switches in CLI dispatch code.
type Intent interface{ isIntent() }

// SwitchIntent — user picked an existing project from the list.
type SwitchIntent struct{ Name string }

// DeleteIntent — user pressed d (or D for purge) on a project row and
// confirmed the modal. The CLI is responsible for the actual registry
// removal, state-dir cleanup, and (if Purge) closing the Ghostty window.
//
// Confirmation has already happened in the TUI; the CLI should NOT
// prompt again. (`boo delete --force` semantics.)
type DeleteIntent struct {
	Name  string
	Purge bool
}

// SetLayoutIntent — user picked a new template on the set-layout
// sub-screen. The CLI re-resolves the template and writes both the
// per-project layout snapshot and the registry's display field, same
// as `boo set-layout <name> <template>`.
type SetLayoutIntent struct {
	Name     string
	Template string
}

// EditIntent — user submitted the edit form for an existing project.
// OldName is the project's pre-edit key (used to find the row to
// update); NewName, NewDir, NewTemplate are the desired post-edit
// values. The CLI side decides whether each constitutes a real change
// (a no-op edit is fine and silently does nothing).
//
// Like Delete and SetLayout, edits are dispatched through the in-TUI
// action loop via Options.OnEdit; the picker stays open and refreshes
// the list on success or shows an error screen on failure.
type EditIntent struct {
	OldName     string
	NewName     string
	NewDir      string
	NewTemplate string
}

// NewProjectIntent — user submitted the new-project form. The CLI turns
// this into the actual registry/clone work.
//
// Exactly one of Dir or From is meaningful at submit time:
//
//   - If From is non-empty, the project will be created by cloning into Dir
//     (Dir defaults to a sibling of cwd named after the repo).
//   - Otherwise Dir is the existing directory to register.
type NewProjectIntent struct {
	Name     string
	Dir      string
	From     string
	Template string
}

func (SwitchIntent) isIntent()     {}
func (DeleteIntent) isIntent()     {}
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
	// PreviewTemplate, if set, is called whenever the form's "Layout
	// template" field changes; the returned string is rendered below the
	// form as a preview of what the layout will look like. Empty return =
	// no preview shown (e.g. unknown template name while the user types).
	//
	// We pass a callback rather than importing the layout package here so
	// the picker stays free of project-specific dependencies — the same
	// reason Item, FormDefaults etc. don't reference internal/project or
	// internal/ghostty.
	PreviewTemplate func(name string) string
	// LayoutNames, if non-empty, turns the form's "Layout template" field
	// from a free-text input into a left/right cycler over this list. The
	// CLI populates it from layout.ListTemplates so users see exactly the
	// templates they have available (built-ins + any user overrides) and
	// can't typo their way into a validation error after submit.
	//
	// Empty = back to the legacy free-text input. Useful for tests and
	// for any future caller that wants the old behaviour.
	LayoutNames []string
	// Theme selects a named visual theme. Empty or unknown names fall
	// back to the built-in default. The CLI populates this from the
	// `ui.theme` config key.
	Theme string
	// ThemesDir is the directory holding user-authored theme YAML
	// files (typically ~/.config/boo/themes). Empty means "built-ins
	// only" — useful in tests and when boo runs on a clean machine.
	ThemesDir string
	// ConfigPath is the path to the user's config.yaml. When set, the
	// picker's `T` keybind cycles through available themes and
	// persists the choice by rewriting this file. Empty disables the
	// keybind (the picker still respects Theme for the session, but
	// can't save changes — used in tests and any future read-only
	// caller).
	ConfigPath string
	// PreviewProject, if set, is called whenever the cursor moves to a
	// project row in the list; the returned multi-line string is shown
	// in the right-hand split pane. Empty return = render a default
	// "no preview" placeholder.
	//
	// Like PreviewTemplate, this is a callback so the picker package
	// stays free of project-specific dependencies (project.Registry,
	// project.LoadRuntime, ghostty.WindowExists, etc. live in CLI).
	PreviewProject func(name string) string

	// Action callbacks. nil = the corresponding key is disabled and
	// hidden from the help footer (so `boo delete` selection-only
	// pickers don't advertise actions they can't perform).
	//
	// Successful callbacks return nil; the picker then calls
	// RefreshItems and re-renders the list. Failed callbacks return an
	// error which the picker shows on a transient error screen until
	// the user dismisses it with any key.
	//
	// Confirmation/UX flow lives in the picker. The CLI side just
	// performs the side effect — it does NOT prompt or print, because
	// we're still inside the alt-screen and the picker owns the
	// presentation.
	OnDelete    func(name string, purge bool) error
	OnSetLayout func(name, template string) error
	// OnEdit applies a pending edit. Receives the original project key
	// (oldName) plus the user's desired post-edit values. The CLI
	// decides whether each field actually changed and what side effects
	// (rename state dir, re-resolve template, etc.) are needed; nil
	// callback = the 'e' key is disabled and dropped from help.
	OnEdit func(oldName, newName, newDir, newTemplate string) error

	// OnOpenLayout returns a tea.Cmd that opens the highlighted
	// project's layout file in $EDITOR. The CLI is expected to wrap
	// tea.ExecProcess so the alt-screen suspends, the editor runs in
	// the user's controlling terminal, and the picker resumes when the
	// editor exits. Returning nil from the callback (or leaving the
	// field nil) disables the 'o' key and drops it from help.
	//
	// We give the picker a tea.Cmd rather than running the editor
	// here so the picker package stays free of $EDITOR / exec.Command
	// knowledge — same separation as PreviewProject etc.
	OnOpenLayout func(name string) tea.Cmd

	// RefreshItems is called after a successful action callback. It
	// returns the new list of items the picker should display. nil =
	// the picker leaves the existing list in place (which will show
	// stale data until the user exits and re-enters), so most callers
	// should set this whenever they set any On* callback.
	RefreshItems func() []Item
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

	m := &model{
		list:                l,
		form:                form,
		theme:               theme,
		keys:                keys,
		screen:              scr,
		themeName:           opts.Theme,
		themesDir:           opts.ThemesDir,
		configPath:          opts.ConfigPath,
		formOnly:            formOnly,
		hideNewProject:      opts.HideNewProject,
		alreadyRegisteredAs: opts.Defaults.AlreadyRegisteredAs,
		previewProject:      opts.PreviewProject,
		previewTemplate:     opts.PreviewTemplate,
		layoutNames:         opts.LayoutNames,
		onDelete:            opts.OnDelete,
		onSetLayout:         opts.OnSetLayout,
		onEdit:              opts.OnEdit,
		onOpenLayout:        opts.OnOpenLayout,
		refreshItems:        opts.RefreshItems,
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
	// knows where in the cycle to advance from. themesDir + configPath
	// let cycleTheme reload the theme list and persist the choice;
	// when configPath is empty the keybind is a no-op.
	themeName  string
	themesDir  string
	configPath string

	formOnly            bool   // when true, esc on form cancels the whole TUI rather than going back to the list
	hideNewProject      bool   // when true, the form/intent path is unreachable; selection-only mode
	alreadyRegisteredAs string // shown on the AlreadyRegistered screen
	previewProject      func(name string) string
	previewTemplate     func(name string) string // shared with form; reused by setLayout sub-screen
	layoutNames         []string                 // shared with form; reused by setLayout sub-screen

	// Action callbacks (see Options docs). nil = action key is disabled.
	onDelete     func(name string, purge bool) error
	onSetLayout  func(name, template string) error
	onEdit       func(oldName, newName, newDir, newTemplate string) error
	onOpenLayout func(name string) tea.Cmd
	refreshItems func() []Item

	intent    Intent // nil + cancelled=false should not happen; nil + cancelled=true means dismissed
	cancelled bool

	// confirm is the active modal, valid iff screen == screenConfirm.
	confirm confirmModel

	// setLayout is the set-layout sub-screen state, valid iff
	// screen == screenSetLayout.
	setLayout setLayoutModel

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

// splitThreshold is the minimum total terminal width below which we
// collapse the list+detail split-pane back to single-pane (just the list).
// At 90 cols the list pane gets ~45 inner cols (after rightPaneWidth=40 +
// 1-col gutter + 4 cols of list-pane border/padding) — enough for project
// name + status pill + last-launched without horizontal cramping, and the
// 40-col right pane has comfortable room for metadata + a 36-col layout
// preview. Was 70 (and 80 before that): at 70 the math fit but produced
// an exact-fit layout in real Ghostty panes (~83 cols) where any content
// drift overflowed visibly into the adjacent pane. 90 keeps the
// problem range (~70-89 cols) in single-pane mode where overflow is
// harmless.
const splitThreshold = 90

// splitMinHeight is the minimum terminal height below which split mode
// is suppressed regardless of width. The right-pane content (project
// title + 4 metadata rows + "layout preview" header + tab title + an
// 8-row layout preview ASCII = ~17 rows minimum, more if the dir path
// wraps) clips ugly when the inner height is below ~20. Below this
// threshold we drop the right pane and use the full width for the list,
// where vertical space is more useful anyway.
const splitMinHeight = 24

// rightPaneWidth is the width budget given to the right pane when split is
// active. Leaving the rest for the list keeps long project names readable.
// 40 chosen as a happy medium between "enough to render a layout preview"
// (~25 cols) and "doesn't squeeze the list" (the list needs ~30+ for
// status pills).
const rightPaneWidth = 40

// Layout overheads used by the WindowSize math. Split out as named
// constants so the relationship between bordered panes and the status
// bar is auditable in one place rather than scattered as +1/+2/+4.
const (
	// statusBarHeight is the number of vertical lines reserved at the
	// bottom of the screen for the status bar.
	statusBarHeight = 1
	// listBorderOverhead is the vertical lines consumed by the list
	// pane's top + bottom border.
	listBorderOverhead = 2
	// listPaneInnerInset is the horizontal cols consumed by the list
	// pane's left/right borders (2) + Padding(0,1) (2 = one col on
	// each side). lipgloss's bordered styles report Width as the
	// *content* width, so we subtract this from the available width
	// before calling list.SetSize.
	listPaneInnerInset = 4
	// listMinInnerWidth is the floor for the list's inner content
	// width. Going below this leaves a useless sliver of a list (no
	// title visible). Mirrors the previous hardcoded "20" floor.
	listMinInnerWidth = 20
	// brandStripReserved is the vertical lines reserved at the top of
	// the list pane for the brand strip (3 rows of art) plus a 1-row
	// gap separating it from the bubbles list title. We only render
	// the strip when the list pane is tall enough to spare 4 rows
	// AND wide enough for the 10-col art (see viewList).
	brandStripReserved = 4
	// brandStripMinHeight is the inner-list-pane height below which
	// we drop the strip rather than starve the items area. Picked so
	// at least ~6 rows remain for items + the bubbles title + help.
	brandStripMinHeight = 12
)

// editorFinishedMsg is dispatched by the CLI's OnOpenLayout callback
// after tea.ExecProcess returns. err is whatever the spawned editor
// returned (nil = success). Picker handles this in Update by refreshing
// the items list (the user may have rewritten the layout file) and
// surfacing any error via screenError.
//
// Public so CLI-side code that constructs the tea.Cmd can dispatch the
// right message; callers should treat it as opaque otherwise.
type EditorFinishedMsg = editorFinishedMsg

type editorFinishedMsg struct {
	err error
}

// NewEditorFinishedMsg constructs an editorFinishedMsg. CLI-side code
// that wraps tea.ExecProcess uses this so it doesn't need to depend on
// the unexported type name.
func NewEditorFinishedMsg(err error) tea.Msg {
	return editorFinishedMsg{err: err}
}

// brandStripActive reports whether the list pane is large enough to
// host the brand strip header. The strip is decorative; it must yield
// when space is tight rather than steal rows from the items list.
func (m *model) brandStripActive(innerListHeight int) bool {
	return innerListHeight >= brandStripMinHeight
}

// usableInnerHeight returns the per-pane content height (rows inside
// the bordered box, excluding borders and the status bar). The -1
// safety margin matches the one in Update's listHeight calc — see the
// comment there for why both sites must agree.
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

func (m *model) Init() tea.Cmd { return nil }

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if sz, ok := msg.(tea.WindowSizeMsg); ok {
		m.width, m.height = sz.Width, sz.Height
		// Reserve 1 line for the status bar at the bottom and 2 lines
		// for the list-pane border (top + bottom edges). The extra
		// 1-line safety margin guards against terminals that report
		// height inclusive of chrome they reserve outside Bubble Tea's
		// alt-screen (e.g. iTerm tab bars, Ghostty's title bar in some
		// configs). Without it, the bottom row of either pane can be
		// scrolled off-screen.
		listHeight := m.usableInnerHeight()
		// Width: in split mode, the right pane + gutter take
		// rightPaneWidth+1 cols. In non-split mode, the list spans the
		// whole width. Either way the list's own border + padding
		// (listPaneInnerInset) eats 4 more cols from the inner area.
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

	// editorFinishedMsg arrives after tea.ExecProcess returns — the
	// alt-screen has already been restored by the runtime. We just
	// need to pull fresh data (the user may have rewritten the layout
	// file) and surface any spawn error. Handled here at the top of
	// Update so a late-arriving message can't be swallowed by a
	// sub-screen dispatch (e.g. if the user opened the form between
	// pressing 'o' and the editor exiting — shouldn't happen, but
	// dispatching from here is robust either way).
	if fin, ok := msg.(editorFinishedMsg); ok {
		if fin.err != nil {
			return m.showError(fmt.Sprintf("editor: %v", fin.err)), nil
		}
		m.refreshList()
		m.setStatusOK("layout file saved")
		return m, nil
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
					// tea.ExecProcess (which the CLI wraps around the
					// editor) suspends the alt-screen for the duration
					// of the editor and resumes it afterwards. The
					// CLI's callback wraps the editor's exit in an
					// editorFinishedMsg so we can refresh items below.
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
				m.cycleTheme()
				return m, nil
			case matches(m.keys.Select, pressed):
				switch v := m.list.SelectedItem().(type) {
				case Item:
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
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

// openEditForm switches to the form sub-screen pre-populated with the
// highlighted project's current values and flipped into edit mode. The
// form's existing inputs are mutated in place (rather than constructing
// a fresh formModel) so the cycler index, layout names, and preview
// callback all stay wired up.
//
// Item.Description carries the project's directory (set by the CLI's
// item-builder), and Item.Layout carries the template name. Both can
// be edited freely — the CLI decides which changes are meaningful.
func (m *model) openEditForm(it Item) {
	m.form.inputs[fieldName].SetValue(it.Key)
	m.form.inputs[fieldDir].SetValue(it.Description)
	m.form.inputs[fieldFrom].SetValue("") // hidden in edit mode but reset for cleanliness
	if it.Layout != "" {
		m.form.inputs[fieldTemplate].SetValue(it.Layout)
		// Re-anchor the cycler at the project's current template so
		// ←/→ moves relative to "what it is now".
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

// openDeleteConfirm transitions to the confirm modal pre-loaded with a
// DeleteIntent for the given item. Pulled out so the d/D handlers stay
// readable and to give tests a stable hook.
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

// openSetLayout transitions to the set-layout sub-screen for the given
// project, anchoring the cycler at the project's current template.
//
// Returns false (and does NOT switch screen) when no template names are
// available — pressing 'l' in a picker that wasn't given LayoutNames
// (e.g. selection-only mode for `boo delete`) is a no-op rather than a
// confusing empty cycler.
func (m *model) openSetLayout(it Item) bool {
	if len(m.layoutNames) == 0 {
		return false
	}
	m.setLayout = newSetLayoutModel(it.Key, it.Layout, m.layoutNames, m.previewTemplate)
	m.screen = screenSetLayout
	return true
}

// updateConfirm handles y/n on the active confirm modal.
//
// On confirm: copy modal.pending into mm.intent and quit. On cancel:
// drop back to the list (modal pending is discarded). The CLI then
// type-switches on the intent.
func (m *model) updateConfirm(msg tea.Msg) (tea.Model, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok {
		pressed := km.String()
		switch {
		case matches(m.keys.ConfirmYes, pressed):
			if m.confirm.pending == nil {
				// Defensive: should be impossible if openDeleteConfirm
				// is the only entry point, but rather than panic we
				// just drop back to the list.
				m.screen = screenList
				return m, nil
			}
			// Dispatch the pending intent through the in-TUI action
			// runner. Successful actions return us to the list with
			// fresh data; failed ones land on screenError.
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

// runIntent executes a mutating intent in-process via the configured
// action callbacks, then refreshes the list. SwitchIntent and
// NewProjectIntent are handed back to the caller (they exit the TUI
// because they hand off to Ghostty); everything else is handled here.
//
// On callback error we transition to screenError so the user can read
// the message before it disappears. Old-style "set m.intent + tea.Quit"
// is reserved for intents that genuinely need the CLI to take over the
// terminal.
func (m *model) runIntent(in Intent) (tea.Model, tea.Cmd) {
	switch v := in.(type) {
	case SwitchIntent, NewProjectIntent:
		// Quit-and-handoff intents — CLI executes after Run returns.
		m.intent = in
		return m, tea.Quit
	case DeleteIntent:
		if m.onDelete == nil {
			// No callback wired — shouldn't be reachable because the
			// d/D keys are gated on onDelete being set, but keep the
			// system honest.
			m.screen = screenList
			return m, nil
		}
		if err := m.onDelete(v.Name, v.Purge); err != nil {
			return m.showError(fmt.Sprintf("delete %q: %v", v.Name, err)), nil
		}
		m.refreshList()
		if v.Purge {
			m.setStatusOK(fmt.Sprintf("deleted %s and closed window", v.Name))
		} else {
			m.setStatusOK(fmt.Sprintf("deleted %s", v.Name))
		}
		m.screen = screenList
		return m, nil
	case SetLayoutIntent:
		if m.onSetLayout == nil {
			m.screen = screenList
			return m, nil
		}
		if err := m.onSetLayout(v.Name, v.Template); err != nil {
			return m.showError(fmt.Sprintf("set layout for %q to %q: %v", v.Name, v.Template, err)), nil
		}
		m.refreshList()
		m.setStatusOK(fmt.Sprintf("set layout for %s to %s", v.Name, v.Template))
		m.screen = screenList
		return m, nil
	case EditIntent:
		if m.onEdit == nil {
			m.screen = screenList
			return m, nil
		}
		if err := m.onEdit(v.OldName, v.NewName, v.NewDir, v.NewTemplate); err != nil {
			return m.showError(fmt.Sprintf("edit %q: %v", v.OldName, err)), nil
		}
		m.refreshList()
		// Status describes what changed concisely; if the rename
		// happened, lead with old→new, otherwise just the project name.
		if v.OldName != v.NewName {
			m.setStatusOK(fmt.Sprintf("edited %s → %s", v.OldName, v.NewName))
		} else {
			m.setStatusOK(fmt.Sprintf("edited %s", v.NewName))
		}
		// Drop the form back to new-project mode so the next 'n' press
		// doesn't surprise the user with leftover edit state.
		m.form.setEditMode(false, "")
		m.screen = screenList
		return m, nil
	default:
		// Unknown intent — defensive; should be impossible because the
		// sealed interface forbids external implementations.
		m.screen = screenList
		return m, nil
	}
}

// refreshList rebuilds the bubbles/list contents from the caller's
// RefreshItems callback. No-op when the callback wasn't supplied (the
// list will show stale data until the user exits — caller's choice).
func (m *model) refreshList() {
	if m.refreshItems == nil {
		return
	}
	items := m.refreshItems()
	listItems := make([]list.Item, 0, len(items)+1)
	for _, it := range items {
		listItems = append(listItems, it)
	}
	if !m.hideNewProject {
		listItems = append(listItems, newProjectItem{})
	}
	m.list.SetItems(listItems)
}

// showError transitions to screenError pre-loaded with msg. Returns the
// receiver so callers can `return m.showError(...), nil` in a single line.
func (m *model) showError(msg string) *model {
	m.errMsg = msg
	m.screen = screenError
	// Mirror the failure into the status bar so it persists after the
	// user dismisses the error screen — they shouldn't have to remember
	// what just failed.
	m.setStatusErr(msg)
	return m
}

// setStatusOK records a successful action's outcome for the status bar.
// Called from runIntent after a callback returns nil.
func (m *model) setStatusOK(msg string) {
	m.status = statusLine{text: msg, isErr: false}
}

// setStatusErr records a failed action's outcome for the status bar.
// Wraps showError's message in red.
func (m *model) setStatusErr(msg string) {
	m.status = statusLine{text: msg, isErr: true}
}

// cycleTheme advances to the next available theme (built-in + user)
// in alphabetical order, applies it live to every visible surface
// (list delegate, list title, form), and persists the choice to
// config.yaml. Failures fall into the status bar; the cycle still
// advances so the user can keep trying.
//
// No-ops gracefully when configPath is empty (read-only callers) or
// when theme.List returns nothing (impossible in production — the
// built-in default is always present — but guarded so a packaging
// bug doesn't crash the TUI).
func (m *model) cycleTheme() {
	names, err := theme.List(m.themesDir)
	if err != nil || len(names) == 0 {
		m.setStatusErr(fmt.Sprintf("themes: %v", err))
		return
	}

	// Find current position; default to -1 so an unknown current
	// name (theme deleted from disk between launches) advances to
	// names[0] rather than wrapping past the end.
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
		// theme.List returned a name that ThemeByName couldn't load.
		// Skip it and report — the user can keep cycling past it.
		m.setStatusErr(fmt.Sprintf("theme %q failed to load", next))
		return
	}

	m.applyTheme(next, t)

	if m.configPath == "" {
		// Session-only mode (no persistence wired). Still useful for
		// test harnesses; just don't claim we saved.
		m.setStatusOK(fmt.Sprintf("theme: %s", next))
		return
	}

	if err := config.SetUITheme(m.configPath, next); err != nil {
		m.setStatusErr(fmt.Sprintf("theme: %s (save failed: %v)", next, err))
		return
	}
	m.setStatusOK(fmt.Sprintf("theme: %s (saved)", next))
}

// applyTheme swaps the live theme on every surface that caches
// lipgloss styles. Centralised here so future theme-mutating paths
// (e.g. a `boo themes pick` modal) reuse the same propagation rules
// and don't leave half the UI rendering in the old palette.
func (m *model) applyTheme(name string, t Theme) {
	m.theme = t
	m.themeName = name

	// Rebuild the list delegate so row styles (selected title, dim
	// description, etc.) pick up the new palette.
	m.list.SetDelegate(newDelegate(t))
	m.list.Styles.Title = t.ListTitle

	// The form caches its own theme — propagate.
	m.form.theme = t
}

// updateError dismisses the error screen on any keypress and returns to
// the list. We deliberately accept any key (not just esc/enter) so a
// reflexive keypress doesn't accidentally trigger another action.
func (m *model) updateError(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := msg.(tea.KeyMsg); ok {
		m.errMsg = ""
		m.screen = screenList
	}
	return m, nil
}

// viewError renders the error screen — title + message + "any key" hint.
// Wrapped in a rounded border like the other modals.
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
		// Dispatch through runIntent so edit-style intents (EditIntent)
		// run in-loop and refresh the list, while quit-and-handoff
		// intents (NewProjectIntent) still exit the TUI for the CLI to
		// take over the terminal.
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
		case matches(m.keys.Quit, pressed):
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
	case screenError:
		raw = m.viewError()
	default:
		raw = m.viewList()
	}
	// Hard cap to the terminal viewport. clipToHeight + per-pane sizing
	// give the right answer in the common case, but lipgloss's
	// auto-wrap can sneak extra rows into bordered content (e.g. a
	// preview line that visually equals the inner width but lipgloss
	// wraps anyway because of padding accounting). MaxHeight here is
	// the last line of defence — without it, alt-screen scrolls the
	// top off. MaxWidth covers the symmetric horizontal case.
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
		// Strip + 1-row gap + list. The "\n" between strip and the
		// list's first line provides the visual separator; the
		// strip itself is 3 rows so this yields 3+1=4 rows above
		// the list, matching brandStripReserved.
		listView = m.theme.RightPaneFaint.Render(brandStrip) + "\n\n" + listView
	}
	// lipgloss .Width(N) treats N as "frame minus border" — padding
	// is subtracted from N to get the content area. innerListWidth is
	// the content width we want (and what we passed to list.SetSize),
	// so add back the 2 cols of horizontal padding here.
	listBoxed := m.theme.ListPaneBorder.
		Width(innerListWidth + 2).
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

// viewStatusBar renders the bottom status line. Shows the most recent
// action's outcome (success in green, failure in red) or a faint idle
// hint when nothing has happened yet. The output is hard-truncated to
// m.width so a long error message can never wrap onto a second line —
// a wrap would steal a row from the panes above and invalidate the
// statusBarHeight=1 layout math.
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

// truncateToWidth returns s clipped to at most width visible runes.
// width<=0 returns s unchanged (the WindowSizeMsg hasn't been received
// yet — the caller has bigger problems than wrapping). We use rune
// count rather than a wcwidth-aware measure because the strings we
// truncate are short status messages without wide glyphs; if that
// changes, swap in lipgloss.Width / runewidth.
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
	// Replace the last visible rune with an ellipsis to signal
	// truncation rather than silently chopping.
	return string(runes[:width-1]) + "…"
}

// viewRightPane renders the context-sensitive right-hand pane. Content
// depends on what's currently highlighted in the list:
//
//   - An Item: project detail (delegates to opts.PreviewProject if wired,
//     otherwise renders a minimal name/dir summary from the Item itself).
//   - newProjectItem: a hint about the form keystroke.
//   - Nothing selected (empty list): a placeholder.
//
// The pane is wrapped in a rounded border styled by the theme. Content
// width is rightPaneWidth - 4 (border + padding). Content is also
// hard-clipped to innerHeight via clipToHeight before being handed to
// lipgloss — splitMinHeight is only a heuristic floor, and at exactly
// 24 rows a layout preview with deep column splits could still overflow
// without this cap.
func (m *model) viewRightPane() string {
	// rightPaneWidth is the screen footprint (border 2 + padding 2 +
	// content). lipgloss .Width(N) renders a box that is N + border
	// wide, AND .Width consumes the padding — i.e. content area is
	// N - horizontal padding. So pass rightPaneWidth - 2 (for the
	// border) and the resulting content area = rightPaneWidth - 4
	// = innerWidth, which content code sizes to.
	const innerWidth = rightPaneWidth - 4
	innerHeight := m.usableInnerHeight()
	// Right pane gets +1 height to match the list pane's apparent
	// rendered height. The list pane's content (brand strip + bubbles
	// list) ends up 1 visual row taller than its declared Height in
	// the actual terminal — likely a lipgloss padding/wrap interaction
	// that doesn't reproduce in unit tests but is visible at runtime.
	// Without this, the right pane's bottom border sits 1 row above
	// the list pane's, which looks broken.
	border := m.theme.RightPaneBorder.Width(rightPaneWidth - 2).Height(innerHeight + 1)

	switch v := m.list.SelectedItem().(type) {
	case Item:
		return border.Render(clipToHeight(m.renderItemDetail(v, innerWidth), innerHeight))
	case newProjectItem:
		// If there are no real projects yet, lean into the empty
		// state with the brand mascot. Once the user has at least
		// one project, drop back to a plain prompt — the mascot
		// would just be visual noise.
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

// clipToHeight returns s truncated to at most height lines. If s has
// more lines, the last surviving line is replaced with an ellipsis to
// signal there's more content. Returning s unchanged when height<=0
// keeps the function safe to call before the WindowSizeMsg arrives.
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
// Falls back to the Item's own fields if no PreviewProject callback was
// wired (e.g. selection-only callers like `boo delete`).
func (m *model) renderItemDetail(it Item, _ int) string {
	if m.previewProject != nil {
		if rendered := m.previewProject(it.Key); rendered != "" {
			return rendered
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
	// Derive the footer text from the keymap so it can never drift
	// from what updateAlreadyRegistered actually accepts.
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

// Height is 1: each project (and the synthetic "+ New project" row)
// occupies a single line. The previous 2-line layout duplicated the
// project's directory between the list and the right pane and forced
// a wide splitThreshold to keep both panes visible. The right pane
// already shows the directory and other metadata in detail.
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

	// Single-line layout: cursor + title + status pill + optional
	// trailing (last-launched). Path lives in the right pane only.
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
