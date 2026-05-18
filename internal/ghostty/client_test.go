package ghostty

import (
	"context"
	"strings"
	"testing"

	booexec "github.com/erzz/boo/internal/exec"
)

func TestVersion_ParsesJSON(t *testing.T) {
	fake := booexec.NewFake(func(name string, args []string, _ []byte) ([]byte, []byte, error) {
		if name != "osascript" {
			t.Fatalf("expected osascript, got %s", name)
		}
		if len(args) < 2 || args[0] != "-l" || args[1] != "JavaScript" {
			t.Fatalf("expected -l JavaScript, got %v", args[:2])
		}
		return []byte(`{"version":"1.3.1"}`), nil, nil
	})
	c := New(fake)
	v, err := c.Version(context.Background())
	if err != nil {
		t.Fatalf("Version: %v", err)
	}
	if v != "1.3.1" {
		t.Fatalf("expected 1.3.1, got %q", v)
	}
}

func TestVersion_PropagatesScriptError(t *testing.T) {
	fake := booexec.NewFake(func(_ string, _ []string, _ []byte) ([]byte, []byte, error) {
		return []byte(`{"error":"Application can't be found"}`), nil, nil
	})
	c := New(fake)
	_, err := c.Version(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "Application can't be found") {
		t.Fatalf("expected propagated error, got %v", err)
	}
}

func TestOpenWindow_PassesParamsAsStdinJSON(t *testing.T) {
	var seenStdin []byte
	fake := booexec.NewFake(func(_ string, _ []string, stdin []byte) ([]byte, []byte, error) {
		seenStdin = stdin
		return []byte(`{"windowId":"abc-123"}`), nil, nil
	})
	c := New(fake)
	res, err := c.OpenWindow(context.Background(), OpenWindowParams{
		WorkingDirectory: "/tmp/projA",
		InitialInput:     "nvim .\n",
		Env:              map[string]string{"FOO": "bar"},
	})
	if err != nil {
		t.Fatalf("OpenWindow: %v", err)
	}
	if res.WindowID != "abc-123" {
		t.Fatalf("expected windowId abc-123, got %q", res.WindowID)
	}
	got := string(seenStdin)
	if !strings.Contains(got, `"workingDirectory":"/tmp/projA"`) {
		t.Fatalf("stdin missing workingDirectory: %s", got)
	}
	if !strings.Contains(got, `"initialInput":"nvim .\n"`) {
		t.Fatalf("stdin missing initialInput: %s", got)
	}
	if !strings.Contains(got, `"FOO":"bar"`) {
		t.Fatalf("stdin missing env: %s", got)
	}
}

func TestOpenLayout_PassesLayoutAsStdinJSON(t *testing.T) {
	var seenStdin []byte
	fake := booexec.NewFake(func(_ string, _ []string, stdin []byte) ([]byte, []byte, error) {
		seenStdin = stdin
		return []byte(`{"windowId":"win-9"}`), nil, nil
	})
	c := New(fake)
	res, err := c.OpenLayout(context.Background(), OpenLayoutParams{
		Tabs: []LayoutTab{
			{
				Name: "edit",
				// Single-leaf tab: root is itself a leaf.
				Root: LayoutSplit{WorkingDirectory: "/projA", InitialInput: "nvim .\n"},
			},
			{
				Name: "run",
				// Two-pane row: root is interior with two leaves.
				Root: LayoutSplit{
					Direction: "row",
					Children: []LayoutSplit{
						{WorkingDirectory: "/projA"},
						{WorkingDirectory: "/projA", InitialInput: "npm run dev\n"},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("OpenLayout: %v", err)
	}
	if res.WindowID != "win-9" {
		t.Fatalf("windowId: got %q", res.WindowID)
	}
	got := string(seenStdin)
	for _, want := range []string{
		`"name":"edit"`,
		`"name":"run"`,
		`"direction":"row"`,
		`"children":[`,
		`"initialInput":"npm run dev\n"`,
		`"workingDirectory":"/projA"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("stdin missing %s\nfull: %s", want, got)
		}
	}
}

func TestOpenLayout_RejectsEmptyTabs(t *testing.T) {
	fake := booexec.NewFake(func(_ string, _ []string, _ []byte) ([]byte, []byte, error) {
		t.Fatal("runner should not be invoked for empty tabs")
		return nil, nil, nil
	})
	c := New(fake)
	if _, err := c.OpenLayout(context.Background(), OpenLayoutParams{}); err == nil {
		t.Fatal("expected error for empty tabs")
	}
}

func TestOpenLayout_PropagatesScriptError(t *testing.T) {
	fake := booexec.NewFake(func(_ string, _ []string, _ []byte) ([]byte, []byte, error) {
		return []byte(`{"error":"interior node must have exactly 2 children"}`), nil, nil
	})
	c := New(fake)
	_, err := c.OpenLayout(context.Background(), OpenLayoutParams{
		Tabs: []LayoutTab{{Root: LayoutSplit{}}},
	})
	if err == nil || !strings.Contains(err.Error(), "exactly 2 children") {
		t.Fatalf("expected propagated error, got %v", err)
	}
}

// TestOpenLayoutScript_UsesPerformActionNotPerform pins the JXA spelling for
// the "perform action" sdef command. AppleScript's "perform action" maps to
// JXA's `performAction(...)`. Using `app.perform(...)` silently dispatches
// to a generic Cocoa selector that returns "Message not understood." at
// runtime — symptom: dividers don't move and the failure is swallowed by
// flushResizes's best-effort catch. Regression guard.
func TestOpenLayoutScript_UsesPerformActionNotPerform(t *testing.T) {
	if strings.Contains(openLayoutScript, "app.perform(") {
		t.Fatal("open_layout.js uses `app.perform(`; must be `app.performAction(` — the sdef command is `perform action`, which JXA exposes as performAction. Using `perform` triggers a silent `Message not understood` from Cocoa and dividers never move.")
	}
	if !strings.Contains(openLayoutScript, "app.performAction(") {
		t.Fatal("open_layout.js no longer calls `app.performAction(`; the resize_split pass is dead code.")
	}
}
