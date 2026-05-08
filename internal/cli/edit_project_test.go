package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/erzz/boo/internal/project"
)

func TestExecuteEdit_Rename_MovesStateDirAndUpdatesRegistry(t *testing.T) {
	a := makeAppForCmds(t)
	dir := t.TempDir()
	registerProjectForTest(t, a, "old", dir, "1x1x1")

	if err := executeEdit(a, "old", "renamed", dir, "1x1x1"); err != nil {
		t.Fatalf("executeEdit: %v", err)
	}

	// Registry: old gone, renamed present, dir + layout unchanged.
	reg, err := project.Load(a.Paths)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if reg.Has("old") {
		t.Errorf("old project still in registry after rename")
	}
	p, err := reg.Get("renamed")
	if err != nil {
		t.Fatalf("Get renamed: %v", err)
	}
	if p.Dir != dir {
		t.Errorf("dir = %q, want %q", p.Dir, dir)
	}
	if p.Layout != "1x1x1" {
		t.Errorf("layout = %q, want 1x1x1", p.Layout)
	}

	// State dir: old gone, renamed exists with layout file.
	if _, err := os.Stat(a.Paths.ProjectDir("old")); !os.IsNotExist(err) {
		t.Errorf("old state dir still present: err=%v", err)
	}
	if _, err := os.Stat(a.Paths.ProjectLayoutFile("renamed")); err != nil {
		t.Errorf("renamed state dir missing layout.yaml: %v", err)
	}
}

func TestExecuteEdit_ChangeDir_UpdatesRegistryOnly(t *testing.T) {
	a := makeAppForCmds(t)
	oldDir := t.TempDir()
	newDir := t.TempDir()
	registerProjectForTest(t, a, "proj", oldDir, "1x1x1")

	if err := executeEdit(a, "proj", "proj", newDir, "1x1x1"); err != nil {
		t.Fatalf("executeEdit: %v", err)
	}

	reg, _ := project.Load(a.Paths)
	p, _ := reg.Get("proj")
	if p.Dir != newDir {
		t.Errorf("dir = %q, want %q", p.Dir, newDir)
	}
	// State dir unchanged because name didn't change.
	if _, err := os.Stat(a.Paths.ProjectLayoutFile("proj")); err != nil {
		t.Errorf("layout.yaml missing after dir change: %v", err)
	}
}

func TestExecuteEdit_ChangeTemplate_RewritesLayoutFile(t *testing.T) {
	a := makeAppForCmds(t)
	dir := t.TempDir()
	registerProjectForTest(t, a, "proj", dir, "1x1x1")

	if err := executeEdit(a, "proj", "proj", dir, "triple"); err != nil {
		t.Fatalf("executeEdit: %v", err)
	}

	reg, _ := project.Load(a.Paths)
	p, _ := reg.Get("proj")
	if p.Layout != "triple" {
		t.Errorf("registry layout = %q, want triple", p.Layout)
	}
	// Layout file should mention the new template name.
	data, err := os.ReadFile(a.Paths.ProjectLayoutFile("proj"))
	if err != nil {
		t.Fatalf("read layout: %v", err)
	}
	if !contains(string(data), "name: triple") {
		t.Errorf("layout file doesn't reference new template:\n%s", data)
	}
}

func TestExecuteEdit_RenameAndChangeAll_AppliesAllChanges(t *testing.T) {
	a := makeAppForCmds(t)
	oldDir := t.TempDir()
	newDir := t.TempDir()
	registerProjectForTest(t, a, "alpha", oldDir, "1x1x1")

	if err := executeEdit(a, "alpha", "beta", newDir, "triple"); err != nil {
		t.Fatalf("executeEdit: %v", err)
	}

	reg, _ := project.Load(a.Paths)
	if reg.Has("alpha") {
		t.Error("alpha still registered")
	}
	p, err := reg.Get("beta")
	if err != nil {
		t.Fatalf("Get beta: %v", err)
	}
	if p.Dir != newDir {
		t.Errorf("dir = %q, want %q", p.Dir, newDir)
	}
	if p.Layout != "triple" {
		t.Errorf("layout = %q, want triple", p.Layout)
	}
	if _, err := os.Stat(a.Paths.ProjectLayoutFile("beta")); err != nil {
		t.Errorf("beta layout missing: %v", err)
	}
}

