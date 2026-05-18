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
// Editor opens in LAYOUT mode by default, so this test enters COMMAND mode
// first (where tab cycles leaves rather than dividers).
func TestLayoutEditor_CycleWalksDFSWithWraparound(t *testing.T) {
	lay := fixedLayout()
	m, _ := editorModelWithResolver(t, lay)
	openLayoutEditorViaForm(t, m, "myproj", "/tmp/myproj", "triple")
	m = pressKey(t, m, "c") // enter COMMAND mode

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
// enter COMMAND mode → type → ctrl+s; intent's MaterialisedLayout reflects
// the typed command on the active leaf and the picker quits with that intent.
func TestLayoutEditor_ApplyAttachesMaterialisedLayout(t *testing.T) {
	lay := fixedLayout()
	m, _ := editorModelWithResolver(t, lay)
	openLayoutEditorViaForm(t, m, "myproj", "/tmp/myproj", "triple")

	// Enter command mode then type "htop" into the first leaf's command field.
	m = pressKey(t, m, "c")
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
// materialised layout. All in COMMAND mode (where tab cycles leaves).
func TestLayoutEditor_ApplyPersistsEditsAcrossLeafSwitches(t *testing.T) {
	lay := fixedLayout()
	m, _ := editorModelWithResolver(t, lay)
	openLayoutEditorViaForm(t, m, "myproj", "/tmp/myproj", "triple")
	m = pressKey(t, m, "c") // → COMMAND mode

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
// from LAYOUT mode (the default), esc backs out to the form. Per-leaf typed
// commands made during a previous COMMAND-mode session are discarded — esc
// means "back, let me amend my submission", not "create with whatever I
// already typed". (Blank-Command-means-shell is the natural default; users
// wanting an unmodified template just hit ctrl+s without typing.)
func TestLayoutEditor_EscReturnsToFormAndDiscardsEdits(t *testing.T) {
	lay := fixedLayout()
	m, _ := editorModelWithResolver(t, lay)
	openLayoutEditorViaForm(t, m, "myproj", "/tmp/myproj", "triple")

	// Type something into a leaf so we can confirm it's discarded on back.
	m = pressKey(t, m, "c")
	for _, r := range "htop" {
		m = pressKey(t, m, string(r))
	}
	m = pressKey(t, m, "enter") // commit, return to LAYOUT mode
	if m.layoutEditor.mode != modeDivider {
		t.Fatalf("after enter: mode=%v, want modeDivider (back in LAYOUT)", m.layoutEditor.mode)
	}
	// Now esc from LAYOUT mode must exit the editor.
	m = pressKey(t, m, "esc")

	if m.screen != screenForm {
		t.Errorf("screen = %v, want screenForm (esc in LAYOUT mode must return to the form)", m.screen)
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
// editor names the project, the template, the mode banner, and the keys that
// drive the LAYOUT-mode footer (the default on entry) — so future drift in
// the footer text or default mode fails loudly rather than producing a
// confusing first impression. "esc" is in the footer; "ctrl+s" is too.
func TestLayoutEditor_ViewMentionsKeysAndProject(t *testing.T) {
	lay := fixedLayout()
	m, _ := editorModelWithResolver(t, lay)
	openLayoutEditorViaForm(t, m, "myproj", "/tmp/myproj", "triple")

	v := m.View()
	for _, want := range []string{"myproj", "triple", "LAYOUT", "COMMANDS", "tab", "shift+tab", "ctrl+s", "esc", "[c]"} {
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

// TestLayoutEditor_ModeSwitchPreservesCursors: `c` enters COMMAND mode, `esc`
// (or enter) returns to LAYOUT mode; each mode's cursor position is
// independent so switching back and forth doesn't reset progress.
func TestLayoutEditor_ModeSwitchPreservesCursors(t *testing.T) {
	lay := fixedLayout()
	m, _ := editorModelWithResolver(t, lay)
	openLayoutEditorViaForm(t, m, "myproj", "/tmp/myproj", "triple")

	// Default: LAYOUT mode. Advance the divider cursor to interior 1.
	if m.layoutEditor.mode != modeDivider {
		t.Fatalf("default mode = %v, want modeDivider (LAYOUT)", m.layoutEditor.mode)
	}
	m = pressKey(t, m, "tab")
	if m.layoutEditor.interiorIdx != 1 {
		t.Fatalf("after tab in LAYOUT: interiorIdx=%d, want 1", m.layoutEditor.interiorIdx)
	}

	// Enter COMMAND mode and advance the leaf cursor to leaf 1.
	m = pressKey(t, m, "c")
	if m.layoutEditor.mode != modeLeaf {
		t.Fatalf("after c: mode=%v, want modeLeaf (COMMAND)", m.layoutEditor.mode)
	}
	m = pressKey(t, m, "tab")
	if m.layoutEditor.currentIdx != 1 {
		t.Fatalf("after tab in COMMAND: currentIdx=%d, want 1", m.layoutEditor.currentIdx)
	}

	// Exit back to LAYOUT — divider cursor must still be at 1, leaf cursor at 1.
	m = pressKey(t, m, "esc")
	if m.layoutEditor.mode != modeDivider {
		t.Fatalf("after esc: mode=%v, want modeDivider", m.layoutEditor.mode)
	}
	if m.layoutEditor.interiorIdx != 1 {
		t.Errorf("divider cursor lost: interiorIdx=%d, want 1", m.layoutEditor.interiorIdx)
	}
	if m.layoutEditor.currentIdx != 1 {
		t.Errorf("leaf cursor lost across mode switch: currentIdx=%d, want 1", m.layoutEditor.currentIdx)
	}
	// Editor must still be on its screen — esc from LAYOUT exits the editor,
	// but we only reach LAYOUT here via an esc from COMMAND, so the screen
	// stays put.
	if m.screen != screenLayoutEditor {
		t.Errorf("screen = %v, want screenLayoutEditor (esc-from-COMMAND must NOT exit the editor)", m.screen)
	}
}

// TestLayoutEditor_DividerCycleMatchesInteriorPointers: tab/shift+tab in
// LAYOUT mode (the default) walks layout.InteriorPointers in order, with
// wraparound. The fixed layout has 2 interiors (outer row + inner column).
func TestLayoutEditor_DividerCycleMatchesInteriorPointers(t *testing.T) {
	lay := fixedLayout()
	m, _ := editorModelWithResolver(t, lay)
	openLayoutEditorViaForm(t, m, "myproj", "/tmp/myproj", "triple")
	// Default mode is LAYOUT — no toggle needed.

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
// bumps clamp to [sizeMin, sizeMax]. LAYOUT mode is the default.
func TestLayoutEditor_BumpSizePromotesZeroAndClamps(t *testing.T) {
	lay := fixedLayout()
	m, _ := editorModelWithResolver(t, lay)
	openLayoutEditorViaForm(t, m, "myproj", "/tmp/myproj", "triple")
	// LAYOUT mode by default, on the outer row.

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
// 0 (which the renderer + JXA walker treat as "split evenly"). LAYOUT mode is
// the default.
func TestLayoutEditor_ResetSizeRestoresEvenSplit(t *testing.T) {
	lay := fixedLayout()
	m, _ := editorModelWithResolver(t, lay)
	openLayoutEditorViaForm(t, m, "myproj", "/tmp/myproj", "triple")
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
// when the editor is in COMMAND mode — they should fall through to the
// textinput as plain typed characters so users can put a "+" in a command
// name. Enters command mode explicitly first.
func TestLayoutEditor_DividerKeysAreInertInLeafMode(t *testing.T) {
	lay := fixedLayout()
	m, _ := editorModelWithResolver(t, lay)
	openLayoutEditorViaForm(t, m, "myproj", "/tmp/myproj", "triple")
	m = pressKey(t, m, "c") // → COMMAND mode
	// Press "+" — must land in the textinput, not Size.
	m = pressKey(t, m, "+")
	if got := m.layoutEditor.interiors[0].split.Size; got != 0 {
		t.Errorf("COMMAND-mode +: Size mutated to %v, want 0", got)
	}
	if got := m.layoutEditor.cmdInput.Value(); got != "+" {
		t.Errorf("COMMAND-mode +: textinput = %q, want %q", got, "+")
	}
}

// TestLayoutEditor_ApplyPersistsSize: a divider edit survives apply and lands
// on the materialised layout the CLI receives. LAYOUT mode is the default.
func TestLayoutEditor_ApplyPersistsSize(t *testing.T) {
	lay := fixedLayout()
	m, _ := editorModelWithResolver(t, lay)
	openLayoutEditorViaForm(t, m, "myproj", "/tmp/myproj", "triple")
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
// has zero interiors. The LAYOUT view (default mode) shows an empty-state
// message; tab/+/- are no-ops.
func TestLayoutEditor_DividerEmptyStateForSingleLeafLayout(t *testing.T) {
	lay := &layout.Layout{
		Name: "solo",
		Tabs: []layout.Tab{{Name: "main", Root: layout.Split{Cwd: "."}}},
	}
	m, _ := editorModelWithResolver(t, lay)
	openLayoutEditorViaForm(t, m, "solo", "/tmp/solo", "triple")

	if m.layoutEditor.mode != modeDivider {
		t.Fatalf("default mode = %v, want modeDivider", m.layoutEditor.mode)
	}
	if got := len(m.layoutEditor.interiors); got != 0 {
		t.Errorf("interiors = %d, want 0", got)
	}
	// View must mention the empty state — anchor on a stable substring.
	if !strings.Contains(m.View(), "no dividers") {
		t.Errorf("LAYOUT view missing empty-state hint. View:\n%s", m.View())
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
// of TestLayoutEditor_DividerKeysAreInertInLeafMode: every printable LAYOUT-
// mode key falls through to the textinput in COMMAND mode and never mutates
// Size. Pins the "don't eat printable chars" contract for the full key set,
// including "c" (which is the LAYOUT-mode shortcut to enter COMMAND mode —
// but in COMMAND mode it must be a literal letter).
func TestLayoutEditor_AllDividerKeysAreInertInLeafMode(t *testing.T) {
	cases := []string{"+", "=", "-", "_", "0", "c"}
	for _, k := range cases {
		t.Run(k, func(t *testing.T) {
			lay := fixedLayout()
			m, _ := editorModelWithResolver(t, lay)
			openLayoutEditorViaForm(t, m, "myproj", "/tmp/myproj", "triple")
			m = pressKey(t, m, "c") // → COMMAND mode
			// Verify mode flip happened; otherwise the assertions below are
			// vacuous.
			if m.layoutEditor.mode != modeLeaf {
				t.Fatalf("setup: failed to enter COMMAND mode, got %v", m.layoutEditor.mode)
			}
			m = pressKey(t, m, k)
			if got := m.layoutEditor.interiors[0].split.Size; got != 0 {
				t.Errorf("COMMAND-mode %q: Size mutated to %v, want 0", k, got)
			}
			if got := m.layoutEditor.cmdInput.Value(); got != k {
				t.Errorf("COMMAND-mode %q: textinput = %q, want %q", k, got, k)
			}
		})
	}
}

// TestLayoutEditor_ExitCommandModeFlushesInFlightCommand: typing into the
// textinput then pressing enter (or esc) to exit COMMAND mode must persist
// the typed text to the active leaf's Command. Otherwise users would silently
// lose edits when finishing a command edit. Verifies the invariant by then
// applying (ctrl+s) and checking the materialised layout.
func TestLayoutEditor_ExitCommandModeFlushesInFlightCommand(t *testing.T) {
	for _, exitKey := range []string{"enter", "esc"} {
		t.Run(exitKey, func(t *testing.T) {
			lay := fixedLayout()
			m, _ := editorModelWithResolver(t, lay)
			openLayoutEditorViaForm(t, m, "myproj", "/tmp/myproj", "triple")
			m = pressKey(t, m, "c") // enter COMMAND mode

			for _, r := range "vim" {
				m = pressKey(t, m, string(r))
			}
			// Exit back to LAYOUT mode without an explicit save.
			m = pressKey(t, m, exitKey)
			if m.layoutEditor.mode != modeDivider {
				t.Fatalf("after %s: mode=%v, want modeDivider (must return to LAYOUT)", exitKey, m.layoutEditor.mode)
			}
			// Dispatch — the only way "vim" survives to here is if
			// exitCommandMode flushed it; saveCurrent in apply is a no-op now
			// (we're in LAYOUT mode).
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
				t.Errorf("first leaf Command = %q, want %q (%s must flush in-flight typed text)", got, "vim", exitKey)
			}
		})
	}
}

// TestLayoutEditor_DefaultModeIsLayout pins the entry mode. Most users want
// to nudge sizes, not type commands; opening straight into a focused
// textinput is more friction than landing on a navigable preview.
func TestLayoutEditor_DefaultModeIsLayout(t *testing.T) {
	lay := fixedLayout()
	m, _ := editorModelWithResolver(t, lay)
	openLayoutEditorViaForm(t, m, "myproj", "/tmp/myproj", "triple")
	if m.layoutEditor.mode != modeDivider {
		t.Errorf("default mode = %v, want modeDivider (LAYOUT). Users should land in navigation mode, not a focused textinput.", m.layoutEditor.mode)
	}
	if m.layoutEditor.cmdInput.Focused() {
		t.Errorf("cmdInput.Focused() = true, want false (no textinput should grab keys on entry)")
	}
}

// TestLayoutEditor_EscIsModeAwareCorrectScreen pins the dual meaning of esc:
//   - COMMAND mode: commit & return to LAYOUT mode (stays in the editor)
//   - LAYOUT mode: exit the editor back to the form
//
// A regression that lost the mode-gate would either trap the user in COMMAND
// mode (esc does nothing) or make a single esc from inside the textinput blow
// the editor away (losing context).
func TestLayoutEditor_EscIsModeAwareCorrectScreen(t *testing.T) {
	t.Run("esc in COMMAND mode returns to LAYOUT and stays in editor", func(t *testing.T) {
		lay := fixedLayout()
		m, _ := editorModelWithResolver(t, lay)
		openLayoutEditorViaForm(t, m, "myproj", "/tmp/myproj", "triple")
		m = pressKey(t, m, "c")
		m = pressKey(t, m, "esc")
		if m.layoutEditor.mode != modeDivider {
			t.Errorf("mode = %v, want modeDivider", m.layoutEditor.mode)
		}
		if m.screen != screenLayoutEditor {
			t.Errorf("screen = %v, want screenLayoutEditor (esc in COMMAND must NOT exit the editor)", m.screen)
		}
	})
	t.Run("esc in LAYOUT mode exits the editor to the form", func(t *testing.T) {
		lay := fixedLayout()
		m, _ := editorModelWithResolver(t, lay)
		openLayoutEditorViaForm(t, m, "myproj", "/tmp/myproj", "triple")
		// Default is LAYOUT mode — no setup needed.
		m = pressKey(t, m, "esc")
		if m.screen != screenForm {
			t.Errorf("screen = %v, want screenForm (esc in LAYOUT must exit the editor)", m.screen)
		}
	})
}

// TestLayoutEditor_EnterCommandFocusesInput pins that entering COMMAND mode
// actually focuses the textinput. Without focus, the bubbles textinput drops
// every keystroke and users would see an empty input no matter what they
// type — silent failure mode.
func TestLayoutEditor_EnterCommandFocusesInput(t *testing.T) {
	lay := fixedLayout()
	m, _ := editorModelWithResolver(t, lay)
	openLayoutEditorViaForm(t, m, "myproj", "/tmp/myproj", "triple")
	if m.layoutEditor.cmdInput.Focused() {
		t.Fatalf("precondition: cmdInput already focused before entering COMMAND mode")
	}
	m = pressKey(t, m, "c")
	if !m.layoutEditor.cmdInput.Focused() {
		t.Errorf("after c: cmdInput.Focused() = false, want true (input must accept keystrokes)")
	}
	// Typing should land in the input.
	m = pressKey(t, m, "v")
	if got := m.layoutEditor.cmdInput.Value(); got != "v" {
		t.Errorf("after typing 'v': cmdInput value = %q, want %q", got, "v")
	}
}
