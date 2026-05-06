package cli

import (
	"fmt"
	"os"
	"path/filepath"

	booexec "github.com/erzz/boo/internal/exec"
	"github.com/erzz/boo/internal/ghostty"
	"github.com/erzz/boo/internal/state"
)

// app is the shared dependency bundle for cobra commands.
//
// Constructed lazily per invocation in newApp so that command tree creation
// (used in tests / completions) doesn't touch the filesystem or external
// processes.
type app struct {
	Paths   state.Paths
	Runner  booexec.Runner
	Ghostty *ghostty.Client
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
	r := booexec.NewReal()
	return &app{Paths: p, Runner: r, Ghostty: ghostty.New(r)}, nil
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
