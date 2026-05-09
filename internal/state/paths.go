// Package state owns boo's on-disk paths and atomic file IO.
// All persistent files live under $XDG_CONFIG_HOME/boo (default ~/.config/boo):
// config.yaml, layouts/*.yaml, themes/*.yaml, projects.toml, projects/<name>/.
package state

import (
	"fmt"
	"os"
	"path/filepath"
)

// Paths is the resolved set of directories and files boo uses.
type Paths struct {
	ConfigDir   string // ~/.config/boo
	ConfigFile  string // ConfigDir/config.yaml
	LayoutsDir  string // ConfigDir/layouts
	ThemesDir   string // ConfigDir/themes
	Registry    string // ConfigDir/projects.toml
	ProjectsDir string // ConfigDir/projects
}

// Default returns paths derived from XDG env vars. If $BOO_HOME is set it
// overrides everything; intended for tests and power users.
func Default() (Paths, error) {
	if root := os.Getenv("BOO_HOME"); root != "" {
		return ForRoot(root), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return Paths{}, fmt.Errorf("state: resolve home: %w", err)
	}
	cfg := os.Getenv("XDG_CONFIG_HOME")
	if cfg == "" {
		cfg = filepath.Join(home, ".config")
	}
	return forBase(filepath.Join(cfg, "boo")), nil
}

// ForRoot returns Paths rooted at base. Used in tests to avoid polluting the user's config dir.
func ForRoot(root string) Paths {
	return forBase(root)
}

func forBase(configDir string) Paths {
	return Paths{
		ConfigDir:   configDir,
		ConfigFile:  filepath.Join(configDir, "config.yaml"),
		LayoutsDir:  filepath.Join(configDir, "layouts"),
		ThemesDir:   filepath.Join(configDir, "themes"),
		Registry:    filepath.Join(configDir, "projects.toml"),
		ProjectsDir: filepath.Join(configDir, "projects"),
	}
}

// ProjectDir returns the per-project state directory for the given name.
func (p Paths) ProjectDir(name string) string {
	return filepath.Join(p.ProjectsDir, name)
}

// ProjectLayoutFile returns the path to the per-project layout snapshot.
// Centralised here because the filename changed once (.toml → .yaml) and may again.
func (p Paths) ProjectLayoutFile(name string) string {
	return filepath.Join(p.ProjectDir(name), "layout.yaml")
}

// ProjectStateFile returns the path to the per-project runtime state file (state.json).
func (p Paths) ProjectStateFile(name string) string {
	return filepath.Join(p.ProjectDir(name), "state.json")
}

// EnsureDirs creates ConfigDir, LayoutsDir, and ProjectsDir if they
// don't already exist.
func (p Paths) EnsureDirs() error {
	for _, d := range []string{p.ConfigDir, p.LayoutsDir, p.ProjectsDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return fmt.Errorf("state: mkdir %s: %w", d, err)
		}
	}
	return nil
}
