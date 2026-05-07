package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/erzz/boo/internal/layout"
)

// Cell width inside box (excluding the bordering '|'). Keeps boxes
// predictable across tabs and makes golden tests trivially diffable.
const renderCellWidth = 12

// renderDiff writes the human-readable before/after view of a SaveDiff to
// w. Caller is expected to have already printed the "Captured N tab(s),
// M split(s)…" summary line. renderDiff prints nothing when the outcome
// is OutcomeSilent — silent saves are silent.
func renderDiff(d SaveDiff, w io.Writer) {
	if d.Outcome == OutcomeSilent {
		return
	}

	header := "Layout will change:"
	if d.Outcome == OutcomeLossy {
		header = "Layout will change (some data CANNOT be recovered, marked with !):"
	}
	_, _ = fmt.Fprintln(w, header)
	_, _ = fmt.Fprintln(w)

	for _, td := range d.ChangedTabs {
		_, _ = fmt.Fprintln(w, renderTabDiff(td))
		_, _ = fmt.Fprintln(w)
	}

	if len(d.LossReasons) > 0 {
		_, _ = fmt.Fprintln(w, "Unrecoverable on next save:")
		for _, r := range d.LossReasons {
			_, _ = fmt.Fprintf(w, "  - %s\n", r)
		}
		_, _ = fmt.Fprintln(w)
	}

	// Structural and lossy outcomes both deserve the limitation
	// explainer. Even a "clean" structural diff may be hiding a real
	// loss the differ can't see: Ghostty's AppleScript dictionary
	// returns a flat terminal list per tab, so a tree like
	// `column(row(A, B), row(C, D))` is captured as four leaves in a
	// row. mergeForSave will preserve the previous tree shape when
	// the leaf count matches; if it doesn't, the tab gets flattened
	// into a right-chain row and that flattening shows up as a
	// structural change. Silent saves stay silent — we never reached
	// this branch anyway.
	_, _ = fmt.Fprintln(w, "Why this happens:")
	_, _ = fmt.Fprintln(w, "  Ghostty's AppleScript API returns a flat list of terminals per")
	_, _ = fmt.Fprintln(w, "  tab — it does not expose split direction, nesting, or the")
	_, _ = fmt.Fprintln(w, "  command/env that launched a terminal. boo can reopen the same")
	_, _ = fmt.Fprintln(w, "  number of panes with the same cwds, but if the captured pane")
	_, _ = fmt.Fprintln(w, "  count differs from the previous layout the tree will be flattened")
	_, _ = fmt.Fprintln(w, "  into a right-leaning row, and any field marked '!' above will be")
	_, _ = fmt.Fprintln(w, "  dropped.")
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "Recommended:")
	_, _ = fmt.Fprintln(w, "  If you rely on a specific tree shape, command, or env, write a")
	_, _ = fmt.Fprintln(w, "  layout YAML by hand and use it via 'boo new --layout <name>' or")
	_, _ = fmt.Fprintln(w, "  by editing the project's layout.yaml directly. Hand-authored")
	_, _ = fmt.Fprintln(w, "  layouts are reapplied verbatim on every launch — and survive")
	_, _ = fmt.Fprintln(w, "  re-saves intact when the captured pane count matches.")
	_, _ = fmt.Fprintln(w)
}

// renderTabDiff returns a multi-line string showing one tab side-by-side
// (before → after). Both sides use the same fixed cell width so the
// columns line up regardless of content.
//
// Layout is intentionally simple ASCII: no Unicode box-drawing, no colour.
// That keeps the output greppable, terminal-portable, and trivial to test
// against golden strings.
func renderTabDiff(td TabDiff) string {
	left := renderTab(td.Prev, td.LossyLeaves)
	right := renderTab(td.Next, nil)
	title := fmt.Sprintf("Tab %d %s", td.Index, quotedOrEmpty(td.Name))

	var b strings.Builder
	_, _ = fmt.Fprintln(&b, title)

	leftLines := strings.Split(left, "\n")
	rightLines := strings.Split(right, "\n")
	maxLines := len(leftLines)
	if n := len(rightLines); n > maxLines {
		maxLines = n
	}

	leftWidth := visibleWidth(leftLines)

	for i := 0; i < maxLines; i++ {
		l := lineAt(leftLines, i)
		r := lineAt(rightLines, i)
		arrow := "  "
		if i == middleLine(maxLines) {
			arrow = "→ "
		}
		_, _ = fmt.Fprintf(&b, "%s   %s%s\n", padRight(l, leftWidth), arrow, r)
	}
	return strings.TrimRight(b.String(), "\n")
}

