package cli

import (
	"strings"
	"testing"

	"github.com/erzz/boo/internal/layout"
)

// save_merge_test.go pins the merge contract for `boo save`:
// same leaf count → lossless tree-shape + cwd zip; different count → right-leaning row chain with lossy report.
// leaf/row/col/tab/lay helpers live in save_diff_test.go.

// scenarios -------------------------------------------------------------

func TestMergeForSave_IdenticalShapeIsLosslessAndProducesSilentDiff(t *testing.T) {
	// User: re-saves immediately after launching with no changes. The
	// merge must NOT invent changes (e.g. by re-ordering env keys or
	// flattening the tree).
	prev := lay("1x1x1", tab("edit", layout.Split{
		Cwd: ".", Command: "nvim", Env: map[string]string{"EDITOR": "nvim"},
	}))
	captured := lay("1x1x1", tab("edit", leaf(".")))
	merged, lost := mergeForSave(prev, captured)
	if len(lost) != 0 {
		t.Fatalf("unexpected loss: %v", lost)
	}
	if merged.Tabs[0].Root.Command != "nvim" {
		t.Errorf("command not carried forward: %+v", merged.Tabs[0].Root)
	}
	if merged.Tabs[0].Root.Env["EDITOR"] != "nvim" {
		t.Errorf("env not carried forward: %+v", merged.Tabs[0].Root.Env)
	}
	if d := diffForSave(prev, merged); d.Outcome != OutcomeSilent {
		t.Errorf("outcome = %v, want OutcomeSilent (no real change)", d.Outcome)
	}
}

func TestMergeForSave_CwdChangeIsNotLossyAndCarriesCommandForward(t *testing.T) {
	// User: cd-d in one of the leaves, ran `boo save`. Cwd moves to
	// the new value, command stays put. Same leaf count → shape
	// preserved.
	prev := lay("1x2x1", tab("run", row(
		leaf("."),
		layout.Split{Cwd: "logs", Command: "tail -f app.log"},
	)))
	captured := lay("1x2x1", tab("run", row(
		leaf("."),
		leaf("logs/2024"), // user cd'd deeper
	)))
	merged, lost := mergeForSave(prev, captured)
	if len(lost) != 0 {
		t.Fatalf("unexpected loss: %v", lost)
	}
	rightLeaf := merged.Tabs[0].Root.Children[1]
	if rightLeaf.Cwd != "logs/2024" {
		t.Errorf("cwd should track captured, got %q", rightLeaf.Cwd)
	}
	if rightLeaf.Command != "tail -f app.log" {
		t.Errorf("command not carried, got %q", rightLeaf.Command)
	}
}

func TestMergeForSave_HandAuthoredColumnTreePreservedOnReSave(t *testing.T) {
	// Critical: this is what makes the tree model worthwhile. User
	// hand-authored a "triple" — row(a, col(b, c)) — re-saved without
	// changes. The flat capture has 3 leaves in DFS order; the merge
	// must recognise the leaf count matches prev and rebuild against
	// prev's shape, preserving the column.
	prev := lay("triple", tab("main", row(
		leaf("."),
		col(leaf("logs"), leaf("metrics")),
	)))
	// Capture would arrive flat (right-chain). buildFlatRoot yields:
	// row(., row(logs, metrics)).
	captured := lay("triple", tab("main", row(
		leaf("."),
		row(leaf("logs"), leaf("metrics")),
	)))
	merged, lost := mergeForSave(prev, captured)
	if len(lost) != 0 {
		t.Fatalf("unexpected loss: %v", lost)
	}
	// Right child of root must still be the column from prev.
	right := merged.Tabs[0].Root.Children[1]
	if right.Direction != layout.DirColumn {
		t.Fatalf("right subtree direction = %q, want column (prev shape lost)", right.Direction)
	}
	if d := diffForSave(prev, merged); d.Outcome != OutcomeSilent {
		t.Errorf("outcome = %v, want OutcomeSilent (re-save of hand-authored shape)", d.Outcome)
	}
}

func TestMergeForSave_AddedLeafTakesCapturedDefaultsAndDoesNotInventCommand(t *testing.T) {
	// User: opened a third pane via Cmd-D. Leaf count differs from
	// prev → flat-rebuild path. Aligned leaves (0, 1) carry their
	// invisibles forward; the new leaf at index 2 lands bare. We must
	// NOT pull command from leaf 1 onto leaf 2.
	prev := lay("1x2x1", tab("run", row(
		leaf("."),
		layout.Split{Cwd: ".", Command: "npm run dev"},
	)))
	captured := lay("1x1x2", tab("run", row(
		leaf("."),
		row(leaf("."), leaf(".")),
	)))
	merged, lost := mergeForSave(prev, captured)
	if len(lost) != 0 {
		t.Fatalf("unexpected loss for additive change: %v", lost)
	}
	leaves := collectLeaves(merged.Tabs[0].Root)
	if len(leaves) != 3 {
		t.Fatalf("expected 3 leaves, got %d", len(leaves))
	}
	if leaves[1].Command != "npm run dev" {
		t.Errorf("aligned leaf 1 lost its command: %q", leaves[1].Command)
	}
	if leaves[2].Command != "" {
		t.Errorf("new leaf 2 inherited a command it shouldn't have: %q", leaves[2].Command)
	}
}

