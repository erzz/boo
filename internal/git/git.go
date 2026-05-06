// Package git is the only place in boo that shells out to `git`.
//
// Currently we only need clone for `boo new --from <url>`. Like internal/exec
// elsewhere in boo, every git invocation goes through a Runner so the call
// can be unit-tested without hitting the network.
package git

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	booexec "github.com/erzz/boo/internal/exec"
)

// Cloner clones git repositories.
type Cloner struct {
	runner booexec.Runner
}

// New returns a Cloner backed by the given Runner.
func New(runner booexec.Runner) *Cloner { return &Cloner{runner: runner} }

// Clone runs `git clone <url> <dest>`. dest must not already exist as a
// non-empty directory; we refuse to clone into populated paths so we never
// accidentally mix unrelated content. The parent directory of dest is
// created if missing.
//
// On success, the absolute, cleaned dest path is returned.
func (c *Cloner) Clone(ctx context.Context, url, dest string) (string, error) {
	if strings.TrimSpace(url) == "" {
		return "", fmt.Errorf("git clone: url is required")
	}
	if strings.TrimSpace(dest) == "" {
		return "", fmt.Errorf("git clone: destination is required")
	}
	abs, err := filepath.Abs(dest)
	if err != nil {
		return "", fmt.Errorf("git clone: resolve dest %q: %w", dest, err)
	}
	abs = filepath.Clean(abs)

	if err := assertDestUsable(abs); err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return "", fmt.Errorf("git clone: mkdir parent of %s: %w", abs, err)
	}

	_, stderr, err := c.runner.Run(ctx, "git", "clone", "--", url, abs)
	if err != nil {
		return "", fmt.Errorf("git clone %s: %w (stderr: %s)", url, err, strings.TrimSpace(string(stderr)))
	}
	return abs, nil
}

// DeriveDestination produces a sensible default destination path for a clone
// when the user didn't pass --into. The directory is named after the
// repository (last URL segment, minus a trailing .git) and placed under
// parentDir. If parentDir is empty, the current working directory is used.
//
// Returns an absolute path. Does not create anything on disk.
func DeriveDestination(parentDir, url string) (string, error) {
	repo, err := repoNameFromURL(url)
	if err != nil {
		return "", err
	}
	if parentDir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("git: resolve cwd: %w", err)
		}
		parentDir = cwd
	}
	abs, err := filepath.Abs(filepath.Join(parentDir, repo))
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}

// repoNameFromURL extracts the trailing repository name from a clone URL.
// Handles https://host/org/repo, https://host/org/repo.git, and SSH-style
// git@host:org/repo(.git).
func repoNameFromURL(url string) (string, error) {
	url = strings.TrimSpace(url)
	if url == "" {
		return "", fmt.Errorf("git: empty url")
	}
	// Trim trailing slashes and .git so the suffix isn't mistaken for the name.
	trimmed := strings.TrimRight(url, "/")
	// Extract the last path-ish segment. Both "/" and ":" can separate the
	// repo name from the rest in the URL forms we support.
	sepIdx := strings.LastIndexAny(trimmed, "/:")
	if sepIdx == -1 {
		return "", fmt.Errorf("git: cannot derive repo name from url %q", url)
	}
	name := trimmed[sepIdx+1:]
	name = strings.TrimSuffix(name, ".git")
	if name == "" {
		return "", fmt.Errorf("git: empty repo name derived from url %q", url)
	}
	return name, nil
}

// assertDestUsable returns nil if dest does not exist, or if it exists as an
// empty directory. Otherwise it returns an error explaining why we won't
// clone into it.
func assertDestUsable(dest string) error {
	st, err := os.Stat(dest)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("git clone: stat %s: %w", dest, err)
	}
	if !st.IsDir() {
		return fmt.Errorf("git clone: destination %s exists and is not a directory", dest)
	}
	entries, err := os.ReadDir(dest)
	if err != nil {
		return fmt.Errorf("git clone: read %s: %w", dest, err)
	}
	if len(entries) > 0 {
		return fmt.Errorf("git clone: destination %s already exists and is not empty", dest)
	}
	return nil
}
