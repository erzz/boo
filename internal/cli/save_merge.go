package cli

import (
	"fmt"

	"github.com/erzz/boo/internal/layout"
)

// mergeForSave folds invisible-but-stable fields from prev into captured,
// returning the layout to actually save and a list of human-readable
// notes about prev data that could NOT be carried forward.
//
// Why a merge exists at all
// -------------------------
// Ghostty's AppleScript API doesn't tell us the launch command, env,
// initial_input, or split direction of a live terminal — it only exposes
// id / title / working directory. A naive "snapshot the live window" save
// therefore wipes those fields on every re-save, which makes `boo save`
// hostile to anyone who has commands or env in their layout.
//
// But we already have those fields on disk, in the previous layout file.
// As long as the captured shape lines up with the previous shape, we can
// safely carry that data forward by position. Cwd we always take from the
// capture, because cwd is the one thing that legitimately changes when
// the user `cd`-s around.
//
// Match policy (per tab)
// ----------------------
// Same-shape (same split count) → merge by index. Direction, command,
// env, initial_input from prev; cwd from captured.
//
// Captured added splits → merge what aligns up to min(len), leave new
// trailing positions exactly as captured (blank command/env, default
// "right" direction set by capturedToLayout).
//
// Captured removed splits → merge what aligns; the dropped tail of prev
// is reported as lossy if it carried command/env/initial_input/direction.
// Cwd alone being lost on a closed split is not interesting — the user
// closed it on purpose.
//
// Anything more complex (tab reorder, simultaneous add+remove on the same
// tab, tab count change of the wrong sign) is handled at the tab level by
// only merging tabs that align by index up to min(len). Extra captured
// tabs are taken as-is. Dropped prev tabs are reported as lossy if they
// carried unrecoverable data.
//
// Position-based matching is intentionally simple. A user who reorders
// splits within a tab will see their command move to the wrong split —
// but they'll see this in the diff (cwds and commands now mismatched)
// before approving the save. The alternative (refusing to merge the
// moment shapes don't match exactly) reverts us to today's "every
// re-save is lossy" behaviour, which is what we're trying to fix.
func mergeForSave(prev, captured layout.Layout) (layout.Layout, []string) {
	// Captured is the source of truth for shape, name, and cwd. Start
	// from a deep copy so we don't mutate the caller's value.
	merged := layout.Layout{Name: captured.Name}
	merged.Tabs = make([]layout.Tab, len(captured.Tabs))
	for i := range captured.Tabs {
		merged.Tabs[i] = copyTab(captured.Tabs[i])
	}

	var lost []string

	// Tab-level alignment: merge by index up to min(len). Extra captured
	// tabs keep their captured-only state (already in merged from the copy
	// above). Extra prev tabs are reported as lossy when they carried
	// unrecoverable data.
	commonTabs := min(len(prev.Tabs), len(captured.Tabs))
	for i := 0; i < commonTabs; i++ {
		mergeTab(&merged.Tabs[i], prev.Tabs[i], &lost)
	}
	for i := commonTabs; i < len(prev.Tabs); i++ {
		// Prev tab dropped entirely — report any unrecoverable contents.
		for j, s := range prev.Tabs[i].Splits {
			for _, r := range lossReasonsFor(i, prev.Tabs[i].Name, j, s) {
				lost = append(lost, fmt.Sprintf("dropped: %s", r))
			}
		}
	}

	return merged, lost
}

// mergeTab folds prev's invisible fields into capturedTab in place, and
// appends loss notes for any split positions that prev had but capturedTab
// doesn't.
func mergeTab(capturedTab *layout.Tab, prev layout.Tab, lost *[]string) {
	// Preserve tab name from prev when captured didn't get one. Ghostty
	// generally returns tab names, but if it doesn't (or the user set it
	// only in the layout file), keep what we had.
	if capturedTab.Name == "" && prev.Name != "" {
		capturedTab.Name = prev.Name
	}

	commonSplits := min(len(prev.Splits), len(capturedTab.Splits))
	for j := 0; j < commonSplits; j++ {
		mergeSplit(&capturedTab.Splits[j], prev.Splits[j], j)
	}
	for j := commonSplits; j < len(prev.Splits); j++ {
		// Prev had a split at position j that's gone now. Report only the
		// unrecoverable fields — losing a closed split's cwd is normal.
		for _, r := range lossReasonsFor(0, capturedTab.Name, j, prev.Splits[j]) {
			*lost = append(*lost, fmt.Sprintf("dropped: %s", r))
		}
	}
}

// mergeSplit folds prev's invisible-but-stable fields into capturedSplit
// in place. Cwd is always taken from captured (it can legitimately change
// between saves).
//
// j is the index of this split within its tab. The primary split (j == 0)
// must not have a direction — the layout validator forbids it — so we
// skip the direction merge when j == 0 even if prev somehow had one.
func mergeSplit(capturedSplit *layout.Split, prev layout.Split, j int) {
	// Cwd: always trust captured. This is the whole point of `boo save`.

	// Command / Env / InitialInput: invisible to the API. Carry forward.
	if capturedSplit.Command == "" && prev.Command != "" {
		capturedSplit.Command = prev.Command
	}
	if capturedSplit.InitialInput == "" && prev.InitialInput != "" {
		capturedSplit.InitialInput = prev.InitialInput
	}
	if len(capturedSplit.Env) == 0 && len(prev.Env) > 0 {
		// Copy the map so a later mutation of merged doesn't leak back
		// into the previous layout value (defensive — prev shouldn't be
		// reused after merge, but cheap insurance).
		capturedSplit.Env = make(map[string]string, len(prev.Env))
		for k, v := range prev.Env {
			capturedSplit.Env[k] = v
		}
	}

	// Direction: invisible to the API. Carry forward only on non-primary
	// splits. capturedToLayout always writes "right" for j > 0; if prev
	// had something else (down/left/up), prefer prev's value so we don't
	// silently flip the user's chosen direction. If prev was empty (e.g.
	// from a hand-authored layout that omitted it), keep captured's
	// "right" as a sane default.
	if j > 0 && prev.Direction != "" {
		capturedSplit.Direction = prev.Direction
	}
}

// copyTab deep-copies a Tab. Splits hold a map (Env) which must be
// copied separately to avoid aliasing.
func copyTab(t layout.Tab) layout.Tab {
	out := layout.Tab{Name: t.Name}
	out.Splits = make([]layout.Split, len(t.Splits))
	for i, s := range t.Splits {
		out.Splits[i] = s
		if len(s.Env) > 0 {
			out.Splits[i].Env = make(map[string]string, len(s.Env))
			for k, v := range s.Env {
				out.Splits[i].Env[k] = v
			}
		}
	}
	return out
}
