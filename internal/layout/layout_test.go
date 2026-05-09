package layout

import (
	"strings"
	"testing"
)

// TestDefault_IsValid: Default() must produce a well-formed, validating Layout.
func TestDefault_IsValid(t *testing.T) {
	if err := Default().Validate(); err != nil {
		t.Fatalf("Default invalid: %v", err)
	}
}

// TestParse_RoundTrip: Marshal+Parse on a non-trivial tree must round-trip cleanly.
// Exercises leaf/interior splits, both directions, and per-leaf optional fields (command, env, initial_input).
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
	// env drop (was a regression in TOML land)
	gotEnv := right.Children[1].Env["LOG_LEVEL"]
	if gotEnv != "debug" {
		t.Fatalf("env dropped on round-trip: got %q", gotEnv)
	}
}

// TestValidate_Errors: each Validate failure mode in its own case; name matches the user-visible mistake.
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

// TestValidate_RejectsExcessiveDepth: chains MaxDepth+1 deep must fail with "nested too deep".
func TestValidate_RejectsExcessiveDepth(t *testing.T) {
	leaf := Split{Cwd: "."}
	// Build a row chain MaxDepth+1 levels deep.
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

// TestParse_UnknownFieldErrors: typos in YAML field names must error, not silently revert to default.
func TestParse_UnknownFieldErrors(t *testing.T) {
	cases := []struct {
		name    string
		yaml    string
		wantKey string
	}{
		{
			"unknown top-level field",
			`name: test
windws:
  - name: main
    split:
      cwd: .
`,
			"windws",
		},
		{
			"unknown field inside split",
			`name: test
tabs:
  - name: main
    split:
      comand: nvim .
`,
			"comand",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := Parse([]byte(c.yaml))
			if err == nil {
				t.Fatalf("Parse(%q): expected error for unknown field, got nil", c.name)
			}
			msg := err.Error()
			if !strings.Contains(msg, "unknown field") {
				t.Errorf("error should contain 'unknown field'; got: %v", err)
			}
			if !strings.Contains(msg, c.wantKey) {
				t.Errorf("error should name offending key %q; got: %v", c.wantKey, err)
			}
			if !strings.Contains(msg, "layout: parse") {
				t.Errorf("error should carry 'layout: parse' prefix; got: %v", err)
			}
		})
	}
}

// TestIsLeaf_Contract: IsLeaf predicate must stay consistent with Validate;
// drift between them would cause renderer/JXA to branch incorrectly.
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
