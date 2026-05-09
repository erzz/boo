package cli

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/erzz/boo/internal/state"
)

// brokenConfigRoot sets up a temp BOO_HOME with an intentionally malformed
// config.yaml, returning the resolved Paths for assertions.
func brokenConfigRoot(t *testing.T) state.Paths {
	t.Helper()
	root := t.TempDir()
	t.Setenv("BOO_HOME", root)
	p := state.ForRoot(root)
	if err := p.EnsureDirs(); err != nil {
		t.Fatalf("EnsureDirs: %v", err)
	}
	// Write a syntactically-broken config.yaml so newApp() would fail.
	if err := os.WriteFile(p.ConfigFile, []byte("default_layout: [unclosed\n"), 0o644); err != nil {
		t.Fatalf("write broken config: %v", err)
	}
	return p
}

// TestConfigPath_WorksWithBrokenConfig ensures 'boo config path' succeeds
// even when the config file contains invalid YAML.  The point of the command
// is to help users find the file they need to fix — failing early would make
// recovery impossible.
func TestConfigPath_WorksWithBrokenConfig(t *testing.T) {
	p := brokenConfigRoot(t)

	cmd := newConfigPathCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("config path with broken config: %v", err)
	}
	got := strings.TrimSpace(buf.String())
	if got != p.ConfigFile {
		t.Errorf("config path = %q, want %q", got, p.ConfigFile)
	}
}

// TestConfigEdit_WorksWithBrokenConfig ensures 'boo config edit' opens the
// editor even when the config file is malformed.  Uses the shell built-in
// "true" as the editor so no real process launches during the test.
func TestConfigEdit_WorksWithBrokenConfig(t *testing.T) {
	brokenConfigRoot(t)

	// "true" is a POSIX command that exits 0 immediately — a safe no-op
	// stand-in for $EDITOR in unit tests.
	t.Setenv("EDITOR", "true")
	t.Setenv("VISUAL", "") // ensure EDITOR wins

	cmd := newConfigEditCmd()
	if err := cmd.Execute(); err != nil {
		t.Fatalf("config edit with broken config: %v", err)
	}
}

// TestConfigPath_MissingConfigFileStillWorks verifies the path command works
// even when no config.yaml exists yet (the common first-run case).
func TestConfigPath_MissingConfigFileStillWorks(t *testing.T) {
	root := t.TempDir()
	t.Setenv("BOO_HOME", root)
	p := state.ForRoot(root)
	// Deliberately do NOT create or EnsureDirs — config file does not exist.
	// The path command must still return the would-be path without error.

	cmd := newConfigPathCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("config path with no config file: %v", err)
	}
	got := strings.TrimSpace(buf.String())
	if got != p.ConfigFile {
		t.Errorf("config path = %q, want %q", got, p.ConfigFile)
	}
}
