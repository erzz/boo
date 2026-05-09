package picker

import (
	"errors"
	"strings"
	"testing"
)

// editModel returns a sized model with edit callbacks wired so 'e' is live and EditIntent dispatches via onEdit.
// Mirrors modelWithDelete / setLayoutModelWithNames.
func editModel(t *testing.T, items ...Item) (*model, *[]string) {
	t.Helper()
	m := sizedModel(120, items...)
	// sizedModel/newTestModel don't init the form (production Run() does); 'e' path needs a real formModel.
	m.form = newFormModel(FormDefaults{}, defaultTheme())
	m.form.setLayoutNames([]string{"1x1x1", "triple"})
	m.layoutNames = []string{"1x1x1", "triple"}
	m.previewTemplate = func(name string) string { return "preview:" + name }
	calls := []string{}
	m.onEdit = func(oldName, newName, newDir, newTemplate string) error {
		calls = append(calls, oldName+"→"+newName+":"+newDir+":"+newTemplate)
		return nil
	}
	m.refreshItems = func() ([]Item, error) { return items, nil }
	return m, &calls
}

// Form-level tests: edit mode flips the title, hides From, and skips it during focus traversal.

func TestForm_EditMode_TitleAndHiddenFrom(t *testing.T) {
	f := newFormModel(FormDefaults{Name: "alpha", Dir: "/tmp/a", Template: "triple"}, defaultTheme())
	f.setEditMode(true, "alpha")

	v := f.view()
	if !strings.Contains(v, "Edit project") {
		t.Errorf("view missing 'Edit project' title:\n%s", v)
	}
	if !strings.Contains(v, "alpha") {
		t.Errorf("view missing edited project name in title:\n%s", v)
	}
	if strings.Contains(v, "Clone from URL") {
		t.Errorf("view shows Clone from URL field in edit mode:\n%s", v)
	}
}

func TestForm_EditMode_FocusSkipsHiddenFromField(t *testing.T) {
	f := newFormModel(FormDefaults{Name: "alpha", Dir: "/tmp/a"}, defaultTheme())
	f.setEditMode(true, "alpha")

	if f.focus != fieldName {
		t.Fatalf("initial focus = %v, want fieldName", f.focus)
	}
	f.focusNext()
	if f.focus != fieldDir {
		t.Errorf("after 1 next: focus = %v, want fieldDir", f.focus)
	}
	f.focusNext()
	if f.focus != fieldTemplate {
		t.Errorf("after 2 next: focus = %v, want fieldTemplate (skipping hidden fieldFrom)", f.focus)
	}
	f.focusNext()
	if f.focus != fieldName {
		t.Errorf("after 3 next: focus = %v, want fieldName (wraparound)", f.focus)
	}

	// Going backwards from Name should skip From in the same way.
	f.focusPrev()
	if f.focus != fieldTemplate {
		t.Errorf("focusPrev from Name: focus = %v, want fieldTemplate", f.focus)
	}
	f.focusPrev()
	if f.focus != fieldDir {
		t.Errorf("focusPrev: focus = %v, want fieldDir (skipping fieldFrom)", f.focus)
	}
}

func TestForm_CollectEdit_BuildsEditIntent(t *testing.T) {
	f := newFormModel(FormDefaults{Name: "alpha", Dir: "/tmp/a", Template: "triple"}, defaultTheme())
	f.setEditMode(true, "alpha")
	f.inputs[fieldName].SetValue("beta")
	f.inputs[fieldDir].SetValue("/tmp/b")
	f.inputs[fieldTemplate].SetValue("1x1x1")

	got, err := f.collectEdit()
	if err != nil {
		t.Fatalf("collectEdit: %v", err)
	}
	if got.OldName != "alpha" {
		t.Errorf("OldName = %q, want alpha", got.OldName)
	}
	if got.NewName != "beta" || got.NewDir != "/tmp/b" || got.NewTemplate != "1x1x1" {
		t.Errorf("EditIntent = %+v", got)
	}
}

func TestForm_CollectEdit_RequiresNameAndDir(t *testing.T) {
	f := newFormModel(FormDefaults{Name: "alpha", Dir: "/tmp/a"}, defaultTheme())
	f.setEditMode(true, "alpha")

	f.inputs[fieldName].SetValue("")
	if _, err := f.collectEdit(); err == nil {
		t.Error("expected error for empty name")
	}

	f.inputs[fieldName].SetValue("beta")
	f.inputs[fieldDir].SetValue("")
	if _, err := f.collectEdit(); err == nil {
		t.Error("expected error for empty dir")
	}
}

func TestForm_TryCommit_EditModeReturnsEditIntent(t *testing.T) {
	f := newFormModel(FormDefaults{Name: "alpha", Dir: "/tmp/a", Template: "triple"}, defaultTheme())
	f.setEditMode(true, "alpha")
	_, submitted, intent, _ := f.tryCommit()
	if !submitted {
		t.Fatal("tryCommit did not submit")
	}
	if _, ok := intent.(EditIntent); !ok {
		t.Errorf("intent type = %T, want EditIntent (value, not pointer)", intent)
	}
}

