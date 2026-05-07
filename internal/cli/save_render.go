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
	fmt.Fprintln(w, header)
	fmt.Fprintln(w)

	for _, td := range d.ChangedTabs {
		fmt.Fprintln(w, renderTabDiff(td))
		fmt.Fprintln(w)
	}

	if len(d.LossReasons) > 0 {
		fmt.Fprintln(w, "Unrecoverable on next save:")
		for _, r := range d.LossReasons {
			fmt.Fprintf(w, "  - %s\n", r)
		}
		fmt.Fprintln(w)
	}

	// Structural and lossy outcomes both deserve the limitation
	// explainer. Even a "clean" structural diff may be hiding a real
	// loss the differ can't see: Ghostty's AppleScript dictionary
	// returns a flat terminal list per tab, so a tree like
	// `vertical → [horizontal, horizontal]` is captured as three
	// row-of-three splits. The user never sees that flattening unless
	// we tell them. Silent saves stay silent — we never reached this
	// branch anyway.
	fmt.Fprintln(w, "Why this happens:")
	fmt.Fprintln(w, "  Ghostty's AppleScript API returns a flat list of terminals per")
	fmt.Fprintln(w, "  tab — it does not expose split direction, nesting, or the")
	fmt.Fprintln(w, "  command/env that launched a terminal. boo can reopen the same")
	fmt.Fprintln(w, "  number of panes with the same cwds, but any nested split tree")
	fmt.Fprintln(w, "  will be flattened into a single row, and any field marked '!'")
	fmt.Fprintln(w, "  above will be dropped.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Recommended:")
	fmt.Fprintln(w, "  If you rely on a specific split shape, command, or env, write a")
	fmt.Fprintln(w, "  layout TOML by hand and use it via 'boo new --layout <name>' or")
	fmt.Fprintln(w, "  by editing the project's layout.toml directly. Hand-authored")
	fmt.Fprintln(w, "  layouts are reapplied verbatim on every launch.")
	fmt.Fprintln(w)
}

// renderTabDiff returns a multi-line string showing one tab side-by-side
// (before → after). Both sides use the same fixed cell width so the
// columns line up regardless of content.
//
// Layout is intentionally simple ASCII: no Unicode box-drawing, no colour.
// That keeps the output greppable, terminal-portable, and trivial to test
// against golden strings.
func renderTabDiff(td TabDiff) string {
	left := renderTab(td.Prev, td.LossyCells)
	right := renderTab(td.Next, nil)
	title := fmt.Sprintf("Tab %d %s", td.Index, quotedOrEmpty(td.Name))

	var b strings.Builder
	fmt.Fprintln(&b, title)

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
		fmt.Fprintf(&b, "%s   %s%s\n", padRight(l, leftWidth), arrow, r)
	}
	return strings.TrimRight(b.String(), "\n")
}

// renderTab draws one side of the diff: a row of fixed-width cells plus
// a label row showing the split's recovered properties (cwd; "(cmd)" if a
// command will be set on apply; direction for non-primary splits). Cells
// listed in lossyCells get a trailing "!" on the marker row.
//
// A nil tab renders as "(removed)".
func renderTab(t *layout.Tab, lossyCells []int) string {
	if t == nil {
		return "(removed)"
	}
	if len(t.Splits) == 0 {
		return "(empty tab)"
	}

	lossySet := make(map[int]bool, len(lossyCells))
	for _, j := range lossyCells {
		lossySet[j] = true
	}

	var (
		topB    strings.Builder
		cwdB    strings.Builder
		extraB  strings.Builder
		dirB    strings.Builder
		bottomB strings.Builder
	)
	for j := range t.Splits {
		writeBorder(&topB, j == 0)
		writeBorder(&bottomB, j == 0)
	}

	for j, s := range t.Splits {
		if j == 0 {
			cwdB.WriteString("|")
			extraB.WriteString("|")
			dirB.WriteString("|")
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

		dirLabel := ""
		if j > 0 {
			dirLabel = s.Direction
			if dirLabel == "" {
				dirLabel = "?"
			}
		}
		if lossySet[j] {
			dirLabel = strings.TrimSpace(dirLabel) + " !"
		}
		dirB.WriteString(padCell(dirLabel))
		dirB.WriteString("|")
	}

	return strings.Join([]string{
		topB.String(),
		cwdB.String(),
		extraB.String(),
		dirB.String(),
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
