package picker

import (
	"strings"
	"testing"
)

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
	if got.Template != "triple" {
		t.Errorf("template = %q, want 'triple'", got.Template)
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

// `boo new` and bare `boo` show the new-project form. When a preview
// callback is wired in, the form should:
//   - render the preview output below the inputs
//   - default to "triple" when the template field is blank (mirrors
//     collect()'s behaviour, so what the user sees matches what they
//     get on submit)
//   - show NO preview block when the callback returns "" (template
//     unknown / mid-typing) — surfacing errors inside a TUI form is
//     hostile, silence is the right default here

func TestForm_View_RendersPreviewWhenCallbackReturnsContent(t *testing.T) {
	f := newFormModel(FormDefaults{Name: "a", Dir: "/x", Template: "dev"})
	f.preview = func(name string) string {
		if name != "dev" {
			t.Errorf("preview called with %q, want %q", name, "dev")
		}
		return "+---+\n| . |\n+---+"
	}
	out := f.view()
	if !strings.Contains(out, "Preview of layout \"dev\"") {
		t.Errorf("view missing preview header:\n%s", out)
	}
	if !strings.Contains(out, "+---+") {
		t.Errorf("view missing preview body:\n%s", out)
	}
}

func TestForm_View_HidesPreviewWhenCallbackReturnsEmpty(t *testing.T) {
	f := newFormModel(FormDefaults{Name: "a", Dir: "/x", Template: "nope"})
	called := false
	f.preview = func(name string) string {
		called = true
		return ""
	}
	out := f.view()
	if !called {
		t.Error("preview callback not invoked")
	}
	if strings.Contains(out, "Preview of layout") {
		t.Errorf("view should hide preview header on empty callback result:\n%s", out)
	}
}

func TestForm_View_PreviewDefaultsToDefaultWhenTemplateBlank(t *testing.T) {
	f := newFormModel(FormDefaults{Name: "a", Dir: "/x"})
	f.inputs[fieldTemplate].SetValue("")
	got := ""
	f.preview = func(name string) string {
		got = name
		return "preview-body"
	}
	_ = f.view()
	if got != "triple" {
		t.Errorf("preview asked for %q, want %q (must mirror collect())", got, "triple")
	}
}

func TestForm_View_NoPreviewWhenCallbackNil(t *testing.T) {
	// Most callers (delete, save) never wire a previewer. The form
	// must still render cleanly with no preview block at all.
	f := newFormModel(FormDefaults{Name: "a", Dir: "/x", Template: "dev"})
	out := f.view()
	if strings.Contains(out, "Preview of layout") {
		t.Errorf("view should not render preview header without a callback:\n%s", out)
	}
}

// Cycler defaults must not be silently overridden by a stale default
// from upstream callers.
//
// Real regression caught in the YAML migration: `boo new` (no flags)
// was preselecting `1x1x1` in the cycler instead of `triple`. Cause:
// internal/cli/new.go set Template = "default" as a defensive fallback;
// setLayoutNames searched the cycler list for "default", missed, and
// fell back to layoutIdx=0 (alphabetical first = "1x1x1"). The
// underlying textinput value was also stale ("default"), so even if
// the user never touched the cycler, collect() would have submitted
// "default" — not what was visible.
//
// Pin both halves of the contract:
//
//   - With Template="" (the post-fix state), the form starts with
//     "triple" both in the underlying textinput and in the cycler,
//     and collect() agrees.
//   - With Template set to a name that ISN'T in the cycler list,
//     setLayoutNames forces both the cycler index AND the underlying
//     textinput value to a real cycler entry. The form must never
//     submit a value the cycler isn't showing.

func TestForm_NewFormModel_DefaultsTemplateInputToTriple(t *testing.T) {
	f := newFormModel(FormDefaults{Name: "a", Dir: "/x", Template: ""})
	if got := f.inputs[fieldTemplate].Value(); got != "triple" {
		t.Errorf("template input = %q, want %q (mirrors collect() default; bug if these drift)", got, "triple")
	}
}

func TestForm_SetLayoutNames_UnknownTemplateResetsToFirstAndOverwritesInput(t *testing.T) {
	// Caller passes a stale template name that doesn't appear in the
	// cycler list. The cycler must reset to index 0 AND the underlying
	// textinput must be rewritten to match — otherwise collect() would
	// later return the stale name, silently submitting a value the
	// user never saw.
	f := newFormModel(FormDefaults{Name: "a", Dir: "/x", Template: "default"})
	f.setLayoutNames([]string{"1x1x1", "1x2x1", "triple"})

	if f.layoutIdx != 0 {
		t.Errorf("layoutIdx = %d, want 0 (unknown template should reset cycler)", f.layoutIdx)
	}
	if got := f.inputs[fieldTemplate].Value(); got != "1x1x1" {
		t.Errorf("template input = %q, want %q (must match cycler so collect() agrees)", got, "1x1x1")
	}

	intent, err := f.collect()
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if intent.Template != "1x1x1" {
		t.Errorf("intent.Template = %q, want %q (form must never submit a value the cycler isn't showing)", intent.Template, "1x1x1")
	}
}

func TestForm_SetLayoutNames_KnownTemplatePreservesIndex(t *testing.T) {
	// Sanity check the happy path: a template name that IS in the
	// cycler list selects that entry and preserves the textinput value.
	f := newFormModel(FormDefaults{Name: "a", Dir: "/x", Template: "triple"})
	f.setLayoutNames([]string{"1x1x1", "1x2x1", "triple"})

	if f.layoutIdx != 2 {
		t.Errorf("layoutIdx = %d, want 2 (cycler should land on 'triple')", f.layoutIdx)
	}
	if got := f.inputs[fieldTemplate].Value(); got != "triple" {
		t.Errorf("template input = %q, want %q", got, "triple")
	}
}
