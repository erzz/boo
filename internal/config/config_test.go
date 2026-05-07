package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", p, err)
	}
	return p
}

func TestLoad_MissingFileReturnsFactoryDefaults(t *testing.T) {
	cfg, src, err := Load(filepath.Join(t.TempDir(), "absent.yaml"))
	if err != nil {
		t.Fatalf("missing file should not error: %v", err)
	}
	if got := cfg.DefaultLayoutOr(""); got != "triple" {
		t.Errorf("DefaultLayout = %q, want 'triple'", got)
	}
	if got := cfg.ThemeOr(""); got != "default" {
		t.Errorf("Theme = %q, want 'default'", got)
	}
	for _, key := range []string{"default_layout", "projects_dir", "git.default_remote", "ui.theme"} {
		if src[key] != "factory" {
			t.Errorf("Sources[%q] = %q, want 'factory'", key, src[key])
		}
	}
}

func TestLoad_MalformedFileErrors(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "config.yaml", "default_layout: [not, a, string\n")
	_, _, err := Load(p)
	if err == nil {
		t.Fatal("expected parse error for malformed YAML")
	}
}

func TestLoad_OverridesFactoryDefaults(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "config.yaml", `
default_layout: 1x2x1
projects_dir: /tmp/projects
git:
  default_remote: https://github.com/erzz
ui:
  theme: dark
`)
	cfg, src, err := Load(p)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got := cfg.DefaultLayoutOr(""); got != "1x2x1" {
		t.Errorf("DefaultLayout = %q, want '1x2x1'", got)
	}
	if got := cfg.ProjectsDirOr(""); got != "/tmp/projects" {
		t.Errorf("ProjectsDir = %q, want '/tmp/projects'", got)
	}
	if got := cfg.GitDefaultRemoteOr(""); got != "https://github.com/erzz" {
		t.Errorf("GitDefaultRemote = %q, want 'https://github.com/erzz'", got)
	}
	if got := cfg.ThemeOr(""); got != "dark" {
		t.Errorf("Theme = %q, want 'dark'", got)
	}
	for _, key := range []string{"default_layout", "projects_dir", "git.default_remote", "ui.theme"} {
		if src[key] != p {
			t.Errorf("Sources[%q] = %q, want %q", key, src[key], p)
		}
	}
}

func TestLoad_PartialFileKeepsFactoryDefaultsForUnsetKeys(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "config.yaml", "default_layout: 2x1x1\n")
	cfg, src, err := Load(p)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.DefaultLayoutOr("") != "2x1x1" {
		t.Error("default_layout from file should win")
	}
	if cfg.ThemeOr("") != "default" {
		t.Error("theme should keep factory default when not in file")
	}
	if src["default_layout"] != p {
		t.Error("default_layout source should be the file")
	}
	if src["ui.theme"] != "factory" {
		t.Errorf("ui.theme source should be 'factory', got %q", src["ui.theme"])
	}
}

func TestLoad_TildeExpansionInProjectsDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := t.TempDir()
	p := writeFile(t, dir, "config.yaml", "projects_dir: ~/code\n")
	cfg, _, err := Load(p)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	want := filepath.Join(home, "code")
	if got := cfg.ProjectsDirOr(""); got != want {
		t.Errorf("ProjectsDir = %q, want %q (tilde should expand to $HOME)", got, want)
	}
}

func TestLoad_EmptyFileIsValid(t *testing.T) {
	// An empty config.yaml is a perfectly reasonable thing for a
	// user to have ("I want a config file but no overrides yet").
	// Must not error and must yield factory defaults.
	dir := t.TempDir()
	p := writeFile(t, dir, "config.yaml", "")
	cfg, src, err := Load(p)
	if err != nil {
		t.Fatalf("empty file should not error: %v", err)
	}
	if cfg.DefaultLayoutOr("") != "triple" {
		t.Error("empty file should keep factory default for default_layout")
	}
	if src["default_layout"] != "factory" {
		t.Error("empty file should leave default_layout source as 'factory'")
	}
}
