package theme

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolve_BuiltinDefault(t *testing.T) {
	t.Parallel()
	r, err := Resolve("", "default")
	if err != nil {
		t.Fatalf("Resolve(default): %v", err)
	}
	if r.Source != SourceBuiltin {
		t.Errorf("Source = %q, want builtin", r.Source)
	}
	if r.Theme.Name != "default" {
		t.Errorf("Name = %q, want default", r.Theme.Name)
	}
	if r.Theme.Colors.Accent == "" {
		t.Error("default theme must populate Accent")
	}
	if r.Theme.Colors.Border == "" {
		t.Error("default theme must populate Border")
	}
}

func TestResolve_EmptyNameMeansDefault(t *testing.T) {
	t.Parallel()
	r, err := Resolve("", "")
	if err != nil {
		t.Fatalf("Resolve(\"\"): %v", err)
	}
	if r.Theme.Name != "default" {
		t.Errorf("empty name should resolve to default, got %q", r.Theme.Name)
	}
}

func TestResolve_UnknownName(t *testing.T) {
	t.Parallel()
	if _, err := Resolve("", "no-such-theme"); err == nil {
		t.Error("Resolve unknown name should error")
	}
}

func TestResolve_RejectsPathTraversal(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"../etc/passwd", "../default", ".", "..", "foo/bar"} {
		if _, err := Resolve("", name); err == nil {
			t.Errorf("Resolve(%q) should reject invalid name", name)
		}
	}
}

func TestResolve_UserShadowsBuiltin(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// Override the built-in default with a user theme of the same
	// name. User wins.
	custom := []byte(`name: default
description: user override
colors:
  accent: "#ff0000"
`)
	if err := os.WriteFile(filepath.Join(dir, "default.yaml"), custom, 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := Resolve(dir, "default")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if r.Source != SourceUser {
		t.Errorf("Source = %q, want user", r.Source)
	}
	if r.Theme.Colors.Accent != "#ff0000" {
		t.Errorf("Accent = %q, want #ff0000 (user override)", r.Theme.Colors.Accent)
	}
}

func TestResolve_UserThemeBackfillsMissingSlots(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// User specifies only accent. All other slots should fall back
	// to the built-in default's values.
	partial := []byte(`name: minimal
colors:
  accent: "#abcdef"
`)
	if err := os.WriteFile(filepath.Join(dir, "minimal.yaml"), partial, 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := Resolve(dir, "minimal")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if r.Theme.Colors.Accent != "#abcdef" {
		t.Errorf("Accent = %q, want user override #abcdef", r.Theme.Colors.Accent)
	}
	// These slots were not specified; expect default theme's values.
	def := MustDefault()
	if r.Theme.Colors.Border != def.Colors.Border {
		t.Errorf("Border = %q, want backfilled %q", r.Theme.Colors.Border, def.Colors.Border)
	}
	if r.Theme.Colors.OK != def.Colors.OK {
		t.Errorf("OK = %q, want backfilled %q", r.Theme.Colors.OK, def.Colors.OK)
	}
}

func TestResolve_UserThemeMissingNameFallsBackToFilename(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// No `name:` field — should pick up the filename stem.
	noName := []byte(`description: anonymous
colors:
  accent: "#123456"
`)
	if err := os.WriteFile(filepath.Join(dir, "anon.yaml"), noName, 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := Resolve(dir, "anon")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if r.Theme.Name != "anon" {
		t.Errorf("Name = %q, want anon (from filename)", r.Theme.Name)
	}
}

func TestResolve_MalformedUserThemeReturnsError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "broken.yaml"), []byte("colors: [not, a, map\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Resolve(dir, "broken"); err == nil {
		t.Error("Resolve of malformed YAML should error")
	}
}

func TestList_IncludesBuiltinAndUser(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, name := range []string{"alpha", "beta"} {
		if err := os.WriteFile(filepath.Join(dir, name+".yaml"), []byte("name: "+name+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	names, err := List(dir)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	got := map[string]bool{}
	for _, n := range names {
		got[n] = true
	}
	for _, want := range []string{"default", "alpha", "beta"} {
		if !got[want] {
			t.Errorf("List missing %q (got %v)", want, names)
		}
	}
}

func TestList_DeduplicatesUserShadowingBuiltin(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// User theme with the same name as the built-in default.
	if err := os.WriteFile(filepath.Join(dir, "default.yaml"), []byte("name: default\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	names, err := List(dir)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	count := 0
	for _, n := range names {
		if n == "default" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("default appears %d times, want 1", count)
	}
}

func TestSourceOf(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "mine.yaml"), []byte("name: mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if src, ok := SourceOf(dir, "mine"); !ok || src != SourceUser {
		t.Errorf("SourceOf(mine) = %q, %v; want user, true", src, ok)
	}
	if src, ok := SourceOf(dir, "default"); !ok || src != SourceBuiltin {
		t.Errorf("SourceOf(default) = %q, %v; want builtin, true", src, ok)
	}
	if _, ok := SourceOf(dir, "nope"); ok {
		t.Error("SourceOf(nope) should be false")
	}
}

func TestMustDefault_PanicSafe(t *testing.T) {
	t.Parallel()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("MustDefault panicked: %v", r)
		}
	}()
	th := MustDefault()
	if th.Name != "default" {
		t.Errorf("Name = %q, want default", th.Name)
	}
}

func TestBuiltinYAML(t *testing.T) {
	t.Parallel()
	data, err := BuiltinYAML("default")
	if err != nil {
		t.Fatalf("BuiltinYAML(default): %v", err)
	}
	if len(data) == 0 {
		t.Error("BuiltinYAML returned empty bytes")
	}
	if _, err := BuiltinYAML("../etc/passwd"); err == nil {
		t.Error("BuiltinYAML should reject path traversal")
	}
	if _, err := BuiltinYAML("nonexistent"); err == nil {
		t.Error("BuiltinYAML should error for unknown theme")
	}
}

// TestResolve_UnknownFieldErrors verifies that a user theme with a typo in a
// field name (e.g. "acent" instead of "accent") is rejected rather than
// silently dropped.  Strict decoding surfaces the mistake immediately.
func TestResolve_UnknownFieldErrors(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	typo := []byte(`name: oops
colors:
  acent: "#ff0000"
`)
	if err := os.WriteFile(filepath.Join(dir, "oops.yaml"), typo, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Resolve(dir, "oops"); err == nil {
		t.Error("Resolve should error on unknown field 'acent' (typo of 'accent'), got nil")
	}
}
