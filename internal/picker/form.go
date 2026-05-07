package picker

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// NewProjectIntent is what the form returns when the user submits. The CLI
// layer turns this into the actual registry/clone work.
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
type formModel struct {
	inputs    []textinput.Model
	focus     formField
	gitRemote string
	width     int
	err       string
}

func newFormModel(d FormDefaults) formModel {
	mk := func(placeholder, value string) textinput.Model {
		ti := textinput.New()
		ti.Placeholder = placeholder
		ti.Prompt = "  "
		ti.SetValue(value)
		ti.CharLimit = 1024
		return ti
	}
	inputs := make([]textinput.Model, numFormFields)
	inputs[fieldName] = mk("project-name", d.Name)
	inputs[fieldDir] = mk("/path/to/dir", d.Dir)
	inputs[fieldFrom] = mk("https://github.com/owner/repo (optional)", d.From)
	tpl := d.Template
	if tpl == "" {
		tpl = "default"
	}
	inputs[fieldTemplate] = mk("default", tpl)

	inputs[fieldName].Focus()
	return formModel{
		inputs:    inputs,
		focus:     fieldName,
		gitRemote: d.GitRemote,
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

// Update returns (next model, cmd, submitted, intent, cancelled).
//   - submitted=true: user pressed enter on the last field with valid content.
//   - cancelled=true: user pressed esc.
//   - both false: still editing.
func (f *formModel) update(msg tea.Msg) (tea.Cmd, bool, *NewProjectIntent, bool) {
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
			if f.focus == numFormFields-1 {
				return f.tryCommit()
			}
			f.focusNext()
			return nil, false, nil, false
		}
	}

	var cmd tea.Cmd
	f.inputs[f.focus], cmd = f.inputs[f.focus].Update(msg)
	return cmd, false, nil, false
}

func (f *formModel) tryCommit() (tea.Cmd, bool, *NewProjectIntent, bool) {
	intent, err := f.collect()
	if err != nil {
		f.err = err.Error()
		return nil, false, nil, false
	}
	return nil, true, intent, false
}

func (f *formModel) collect() (*NewProjectIntent, error) {
	name := strings.TrimSpace(f.inputs[fieldName].Value())
	dir := strings.TrimSpace(f.inputs[fieldDir].Value())
	from := strings.TrimSpace(f.inputs[fieldFrom].Value())
	tpl := strings.TrimSpace(f.inputs[fieldTemplate].Value())
	if tpl == "" {
		tpl = "default"
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

func (f *formModel) focusNext() {
	f.inputs[f.focus].Blur()
	f.focus = (f.focus + 1) % numFormFields
	f.inputs[f.focus].Focus()
}

func (f *formModel) focusPrev() {
	f.inputs[f.focus].Blur()
	f.focus = (f.focus - 1 + numFormFields) % numFormFields
	f.inputs[f.focus].Focus()
}

// view renders the form. Caller decides where to place it.
func (f *formModel) view() string {
	var b strings.Builder
	b.WriteString(formTitleStyle.Render("Register a new project"))
	b.WriteString("\n\n")

	if f.gitRemote != "" {
		b.WriteString(formInfoStyle.Render("Detected git remote: " + f.gitRemote))
		b.WriteString("\n\n")
	}

	for i := range f.inputs {
		label := formField(i).label()
		if formField(i) == f.focus {
			label = formLabelFocusStyle.Render("› " + label)
		} else {
			label = formLabelStyle.Render("  " + label)
		}
		b.WriteString(label)
		b.WriteString("\n")
		b.WriteString(f.inputs[i].View())
		b.WriteString("\n\n")
	}

	if f.err != "" {
		b.WriteString(formErrStyle.Render("✖ " + f.err))
		b.WriteString("\n\n")
	}

	b.WriteString(formHelpStyle.Render(
		"tab/↓ next · shift-tab/↑ prev · enter on last field submits · ctrl-s submits · esc cancels"))
	return b.String()
}

var (
	formTitleStyle      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("13"))
	formLabelStyle      = lipgloss.NewStyle().Faint(true)
	formLabelFocusStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("13"))
	formInfoStyle       = lipgloss.NewStyle().Faint(true).Italic(true)
	formErrStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
	formHelpStyle       = lipgloss.NewStyle().Faint(true)
)
