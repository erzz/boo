package layout

import (
	"strings"
	"testing"
)

// Default exists so callers that can't resolve a real template still get a
// well-formed Layout. The contract: it must validate. Anything else is
// implementation detail.
func TestDefault_IsValid(t *testing.T) {
	if err := Default().Validate(); err != nil {
		t.Fatalf("Default invalid: %v", err)
	}
}

// Round-trip via Marshal+Parse on a non-trivial tree. This is the load-bearing
// invariant for `boo save`: write to disk, read back, get the same structure.
// We exercise both leaf and interior splits, both directions, and per-leaf
// optional fields (command, env, initial_input).
func TestParse_RoundTrip(t *testing.T) {
	in := Layout{
		Name: "round-trip",
		Tabs: []Tab{
			{
				Name: "edit",
				Root: Split{Cwd: ".", Command: "nvim ."},
			},
			{
				Name: "dev",
				Root: Split{
					Direction: DirRow,
					Children: []Split{
						{Cwd: "."},
						{
							Direction: DirColumn,
							Children: []Split{
								{Cwd: ".", Command: "npm run dev"},
								{Cwd: "logs", Command: "tail -f app.log",
									Env: map[string]string{"LOG_LEVEL": "debug"}},
							},
						},
					},
				},
			},
		},
	}
	data, err := Marshal(in)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	out, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v\n%s", err, data)
	}
	if out.Name != in.Name || len(out.Tabs) != 2 {
		t.Fatalf("round-trip lost top-level shape: %+v", out)
	}
	// Tab 1's tree: row of [leaf, column of [leaf, leaf]].
	root := out.Tabs[1].Root
	if root.Direction != DirRow || len(root.Children) != 2 {
		t.Fatalf("tab 1 root should be row of 2; got %+v", root)
	}
	right := root.Children[1]
	if right.Direction != DirColumn || len(right.Children) != 2 {
		t.Fatalf("tab 1 right child should be column of 2; got %+v", right)
	}
	// Per-leaf optional carry-through that bit us once before in TOML
	// land — env was dropping silently.
	gotEnv := right.Children[1].Env["LOG_LEVEL"]
	if gotEnv != "debug" {
		t.Fatalf("env dropped on round-trip: got %q", gotEnv)
	}
}

// Each Validate failure mode is its own test case. Naming each case after
// the user-visible mistake makes it obvious from `go test -v` what regressed.
func TestValidate_Errors(t *testing.T) {
	leaf := Split{Cwd: "."}
	cases := []struct {
		name string
		l    Layout
		want string
	}{
		{
			"name missing",
			Layout{Tabs: []Tab{{Root: leaf}}},
			"name is required",
		},
		{
			"no tabs",
			Layout{Name: "x"},
			"at least one tab",
		},
		{
			"interior+leaf XOR violated",
			Layout{Name: "x", Tabs: []Tab{{Root: Split{
				Direction: DirRow,
				Children:  []Split{leaf, leaf},
				Cwd:       ".", // illegal: also has leaf data
			}}}},
			"either a leaf",
		},
		{
			"interior with bad direction",
			Layout{Name: "x", Tabs: []Tab{{Root: Split{
				Direction: "sideways",
				Children:  []Split{leaf, leaf},
			}}}},
			"is not one of row|column",
		},
		{
			"interior with one child is degenerate",
			Layout{Name: "x", Tabs: []Tab{{Root: Split{
				Direction: DirRow,
				Children:  []Split{leaf},
			}}}},
			"exactly 2 children",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.l.Validate()
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("expected error containing %q, got %v", c.want, err)
			}
		})
	}
}

// MaxDepth must reject pathological nesting before any downstream code
// (renderer, JXA walker) gets a chance to misbehave. We build a chain
// MaxDepth+1 deep and confirm it errors with a useful message.
func TestValidate_RejectsExcessiveDepth(t *testing.T) {
	leaf := Split{Cwd: "."}
	// Chain of nested rows: each level wraps the previous in row[leaf, prev].
	cur := leaf
	for i := 0; i <= MaxDepth; i++ {
		cur = Split{Direction: DirRow, Children: []Split{leaf, cur}}
	}
	l := Layout{Name: "deep", Tabs: []Tab{{Root: cur}}}
	err := l.Validate()
	if err == nil {
		t.Fatalf("expected depth error, got nil")
	}
	if !strings.Contains(err.Error(), "nested too deep") {
		t.Fatalf("expected 'nested too deep', got %v", err)
	}
}

// IsLeaf is the canonical predicate downstream code branches on. If its
// definition drifts from Validate's, things will misbehave silently
// (renderer rendering the wrong shape, JXA splitting the wrong terminal).
// Pin it down here.
func TestIsLeaf_Contract(t *testing.T) {
	leaf := Split{Cwd: "."}
	if !leaf.IsLeaf() {
		t.Errorf("a cwd-only split should be a leaf")
	}
	bareLeaf := Split{}
	if !bareLeaf.IsLeaf() {
		t.Errorf("an empty split is treated as a leaf (inherit cwd)")
	}
	interior := Split{Direction: DirRow, Children: []Split{leaf, leaf}}
	if interior.IsLeaf() {
		t.Errorf("an interior split must not report as leaf")
	}
}
