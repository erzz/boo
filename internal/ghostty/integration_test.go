//go:build integration
// +build integration

// Integration tests for the Ghostty client. These hit a real Ghostty install
// via osascript. Run with: make test-int
//
// Requirements:
//   - macOS
//   - Ghostty installed AND running
//   - Automation permission granted to the test runner's terminal for Ghostty
//
// These tests will open and close real Ghostty windows.
package ghostty

import (
	"context"
	"runtime"
	"testing"
	"time"

	booexec "github.com/erzz/boo/internal/exec"
)

func skipIfNotMac(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "darwin" {
		t.Skip("ghostty integration tests require macOS")
	}
}

func newRealClient() *Client { return New(booexec.NewReal()) }

func TestIntegration_Version(t *testing.T) {
	skipIfNotMac(t)
	guardAgainstSelfTermination(t)
	c := newRealClient()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	v, err := c.Version(ctx)
	if err != nil {
		t.Fatalf("Version: %v (is Ghostty running?)", err)
	}
	if v == "" {
		t.Fatal("expected non-empty version")
	}
	t.Logf("Ghostty version: %s", v)
}

func TestIntegration_ProbeAutomation(t *testing.T) {
	skipIfNotMac(t)
	guardAgainstSelfTermination(t)
	c := newRealClient()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.ProbeAutomation(ctx); err != nil {
		t.Fatalf("ProbeAutomation: %v (is Automation permission granted?)", err)
	}
}

func TestIntegration_OpenWindow(t *testing.T) {
	skipIfNotMac(t)
	guardAgainstSelfTermination(t)
	c := newRealClient()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.OpenWindow(ctx, OpenWindowParams{
		WorkingDirectory: "/tmp",
	})
	if err != nil {
		t.Fatalf("OpenWindow: %v", err)
	}
	if res.WindowID == "" {
		t.Fatal("expected non-empty windowId")
	}
	t.Logf("opened window: %s (close manually if it lingers)", res.WindowID)
}

// TestIntegration_OpenLayout_MultiTabWithSplits exercises the Phase 2 layout
// pipeline end-to-end: a window with two tabs, one of which has a split.
// Cleanup closes the window so we don't leave windows around for the user.
func TestIntegration_OpenLayout_MultiTabWithSplits(t *testing.T) {
	skipIfNotMac(t)
	guardAgainstSelfTermination(t)
	c := newRealClient()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	res, err := c.OpenLayout(ctx, OpenLayoutParams{
		Tabs: []LayoutTab{
			{
				Name: "edit",
				Root: LayoutSplit{WorkingDirectory: "/tmp"},
			},
			{
				Name: "run",
				Root: LayoutSplit{
					Direction: "row",
					Children: []LayoutSplit{
						{WorkingDirectory: "/tmp"},
						{WorkingDirectory: "/tmp"},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("OpenLayout: %v", err)
	}
	if res.WindowID == "" {
		t.Fatal("expected non-empty windowId")
	}
	t.Logf("opened layout window %s", res.WindowID)
	t.Cleanup(func() { _ = c.CloseWindow(ctx, res.WindowID) })

	exists, err := c.WindowExists(ctx, res.WindowID)
	if err != nil {
		t.Fatalf("WindowExists: %v", err)
	}
	if !exists {
		t.Fatalf("expected window %s to exist", res.WindowID)
	}
}
// TestIntegration_OpenFocusCloseRoundTrip exercises the full lifecycle: open
// a window, verify it exists, focus it, close it, verify it no longer exists.
func TestIntegration_OpenFocusCloseRoundTrip(t *testing.T) {
	skipIfNotMac(t)
	guardAgainstSelfTermination(t)
	c := newRealClient()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	res, err := c.OpenWindow(ctx, OpenWindowParams{WorkingDirectory: "/tmp"})
	if err != nil {
		t.Fatalf("OpenWindow: %v", err)
	}
	id := res.WindowID
	t.Logf("opened window %s", id)

	// Schedule cleanup before any further failures.
	t.Cleanup(func() {
		_ = c.CloseWindow(ctx, id)
	})

	exists, err := c.WindowExists(ctx, id)
	if err != nil {
		t.Fatalf("WindowExists: %v", err)
	}
	if !exists {
		t.Fatalf("expected window %s to exist after open", id)
	}

	if err := c.FocusWindow(ctx, id); err != nil {
		t.Fatalf("FocusWindow: %v", err)
	}

	if err := c.CloseWindow(ctx, id); err != nil {
		t.Fatalf("CloseWindow: %v", err)
	}
	exists, err = c.WindowExists(ctx, id)
	if err != nil {
		t.Fatalf("WindowExists post-close: %v", err)
	}
	if exists {
		t.Fatalf("expected window %s to be gone after close", id)
	}
}
