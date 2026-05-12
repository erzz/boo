package picker

import (
	"strings"
	"testing"

	"github.com/erzz/boo/internal/layout"
)

// fixedLayout returns a layout with a known shape and recognisable leaf cwds
// so tests can assert against specific leaves without index magic.
//
// Shape: row(left, column(topright, bottomright)) — three leaves total in DFS
// order: "left", "topright", "bottomright". Same as Default() but with cwds
// set to leaf names so we can spot-check which one the editor mutated.
func fixedLayout() *layout.Layout {
	return &layout.Layout{
		Name: "triple",
		Tabs: []layout.Tab{{
			Name: "main",
			Root: layout.Split{
				Direction: layout.DirRow,
				Children: []layout.Split{
					{Cwd: "left"},
					{
						Direction: layout.DirColumn,
						Children: []layout.Split{
							{Cwd: "topright"},
							{Cwd: "bottomright"},
						},
					},
				},
			},
		}},
	}
}

// editorModelWithResolver builds a model wired so form-submit transitions to
// the layout editor. Returns the model and a "captured" pointer that the
// resolver populates so tests can inspect what the resolver received.
func editorModelWithResolver(t *testing.T, lay *layout.Layout) (*model, *[]string) {
	t.Helper()
	m := sizedModel(120, Item{Key: "alpha", Title: "alpha", Description: "/tmp/a"})
	// sizedModel/newTestModel skip form construction; tests that drive the
	// form need a real one. defaults stay empty — each test sets fields.
	m.form = newFormModel(FormDefaults{}, m.theme)
	m.layoutNames = []string{"triple"}
	calls := []string{}
	m.resolveLayout = func(template string) (*layout.Layout, error) {
		calls = append(calls, template)
		return lay, nil
	}
	return m, &calls
}

// TestLayoutEditor_OpensWithResolvedLayoutOnFormSubmit:
// when ResolveLayout is wired and template is non-empty, form submit lands
// on the editor sub-screen instead of quitting with the intent.
func TestLayoutEditor_OpensWithResolvedLayoutOnFormSubmit(t *testing.T) {
	lay := fixedLayout()
	m, calls := editorModelWithResolver(t, lay)
	openLayoutEditorViaForm(t, m, "myproj", "/tmp/myproj", "triple")

	if m.screen != screenLayoutEditor {
		t.Fatalf("screen = %v, want screenLayoutEditor", m.screen)
	}
	if m.pendingNewProject == nil || m.pendingNewProject.Name != "myproj" {
		t.Errorf("pendingNewProject = %#v, want stashed intent", m.pendingNewProject)
	}
	if len(*calls) != 1 || (*calls)[0] != "triple" {
		t.Errorf("resolver calls = %v, want [triple]", *calls)
	}
	if got := len(m.layoutEditor.leaves); got != 3 {
		t.Errorf("leaves = %d, want 3 (one per cwd in fixedLayout)", got)
	}
}

// TestLayoutEditor_NoResolverFallsThroughToQuit:
// without ResolveLayout, form submit dispatches NewProjectIntent immediately
// (the editor is opt-in via the callback).
func TestLayoutEditor_NoResolverFallsThroughToQuit(t *testing.T) {
	m := sizedModel(120, Item{Key: "alpha", Title: "alpha", Description: "/tmp/a"})
	m.form = newFormModel(FormDefaults{}, m.theme)
	m.layoutNames = []string{"triple"}
	// resolveLayout deliberately nil.
	openLayoutEditorViaForm(t, m, "myproj", "/tmp/myproj", "triple")

	if m.screen == screenLayoutEditor {
		t.Errorf("screen unexpectedly screenLayoutEditor without resolver")
	}
	intent, ok := m.intent.(NewProjectIntent)
	if !ok {
		t.Fatalf("intent = %#v, want NewProjectIntent", m.intent)
	}
	if intent.MaterialisedLayout != nil {
		t.Errorf("MaterialisedLayout = %v, want nil (no editor was reached)", intent.MaterialisedLayout)
	}
}

