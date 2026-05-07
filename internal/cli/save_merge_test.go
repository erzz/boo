package cli

import (
	"strings"
	"testing"

	"github.com/erzz/boo/internal/layout"
)

// These tests pin down the merge contract for `boo save`. The whole point
// of the merge is to make the common case (re-save after rearranging
// splits with the cwd unchanged) NON-lossy, while still surfacing genuine
// data loss (closed splits that held command/env, dropped tabs, etc.).
//
// Each test names the user scenario it represents, because the merge is
// behavioural code first and structural code second — if a test name
// stops matching the user story, the merge is wrong, not the test.

// helpers ---------------------------------------------------------------

func tab(name string, splits ...layout.Split) layout.Tab {
	return layout.Tab{Name: name, Splits: splits}
}

func split(cwd string) layout.Split {
	return layout.Split{Cwd: cwd}
}

func splitR(cwd string) layout.Split {
	return layout.Split{Direction: layout.DirRight, Cwd: cwd}
}

// scenarios -------------------------------------------------------------

func TestMergeForSave_IdenticalShapeIsLosslessAndProducesSilentDiff(t *testing.T) {
	// User: re-saves immediately after launching with no changes. Today
	// (pre-merge) this would already be Silent because nothing changed.
	// After merge it must STILL be silent — the merge must not invent
	// changes by, say, re-ordering env keys.
	prev := layout.Layout{Name: "default", Tabs: []layout.Tab{
		tab("edit", layout.Split{Cwd: ".", Command: "nvim", Env: map[string]string{"EDITOR": "nvim"}}),
	}}
	captured := layout.Layout{Name: "default", Tabs: []layout.Tab{
		tab("edit", split(".")),
	}}
	merged, lost := mergeForSave(prev, captured)
	if len(lost) != 0 {
		t.Fatalf("unexpected loss: %v", lost)
	}
	if merged.Tabs[0].Splits[0].Command != "nvim" {
		t.Errorf("command not carried forward: %+v", merged.Tabs[0].Splits[0])
	}
	if merged.Tabs[0].Splits[0].Env["EDITOR"] != "nvim" {
		t.Errorf("env not carried forward: %+v", merged.Tabs[0].Splits[0].Env)
	}
	// Diff against prev should be Silent — merged === prev.
	if d := diffForSave(prev, merged); d.Outcome != OutcomeSilent {
		t.Errorf("outcome = %v, want OutcomeSilent (no real change)", d.Outcome)
	}
}

func TestMergeForSave_CwdChangeIsNotLossyAndCarriesCommandForward(t *testing.T) {
	// User: cd-d in one of the splits, ran `boo save`. The cwd should
	// move to the new value and the command should still be there.
	prev := layout.Layout{Name: "default", Tabs: []layout.Tab{
		tab("run",
			split("."),
			layout.Split{Direction: layout.DirRight, Cwd: "logs", Command: "tail -f app.log"},
		),
	}}
	captured := layout.Layout{Name: "default", Tabs: []layout.Tab{
		tab("run",
			split("."),
			splitR("logs/2024"), // user cd'd deeper
		),
	}}
	merged, lost := mergeForSave(prev, captured)
	if len(lost) != 0 {
		t.Fatalf("unexpected loss: %v", lost)
	}
	if merged.Tabs[0].Splits[1].Cwd != "logs/2024" {
		t.Errorf("cwd should track captured, got %q", merged.Tabs[0].Splits[1].Cwd)
	}
	if merged.Tabs[0].Splits[1].Command != "tail -f app.log" {
		t.Errorf("command not carried, got %q", merged.Tabs[0].Splits[1].Command)
	}
}

func TestMergeForSave_NonDefaultDirectionPreservedOnAlignedSplit(t *testing.T) {
	// User had `direction = "down"` on split 1 in the layout file.
	// Captured (which always writes "right") must not silently flip it.
	prev := layout.Layout{Name: "default", Tabs: []layout.Tab{
		tab("split-down",
			split("."),
			layout.Split{Direction: layout.DirDown, Cwd: "."},
		),
	}}
	captured := layout.Layout{Name: "default", Tabs: []layout.Tab{
		tab("split-down",
			split("."),
			splitR("."), // capturedToLayout always writes "right"
		),
	}}
	merged, lost := mergeForSave(prev, captured)
	if len(lost) != 0 {
		t.Fatalf("unexpected loss: %v", lost)
	}
	if got := merged.Tabs[0].Splits[1].Direction; got != layout.DirDown {
		t.Errorf("direction silently flipped: got %q, want %q", got, layout.DirDown)
	}
}

func TestMergeForSave_AddedSplitTakesCapturedDefaultsAndDoesNotInventCommand(t *testing.T) {
	// User: added a third split via Cmd-D. There's no prev data for
	// position 2, so it should land exactly as captured (blank command,
	// "right" direction). Critically, we must NOT pull command from
	// position 1 — that would attribute someone else's command to a new
	// surface the user just opened bare.
	prev := layout.Layout{Name: "default", Tabs: []layout.Tab{
		tab("run",
			split("."),
			layout.Split{Direction: layout.DirRight, Cwd: ".", Command: "npm run dev"},
		),
	}}
	captured := layout.Layout{Name: "default", Tabs: []layout.Tab{
		tab("run", split("."), splitR("."), splitR(".")),
	}}
	merged, lost := mergeForSave(prev, captured)
	if len(lost) != 0 {
		t.Fatalf("unexpected loss: %v", lost)
	}
	if got := merged.Tabs[0].Splits[1].Command; got != "npm run dev" {
		t.Errorf("aligned split lost its command: %q", got)
	}
	if got := merged.Tabs[0].Splits[2].Command; got != "" {
		t.Errorf("new split inherited a command it shouldn't have: %q", got)
	}
}

