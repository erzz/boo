package state

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestWriteAtomic_LeavesNoTempFiles pins the temp-rename contract: on success
// no .boo-*.tmp files survive in the directory.
func TestWriteAtomic_LeavesNoTempFiles(t *testing.T) {
	dir := t.TempDir()
	if err := WriteAtomic(filepath.Join(dir, "f.txt"), []byte("hello")); err != nil {
		t.Fatalf("write: %v", err)
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.Name() != "f.txt" {
			t.Fatalf("unexpected file left behind: %s", e.Name())
		}
	}
}

// TestReadOrEmpty_MissingReturnsNilNil pins the contract: a missing file is
// not an error; callers distinguish "absent" from "empty" via the nil return.
func TestReadOrEmpty_MissingReturnsNilNil(t *testing.T) {
	b, err := ReadOrEmpty(filepath.Join(t.TempDir(), "nope"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b != nil {
		t.Fatalf("expected nil, got %q", b)
	}
}

// TestForRoot_PathsRootedUnderConfigDir pins the XDG-consolidation contract:
// every path returned by ForRoot is inside the root directory.
func TestForRoot_PathsRootedUnderConfigDir(t *testing.T) {
	root := t.TempDir()
	p := ForRoot(root)
	for _, path := range []string{
		p.ConfigFile,
		p.LayoutsDir,
		p.ThemesDir,
		p.Registry,
		p.ProjectsDir,
		p.LockPath(),
		p.ProjectDir("myproject"),
		p.ProjectLayoutFile("myproject"),
		p.ProjectStateFile("myproject"),
	} {
		if !strings.HasPrefix(path, root) {
			t.Errorf("path %q not under root %q", path, root)
		}
	}
}

// TestEnsureDirs_CreatesDirTree verifies all three required dirs are created
// and that a second call does not error (idempotent).
func TestEnsureDirs_CreatesDirTree(t *testing.T) {
	p := ForRoot(t.TempDir())
	for i := range 2 {
		if err := p.EnsureDirs(); err != nil {
			t.Fatalf("EnsureDirs call %d: %v", i+1, err)
		}
	}
	for _, d := range []string{p.ConfigDir, p.LayoutsDir, p.ProjectsDir} {
		st, err := os.Stat(d)
		if err != nil {
			t.Errorf("%s missing: %v", d, err)
			continue
		}
		if !st.IsDir() {
			t.Errorf("%s is not a directory", d)
		}
	}
}
