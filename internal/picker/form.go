package picker

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// FormDefaults is what the caller passes in to pre-populate the form.
// All fields optional. AlreadyRegisteredAs, if non-empty, causes the model
// to render a "this dir is already registered" prompt before the form.
type FormDefaults struct {
	Name                string
	Dir                 string
	From                string
	Template            string
	GitRemote           string // informational only; rendered above the form
	AlreadyRegisteredAs string
	// DefaultLayout is the layout name used when Template is empty.
	// Empty means "use the package's hardcoded fallback ('triple')".
	// Wired by the CLI from user config so a user who sets
	// default_layout in config.yaml sees that layout preselected.
	DefaultLayout string
}

// formField is an index into the form's tab order.
type formField int

const (
	fieldName formField = iota
	fieldDir
	fieldFrom
	fieldTemplate
	numFormFields
)

func (f formField) label() string {
	switch f {
	case fieldName:
		return "Name"
	case fieldDir:
		return "Directory"
	case fieldFrom:
		return "Clone from URL"
	case fieldTemplate:
		return "Layout template"
	}
	return ""
}

// formModel is the new-project sub-screen. It owns four text inputs plus
// a focus index.
//
// The Layout template field has two modes:
//   - Free-text input (default, for tests and any caller that doesn't
//     supply Options.LayoutNames). The textinput at inputs[fieldTemplate]
//     is the source of truth.
//   - Cycler over a known list (when layoutNames is non-empty). The
//     textinput at inputs[fieldTemplate] is bypassed entirely; layoutIdx
//     points into layoutNames. ←/→ (and h/l) cycle when the field is
//     focused. The textinput's value is kept in sync as a fallback so
//     collect() can read it the same way in both modes.
//
// Edit mode (set via setEditMode) re-purposes the same widget for
// editing an existing project. Differences:
//   - Title says "Edit project" instead of "Register a new project".
//   - The "Clone from URL" field is hidden — you can't change a
//     registered project's clone source after the fact, and showing
//     an empty unrelated field would be confusing. focusNext/focusPrev
//     skip it; view() doesn't render it.
//   - Submit produces an EditIntent (with the original name carried
//     through editOldName) instead of a NewProjectIntent.
type formModel struct {
	inputs    []textinput.Model
	focus     formField
	gitRemote string
	width     int
	err       string
	theme     Theme
	// preview, if set, is called with the current value of the Layout
	// template field; the returned string is shown below the form. See
	// Options.PreviewTemplate for the rationale (kept as a callback so
	// the picker package doesn't import internal/layout).
	preview func(name string) string

	// layoutNames is the cycler's list of template names. Empty means
	// the Layout field stays a plain text input. See setLayoutNames.
	layoutNames []string
	layoutIdx   int

	// defaultLayout is the template name to use when the Layout field
	// is left blank at submit time and when the preview is requested
	// without an explicit name. Set from FormDefaults.DefaultLayout
	// (which the CLI populates from user config), falling back to
	// hardcodedFallbackLayout.
	defaultLayout string

	// editMode + editOldName turn this widget into an "edit existing
	// project" form. editOldName is the project's original key — the
	// EditIntent emitted on submit ships it as OldName so the CLI can
	// distinguish a rename from a no-op. Empty editOldName with
	// editMode true would be a programming error; we don't enforce it
	// here because setEditMode is the only entry point.
	editMode    bool
	editOldName string
}

// hardcodedFallbackLayout is the layout name the form uses when no
// other source (caller-supplied DefaultLayout or user-typed Template)
// has provided one. Last-resort fallback only — `boo` users who want
// to change the default should set `default_layout:` in config.yaml,
// which the CLI threads through FormDefaults.DefaultLayout.
const hardcodedFallbackLayout = "triple"

// effectiveDefault returns the layout name the form should treat as
// "the default" — the value that pre-fills the Template input and is
// used when the user submits with the field empty.
func (d FormDefaults) effectiveDefault() string {
	if d.DefaultLayout != "" {
		return d.DefaultLayout
	}
	return hardcodedFallbackLayout
}

