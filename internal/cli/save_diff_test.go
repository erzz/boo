package cli

import (
	"strings"
	"testing"

	"github.com/erzz/boo/internal/layout"
)

// Test helpers — short-name wrappers so fixtures stay readable. They mirror
// the helpers used in internal/layoutpreview tests so anyone moving between
// the two packages sees the same vocabulary.

func leaf(cwd string) layout.Split { return layout.Split{Cwd: cwd} }

func leafCmd(cwd, cmd string) layout.Split { return layout.Split{Cwd: cwd, Command: cmd} }

func leafEnv(cwd string, env map[string]string) layout.Split {
	return layout.Split{Cwd: cwd, Env: env}
}

func row(children ...layout.Split) layout.Split {
	return layout.Split{Direction: layout.DirRow, Children: children}
}

func col(children ...layout.Split) layout.Split {
	return layout.Split{Direction: layout.DirColumn, Children: children}
}

func tab(name string, root layout.Split) layout.Tab { return layout.Tab{Name: name, Root: root} }

func lay(name string, tabs ...layout.Tab) layout.Layout {
	return layout.Layout{Name: name, Tabs: tabs}
}

func TestDiffForSave_FirstSaveIsSilent(t *testing.T) {
	// No previous layout → first save → never lossy/structural by definition.
	next := lay("1x1x1", tab("shell", leaf(".")))
	d := diffForSave(layout.Layout{}, next)
	if d.Outcome != OutcomeSilent {
		t.Fatalf("outcome = %v, want OutcomeSilent", d.Outcome)
	}
	if len(d.ChangedTabs) != 0 || len(d.LossReasons) != 0 {
		t.Errorf("expected empty changes/losses on first save, got %+v", d)
	}
}

func TestDiffForSave_IdenticalIsSilent(t *testing.T) {
	l := lay("1x2x1", tab("shell", row(leaf("."), leaf("logs"))))
	d := diffForSave(l, l)
	if d.Outcome != OutcomeSilent {
		t.Fatalf("outcome = %v, want OutcomeSilent (no-op save)", d.Outcome)
	}
}

func TestDiffForSave_TabAddedIsStructural(t *testing.T) {
	prev := lay("1x1x1", tab("shell", leaf(".")))
	next := lay("1x1x1",
		tab("shell", leaf(".")),
		tab("logs", leaf("logs")),
	)
	d := diffForSave(prev, next)
	if d.Outcome != OutcomeStructural {
		t.Fatalf("outcome = %v, want OutcomeStructural", d.Outcome)
	}
	if len(d.ChangedTabs) != 1 || d.ChangedTabs[0].Index != 1 {
		t.Errorf("expected one changed tab at index 1, got %+v", d.ChangedTabs)
	}
}

func TestDiffForSave_LeafRemovedIsStructural(t *testing.T) {
	// A pane was closed: prev had 2 leaves, next has 1. That's a leaf-count
	// change — structural at the diff level. (No invisible data was lost,
	// so it's not lossy.)
	prev := lay("1x2x1", tab("run", row(leaf("."), leaf("logs"))))
	next := lay("1x1x1", tab("run", leaf(".")))
	d := diffForSave(prev, next)
	if d.Outcome != OutcomeStructural {
		t.Fatalf("outcome = %v, want OutcomeStructural", d.Outcome)
	}
}

func TestDiffForSave_TreeShapeChangedIsStructural(t *testing.T) {
	// Same leaf count (3) but different nesting: prev is row(leaf,
	// col(leaf, leaf)) — the canonical "triple" — and next has been
	// flattened by the merge into a right-chain row(leaf, row(leaf,
	// leaf)). Same leaves, different shape → structural.
	prev := lay("triple", tab("main", row(leaf("a"), col(leaf("b"), leaf("c")))))
	next := lay("triple", tab("main", row(leaf("a"), row(leaf("b"), leaf("c")))))
	d := diffForSave(prev, next)
	if d.Outcome != OutcomeStructural {
		t.Fatalf("outcome = %v, want OutcomeStructural (tree shape changed)", d.Outcome)
	}
}

func TestDiffForSave_CommandDroppedIsLossy(t *testing.T) {
	// A leaf with a Command in prev whose merged counterpart no longer
	// has it (because the merge couldn't carry it forward, e.g. shape
	// mismatch turning into the flat-rebuild path that empties the leaf).
	// We model the post-merge state directly here.
	prev := lay("1x1x1", tab("edit", leafCmd(".", "nvim")))
	next := lay("1x1x1", tab("edit", leaf(".")))
	d := diffForSave(prev, next)
	if d.Outcome != OutcomeLossy {
		t.Fatalf("outcome = %v, want OutcomeLossy", d.Outcome)
	}
	if len(d.LossReasons) != 1 || !strings.Contains(d.LossReasons[0], "command") {
		t.Errorf("expected loss reason mentioning 'command', got %v", d.LossReasons)
	}
	if len(d.ChangedTabs) != 1 || len(d.ChangedTabs[0].LossyLeaves) != 1 {
		t.Errorf("expected one tab with one lossy leaf, got %+v", d.ChangedTabs)
	}
}

