// Package project owns the registry of known projects and the per-project
// state stored on disk.
//
// The registry (projects.toml) is the single source of truth for "which
// projects exist and where they live." Per-project state — the resolved
// layout snapshot and runtime state (last WindowID) — lives under
// state.Paths.ProjectDir(name).
package project

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"

	"github.com/erzz/boo/internal/state"
)

// Project is a single registered project.
type Project struct {
	Name      string    `toml:"name"`
	Dir       string    `toml:"dir"`
	Layout    string    `toml:"layout"` // template name used at create time, for display only
	CreatedAt time.Time `toml:"created_at,omitzero"`
}

// Registry is the on-disk index of projects.
type Registry struct {
	Projects []Project `toml:"project"`
}

// ErrNotFound is returned when a lookup misses.
var ErrNotFound = errors.New("project not found")

// ErrAlreadyExists is returned when adding a project whose name is taken.
var ErrAlreadyExists = errors.New("project already exists")

// nameRE constrains project names to a safe character set so they can be used
// as directory names without escaping headaches.
var nameRE = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,63}$`)

// reservedNames are CLI verbs / flags that would shadow boo subcommands.
// Refusing them keeps `boo <name>` unambiguous.
var reservedNames = map[string]bool{
	"doctor": true, "new": true, "list": true, "delete": true,
	"save": true, "fzf": true, "layouts": true, "config": true,
	"edit": true, "set-layout": true, "show": true,
	"help": true, "completion": true,
}

// ValidateName enforces the naming rules used by the registry and CLI.
func ValidateName(name string) error {
	if !nameRE.MatchString(name) {
		return fmt.Errorf("project name %q is invalid (allowed: letters, digits, _ . -; max 64 chars; must start with letter or digit)", name)
	}
	if reservedNames[strings.ToLower(name)] {
		return fmt.Errorf("project name %q is a reserved boo command", name)
	}
	return nil
}

// Load reads the registry from disk. A missing file yields an empty registry.
func Load(p state.Paths) (*Registry, error) {
	data, err := state.ReadOrEmpty(p.Registry)
	if err != nil {
		return nil, fmt.Errorf("project: load registry: %w", err)
	}
	r := &Registry{}
	if len(data) == 0 {
		return r, nil
	}
	if err := toml.Unmarshal(data, r); err != nil {
		return nil, fmt.Errorf("project: parse registry: %w", err)
	}
	return r, nil
}

// Save writes the registry to disk atomically.
func (r *Registry) Save(p state.Paths) error {
	if err := p.EnsureDirs(); err != nil {
		return err
	}
	// Stable order on disk: alphabetical by name.
	sorted := make([]Project, len(r.Projects))
	copy(sorted, r.Projects)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })
	out := Registry{Projects: sorted}
	data, err := toml.Marshal(out)
	if err != nil {
		return fmt.Errorf("project: marshal registry: %w", err)
	}
	return state.WriteAtomic(p.Registry, data)
}

// Get returns the named project or ErrNotFound.
func (r *Registry) Get(name string) (Project, error) {
	for _, p := range r.Projects {
		if p.Name == name {
			return p, nil
		}
	}
	return Project{}, fmt.Errorf("%w: %s", ErrNotFound, name)
}

// Has reports whether name is registered.
func (r *Registry) Has(name string) bool {
	_, err := r.Get(name)
	return err == nil
}

// Add inserts a project; returns ErrAlreadyExists if the name is taken.
func (r *Registry) Add(p Project) error {
	if r.Has(p.Name) {
		return fmt.Errorf("%w: %s", ErrAlreadyExists, p.Name)
	}
	r.Projects = append(r.Projects, p)
	return nil
}

// Remove deletes a project by name; returns ErrNotFound if absent.
func (r *Registry) Remove(name string) error {
	for i, p := range r.Projects {
		if p.Name == name {
			r.Projects = append(r.Projects[:i], r.Projects[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("%w: %s", ErrNotFound, name)
}

// Update replaces the project entry with the same name as p. Returns
// ErrNotFound if no such project is registered. Used by commands like
// `boo set-layout` that mutate an existing project's metadata.
func (r *Registry) Update(p Project) error {
	for i, existing := range r.Projects {
		if existing.Name == p.Name {
			r.Projects[i] = p
			return nil
		}
	}
	return fmt.Errorf("%w: %s", ErrNotFound, p.Name)
}

// FindByDir returns the project whose Dir matches dir exactly, or ErrNotFound.
// Used by `boo` (no args) to detect the current project.
//
// If multiple projects share the same Dir (which Add now prevents but a
// hand-edited registry could produce), the first match by alphabetical name
// wins. Callers that need to detect ambiguity should use HasDir first.
func (r *Registry) FindByDir(dir string) (Project, error) {
	for _, p := range r.Projects {
		if p.Dir == dir {
			return p, nil
		}
	}
	return Project{}, fmt.Errorf("%w: dir %s", ErrNotFound, dir)
}

// HasDir reports whether any registered project lives at dir.
func (r *Registry) HasDir(dir string) bool {
	for _, p := range r.Projects {
		if p.Dir == dir {
			return true
		}
	}
	return false
}
