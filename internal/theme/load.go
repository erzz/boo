package theme

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"sigs.k8s.io/yaml"
)

//go:embed all:themes
var bundledThemes embed.FS

// Resolve looks up a named theme, preferring user themes in themesDir
// over built-ins. themesDir may be empty, in which case only built-ins
// are consulted (used in tests and when boo is run on a machine with no
// config dir yet).
//
// An empty name resolves to "default". This matches the contract used
// by `internal/config` — `ui.theme: ""` means "use the default theme".
//
// Validation is lenient: a malformed user theme returns the parse
// error, but the caller (typically the picker) is expected to fall back
// to the built-in default theme rather than fail. See package doc.
func Resolve(themesDir, name string) (Resolved, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "default"
	}
	if err := validThemeName(name); err != nil {
		return Resolved{}, err
	}

	// 1. User theme.
	if themesDir != "" {
		path := filepath.Join(themesDir, name+".yaml")
		data, err := os.ReadFile(path)
		switch {
		case err == nil:
			t, perr := parseTheme(data, name)
			if perr != nil {
				return Resolved{}, fmt.Errorf("theme %s: %w", path, perr)
			}
			return Resolved{
				Theme:  fillDefaults(t),
				Source: SourceUser,
				Path:   path,
			}, nil
		case os.IsNotExist(err):
			// fall through to built-in
		default:
			return Resolved{}, fmt.Errorf("theme %s: %w", path, err)
		}
	}

	// 2. Built-in.
	embedPath := "themes/" + name + ".yaml"
	data, err := bundledThemes.ReadFile(embedPath)
	if err != nil {
		return Resolved{}, fmt.Errorf("theme %q not found (no user theme at %s, no built-in)",
			name, filepath.Join(themesDir, name+".yaml"))
	}
	t, perr := parseTheme(data, name)
	if perr != nil {
		// A broken built-in is a build-time bug, not a user bug.
		return Resolved{}, fmt.Errorf("built-in theme %q: %w", name, perr)
	}
	return Resolved{
		Theme:  fillDefaults(t),
		Source: SourceBuiltin,
		Path:   embedPath,
	}, nil
}

// MustDefault returns the embedded default theme. Used as the ultimate
// fallback when even user-requested themes fail to load. Panics only if
// the embedded `default.yaml` is missing or malformed, which would
// indicate a corrupt binary.
func MustDefault() Theme {
	r, err := Resolve("", "default")
	if err != nil {
		panic(fmt.Sprintf("theme: built-in default missing or broken: %v", err))
	}
	return r.Theme
}

// List returns the union of built-in and user theme names, sorted.
// Each name appears once; user themes shadow built-ins of the same name.
func List(themesDir string) ([]string, error) {
	seen := map[string]struct{}{}

	entries, err := bundledThemes.ReadDir("themes")
	if err != nil {
		return nil, fmt.Errorf("theme: list built-ins: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if name, ok := stripYAMLExt(e.Name()); ok {
			seen[name] = struct{}{}
		}
	}

	if themesDir != "" {
		dirEntries, err := os.ReadDir(themesDir)
		if err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("theme: list %s: %w", themesDir, err)
		}
		for _, e := range dirEntries {
			if e.IsDir() {
				continue
			}
			if name, ok := stripYAMLExt(e.Name()); ok {
				seen[name] = struct{}{}
			}
		}
	}

	out := make([]string, 0, len(seen))
	for n := range seen {
		out = append(out, n)
	}
	sort.Strings(out)
	return out, nil
}

// SourceOf returns where a theme of the given name would be loaded from
// without actually parsing it. Used by `boo themes` to label entries.
// Returns SourceUser if a user file exists for this name; SourceBuiltin
// otherwise. Returns ("", false) if neither exists.
func SourceOf(themesDir, name string) (Source, bool) {
	if themesDir != "" {
		path := filepath.Join(themesDir, name+".yaml")
		if _, err := os.Stat(path); err == nil {
			return SourceUser, true
		}
	}
	if _, err := bundledThemes.ReadFile("themes/" + name + ".yaml"); err == nil {
		return SourceBuiltin, true
	}
	return "", false
}

// BuiltinYAML returns the raw YAML bytes of the named built-in theme.
// Used by `boo themes init` to seed a starter file the user can edit.
// Returns an error if no built-in by that name exists.
func BuiltinYAML(name string) ([]byte, error) {
	if err := validThemeName(name); err != nil {
		return nil, err
	}
	return bundledThemes.ReadFile("themes/" + name + ".yaml")
}

func parseTheme(data []byte, defaultName string) (Theme, error) {
	var t Theme
	if err := yaml.Unmarshal(data, &t); err != nil {
		return Theme{}, err
	}
	if t.Name == "" {
		t.Name = defaultName
	}
	return t, nil
}

// fillDefaults backfills any unset colour slot from the built-in
// `default` theme. This means user themes can override only the slots
// they care about — e.g. just the `border` colour — without copying
// the entire palette. Without this, an empty slot would render as
// terminal default fg, which the user almost certainly didn't intend.
//
// Only applied to user themes (and to the default itself, which is a
// no-op); a broken built-in default would surface as the panic in
// MustDefault rather than as silent fallback.
func fillDefaults(t Theme) Theme {
	if t.Name == "default" {
		return t
	}
	data, err := bundledThemes.ReadFile("themes/default.yaml")
	if err != nil {
		// Built-in default missing — caller will hit MustDefault
		// and panic. Don't compound the failure here.
		return t
	}
	var def Theme
	if err := yaml.Unmarshal(data, &def); err != nil {
		return t
	}
	if t.Colors.Accent == "" {
		t.Colors.Accent = def.Colors.Accent
	}
	if t.Colors.Info == "" {
		t.Colors.Info = def.Colors.Info
	}
	if t.Colors.Border == "" {
		t.Colors.Border = def.Colors.Border
	}
	if t.Colors.OK == "" {
		t.Colors.OK = def.Colors.OK
	}
	if t.Colors.Warn == "" {
		t.Colors.Warn = def.Colors.Warn
	}
	if t.Colors.Stopped == "" {
		t.Colors.Stopped = def.Colors.Stopped
	}
	return t
}

func stripYAMLExt(filename string) (string, bool) {
	if !strings.HasSuffix(filename, ".yaml") {
		return "", false
	}
	return strings.TrimSuffix(filename, ".yaml"), true
}

// validThemeName mirrors layout's name validation: reject path traversal
// and anything that isn't a plain identifier. Themes live as
// `<name>.yaml` inside themesDir or the embedded FS; we never want a
// "../foo" or "/etc/passwd" lookup to slip through.
func validThemeName(name string) error {
	if name == "" {
		return fmt.Errorf("theme name is empty")
	}
	if strings.ContainsAny(name, `/\`) || name == "." || name == ".." {
		return fmt.Errorf("theme name %q is invalid", name)
	}
	return nil
}

// ensure embed.FS is referenced in non-test builds.
var _ fs.FS = bundledThemes
