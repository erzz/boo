package layoutpreview

import (
	"strings"
	"testing"

	"github.com/erzz/boo/internal/layout"
)

// These tests pin down the structural guarantees of the renderer
// (uniform width, exact height, correct shape for the canonical
// `triple` layout) without locking in the exact ASCII output —
// painted-cell content is harder to keep stable across editing rounds.

// rowOf builds a row split with the given children — small helper to
// keep test fixtures readable.
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
