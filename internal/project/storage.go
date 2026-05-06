package project

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/sean-erswell-liljefelt/boo/internal/layout"
	"github.com/sean-erswell-liljefelt/boo/internal/state"
)

// Runtime is the per-project state captured between invocations.
//
// Stored as JSON (not TOML) because it's machine-managed only — never
// hand-edited — and JSON gives us cleaner zero-value handling for nested
// structures we may add later.
type Runtime struct {
	WindowID       string    `json:"window_id,omitempty"`
	LastLaunchedAt time.Time `json:"last_launched_at,omitempty"`
}

// SaveLayout writes the resolved layout snapshot for project name.
func SaveLayout(p state.Paths, name string, l layout.Layout) error {
	if err := os.MkdirAll(p.ProjectDir(name), 0o755); err != nil {
		return fmt.Errorf("project: mkdir %s: %w", p.ProjectDir(name), err)
	}
	data, err := layout.Marshal(l)
	if err != nil {
		return err
	}
	return state.WriteAtomic(filepath.Join(p.ProjectDir(name), "layout.toml"), data)
}

// LoadLayout reads the resolved layout snapshot for project name.
func LoadLayout(p state.Paths, name string) (layout.Layout, error) {
	path := filepath.Join(p.ProjectDir(name), "layout.toml")
	data, err := os.ReadFile(path)
	if err != nil {
		return layout.Layout{}, fmt.Errorf("project: read layout %s: %w", path, err)
	}
	return layout.Parse(data)
}

// LoadRuntime reads runtime state for project name. A missing file yields a
// zero-value Runtime with no error.
func LoadRuntime(p state.Paths, name string) (Runtime, error) {
	path := filepath.Join(p.ProjectDir(name), "state.json")
	data, err := state.ReadOrEmpty(path)
	if err != nil {
		return Runtime{}, err
	}
	if len(data) == 0 {
		return Runtime{}, nil
	}
	var rt Runtime
	if err := json.Unmarshal(data, &rt); err != nil {
		return Runtime{}, fmt.Errorf("project: parse runtime %s: %w", path, err)
	}
	return rt, nil
}

// SaveRuntime writes runtime state for project name atomically.
func SaveRuntime(p state.Paths, name string, rt Runtime) error {
	if err := os.MkdirAll(p.ProjectDir(name), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(rt, "", "  ")
	if err != nil {
		return fmt.Errorf("project: marshal runtime: %w", err)
	}
	return state.WriteAtomic(filepath.Join(p.ProjectDir(name), "state.json"), data)
}

// PurgeProjectDir removes the per-project state directory entirely. Used by
// `boo rm`.
func PurgeProjectDir(p state.Paths, name string) error {
	dir := p.ProjectDir(name)
	if err := os.RemoveAll(dir); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("project: remove %s: %w", dir, err)
	}
	return nil
}