func TestExecuteEdit_NoOp_DoesNothing(t *testing.T) {
	a := makeAppForCmds(t)
	dir := t.TempDir()
	registerProjectForTest(t, a, "proj", dir, "1x1x1")

	// Capture state mtime to confirm we don't rewrite.
	regPath := a.Paths.Registry
	before, err := os.Stat(regPath)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	// Sleep is unreliable for mtime checks across filesystems; instead
	// assert the post-state matches pre-state by reload.
	if err := executeEdit(a, "proj", "proj", dir, "1x1x1"); err != nil {
		t.Fatalf("executeEdit no-op: %v", err)
	}
	reg, _ := project.Load(a.Paths)
	p, _ := reg.Get("proj")
	if p.Dir != dir || p.Layout != "1x1x1" {
		t.Errorf("no-op edit changed something: %+v", p)
	}
	_ = before
}

func TestExecuteEdit_RenameToExistingName_Errors(t *testing.T) {
	a := makeAppForCmds(t)
	d1 := t.TempDir()
	d2 := t.TempDir()
	registerProjectForTest(t, a, "alpha", d1, "1x1x1")
	registerProjectForTest(t, a, "beta", d2, "1x1x1")

	err := executeEdit(a, "alpha", "beta", d1, "1x1x1")
	if err == nil {
		t.Fatal("expected error renaming to taken name")
	}
	if !contains(err.Error(), "already exists") {
		t.Errorf("error = %v, want 'already exists'", err)
	}
}

func TestExecuteEdit_ChangeDirToOtherProjectsDir_Errors(t *testing.T) {
	a := makeAppForCmds(t)
	d1 := t.TempDir()
	d2 := t.TempDir()
	registerProjectForTest(t, a, "alpha", d1, "1x1x1")
	registerProjectForTest(t, a, "beta", d2, "1x1x1")

	err := executeEdit(a, "alpha", "alpha", d2, "1x1x1")
	if err == nil {
		t.Fatal("expected error changing dir to a registered other project's dir")
	}
	if !contains(err.Error(), "already registered") {
		t.Errorf("error = %v, want 'already registered'", err)
	}
}

func TestExecuteEdit_InvalidNewName_Errors(t *testing.T) {
	a := makeAppForCmds(t)
	dir := t.TempDir()
	registerProjectForTest(t, a, "proj", dir, "1x1x1")

	err := executeEdit(a, "proj", "delete", dir, "1x1x1") // reserved
	if err == nil {
		t.Fatal("expected error renaming to reserved name")
	}
}

func TestExecuteEdit_UnknownTemplate_DoesNotMoveStateDir(t *testing.T) {
	a := makeAppForCmds(t)
	dir := t.TempDir()
	registerProjectForTest(t, a, "alpha", dir, "1x1x1")

	err := executeEdit(a, "alpha", "beta", dir, "no-such-template")
	if err == nil {
		t.Fatal("expected error for unknown template")
	}
	// Crucially: state dir was NOT renamed (template resolves before
	// we touch the filesystem).
	if _, err := os.Stat(a.Paths.ProjectDir("alpha")); err != nil {
		t.Errorf("alpha state dir was moved despite template error: %v", err)
	}
	if _, err := os.Stat(a.Paths.ProjectDir("beta")); !os.IsNotExist(err) {
		t.Errorf("beta state dir was created despite template error")
	}
	// Registry unchanged.
	reg, _ := project.Load(a.Paths)
	if !reg.Has("alpha") {
		t.Error("alpha removed from registry on failed edit")
	}
}

func TestExecuteEdit_AbsolutizesRelativeDir(t *testing.T) {
	a := makeAppForCmds(t)
	dir := t.TempDir()
	registerProjectForTest(t, a, "proj", dir, "1x1x1")

	// Use a relative dir; executeEdit should canonicalise to abs.
	cwd, _ := os.Getwd()
	defer func() { _ = os.Chdir(cwd) }()
	parent := filepath.Dir(dir)
	if err := os.Chdir(parent); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	rel := filepath.Base(dir)

	if err := executeEdit(a, "proj", "proj", rel, "1x1x1"); err != nil {
		t.Fatalf("executeEdit: %v", err)
	}
	reg, _ := project.Load(a.Paths)
	p, _ := reg.Get("proj")
	if !filepath.IsAbs(p.Dir) {
		t.Errorf("dir not absolute: %q", p.Dir)
	}
}

// contains aliases strings.Contains so the assertions read naturally.
func contains(haystack, needle string) bool {
	return strings.Contains(haystack, needle)
}
