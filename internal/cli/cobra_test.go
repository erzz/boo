package cli

// cobra_test.go exercises the cobra wiring layer — flag parsing,
// RunE dispatch, and the interaction between cobra flags and the
// internal helper functions.  These tests sit at the boundary between
// the cobra command tree and the app helpers; they complement the
// direct-helper tests elsewhere in the package.
//
// Strategy: each command under test is constructed via its *WithApp /
// *WithRunner variant so a fake *app can be injected.  This keeps
// state isolated without BOO_HOME pollution while still exercising:
//   - flag registration (the flags must exist on the cobra.Command)
//   - flag parsing (cobra parses the args slice)
//   - the RunE path through to internal helpers
//
// The tests are deliberately narrow: they verify the cobra path produces
// the right side-effects or errors, not the full business logic (which
// is covered by the direct-helper tests).

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	booexec "github.com/erzz/boo/internal/exec"
	"github.com/erzz/boo/internal/git"
	"github.com/erzz/boo/internal/ghostty"
	"github.com/erzz/boo/internal/project"
	"github.com/erzz/boo/internal/state"
)

// ─── helper ───────────────────────────────────────────────────────────────────

// executeCobraCmd wires stdout/stderr capture onto cmd, sets the given args,
// and executes it with a background context.  Returns the captured output
// and any error returned by Execute.
func executeCobraCmd(t *testing.T, cmd *cobra.Command, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	var outBuf, errBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	cmd.SetErr(&errBuf)
	cmd.SetArgs(args)
	err = cmd.ExecuteContext(context.Background())
	return outBuf.String(), errBuf.String(), err
}

// makeAppWithFakeGit returns a test *app whose Git cloner is backed by the
// given respond function.  Every git clone call will be intercepted.
// The Ghostty client is wired to a do-nothing fake so any incidental
// Ghostty calls don't panic.
func makeAppWithFakeGit(t *testing.T, respond func(name string, args []string, stdin []byte) ([]byte, []byte, error)) *app {
	t.Helper()
	a := makeAppForCmds(t)
	fakeRunner := booexec.NewFake(respond)
	a.Git = git.New(fakeRunner)
	return a
}

// ─── boo config path ──────────────────────────────────────────────────────────

// TestCobra_ConfigPath_PrintsConfigFilePath verifies the cobra flag-parsing
// and RunE for `boo config path`.  The command must not call newApp() so
// it succeeds even when config is absent.
func TestCobra_ConfigPath_PrintsConfigFilePath(t *testing.T) {
	root := t.TempDir()
	t.Setenv("BOO_HOME", root)

	// Use the real NewRoot — config path doesn't need app injection.
	cmd := NewRoot("test", "none", "test")
	stdout, _, err := executeCobraCmd(t, cmd, "config", "path")
	if err != nil {
		t.Fatalf("boo config path: unexpected error: %v", err)
	}
	// Output must contain the expected config file path.
	wantSubstr := filepath.Join("config", "config.yaml")
	if !strings.Contains(stdout, wantSubstr) {
		t.Errorf("stdout %q does not contain %q", stdout, wantSubstr)
	}
}

