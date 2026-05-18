package cli

import (
	"fmt"

	"github.com/erzz/boo/internal/layout"
)

// mergeForSave folds invisible-but-stable fields from prev into the captured
// layout, returning the layout to save and a list of notes about data that
// could NOT be carried forward.
//
// Why: Ghostty's AppleScript API exposes only id/title/cwd per terminal — not
// command, env, initial_input, or the split tree shape. A naive capture wipes
// those fields on every re-save. Since they exist in the previous layout file,
// we carry them forward by leaf position when the captured leaf count matches.
//
// Match policy (per tab):
//   - Same leaf count as prev → keep prev's tree shape, zip captured cwds onto leaves. Lossless.
//   - Different count → use captured's flat tree, merge invisibles by index up to min(counts).
//     Dropped prev leaves with unrecoverable data are reported as lost.
func mergeForSave(prev, captured layout.Layout) (layout.Layout, []string) {
	// Captured is the source of truth for tab list, name, and per-leaf cwd.
	merged := layout.Layout{Name: captured.Name}
	merged.Tabs = make([]layout.Tab, len(captured.Tabs))

	var lost []string

	commonTabs := min(len(prev.Tabs), len(captured.Tabs))
	for i := 0; i < commonTabs; i++ {
		mergedTab, tabLost := mergeTab(prev.Tabs[i], captured.Tabs[i])
		merged.Tabs[i] = mergedTab
		lost = append(lost, tabLost...)
	}
	// Extra captured tabs (beyond prev): take as-is.
	for i := commonTabs; i < len(captured.Tabs); i++ {
		merged.Tabs[i] = copyTab(captured.Tabs[i])
	}
	// Dropped prev tabs: report unrecoverable leaf contents.
	for i := commonTabs; i < len(prev.Tabs); i++ {
		for j, leaf := range collectLeaves(prev.Tabs[i].Root) {
			for _, r := range leafLossReasons(i, prev.Tabs[i].Name, j, leaf) {
				lost = append(lost, fmt.Sprintf("dropped: %s", r))
			}
		}
	}

	return merged, lost
}

// mergeTab folds prev's invisibles into a captured tab.
// Same leaf count → keep prev's tree shape, zip captured cwds, carry forward invisibles (lossless).
// Different count → keep captured's flat shape, merge by index, report dropped tail.
func mergeTab(prev, captured layout.Tab) (layout.Tab, []string) {
	prevLeaves := collectLeaves(prev.Root)
	capLeaves := collectLeaves(captured.Root)

	out := layout.Tab{Name: captured.Name}
	if out.Name == "" && prev.Name != "" {
		out.Name = prev.Name
	}

	var lost []string

	if len(prevLeaves) == len(capLeaves) {
		// Lossless: clone prev's tree, overlay captured cwds by leaf order.
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

// mergePreservingShape clones prev's tree replacing each leaf's cwd with the
// next captured leaf's cwd, then layers prev's invisibles on top.
// cursor is a shared counter (single-element slice) for DFS traversal.
func mergePreservingShape(prevNode layout.Split, capLeaves []layout.Split, cursor *[]int) layout.Split {
	if prevNode.IsLeaf() {
		idx := (*cursor)[0]
		(*cursor)[0] = idx + 1
		// Start from captured leaf (fresh cwd), then layer prev's invisibles.
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

// adoptInvisibles copies command/env/initial_input from prev into dst when
// dst doesn't already have a value. Cwd is intentionally NOT copied — capture
// is the authority on cwd.
func adoptInvisibles(dst *layout.Split, prev layout.Split) {
	if dst.Command == "" && prev.Command != "" {
		dst.Command = prev.Command
	}
	if dst.InitialInput == "" && prev.InitialInput != "" {
		dst.InitialInput = prev.InitialInput
	}
	if len(dst.Env) == 0 && len(prev.Env) > 0 {
		// Copy the map so a later mutation doesn't alias the previous layout value.
		dst.Env = make(map[string]string, len(prev.Env))
		for k, v := range prev.Env {
			dst.Env[k] = v
		}
	}
}

// buildFlatRoot builds the canonical flat tree for N leaves:
// N==0 → empty leaf; N==1 → the leaf itself; N≥2 → right-leaning row chain.
// Right-chain matches the shape Ghostty produces on Cmd-D×(N-1), matching
// user expectation and avoiding silent pane-proportion changes on re-open.
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

// copyLeaf clones a single leaf split, copying the Env map to avoid aliasing.
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