// renderTab draws one side of the diff as a row of fixed-width cells,
// one per leaf in left-to-right depth-first order. Each cell shows:
//   - cwd
//   - "(cmd)" / "(input)" / "(env xN)" if the leaf carries that
//   - leaf index "L0..LN" plus a trailing "!" for entries listed in
//     lossyLeaves
//
// The flat-row presentation deliberately doesn't try to re-create the
// 2-D tree shape — `internal/layoutpreview` does that for the catalogue
// preview. Here the structural-change outcome and the "Why this
// happens" footer carry the "the tree shape changed" message, and the
// row format keeps before / after horizontally aligned for an easy
// visual scan.
//
// A nil tab renders as "(removed)".
func renderTab(t *layout.Tab, lossyLeaves []int) string {
	if t == nil {
		return "(removed)"
	}
	leaves := collectLeaves(t.Root)
	if len(leaves) == 0 {
		return "(empty tab)"
	}

	lossySet := make(map[int]bool, len(lossyLeaves))
	for _, j := range lossyLeaves {
		lossySet[j] = true
	}

	var (
		topB    strings.Builder
		cwdB    strings.Builder
		extraB  strings.Builder
		idxB    strings.Builder
		bottomB strings.Builder
	)
	for j := range leaves {
		writeBorder(&topB, j == 0)
		writeBorder(&bottomB, j == 0)
	}

	for j, s := range leaves {
		if j == 0 {
			cwdB.WriteString("|")
			extraB.WriteString("|")
			idxB.WriteString("|")
		}
		cwdB.WriteString(padCell(displayCwd(s.Cwd)))
		cwdB.WriteString("|")

		extra := ""
		if s.Command != "" {
			extra = "(cmd)"
		} else if s.InitialInput != "" {
			extra = "(input)"
		} else if len(s.Env) > 0 {
			extra = fmt.Sprintf("(env x%d)", len(s.Env))
		}
		extraB.WriteString(padCell(extra))
		extraB.WriteString("|")

		idxLabel := fmt.Sprintf("L%d", j)
		if lossySet[j] {
			idxLabel += " !"
		}
		idxB.WriteString(padCell(idxLabel))
		idxB.WriteString("|")
	}

	return strings.Join([]string{
		topB.String(),
		cwdB.String(),
		extraB.String(),
		idxB.String(),
		bottomB.String(),
	}, "\n")
}

// writeBorder writes one cell's worth of horizontal border, plus the
// leading "+" before the first cell.
func writeBorder(b *strings.Builder, first bool) {
	if first {
		b.WriteString("+")
	}
	b.WriteString(strings.Repeat("-", renderCellWidth))
	b.WriteString("+")
}

// padCell truncates or right-pads s to renderCellWidth, including leading
// and trailing spaces inside the cell. Truncation keeps the last char as
// "…" (ASCII "~") so the user knows it was clipped.
func padCell(s string) string {
	const ellipsis = "~"
	w := renderCellWidth
	// Visual padding on both sides for readability: " content    ".
	// Effective text capacity = w - 2.
	cap := w - 2
	if cap < 1 {
		cap = 1
	}
	if len(s) > cap {
		s = s[:cap-1] + ellipsis
	}
	return " " + s + strings.Repeat(" ", w-2-len(s)) + " "
}

func padRight(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}

func displayCwd(cwd string) string {
	if cwd == "" {
		return "."
	}
	return cwd
}

func quotedOrEmpty(name string) string {
	if name == "" {
		return ""
	}
	return fmt.Sprintf("%q", name)
}

func lineAt(lines []string, i int) string {
	if i < len(lines) {
		return lines[i]
	}
	return ""
}

func visibleWidth(lines []string) int {
	max := 0
	for _, l := range lines {
		if n := len(l); n > max {
			max = n
		}
	}
	return max
}

func middleLine(n int) int {
	if n <= 0 {
		return 0
	}
	return n / 2
}