func newFormModel(d FormDefaults, theme Theme) formModel {
	mk := func(placeholder, value string) textinput.Model {
		ti := textinput.New()
		ti.Placeholder = placeholder
		ti.Prompt = "  "
		ti.SetValue(value)
		ti.CharLimit = 1024
		return ti
	}
	def := d.effectiveDefault()
	inputs := make([]textinput.Model, numFormFields)
	inputs[fieldName] = mk("project-name", d.Name)
	inputs[fieldDir] = mk("/path/to/dir", d.Dir)
	inputs[fieldFrom] = mk("https://github.com/owner/repo (optional)", d.From)
	tpl := d.Template
	if tpl == "" {
		tpl = def
	}
	inputs[fieldTemplate] = mk(def, tpl)

	inputs[fieldName].Focus()
	return formModel{
		inputs:        inputs,
		focus:         fieldName,
		gitRemote:     d.GitRemote,
		theme:         theme,
		defaultLayout: def,
	}
}

func (f *formModel) setSize(width int) {
	f.width = width
	w := width - 6
	if w < 20 {
		w = 20
	}
	for i := range f.inputs {
		f.inputs[i].Width = w
	}
}

// setLayoutNames switches the Layout template field into cycler mode
// over the supplied list. The currently-typed value (from defaults or
// fallback text input) is preserved if it matches one of the names;
// otherwise the cycler starts at index 0 and the input value is
// updated to match.
//
// Calling with an empty / nil slice puts the field back into free-text
// mode.
func (f *formModel) setLayoutNames(names []string) {
	if len(names) == 0 {
		f.layoutNames = nil
		f.layoutIdx = 0
		return
	}
	f.layoutNames = names
	current := strings.TrimSpace(f.inputs[fieldTemplate].Value())
	f.layoutIdx = 0
	for i, n := range names {
		if n == current {
			f.layoutIdx = i
			break
		}
	}
	// Keep the underlying textinput in sync so collect() reads the
	// same value the cycler is showing — single source of truth at
	// submit time.
	f.inputs[fieldTemplate].SetValue(names[f.layoutIdx])
}

// cyclerActive reports whether the Layout field is currently in
// cycler mode (i.e. setLayoutNames was called with a non-empty list).
func (f *formModel) cyclerActive() bool { return len(f.layoutNames) > 0 }

// cycleLayout moves the cycler by delta (-1 or +1), wraps around, and
// updates the underlying textinput value.
func (f *formModel) cycleLayout(delta int) {
	if !f.cyclerActive() {
		return
	}
	n := len(f.layoutNames)
	f.layoutIdx = (f.layoutIdx + delta + n) % n
	f.inputs[fieldTemplate].SetValue(f.layoutNames[f.layoutIdx])
}

// setEditMode flips the form into "edit existing project" mode. The
// caller passes the project's current key — preserved so the emitted
// EditIntent ships OldName even if the user changed Name in the form.
//
// Pre-populating the input values themselves is the caller's job (via
// FormDefaults at construction time); this method only sets the flags
// that change form behaviour. Calling with editMode=false reverts to
// new-project semantics.
func (f *formModel) setEditMode(editMode bool, oldName string) {
	f.editMode = editMode
	f.editOldName = oldName
}

// fieldHidden reports whether a field should be skipped during focus
// navigation and rendering. Used to hide From in edit mode.
func (f *formModel) fieldHidden(field formField) bool {
	return f.editMode && field == fieldFrom
}

