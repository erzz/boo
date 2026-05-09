package state

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteAtomic_CreatesAndOverwrites(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")

	if err := WriteAtomic(path, []byte("hello")); err != nil {
		t.Fatalf("first write: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != "hello" {
		t.Fatalf("expected hello, got %q", got)
	}

	if err := WriteAtomic(path, []byte("world")); err != nil {
		t.Fatalf("second write: %v", err)
	}
	got, _ = os.ReadFile(path)
	if string(got) != "world" {
		t.Fatalf("expected world, got %q", got)
	}

	// No leftover temp files in the directory.
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.Name() != "f.txt" {
			t.Fatalf("unexpected file left behind: %s", e.Name())
		}
	}
}

func TestReadOrEmpty_MissingIsNilNil(t *testing.T) {
	dir := t.TempDir()
	b, err := ReadOrEmpty(filepath.Join(dir, "nope"))
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if b != nil {
		t.Fatalf("expected nil bytes, got %q", b)
	}
}

func TestForRoot_AndEnsureDirs(t *testing.T) {
	root := t.TempDir()
	p := ForRoot(root)
	if err := p.EnsureDirs(); err != nil {
		t.Fatalf("EnsureDirs: %v", err)
	}
	for _, d := range []string{p.ConfigDir, p.LayoutsDir, p.ProjectsDir} {
		if st, err := os.Stat(d); err != nil || !st.IsDir() {
			t.Fatalf("expected dir %s to exist: %v", d, err)
		}
	}
}
