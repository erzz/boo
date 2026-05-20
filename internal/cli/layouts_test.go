package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/erzz/boo/internal/state"
)

// boo layouts tests: every layout listed, source tag present, description shown, preview renders.
// Any formatting change that hides those is a regression.

// makeAppForLayouts builds a minimal app pointing at a temp config dir.
func makeAppForLayouts(t *testing.T) *app {
	t.Helper()
	dir := t.TempDir()
	p := state.Paths{
		ConfigDir:   filepath.Join(dir, "config"),
		LayoutsDir:  filepath.Join(dir, "config", "layouts"),
		ProjectsDir: filepath.Join(dir, "config", "projects"),
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
	// Every entry must show its source so users can distinguish their own from built-ins.
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
	// `triple`'s description carries the layout rationale; losing it strips educational value.
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
	// Every layout must produce ASCII border characters in the indented preview block.
	if !strings.Contains(got, "+--") || !strings.Contains(got, "--+") {
		t.Errorf("output missing ASCII preview borders:\n%s", got)
	}
}

func TestRunLayouts_UserTemplateIsTaggedUserAndShadowsBuiltin(t *testing.T) {
	a := makeAppForLayouts(t)
	// Drop a user override of `1x1x1`. Description from leading comment block (extractDescription),
	// not a YAML field. `split:` is the correct key (json:"split"), not `root:`.
	userYAML := []byte(`# This is the user's override.
name: 1x1x1
tabs:
  - name: main
    split:
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
	// `1x1x1` should appear exactly once (no duplicate from built-in + user).
	if c := strings.Count(got, "\n1x1x1 "); c > 1 {
		t.Errorf("1x1x1 listed %d times, want 1:\n%s", c, got)
	}
}

func TestRunLayouts_BadUserTemplateIsReportedInline(t *testing.T) {
	// A broken user template must NOT abort listing — show it as [error] alongside working ones.
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

func TestRunLayouts_ResponsiveTemplateUsesDefaultVariantForPreview(t *testing.T) {
	a := makeAppForLayouts(t)
	responsive := []byte(`# Responsive template.
name: responsive
variants:
  - min_cols: 120
    tabs:
      - name: wide
        split:
          direction: row
          children:
            - cwd: "."
            - cwd: logs
  - tabs:
      - name: compact
        split:
          cwd: "."
`)
	if err := os.WriteFile(filepath.Join(a.Paths.LayoutsDir, "responsive.yaml"), responsive, 0o644); err != nil {
		t.Fatalf("write responsive template: %v", err)
	}
	var out bytes.Buffer
	if err := runLayouts(a, &out); err != nil {
		t.Fatalf("runLayouts: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, `Tab 0 "compact"`) {
		t.Fatalf("output missing default responsive preview:\n%s", got)
	}
	if strings.Contains(got, "preview unavailable") {
		t.Fatalf("responsive preview unexpectedly unavailable:\n%s", got)
	}
}
