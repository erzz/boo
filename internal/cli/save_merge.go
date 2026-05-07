package cli

import (
	"fmt"

	"github.com/erzz/boo/internal/layout"
)

// mergeForSave folds invisible-but-stable fields from prev into the
// flat captured layout, returning the layout to actually save and a
// list of human-readable notes about prev data that could NOT be
// carried forward.
//
// Why a merge exists at all
// -------------------------
// Ghostty's AppleScript API doesn't tell us the launch command, env, or
// initial_input of a live terminal — it only exposes id / title /
// working directory. It also returns terminals as a flat list per tab,
// so we lose any nested split tree on capture. A naive "snapshot the
// live window" save therefore wipes those fields AND flattens the tree
// on every re-save, which makes `boo save` hostile to anyone with a
// hand-authored layout.
//
// But we already have those fields (and the tree shape) on disk in the
// previous layout file. As long as the captured leaf count matches the
// previous tab's leaf count, we can keep prev's tree shape and merge
// invisibles by leaf order. Cwd we always take from capture, because
// cwd is the one thing that legitimately changes when the user `cd`-s
// around.
//
// Match policy (per tab)
// ----------------------
// Same leaf count as prev → keep prev's tree shape verbatim, walk it
// in left-to-right depth-first leaf order, and zip captured cwds onto
// each leaf. Command / env / initial_input are carried forward from
// prev. This is the lossless path: hand-authored trees survive
// re-saves intact.
//
// Captured leaf count differs from prev → we cannot keep the tree
// shape (the schema requires interior nodes to have exactly 2
// children, so an N-leaf tab from capture has to be rebuilt). The
// merge produces a *flat* tree (single leaf if N==1, otherwise a
// right-leaning chain of row splits), zips captured cwds onto each
// leaf, and merges invisibles by leaf index up to min(prevLeaves,
// capturedLeaves). The dropped tail of prev is reported as lossy if
// it carried command / env / initial_input.
//
// Tab count changes are handled at the tab level: merge tabs that
// align by index up to min(len). Extra captured tabs are taken
// as-is; dropped prev tabs are reported as lossy if they carried
// unrecoverable data.
//
// Position-based matching is intentionally simple. A user who
// reorders panes within a tab will see their command move to the
// wrong leaf — but they'll see this in the diff (cwds and commands
// now mismatched) before approving the save. The alternative
// (refusing to merge the moment counts don't match) would revert to
// the lossy "every re-save erases everything" behaviour we're
// fixing.
func mergeForSave(prev, captured layout.Layout) (layout.Layout, []string) {
	// Captured is the source of truth for tab list, name, and per-leaf
	// cwd. The captured layout from capturedToLayout is always flat
	// (one leaf per terminal Ghostty reported, as a single leaf or a
	// right-leaning row chain).
	merged := layout.Layout{Name: captured.Name}
	merged.Tabs = make([]layout.Tab, len(captured.Tabs))

	var lost []string

	commonTabs := min(len(prev.Tabs), len(captured.Tabs))
	for i := 0; i < commonTabs; i++ {
		mergedTab, tabLost := mergeTab(prev.Tabs[i], captured.Tabs[i])
		merged.Tabs[i] = mergedTab
		lost = append(lost, tabLost...)
	}
	// Captured tabs beyond what prev had: nothing to merge, take as-is
	// (deep-copied so callers don't share state with `captured`).
	for i := commonTabs; i < len(captured.Tabs); i++ {
		merged.Tabs[i] = copyTab(captured.Tabs[i])
	}
	// Prev tabs beyond what captured has: dropped entirely. Report any
	// unrecoverable leaf contents.
	for i := commonTabs; i < len(prev.Tabs); i++ {
		for j, leaf := range collectLeaves(prev.Tabs[i].Root) {
			for _, r := range leafLossReasons(i, prev.Tabs[i].Name, j, leaf) {
				lost = append(lost, fmt.Sprintf("dropped: %s", r))
			}
		}
	}

	return merged, lost
}

// mergeTab folds prev's invisibles into the captured tab. capturedTab
// is the flat result from capturedToLayout (a leaf or right-chain row).
//
// Two cases by leaf count:
//
//   - Same count → keep prev's tree shape; walk both in left-to-right
//     leaf order; zip captured cwd onto each leaf, carry forward
//     invisibles from prev. Lossless.
//   - Different count → keep capturedTab's flat shape; merge invisibles
//     by leaf index up to min; report dropped tail leaves as lossy.
//
// Tab name: prefer captured (Ghostty usually returns it); fall back to
// prev when captured didn't.
func mergeTab(prev, captured layout.Tab) (layout.Tab, []string) {
	prevLeaves := collectLeaves(prev.Root)
	capLeaves := collectLeaves(captured.Root)

	out := layout.Tab{Name: captured.Name}
	if out.Name == "" && prev.Name != "" {
		out.Name = prev.Name
	}

	var lost []string

	if len(prevLeaves) == len(capLeaves) {
		// Lossless path: clone prev's tree, then overlay captured cwds
		// and prev invisibles by leaf order.
		out.Root = mergePreservingShape(prev.Root, capLeaves, &[]int{0})
		return out, lost
	}

	// Lossy path: keep captured's flat shape, merge what aligns.
	common := min(len(prevLeaves), len(capLeaves))
	mergedLeaves := make([]layout.Split, len(capLeaves))
	for i, capLeaf := range capLeaves {
		merged := copyLeaf(capLeaf)
		if i < common {
			adoptInvisibles(&merged, prevLeaves[i])
		}
		mergedLeaves[i] = merged
	}
	out.Root = buildFlatRoot(mergedLeaves)

	// Dropped tail of prev: report unrecoverable data.
	for i := common; i < len(prevLeaves); i++ {
		for _, r := range leafLossReasons(0, captured.Name, i, prevLeaves[i]) {
			lost = append(lost, fmt.Sprintf("dropped: %s", r))
		}
	}

	return out, lost
}

