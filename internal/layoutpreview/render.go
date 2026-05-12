// Package layoutpreview renders a layout.Layout as ASCII art for `boo layouts`
// and the new-project TUI preview. The renderer walks the split tree allocating
// rectangles, then paints borders and content. Adjacent siblings share a 1-cell
// border so dividers coalesce. Output: plain ASCII (+, -, |), no Unicode.
package layoutpreview

import (
	"fmt"
	"strings"

	"github.com/erzz/boo/internal/layout"
)

// Geometry constants. MinLeafHeight=4 gives 2 interior rows so cwd + an
// annotation line both fit without silently disappearing.
const (
	MinLeafWidth  = 12
	MinLeafHeight = 4
)

// RenderTab paints one tab's split tree at width × height.
// Output is exactly height lines tall and width chars wide (space-padded).
// Inputs below Min* floors are bumped up rather than producing degenerate output.
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

// RenderLayout renders an entire layout: one labelled box per tab, stacked
// vertically. Tab height is derived from the tree's column-depth.
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

// Region describes a single rectangle in a rendered tab, in cell coordinates
// relative to the rendered string's top-left. Exactly one of LeafIndex /
// NodeIndex is >= 0; the other is -1. Used by the picker's layout editor to
// overlay highlights on the renderer's plain-ASCII output without coupling
// the renderer to a TUI styling library.
type Region struct {
	LeafIndex int // -1 if this Region describes an interior node
	NodeIndex int // -1 if this Region describes a leaf
	X, Y      int
	W, H      int
}

// RenderTabAnnotated renders a single tab and returns both the painted string
// and a slice of Regions describing every leaf and interior node in DFS order.
// Region geometry exactly matches what drawSplit painted. Callers can use
// LeafIndex / NodeIndex to map regions back to layout.LeafPointers /
// layout.InteriorPointers indices.
func RenderTabAnnotated(t layout.Tab, width, height int) (string, []Region) {
	if width < MinLeafWidth {
		width = MinLeafWidth
	}
	if height < MinLeafHeight {
		height = MinLeafHeight
	}
	out := RenderTab(t, width, height)
	var regions []Region
	walkRegions(t.Root, 0, 0, width, height, &regions, new(int), new(int))
	return out, regions
}

// walkRegions mirrors drawSplit's geometry exactly to record each node's
// rectangle. It MUST stay in lockstep with drawSplit — if drawSplit's
// allocation strategy changes (sized splits, padding, etc.), this walker
// changes the same way. leafCounter and nodeCounter are shared DFS counters
// so indices match layout.LeafPointers / layout.InteriorPointers ordering.
func walkRegions(s layout.Split, x, y, w, h int, out *[]Region, leafCounter, nodeCounter *int) {
	if w <= 0 || h <= 0 {
		return
	}
	if s.IsLeaf() {
		*out = append(*out, Region{LeafIndex: *leafCounter, NodeIndex: -1, X: x, Y: y, W: w, H: h})
		*leafCounter++
		return
	}
	n := len(s.Children)
	*out = append(*out, Region{LeafIndex: -1, NodeIndex: *nodeCounter, X: x, Y: y, W: w, H: h})
	*nodeCounter++
	if n < 2 {
		if n == 1 {
			walkRegions(s.Children[0], x, y, w, h, out, leafCounter, nodeCounter)
		}
		return
	}
	if s.Direction == layout.DirColumn {
		offsets := allocateSized(h, n, s.Size)
		for i, child := range s.Children {
			yi := y + offsets[i]
			hi := offsets[i+1] - offsets[i] + 1
			if i == n-1 {
				hi = (y + h) - yi
			}
			walkRegions(child, x, yi, w, hi, out, leafCounter, nodeCounter)
		}
		return
	}
	offsets := allocateSized(w, n, s.Size)
	for i, child := range s.Children {
		xi := x + offsets[i]
		wi := offsets[i+1] - offsets[i] + 1
		if i == n-1 {
			wi = (x + w) - xi
		}
		walkRegions(child, xi, y, wi, h, out, leafCounter, nodeCounter)
	}
}

// tabHeight returns a tab's required height in rows, based on column-depth
// (max vertically-stacked leaves on any root→leaf path) × MinLeafHeight.
func tabHeight(t layout.Tab) int {
	d := columnDepth(t.Root)
	if d < 1 {
		d = 1
	}
	return d * MinLeafHeight
}

// columnDepth returns height in MinLeafHeight units: leaf=1, row=max(children), column=sum(children).
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
// Interior nodes allocate equal-sized cells per child with 1-cell overlap so
// adjacent borders coalesce.
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
		// Defensive: validation enforces >=2, but render best-effort if not.
		if n == 1 {
			drawSplit(grid, s.Children[0], x, y, w, h)
		}
		return
	}

	if s.Direction == layout.DirColumn {
		// Stack children vertically; consecutive children overlap by 1 row so horizontal borders coalesce.
		offsets := allocateSized(h, n, s.Size)
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
	offsets := allocateSized(w, n, s.Size)
	for i, child := range s.Children {
		xi := x + offsets[i]
		wi := offsets[i+1] - offsets[i] + 1
		if i == n-1 {
			wi = (x + w) - xi
		}
		drawSplit(grid, child, xi, y, wi, h)
	}
}

// allocateSized returns cumulative offsets honoring `size` (fraction for child 0)
// when n==2 and size is in (0,1). Otherwise it falls back to even allocation.
// Floors the first child at 3 cells so leaf borders still render.
func allocateSized(L, n int, size float64) []int {
	if n != 2 || size <= 0 || size >= 1 {
		return allocate(L, n)
	}
	first := int(float64(L)*size + 0.5)
	const minCells = 3
	if first < minCells {
		first = minCells
	}
	if first > L-minCells {
		first = L - minCells
	}
	return []int{0, first, L}
}

// allocate divides span L into n+1 cumulative offsets, distributing leftover
// cells across early children for even sizing (e.g. 1x2x2 gets equal halves).
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

// drawLeaf paints a single bordered cell. Adjacent cells share borders (idempotent overwrite).
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

	// Content: cwd + optional annotation, vertically centred inside the leaf.
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

// leafLines returns 1–2 lines of text for a leaf cell: cwd, then the most
// relevant annotation if there is room.
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

// writeText writes s into grid at (x,y), padding/truncating to width chars, horizontally centred.
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
