package git

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	booexec "github.com/erzz/boo/internal/exec"
)

func TestRepoNameFromURL(t *testing.T) {
	cases := []struct {
		url, want string
		wantErr   bool
	}{
		{"https://github.com/erzz/boo", "boo", false},
		{"https://github.com/erzz/boo.git", "boo", false},
		{"https://github.com/erzz/boo/", "boo", false},
		{"git@github.com:erzz/boo.git", "boo", false},
		{"git@github.com:erzz/boo", "boo", false},
		{"ssh://git@github.com/erzz/boo.git", "boo", false},
		{"", "", true},
		{"   ", "", true},
		{"justastring", "", true},
	}
	for _, c := range cases {
		got, err := repoNameFromURL(c.url)
		if c.wantErr {
			if err == nil {
				t.Errorf("repoNameFromURL(%q): expected error, got %q", c.url, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("repoNameFromURL(%q): %v", c.url, err)
			continue
		}
		if got != c.want {
			t.Errorf("repoNameFromURL(%q) = %q, want %q", c.url, got, c.want)
		}
	}
}

func TestDeriveDestination(t *testing.T) {
	parent := t.TempDir()
	got, err := DeriveDestination(parent, "https://github.com/erzz/boo.git")
	if err != nil {
		t.Fatalf("DeriveDestination: %v", err)
	}
	want := filepath.Join(parent, "boo")
	if got != want {
		t.Fatalf("DeriveDestination = %q, want %q", got, want)
	}
}

func TestDeriveDestination_DefaultsToCwd(t *testing.T) {
	dir := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })

	got, err := DeriveDestination("", "git@github.com:erzz/boo.git")
	if err != nil {
		t.Fatalf("DeriveDestination: %v", err)
	}
	// On macOS /tmp resolves through /private/tmp; both are valid roots.
	if !strings.HasSuffix(got, filepath.Join("boo")) {
		t.Fatalf("DeriveDestination = %q, want suffix 'boo'", got)
	}
	if !filepath.IsAbs(got) {
		t.Fatalf("expected absolute path, got %q", got)
	}
}

func TestClone_PassesUrlAndDestToGit(t *testing.T) {
	parent := t.TempDir()
	dest := filepath.Join(parent, "newrepo")

	var capturedArgs []string
	fake := booexec.NewFake(func(name string, args []string, _ []byte) ([]byte, []byte, error) {
		if name != "git" {
			t.Errorf("expected git, got %s", name)
		}
		capturedArgs = args
		// Pretend git created the dir.
		if err := os.MkdirAll(dest, 0o755); err != nil {
			return nil, nil, err
		}
		return nil, nil, nil
	})
	c := New(fake)

	got, err := c.Clone(context.Background(), "https://example.com/foo.git", dest)
	if err != nil {
		t.Fatalf("Clone: %v", err)
	}
	if got != dest {
		t.Fatalf("Clone returned %q, want %q", got, dest)
	}
	want := []string{"clone", "--", "https://example.com/foo.git", dest}
	if len(capturedArgs) != len(want) {
		t.Fatalf("args = %v, want %v", capturedArgs, want)
	}
	for i, a := range want {
		if capturedArgs[i] != a {
			t.Fatalf("args[%d] = %q, want %q", i, capturedArgs[i], a)
		}
	}
}

func TestClone_RefusesNonEmptyDestination(t *testing.T) {
	parent := t.TempDir()
	dest := filepath.Join(parent, "exists")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dest, "junk"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	fake := booexec.NewFake(func(_ string, _ []string, _ []byte) ([]byte, []byte, error) {
		t.Fatal("git should not be invoked when dest is non-empty")
		return nil, nil, nil
	})
	c := New(fake)
	_, err := c.Clone(context.Background(), "https://example.com/x.git", dest)
	if err == nil || !strings.Contains(err.Error(), "not empty") {
		t.Fatalf("expected non-empty destination error, got %v", err)
	}
}

func TestClone_AcceptsEmptyDestination(t *testing.T) {
	parent := t.TempDir()
	dest := filepath.Join(parent, "empty")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	fake := booexec.NewFake(func(_ string, _ []string, _ []byte) ([]byte, []byte, error) {
		return nil, nil, nil
	})
	if _, err := New(fake).Clone(context.Background(), "https://example.com/x.git", dest); err != nil {
		t.Fatalf("Clone into empty dir: %v", err)
	}
}

func TestClone_RefusesFileDestination(t *testing.T) {
	parent := t.TempDir()
	dest := filepath.Join(parent, "afile")
	if err := os.WriteFile(dest, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	fake := booexec.NewFake(func(_ string, _ []string, _ []byte) ([]byte, []byte, error) {
		t.Fatal("git should not be invoked")
		return nil, nil, nil
	})
	_, err := New(fake).Clone(context.Background(), "https://example.com/x.git", dest)
	if err == nil || !strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("expected not-a-directory error, got %v", err)
	}
}

func TestClone_RequiresUrlAndDest(t *testing.T) {
	c := New(booexec.NewFake(nil))
	if _, err := c.Clone(context.Background(), "", "/tmp/x"); err == nil {
		t.Error("expected error for empty url")
	}
	if _, err := c.Clone(context.Background(), "https://x", ""); err == nil {
		t.Error("expected error for empty dest")
	}
}

func TestClone_PropagatesGitError(t *testing.T) {
	parent := t.TempDir()
	dest := filepath.Join(parent, "x")
	fake := booexec.NewFake(func(_ string, _ []string, _ []byte) ([]byte, []byte, error) {
		return nil, []byte("fatal: repository not found"), errors.New("exit 128")
	})
	_, err := New(fake).Clone(context.Background(), "https://example.com/missing.git", dest)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "repository not found") {
		t.Fatalf("expected stderr in error, got %v", err)
	}
}
