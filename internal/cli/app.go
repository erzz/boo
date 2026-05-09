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
// Constructed lazily per invocation so command-tree creation doesn't touch
// the filesystem or external processes.
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
		// Malformed config is a hard failure — silently ignoring would mask user typos.
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

// resolveCloneDestination returns the absolute clone destination path.
//   - into non-empty: absolutised and returned.
//   - projectsDir non-empty: <projectsDir>/<repo-name>.
//   - otherwise: <cwd>/<repo-name> (strip .git from repo name).
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

// expandRepoShorthand prepends defaultRemote when from is a bare repo name
// (no "://", ":", or "/" — not a full URL or SSH-style path). Result is not
// validated; the cloner surfaces a clear error for unresolvable URLs.
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