// TestLayoutEditor_BlankTemplateStillOpensEditorViaFormDefault:
// the form's collect() substitutes the default layout for blank Template
// (form responsibility, not the picker's), so even submitting with blank
// Template lands in the editor — that's the right place to customise a
// "I don't care, give me the default" project.
func TestLayoutEditor_BlankTemplateStillOpensEditorViaFormDefault(t *testing.T) {
	lay := fixedLayout()
	m, calls := editorModelWithResolver(t, lay)
	openLayoutEditorViaForm(t, m, "myproj", "/tmp/myproj", "")

	if m.screen != screenLayoutEditor {
		t.Errorf("screen = %v, want screenLayoutEditor (form supplies default)", m.screen)
	}
	if len(*calls) != 1 {
		t.Errorf("resolver calls = %v, want 1 call (form's default template)", *calls)
	}
}

// TestLayoutEditor_CycleWalksDFSWithWraparound:
// tab cycles through leaves in DFS order; shift+tab goes backwards; both wrap.
func TestLayoutEditor_CycleWalksDFSWithWraparound(t *testing.T) {
	lay := fixedLayout()
	m, _ := editorModelWithResolver(t, lay)
	openLayoutEditorViaForm(t, m, "myproj", "/tmp/myproj", "triple")

	// Confirm starting position.
	if got := m.layoutEditor.leaves[m.layoutEditor.currentIdx].split.Cwd; got != "left" {
		t.Fatalf("initial leaf cwd = %q, want left", got)
	}

	m = pressKey(t, m, "tab")
	if got := m.layoutEditor.leaves[m.layoutEditor.currentIdx].split.Cwd; got != "topright" {
		t.Errorf("after tab: leaf cwd = %q, want topright", got)
	}
	m = pressKey(t, m, "tab")
	if got := m.layoutEditor.leaves[m.layoutEditor.currentIdx].split.Cwd; got != "bottomright" {
		t.Errorf("after 2×tab: leaf cwd = %q, want bottomright", got)
	}
	m = pressKey(t, m, "tab") // wrap
	if got := m.layoutEditor.leaves[m.layoutEditor.currentIdx].split.Cwd; got != "left" {
		t.Errorf("after 3×tab: leaf cwd = %q, want left (wrap)", got)
	}
	m = pressKey(t, m, "shift+tab") // wrap backwards
	if got := m.layoutEditor.leaves[m.layoutEditor.currentIdx].split.Cwd; got != "bottomright" {
		t.Errorf("after shift+tab: leaf cwd = %q, want bottomright (wrap)", got)
	}
}

// TestLayoutEditor_ApplyAttachesMaterialisedLayout:
// type → ctrl+s; intent's MaterialisedLayout reflects the typed command on the
// active leaf and the picker quits with that intent.
func TestLayoutEditor_ApplyAttachesMaterialisedLayout(t *testing.T) {
	lay := fixedLayout()
	m, _ := editorModelWithResolver(t, lay)
	openLayoutEditorViaForm(t, m, "myproj", "/tmp/myproj", "triple")

	// Type "htop" into the first leaf's command field.
	for _, r := range "htop" {
		m = pressKey(t, m, string(r))
	}
	m = pressKey(t, m, "ctrl+s")

	intent, ok := m.intent.(NewProjectIntent)
	if !ok {
		t.Fatalf("intent = %#v, want NewProjectIntent", m.intent)
	}
	if intent.MaterialisedLayout == nil {
		t.Fatal("MaterialisedLayout = nil, want non-nil after apply")
	}
	leaves := layout.LeafPointers(&intent.MaterialisedLayout.Tabs[0].Root)
	if got := leaves[0].Command; got != "htop" {
		t.Errorf("leaf 0 command = %q, want htop", got)
	}
	// Verify other leaves untouched.
	if got := leaves[1].Command; got != "" {
		t.Errorf("leaf 1 command = %q, want empty (untouched)", got)
	}
	if m.pendingNewProject != nil {
		// Pending should be cleared after apply.
		t.Errorf("pendingNewProject = %#v, want nil after apply", m.pendingNewProject)
	}
}

// TestLayoutEditor_ApplyPersistsEditsAcrossLeafSwitches:
// edits made before cycling should survive the cycle and end up in the final
// materialised layout.
func TestLayoutEditor_ApplyPersistsEditsAcrossLeafSwitches(t *testing.T) {
	lay := fixedLayout()
	m, _ := editorModelWithResolver(t, lay)
	openLayoutEditorViaForm(t, m, "myproj", "/tmp/myproj", "triple")

	for _, r := range "vim" {
		m = pressKey(t, m, string(r))
	}
	m = pressKey(t, m, "tab") // move to leaf 1, persisting "vim" on leaf 0
	for _, r := range "htop" {
		m = pressKey(t, m, string(r))
	}
	m = pressKey(t, m, "ctrl+s")

	intent := m.intent.(NewProjectIntent)
	leaves := layout.LeafPointers(&intent.MaterialisedLayout.Tabs[0].Root)
	if leaves[0].Command != "vim" || leaves[1].Command != "htop" {
		t.Errorf("commands = [%q, %q, %q], want [vim, htop, \"\"]",
			leaves[0].Command, leaves[1].Command, leaves[2].Command)
	}
}