// mergePreservingShape walks prev's tree, returning a clone where every
// leaf is replaced by the next captured leaf's cwd combined with prev
// leaf's invisibles.
//
// `cursor` is a single-element slice used as a mutable counter — Go
// passes ints by value, so the recursion needs a shared writable cell
// to advance through capLeaves in depth-first order. This is the same
// pattern stdlib uses for tree-walking iterators.
func mergePreservingShape(prevNode layout.Split, capLeaves []layout.Split, cursor *[]int) layout.Split {
	if prevNode.IsLeaf() {
		idx := (*cursor)[0]
		(*cursor)[0] = idx + 1
		// Start from the captured leaf (so cwd is fresh), then layer
		// prev's invisibles on top.
		merged := copyLeaf(capLeaves[idx])
		adoptInvisibles(&merged, prevNode)
		return merged
	}
	out := layout.Split{
		Direction: prevNode.Direction,
		Children:  make([]layout.Split, len(prevNode.Children)),
	}
	for i, c := range prevNode.Children {
		out.Children[i] = mergePreservingShape(c, capLeaves, cursor)
	}
	return out
}

// adoptInvisibles copies command / env / initial_input from prev into
// dst, but only when dst doesn't already have a value of its own. The
// captured layout never sets these (they're invisible to the API), so
// in practice this always copies — but the "only if empty" rule keeps
// the function safe to call from contexts where dst might have been
// pre-populated (e.g. a future feature where capture learns to read
// some of these from a sidecar file).
//
// Cwd is intentionally NOT copied: capture is the authority on cwd.
func adoptInvisibles(dst *layout.Split, prev layout.Split) {
	if dst.Command == "" && prev.Command != "" {
		dst.Command = prev.Command
	}
	if dst.InitialInput == "" && prev.InitialInput != "" {
		dst.InitialInput = prev.InitialInput
	}
	if len(dst.Env) == 0 && len(prev.Env) > 0 {
		// Copy the map so a later mutation of merged doesn't leak
		// back into the previous layout value.
		dst.Env = make(map[string]string, len(prev.Env))
		for k, v := range prev.Env {
			dst.Env[k] = v
		}
	}
}

// buildFlatRoot constructs the canonical flat tree representation of N
// leaves in left-to-right order:
//
//   - N == 0 → an empty leaf (defensive; capturedToLayout shouldn't
//     produce a tab with zero leaves, but if it does we emit a leaf so
//     the layout still validates).
//   - N == 1 → the leaf itself as the root.
//   - N >= 2 → a right-chain of row splits:
//       row(leaves[0], row(leaves[1], row(leaves[2], ...)))
//
// Why right-chain rather than a balanced tree: the schema requires
// exactly 2 children per interior node, so 3+ leaves must nest. A
// right-chain matches the visual shape Ghostty produces when the user
// hits Cmd-D N-1 times in a row (each split halves the previously-new
// pane), which is the most likely scenario when capture sees N>=3
// terminals it didn't author. A balanced tree would silently change
// pane proportions on re-open.
func buildFlatRoot(leaves []layout.Split) layout.Split {
	if len(leaves) == 0 {
		return layout.Split{}
	}
	if len(leaves) == 1 {
		return leaves[0]
	}
	// Build right-leaning: row(L0, row(L1, row(L2, L3))) for N=4.
	root := layout.Split{
		Direction: layout.DirRow,
		Children:  []layout.Split{leaves[0], buildFlatRoot(leaves[1:])},
	}
	return root
}

// copyTab deep-copies a Tab, including any maps inside leaves.
func copyTab(t layout.Tab) layout.Tab {
	return layout.Tab{Name: t.Name, Root: copySplit(t.Root)}
}

// copySplit deep-copies a Split tree.
func copySplit(s layout.Split) layout.Split {
	if s.IsLeaf() {
		return copyLeaf(s)
	}
	out := layout.Split{
		Direction: s.Direction,
		Children:  make([]layout.Split, len(s.Children)),
	}
	for i, c := range s.Children {
		out.Children[i] = copySplit(c)
	}
	return out
}

// copyLeaf clones a single leaf split, copying the Env map so callers
// can mutate the result without aliasing the source.
func copyLeaf(s layout.Split) layout.Split {
	out := s
	out.Children = nil
	out.Direction = ""
	if len(s.Env) > 0 {
		out.Env = make(map[string]string, len(s.Env))
		for k, v := range s.Env {
			out.Env[k] = v
		}
	}
	return out
}
