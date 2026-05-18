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

// TestLayoutEditor_ToggleModeFlipsAndPreservesCursors: ctrl+l flips between
// leaf and divider modes; each mode's cursor position is independent so
// flipping back and forth doesn't reset progress.
func TestLayoutEditor_ToggleModeFlipsAndPreservesCursors(t *testing.T) {
	lay := fixedLayout()
	m, _ := editorModelWithResolver(t, lay)
	openLayoutEditorViaForm(t, m, "myproj", "/tmp/myproj", "triple")

	// Leaf mode: advance to leaf 1.
	m = pressKey(t, m, "tab")
	if m.layoutEditor.mode != modeLeaf || m.layoutEditor.currentIdx != 1 {
		t.Fatalf("after tab: mode=%v currentIdx=%d, want modeLeaf, 1", m.layoutEditor.mode, m.layoutEditor.currentIdx)
	}

	// Toggle to divider mode.
	m = pressKey(t, m, "ctrl+l")
	if m.layoutEditor.mode != modeDivider {
		t.Fatalf("after ctrl+l: mode=%v, want modeDivider", m.layoutEditor.mode)
	}
	// Advance the interior cursor.
	m = pressKey(t, m, "tab")
	if m.layoutEditor.interiorIdx != 1 {
		t.Fatalf("after tab in divider mode: interiorIdx=%d, want 1", m.layoutEditor.interiorIdx)
	}

	// Toggle back to leaf — leaf cursor must still be at 1, divider cursor at 1.
	m = pressKey(t, m, "ctrl+l")
	if m.layoutEditor.mode != modeLeaf {
		t.Fatalf("after second ctrl+l: mode=%v, want modeLeaf", m.layoutEditor.mode)
	}
	if m.layoutEditor.currentIdx != 1 {
		t.Errorf("leaf cursor lost: currentIdx=%d, want 1", m.layoutEditor.currentIdx)
	}
	if m.layoutEditor.interiorIdx != 1 {
		t.Errorf("divider cursor lost across mode flip: interiorIdx=%d, want 1", m.layoutEditor.interiorIdx)
	}
}

// TestLayoutEditor_DividerCycleMatchesInteriorPointers: tab/shift+tab in
// divider mode walks layout.InteriorPointers in order, with wraparound. The
// fixed layout has 2 interiors (outer row + inner column).
func TestLayoutEditor_DividerCycleMatchesInteriorPointers(t *testing.T) {
	lay := fixedLayout()
	m, _ := editorModelWithResolver(t, lay)
	openLayoutEditorViaForm(t, m, "myproj", "/tmp/myproj", "triple")
	m = pressKey(t, m, "ctrl+l") // → divider mode

	if got := len(m.layoutEditor.interiors); got != 2 {
		t.Fatalf("interiors = %d, want 2", got)
	}

	// idx 0 → outer row.
	if dir := m.layoutEditor.interiors[0].split.Direction; dir != layout.DirRow {
		t.Errorf("interior[0].Direction = %q, want %q", dir, layout.DirRow)
	}
	// tab → idx 1, the inner column.
	m = pressKey(t, m, "tab")
	if m.layoutEditor.interiorIdx != 1 {
		t.Fatalf("interiorIdx = %d, want 1", m.layoutEditor.interiorIdx)
	}
	if dir := m.layoutEditor.interiors[1].split.Direction; dir != layout.DirColumn {
		t.Errorf("interior[1].Direction = %q, want %q", dir, layout.DirColumn)
	}
	// tab again → wraps to 0.
	m = pressKey(t, m, "tab")
	if m.layoutEditor.interiorIdx != 0 {
		t.Errorf("after wrap: interiorIdx = %d, want 0", m.layoutEditor.interiorIdx)
	}
	// shift+tab → wraps back to 1.
	m = pressKey(t, m, "shift+tab")
	if m.layoutEditor.interiorIdx != 1 {
		t.Errorf("after shift+tab wrap: interiorIdx = %d, want 1", m.layoutEditor.interiorIdx)
	}
}

