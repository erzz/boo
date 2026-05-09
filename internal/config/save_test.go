package config

import (
	"os"
	"path/filepath"
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