func TestMergeForSave_RemovedLeafWithCommandIsReportedAsLossy(t *testing.T) {
	// User: closed the second pane. It held command + env. Leaf count
	// differs → flat-rebuild path. We can't keep the command (no leaf
	// to put it on) but MUST report the loss with a "dropped:" prefix
	// so the user is prompted before we destroy it.
	prev := lay("1x2x1", tab("run", row(
		leaf("."),
		layout.Split{
			Cwd:     "logs",
			Command: "tail -f app.log",
			Env:     map[string]string{"LOG": "debug"},
		},
	)))
	captured := lay("1x1x1", tab("run", leaf(".")))
	merged, lost := mergeForSave(prev, captured)
	leaves := collectLeaves(merged.Tabs[0].Root)
	if len(leaves) != 1 {
		t.Fatalf("merged should have 1 leaf (captured wins on shape), got %d", len(leaves))
	}
	joined := strings.Join(lost, "\n")
	if !strings.Contains(joined, "command") || !strings.Contains(joined, "env") {
		t.Errorf("expected loss notes for command and env, got: %v", lost)
	}
	if !strings.Contains(joined, "dropped") {
		t.Errorf("expected 'dropped' prefix on loss notes (merge owns dropped-leaf reasons), got: %v", lost)
	}
}

func TestMergeForSave_DroppedTabReportsLoss(t *testing.T) {
	// User: closed an entire tab that held a command.
	prev := lay("1x1x1",
		tab("keep", leaf(".")),
		tab("drop", layout.Split{Cwd: ".", Command: "make watch"}),
	)
	captured := lay("1x1x1", tab("keep", leaf(".")))
	merged, lost := mergeForSave(prev, captured)
	if len(merged.Tabs) != 1 {
		t.Fatalf("merged should have 1 tab, got %d", len(merged.Tabs))
	}
	if !strings.Contains(strings.Join(lost, "\n"), "make watch") {
		t.Errorf("expected loss notes mentioning the dropped command, got: %v", lost)
	}
}

func TestMergeForSave_AddedTabIsTakenAsCaptured(t *testing.T) {
	// User: opened a brand-new tab. No prev data → land as captured.
	prev := lay("1x1x1", tab("edit", leaf(".")))
	captured := lay("1x1x1",
		tab("edit", leaf(".")),
		tab("logs", leaf("logs")),
	)
	merged, lost := mergeForSave(prev, captured)
	if len(lost) != 0 {
		t.Fatalf("unexpected loss for purely additive change: %v", lost)
	}
	if len(merged.Tabs) != 2 || merged.Tabs[1].Name != "logs" {
		t.Fatalf("new tab missing or wrong: %+v", merged.Tabs)
	}
}

func TestMergeForSave_FirstSaveHasNoPrevAndIsLossless(t *testing.T) {
	// User: first save ever. prev is the zero Layout. Nothing to
	// merge, nothing to lose.
	captured := lay("1x1x1", tab("edit", leaf(".")))
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
	// captured. Cheap insurance — costs one map alloc per leaf.
	captured := lay("1x1x1", tab("edit", layout.Split{
		Cwd: ".", Env: map[string]string{"A": "1"},
	}))
	merged, _ := mergeForSave(layout.Layout{}, captured)
	merged.Tabs[0].Root.Env["A"] = "MUTATED"
	if captured.Tabs[0].Root.Env["A"] != "1" {
		t.Errorf("merged aliased captured's env map")
	}
}

func TestMergeForSave_PreservesPrevTabNameWhenCapturedIsBlank(t *testing.T) {
	// Defensive: if Ghostty returns a tab without a name (it usually
	// does — Ghostty <2.0 doesn't expose tab titles via AppleScript)
	// and prev had one, keep prev's name.
	prev := lay("1x1x1", tab("named-by-user", leaf(".")))
	captured := lay("1x1x1", layout.Tab{Root: leaf(".")}) // no name
	merged, _ := mergeForSave(prev, captured)
	if merged.Tabs[0].Name != "named-by-user" {
		t.Errorf("tab name not preserved, got %q", merged.Tabs[0].Name)
	}
}
