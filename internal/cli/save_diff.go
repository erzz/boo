package cli

import (
	"fmt"

	"github.com/erzz/boo/internal/layout"
)

// SaveOutcome categorises the result of comparing prev vs captured layouts.
//   - OutcomeSilent: nothing changed; no prompt, no output.
//   - OutcomeStructural: shape changed but no unrecoverable data lost; show diff, ask to confirm.
//   - OutcomeLossy: data invisible to Ghostty's API (command/env/initial_input) WILL be wiped;
//     show diff with markers, require confirmation; --force skips prompt but still logs to stderr.
type SaveOutcome int

const (
	OutcomeSilent SaveOutcome = iota
	OutcomeStructural
	OutcomeLossy
)

// SaveDiff is the result of comparing previous and next layouts.
// ChangedTabs lists only tabs that differ or contain lossy leaves; LossReasons
// summarises why the save is lossy (empty for silent/structural outcomes).
type SaveDiff struct {
	Outcome     SaveOutcome
	ChangedTabs []TabDiff
	LossReasons []string
}

// TabDiff is one tab's change. Prev is nil for added tabs; Next is nil for removed.
// LossyLeaves holds 0-based leaf indices (DFS left-to-right) where prev had
// unrecoverable data that the merged Next did not preserve.
type TabDiff struct {
	Index       int
	Name        string
	Prev        *layout.Tab
	Next        *layout.Tab
	LossyLeaves []int
}

// diffForSave computes a SaveDiff between the previously-saved layout and the
// about-to-be-written merged layout. A zero-value prev (no Tabs) → OutcomeSilent
// (first save; nothing to lose). "Next" is the post-merge layout to be written.
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

		// Structural changes: tab added/removed, leaf count changed, or tree shape changed.
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
				// Same leaf count but different nesting.
				structural = true
			}
		}

		// Lossy leaves: walk prev's leaves and check whether next preserved each invisible field.
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

// collectLeaves returns a split tree's leaves in left-to-right DFS order —
// the same order JXA materialises terminals and Ghostty returns them in DescribeWindow.
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
// (same direction at each interior node). Leaf content is ignored.
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

// leafLosesData reports whether a prev leaf holds unrecoverable data that the
// merged next leaf fails to preserve. nextLeaf may be nil (closed pane) — then
// every unrecoverable field on prev is lost. "Unrecoverable" = invisible to
// Ghostty's AppleScript API: command, env, initial_input. Cwd is NOT in this set.
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

// leafHasInvisibleData reports whether a leaf carries any field that capture cannot recover.
func leafHasInvisibleData(s layout.Split) bool {
	return s.Command != "" || s.InitialInput != "" || len(s.Env) > 0
}

// envEqual reports whether two env maps have identical key/value sets.
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

// leafLossReasons returns one human-readable reason per unrecoverable property on a prev leaf.
// j is the leaf's DFS index within its tab (matches LossyLeaves in TabDiff).
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
