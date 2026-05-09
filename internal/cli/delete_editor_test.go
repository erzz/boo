package cli

import (
	"context"
	"errors"
	"strings"
	"testing"

	booexec "github.com/erzz/boo/internal/exec"
	"github.com/erzz/boo/internal/ghostty"
	"github.com/erzz/boo/internal/project"
	"github.com/erzz/boo/internal/state"
)

// ---------------------------------------------------------------------------
// Fix 2: Surface purge-delete close failures
// ---------------------------------------------------------------------------

// makeAppWithFailingClose builds a test app whose Ghostty client returns an
// error for every osascript call, simulating a CloseWindow failure.
func makeAppWithFailingClose(t *testing.T) *app {
	t.Helper()
	dir := t.TempDir()
	p := state.ForRoot(dir)
	if err := p.EnsureDirs(); err != nil {
		t.Fatalf("EnsureDirs: %v", err)
	}
	fake := booexec.NewFake(func(name string, _ []string, _ []byte) ([]byte, []byte, error) {
		if name == "osascript" {
			return nil, []byte("automation permission denied"), errors.New("exit status 1")
		}
		return nil, nil, nil
	})
	return &app{
		Paths:   p,
		Ghostty: ghostty.New(fake),
	}
}

// TestExecuteDelete_PurgeWindowCloseFailure_WarningIsReturned verifies that
// when CloseWindow fails during a purge delete:
//   - The deletion itself still succeeds (project removed from registry).
//   - A non-empty warnings slice is returned instead of an error.
//   - The first warning mentions the window ID.
func TestExecuteDelete_PurgeWindowCloseFailure_WarningIsReturned(t *testing.T) {
	a := makeAppWithFailingClose(t)
	dir := t.TempDir()
	registerProjectForTest(t, a, "proj", dir, "triple")

	// Write a runtime file so CloseWindow has a window ID to close.
	if err := project.SaveRuntime(a.Paths, "proj", project.Runtime{WindowID: "w-deadbeef"}); err != nil {
		t.Fatalf("SaveRuntime: %v", err)
	}

	var warns []string
	err := a.Paths.WithLock(func() error {
		reg, err := project.Load(a.Paths)
		if err != nil {
			return err
		}
		p, err := reg.Get("proj")
		if err != nil {
			return err
		}
		var werr error
		warns, werr = executeDelete(context.Background(), a, reg, p, true /*purge*/)
		return werr
	})

	if err != nil {
		t.Fatalf("executeDelete returned error %v; want success (deletion must not be blocked by close failure)", err)
	}
	if len(warns) == 0 {
		t.Fatal("expected non-empty warnings when CloseWindow fails, got empty slice")
	}
	if !strings.Contains(warns[0], "w-deadbeef") {
		t.Errorf("warning %q should mention the window ID %q", warns[0], "w-deadbeef")
	}

	// Project must be gone from the registry.
	reg, err := project.Load(a.Paths)
	if err != nil {
		t.Fatalf("Load after delete: %v", err)
	}
	if _, err := reg.Get("proj"); err == nil {
		t.Error("project still in registry after successful executeDelete")
	}
}

// TestExecuteDelete_NoPurge_NoWarning verifies that without --purge no
// warning is returned (no attempt to close the window is made).
func TestExecuteDelete_NoPurge_NoWarning(t *testing.T) {
	// Even with a failing Ghostty, purge=false should produce no warnings.
	a := makeAppWithFailingClose(t)
	dir := t.TempDir()
	registerProjectForTest(t, a, "proj", dir, "triple")

	var warns []string
	err := a.Paths.WithLock(func() error {
		reg, err := project.Load(a.Paths)
		if err != nil {
			return err
		}
		p, err := reg.Get("proj")
		if err != nil {
			return err
		}
		var werr error
		warns, werr = executeDelete(context.Background(), a, reg, p, false /*purge*/)
		return werr
	})

	if err != nil {
		t.Fatalf("executeDelete: %v", err)
	}
	if len(warns) != 0 {
		t.Errorf("expected empty warnings without --purge, got %v", warns)
	}
}

