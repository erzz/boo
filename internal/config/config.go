// Package config loads boo's global user configuration.
//
// The config file lives at ~/.config/boo/config.yaml (or wherever
// state.Paths points). It is OPTIONAL: a missing file is not an error;
// boo runs entirely on factory defaults. A present file's values
// override the corresponding factory defaults; unset keys keep their
// factory values (no merge surprises).
//
// We deliberately do NOT support env-var or per-project overrides at
// this layer. Per-project layout customisation lives in the project's
// own layout.yaml; env vars would create a third precedence layer that
// nobody asks for and everybody trips over.
package config

import (
	"fmt"
	"os"

	"sigs.k8s.io/yaml"
)

// Config is boo's effective global config.
//
// Add new keys here. Factory defaults live in DefaultConfig() — never
// rely on Go's zero values for "default", because then we can't tell
// "user explicitly set this to empty/zero" from "user said nothing".
// All optional fields are pointers so an absent key in YAML is
// distinguishable from a present-but-empty value.
type Config struct {
	// DefaultLayout is the layout template name used when --layout is
	// omitted on `boo new` and the new-project form is opened without
	// a preselection. Factory default: "triple".
	DefaultLayout *string `json:"default_layout,omitempty" yaml:"default_layout,omitempty"`

	// ProjectsDir is the parent directory `boo new --from <url>` clones
	// into when --into isn't given. Tilde expansion is applied at read
	// time. Factory default: "" (clone destination derived relative to
	// the current working directory, matching pre-config behaviour).
	ProjectsDir *string `json:"projects_dir,omitempty" yaml:"projects_dir,omitempty"`

	// Git holds git-related preferences.
	Git GitConfig `json:"git,omitempty" yaml:"git,omitempty"`

	// UI holds TUI/CLI presentation preferences.
	UI UIConfig `json:"ui,omitempty" yaml:"ui,omitempty"`
}

// GitConfig groups git-related preferences.
type GitConfig struct {
	// DefaultRemote, if set, pre-fills the new-project form's "Clone
	// from URL" field. Typing a bare repo name in that field expands
	// to "<DefaultRemote>/<name>". Empty = no prefill, no expansion.
	DefaultRemote *string `json:"default_remote,omitempty" yaml:"default_remote,omitempty"`
}

// UIConfig groups TUI/CLI presentation preferences.
//
// Reserved-only for now. Theme has no behaviour wired up yet; it's
// here so the schema is forward-stable when we add theming.
type UIConfig struct {
	// Theme selects a named theme. Factory default: "default". No
	// behaviour is wired yet — this is a placeholder so users can
	// start adding the key without it breaking on parse.
	Theme *string `json:"theme,omitempty" yaml:"theme,omitempty"`
}

// DefaultConfig returns the factory defaults. Every field that has a
// non-empty default value MUST set it here, not rely on Go zero values.
func DefaultConfig() Config {
	dl := "triple"
	theme := "default"
	return Config{
		DefaultLayout: &dl,
		UI:            UIConfig{Theme: &theme},
	}
}

// Load reads the config file at path, merges it onto factory defaults,
// and returns the effective config plus a Sources map describing where
// each value came from (factory vs file). Sources is intended for
// `boo config show` so users can debug which value is winning.
//
// A missing file is not an error: defaults are returned with every
// source = "factory".
//
// A malformed file IS an error: silently falling back to defaults
// would mask user mistakes (typos in YAML keys, wrong types).
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
	if err := yaml.Unmarshal(data, &fromFile); err != nil {
		return cfg, src, fmt.Errorf("config: parse %s: %w", path, err)
	}

	merge(&cfg, &fromFile, src, path)
	return cfg, src, nil
}

// Sources records the origin of each config value. Keys are dotted
// field paths (e.g. "default_layout", "git.default_remote"). Values
// are either "factory" or the absolute path of the config file.
type Sources map[string]string

func newSources(origin string) Sources {
	return Sources{
		"default_layout":     origin,
		"projects_dir":       origin,
		"git.default_remote": origin,
		"ui.theme":           origin,
	}
}

// merge applies non-nil fields from src on top of dst, recording the
// origin in srcs for every field that was overridden.
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

// expandTilde replaces a leading "~" with $HOME. Done at load time so
// downstream consumers can treat the value as a plain absolute path.
// If $HOME can't be resolved, the path is returned unchanged — the
// caller will get a clearer error from whatever filesystem op fails
// than we could produce here.
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

// Convenience getters that dereference pointers safely. Callers can
// use these instead of nil-checking every field.

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
