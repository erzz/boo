package cli

import (
	"fmt"

	"github.com/erzz/boo/internal/layout"
)

// SaveOutcome categorises the result of comparing the previously-saved
// layout against the layout we just captured from Ghostty.
//
//   - OutcomeSilent: nothing meaningful changed; the user should see only
//     the standard "Saved layout for X." line. No prompt, no warnings.
//
//   - OutcomeStructural: the shape changed (tabs added/removed, leaf
//     count changed, or tree shape couldn't be preserved) but no
//     unrecoverable data was lost. Show a focused before/after diff and
//     ask for confirmation (skipped under --force).
//
//   - OutcomeLossy: the previous layout contained data that Ghostty's
//     AppleScript dictionary doesn't expose (command, env vars,
//     initial input, or leaves dropped because the captured pane count
//     shrank). That data WILL be wiped by this save. Show the diff with
//     explicit lossy markers and require confirmation; --force skips
//     the prompt but still prints the diff to stderr so audit logs
//     show what was lost.
type SaveOutcome int

const (
	OutcomeSilent SaveOutcome = iota
	OutcomeStructural
	OutcomeLossy
)

// SaveDiff is the result of comparing previous and next layouts.
//
// ChangedTabs lists only tabs that differ structurally OR contain lossy
// leaves; identical tabs are not included. LossReasons is a short
// human-readable summary of WHY the save is lossy ("tab 0 leaf 1: command
// 'go test' will be lost"); empty for silent and structural outcomes.
type SaveDiff struct {
	Outcome     SaveOutcome
	ChangedTabs []TabDiff
	LossReasons []string
}

// TabDiff is one tab's worth of change information.
//
// Index/Name identify the tab in the captured layout (Next).
// Prev is nil when the tab is being added; Next is nil when it's removed.
//
// LossyLeaves holds 0-based leaf indices (in left-to-right depth-first
// order) within Prev where unrecoverable information lived that the
// merged Next did not preserve. Empty for tabs that are merely
// structurally different. The renderer uses these to mark the
// corresponding cells in the before-side of the diff.
type TabDiff struct {
	Index       int
	Name        string
	Prev        *layout.Tab
	Next        *layout.Tab
	LossyLeaves []int
}

// diffForSave computes a SaveDiff between the previously-saved layout and
// the about-to-be-written merged one.
//
// A zero-value `prev` (no Tabs) is treated as "no previous layout" and the
// outcome is always OutcomeSilent — the first save of a project has nothing
// to lose. Callers should pass layout.Layout{} when the on-disk file is
// missing or fails to parse rather than propagating that error.
//
// "Next" here is the post-merge layout we're about to write. The merge
// has already done its best to carry forward invisible fields; this diff
// reports what survived and what didn't.
func diffForSave(prev, next layout.Layout) SaveDiff {
	if len(prev.Tabs) == 0 {
		return SaveDiff{Outcome: OutcomeSilent}
	}

	var (
		changed     []TabDiff
		lossReasons []string
		anyLossy    bool
		anyStruct   bool
	)

	maxTabs := len(prev.Tabs)
	if n := len(next.Tabs); n > maxTabs {
		maxTabs = n
	}

	for i := 0; i < maxTabs; i++ {
		var pTab, nTab *layout.Tab
		if i < len(prev.Tabs) {
			t := prev.Tabs[i]
			pTab = &t
		}
		if i < len(next.Tabs) {
			t := next.Tabs[i]
			nTab = &t
		}

		// Structural changes: tab added, tab removed, leaf count
		// changed, or tree shape couldn't be preserved.
		structural := false
		var prevLeaves, nextLeaves []layout.Split
		switch {
		case pTab == nil:
			structural = true
		case nTab == nil:
			structural = true
		default:
			prevLeaves = collectLeaves(pTab.Root)
			nextLeaves = collectLeaves(nTab.Root)
			if len(prevLeaves) != len(nextLeaves) {
				structural = true
			} else if !sameTreeShape(pTab.Root, nTab.Root) {
				// Same leaf count but different nesting — happens
				// when the merge had to flatten a previously-nested
				// tree because the captured leaf count didn't fit.
				structural = true
			}
		}

		// Lossy leaves: walk prev's leaves left-to-right and check
		// whether the merged result preserved each one's invisible
		// fields. Two cases:
		//   1. Leaf has a counterpart in next at the same leaf index
		//      and next dropped a field → mark the leaf AND emit a
		//      text reason (the user sees both).
		//   2. Leaf has no counterpart (next has fewer leaves) → mark
		//      it and emit a "dropped:" text reason via mergeForSave;
		//      we only emit the cell marker here to avoid duplicating
		//      the textual reason. (mergeForSave is the authority on
		//      dropped-leaf reasons because it owns the alignment.)
		var lossy []int
		if pTab != nil {
			for j, prevLeaf := range prevLeaves {
				var nextLeaf *layout.Split
				if j < len(nextLeaves) {
					nextLeaf = &nextLeaves[j]
				}
				if !leafLosesData(prevLeaf, nextLeaf) {
					continue
				}
				lossy = append(lossy, j)
				anyLossy = true
				if nextLeaf != nil {
					lossReasons = append(lossReasons, leafLossReasons(i, pTab.Name, j, prevLeaf)...)
				}
			}
		}

		if structural {
			anyStruct = true
		}
		if structural || len(lossy) > 0 {
			changed = append(changed, TabDiff{
				Index:       i,
				Name:        tabDiffName(pTab, nTab),
				Prev:        pTab,
				Next:        nTab,
				LossyLeaves: lossy,
			})
		}
	}

	switch {
	case anyLossy:
		return SaveDiff{Outcome: OutcomeLossy, ChangedTabs: changed, LossReasons: lossReasons}
	case anyStruct:
		return SaveDiff{Outcome: OutcomeStructural, ChangedTabs: changed}
	default:
		return SaveDiff{Outcome: OutcomeSilent}
	}
}

