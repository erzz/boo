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
//   - OutcomeStructural: the shape changed (tabs/splits added or removed)
//     but no information that boo can't recover was lost. Show a focused
//     before/after diff and ask for confirmation (skipped under --force).
//
//   - OutcomeLossy: the previous layout contained data that Ghostty's
//     AppleScript dictionary doesn't expose (custom split direction, a
//     command, env vars, initial input). That data WILL be wiped by this
//     save. Show the diff with explicit lossy markers and require
//     confirmation; --force skips the prompt but still prints the diff to
//     stderr so audit logs show what was lost.
type SaveOutcome int

const (
	OutcomeSilent SaveOutcome = iota
	OutcomeStructural
	OutcomeLossy
)

// SaveDiff is the result of comparing previous and next layouts.
//
// ChangedTabs lists only tabs that differ structurally OR contain lossy
// cells; identical tabs are not included. LossReasons is a short
// human-readable summary of WHY the save is lossy ("split 1 of tab 'run'
// had direction = down"); empty for silent and structural outcomes.
type SaveDiff struct {
	Outcome     SaveOutcome
	ChangedTabs []TabDiff
	LossReasons []string
}

// TabDiff is one tab's worth of change information.
//
// Index/Name identify the tab in the captured layout (Next).
// Prev is nil when the tab is being added; Next is nil when it's removed.
// LossyCells holds split indices in Prev where unrecoverable information
// lived; empty for tabs that are merely structurally different.
type TabDiff struct {
	Index      int
	Name       string
	Prev       *layout.Tab
	Next       *layout.Tab
	LossyCells []int
}

// diffForSave computes a SaveDiff between the previously-saved layout and
// the freshly-captured one.
//
// A zero-value `prev` (no Tabs) is treated as "no previous layout" and the
// outcome is always OutcomeSilent — the first save of a project has nothing
// to lose. Callers should pass layout.Layout{} when the on-disk file is
// missing or fails to parse rather than propagating that error.
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
		var (
			pTab, nTab *layout.Tab
		)
		if i < len(prev.Tabs) {
			t := prev.Tabs[i]
			pTab = &t
		}
		if i < len(next.Tabs) {
			t := next.Tabs[i]
			nTab = &t
		}

		// Tab added or removed → structural change.
		structural := false
		switch {
		case pTab == nil:
			structural = true
		case nTab == nil:
			structural = true
		case len(pTab.Splits) != len(nTab.Splits):
			structural = true
		}

		// Lossy cells live in Prev: anything Ghostty can't reproduce on
		// recapture AND that the corresponding Next split didn't carry
		// forward.
		//
		// Two distinct cases:
		//   1. prev has split[j], next has split[j], next dropped a
		//      field → contribute to BOTH LossyCells (cell marker in
		//      the rendered table) AND LossReasons (text list).
		//   2. prev has split[j], next has no split[j] (closed split):
		//      contribute to LossyCells only. The text reason for this
		//      case comes from mergeForSave, which has the same info
		//      and reports it under the "dropped:" prefix. Emitting it
		//      here too would duplicate every closed-split reason.
		var lossy []int
		if pTab != nil {
			for j, prevSplit := range pTab.Splits {
				var nextSplit *layout.Split
				if nTab != nil && j < len(nTab.Splits) {
					nextSplit = &nTab.Splits[j]
				}
				if !splitLosesData(prevSplit, nextSplit, j) {
					continue
				}
				lossy = append(lossy, j)
				anyLossy = true
				if nextSplit != nil {
					// Aligned-but-dropped-field case: emit the text reason
					// here. Closed-split case is left to mergeForSave.
					lossReasons = append(lossReasons, lossReasonsFor(i, pTab.Name, j, prevSplit)...)
				}
			}
		}

		if structural {
			anyStruct = true
		}
		if structural || len(lossy) > 0 {
			changed = append(changed, TabDiff{
				Index:      i,
				Name:       tabDiffName(pTab, nTab),
				Prev:       pTab,
				Next:       nTab,
				LossyCells: lossy,
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

// splitLosesData reports whether a previously-saved Split holds
// unrecoverable data that the corresponding Next split fails to preserve.
// nextSplit may be nil when prev had a split at this position but next
// doesn't (closed split) — in that case every unrecoverable field on prev
// is, by definition, lost.
//
// "Unrecoverable" means: invisible to Ghostty's AppleScript dictionary
// (command, env, initial_input, non-default direction). Cwd is not in
// this set — cwd is the one thing capture always sees correctly, so a
// cwd change is intentional, not loss.
func splitLosesData(prev layout.Split, next *layout.Split, j int) bool {
	if next == nil {
		return len(lossReasonsFor(0, "", j, prev)) > 0
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
	if j > 0 && prev.Direction != "" && prev.Direction != layout.DirRight && next.Direction != prev.Direction {
		return true
	}
	return false
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

// lossReasonsFor returns one human-readable reason per unrecoverable
// property on a previously-saved Split. Returning a slice (rather than
// the first reason) is deliberate — a single split can carry several
// lossy fields at once (e.g. command + env + non-default direction), and
// the user deserves to see all of them before approving the save.
//
// j == 0 is the primary split: a non-empty Direction would be a layout
// validation error, so we only check Command / Env / InitialInput on it.
// j > 0 splits also lose any non-"right" Direction.
//
// An empty result means the split has no unrecoverable properties.
func lossReasonsFor(tabIdx int, tabName string, j int, s layout.Split) []string {
	tag := fmt.Sprintf("tab %d", tabIdx)
	if tabName != "" {
		tag = fmt.Sprintf("tab %d (%q)", tabIdx, tabName)
	}
	var out []string
	if s.Command != "" {
		out = append(out, fmt.Sprintf("%s split %d: command %q will be lost", tag, j, s.Command))
	}
	if s.InitialInput != "" {
		out = append(out, fmt.Sprintf("%s split %d: initial_input will be lost", tag, j))
	}
	if len(s.Env) > 0 {
		out = append(out, fmt.Sprintf("%s split %d: %d env var(s) will be lost", tag, j, len(s.Env)))
	}
	if j > 0 && s.Direction != "" && s.Direction != layout.DirRight {
		out = append(out, fmt.Sprintf("%s split %d: direction = %q will be saved as \"right\"", tag, j, s.Direction))
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