// TestLayoutEditor_BumpSizePromotesZeroAndClamps: an unset Size (0 = "even")
// is promoted to 0.5 on first bump so the visible move feels natural; further
// bumps clamp to [sizeMin, sizeMax].
func TestLayoutEditor_BumpSizePromotesZeroAndClamps(t *testing.T) {
	lay := fixedLayout()
	m, _ := editorModelWithResolver(t, lay)
	openLayoutEditorViaForm(t, m, "myproj", "/tmp/myproj", "triple")
	m = pressKey(t, m, "ctrl+l") // → divider mode, on outer row

	// Initial: Size == 0.
	if m.layoutEditor.interiors[0].split.Size != 0 {
		t.Fatalf("seed Size = %v, want 0", m.layoutEditor.interiors[0].split.Size)
	}
	// One "+" → 0.5 + 0.05 = 0.55.
	m = pressKey(t, m, "+")
	if got := m.layoutEditor.interiors[0].split.Size; !approxEq(got, 0.55) {
		t.Errorf("after one +: Size = %v, want ~0.55", got)
	}
	// Many "+" — should clamp at sizeMax.
	for i := 0; i < 50; i++ {
		m = pressKey(t, m, "+")
	}
	if got := m.layoutEditor.interiors[0].split.Size; !approxEq(got, sizeMax) {
		t.Errorf("after many +: Size = %v, want %v (clamped)", got, sizeMax)
	}
	// Many "-" — should clamp at sizeMin.
	for i := 0; i < 50; i++ {
		m = pressKey(t, m, "-")
	}
	if got := m.layoutEditor.interiors[0].split.Size; !approxEq(got, sizeMin) {
		t.Errorf("after many -: Size = %v, want %v (clamped)", got, sizeMin)
	}
}

// TestLayoutEditor_ResetSizeRestoresEvenSplit: the "0" key clears Size back to
// 0 (which the renderer + JXA walker treat as "split evenly").
func TestLayoutEditor_ResetSizeRestoresEvenSplit(t *testing.T) {
	lay := fixedLayout()
	m, _ := editorModelWithResolver(t, lay)
	openLayoutEditorViaForm(t, m, "myproj", "/tmp/myproj", "triple")
	m = pressKey(t, m, "ctrl+l")
	m = pressKey(t, m, "+")
	m = pressKey(t, m, "+")
	if m.layoutEditor.interiors[0].split.Size == 0 {
		t.Fatalf("precondition failed: Size still 0 after two +")
	}
	m = pressKey(t, m, "0")
	if got := m.layoutEditor.interiors[0].split.Size; got != 0 {
		t.Errorf("after 0 (reset): Size = %v, want 0", got)
	}
}

// TestLayoutEditor_DividerKeysAreInertInLeafMode: +/-/0 must NOT mutate Size
// when the editor is in leaf mode — they should fall through to the textinput
// as plain typed characters so users can put a "+" in a command name.
func TestLayoutEditor_DividerKeysAreInertInLeafMode(t *testing.T) {
	lay := fixedLayout()
	m, _ := editorModelWithResolver(t, lay)
	openLayoutEditorViaForm(t, m, "myproj", "/tmp/myproj", "triple")
	// Leaf mode by default. Press "+" — must land in the textinput, not Size.
	m = pressKey(t, m, "+")
	if got := m.layoutEditor.interiors[0].split.Size; got != 0 {
		t.Errorf("leaf-mode +: Size mutated to %v, want 0", got)
	}
	if got := m.layoutEditor.cmdInput.Value(); got != "+" {
		t.Errorf("leaf-mode +: textinput = %q, want %q", got, "+")
	}
}

// TestLayoutEditor_ApplyPersistsSize: a divider edit survives apply and lands
// on the materialised layout the CLI receives.
func TestLayoutEditor_ApplyPersistsSize(t *testing.T) {
	lay := fixedLayout()
	m, _ := editorModelWithResolver(t, lay)
	openLayoutEditorViaForm(t, m, "myproj", "/tmp/myproj", "triple")
	m = pressKey(t, m, "ctrl+l")
	m = pressKey(t, m, "+") // outer row → 0.55
	m = pressKey(t, m, "ctrl+s")

	intent, ok := m.intent.(NewProjectIntent)
	if !ok {
		t.Fatalf("intent = %#v, want NewProjectIntent", m.intent)
	}
	if intent.MaterialisedLayout == nil {
		t.Fatalf("MaterialisedLayout = nil, want non-nil")
	}
	interiors := layout.InteriorPointers(&intent.MaterialisedLayout.Tabs[0].Root)
	if len(interiors) == 0 {
		t.Fatalf("no interiors on persisted layout")
	}
	if got := interiors[0].Size; !approxEq(got, 0.55) {
		t.Errorf("persisted Size = %v, want ~0.55", got)
	}
}

