// Package layout defines boo's layout vocabulary and YAML parsing.
// A Layout has Tabs, each Tab has a root Split tree. A Split is either a leaf
// (cwd/command/env/initial_input) or an interior node (direction + 2 children).
// "row" = left-to-right, "column" = top-to-bottom (CSS flexbox conventions).
package layout

import (
	"errors"
	"fmt"
	"strings"

	"sigs.k8s.io/yaml"
)

// Direction values accepted on an interior Split.
const (
	DirRow    = "row"
	DirColumn = "column"
)

// MaxDepth caps recursion in the validator. Protects the renderer and JXA walker
// against pathological inputs; 8 is well beyond any useful human-authored layout.
const MaxDepth = 8

// Layout is a complete project layout.
type Layout struct {
	Name string `json:"name"`
	Tabs []Tab  `json:"tabs"`
}

// Tab is a single Ghostty tab. Its Root is the (recursive) split tree for
// the tab. A Tab whose Root is a leaf renders as a single shell.
type Tab struct {
	Name string `json:"name,omitempty"`
	Root Split  `json:"split"`
}

// Split is one node in a tab's split tree.
//
// Exactly one of two shapes is valid:
//   - LEAF: Direction == "" && Children == nil. Carries Cwd plus optional Command/InitialInput/Env.
//   - INTERIOR: Direction != "" && len(Children) == 2. No leaf properties.
//
// Why exactly 2 children: Ghostty's AppleScript `split` command always halves a pane.
// Forcing N=2 means a layout that parses is a layout that renders faithfully.
// Asymmetric N-way splits are still possible by nesting (e.g. row(A, row(B,C))).
//
// json tags are used throughout (sigs.k8s.io/yaml routes through encoding/json).
type Split struct {
	Direction string  `json:"direction,omitempty"`
	Children  []Split `json:"children,omitempty"`

	Cwd          string            `json:"cwd,omitempty"`
	Command      string            `json:"command,omitempty"`
	InitialInput string            `json:"initial_input,omitempty"`
	Env          map[string]string `json:"env,omitempty"`
}

// IsLeaf reports whether s is a leaf node (no Direction and no Children).
// Prefer this over checking Direction/Children directly.
func (s Split) IsLeaf() bool {
	return s.Direction == "" && len(s.Children) == 0
}

// Default returns the built-in "triple" layout: one large left pane, two stacked right.
func Default() Layout {
	return Layout{
		Name: "triple",
		Tabs: []Tab{{
			Name: "main",
			Root: Split{
				Direction: DirRow,
				Children: []Split{
					{Cwd: "."},
					{
						Direction: DirColumn,
						Children: []Split{
							{Cwd: "."},
							{Cwd: "."},
						},
					},
				},
			},
		}},
	}
}

// Parse decodes a YAML document into a Layout, then validates it.
func Parse(data []byte) (Layout, error) {
	var l Layout
	if err := yaml.UnmarshalStrict(data, &l); err != nil {
		return Layout{}, fmt.Errorf("layout: parse: %w", err)
	}
	if err := l.Validate(); err != nil {
		return Layout{}, err
	}
	return l, nil
}

// Marshal encodes a Layout as YAML. Validates first so we never write a
// bad file to disk.
func Marshal(l Layout) ([]byte, error) {
	if err := l.Validate(); err != nil {
		return nil, err
	}
	return yaml.Marshal(l)
}

// Validate enforces structural rules.
func (l Layout) Validate() error {
	if strings.TrimSpace(l.Name) == "" {
		return errors.New("layout: name is required")
	}
	if len(l.Tabs) == 0 {
		return errors.New("layout: at least one tab is required")
	}
	for i, t := range l.Tabs {
		if err := validateSplit(t.Root, fmt.Sprintf("tab %d (%q)", i, t.Name), 0); err != nil {
			return err
		}
	}
	return nil
}

// validateSplit walks a Split subtree enforcing the leaf-XOR-interior rule and depth cap.
// path is a human-readable breadcrumb for error messages; depth 0 = tab root.
func validateSplit(s Split, path string, depth int) error {
	if depth > MaxDepth {
		return fmt.Errorf("layout: %s: nested too deep (max %d)", path, MaxDepth)
	}

	hasInterior := s.Direction != "" || len(s.Children) > 0
	hasLeaf := s.Cwd != "" || s.Command != "" || s.InitialInput != "" || len(s.Env) > 0

	if hasInterior && hasLeaf {
		return fmt.Errorf("layout: %s: a split is either a leaf (cwd/command/env/initial_input) or an interior node (direction/children); not both", path)
	}

	// Pure leaf: allow cwd-less leaves (inherit project dir, equivalent to `cwd: .`).
	if !hasInterior {
		return nil
	}

	// Interior: direction must be valid, exactly 2 children.
	if !validDirection(s.Direction) {
		return fmt.Errorf("layout: %s: direction %q is not one of row|column", path, s.Direction)
	}
	if len(s.Children) != 2 {
		return fmt.Errorf("layout: %s: an interior split must have exactly 2 children (Ghostty splits panes in half), got %d", path, len(s.Children))
	}
	for i, child := range s.Children {
		childPath := fmt.Sprintf("%s %s[%d]", path, s.Direction, i)
		if err := validateSplit(child, childPath, depth+1); err != nil {
			return err
		}
	}
	return nil
}

func validDirection(d string) bool {
	switch d {
	case DirRow, DirColumn:
		return true
	default:
		return false
	}
}
