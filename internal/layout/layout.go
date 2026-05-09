// Package layout defines boo's layout vocabulary and the rules for parsing
// it from YAML.
//
// A layout is a small recursive tree:
//
//   - A Layout has a name and one or more Tabs.
//   - Each Tab has exactly one root Split.
//   - A Split is either a leaf (carries cwd, command, env, initial input)
//     OR an interior node (carries direction = "row" | "column" and >=2
//     children, each itself a Split). XOR — never both.
//
// The shape of the YAML file IS the shape of the rendered Ghostty window.
// "row" lays children out left-to-right; "column" stacks them top-to-bottom.
// We borrow these names from CSS flexbox where they mean exactly the same
// thing — the parent owns the orientation, not its children.
//
// cwd values may be relative; resolution against the project directory
// happens at apply time (not here).
package layout

import (
	"errors"
	"fmt"
	"strings"

	"sigs.k8s.io/yaml"
)

// Direction values accepted on an interior Split.
//
// We deliberately do NOT model "row-reverse" or "column-reverse". If a user
// wants a different order, they reorder Children. One way to do things.
const (
	DirRow    = "row"
	DirColumn = "column"
)

// MaxDepth caps recursion in the validator. Protects the renderer and JXA
// walker against pathological inputs (deeply-nested trees that would either
// produce sub-character-wide cells or stack `osascript` calls). 8 is well
// past anything a human would hand-author for a useful screen layout.
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
// Exactly one of two shapes is valid (validated by Validate):
//
//   - LEAF: Direction == "" && Children == nil. Carries Cwd plus
//     optional Command, InitialInput, Env.
//   - INTERIOR: Direction != "" && len(Children) == 2. Carries no
//     leaf properties. Children render in order; for "row" the first
//     child is left, the second is right; for "column" the first is
//     top, the second is bottom.
//
// Why exactly 2 children: Ghostty's AppleScript `split` command
// always halves a pane. There is no way to ask for a 3-way split that
// produces three equal-width panes — splitting twice gives 50/25/25.
// Forcing N=2 here means a layout that parses is a layout that can
// faithfully render in Ghostty. Asymmetric N-way layouts are still
// possible by nesting (e.g. row(A, row(B, C)) gives 50/25/25 on
// purpose, which is at least honest about its proportions).
//
// We use json tags throughout (sigs.k8s.io/yaml routes through encoding/json),
// which means the same struct serialises cleanly to either format.
type Split struct {
	Direction string  `json:"direction,omitempty"`
	Children  []Split `json:"children,omitempty"`

	Cwd          string            `json:"cwd,omitempty"`
	Command      string            `json:"command,omitempty"`
	InitialInput string            `json:"initial_input,omitempty"`
	Env          map[string]string `json:"env,omitempty"`
}

// IsLeaf reports whether s is a leaf node. A node is a leaf when it has
// neither Direction nor Children — same condition Validate enforces.
//
// Callers that walk the tree should branch on this rather than re-checking
// Direction/Children directly, so the leaf-vs-interior contract has one
// definition and one place to evolve.
func (s Split) IsLeaf() bool {
	return s.Direction == "" && len(s.Children) == 0
}

// Default returns boo's built-in fallback layout: the `triple` shape —
// one big pane on the left, two stacked panes on the right. Chosen as
// the default because it suits the most common shell-driven workflow
// (editor / runner / log-tail) without being so opinionated that a
// single-shell user feels punished. Used when something upstream can't
// resolve a real template.
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

// validateSplit walks a Split subtree enforcing the leaf-XOR-interior rule
// and the depth cap.
//
// path is a human-readable breadcrumb used only in error messages
// (e.g. `tab 0 ("main") split row[1]`). depth is the current nesting depth;
// the root of a tab is depth 0.
func validateSplit(s Split, path string, depth int) error {
	if depth > MaxDepth {
		return fmt.Errorf("layout: %s: nested too deep (max %d)", path, MaxDepth)
	}

	hasInterior := s.Direction != "" || len(s.Children) > 0
	hasLeaf := s.Cwd != "" || s.Command != "" || s.InitialInput != "" || len(s.Env) > 0

	if hasInterior && hasLeaf {
		return fmt.Errorf("layout: %s: a split is either a leaf (cwd/command/env/initial_input) or an interior node (direction/children); not both", path)
	}

	// Pure leaf: nothing more to check. We allow a "leaf with no cwd",
	// which represents an inherit-cwd-from-project terminal — equivalent
	// to `cwd: .`. Validation of cwd content (e.g. path traversal) is
	// the caller's job at apply time.
	if !hasInterior {
		return nil
	}

	// Interior: direction must be valid, children >= 2.
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