// TestLayoutEditor_DividerEmptyStateForSingleLeafLayout: a tab with one leaf
// has zero interiors. The divider view shows an empty-state message; tab/+/-
// are no-ops.
func TestLayoutEditor_DividerEmptyStateForSingleLeafLayout(t *testing.T) {
	lay := &layout.Layout{
		Name: "solo",
		Tabs: []layout.Tab{{Name: "main", Root: layout.Split{Cwd: "."}}},
	}
	m, _ := editorModelWithResolver(t, lay)
	openLayoutEditorViaForm(t, m, "solo", "/tmp/solo", "triple")
	m = pressKey(t, m, "ctrl+l")

	if m.layoutEditor.mode != modeDivider {
		t.Fatalf("toggle did not enter divider mode: mode=%v", m.layoutEditor.mode)
	}
	if got := len(m.layoutEditor.interiors); got != 0 {
		t.Errorf("interiors = %d, want 0", got)
	}
	// View must mention the empty state — anchor on a stable substring.
	if !strings.Contains(m.View(), "no dividers") {
		t.Errorf("divider view missing empty-state hint. View:\n%s", m.View())
	}
	// Bumps must not panic and must not invent a phantom interior. Final
	// assignment is to the same model — ignore the result, the side-effect
	// (no panic, no state change) is what we're testing.
	m = pressKey(t, m, "+")
	m = pressKey(t, m, "-")
	_ = pressKey(t, m, "0")
}

// approxEq compares floats within a small epsilon — accumulated 0.05 steps
// drift slightly from "exact".
func approxEq(a, b float64) bool {
	const eps = 1e-9
	d := a - b
	if d < 0 {
		d = -d
	}
	return d < eps
}

// TestLayoutEditor_AllDividerKeysAreInertInLeafMode is the table-driven sibling
// of TestLayoutEditor_DividerKeysAreInertInLeafMode: every printable divider
// key falls through to the textinput in leaf mode and never mutates Size. This
// pins the "don't eat printable chars" contract for the full key set, not just
// "+".
func TestLayoutEditor_AllDividerKeysAreInertInLeafMode(t *testing.T) {
	cases := []string{"+", "=", "-", "_", "0"}
	for _, k := range cases {
		t.Run(k, func(t *testing.T) {
			lay := fixedLayout()
			m, _ := editorModelWithResolver(t, lay)
			openLayoutEditorViaForm(t, m, "myproj", "/tmp/myproj", "triple")
			// Leaf mode by default.
			m = pressKey(t, m, k)
			if got := m.layoutEditor.interiors[0].split.Size; got != 0 {
				t.Errorf("leaf-mode %q: Size mutated to %v, want 0", k, got)
			}
			if got := m.layoutEditor.cmdInput.Value(); got != k {
				t.Errorf("leaf-mode %q: textinput = %q, want %q", k, got, k)
			}
		})
	}
}

// TestLayoutEditor_ToggleModeFlushesInFlightCommand: typing into the textinput
// then toggling to divider mode (without an explicit save) must persist the
// typed text to the active leaf's Command. Otherwise users would silently lose
// edits when pressing ctrl+l mid-type. Verifies the invariant by then applying
// (ctrl+s) and checking the materialised layout.
func TestLayoutEditor_ToggleModeFlushesInFlightCommand(t *testing.T) {
	lay := fixedLayout()
	m, _ := editorModelWithResolver(t, lay)
	openLayoutEditorViaForm(t, m, "myproj", "/tmp/myproj", "triple")

	for _, r := range "vim" {
		m = pressKey(t, m, string(r))
	}
	// Toggle to divider mode without an explicit save.
	m = pressKey(t, m, "ctrl+l")
	// Dispatch — saveCurrent in apply is a no-op now (we're in divider mode),
	// so the only way "vim" survives is if toggleMode flushed it.
	m = pressKey(t, m, "ctrl+s")

	intent, ok := m.intent.(NewProjectIntent)
	if !ok || intent.MaterialisedLayout == nil {
		t.Fatalf("intent = %#v, want NewProjectIntent with MaterialisedLayout", m.intent)
	}
	leaves := layout.LeafPointers(&intent.MaterialisedLayout.Tabs[0].Root)
	if len(leaves) == 0 {
		t.Fatalf("no leaves on persisted layout")
	}
	if got := leaves[0].Command; got != "vim" {
		t.Errorf("first leaf Command = %q, want %q (toggle must flush in-flight typed text)", got, "vim")
	}
}
