package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/erzz/boo/internal/state"
)

// `boo layouts` is the discovery surface for the layout system. These
// tests pin down the things a user needs to be able to rely on:
// every layout is listed, the source tag is present, the description
// shows up, the preview renders. A future change to formatting that
// hides any of these would be a regression even if it looked prettier.

// makeAppForLayouts builds a minimal app pointing at a temp config dir
// so we can drop user templates in and have ListTemplates / ResolveTemplate
// see them. We don't need Ghostty / Runner / Git for runLayouts.
func makeAppForLayouts(t *testing.T) *app {
	t.Helper()
	dir := t.TempDir()
	p := state.Paths{
		ConfigDir:   filepath.Join(dir, "config"),
		DataDir:     filepath.Join(dir, "data"),
		LayoutsDir:  filepath.Join(dir, "config", "layouts"),
		ProjectsDir: filepath.Join(dir, "data", "projects"),
	}
	if err := p.EnsureDirs(); err != nil {
		t.Fatalf("EnsureDirs: %v", err)
	}
	return &app{Paths: p}
}

func TestRunLayouts_ShowsAllBuiltinsWithSourceTag(t *testing.T) {
	a := makeAppForLayouts(t)
	var out bytes.Buffer
	if err := runLayouts(a, &out); err != nil {
		t.Fatalf("runLayouts: %v", err)
	}
	got := out.String()
	for _, name := range []string{"1x1x1", "1x2x1", "triple"} {
		if !strings.Contains(got, name) {
			t.Errorf("output missing built-in %q:\n%s", name, got)
		}
	}
	// Every entry must show its source. If this assertion fails after
	// a formatting change, users can no longer tell which layouts
	// they themselves added vs which ship with boo.
	if !strings.Contains(got, "[built-in]") {
		t.Errorf("output missing [built-in] tag:\n%s", got)
	}
}

func TestRunLayouts_IncludesDescription(t *testing.T) {
	a := makeAppForLayouts(t)
	var out bytes.Buffer
	if err := runLayouts(a, &out); err != nil {
		t.Fatalf("runLayouts: %v", err)
	}
	got := out.String()
	// `triple`'s description references the AppleScript flattening
	// caveat — that's the whole reason the layout exists. Losing
	// this would silently strip the educational value.
	if !strings.Contains(got, "1-on-the-left") {
		t.Errorf("triple description missing key phrase:\n%s", got)
	}
}

func TestRunLayouts_IncludesPreview(t *testing.T) {
	a := makeAppForLayouts(t)
	var out bytes.Buffer
	if err := runLayouts(a, &out); err != nil {
		t.Fatalf("runLayouts: %v", err)
	}
	got := out.String()
	// At minimum: every layout must produce ASCII border characters
	// in the indented preview block. If a future change moves the
	// preview elsewhere or drops it, this will catch it.
	if !strings.Contains(got, "+--") || !strings.Contains(got, "--+") {
		t.Errorf("output missing ASCII preview borders:\n%s", got)
	}
}

func TestRunLayouts_UserTemplateIsTaggedUserAndShadowsBuiltin(t *testing.T) {
	a := makeAppForLayouts(t)
	// Drop a user override of `1x1x1`. Same name, different
	// description so we can tell which one rendered.
	userYAML := []byte(`# This is the user's override.
name: 1x1x1
tabs:
  - root:
      cwd: "."
`)
	if err := os.WriteFile(filepath.Join(a.Paths.LayoutsDir, "1x1x1.yaml"), userYAML, 0o644); err != nil {
		t.Fatalf("write user template: %v", err)
	}
	var out bytes.Buffer
	if err := runLayouts(a, &out); err != nil {
		t.Fatalf("runLayouts: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "[user]") {
		t.Errorf("expected [user] tag for shadowed 1x1x1:\n%s", got)
	}
	if !strings.Contains(got, "user's override") {
		t.Errorf("expected user description (not built-in) to render:\n%s", got)
	}
	// `1x1x1` should still appear exactly once (no duplicate from
	// built-in + user).
	if c := strings.Count(got, "\n1x1x1 "); c > 1 {
		t.Errorf("1x1x1 listed %d times, want 1:\n%s", c, got)
	}
}

func TestRunLayouts_BadUserTemplateIsReportedInline(t *testing.T) {
	// A broken user template must NOT abort the listing — we want
	// the user to see "this one is broken, here's why" alongside
	// the working ones, so they can fix it without being locked out.
	a := makeAppForLayouts(t)
	// Invalid YAML: tab map key without value, then a stray bracket.
	if err := os.WriteFile(filepath.Join(a.Paths.LayoutsDir, "broken.yaml"), []byte("name: broken\ntabs: [ {root: }\n"), 0o644); err != nil {
		t.Fatalf("write broken template: %v", err)
	}
	var out bytes.Buffer
	if err := runLayouts(a, &out); err != nil {
		t.Fatalf("runLayouts must not error on a broken user template: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "broken") {
		t.Errorf("broken template not listed:\n%s", got)
	}
	if !strings.Contains(got, "[error]") {
		t.Errorf("broken template not tagged [error]:\n%s", got)
	}
	// The healthy built-ins must still be there.
	if !strings.Contains(got, "1x1x1") {
		t.Errorf("broken template suppressed healthy listings:\n%s", got)
	}
}
