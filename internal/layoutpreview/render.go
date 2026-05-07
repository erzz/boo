// Package layoutpreview renders a layout.Layout as ASCII art for use in
// `boo layouts` and the new-project TUI preview pane.
//
// The model
// ---------
// A tab's content is a recursive tree of splits (see internal/layout):
//
//   - A leaf is a single terminal cell.
//   - An interior node has direction = "row" (children laid left-to-right
//     in equal-width columns) or "column" (children stacked top-to-bottom
//     in equal-height rows) and >= 2 children.
//
// The renderer walks the tree allocating each node a rectangle, then paints
// borders and content. Adjacent siblings share a 1-cell border so the
// dividers coalesce — the right edge of the left sibling and the left edge
// of the right sibling occupy the same column, both drawing '|', producing
// a single-pixel divider.
//
// Public API: RenderTab and RenderLayout. Output is plain ASCII (+, -, |),
// no Unicode, exact width/height guarantees per RenderTab.
package layoutpreview

import (
	"fmt"
	"strings"

	"github.com/erzz/boo/internal/layout"
)

// Geometry constants.
//
// Min*  are *exterior* leaf dimensions (including borders). MinLeafHeight=4
// gives 2 interior rows so cwd + an annotation line both fit; smaller and
// annotations silently disappear, which is worse than a slightly larger
// preview.
const (
	MinLeafWidth  = 12
	MinLeafHeight = 4
)

// RenderTab paints one tab's split tree at the given width × height.
//
// Guarantees:
//   - Output is exactly height lines tall.
//   - Every line is exactly width characters wide (padded with spaces).
//   - ASCII only: +, -, |, space, plus content text.
//
// width / height below the Min* floors are bumped up rather than producing
// degenerate output — small previews are noisier than misleading.
func RenderTab(t layout.Tab, width, height int) string {
	if width < MinLeafWidth {
		width = MinLeafWidth
	}
	if height < MinLeafHeight {
		height = MinLeafHeight
	}
	grid := make([][]rune, height)
	for i := range grid {
		grid[i] = make([]rune, width)
		for j := range grid[i] {
			grid[i][j] = ' '
		}
	}
	drawSplit(grid, t.Root, 0, 0, width, height)
	var b strings.Builder
	for i, row := range grid {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(string(row))
	}
	return b.String()
}

// RenderLayout renders an entire layout: one labelled box per tab,
// stacked vertically. Tabs are stacked rather than placed side-by-side
// so layouts with many tabs don't wrap awkwardly at typical terminal
// widths.
//
// width is the target outer width per tab. Height per tab is derived
// from the tree's column-depth (how many vertically-stacked rows the
// tab needs).
func RenderLayout(l layout.Layout, width int) string {
	if width < MinLeafWidth+2 {
		width = MinLeafWidth + 2
	}
	if len(l.Tabs) == 0 {
		return "(no tabs)"
	}
	var blocks []string
	for i, t := range l.Tabs {
		title := fmt.Sprintf("Tab %d", i)
		if t.Name != "" {
			title = fmt.Sprintf("Tab %d %q", i, t.Name)
		}
		body := RenderTab(t, width, tabHeight(t))
		blocks = append(blocks, title+"\n"+body)
	}
	return strings.Join(blocks, "\n\n")
}

// tabHeight gives a tab enough vertical space to render every nested
// column without squashing leaves below MinLeafHeight.
//
// We measure "column depth" — the maximum number of vertically-stacked
// leaves on any path from root to leaf — and multiply by MinLeafHeight.
// A tab with no column splits gets exactly MinLeafHeight, which is
// what we want for the all-leaves and all-row cases (1x1x1, 1x2x1,
// 2x1x1, 2x2x1).
func tabHeight(t layout.Tab) int {
	d := columnDepth(t.Root)
	if d < 1 {
		d = 1
	}
	return d * MinLeafHeight
}

// columnDepth returns the height the subtree needs in MinLeafHeight units.
//
//   - leaf: 1
//   - row: max over children (children share the row's height)
//   - column: sum over children (children stack)
func columnDepth(s layout.Split) int {
	if s.IsLeaf() {
		return 1
	}
	switch s.Direction {
	case layout.DirColumn:
		total := 0
		for _, c := range s.Children {
			total += columnDepth(c)
		}
		return total
	default: // row (or invalid — render best-effort, validation already ran)
		max := 0
		for _, c := range s.Children {
			if d := columnDepth(c); d > max {
				max = d
			}
		}
		if max == 0 {
			max = 1
		}
		return max
	}
}

