// Package ghostty is the only place in boo that talks to Ghostty.
//
// All interaction is via JXA (JavaScript for Automation) scripts run through
// `osascript -l JavaScript`. Cold-start (Ghostty not running) uses
// `open -na Ghostty.app`. See SPIKE.md for the rationale.
package ghostty

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	booexec "github.com/erzz/boo/internal/exec"
)

//go:embed jxa/version.js
var versionScript string

//go:embed jxa/open_window.js
var openWindowScript string

//go:embed jxa/probe_automation.js
var probeAutomationScript string

//go:embed jxa/window_exists.js
var windowExistsScript string

//go:embed jxa/focus_window.js
var focusWindowScript string

//go:embed jxa/close_window.js
var closeWindowScript string

//go:embed jxa/open_layout.js
var openLayoutScript string

// Client controls a running Ghostty instance.
type Client struct {
	runner booexec.Runner
}

// New returns a Client backed by the given Runner.
func New(runner booexec.Runner) *Client {
	return &Client{runner: runner}
}

// Version returns the running Ghostty's reported version, or an error if
// Ghostty isn't running / responsive.
func (c *Client) Version(ctx context.Context) (string, error) {
	stdout, stderr, err := c.run(ctx, versionScript, nil)
	if err != nil {
		return "", fmt.Errorf("ghostty version: %w (stderr: %s)", err, stderr)
	}
	var out struct {
		Version string `json:"version"`
		Error   string `json:"error,omitempty"`
	}
	if jerr := json.Unmarshal(stdout, &out); jerr != nil {
		return "", fmt.Errorf("ghostty version: parse %q: %w", stdout, jerr)
	}
	if out.Error != "" {
		return "", errors.New(out.Error)
	}
	if out.Version == "" {
		return "", fmt.Errorf("ghostty version: empty version in response %q", stdout)
	}
	return out.Version, nil
}

// OpenWindowParams is the input for OpenWindow. Kept intentionally small for
// the Phase 0 proof-of-life; real layout support will land in Phase 1+.
type OpenWindowParams struct {
	WorkingDirectory string            `json:"workingDirectory,omitempty"`
	Command          string            `json:"command,omitempty"`
	Env              map[string]string `json:"env,omitempty"`
}

// OpenWindowResult is what OpenWindow returns on success.
type OpenWindowResult struct {
	WindowID string `json:"windowId"`
}

// OpenWindow opens a new Ghostty window with the given surface configuration
// and returns its stable (process-lifetime) window ID.
func (c *Client) OpenWindow(ctx context.Context, p OpenWindowParams) (*OpenWindowResult, error) {
	stdin, err := json.Marshal(p)
	if err != nil {
		return nil, fmt.Errorf("ghostty open window: encode params: %w", err)
	}
	stdout, stderr, err := c.run(ctx, openWindowScript, stdin)
	if err != nil {
		return nil, fmt.Errorf("ghostty open window: %w (stderr: %s)", err, stderr)
	}
	var out struct {
		WindowID string `json:"windowId"`
		Error    string `json:"error,omitempty"`
	}
	if jerr := json.Unmarshal(stdout, &out); jerr != nil {
		return nil, fmt.Errorf("ghostty open window: parse %q: %w", stdout, jerr)
	}
	if out.Error != "" {
		return nil, errors.New(out.Error)
	}
	if out.WindowID == "" {
		return nil, fmt.Errorf("ghostty open window: empty windowId in response %q", stdout)
	}
	return &OpenWindowResult{WindowID: out.WindowID}, nil
}

