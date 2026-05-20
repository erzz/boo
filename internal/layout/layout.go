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
	Name     string    `json:"name"`
	Tabs     []Tab     `json:"tabs,omitempty"`
	Variants []Variant `json:"variants,omitempty"`
}

// Variant is one responsive layout branch selected by terminal width.
// Exactly one variant in a responsive layout must be the default variant
// (no min_cols/max_cols) so callers have a fallback when width is unavailable.
type Variant struct {
	MinCols int   `json:"min_cols,omitempty"`
	MaxCols int   `json:"max_cols,omitempty"`
	Tabs    []Tab `json:"tabs"`
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

	// Size is the fractional space for the FIRST child (0,1) of an interior
	// node; the second child gets 1-Size. Optional; 0 (or omitted) means
	// "split evenly". Honored by the preview renderer and the open-layout
	// JXA resize pass. Only meaningful on interior nodes; the validator
	// rejects it on leaves.
	Size float64 `json:"size,omitempty"`

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

// IsResponsive reports whether l uses width-based variants instead of a single tab set.
func (l Layout) IsResponsive() bool {
	return len(l.Variants) > 0
}

// IsDefault reports whether v is the fallback responsive variant.
func (v Variant) IsDefault() bool {
	return v.MinCols == 0 && v.MaxCols == 0
}

// MatchesCols reports whether v applies to cols. cols must be > 0.
func (v Variant) MatchesCols(cols int) bool {
	if cols <= 0 {
		return false
	}
	if v.MinCols > 0 && cols < v.MinCols {
		return false
	}
	if v.MaxCols > 0 && cols > v.MaxCols {
		return false
	}
	return true
}

// Resolve returns a concrete, non-responsive layout for cols.
//
// Responsive selection rules:
//   - explicit variants are checked in file order; first match wins
//   - when cols <= 0 or nothing matches, the default variant is used
func (l Layout) Resolve(cols int) (Layout, error) {
	if !l.IsResponsive() {
		return l, nil
	}

	defaultIdx := -1
	for i, v := range l.Variants {
		if v.IsDefault() {
			if defaultIdx < 0 {
				defaultIdx = i
			}
			continue
		}
		if cols > 0 && v.MatchesCols(cols) {
			return Layout{Name: l.Name, Tabs: v.Tabs}, nil
		}
	}
	if defaultIdx >= 0 {
		return Layout{Name: l.Name, Tabs: l.Variants[defaultIdx].Tabs}, nil
	}
	return Layout{}, fmt.Errorf("layout: responsive layout %q has no default variant", l.Name)
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
	hasTabs := len(l.Tabs) > 0
	hasVariants := len(l.Variants) > 0
	if hasTabs && hasVariants {
		return errors.New("layout: tabs and variants are mutually exclusive")
	}
	if !hasTabs && !hasVariants {
		return errors.New("layout: at least one tab or responsive variant is required")
	}
	if hasTabs {
		if err := validateTabs(l.Tabs, "tab"); err != nil {
			return err
		}
		return nil
	}

	defaultCount := 0
	for i, v := range l.Variants {
		path := fmt.Sprintf("variant %d", i)
		if v.MinCols < 0 {
			return fmt.Errorf("layout: %s: min_cols must be >= 0, got %d", path, v.MinCols)
		}
		if v.MaxCols < 0 {
			return fmt.Errorf("layout: %s: max_cols must be >= 0, got %d", path, v.MaxCols)
		}
		if v.MaxCols > 0 && v.MinCols > 0 && v.MaxCols < v.MinCols {
			return fmt.Errorf("layout: %s: max_cols (%d) must be >= min_cols (%d)", path, v.MaxCols, v.MinCols)
		}
		if v.IsDefault() {
			defaultCount++
		}
		if err := validateTabs(v.Tabs, path+" tab"); err != nil {
			return err
		}
	}
	if defaultCount != 1 {
		return errors.New("layout: responsive layouts must declare exactly one default variant (no min_cols/max_cols)")
	}
	return nil
}

func validateTabs(tabs []Tab, prefix string) error {
	if len(tabs) == 0 {
		if prefix == "tab" {
			return errors.New("layout: at least one tab is required")
		}
		return fmt.Errorf("layout: %ss must declare at least one tab", prefix)
	}
	for i, t := range tabs {
		if err := validateSplit(t.Root, fmt.Sprintf("%s %d (%q)", prefix, i, t.Name), 0); err != nil {
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
		if s.Size != 0 {
			return fmt.Errorf("layout: %s: size is only valid on interior splits", path)
		}
		return nil
	}

	// Interior: direction must be valid, exactly 2 children, size in (0,1) when set.
	if s.Size != 0 && (s.Size <= 0 || s.Size >= 1) {
		return fmt.Errorf("layout: %s: size must be between 0 and 1 (exclusive), got %v", path, s.Size)
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

// LeafPointers returns pointers to every leaf in s in left-to-right DFS order.
// This is the canonical leaf-ordering contract: same order the JXA walker
// materialises panes, same order DescribeWindow returns terminals, same order
// the save pipeline uses to align leaves across re-saves. Callers that need
// to mutate a tree by leaf index (e.g. the picker's layout editor) should use
// this; callers that only read should prefer the value-returning equivalents.
func LeafPointers(s *Split) []*Split {
	if s == nil {
		return nil
	}
	if s.IsLeaf() {
		return []*Split{s}
	}
	var out []*Split
	for i := range s.Children {
		out = append(out, LeafPointers(&s.Children[i])...)
	}
	return out
}

// InteriorPointers returns pointers to every interior node in s in DFS pre-order
// (parent before children). Used by the layout editor's divider mode to cycle
// through resizable nodes in a stable order. Empty for leaf-only trees.
func InteriorPointers(s *Split) []*Split {
	if s == nil || s.IsLeaf() {
		return nil
	}
	out := []*Split{s}
	for i := range s.Children {
		out = append(out, InteriorPointers(&s.Children[i])...)
	}
	return out
}
