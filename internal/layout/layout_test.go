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

// TestMarshal_FieldNames: pins the user-visible YAML key names we own via json struct tags.
// sigs.k8s.io/yaml routes through encoding/json, so renaming a tag changes the on-disk format
// and silently breaks every existing user layout file. Tab.Root is "split:", not "root:".
func TestMarshal_FieldNames(t *testing.T) {
	l := Layout{
		Name: "check",
		Tabs: []Tab{{
			Name: "t",
			Root: Split{
				Direction: DirRow,
				Children:  []Split{{Cwd: "."}, {Cwd: "logs", Command: "tail -f x.log"}},
			},
		}},
	}
	data, err := Marshal(l)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	s := string(data)
	for _, key := range []string{"tabs:", "split:", "direction:", "children:", "cwd:", "command:"} {
		if !strings.Contains(s, key) {
			t.Errorf("marshalled YAML missing key %q:\n%s", key, s)
		}
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
		// 2-children invariant: Ghostty's split command halves a pane. Any count != 2 yields wrong
		// proportions or a no-op. All three bad counts must be rejected.
		{
			"interior with one child",
			Layout{Name: "x", Tabs: []Tab{{Root: Split{
				Direction: DirRow,
				Children:  []Split{leaf},
			}}}},
			"exactly 2 children",
		},
		{
			"interior with zero children",
			Layout{Name: "x", Tabs: []Tab{{Root: Split{Direction: DirRow}}}},
			"exactly 2 children",
		},
		{
			"interior with three children",
			Layout{Name: "x", Tabs: []Tab{{Root: Split{
				Direction: DirRow,
				Children:  []Split{leaf, leaf, leaf},
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

// TestSize_Validation: Size is interior-only and must be in (0,1).
// Other code (renderer, future JXA resize pass) trusts this contract, so the
// validator is the single gate.
func TestSize_Validation(t *testing.T) {
	cases := []struct {
		name    string
		split   Split
		wantErr string
	}{
		{"size on leaf rejected", Split{Cwd: ".", Size: 0.5}, "size is only valid on interior splits"},
		{"size zero allowed (means even)", Split{Direction: DirRow, Size: 0, Children: []Split{{Cwd: "."}, {Cwd: "."}}}, ""},
		{"size 0.5 allowed", Split{Direction: DirRow, Size: 0.5, Children: []Split{{Cwd: "."}, {Cwd: "."}}}, ""},
		{"size 0.99 allowed", Split{Direction: DirRow, Size: 0.99, Children: []Split{{Cwd: "."}, {Cwd: "."}}}, ""},
		{"size negative rejected", Split{Direction: DirRow, Size: -0.1, Children: []Split{{Cwd: "."}, {Cwd: "."}}}, "size must be between 0 and 1"},
		{"size 1.0 rejected", Split{Direction: DirRow, Size: 1.0, Children: []Split{{Cwd: "."}, {Cwd: "."}}}, "size must be between 0 and 1"},
		{"size 1.5 rejected", Split{Direction: DirRow, Size: 1.5, Children: []Split{{Cwd: "."}, {Cwd: "."}}}, "size must be between 0 and 1"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			l := Layout{Name: "x", Tabs: []Tab{{Name: "t", Root: c.split}}}
			err := l.Validate()
			if c.wantErr == "" {
				if err != nil {
					t.Errorf("expected nil, got %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Errorf("expected error containing %q, got %v", c.wantErr, err)
			}
		})
	}
}

// TestLeafPointers_DFSOrder: leaf pointers must come out in left-to-right DFS order.
// This contract is shared with the JXA walker, save pipeline, and the editor.
func TestLeafPointers_DFSOrder(t *testing.T) {
	// row( A, column( B, row( C, D ) ) ) → A,B,C,D
	root := Split{Direction: DirRow, Children: []Split{
		{Cwd: "A"},
		{Direction: DirColumn, Children: []Split{
			{Cwd: "B"},
			{Direction: DirRow, Children: []Split{{Cwd: "C"}, {Cwd: "D"}}},
		}},
	}}
	got := LeafPointers(&root)
	want := []string{"A", "B", "C", "D"}
	if len(got) != len(want) {
		t.Fatalf("got %d leaves, want %d", len(got), len(want))
	}
	for i, p := range got {
		if p.Cwd != want[i] {
			t.Errorf("leaf %d: got cwd %q, want %q", i, p.Cwd, want[i])
		}
	}
	// Mutation through the pointer reflects in the original tree.
	got[2].Command = "make"
	if root.Children[1].Children[1].Children[0].Command != "make" {
		t.Errorf("mutation via pointer didn't reach the original tree")
	}
}

// TestInteriorPointers_DFSPreOrder: interior nodes come out parent-first in DFS.
// Editor's divider mode cycles in this order; making the order explicit guards
// against surprise reorderings.
func TestInteriorPointers_DFSPreOrder(t *testing.T) {
	// row( column(A,B), row(C,D) )  → nodes: root-row, left-column, right-row
	root := Split{Direction: DirRow, Children: []Split{
		{Direction: DirColumn, Children: []Split{{Cwd: "A"}, {Cwd: "B"}}},
		{Direction: DirRow, Children: []Split{{Cwd: "C"}, {Cwd: "D"}}},
	}}
	got := InteriorPointers(&root)
	wantDirs := []string{DirRow, DirColumn, DirRow}
	if len(got) != len(wantDirs) {
		t.Fatalf("got %d interior nodes, want %d", len(got), len(wantDirs))
	}
	for i, p := range got {
		if p.Direction != wantDirs[i] {
			t.Errorf("interior %d: got direction %q, want %q", i, p.Direction, wantDirs[i])
		}
	}
	// Pure-leaf tree → no interior nodes.
	if got := InteriorPointers(&Split{Cwd: "."}); len(got) != 0 {
		t.Errorf("leaf-only tree should return nil/empty interior pointers, got %d", len(got))
	}
}
