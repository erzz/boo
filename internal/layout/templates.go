// Layout template resolution.
// Resolution order: user template at $XDG_CONFIG_HOME/boo/layouts/<name>.yaml,
// then built-in embedded templates. User templates shadow built-ins on name collision.

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

// ResolvedTemplate is a layout template plus its load source.
// Description is extracted from the YAML leading-comment block (stripped by
// the parser); used by `boo layouts` and the new-project preview only.
type ResolvedTemplate struct {
	Layout      Layout
	Source      TemplateSource
	Path        string // absolute path for SourceUser, embed path for SourceBuiltin
	Description string
}

// ResolveTemplate looks up the named layout template. layoutsDir may be empty,
// in which case only built-ins are consulted.
func ResolveTemplate(layoutsDir, name string) (ResolvedTemplate, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "triple"
	}
	if err := validTemplateName(name); err != nil {
		return ResolvedTemplate{}, err
	}

	// 1. User template.
	if layoutsDir != "" {
		path := filepath.Join(layoutsDir, name+".yaml")
		if data, err := os.ReadFile(path); err == nil {
			l, perr := Parse(data)
			if perr != nil {
				return ResolvedTemplate{}, fmt.Errorf("layout template %s: %w", path, perr)
			}
			if l.Name == "" {
				l.Name = name
			}
			return ResolvedTemplate{
				Layout:      l,
				Source:      SourceUser,
				Path:        path,
				Description: extractDescription(data),
			}, nil
		} else if !os.IsNotExist(err) {
			return ResolvedTemplate{}, fmt.Errorf("layout template %s: %w", path, err)
		}
	}

	// 2. Built-in.
	embedPath := "templates/" + name + ".yaml"
	data, err := bundledTemplates.ReadFile(embedPath)
	if err != nil {
		return ResolvedTemplate{}, fmt.Errorf("layout template %q not found (no user template at %s, no built-in)",
			name, filepath.Join(layoutsDir, name+".yaml"))
	}
	l, perr := Parse(data)
	if perr != nil {
		return ResolvedTemplate{}, fmt.Errorf("built-in layout %q: %w", name, perr)
	}
	if l.Name == "" {
		l.Name = name
	}
	return ResolvedTemplate{
		Layout:      l,
		Source:      SourceBuiltin,
		Path:        embedPath,
		Description: extractDescription(data),
	}, nil
}

// extractDescription pulls a description from the leading comment block of a
// YAML template. Only consecutive lines from the very top count; the block ends
// at the first non-comment, non-blank line. A leading space after '#' is stripped.
// Returns "" if no leading comment block is present.
func extractDescription(data []byte) string {
	var lines []string
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			// Blank line ends the contiguous comment block.
			break
		}
		if !strings.HasPrefix(line, "#") {
			break
		}
		// Strip the '#' and at most one space.
		stripped := strings.TrimPrefix(line, "#")
		stripped = strings.TrimPrefix(stripped, " ")
		lines = append(lines, stripped)
	}
	// Trim trailing blanks (a "# " at the end of the block).
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	return strings.Join(lines, "\n")
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
		if name, ok := stripYAMLExt(e.Name()); ok {
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

func stripYAMLExt(filename string) (string, bool) {
	if !strings.HasSuffix(filename, ".yaml") {
		return "", false
	}
	return strings.TrimSuffix(filename, ".yaml"), true
}

// validTemplateName rejects path traversal so a "../foo" lookup can't escape layoutsDir.
func validTemplateName(name string) error {
	if name == "" {
		return fmt.Errorf("layout template name is empty")
	}
	if strings.ContainsAny(name, `/\`) || name == "." || name == ".." {
		return fmt.Errorf("layout template name %q is invalid", name)
	}
	return nil
}

// ensure embed.FS is used in non-test builds.
var _ fs.FS = bundledTemplates
