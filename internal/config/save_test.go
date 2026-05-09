package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetUITheme_CreatesFileWhenAbsent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

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

func TestSetUITheme_PreservesOtherKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

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

func TestSetUITheme_OverwritesExistingTheme(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("ui:\n  theme: default\n"), 0o644); err != nil {
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

func TestSetUITheme_EmptyPath(t *testing.T) {
	if err := SetUITheme("", "default"); err == nil {
		t.Fatal("expected error for empty path")
	}
}

// TestSetUITheme_PreservesComments is the comment-preservation round-trip test.
// It verifies that comments, key ordering, and all non-theme fields survive a
// SetUITheme call exactly as authored.
func TestSetUITheme_PreservesComments(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

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

	// Theme value must be updated.
	if !strings.Contains(gotStr, "theme: tokyonight") {
		t.Errorf("theme not updated; file:\n%s", gotStr)
	}

	// All comments must be preserved.
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

	// Other keys and values must be preserved.
	if !strings.Contains(gotStr, "show_status: true") {
		t.Errorf("show_status lost; file:\n%s", gotStr)
	}
	if !strings.Contains(gotStr, "default_layout: triple") {
		t.Errorf("default_layout lost; file:\n%s", gotStr)
	}

	// default_layout must appear after ui (key ordering preserved).
	uiIdx := strings.Index(gotStr, "ui:")
	dlIdx := strings.Index(gotStr, "default_layout:")
	if uiIdx < 0 || dlIdx < 0 || dlIdx < uiIdx {
		t.Errorf("key ordering wrong — ui should precede default_layout; file:\n%s", gotStr)
	}
}

// TestSetUITheme_NoUISection verifies that when the config has no "ui:" key,
// SetUITheme adds one without disturbing other top-level keys or their comments.
func TestSetUITheme_NoUISection(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

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

	// Theme must be set.
	if !strings.Contains(gotStr, "theme: solarized-dark") {
		t.Errorf("theme not found; file:\n%s", gotStr)
	}
	// Existing keys and comments must survive.
	if !strings.Contains(gotStr, "default_layout: triple") {
		t.Errorf("default_layout lost; file:\n%s", gotStr)
	}
	if !strings.Contains(gotStr, "# boo configuration") {
		t.Errorf("top-level comment lost; file:\n%s", gotStr)
	}

	// Confirm via Load that the values parse correctly.
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

// TestSetUITheme_EmptyFile verifies that an empty file results in a minimal
// config file containing only ui.theme.
func TestSetUITheme_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
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
