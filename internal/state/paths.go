// Package state owns boo's on-disk paths and atomic file IO.
//
// All persistent files boo writes go through this package. Path layout
// follows the XDG Base Directory spec on macOS, mirroring Linux conventions
// for portability:
//
//	$XDG_CONFIG_HOME/boo/   (default ~/.config/boo)
//	  config.toml           — global config
//	  layouts/*.toml        — user-defined shared layout templates
//
//	$XDG_DATA_HOME/boo/     (default ~/.local/share/boo)
//	  projects.toml         — registry index
//	  projects/<name>/
//	    layout.yaml         — resolved layout snapshot
//	    state.json          — runtime state (last WindowID, last-launched-at)
package state

import (
	"fmt"
	"os"
	"path/filepath"
)

// Paths is the resolved set of directories and files boo uses.
type Paths struct {
	ConfigDir   string // ~/.config/boo
	DataDir     string // ~/.local/share/boo
	ConfigFile  string // ConfigDir/config.toml
	LayoutsDir  string // ConfigDir/layouts
	Registry    string // DataDir/projects.toml
	ProjectsDir string // DataDir/projects
}

// Default returns the standard paths derived from XDG env vars (or sensible
// macOS/Linux fallbacks).
//
// If $BOO_HOME is set, it overrides everything: ConfigDir = $BOO_HOME/config
// and DataDir = $BOO_HOME/data. This is intended for tests, isolation during
// experimentation, and power users who want all boo state in one place.
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
	data := os.Getenv("XDG_DATA_HOME")
	if data == "" {
		data = filepath.Join(home, ".local", "share")
	}
	return forBase(filepath.Join(cfg, "boo"), filepath.Join(data, "boo")), nil
}

// ForRoot returns Paths rooted at a single base directory. Used in tests so
// tests don't pollute the user's real config/data dirs.
func ForRoot(root string) Paths {
	return forBase(filepath.Join(root, "config"), filepath.Join(root, "data"))
}

func forBase(configDir, dataDir string) Paths {
	return Paths{
		ConfigDir:   configDir,
		DataDir:     dataDir,
		ConfigFile:  filepath.Join(configDir, "config.toml"),
		LayoutsDir:  filepath.Join(configDir, "layouts"),
		Registry:    filepath.Join(dataDir, "projects.toml"),
		ProjectsDir: filepath.Join(dataDir, "projects"),
	}
}

// ProjectDir returns the per-project state directory for the given name.
func (p Paths) ProjectDir(name string) string {
	return filepath.Join(p.ProjectsDir, name)
}

// EnsureDirs creates ConfigDir, DataDir, LayoutsDir, and ProjectsDir if they
// don't already exist.
func (p Paths) EnsureDirs() error {
	for _, d := range []string{p.ConfigDir, p.DataDir, p.LayoutsDir, p.ProjectsDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return fmt.Errorf("state: mkdir %s: %w", d, err)
		}
	}
	return nil
}
