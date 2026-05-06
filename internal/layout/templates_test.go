package layout

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveTemplate_BuiltinDefault(t *testing.T) {
	r, err := ResolveTemplate("", "default")
	if err != nil {
		t.Fatalf("ResolveTemplate default: %v", err)
	}
	if r.Source != SourceBuiltin {
		t.Fatalf("expected SourceBuiltin, got %s", r.Source)
	}
	if r.Layout.Name != "default" {
		t.Fatalf("expected name 'default', got %q", r.Layout.Name)
	}
	if err := r.Layout.Validate(); err != nil {
		t.Fatalf("built-in default invalid: %v", err)
	}
}

func TestResolveTemplate_BuiltinDev(t *testing.T) {
	r, err := ResolveTemplate("", "dev")
	if err != nil {
		t.Fatalf("ResolveTemplate dev: %v", err)
	}
	if r.Source != SourceBuiltin {
		t.Fatalf("expected SourceBuiltin, got %s", r.Source)
	}
	if len(r.Layout.Tabs) != 2 {
		t.Fatalf("expected 2 tabs, got %d", len(r.Layout.Tabs))
	}
}

func TestResolveTemplate_EmptyNameDefaultsToDefault(t *testing.T) {
	r, err := ResolveTemplate("", "")
	if err != nil {
		t.Fatalf("ResolveTemplate '': %v", err)
	}
	if r.Layout.Name != "default" {
		t.Fatalf("expected name 'default', got %q", r.Layout.Name)
	}
}

func TestResolveTemplate_UserShadowsBuiltin(t *testing.T) {
	dir := t.TempDir()
	custom := []byte(`name = "default"

[[tab]]
name = "user-override"

  [[tab.split]]
  cwd = "."
`)
	if err := os.WriteFile(filepath.Join(dir, "default.toml"), custom, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	r, err := ResolveTemplate(dir, "default")
	if err != nil {
		t.Fatalf("ResolveTemplate: %v", err)
	}
	if r.Source != SourceUser {
		t.Fatalf("expected SourceUser, got %s", r.Source)
	}
	if r.Layout.Tabs[0].Name != "user-override" {
		t.Fatalf("user template not used: %+v", r.Layout)
	}
}

func TestResolveTemplate_UserOnly(t *testing.T) {
	dir := t.TempDir()
	custom := []byte(`name = "mine"

[[tab]]
name = "x"

  [[tab.split]]
  cwd = "."
`)
	if err := os.WriteFile(filepath.Join(dir, "mine.toml"), custom, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	r, err := ResolveTemplate(dir, "mine")
	if err != nil {
		t.Fatalf("ResolveTemplate: %v", err)
	}
	if r.Source != SourceUser || r.Layout.Name != "mine" {
		t.Fatalf("unexpected: %+v", r)
	}
}

func TestResolveTemplate_NotFound(t *testing.T) {
	_, err := ResolveTemplate(t.TempDir(), "no-such-template")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestResolveTemplate_RejectsTraversal(t *testing.T) {
	for _, name := range []string{"../etc/passwd", "/abs", "..", ".", "a/b"} {
		if _, err := ResolveTemplate(t.TempDir(), name); err == nil {
			t.Fatalf("expected error for %q", name)
		}
	}
}

func TestResolveTemplate_InvalidUserTemplate(t *testing.T) {
	dir := t.TempDir()
	// Layout with no tabs fails Validate.
	bad := []byte(`name = "broken"
`)
	if err := os.WriteFile(filepath.Join(dir, "broken.toml"), bad, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, err := ResolveTemplate(dir, "broken")
	if err == nil {
		t.Fatal("expected error for invalid user template")
	}
}

func TestListTemplates_BuiltinsOnly(t *testing.T) {
	names, err := ListTemplates("")
	if err != nil {
		t.Fatalf("ListTemplates: %v", err)
	}
	if len(names) < 2 {
		t.Fatalf("expected at least 2 built-ins, got %v", names)
	}
	want := map[string]bool{"default": false, "dev": false}
	for _, n := range names {
		if _, ok := want[n]; ok {
			want[n] = true
		}
	}
	for n, found := range want {
		if !found {
			t.Errorf("expected built-in %q to be listed, got %v", n, names)
		}
	}
}

func TestListTemplates_UnionAndDedup(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"default.toml", "mine.toml"} {
		if err := os.WriteFile(filepath.Join(dir, n),
			[]byte("name = \"x\"\n[[tab]]\n[[tab.split]]\ncwd = \".\"\n"),
			0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	names, err := ListTemplates(dir)
	if err != nil {
		t.Fatalf("ListTemplates: %v", err)
	}
	count := map[string]int{}
	for _, n := range names {
		count[n]++
	}
	if count["default"] != 1 {
		t.Errorf("default appeared %d times, want 1", count["default"])
	}
	if count["mine"] != 1 {
		t.Errorf("mine missing or duplicated: %v", names)
	}
}