// TestCobra_ConfigPath_SucceedsWithBrokenConfig is the regression test for
// "boo config path must not fail when config.yaml is malformed."  The command
// deliberately bypasses newApp() / config.Load so users can find and fix the
// file path even when parsing fails.
func TestCobra_ConfigPath_SucceedsWithBrokenConfig(t *testing.T) {
	root := t.TempDir()
	t.Setenv("BOO_HOME", root)

	p := state.ForRoot(root)
	if err := os.MkdirAll(p.ConfigDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(p.ConfigFile, []byte("default_layout: [unclosed\n"), 0o644); err != nil {
		t.Fatalf("write broken config: %v", err)
	}

	cmd := NewRoot("test", "none", "test")
	_, _, err := executeCobraCmd(t, cmd, "config", "path")
	if err != nil {
		t.Errorf("boo config path should succeed with broken config, got: %v", err)
	}
}

// TestCobra_ConfigEdit_SucceedsWithBrokenConfig is the regression test for
// "boo config edit must not fail when config.yaml is malformed."  The fix
// was to make the command bypass newApp() and use state.Default() directly.
// EDITOR is set to "true" (a POSIX utility that exits 0 without any I/O) so
// the test doesn't open a real editor.
func TestCobra_ConfigEdit_SucceedsWithBrokenConfig(t *testing.T) {
	root := t.TempDir()
	t.Setenv("BOO_HOME", root)

	p := state.ForRoot(root)
	if err := os.MkdirAll(p.ConfigDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(p.ConfigFile, []byte("default_layout: [unclosed\n"), 0o644); err != nil {
		t.Fatalf("write broken config: %v", err)
	}

	// `true` is a POSIX utility that exits 0 without touching its arguments.
	t.Setenv("EDITOR", "true")
	t.Setenv("VISUAL", "")

	cmd := NewRoot("test", "none", "test")
	_, _, err := executeCobraCmd(t, cmd, "config", "edit")
	if err != nil {
		t.Errorf("boo config edit should succeed even with broken config, got: %v", err)
	}
}

// ─── boo new ──────────────────────────────────────────────────────────────────

// TestCobra_New_FromFlag_CloneDestinationNotCwd is the regression test for
// "boo new --from <url> was cloning into cwd instead of a subdirectory".
//
// The bug: when --from is set and --dir/--into are absent, buildNewProjectDefaults
// used to set Dir to cwd.  The fix derives Dir from the URL.
//
// We inject a fake git runner that records what git clone was called with
// and returns an error (no network needed).  The test verifies:
//  1. The clone was attempted (not an earlier validation error).
//  2. The clone destination is NOT cwd.
//  3. The clone destination ends with the repo name from the URL.
func TestCobra_New_FromFlag_CloneDestinationNotCwd(t *testing.T) {
	cwd, _ := os.Getwd()

	var clonedInto string
	a := makeAppWithFakeGit(t, func(name string, args []string, _ []byte) ([]byte, []byte, error) {
		if name == "git" && len(args) >= 1 && args[0] == "clone" {
			// git clone -- <url> <dest>; last arg is the destination.
			clonedInto = args[len(args)-1]
			return nil, nil, errors.New("clone failed in test — expected")
		}
		return nil, nil, nil
	})

	cmd := newNewCmdWithApp(a)
	_, _, err := executeCobraCmd(t, cmd,
		"myproj",
		"--from", "https://github.com/owner/myrepo.git",
		"--yes",
	)

	// The command must fail because the fake clone fails.  But it must fail
	// at the clone step, not at an earlier validation step — which proves
	// the flag wiring actually fed --from into buildNewProjectDefaults.
	if err == nil {
		t.Fatal("expected error (fake clone always fails), got nil")
	}
	if clonedInto == "" {
		t.Fatalf("git clone was never invoked; error was: %v\n(if this is a validation error, the --from flag wiring is broken)", err)
	}

	// The clone destination must NOT equal the current working directory.
	if clonedInto == cwd {
		t.Errorf("clone was attempted into cwd %q — this is the regression bug: "+
			"--from without --dir must derive the destination from the URL, not cwd", cwd)
	}

	// The clone destination must end with the repo name derived from the URL.
	if !strings.HasSuffix(filepath.Clean(clonedInto), "myrepo") {
		t.Errorf("clone destination %q must end with repo name 'myrepo' (derived from URL)", clonedInto)
	}
}

// TestCobra_New_FromFlag_WithExplicitDir verifies that --from + --dir
// uses the explicit --dir as the clone destination rather than deriving
// from the URL.  Covers the separate code path in newNewCmdWithApp.
func TestCobra_New_FromFlag_WithExplicitDir(t *testing.T) {
	target := t.TempDir()

	var clonedInto string
	a := makeAppWithFakeGit(t, func(name string, args []string, _ []byte) ([]byte, []byte, error) {
		if name == "git" && len(args) >= 1 && args[0] == "clone" {
			clonedInto = args[len(args)-1]
			return nil, nil, errors.New("clone failed in test — expected")
		}
		return nil, nil, nil
	})

	cmd := newNewCmdWithApp(a)
	_, _, err := executeCobraCmd(t, cmd,
		"myproj",
		"--from", "https://github.com/owner/myrepo.git",
		"--dir", target,
		"--yes",
	)

	if err == nil {
		t.Fatal("expected error (fake clone always fails), got nil")
	}
	if clonedInto == "" {
		t.Fatal("git clone was never called")
	}

	// Explicit --dir must win over the URL-derived destination.
	if filepath.Clean(clonedInto) != filepath.Clean(target) {
		t.Errorf("clone destination = %q, want %q (explicit --dir must win)", clonedInto, target)
	}
}

// ─── boo delete ───────────────────────────────────────────────────────────────

// TestCobra_Delete_Force_RemovesProject tests the cobra path for
// `boo delete <name> --force`.  The project is pre-registered in the
// temp state, and we verify it's gone after the command runs.
func TestCobra_Delete_Force_RemovesProject(t *testing.T) {
	a := makeAppForCmds(t)
	dir := t.TempDir()
	registerProjectForTest(t, a, "toDelete", dir, "triple")

	cmd := newDeleteCmdWithApp(a)
	stdout, _, err := executeCobraCmd(t, cmd, "toDelete", "--force")
	if err != nil {
		t.Fatalf("boo delete --force: %v", err)
	}

	// Output must mention the project name.
	if !strings.Contains(stdout, "toDelete") {
		t.Errorf("stdout %q should mention project name", stdout)
	}

	// Project must no longer be registered.
	reg, err := project.Load(a.Paths)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if reg.Has("toDelete") {
		t.Error("project 'toDelete' still present in registry after delete")
	}
}

// TestCobra_Delete_UnknownProject_Errors verifies that deleting a project
// that isn't registered returns a clear "not found" error.
func TestCobra_Delete_UnknownProject_Errors(t *testing.T) {
	a := makeAppForCmds(t)

	cmd := newDeleteCmdWithApp(a)
	_, _, err := executeCobraCmd(t, cmd, "ghost", "--force")
	if err == nil {
		t.Fatal("expected error for unknown project, got nil")
	}
	if !strings.Contains(err.Error(), "ghost") {
		t.Errorf("error %q should mention the project name 'ghost'", err)
	}
}

// TestCobra_Delete_Purge_CallsGhosttyCloseWindow verifies that
// `boo delete <name> --purge --force` triggers a Ghostty close-window call
// when the project has a saved runtime window ID.
func TestCobra_Delete_Purge_CallsGhosttyCloseWindow(t *testing.T) {
	const wantWindowID = "win-abc-123"

	var receivedStdin []byte
	fakeRunner := booexec.NewFake(func(name string, args []string, stdin []byte) ([]byte, []byte, error) {
		if name == "osascript" && len(stdin) > 0 {
			receivedStdin = stdin
			// close_window.js expects {"ok":true}
			return []byte(`{"ok":true}`), nil, nil
		}
		return []byte(`{"ok":true}`), nil, nil
	})

	a := makeAppForCmds(t)
	a.Ghostty = ghostty.New(fakeRunner)

	dir := t.TempDir()
	registerProjectForTest(t, a, "toClose", dir, "triple")

	// Write a runtime file so the delete --purge path finds a window to close.
	rt := project.Runtime{WindowID: wantWindowID}
	if err := project.SaveRuntime(a.Paths, "toClose", rt); err != nil {
		t.Fatalf("SaveRuntime: %v", err)
	}

	cmd := newDeleteCmdWithApp(a)
	_, _, err := executeCobraCmd(t, cmd, "toClose", "--purge", "--force")
	if err != nil {
		t.Fatalf("boo delete --purge --force: %v", err)
	}

	// The fake runner must have been called with the window ID in the stdin payload.
	if !strings.Contains(string(receivedStdin), wantWindowID) {
		t.Errorf("expected close-window call with windowId %q in stdin payload; got stdin=%q", wantWindowID, receivedStdin)
	}
}

// ─── boo save ────────────────────────────────────────────────────────────────

// TestCobra_Save_UnknownProject_Errors verifies that
// `boo save <unknown>` returns a clear "not found" error through the cobra path.
func TestCobra_Save_UnknownProject_Errors(t *testing.T) {
	a := makeAppForCmds(t)

	cmd := newSaveCmdWithApp(a)
	_, _, err := executeCobraCmd(t, cmd, "nosuchproject")
	if err == nil {
		t.Fatal("expected error for unknown project, got nil")
	}
	if !strings.Contains(err.Error(), "nosuchproject") {
		t.Errorf("error %q should mention the project name 'nosuchproject'", err)
	}
}

// TestCobra_Save_KnownProjectNoLiveWindow_Errors verifies that
// `boo save <name>` returns an actionable error when the project has no
// live window recorded in its runtime state file.
func TestCobra_Save_KnownProjectNoLiveWindow_Errors(t *testing.T) {
	a := makeAppForCmds(t)
	dir := t.TempDir()
	registerProjectForTest(t, a, "proj", dir, "triple")
	// No runtime file → no WindowID → "has no live window" error.

	cmd := newSaveCmdWithApp(a)
	_, _, err := executeCobraCmd(t, cmd, "proj")
	if err == nil {
		t.Fatal("expected error (no live window), got nil")
	}
	if !strings.Contains(err.Error(), "no live window") {
		t.Errorf("error %q should mention 'no live window'", err)
	}
}

// ─── boo doctor ──────────────────────────────────────────────────────────────

// TestCobra_Doctor_Smoke verifies that the doctor command runs its full cobra
// path (flag registration, RunE, check execution, result rendering) without
// panicking.  The fake runner returns plausible Ghostty JSON responses so
// the Ghostty-specific checks that use the Runner don't produce unexpected results.
//
// checkGhosttyInstalled uses exec.LookPath (not the Runner), so it may
// return FAIL if Ghostty isn't installed in the test environment.  That's
// expected: we only assert that the command produces output and doesn't panic.
func TestCobra_Doctor_Smoke(t *testing.T) {
	root := t.TempDir()
	t.Setenv("BOO_HOME", root)

	fakeRunner := booexec.NewFake(func(name string, _ []string, _ []byte) ([]byte, []byte, error) {
		if name == "osascript" {
			// All JXA scripts return a superset response; irrelevant fields
			// are silently ignored by the individual parsers.
			return []byte(`{"version":"1.3.5","ok":true,"windowId":""}`), nil, nil
		}
		return nil, nil, nil
	})

	cmd := newDoctorCmdWithRunner(fakeRunner)
	stdout, _, _ := executeCobraCmd(t, cmd) // error acceptable (e.g. Ghostty not installed)

	// The command must render at least one check result in the [STATUS] format.
	if !strings.Contains(stdout, "[") {
		t.Errorf("doctor produced no check output; stdout = %q", stdout)
	}
}