// TestExecuteDelete_PurgeWindowCloseSuccess_NoWarning verifies the happy path:
// when CloseWindow succeeds the warnings slice is empty.
func TestExecuteDelete_PurgeWindowCloseSuccess_NoWarning(t *testing.T) {
	a := makeAppForCmds(t) // default fake returns success for osascript
	dir := t.TempDir()
	registerProjectForTest(t, a, "proj", dir, "triple")

	if err := project.SaveRuntime(a.Paths, "proj", project.Runtime{WindowID: "w-ok"}); err != nil {
		t.Fatalf("SaveRuntime: %v", err)
	}

	// The CloseWindow JXA script expects a valid JSON response from the
	// fake runner, so we need a smarter fake here.
	fake := booexec.NewFake(func(_ string, _ []string, _ []byte) ([]byte, []byte, error) {
		return []byte(`{"ok":true}`), nil, nil
	})
	a.Ghostty = ghostty.New(fake)

	var warns []string
	err := a.Paths.WithLock(func() error {
		reg, err := project.Load(a.Paths)
		if err != nil {
			return err
		}
		p, err := reg.Get("proj")
		if err != nil {
			return err
		}
		var werr error
		warns, werr = executeDelete(context.Background(), a, reg, p, true /*purge*/)
		return werr
	})

	if err != nil {
		t.Fatalf("executeDelete: %v", err)
	}
	if len(warns) != 0 {
		t.Errorf("expected empty warnings when close succeeds, got %v", warns)
	}
}

// ---------------------------------------------------------------------------
// Fix 3: Multi-word $EDITOR tokeniser
// ---------------------------------------------------------------------------