// Update returns (next model, cmd, submitted, intent, cancelled).
//   - submitted=true: user pressed enter on the last field with valid content.
//   - cancelled=true: user pressed esc.
//   - both false: still editing.
//
// `intent` is nil unless submitted=true. Its concrete type is
// *NewProjectIntent for new-project mode and *EditIntent for edit mode.
// Callers type-switch.
func (f *formModel) update(msg tea.Msg) (tea.Cmd, bool, Intent, bool) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			return nil, false, nil, true
		case "tab", "down":
			f.focusNext()
			return nil, false, nil, false
		case "shift+tab", "up":
			f.focusPrev()
			return nil, false, nil, false
		case "ctrl+s":
			return f.tryCommit()
		case "enter":
			// On the last field, enter submits. Earlier fields advance focus.
			if f.focus == f.lastVisibleField() {
				return f.tryCommit()
			}
			f.focusNext()
			return nil, false, nil, false
		case "left", "h":
			// In cycler mode, ←/h cycles backwards through the layout
			// list when the Layout field is focused. Outside cycler
			// mode (or on other fields) we fall through and let the
			// textinput handle the key — '←' moves the cursor inside
			// the input, 'h' inserts the literal character.
			if f.focus == fieldTemplate && f.cyclerActive() {
				f.cycleLayout(-1)
				return nil, false, nil, false
			}
		case "right", "l":
			if f.focus == fieldTemplate && f.cyclerActive() {
				f.cycleLayout(+1)
				return nil, false, nil, false
			}
		}
	}

	// In cycler mode, the Layout field's textinput is fully bypassed —
	// we don't want characters typed into it to silently override the
	// cycler's value. All other fields (and the Layout field in
	// fallback free-text mode) keep the normal textinput behaviour.
	if f.focus == fieldTemplate && f.cyclerActive() {
		return nil, false, nil, false
	}

	var cmd tea.Cmd
	f.inputs[f.focus], cmd = f.inputs[f.focus].Update(msg)
	return cmd, false, nil, false
}

func (f *formModel) tryCommit() (tea.Cmd, bool, Intent, bool) {
	if f.editMode {
		intent, err := f.collectEdit()
		if err != nil {
			f.err = err.Error()
			return nil, false, nil, false
		}
		// Return the value, not the pointer: the Intent interface is
		// implemented by the value type (see isIntent receivers in
		// picker.go), so a *EditIntent would not satisfy it.
		return nil, true, *intent, false
	}
	intent, err := f.collect()
	if err != nil {
		f.err = err.Error()
		return nil, false, nil, false
	}
	return nil, true, *intent, false
}

// collect builds a NewProjectIntent from the current input values.
// Used in new-project mode.
func (f *formModel) collect() (*NewProjectIntent, error) {
	name := strings.TrimSpace(f.inputs[fieldName].Value())
	dir := strings.TrimSpace(f.inputs[fieldDir].Value())
	from := strings.TrimSpace(f.inputs[fieldFrom].Value())
	tpl := strings.TrimSpace(f.inputs[fieldTemplate].Value())
	if tpl == "" {
		tpl = f.defaultLayout
		if tpl == "" {
			tpl = hardcodedFallbackLayout
		}
	}
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if dir == "" && from == "" {
		return nil, fmt.Errorf("either Directory or Clone from URL is required")
	}
	return &NewProjectIntent{
		Name:     name,
		Dir:      dir,
		From:     from,
		Template: tpl,
	}, nil
}

// collectEdit builds an EditIntent from the current input values.
// Used in edit mode. From is intentionally ignored (it's hidden) — you
// can't change a registered project's clone source.
//
// Validation is intentionally lighter than collect(): an empty Name is
// rejected (you'd lose the project), but an unchanged Dir/Template is
// fine — the CLI side is the one that decides whether anything actually
// needs writing. Keeps the form unopinionated.
func (f *formModel) collectEdit() (*EditIntent, error) {
	name := strings.TrimSpace(f.inputs[fieldName].Value())
	dir := strings.TrimSpace(f.inputs[fieldDir].Value())
	tpl := strings.TrimSpace(f.inputs[fieldTemplate].Value())
	if tpl == "" {
		tpl = f.defaultLayout
		if tpl == "" {
			tpl = hardcodedFallbackLayout
		}
	}
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if dir == "" {
		return nil, fmt.Errorf("directory is required")
	}
	return &EditIntent{
		OldName:     f.editOldName,
		NewName:     name,
		NewDir:      dir,
		NewTemplate: tpl,
	}, nil
}

// focusNext / focusPrev wrap around the visible fields, skipping any
// that fieldHidden() reports — keeps tab/shift-tab/enter navigation
// from "landing" on an invisible field in edit mode.
func (f *formModel) focusNext() {
	f.inputs[f.focus].Blur()
	for i := 0; i < int(numFormFields); i++ {
		f.focus = (f.focus + 1) % numFormFields
		if !f.fieldHidden(f.focus) {
			break
		}
	}
	f.inputs[f.focus].Focus()
}

func (f *formModel) focusPrev() {
	f.inputs[f.focus].Blur()
	for i := 0; i < int(numFormFields); i++ {
		f.focus = (f.focus - 1 + numFormFields) % numFormFields
		if !f.fieldHidden(f.focus) {
			break
		}
	}
	f.inputs[f.focus].Focus()
}