// ProbeAutomation triggers a write-equivalent action against Ghostty (counting
// windows) so callers can detect missing macOS Automation permission. Returns
// nil if Ghostty answered, or the underlying error otherwise.
func (c *Client) ProbeAutomation(ctx context.Context) error {
	stdout, stderr, err := c.run(ctx, probeAutomationScript, nil)
	if err != nil {
		return fmt.Errorf("ghostty probe: %w (stderr: %s)", err, stderr)
	}
	var out struct {
		OK    bool   `json:"ok"`
		Error string `json:"error,omitempty"`
	}
	if jerr := json.Unmarshal(stdout, &out); jerr != nil {
		return fmt.Errorf("ghostty probe: parse %q: %w", stdout, jerr)
	}
	if out.Error != "" {
		return errors.New(out.Error)
	}
	if !out.OK {
		return fmt.Errorf("ghostty probe: unexpected response %q", stdout)
	}
	return nil
}

// WindowExists reports whether the given (process-lifetime) window ID is
// still a live Ghostty window.
func (c *Client) WindowExists(ctx context.Context, windowID string) (bool, error) {
	if windowID == "" {
		return false, nil
	}
	stdin, _ := json.Marshal(map[string]string{"windowId": windowID})
	stdout, stderr, err := c.run(ctx, windowExistsScript, stdin)
	if err != nil {
		return false, fmt.Errorf("ghostty window exists: %w (stderr: %s)", err, stderr)
	}
	var out struct {
		Exists bool   `json:"exists"`
		Error  string `json:"error,omitempty"`
	}
	if jerr := json.Unmarshal(stdout, &out); jerr != nil {
		return false, fmt.Errorf("ghostty window exists: parse %q: %w", stdout, jerr)
	}
	if out.Error != "" {
		return false, errors.New(out.Error)
	}
	return out.Exists, nil
}

// FocusWindow activates the window with the given ID, bringing it to the
// front. Returns an error if the window no longer exists.
func (c *Client) FocusWindow(ctx context.Context, windowID string) error {
	if windowID == "" {
		return errors.New("ghostty focus window: empty windowId")
	}
	stdin, _ := json.Marshal(map[string]string{"windowId": windowID})
	stdout, stderr, err := c.run(ctx, focusWindowScript, stdin)
	if err != nil {
		return fmt.Errorf("ghostty focus window: %w (stderr: %s)", err, stderr)
	}
	var out struct {
		OK    bool   `json:"ok"`
		Error string `json:"error,omitempty"`
	}
	if jerr := json.Unmarshal(stdout, &out); jerr != nil {
		return fmt.Errorf("ghostty focus window: parse %q: %w", stdout, jerr)
	}
	if out.Error != "" {
		return errors.New(out.Error)
	}
	if !out.OK {
		return fmt.Errorf("ghostty focus window: unexpected response %q", stdout)
	}
	return nil
}

// LayoutSplit is one terminal surface in a layout being rendered.
//
// Direction is empty for the primary split of each tab and one of
// "right"|"left"|"up"|"down" for subsequent splits. WorkingDirectory is the
// already-resolved absolute path; layout-relative resolution happens in the
// caller.
type LayoutSplit struct {
	Direction        string            `json:"direction,omitempty"`
	WorkingDirectory string            `json:"workingDirectory,omitempty"`
	Command          string            `json:"command,omitempty"`
	InitialInput     string            `json:"initialInput,omitempty"`
	Env              map[string]string `json:"env,omitempty"`
}

// LayoutTab is one tab in a layout being rendered.
type LayoutTab struct {
	Name   string        `json:"name,omitempty"`
	Splits []LayoutSplit `json:"splits"`
}

// OpenLayoutParams is the input for OpenLayout. Tabs is rendered left-to-right
// with the first split of the first tab seeding the new window.
type OpenLayoutParams struct {
	Tabs []LayoutTab `json:"tabs"`
}