func TestDiffForSave_EnvDroppedIsLossy(t *testing.T) {
	// Same idea as Command but for Env. Environment was authored on the
	// previous layout; the merged result didn't carry it (e.g. the leaf
	// it lived on was closed and rebuilt without it).
	prev := lay("1x1x1", tab("edit", leafEnv(".", map[string]string{"FOO": "bar"})))
	next := lay("1x1x1", tab("edit", leaf(".")))
	d := diffForSave(prev, next)
	if d.Outcome != OutcomeLossy {
		t.Fatalf("outcome = %v, want OutcomeLossy", d.Outcome)
	}
	if len(d.LossReasons) == 0 || !strings.Contains(d.LossReasons[0], "env var") {
		t.Errorf("expected loss reason mentioning 'env var', got %v", d.LossReasons)
	}
}

func TestDiffForSave_DroppedLeafEmitsCellMarkerNotTextReason(t *testing.T) {
	// Closed-pane case: prev leaf has a counterpart-less position in
	// next. The diff must mark the leaf as lossy (so the renderer can
	// flag it visually) but NOT emit a textual reason — that's
	// mergeForSave's job (see the "dropped:" prefix). Doing both would
	// duplicate every closed-pane reason in the user's terminal.
	prev := lay("1x2x1", tab("run", row(leaf("."), leafCmd("logs", "tail -f"))))
	next := lay("1x1x1", tab("run", leaf(".")))
	d := diffForSave(prev, next)
	if d.Outcome != OutcomeLossy {
		t.Fatalf("outcome = %v, want OutcomeLossy", d.Outcome)
	}
	// Cell marker for the dropped leaf at index 1: present.
	if len(d.ChangedTabs) != 1 || len(d.ChangedTabs[0].LossyLeaves) != 1 ||
		d.ChangedTabs[0].LossyLeaves[0] != 1 {
		t.Errorf("expected lossy marker on leaf 1, got %+v", d.ChangedTabs)
	}
	// Text reason: empty here (mergeForSave owns it).
	if len(d.LossReasons) != 0 {
		t.Errorf("text reasons should be empty for dropped leaves (merge owns them), got %v", d.LossReasons)
	}
}

func TestDiffForSave_LossyWinsOverStructural(t *testing.T) {
	// Tab 1 dropped (structural) AND a command lost on tab 0 (lossy).
	// Lossy outcome must win so the user gets the prompt-with-marker
	// flow rather than the silent --force structural path.
	prev := lay("1x1x1",
		tab("edit", leafCmd(".", "nvim")),
		tab("extra", leaf(".")),
	)
	next := lay("1x1x1",
		tab("edit", leaf(".")), // command dropped
		// extra tab gone
	)
	d := diffForSave(prev, next)
	if d.Outcome != OutcomeLossy {
		t.Fatalf("outcome = %v, want OutcomeLossy (lossy must win over structural)", d.Outcome)
	}
}

func TestDiffForSave_ReportsAllLossyFieldsOnOneLeaf(t *testing.T) {
	// A single leaf can lose multiple properties at once (command,
	// initial_input, env). Each one should be surfaced separately so
	// the user sees the full picture before approving the save. The
	// leaf itself is listed once in LossyLeaves regardless of how many
	// fields it lost.
	prev := lay("1x1x1", tab("run", layout.Split{
		Cwd:          "logs",
		Command:      "tail -f app.log",
		InitialInput: "clear\n",
		Env:          map[string]string{"LOG_LEVEL": "debug", "TZ": "UTC"},
	}))
	next := lay("1x1x1", tab("run", leaf("logs")))
	d := diffForSave(prev, next)
	if d.Outcome != OutcomeLossy {
		t.Fatalf("outcome = %v, want OutcomeLossy", d.Outcome)
	}
	// Expect one reason per lossy property: command, initial_input, env.
	// Direction is no longer a leaf field, so it's not in this list.
	if got := len(d.LossReasons); got != 3 {
		t.Fatalf("LossReasons count = %d, want 3 (command + initial_input + env), got: %v", got, d.LossReasons)
	}
	joined := strings.Join(d.LossReasons, "\n")
	for _, want := range []string{"command", "initial_input", "env var"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q in loss reasons:\n%s", want, joined)
		}
	}
	// Leaf 0 listed once in LossyLeaves despite carrying 3 lost fields.
	if len(d.ChangedTabs) != 1 || len(d.ChangedTabs[0].LossyLeaves) != 1 ||
		d.ChangedTabs[0].LossyLeaves[0] != 0 {
		t.Errorf("expected one tab with LossyLeaves=[0], got %+v", d.ChangedTabs)
	}
}

func TestDiffForSave_LeafIndexIsDepthFirst(t *testing.T) {
	// Leaf indices in LossyLeaves must be the depth-first walk order,
	// matching collectLeaves and the JXA walker. This pins the
	// invariant that the renderer can use the index to colour the
	// right cell in the diff.
	//
	// Tree: row(leafA, col(leafB-with-cmd, leafC))
	// DFS order: A=0, B=1, C=2. Loss is on B → LossyLeaves=[1].
	prev := lay("triple", tab("main", row(
		leaf("a"),
		col(leafCmd("b", "make watch"), leaf("c")),
	)))
	// Same shape, B's command dropped.
	next := lay("triple", tab("main", row(
		leaf("a"),
		col(leaf("b"), leaf("c")),
	)))
	d := diffForSave(prev, next)
	if d.Outcome != OutcomeLossy {
		t.Fatalf("outcome = %v, want OutcomeLossy", d.Outcome)
	}
	if len(d.ChangedTabs) != 1 || len(d.ChangedTabs[0].LossyLeaves) != 1 ||
		d.ChangedTabs[0].LossyLeaves[0] != 1 {
		t.Fatalf("expected LossyLeaves=[1] (DFS index of leaf B), got %+v", d.ChangedTabs[0].LossyLeaves)
	}
}