// lastVisibleField returns the highest field index that fieldHidden
// reports as visible. Used so enter on the last *visible* field
// commits, even when later fields are hidden in edit mode.
func (f *formModel) lastVisibleField() formField {
	for i := int(numFormFields) - 1; i >= 0; i-- {
		ff := formField(i)
		if !f.fieldHidden(ff) {
			return ff
		}
	}
	return fieldName // unreachable; nothing is hidden by default
}

// renderCycler draws the Layout field as `‹  <name>  ›` when the
// cycler is active. The arrows are styled subtly when the field is
// not focused so the affordance is unambiguous when the user lands
// on it.
//
// Width follows the same rule as the other inputs (width-6 with a
// 20-column floor), so the cycler row lines up vertically with the
// text inputs above it.
func (f *formModel) renderCycler() string {
	name := ""
	if f.layoutIdx < len(f.layoutNames) {
		name = f.layoutNames[f.layoutIdx]
	}
	left, right := "‹", "›"
	leftStyle, rightStyle := f.theme.FormCyclerArrow, f.theme.FormCyclerArrow
	if f.focus == fieldTemplate {
		leftStyle = f.theme.FormCyclerArrowFocus
		rightStyle = f.theme.FormCyclerArrowFocus
	}
	return "  " + leftStyle.Render(left) + "  " + name + "  " + rightStyle.Render(right)
}

// view renders the form. Caller decides where to place it.
func (f *formModel) view() string {
	var b strings.Builder
	title := "Register a new project"
	if f.editMode {
		title = "Edit project"
		if f.editOldName != "" {
			title += " — " + f.editOldName
		}
	}
	b.WriteString(f.theme.FormTitle.Render(title))
	b.WriteString("\n\n")

	if f.gitRemote != "" && !f.editMode {
		b.WriteString(f.theme.FormInfo.Render("Detected git remote: " + f.gitRemote))
		b.WriteString("\n\n")
	}

	for i := range f.inputs {
		if f.fieldHidden(formField(i)) {
			continue
		}
		label := formField(i).label()
		if formField(i) == f.focus {
			label = f.theme.FormLabelFocus.Render("› " + label)
		} else {
			label = f.theme.FormLabel.Render("  " + label)
		}
		b.WriteString(label)
		b.WriteString("\n")
		// Layout field gets the cycler view when active — every other
		// field, and the layout field in fallback mode, uses the
		// underlying textinput.
		if formField(i) == fieldTemplate && f.cyclerActive() {
			b.WriteString(f.renderCycler())
		} else {
			b.WriteString(f.inputs[i].View())
		}
		b.WriteString("\n\n")
	}

	if f.err != "" {
		b.WriteString(f.theme.FormErr.Render("✖ " + f.err))
		b.WriteString("\n\n")
	}

	help := "tab/↓ next · shift-tab/↑ prev · enter on last field submits · ctrl-s submits · esc cancels"
	if f.cyclerActive() && f.focus == fieldTemplate {
		help = "←/→ cycle layouts · " + help
	}
	b.WriteString(f.theme.FormHelp.Render(help))

	// Layout preview, if a renderer was wired in. We render below the
	// form (option B from the design discussion) rather than side-by-side
	// because (a) it scales gracefully at narrow widths and (b) it keeps
	// the form layout stable as the preview height changes — the user's
	// focus and inputs don't shift around as they type a template name.
	if f.preview != nil {
		tpl := strings.TrimSpace(f.inputs[fieldTemplate].Value())
		if tpl == "" {
			tpl = f.defaultLayout
			if tpl == "" {
				tpl = hardcodedFallbackLayout
			}
		}
		if rendered := f.preview(tpl); rendered != "" {
			b.WriteString("\n\n")
			b.WriteString(f.theme.FormLabel.Render("  Preview of layout \"" + tpl + "\""))
			b.WriteString("\n")
			// Indent each line to align with the form's input column.
			for _, line := range strings.Split(rendered, "\n") {
				b.WriteString("  ")
				b.WriteString(line)
				b.WriteString("\n")
			}
		}
	}
	return b.String()
}
