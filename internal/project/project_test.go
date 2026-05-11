package project

import (
	"errors"
	"testing"
	"time"

	"github.com/erzz/boo/internal/layout"
	"github.com/erzz/boo/internal/state"
)

func TestValidateName(t *testing.T) {
	good := []string{"projA", "my-app", "my_app", "a", "a.b.c", "Web2026"}
	bad := []string{"", "-leading", ".dot", "with space", "very" + longString(70), "doctor", "list", "../escape"}
	for _, n := range good {
		if err := ValidateName(n); err != nil {
			t.Errorf("expected %q to be valid: %v", n, err)
		}
	}
	for _, n := range bad {
		if err := ValidateName(n); err == nil {
			t.Errorf("expected %q to be invalid", n)
		}
	}
}

func longString(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = 'a'
	}
	return string(b)
}

func TestRegistry_AddGetRemove_RoundTrip(t *testing.T) {
	root := t.TempDir()
	p := state.ForRoot(root)
	if err := p.EnsureDirs(); err != nil {
		t.Fatal(err)
	}

	r, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Projects) != 0 {
		t.Fatalf("expected empty registry, got %d", len(r.Projects))
	}

	now := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	if err := r.Add(Project{Name: "alpha", Dir: "/x/alpha", Layout: "default", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := r.Add(Project{Name: "beta", Dir: "/x/beta", Layout: "default", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := r.Add(Project{Name: "alpha", Dir: "/dup"}); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("expected ErrAlreadyExists, got %v", err)
	}
	if err := r.Save(p); err != nil {
		t.Fatal(err)
	}

	r2, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(r2.Projects) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(r2.Projects))
	}
	got, err := r2.Get("alpha")
	if err != nil || got.Dir != "/x/alpha" {
		t.Fatalf("Get alpha: %+v %v", got, err)
	}
	if _, err := r2.Get("missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	if err := r2.Remove("alpha"); err != nil {
		t.Fatal(err)
	}
	if r2.Has("alpha") {
		t.Fatal("expected alpha to be removed")
	}
}

// TestRegistry_FindByDir: boo (no args) uses FindByDir to detect the current project from $PWD.
// Wrong answer here silently re-registers an existing project or opens the picker instead.
func TestRegistry_FindByDir(t *testing.T) {
	r := &Registry{Projects: []Project{
		{Name: "alpha", Dir: "/x/alpha"},
		{Name: "beta", Dir: "/x/beta"},
	}}
	p, err := r.FindByDir("/x/beta")
	if err != nil || p.Name != "beta" {
		t.Fatalf("FindByDir(/x/beta): %+v %v", p, err)
	}
	if _, err := r.FindByDir("/no-such"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for missing dir, got %v", err)
	}
}

// TestRegistry_Update: boo set-layout rewrites a project's metadata in place via Update.
// If Update silently no-ops or duplicates the entry, the layout change is lost.
func TestRegistry_Update(t *testing.T) {
	r := &Registry{}
	if err := r.Add(Project{Name: "alpha", Dir: "/x/alpha", Layout: "triple"}); err != nil {
		t.Fatal(err)
	}
	if err := r.Update(Project{Name: "alpha", Dir: "/x/alpha", Layout: "1x1x1"}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	p, _ := r.Get("alpha")
	if p.Layout != "1x1x1" {
		t.Fatalf("Update did not apply: got layout %q", p.Layout)
	}
	if len(r.Projects) != 1 {
		t.Fatalf("Update must replace in place (no duplication): got %d projects", len(r.Projects))
	}
	if err := r.Update(Project{Name: "missing"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for missing project, got %v", err)
	}
}

func TestSaveLoadLayoutAndRuntime(t *testing.T) {
	root := t.TempDir()
	p := state.ForRoot(root)
	if err := p.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	l := layout.Default()
	l.Name = "for-alpha"
	if err := SaveLayout(p, "alpha", l); err != nil {
		t.Fatal(err)
	}
	got, err := LoadLayout(p, "alpha")
	if err != nil {
		t.Fatalf("LoadLayout: %v", err)
	}
	if got.Name != "for-alpha" {
		t.Fatalf("expected for-alpha, got %s", got.Name)
	}

	rt := Runtime{WindowID: "tab-group-deadbeef", LastLaunchedAt: time.Now().UTC().Truncate(time.Second)}
	if err := SaveRuntime(p, "alpha", rt); err != nil {
		t.Fatal(err)
	}
	got2, err := LoadRuntime(p, "alpha")
	if err != nil {
		t.Fatal(err)
	}
	if got2.WindowID != rt.WindowID {
		t.Fatalf("WindowID round-trip: got %q", got2.WindowID)
	}
}

func TestLoadRuntime_MissingIsZero(t *testing.T) {
	root := t.TempDir()
	p := state.ForRoot(root)
	_ = p.EnsureDirs()
	rt, err := LoadRuntime(p, "no-such")
	if err != nil {
		t.Fatal(err)
	}
	if rt.WindowID != "" {
		t.Fatalf("expected zero, got %+v", rt)
	}
}