// TestLayoutEditor_EscReturnsToFormAndDiscardsEdits:
// type → esc; the editor goes away, screen returns to the form, and no
// intent is dispatched. The user's keystrokes are intentionally discarded —
// esc means "back, let me amend my submission", not "create with no edits".
// (Blank-Command-means-shell is the natural default; users wanting an
// unmodified template just hit ctrl+s without typing.)
func TestLayoutEditor_EscReturnsToFormAndDiscardsEdits(t *testing.T) {
	lay := fixedLayout()
	m, _ := editorModelWithResolver(t, lay)
	openLayoutEditorViaForm(t, m, "myproj", "/tmp/myproj", "triple")

	for _, r := range "htop" {
		m = pressKey(t, m, string(r))
	}
	m = pressKey(t, m, "esc")

	if m.screen != screenForm {
		t.Errorf("screen = %v, want screenForm (esc must return to the form)", m.screen)
	}
	if m.intent != nil {
		t.Errorf("intent = %#v, want nil (esc must NOT dispatch)", m.intent)
	}
	if m.pendingNewProject != nil {
		t.Errorf("pendingNewProject = %#v, want nil (must be cleared on back)", m.pendingNewProject)
	}
}

// TestLayoutEditor_ResolverErrorSkipsEditor:
// resolver returns an error → editor is skipped, intent dispatched as if the
// editor weren't wired. Loud-failure here would block valid project creation
// over a nice-to-have UX path.
func TestLayoutEditor_ResolverErrorSkipsEditor(t *testing.T) {
	m := sizedModel(120, Item{Key: "alpha", Title: "alpha", Description: "/tmp/a"})
	m.form = newFormModel(FormDefaults{}, m.theme)
	m.layoutNames = []string{"triple"}
	m.resolveLayout = func(template string) (*layout.Layout, error) {
		return nil, errFake
	}
	openLayoutEditorViaForm(t, m, "myproj", "/tmp/myproj", "triple")

	if m.screen == screenLayoutEditor {
		t.Errorf("screen unexpectedly screenLayoutEditor after resolver error")
	}
	intent, ok := m.intent.(NewProjectIntent)
	if !ok {
		t.Fatalf("intent = %#v, want NewProjectIntent (fall-through)", m.intent)
	}
	if intent.MaterialisedLayout != nil {
		t.Errorf("MaterialisedLayout = %v, want nil after resolver failure", intent.MaterialisedLayout)
	}
}

// TestLayoutEditor_ViewMentionsKeysAndProject sanity-checks that the rendered
// editor names the project, the template, and the four bindings — so future
// keymap drift fails loudly rather than producing a confusing footer.
func TestLayoutEditor_ViewMentionsKeysAndProject(t *testing.T) {
	lay := fixedLayout()
	m, _ := editorModelWithResolver(t, lay)
	openLayoutEditorViaForm(t, m, "myproj", "/tmp/myproj", "triple")

	v := m.View()
	for _, want := range []string{"myproj", "triple", "tab", "shift+tab", "ctrl+s", "esc"} {
		if !strings.Contains(v, want) {
			t.Errorf("editor view missing %q. View:\n%s", want, v)
		}
	}
}

// openLayoutEditorViaForm drives the form to submit a new-project intent with
// the given fields. Mirrors what a real user does (type into fields, tab to
// the next, enter on the last) so the test exercises the same submit path the
// production form does. Hides ResolveLayout/template-gating logic from each
// individual test.
func openLayoutEditorViaForm(t *testing.T, m *model, name, dir, template string) {
	t.Helper()
	m.screen = screenForm
	m.form.inputs[fieldName].SetValue(name)
	m.form.inputs[fieldDir].SetValue(dir)
	m.form.inputs[fieldTemplate].SetValue(template)
	// Drive submit via ctrl+s — works regardless of which field has focus and
	// avoids stepping through enter-advances-focus semantics.
	_ = pressKey(t, m, "ctrl+s")
}