func TestForm_TryCommit_NewModeStillReturnsNewProjectIntent(t *testing.T) {
	f := newFormModel(FormDefaults{Name: "alpha", Dir: "/tmp/a"}, defaultTheme())
	_, submitted, intent, _ := f.tryCommit()
	if !submitted {
		t.Fatal("tryCommit did not submit")
	}
	if _, ok := intent.(NewProjectIntent); !ok {
		t.Errorf("intent type = %T, want NewProjectIntent", intent)
	}
}

// Picker-level tests: 'e' opens form pre-populated; runIntent dispatches EditIntent; errors → screenError.

func TestList_EPressOpensEditFormPrePopulated(t *testing.T) {
	m, calls := editModel(t,
		Item{Key: "alpha", Title: "alpha", Description: "/tmp/a", Layout: "triple"},
	)
	m = pressKey(t, m, "e")

	if m.screen != screenForm {
		t.Fatalf("screen = %v, want screenForm", m.screen)
	}
	if !m.form.editMode {
		t.Errorf("form.editMode = false, want true")
	}
	if got := m.form.inputs[fieldName].Value(); got != "alpha" {
		t.Errorf("Name input = %q, want alpha", got)
	}
	if got := m.form.inputs[fieldDir].Value(); got != "/tmp/a" {
		t.Errorf("Dir input = %q, want /tmp/a", got)
	}
	if got := m.form.inputs[fieldTemplate].Value(); got != "triple" {
		t.Errorf("Template input = %q, want triple", got)
	}
	if len(*calls) != 0 {
		t.Errorf("OnEdit fired before submit: %v", *calls)
	}
}

func TestList_EPressNoCallbackIsNoop(t *testing.T) {
	m := sizedModel(120, Item{Key: "alpha", Title: "alpha", Description: "/tmp/a"})
	// Deliberately no onEdit.
	m = pressKey(t, m, "e")
	if m.screen != screenList {
		t.Errorf("screen = %v, want screenList (e dead-keyed without OnEdit)", m.screen)
	}
}

func TestEditIntent_DispatchesOnEditAndRefreshes(t *testing.T) {
	refreshCalls := 0
	m, calls := editModel(t,
		Item{Key: "alpha", Title: "alpha", Description: "/tmp/a", Layout: "triple"},
	)
	// Wrap refreshItems so we can count invocations.
	origItems := []Item{{Key: "beta", Title: "beta", Description: "/tmp/b"}}
	m.refreshItems = func() ([]Item, error) {
		refreshCalls++
		return origItems, nil
	}

	_, cmd := m.runIntent(EditIntent{
		OldName: "alpha", NewName: "beta", NewDir: "/tmp/b", NewTemplate: "triple",
	})
	// startEnrich returns a tea.Cmd that calls RefreshItems async; execute manually for synchronous assertion.
	if cmd != nil {
		cmd()
	}

	if len(*calls) != 1 || (*calls)[0] != "alpha→beta:/tmp/b:triple" {
		t.Errorf("OnEdit calls = %v, want [alpha→beta:/tmp/b:triple]", *calls)
	}
	if refreshCalls != 1 {
		t.Errorf("RefreshItems called %d times, want 1", refreshCalls)
	}
	if m.screen != screenList {
		t.Errorf("screen = %v after edit, want screenList", m.screen)
	}
	if m.form.editMode {
		t.Error("form still in editMode after successful EditIntent")
	}
	if m.intent != nil {
		t.Errorf("intent = %v, want nil (handled in loop)", m.intent)
	}
}

func TestEditIntent_OnEditError_ShowsErrorScreen(t *testing.T) {
	m := sizedModel(120, Item{Key: "alpha", Title: "alpha", Description: "/tmp/a"})
	m.onEdit = func(_, _, _, _ string) error {
		return errors.New("disk on fire")
	}
	m.refreshItems = func() ([]Item, error) { return nil, nil }

	m.runIntent(EditIntent{OldName: "alpha", NewName: "beta", NewDir: "/tmp/b"})
	if m.screen != screenError {
		t.Fatalf("screen = %v, want screenError", m.screen)
	}
	if !strings.Contains(m.errMsg, "disk on fire") {
		t.Errorf("errMsg = %q, expected to contain 'disk on fire'", m.errMsg)
	}
}

// TestForm_EscFromEditMode_ClearsEditMode: Esc from edit-mode form returns to list AND clears editMode
// so the next form open (e.g. "+ New project") doesn't inherit the hidden-From / "Edit project" state.
func TestForm_EscFromEditMode_ClearsEditMode(t *testing.T) {
	m, _ := editModel(t,
		Item{Key: "alpha", Title: "alpha", Description: "/tmp/a", Layout: "triple"},
	)
	m = pressKey(t, m, "e")
	if !m.form.editMode {
		t.Fatal("setup: expected editMode after pressing e")
	}
	m = pressKey(t, m, "esc")
	if m.screen != screenList {
		t.Errorf("screen = %v, want screenList after esc", m.screen)
	}
	if m.form.editMode {
		t.Error("form.editMode still true after esc — should be reset")
	}
}
