// Package config loads boo's global user configuration from ~/.config/boo/config.yaml.
// A missing file is not an error; boo runs on factory defaults. A malformed file IS an
// error — silent fallback would mask user mistakes.
package config

import (
	"fmt"
	"os"

	"sigs.k8s.io/yaml"
)

// Config is boo's effective global config.
//
// All optional fields are pointers so an absent YAML key ("user said nothing")
// is distinguishable from a present-but-zero value ("user set empty"). Factory
// defaults live in DefaultConfig() — never rely on Go zero values for defaults.
type Config struct {
	// DefaultLayout is the template used when --layout is omitted. Factory default: "triple".
	DefaultLayout *string `json:"default_layout,omitempty" yaml:"default_layout,omitempty"`

	// ProjectsDir is where `boo new --from <url>` clones when --into isn't given.
	// Tilde-expanded at load time. Factory default: "" (derive from cwd).
	ProjectsDir *string `json:"projects_dir,omitempty" yaml:"projects_dir,omitempty"`

	// Git holds git-related preferences.
	Git GitConfig `json:"git,omitempty" yaml:"git,omitempty"`

	// UI holds TUI/CLI presentation preferences.
	UI UIConfig `json:"ui,omitempty" yaml:"ui,omitempty"`
}

// GitConfig groups git-related preferences.
type GitConfig struct {
	// DefaultRemote pre-fills the new-project form's URL field; a bare repo name
	// expands to "<DefaultRemote>/<name>". Empty = no prefill.
	DefaultRemote *string `json:"default_remote,omitempty" yaml:"default_remote,omitempty"`
}

// UIConfig groups TUI/CLI presentation preferences.
type UIConfig struct {
	// Theme selects a named theme. Factory default: "default".
	Theme *string `json:"theme,omitempty" yaml:"theme,omitempty"`
}

// DefaultConfig returns factory defaults. Every field with a non-zero default
// MUST be set here; don't rely on Go zero values (see Config doc).
func DefaultConfig() Config {
	dl := "triple"
	theme := "default"
	return Config{
		DefaultLayout: &dl,
		UI:            UIConfig{Theme: &theme},
	}
}

// Load reads the config file at path, merges onto factory defaults, and returns
// the effective config plus a Sources map. A missing file returns defaults with
// every source = "factory". A malformed file returns an error.
func Load(path string) (Config, Sources, error) {
	cfg := DefaultConfig()
	src := newSources("factory")

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, src, nil
		}
		return cfg, src, fmt.Errorf("config: read %s: %w", path, err)
	}

	var fromFile Config
	if err := yaml.UnmarshalStrict(data, &fromFile); err != nil {
		return cfg, src, fmt.Errorf("config: parse %s: %w", path, err)
	}

	merge(&cfg, &fromFile, src, path)
	return cfg, src, nil
}

// Sources records the origin of each config value. Keys are dotted field paths
// (e.g. "default_layout"); values are "factory" or the config file path.
type Sources map[string]string

func newSources(origin string) Sources {
	return Sources{
		"default_layout":     origin,
		"projects_dir":       origin,
		"git.default_remote": origin,
		"ui.theme":           origin,
	}
}

// merge applies non-nil fields from src onto dst, recording each override in srcs.
func merge(dst, src *Config, srcs Sources, origin string) {
	if src.DefaultLayout != nil {
		dst.DefaultLayout = src.DefaultLayout
		srcs["default_layout"] = origin
	}
	if src.ProjectsDir != nil {
		expanded := expandTilde(*src.ProjectsDir)
		dst.ProjectsDir = &expanded
		srcs["projects_dir"] = origin
	}
	if src.Git.DefaultRemote != nil {
		dst.Git.DefaultRemote = src.Git.DefaultRemote
		srcs["git.default_remote"] = origin
	}
	if src.UI.Theme != nil {
		dst.UI.Theme = src.UI.Theme
		srcs["ui.theme"] = origin
	}
}

// expandTilde replaces a leading "~" with $HOME. If $HOME can't be resolved
// the path is returned unchanged (the caller will see a clearer fs error).
func expandTilde(p string) string {
	if p == "" || p[0] != '~' {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	if p == "~" {
		return home
	}
	if len(p) > 1 && p[1] == '/' {
		return home + p[1:]
	}
	return p
}

// DefaultLayoutOr returns the configured DefaultLayout or fallback if unset.
func (c Config) DefaultLayoutOr(fallback string) string {
	if c.DefaultLayout == nil {
		return fallback
	}
	return *c.DefaultLayout
}

// ProjectsDirOr returns the configured ProjectsDir or fallback if unset.
func (c Config) ProjectsDirOr(fallback string) string {
	if c.ProjectsDir == nil {
		return fallback
	}
	return *c.ProjectsDir
}

// GitDefaultRemoteOr returns the configured Git.DefaultRemote or fallback.
func (c Config) GitDefaultRemoteOr(fallback string) string {
	if c.Git.DefaultRemote == nil {
		return fallback
	}
	return *c.Git.DefaultRemote
}

// ThemeOr returns the configured UI.Theme or fallback.
func (c Config) ThemeOr(fallback string) string {
	if c.UI.Theme == nil {
		return fallback
	}
	return *c.UI.Theme
}