// drawSplit recursively paints split s into the rectangle (x,y)-(x+w,y+h).
//
// For interior nodes we allocate equal-sized cells per child along the
// split axis, with a 1-cell overlap so adjacent borders coalesce.
//
// Sizing: an interior node with N children gets a total width (or
// height, for column) of w; each child gets `(w + N - 1) / N` rounded
// such that consecutive children share their inner edge. Concretely
// we compute child rectangles end-to-end making each child's start =
// previous child's end - 1 (overlap by 1). This is the same trick the
// old renderer used for the binary case, generalised to N children.
func drawSplit(grid [][]rune, s layout.Split, x, y, w, h int) {
	if w <= 0 || h <= 0 {
		return
	}
	if s.IsLeaf() {
		drawLeaf(grid, s, x, y, w, h)
		return
	}
	n := len(s.Children)
	if n < 2 {
		// Defensive: validation enforces >=2, but if we somehow get
		// here just render the only child filling the rect.
		if n == 1 {
			drawSplit(grid, s.Children[0], x, y, w, h)
		}
		return
	}

	if s.Direction == layout.DirColumn {
		// Stack children vertically. Each child gets a slice of the
		// total height; consecutive children overlap by 1 row so
		// horizontal borders coalesce.
		offsets := allocate(h, n)
		for i, child := range s.Children {
			yi := y + offsets[i]
			hi := offsets[i+1] - offsets[i] + 1 // +1 for the shared border row
			if i == n-1 {
				// Last child: extend to end exactly.
				hi = (y + h) - yi
			}
			drawSplit(grid, child, x, yi, w, hi)
		}
		return
	}

	// Row (default).
	offsets := allocate(w, n)
	for i, child := range s.Children {
		xi := x + offsets[i]
		wi := offsets[i+1] - offsets[i] + 1
		if i == n-1 {
			wi = (x + w) - xi
		}
		drawSplit(grid, child, xi, y, wi, h)
	}
}

// allocate divides a span of length L into n+1 cumulative offsets such
// that each child occupies roughly L/n cells. The returned slice has
// length n+1: child i goes from offsets[i] to offsets[i+1] (with the
// shared-border overlap added by the caller).
//
// We accumulate fractionally so leftover cells are distributed across
// the early children rather than dumped into the last child — that
// way `1x2x2` renders with two equal-width left and right halves
// rather than [left=24, right=26].
func allocate(L, n int) []int {
	out := make([]int, n+1)
	for i := 0; i <= n; i++ {
		// Round to nearest using integer arithmetic.
		out[i] = (i*L + n/2) / n
	}
	out[0] = 0
	out[n] = L
	return out
}

// drawLeaf paints a single bordered cell with the split's annotation.
// Adjacent cells share borders — when this cell starts on a column or
// row that already has a border character from a neighbour, we
// overwrite with the same character (idempotent).
func drawLeaf(grid [][]rune, s layout.Split, x, y, w, h int) {
	if w < 3 || h < 3 {
		// Too small to draw a border. Just dot-fill so it's visible.
		for i := 0; i < h; i++ {
			for j := 0; j < w; j++ {
				if y+i < len(grid) && x+j < len(grid[0]) {
					grid[y+i][x+j] = '.'
				}
			}
		}
		return
	}
	// Borders.
	for j := 0; j < w; j++ {
		setRune(grid, x+j, y, '-')
		setRune(grid, x+j, y+h-1, '-')
	}
	for i := 0; i < h; i++ {
		setRune(grid, x, y+i, '|')
		setRune(grid, x+w-1, y+i, '|')
	}
	// Corners.
	setRune(grid, x, y, '+')
	setRune(grid, x+w-1, y, '+')
	setRune(grid, x, y+h-1, '+')
	setRune(grid, x+w-1, y+h-1, '+')

	// Content lines: cwd, then optional annotation. Centred vertically
	// inside the leaf for a calmer look.
	innerW := w - 2
	innerH := h - 2
	lines := leafLines(s, innerW)
	startY := y + 1 + (innerH-len(lines))/2
	if startY < y+1 {
		startY = y + 1
	}
	for li, line := range lines {
		if li >= innerH {
			break
		}
		writeText(grid, x+1, startY+li, innerW, line)
	}
}

// leafLines returns the 1–2 lines of text shown inside a leaf cell:
// cwd on the first line, then the most-relevant annotation if there
// is room. We don't try to show every annotation — it would force
// taller cells everywhere.
func leafLines(s layout.Split, innerW int) []string {
	cwd := s.Cwd
	if cwd == "" {
		cwd = "."
	}
	lines := []string{truncate(cwd, innerW)}
	switch {
	case s.Command != "":
		lines = append(lines, truncate("$ "+s.Command, innerW))
	case s.InitialInput != "":
		lines = append(lines, "(input)")
	case len(s.Env) > 0:
		lines = append(lines, fmt.Sprintf("(env x%d)", len(s.Env)))
	}
	return lines
}

func truncate(s string, max int) string {
	if max < 1 {
		return ""
	}
	if len(s) <= max {
		return s
	}
	if max == 1 {
		return "~"
	}
	return s[:max-1] + "~"
}

// writeText writes s into grid starting at (x,y), padding/truncating
// to fit width chars. Centres horizontally for a small layout.
func writeText(grid [][]rune, x, y, width int, s string) {
	if y < 0 || y >= len(grid) {
		return
	}
	if len(s) > width {
		s = truncate(s, width)
	}
	pad := (width - len(s)) / 2
	for i := 0; i < width; i++ {
		col := x + i
		if col < 0 || col >= len(grid[0]) {
			continue
		}
		r := ' '
		if i >= pad && i < pad+len(s) {
			r = rune(s[i-pad])
		}
		grid[y][col] = r
	}
}

func setRune(grid [][]rune, x, y int, r rune) {
	if y < 0 || y >= len(grid) {
		return
	}
	if x < 0 || x >= len(grid[0]) {
		return
	}
	grid[y][x] = r
}
