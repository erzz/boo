package layoutpreview

import (
	"strings"
	"testing"

	"github.com/erzz/boo/internal/layout"
)

// render_test.go pins structural guarantees of the renderer (uniform width, exact height, correct shape)
// without locking in exact ASCII output — painted-cell content is harder to keep stable.

// rowOf builds a row split — small helper to keep test fixtures readable.
func rowOf(children ...layout.Split) layout.Split {
	return layout.Split{Direction: layout.DirRow, Children: children}
}

// colOf builds a column split with the given children.
func colOf(children ...layout.Split) layout.Split {
	return layout.Split{Direction: layout.DirColumn, Children: children}
}

// leaf returns an empty leaf — the renderer treats Cwd:"" as ".".
func leaf() layout.Split { return layout.Split{Cwd: "."} }

func TestRenderTab_OutputIsRectangular(t *testing.T) {
	// Every line must be exactly `width` chars and there must be
	// exactly `height` lines. Off-by-one bugs in the grid sizing
	// would silently produce a layout that misaligns under another
	// box.
	tab := layout.Tab{Name: "x", Root: rowOf(leaf(), leaf())}
	const w, h = 30, 5
	out := RenderTab(tab, w, h)
	lines := strings.Split(out, "\n")
	if len(lines) != h {
		t.Fatalf("got %d lines, want %d:\n%s", len(lines), h, out)
	}
	for i, l := range lines {
		// ASCII-only renderer, so len() == rune count.
		if len(l) != w {
			t.Errorf("line %d: width %d, want %d (%q)", i, len(l), w, l)
		}
	}
}

func TestRenderTab_TripleHasOneLeftAndTwoStackedRight(t *testing.T) {
	// The user-facing reason this package exists. If this shape is
	// wrong, the `triple` built-in is misleading and the new-project
	// preview will be wrong on the layout it was built for.
	//
	// Expected interior (borders elided, leaves marked L/T/B):
	//
	//   L T
	//   L T
	//   L B
	//   L B
	//
	// In the new tree model that's: row( leaf, column(leaf, leaf) ).
	tab := layout.Tab{Root: rowOf(
		leaf(),
		colOf(leaf(), leaf()),
	)}
	const w, h = 40, 8
	out := RenderTab(tab, w, h)
	lines := strings.Split(out, "\n")
	if len(lines) != h {
		t.Fatalf("expected %d lines, got %d:\n%s", h, len(lines), out)
	}
	// Find the splitting column: it's the position where the top
	// border has a '+' between the two top corners.
	top := lines[0]
	splitCol := -1
	for i := 1; i < len(top)-1; i++ {
		if top[i] == '+' {
			splitCol = i
			break
		}
	}
	if splitCol < 0 {
		t.Fatalf("no vertical split found in top border: %q", top)
	}
	// Find a horizontal divider in the right half (a row that has
	// only '-' or '+' in the right-half columns, i.e. between the
	// outer borders).
	foundDivider := false
	for i, line := range lines {
		if i == 0 || i == len(lines)-1 {
			continue // skip outer borders
		}
		right := line[splitCol+1 : len(line)-1]
		if len(right) == 0 {
			continue
		}
		isDivider := true
		for _, r := range right {
			if r != '-' && r != '+' {
				isDivider = false
				break
			}
		}
		if isDivider {
			foundDivider = true
			// On the same row, the left half should NOT be a
			// divider — the left pane is continuous.
			left := line[1:splitCol]
			for _, r := range left {
				if r == '-' || r == '+' {
					t.Errorf("row %d: left half has a divider char where left pane should be continuous: %q", i, left)
				}
			}
			break
		}
	}
	if !foundDivider {
		t.Errorf("no horizontal divider found in right half (right pane should be split top/bottom):\n%s", out)
	}
}

func TestRenderTab_AdjacentLeavesShareABorder(t *testing.T) {
	// Two leaves side-by-side must NOT produce '||'. The shared
	// edge should be one '|', not two. (Same idea for stacked
	// leaves and shared '+' corners.)
	tab := layout.Tab{Root: rowOf(leaf(), leaf())}
	out := RenderTab(tab, 30, 4)
	if strings.Contains(out, "||") {
		t.Errorf("shared border doubled (||):\n%s", out)
	}
	if strings.Contains(out, "++") {
		t.Errorf("shared corner doubled (++):\n%s", out)
	}
}

func TestRenderTab_StackedLeavesShareABorder(t *testing.T) {
	// Same shared-border invariant on the column axis: two leaves
	// stacked vertically should share one '-' row, not produce two
	// adjacent '-' rows visible as a thick horizontal line.
	tab := layout.Tab{Root: colOf(leaf(), leaf())}
	out := RenderTab(tab, 20, 8)
	// A doubled horizontal border would show up as two adjacent
	// rows of all '-' (with '+' at the corners). Look for that.
	lines := strings.Split(out, "\n")
	for i := 0; i < len(lines)-1; i++ {
		if isAllDashes(lines[i]) && isAllDashes(lines[i+1]) {
			t.Errorf("doubled horizontal border at rows %d/%d:\n%s", i, i+1, out)
		}
	}
}

// isAllDashes reports whether the line is composed only of '+' and '-'
// (the characters used for horizontal dividers in this renderer).
func isAllDashes(s string) bool {
	if len(s) == 0 {
		return false
	}
	for _, r := range s {
		if r != '-' && r != '+' {
			return false
		}
	}
	return true
}