func TestMergeForSave_RemovedSplitWithCommandIsReportedAsLossy(t *testing.T) {
	// User: closed the second split. It held `command = "..."`. We can't
	// keep that command — there's nowhere to put it — but we MUST report
	// the loss so the user is prompted before we destroy it.
	prev := layout.Layout{Name: "default", Tabs: []layout.Tab{
		tab("run",
			split("."),
			layout.Split{Direction: layout.DirRight, Cwd: "logs", Command: "tail -f app.log", Env: map[string]string{"LOG": "debug"}},
		),
	}}
	captured := layout.Layout{Name: "default", Tabs: []layout.Tab{
		tab("run", split(".")),
	}}
	merged, lost := mergeForSave(prev, captured)
	if len(merged.Tabs[0].Splits) != 1 {
		t.Fatalf("merged should have 1 split (captured wins on shape), got %d", len(merged.Tabs[0].Splits))
	}
	joined := strings.Join(lost, "\n")
	if !strings.Contains(joined, "command") || !strings.Contains(joined, "env") {
		t.Errorf("expected loss notes for command and env, got: %v", lost)
	}
}

func TestMergeForSave_DroppedTabReportsLoss(t *testing.T) {
	// User: closed an entire tab that held a command.
	prev := layout.Layout{Name: "default", Tabs: []layout.Tab{
		tab("keep", split(".")),
		tab("drop", layout.Split{Cwd: ".", Command: "make watch"}),
	}}
	captured := layout.Layout{Name: "default", Tabs: []layout.Tab{
		tab("keep", split(".")),
	}}
	merged, lost := mergeForSave(prev, captured)
	if len(merged.Tabs) != 1 {
		t.Fatalf("merged should have 1 tab, got %d", len(merged.Tabs))
	}
	if !strings.Contains(strings.Join(lost, "\n"), "make watch") {
		t.Errorf("expected loss notes mentioning the dropped command, got: %v", lost)
	}
}

func TestMergeForSave_AddedTabIsTakenAsCaptured(t *testing.T) {
	// User: opened a brand-new tab. It has no prev data — should land
	// exactly as captured.
	prev := layout.Layout{Name: "default", Tabs: []layout.Tab{
		tab("edit", split(".")),
	}}
	captured := layout.Layout{Name: "default", Tabs: []layout.Tab{
		tab("edit", split(".")),
		tab("logs", split("logs")),
	}}
	merged, lost := mergeForSave(prev, captured)
	if len(lost) != 0 {
		t.Fatalf("unexpected loss for purely additive change: %v", lost)
	}
	if len(merged.Tabs) != 2 || merged.Tabs[1].Name != "logs" {
		t.Fatalf("new tab missing or wrong: %+v", merged.Tabs)
	}
}

func TestMergeForSave_FirstSaveHasNoPrevAndIsLossless(t *testing.T) {
	// User: first save ever. prev is the zero Layout. Nothing to merge,
	// nothing to lose.
	captured := layout.Layout{Name: "default", Tabs: []layout.Tab{
		tab("edit", split(".")),
	}}
	merged, lost := mergeForSave(layout.Layout{}, captured)
	if len(lost) != 0 {
		t.Fatalf("first save reported loss: %v", lost)
	}
	if len(merged.Tabs) != 1 {
		t.Fatalf("expected captured to pass through, got %+v", merged.Tabs)
	}
}

func TestMergeForSave_DeepCopyDoesNotAliasEnvMap(t *testing.T) {
	// Mutating merged's env after the merge must not leak back into
	// captured. Cheap insurance — costs one map alloc per split.
	captured := layout.Layout{Name: "default", Tabs: []layout.Tab{
		tab("edit", layout.Split{Cwd: ".", Env: map[string]string{"A": "1"}}),
	}}
	merged, _ := mergeForSave(layout.Layout{}, captured)
	merged.Tabs[0].Splits[0].Env["A"] = "MUTATED"
	if captured.Tabs[0].Splits[0].Env["A"] != "1" {
		t.Errorf("merged aliased captured's env map")
	}
}

func TestMergeForSave_PreservesPrevTabNameWhenCapturedIsBlank(t *testing.T) {
	// Defensive: if Ghostty returns a tab without a name (it usually
	// does, but not always) and prev had one, keep prev's name.
	prev := layout.Layout{Name: "default", Tabs: []layout.Tab{
		tab("named-by-user", split(".")),
	}}
	captured := layout.Layout{Name: "default", Tabs: []layout.Tab{
		{Splits: []layout.Split{split(".")}}, // no name
	}}
	merged, _ := mergeForSave(prev, captured)
	if merged.Tabs[0].Name != "named-by-user" {
		t.Errorf("tab name not preserved, got %q", merged.Tabs[0].Name)
	}
}
