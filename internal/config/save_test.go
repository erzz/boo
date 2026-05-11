package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSetUITheme_CreatesFileWhenAbsent covers the absent-file code path in
// SetUITheme: the file must be created and Load must parse it correctly.
func TestSetUITheme_CreatesFileWhenAbsent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")

	if err := SetUITheme(path, "tokyonight"); err != nil {
		t.Fatalf("SetUITheme: %v", err)
	}

	cfg, _, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ThemeOr("default") != "tokyonight" {
		t.Errorf("got %q, want tokyonight", cfg.ThemeOr("default"))
	}
}

// TestSetUITheme_PreservesOtherKeys verifies that updating the theme does not
// drop other top-level keys or nested values.
func TestSetUITheme_PreservesOtherKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")

	original := []byte("default_layout: grid\nui:\n  theme: default\ngit:\n  default_remote: origin\n")
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := SetUITheme(path, "solarized-dark"); err != nil {
		t.Fatalf("SetUITheme: %v", err)
	}

	cfg, _, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ThemeOr("default") != "solarized-dark" {
		t.Errorf("theme: got %q, want solarized-dark", cfg.ThemeOr("default"))
	}
	if cfg.DefaultLayout == nil || *cfg.DefaultLayout != "grid" {
		t.Errorf("default_layout lost: %+v", cfg.DefaultLayout)
	}
	if cfg.Git.DefaultRemote == nil || *cfg.Git.DefaultRemote != "origin" {
		t.Errorf("git.default_remote lost: %+v", cfg.Git.DefaultRemote)
	}
}

// TestSetUITheme_EmptyPath verifies that an empty path is rejected before any
// filesystem work is attempted.
func TestSetUITheme_EmptyPath(t *testing.T) {
	if err := SetUITheme("", "default"); err == nil {
		t.Fatal("expected error for empty path")
	}
}

// TestSetUITheme_PreservesComments is the critical round-trip test for the
// comment-preserving writer. Comments, key ordering, and non-theme fields must
// all survive a SetUITheme call exactly as authored — this was the whole point.
func TestSetUITheme_PreservesComments(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")

	original := `# boo configuration

ui:
  # theme picks the colour palette
  theme: default
  # picker behaviour
  show_status: true

# layout defaults
default_layout: triple
`
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatalf("seed config: %v", err)
	}

	if err := SetUITheme(path, "tokyonight"); err != nil {
		t.Fatalf("SetUITheme: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("re-read config: %v", err)
	}
	gotStr := string(got)

	if !strings.Contains(gotStr, "theme: tokyonight") {
		t.Errorf("theme not updated; file:\n%s", gotStr)
	}
	for _, comment := range []string{
		"# boo configuration",
		"# theme picks the colour palette",
		"# picker behaviour",
		"# layout defaults",
	} {
		if !strings.Contains(gotStr, comment) {
			t.Errorf("comment lost %q; file:\n%s", comment, gotStr)
		}
	}
	if !strings.Contains(gotStr, "show_status: true") {
		t.Errorf("show_status lost; file:\n%s", gotStr)
	}
	if !strings.Contains(gotStr, "default_layout: triple") {
		t.Errorf("default_layout lost; file:\n%s", gotStr)
	}
	// Key ordering: ui must precede default_layout.
	uiIdx := strings.Index(gotStr, "ui:")
	dlIdx := strings.Index(gotStr, "default_layout:")
	if uiIdx < 0 || dlIdx < 0 || dlIdx < uiIdx {
		t.Errorf("key ordering wrong — ui should precede default_layout; file:\n%s", gotStr)
	}
}

// TestSetUITheme_NoUISection verifies that when no "ui:" key exists, SetUITheme
// adds one without disturbing other top-level keys or their comments.
func TestSetUITheme_NoUISection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")

	original := `# boo configuration
# layout defaults
default_layout: triple
`
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatalf("seed config: %v", err)
	}

	if err := SetUITheme(path, "solarized-dark"); err != nil {
		t.Fatalf("SetUITheme: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("re-read config: %v", err)
	}
	gotStr := string(got)

	if !strings.Contains(gotStr, "theme: solarized-dark") {
		t.Errorf("theme not found; file:\n%s", gotStr)
	}
	if !strings.Contains(gotStr, "default_layout: triple") {
		t.Errorf("default_layout lost; file:\n%s", gotStr)
	}
	if !strings.Contains(gotStr, "# boo configuration") {
		t.Errorf("top-level comment lost; file:\n%s", gotStr)
	}
	// Confirm via Load that the values round-trip cleanly.
	cfg, _, err := Load(path)
	if err != nil {
		t.Fatalf("Load after write: %v", err)
	}
	if cfg.ThemeOr("default") != "solarized-dark" {
		t.Errorf("Load theme: got %q", cfg.ThemeOr("default"))
	}
	if cfg.DefaultLayoutOr("") != "triple" {
		t.Errorf("Load default_layout: got %q", cfg.DefaultLayoutOr(""))
	}
}

// TestSetUITheme_EmptyFile verifies that an empty existing file results in a
// minimal config containing only ui.theme.
func TestSetUITheme_EmptyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(""), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := SetUITheme(path, "light"); err != nil {
		t.Fatalf("SetUITheme: %v", err)
	}

	cfg, _, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ThemeOr("default") != "light" {
		t.Errorf("got %q, want light", cfg.ThemeOr("default"))
	}
}