// collectLeaves returns a tab's leaves in left-to-right depth-first
// order — the same order the JXA walker uses to materialise terminals,
// and the same order Ghostty returns them in DescribeWindow. This is the
// canonical "leaf index" used for diff alignment.
func collectLeaves(s layout.Split) []layout.Split {
	if s.IsLeaf() {
		return []layout.Split{s}
	}
	var out []layout.Split
	for _, c := range s.Children {
		out = append(out, collectLeaves(c)...)
	}
	return out
}

// sameTreeShape reports whether two split trees have identical structure
// (same direction at each interior node, same recursive shape). Leaf
// content is ignored — this only compares nesting.
func sameTreeShape(a, b layout.Split) bool {
	if a.IsLeaf() != b.IsLeaf() {
		return false
	}
	if a.IsLeaf() {
		return true
	}
	if a.Direction != b.Direction || len(a.Children) != len(b.Children) {
		return false
	}
	for i := range a.Children {
		if !sameTreeShape(a.Children[i], b.Children[i]) {
			return false
		}
	}
	return true
}

// leafLosesData reports whether a previously-saved leaf holds
// unrecoverable data that the corresponding merged leaf fails to
// preserve. nextLeaf may be nil when prev had a leaf at this index but
// next doesn't (closed pane) — in that case every unrecoverable field
// on prev is, by definition, lost.
//
// "Unrecoverable" means: invisible to Ghostty's AppleScript dictionary
// (command, env, initial_input). Cwd is not in this set — capture sees
// cwd correctly, so a cwd change is intentional, not loss. Direction
// is no longer in this set either: it's a property of interior nodes,
// and tree-shape loss is reported at the tab level via sameTreeShape.
func leafLosesData(prev layout.Split, next *layout.Split) bool {
	if next == nil {
		return leafHasInvisibleData(prev)
	}
	if prev.Command != "" && next.Command != prev.Command {
		return true
	}
	if prev.InitialInput != "" && next.InitialInput != prev.InitialInput {
		return true
	}
	if len(prev.Env) > 0 && !envEqual(prev.Env, next.Env) {
		return true
	}
	return false
}

// leafHasInvisibleData reports whether a leaf carries any field that
// capture cannot recover — used to decide whether dropping a leaf
// constitutes loss worth surfacing.
func leafHasInvisibleData(s layout.Split) bool {
	return s.Command != "" || s.InitialInput != "" || len(s.Env) > 0
}

// envEqual reports whether two env maps have identical key/value sets.
// Order-insensitive by definition (maps don't have order).
func envEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if bv, ok := b[k]; !ok || bv != v {
			return false
		}
	}
	return true
}

// leafLossReasons returns one human-readable reason per unrecoverable
// property on a previously-saved leaf. Returning a slice (rather than
// the first reason) is deliberate — a single leaf can carry several
// lossy fields at once, and the user deserves to see all of them
// before approving the save.
//
// j is the leaf's depth-first index within its tab (matching the
// LossyLeaves entries on TabDiff and the column index the renderer
// uses to mark cells).
//
// An empty result means the leaf has no unrecoverable properties.
func leafLossReasons(tabIdx int, tabName string, j int, s layout.Split) []string {
	tag := fmt.Sprintf("tab %d", tabIdx)
	if tabName != "" {
		tag = fmt.Sprintf("tab %d (%q)", tabIdx, tabName)
	}
	var out []string
	if s.Command != "" {
		out = append(out, fmt.Sprintf("%s leaf %d: command %q will be lost", tag, j, s.Command))
	}
	if s.InitialInput != "" {
		out = append(out, fmt.Sprintf("%s leaf %d: initial_input will be lost", tag, j))
	}
	if len(s.Env) > 0 {
		out = append(out, fmt.Sprintf("%s leaf %d: %d env var(s) will be lost", tag, j, len(s.Env)))
	}
	return out
}

func tabDiffName(prev, next *layout.Tab) string {
	if next != nil && next.Name != "" {
		return next.Name
	}
	if prev != nil && prev.Name != "" {
		return prev.Name
	}
	return ""
}