func TestRenderLayout_StacksTabsWithLabels(t *testing.T) {
	l := layout.Layout{Name: "two", Tabs: []layout.Tab{
		{Name: "a", Root: leaf()},
		{Name: "b", Root: leaf()},
	}}
	out := RenderLayout(l, 20)
	if !strings.Contains(out, `Tab 0 "a"`) {
		t.Errorf("missing tab 0 label:\n%s", out)
	}
	if !strings.Contains(out, `Tab 1 "b"`) {
		t.Errorf("missing tab 1 label:\n%s", out)
	}
	// Tab 1 label must appear AFTER tab 0's box (vertical stack,
	// not side-by-side).
	if strings.Index(out, "Tab 0") > strings.Index(out, "Tab 1") {
		t.Errorf("tab order wrong:\n%s", out)
	}
}

func TestRenderTab_LeafShowsCommandAnnotation(t *testing.T) {
	// `$ <cmd>` must appear under the cwd when a command is set.
	// This is what makes the preview useful for layouts with
	// non-trivial setup; if it silently disappears the preview
	// teaches users the wrong thing.
	tab := layout.Tab{Root: layout.Split{Cwd: ".", Command: "make watch"}}
	out := RenderTab(tab, 30, 4)
	if !strings.Contains(out, "$ make watch") {
		t.Errorf("expected '$ make watch' annotation:\n%s", out)
	}
}

func TestRenderTab_DefaultsBelowMinimums(t *testing.T) {
	// Caller passes degenerate dimensions → renderer bumps them up.
	// Important for the TUI which may compute a width based on
	// available space and end up with something silly during layout.
	out := RenderTab(layout.Tab{Root: leaf()}, 1, 1)
	lines := strings.Split(out, "\n")
	if len(lines) < MinLeafHeight {
		t.Errorf("height not bumped to MinLeafHeight: got %d lines", len(lines))
	}
	if len(lines[0]) < MinLeafWidth {
		t.Errorf("width not bumped to MinLeafWidth: got width %d", len(lines[0]))
	}
}

// TestRenderTabAnnotated_RegionsMatchPainted geometry: every Region in the
// returned slice corresponds to either a leaf or an interior node, and its
// rectangle exactly matches what drawSplit painted. This is the contract the
// picker leans on to overlay highlights — drift would mis-paint the cursor.
func TestRenderTabAnnotated_RegionsMatchPainted(t *testing.T) {
	// triple: row( leaf, column(leaf, leaf) ) → 3 leaves, 2 interior nodes.
	tab := layout.Tab{Root: rowOf(leaf(), colOf(leaf(), leaf()))}
	const w, h = 40, 8
	_, regions := RenderTabAnnotated(tab, w, h)

	var leaves, interior []Region
	for _, r := range regions {
		switch {
		case r.LeafIndex >= 0:
			leaves = append(leaves, r)
		case r.NodeIndex >= 0:
			interior = append(interior, r)
		default:
			t.Errorf("region has neither leaf nor node index: %+v", r)
		}
	}
	if len(leaves) != 3 {
		t.Fatalf("expected 3 leaf regions, got %d: %+v", len(leaves), leaves)
	}
	if len(interior) != 2 {
		t.Fatalf("expected 2 interior regions, got %d: %+v", len(interior), interior)
	}
	// Indices ascend in DFS order.
	for i, r := range leaves {
		if r.LeafIndex != i {
			t.Errorf("leaves[%d].LeafIndex = %d, want %d", i, r.LeafIndex, i)
		}
	}
	for i, r := range interior {
		if r.NodeIndex != i {
			t.Errorf("interior[%d].NodeIndex = %d, want %d", i, r.NodeIndex, i)
		}
	}
	// Outer node spans the whole tab.
	root := interior[0]
	if root.X != 0 || root.Y != 0 || root.W != w || root.H != h {
		t.Errorf("root interior region should span the whole tab, got %+v", root)
	}
	// Leaf 0 is the left pane; its right edge meets the inner column's left edge.
	left := leaves[0]
	innerCol := interior[1]
	if left.X+left.W != innerCol.X+1 {
		// The +1 captures the shared 1-cell border with the right subtree.
		t.Errorf("left leaf right edge (%d) and inner column left edge (%d) should share a border", left.X+left.W, innerCol.X)
	}
}

// TestRenderTabAnnotated_HonorsSize: a sized row split makes the left leaf's
// region wider than 50%. If this test fails, drawSplit and walkRegions have
// drifted apart — that drift would make the picker's highlight overlay the
// wrong rectangle.
func TestRenderTabAnnotated_HonorsSize(t *testing.T) {
	tab := layout.Tab{Root: layout.Split{
		Direction: layout.DirRow,
		Size:      0.75,
		Children:  []layout.Split{leaf(), leaf()},
	}}
	const w, h = 40, 6
	_, regions := RenderTabAnnotated(tab, w, h)
	var leafRegions []Region
	for _, r := range regions {
		if r.LeafIndex >= 0 {
			leafRegions = append(leafRegions, r)
		}
	}
	if len(leafRegions) != 2 {
		t.Fatalf("expected 2 leaves, got %d", len(leafRegions))
	}
	// At size=0.75, leaf 0 should occupy ~30 cells, leaf 1 ~10.
	if leafRegions[0].W <= leafRegions[1].W {
		t.Errorf("size=0.75 should make leaf 0 wider than leaf 1; got %d vs %d", leafRegions[0].W, leafRegions[1].W)
	}
	if leafRegions[0].W < 25 || leafRegions[0].W > 33 {
		t.Errorf("leaf 0 width should be ~30 cells at size=0.75 in width=40; got %d", leafRegions[0].W)
	}
}