func TestShsplit(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    []string
		wantErr bool
	}{
		{
			name:  "plain command",
			input: "nvim",
			want:  []string{"nvim"},
		},
		{
			name:  "code --wait",
			input: "code --wait",
			want:  []string{"code", "--wait"},
		},
		{
			name:  "emacs -nw -Q",
			input: "emacs -nw -Q",
			want:  []string{"emacs", "-nw", "-Q"},
		},
		{
			name:  "leading and trailing spaces",
			input: "  nvim  ",
			want:  []string{"nvim"},
		},
		{
			name:  "single-quoted arg",
			input: `nvim '-u' NONE`,
			want:  []string{"nvim", "-u", "NONE"},
		},
		{
			name:  "double-quoted arg with spaces",
			input: `code "--wait" "--new-window"`,
			want:  []string{"code", "--wait", "--new-window"},
		},
		{
			name:  "single-quoted value with spaces inside",
			input: `my-ed 'arg with spaces'`,
			want:  []string{"my-ed", "arg with spaces"},
		},
		{
			name:  "double-quoted value with spaces inside",
			input: `my-ed "arg with spaces"`,
			want:  []string{"my-ed", "arg with spaces"},
		},
		{
			name:    "empty string",
			input:   "",
			want:    nil,
			wantErr: false, // shsplit returns nil,nil for empty; splitEditorCommand errors
		},
		{
			name:    "whitespace only",
			input:   "   \t  ",
			want:    nil,
			wantErr: false,
		},
		{
			name:    "backtick rejected",
			input:   "nvim `echo foo`",
			wantErr: true,
		},
		{
			name:    "unterminated single quote",
			input:   "nvim 'oops",
			wantErr: true,
		},
		{
			name:    "unterminated double quote",
			input:   `nvim "oops`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := shsplit(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("shsplit(%q) = %v, want error", tt.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("shsplit(%q) error = %v", tt.input, err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("shsplit(%q) = %v (len %d), want %v (len %d)", tt.input, got, len(got), tt.want, len(tt.want))
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("shsplit(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestSplitEditorCommand(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantCmd  string
		wantArgs []string
		wantErr  bool
	}{
		{
			name:     "nvim",
			input:    "nvim",
			wantCmd:  "nvim",
			wantArgs: []string{},
		},
		{
			name:     "code --wait",
			input:    "code --wait",
			wantCmd:  "code",
			wantArgs: []string{"--wait"},
		},
		{
			name:     "emacs -nw -Q",
			input:    "emacs -nw -Q",
			wantCmd:  "emacs",
			wantArgs: []string{"-nw", "-Q"},
		},
		{
			name:    "empty string returns error",
			input:   "",
			wantErr: true,
		},
		{
			name:    "whitespace-only returns error",
			input:   "  \t  ",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, args, err := splitEditorCommand(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("splitEditorCommand(%q) = %q, %v, want error", tt.input, cmd, args)
				}
				return
			}
			if err != nil {
				t.Fatalf("splitEditorCommand(%q) error = %v", tt.input, err)
			}
			if cmd != tt.wantCmd {
				t.Errorf("cmd = %q, want %q", cmd, tt.wantCmd)
			}
			if len(args) != len(tt.wantArgs) {
				t.Fatalf("args = %v (len %d), want %v (len %d)", args, len(args), tt.wantArgs, len(tt.wantArgs))
			}
			for i := range args {
				if args[i] != tt.wantArgs[i] {
					t.Errorf("args[%d] = %q, want %q", i, args[i], tt.wantArgs[i])
				}
			}
		})
	}
}

// TestBuildEditorCmd_NoEditorSet verifies that buildEditorCmd returns a
// useful error when neither override nor $EDITOR nor $VISUAL is set.
func TestBuildEditorCmd_NoEditorSet(t *testing.T) {
	t.Setenv("EDITOR", "")
	t.Setenv("VISUAL", "")
	_, err := buildEditorCmd("", "/tmp/foo.yaml")
	if err == nil {
		t.Fatal("expected error when no editor is configured")
	}
	if !strings.Contains(err.Error(), "$EDITOR") {
		t.Errorf("error should mention $EDITOR, got: %v", err)
	}
}

// TestBuildEditorCmd_MultiWordEditor verifies that a multi-word editor
// string (e.g. "code --wait") is correctly split so the resulting *exec.Cmd
// has the editor binary as Path and flags as separate Args entries — not
// a single string with spaces in the binary name.
func TestBuildEditorCmd_MultiWordEditor(t *testing.T) {
	t.Setenv("EDITOR", "code --wait")
	t.Setenv("VISUAL", "")

	cmd, err := buildEditorCmd("", "/tmp/foo.yaml")
	if err != nil {
		t.Fatalf("buildEditorCmd: %v", err)
	}
	// cmd.Args[0] is conventionally the program name, cmd.Args[1:] are
	// the arguments. We want ["code", "--wait", "/tmp/foo.yaml"].
	if len(cmd.Args) < 3 {
		t.Fatalf("cmd.Args = %v, want at least 3 entries", cmd.Args)
	}
	if !strings.HasSuffix(cmd.Args[0], "code") {
		t.Errorf("cmd.Args[0] = %q, want path ending in 'code'", cmd.Args[0])
	}
	if cmd.Args[1] != "--wait" {
		t.Errorf("cmd.Args[1] = %q, want \"--wait\"", cmd.Args[1])
	}
	if cmd.Args[2] != "/tmp/foo.yaml" {
		t.Errorf("cmd.Args[2] = %q, want \"/tmp/foo.yaml\"", cmd.Args[2])
	}
}

// TestBuildEditorCmd_OverrideTakesPrecedence verifies the resolution order:
// an explicit editorOverride beats $EDITOR.
func TestBuildEditorCmd_OverrideTakesPrecedence(t *testing.T) {
	t.Setenv("EDITOR", "vi")
	t.Setenv("VISUAL", "")

	cmd, err := buildEditorCmd("nano", "/tmp/foo.yaml")
	if err != nil {
		t.Fatalf("buildEditorCmd: %v", err)
	}
	if !strings.HasSuffix(cmd.Args[0], "nano") {
		t.Errorf("expected nano from override, got %q", cmd.Args[0])
	}
}
