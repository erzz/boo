package picker

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// FormDefaults pre-populates the new-project form. All fields optional.
// AlreadyRegisteredAs triggers a "dir already registered" interstitial when non-empty.
type FormDefaults struct {
	Name                string
	Dir                 string
	From                string
	Template            string
	GitRemote           string // informational only; rendered above the form
	AlreadyRegisteredAs string
	// DefaultLayout is the layout name used when Template is empty.
	// Empty falls back to the package hardcoded default ("triple").
	// CLI threads user config here so the user's default_layout is pre-selected.
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

// formModel is the new-project sub-screen: four text inputs plus a focus index.
//
// Layout template field has two modes: free-text (default, for tests / callers without LayoutNames)
// and cycler over a known list (when layoutNames is non-empty; ←/→ cycle when focused).
//
// Edit mode (setEditMode) re-purposes the widget for editing an existing project:
// title changes, the Clone from URL field is hidden, and submit emits EditIntent (not NewProjectIntent).
type formModel struct {
	inputs    []textinput.Model
	focus     formField
	gitRemote string
	width     int
	err       string
	theme     Theme
	// preview, if set, is called with the current template name; returned string shown below the form.
	// Callback so the picker package doesn't import internal/layout.
	preview func(name string) string

	// layoutNames is the cycler's list. Empty keeps Layout as a plain text input.
	layoutNames []string
	layoutIdx   int

	// defaultLayout is the template used when Layout is blank at submit; set from FormDefaults.DefaultLayout.
	defaultLayout string

	// editMode + editOldName: edit-existing-project form. editOldName is the project's original key.
	editMode    bool
	editOldName string
}

// hardcodedFallbackLayout is the last-resort default layout when no caller-supplied or user-config default exists.
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

// setLayoutNames switches the Layout field into cycler mode over names, preserving the current value if present.
// Empty/nil names reverts to free-text mode.
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
	// Keep the underlying textinput in sync — single source of truth at submit time.
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

// setEditMode flips the form into edit-existing-project mode. Pre-populating inputs is the caller's job.
// oldName is carried through as EditIntent.OldName to distinguish renames from no-ops.
func (f *formModel) setEditMode(editMode bool, oldName string) {
	f.editMode = editMode
	f.editOldName = oldName
}

// fieldHidden reports whether a field should be skipped during focus
// navigation and rendering. Used to hide From in edit mode.
func (f *formModel) fieldHidden(field formField) bool {
	return f.editMode && field == fieldFrom
}

// Update returns (cmd, submitted, intent, cancelled). submitted=true means the user pressed enter/ctrl-s
// with valid content; intent is non-nil iff submitted. Concrete type: *NewProjectIntent or *EditIntent.
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
			// Cycler mode: ←/h cycles backwards. Outside cycler mode, fall through to textinput.
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

	// In cycler mode, the Layout field's textinput is fully bypassed — typed chars
	// would silently override the cycler's value.
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
		// Return value, not pointer — Intent interface is implemented by the value type.
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

// collectEdit builds an EditIntent from current inputs (edit mode). From is ignored (hidden).
// Validation is lighter than collect(): empty Dir is rejected, but unchanged Dir/Template is fine.
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

// focusNext / focusPrev wrap around visible fields, skipping fieldHidden() fields.
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

// lastVisibleField returns the highest visible field index, so enter on the last visible field commits.
func (f *formModel) lastVisibleField() formField {
	for i := int(numFormFields) - 1; i >= 0; i-- {
		ff := formField(i)
		if !f.fieldHidden(ff) {
			return ff
		}
	}
	return fieldName // unreachable; nothing is hidden by default
}

// renderCycler draws the Layout field as `‹  <name>  ›`. Arrows are styled based on focus.
// Width matches other inputs (width-6, floor 20) so the cycler row aligns vertically.
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
		// Layout field: cycler view when active; textinput otherwise.
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

	// Layout preview rendered below the form — scales at narrow widths; form layout stays stable.
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
