package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/erzz/boo/internal/config"
	booexec "github.com/erzz/boo/internal/exec"
	"github.com/erzz/boo/internal/ghostty"
	"github.com/erzz/boo/internal/git"
	"github.com/erzz/boo/internal/state"
)

// app is the shared dependency bundle for cobra commands.
//
// Constructed lazily per invocation in newApp so that command tree creation
// (used in tests / completions) doesn't touch the filesystem or external
// processes.
type app struct {
	Paths      state.Paths
	Runner     booexec.Runner
	Ghostty    *ghostty.Client
	Git        *git.Cloner
	Config     config.Config
	ConfigSrcs config.Sources
}

// newApp resolves paths and builds the default dependency set.
func newApp() (*app, error) {
	p, err := state.Default()
	if err != nil {
		return nil, err
	}
	if err := p.EnsureDirs(); err != nil {
		return nil, err
	}
	cfg, srcs, err := config.Load(p.ConfigFile)
	if err != nil {
		// A malformed config is a hard failure — silently falling back
		// to defaults would mask user typos. Missing file is fine and
		// already handled inside Load (returns factory defaults, no
		// error).
		return nil, err
	}
	r := booexec.NewReal()
	return &app{
		Paths:      p,
		Runner:     r,
		Ghostty:    ghostty.New(r),
		Git:        git.New(r),
		Config:     cfg,
		ConfigSrcs: srcs,
	}, nil
}

// resolveDir cleans and absolutises a user-supplied directory path. If empty,
// returns the current working directory.
func resolveDir(dir string) (string, error) {
	if dir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("resolve cwd: %w", err)
		}
		return cwd, nil
	}
	if !filepath.IsAbs(dir) {
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("resolve cwd: %w", err)
		}
		dir = filepath.Join(cwd, dir)
	}
	clean := filepath.Clean(dir)
	st, err := os.Stat(clean)
	if err != nil {
		return "", fmt.Errorf("directory %q: %w", clean, err)
	}
	if !st.IsDir() {
		return "", fmt.Errorf("%q is not a directory", clean)
	}
	return clean, nil
}

// resolveCloneDestination returns the absolute path a clone should target.
//
//   - If into is non-empty, it is absolutised and returned (existence and
//     emptiness are validated by the cloner itself, not here).
//   - Otherwise, if projectsDir is non-empty, the destination is
//     <projectsDir>/<repo-name> — honours the user's `projects_dir`
//     config so clones land in a consistent place regardless of the
//     directory `boo new` was run from.
//   - Otherwise the destination is derived relative to cwd:
//     <cwd>/<repo-name>, with .git stripped from the repo name.
func resolveCloneDestination(into, url, projectsDir string) (string, error) {
	if into != "" {
		abs := into
		if !filepath.IsAbs(abs) {
			cwd, err := os.Getwd()
			if err != nil {
				return "", fmt.Errorf("resolve cwd: %w", err)
			}
			abs = filepath.Join(cwd, abs)
		}
		return filepath.Clean(abs), nil
	}
	if projectsDir != "" {
		return git.DeriveDestination(projectsDir, url)
	}
	return git.DeriveDestination("", url)
}

// expandRepoShorthand turns a bare repo name into a full clone URL by
// prepending the configured default remote.
//
//   - If from already looks like a full URL (contains "://" or ":" for
//     SSH-style git@host:owner/repo) or contains a path separator,
//     it's returned unchanged.
//   - If defaultRemote is empty, from is returned unchanged.
//   - Otherwise, returns "<defaultRemote>/<from>" (with one slash,
//     trailing slashes stripped from defaultRemote).
//
// The result is intentionally not validated as a URL — the cloner
// surfaces a clear error if the resulting URL doesn't resolve.
func expandRepoShorthand(from, defaultRemote string) string {
	if from == "" || defaultRemote == "" {
		return from
	}
	// Already a full URL or SSH-style git@host:owner/repo.
	if strings.Contains(from, "://") || strings.Contains(from, ":") || strings.Contains(from, "/") {
		return from
	}
	return strings.TrimRight(defaultRemote, "/") + "/" + from
}