// OpenLayout opens a new Ghostty window and renders the given multi-tab,
// multi-split layout into it. Returns the new window's stable ID.
//
// On any failure after the window has been created, the JXA helper attempts
// to close the partially-built window so we never leak state visible to the
// user.
func (c *Client) OpenLayout(ctx context.Context, p OpenLayoutParams) (*OpenWindowResult, error) {
	if len(p.Tabs) == 0 {
		return nil, errors.New("ghostty open layout: layout has no tabs")
	}
	stdin, err := json.Marshal(p)
	if err != nil {
		return nil, fmt.Errorf("ghostty open layout: encode params: %w", err)
	}
	stdout, stderr, err := c.run(ctx, openLayoutScript, stdin)
	if err != nil {
		return nil, fmt.Errorf("ghostty open layout: %w (stderr: %s)", err, stderr)
	}
	var out struct {
		WindowID string `json:"windowId"`
		Error    string `json:"error,omitempty"`
	}
	if jerr := json.Unmarshal(stdout, &out); jerr != nil {
		return nil, fmt.Errorf("ghostty open layout: parse %q: %w", stdout, jerr)
	}
	if out.Error != "" {
		return nil, errors.New(out.Error)
	}
	if out.WindowID == "" {
		return nil, fmt.Errorf("ghostty open layout: empty windowId in response %q", stdout)
	}
	return &OpenWindowResult{WindowID: out.WindowID}, nil
}

// CloseWindow closes the window with the given ID. A window that no longer
// exists is treated as success (idempotent).
func (c *Client) CloseWindow(ctx context.Context, windowID string) error {
	if windowID == "" {
		return nil
	}
	stdin, _ := json.Marshal(map[string]string{"windowId": windowID})
	stdout, stderr, err := c.run(ctx, closeWindowScript, stdin)
	if err != nil {
		return fmt.Errorf("ghostty close window: %w (stderr: %s)", err, stderr)
	}
	var out struct {
		OK    bool   `json:"ok"`
		Error string `json:"error,omitempty"`
	}
	if jerr := json.Unmarshal(stdout, &out); jerr != nil {
		return fmt.Errorf("ghostty close window: parse %q: %w", stdout, jerr)
	}
	if out.Error != "" {
		return errors.New(out.Error)
	}
	return nil
}

// IsNotRunning reports whether err looks like "Ghostty isn't running" /
// "Application can't be found". Used by callers to decide whether to retry
// after EnsureRunning.
func IsNotRunning(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "isn't running") ||
		strings.Contains(msg, "Application can't be found")
}

// EnsureRunning launches Ghostty if it isn't already running and waits until
// it responds to AppleScript (or the context expires).
//
// Idempotent: if Ghostty is already up, returns immediately after a quick probe.
func (c *Client) EnsureRunning(ctx context.Context) error {
	// Fast path: already responsive.
	if _, err := c.Version(ctx); err == nil {
		return nil
	} else if !IsNotRunning(err) {
		// Some other error (e.g. permissions); surface it.
		return err
	}

	// Cold-start. `open -na` always launches a new instance even if one is
	// hidden; we've already established no instance is responsive, so this is
	// safe.
	if _, stderr, err := c.runner.Run(ctx, "open", "-na", "Ghostty.app"); err != nil {
		return fmt.Errorf("ghostty cold-start: %w (stderr: %s)", err, stderr)
	}

	// Poll Version() until success or context expires.
	deadline := time.Now().Add(5 * time.Second)
	if dl, ok := ctx.Deadline(); ok && dl.Before(deadline) {
		deadline = dl
	}
	for {
		if _, err := c.Version(ctx); err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return errors.New("ghostty cold-start: timed out waiting for Ghostty to respond")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(150 * time.Millisecond):
		}
	}
}

// run executes a JXA script via osascript. The script reads its parameters
// (if any) from stdin as JSON and writes a single JSON object to stdout.
func (c *Client) run(ctx context.Context, script string, stdin []byte) ([]byte, []byte, error) {
	args := []string{"-l", "JavaScript"}
	if stdin == nil {
		// Pass the script directly via -e; no stdin needed.
		args = append(args, "-e", script)
		return c.runner.Run(ctx, "osascript", args...)
	}
	// With stdin, we still pass the script via -e — stdin is reserved for params.
	args = append(args, "-e", script)
	return c.runner.RunWithStdin(ctx, stdin, "osascript", args...)
}
