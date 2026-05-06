// Layout template resolution.
//
// `boo new --layout <name>` resolves the layout in this order:
//
//  1. User template at $XDG_CONFIG_HOME/boo/layouts/<name>.toml (if present).
//  2. Built-in template embedded in the binary (currently "default" and "dev").
//
// Built-ins ensure `boo new` works on a clean machine. User templates always
// win on name collision so users can override even "default" by dropping their
// own default.toml in the layouts dir.

package layout

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

//go:embed all:templates
var bundledTemplates embed.FS

// TemplateSource describes where a resolved template came from.
type TemplateSource string

const (
	SourceUser    TemplateSource = "user"
	SourceBuiltin TemplateSource = "builtin"
)

// ResolvedTemplate is a layout template plus where it was loaded from.
type ResolvedTemplate struct {
	Layout Layout
	Source TemplateSource
	Path   string // absolute path for SourceUser, embed path for SourceBuiltin
}

// ResolveTemplate looks up the named layout template. layoutsDir may be empty,
// in which case only built-ins are consulted.
func ResolveTemplate(layoutsDir, name string) (ResolvedTemplate, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "default"
	}
	if err := validTemplateName(name); err != nil {
		return ResolvedTemplate{}, err
	}

	// 1. User template.
	if layoutsDir != "" {
		path := filepath.Join(layoutsDir, name+".toml")
		if data, err := os.ReadFile(path); err == nil {
			l, perr := Parse(data)
			if perr != nil {
				return ResolvedTemplate{}, fmt.Errorf("layout template %s: %w", path, perr)
			}
			if l.Name == "" {
				l.Name = name
			}
			return ResolvedTemplate{Layout: l, Source: SourceUser, Path: path}, nil
		} else if !os.IsNotExist(err) {
			return ResolvedTemplate{}, fmt.Errorf("layout template %s: %w", path, err)
		}
	}

	// 2. Built-in.
	embedPath := "templates/" + name + ".toml"
	data, err := bundledTemplates.ReadFile(embedPath)
	if err != nil {
		return ResolvedTemplate{}, fmt.Errorf("layout template %q not found (no user template at %s, no built-in)",
			name, filepath.Join(layoutsDir, name+".toml"))
	}
	l, perr := Parse(data)
	if perr != nil {
		return ResolvedTemplate{}, fmt.Errorf("built-in layout %q: %w", name, perr)
	}
	if l.Name == "" {
		l.Name = name
	}
	return ResolvedTemplate{Layout: l, Source: SourceBuiltin, Path: embedPath}, nil
}

// ListTemplates returns the union of built-in and user template names, sorted.
// Each name appears once even if a user template shadows a built-in.
func ListTemplates(layoutsDir string) ([]string, error) {
	seen := map[string]struct{}{}

	// Built-ins.
	entries, err := bundledTemplates.ReadDir("templates")
	if err != nil {
		return nil, fmt.Errorf("layout: list built-ins: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if name, ok := stripTOML(e.Name()); ok {
			seen[name] = struct{}{}
		}
	}

	// User templates.
	if layoutsDir != "" {
		dirEntries, err := os.ReadDir(layoutsDir)
		if err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("layout: list %s: %w", layoutsDir, err)
		}
		for _, e := range dirEntries {
			if e.IsDir() {
				continue
			}
			if name, ok := stripTOML(e.Name()); ok {
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

func stripTOML(filename string) (string, bool) {
	if !strings.HasSuffix(filename, ".toml") {
		return "", false
	}
	return strings.TrimSuffix(filename, ".toml"), true
}

// validTemplateName rejects path traversal and anything that isn't a plain
// identifier. Templates live as `<name>.toml` inside layoutsDir or the
// embedded FS; we never want a "../foo" or "/etc/passwd" lookup to slip
// through.
func validTemplateName(name string) error {
	if name == "" {
		return fmt.Errorf("layout template name is empty")
	}
	if strings.ContainsAny(name, `/\`) || name == "." || name == ".." {
		return fmt.Errorf("layout template name %q is invalid", name)
	}
	return nil
}

// ensure embed.FS is used in non-test builds so the linter doesn't complain
// in environments where only ListTemplates is called.
var _ fs.FS = bundledTemplates
