package picker

import "testing"

func TestForm_Collect_RequiresName(t *testing.T) {
	f := newFormModel(FormDefaults{Dir: "/x"})
	f.inputs[fieldName].SetValue("  ")
	if _, err := f.collect(); err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestForm_Collect_RequiresDirOrFrom(t *testing.T) {
	f := newFormModel(FormDefaults{Name: "alpha"})
	f.inputs[fieldDir].SetValue("")
	f.inputs[fieldFrom].SetValue("")
	if _, err := f.collect(); err == nil {
		t.Fatal("expected error when both Dir and From empty")
	}
}

func TestForm_Collect_DefaultsTemplateToDefault(t *testing.T) {
	f := newFormModel(FormDefaults{Name: "a", Dir: "/x", Template: ""})
	f.inputs[fieldTemplate].SetValue("")
	got, err := f.collect()
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got.Template != "default" {
		t.Errorf("template = %q, want 'default'", got.Template)
	}
}

func TestForm_Collect_TrimsWhitespace(t *testing.T) {
	f := newFormModel(FormDefaults{})
	f.inputs[fieldName].SetValue("  alpha  ")
	f.inputs[fieldDir].SetValue("  /tmp/x  ")
	f.inputs[fieldFrom].SetValue("  https://example/repo  ")
	f.inputs[fieldTemplate].SetValue("  dev  ")
	got, err := f.collect()
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got.Name != "alpha" || got.Dir != "/tmp/x" || got.From != "https://example/repo" || got.Template != "dev" {
		t.Errorf("trimming failed: %+v", got)
	}
}

func TestForm_FocusNextWraps(t *testing.T) {
	f := newFormModel(FormDefaults{})
	if f.focus != fieldName {
		t.Fatalf("initial focus = %d, want %d", f.focus, fieldName)
	}
	for i := 0; i < int(numFormFields); i++ {
		f.focusNext()
	}
	if f.focus != fieldName {
		t.Errorf("after %d nexts focus = %d, want wrapped to %d", numFormFields, f.focus, fieldName)
	}
}

func TestForm_FocusPrevWraps(t *testing.T) {
	f := newFormModel(FormDefaults{})
	f.focusPrev()
	if f.focus != numFormFields-1 {
		t.Errorf("focus after prev from 0 = %d, want %d", f.focus, numFormFields-1)
	}
}
