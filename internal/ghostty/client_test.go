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
		Command:          "nvim .",
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
	if !strings.Contains(got, `"command":"nvim ."`) {
		t.Fatalf("stdin missing command: %s", got)
	}
	if !strings.Contains(got, `"FOO":"bar"`) {
		t.Fatalf("stdin missing env: %s", got)
	}
}
