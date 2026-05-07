package cli

import (
	"strings"
	"testing"

	"github.com/erzz/boo/internal/layout"
)

func TestDiffForSave_FirstSaveIsSilent(t *testing.T) {
	// No previous layout → first save → never lossy/structural by definition.
	next := layout.Layout{Name: "default", Tabs: []layout.Tab{
		{Name: "shell", Splits: []layout.Split{{Cwd: "."}}},
	}}
	d := diffForSave(layout.Layout{}, next)
	if d.Outcome != OutcomeSilent {
		t.Fatalf("outcome = %v, want OutcomeSilent", d.Outcome)
	}
	if len(d.ChangedTabs) != 0 || len(d.LossReasons) != 0 {
		t.Errorf("expected empty changes/losses on first save, got %+v", d)
	}
}

func TestDiffForSave_IdenticalIsSilent(t *testing.T) {
	l := layout.Layout{Name: "default", Tabs: []layout.Tab{
		{Name: "shell", Splits: []layout.Split{
			{Cwd: "."},
			{Direction: layout.DirRight, Cwd: "logs"},
		}},
	}}
	d := diffForSave(l, l)
	if d.Outcome != OutcomeSilent {
		t.Fatalf("outcome = %v, want OutcomeSilent (no-op save)", d.Outcome)
	}
}

func TestDiffForSave_TabAddedIsStructural(t *testing.T) {
	prev := layout.Layout{Name: "default", Tabs: []layout.Tab{
		{Name: "shell", Splits: []layout.Split{{Cwd: "."}}},
	}}
	next := layout.Layout{Name: "default", Tabs: []layout.Tab{
		{Name: "shell", Splits: []layout.Split{{Cwd: "."}}},
		{Name: "logs", Splits: []layout.Split{{Cwd: "logs"}}},
	}}
	d := diffForSave(prev, next)
	if d.Outcome != OutcomeStructural {
		t.Fatalf("outcome = %v, want OutcomeStructural", d.Outcome)
	}
	if len(d.ChangedTabs) != 1 || d.ChangedTabs[0].Index != 1 {
		t.Errorf("expected one changed tab at index 1, got %+v", d.ChangedTabs)
	}
}

func TestDiffForSave_SplitRemovedIsStructural(t *testing.T) {
	prev := layout.Layout{Name: "default", Tabs: []layout.Tab{
		{Name: "run", Splits: []layout.Split{
			{Cwd: "."},
			{Direction: layout.DirRight, Cwd: "logs"},
		}},
	}}
	next := layout.Layout{Name: "default", Tabs: []layout.Tab{
		{Name: "run", Splits: []layout.Split{{Cwd: "."}}},
	}}
	d := diffForSave(prev, next)
	if d.Outcome != OutcomeStructural {
		t.Fatalf("outcome = %v, want OutcomeStructural", d.Outcome)
	}
}

func TestDiffForSave_NonRightDirectionIsLossy(t *testing.T) {
	prev := layout.Layout{Name: "default", Tabs: []layout.Tab{
		{Name: "run", Splits: []layout.Split{
			{Cwd: "."},
			{Direction: layout.DirDown, Cwd: "logs"},
		}},
	}}
	next := layout.Layout{Name: "default", Tabs: []layout.Tab{
		{Name: "run", Splits: []layout.Split{
			{Cwd: "."},
			{Direction: layout.DirRight, Cwd: "logs"},
		}},
	}}
	d := diffForSave(prev, next)
	if d.Outcome != OutcomeLossy {
		t.Fatalf("outcome = %v, want OutcomeLossy", d.Outcome)
	}
	if len(d.LossReasons) != 1 || !strings.Contains(d.LossReasons[0], "down") {
		t.Errorf("expected loss reason mentioning 'down', got %v", d.LossReasons)
	}
	if len(d.ChangedTabs) != 1 || len(d.ChangedTabs[0].LossyCells) != 1 {
		t.Errorf("expected one tab with one lossy cell, got %+v", d.ChangedTabs)
	}
}

func TestDiffForSave_CommandSetIsLossy(t *testing.T) {
	prev := layout.Layout{Name: "default", Tabs: []layout.Tab{
		{Name: "edit", Splits: []layout.Split{{Cwd: ".", Command: "nvim"}}},
	}}
	next := layout.Layout{Name: "default", Tabs: []layout.Tab{
		{Name: "edit", Splits: []layout.Split{{Cwd: "."}}},
	}}
	d := diffForSave(prev, next)
	if d.Outcome != OutcomeLossy {
		t.Fatalf("outcome = %v, want OutcomeLossy", d.Outcome)
	}
}

func TestDiffForSave_EnvSetIsLossy(t *testing.T) {
	// Post-merge contract: a split's env is "lost" when the corresponding
	// next split doesn't carry it. Modelled here as next dropping the env
	// (e.g. user closed the split holding it; merge couldn't carry it
	// into a non-existent position).
	prev := layout.Layout{Name: "default", Tabs: []layout.Tab{
		{Name: "edit", Splits: []layout.Split{{Cwd: ".", Env: map[string]string{"FOO": "bar"}}}},
	}}
	next := layout.Layout{Name: "default", Tabs: []layout.Tab{
		{Name: "edit", Splits: []layout.Split{{Cwd: "."}}}, // env gone
	}}
	d := diffForSave(prev, next)
	if d.Outcome != OutcomeLossy {
		t.Fatalf("outcome = %v, want OutcomeLossy", d.Outcome)
	}
}

func TestDiffForSave_LossyWinsOverStructural(t *testing.T) {
	prev := layout.Layout{Name: "default", Tabs: []layout.Tab{
		{Name: "run", Splits: []layout.Split{
			{Cwd: "."},
			{Direction: layout.DirDown, Cwd: "logs"},
		}},
		{Name: "extra", Splits: []layout.Split{{Cwd: "."}}},
	}}
	next := layout.Layout{Name: "default", Tabs: []layout.Tab{
		{Name: "run", Splits: []layout.Split{
			{Cwd: "."},
			{Direction: layout.DirRight, Cwd: "logs"},
		}},
		// "extra" tab is gone → also a structural change
	}}
	d := diffForSave(prev, next)
	if d.Outcome != OutcomeLossy {
		t.Fatalf("outcome = %v, want OutcomeLossy (lossy must win over structural)", d.Outcome)
	}
}

func TestDiffForSave_PrimarySplitDirectionNotFlaggedAsLossy(t *testing.T) {
	// Validation forbids a non-empty direction on split 0; a zero-value
	// Direction on the primary split should never be reported as lossy
	// (otherwise every save with a 1-split tab would be lossy).
	prev := layout.Layout{Name: "default", Tabs: []layout.Tab{
		{Name: "shell", Splits: []layout.Split{{Cwd: "."}}},
	}}
	next := prev
	d := diffForSave(prev, next)
	if d.Outcome != OutcomeSilent {
		t.Fatalf("outcome = %v, want OutcomeSilent (single-split tab no-op)", d.Outcome)
	}
}

func TestDiffForSave_ReportsAllLossyFieldsOnOneSplit(t *testing.T) {
	// A single split can lose multiple properties at once. Each one
	// should be surfaced separately so the user sees the full picture
	// before approving the save.
	prev := layout.Layout{Name: "default", Tabs: []layout.Tab{
		{Name: "run", Splits: []layout.Split{
			{Cwd: "."},
			{
				Direction:    layout.DirDown,
				Cwd:          "logs",
				Command:      "tail -f app.log",
				InitialInput: "clear\n",
				Env:          map[string]string{"LOG_LEVEL": "debug", "TZ": "UTC"},
			},
		}},
	}}
	next := layout.Layout{Name: "default", Tabs: []layout.Tab{
		{Name: "run", Splits: []layout.Split{
			{Cwd: "."},
			{Direction: layout.DirRight, Cwd: "logs"},
		}},
	}}
	d := diffForSave(prev, next)
	if d.Outcome != OutcomeLossy {
		t.Fatalf("outcome = %v, want OutcomeLossy", d.Outcome)
	}
	// Expect one reason per lossy property: command, initial_input, env, direction.
	if got := len(d.LossReasons); got != 4 {
		t.Fatalf("LossReasons count = %d, want 4 (one per lossy property), got: %v", got, d.LossReasons)
	}
	joined := strings.Join(d.LossReasons, "\n")
	for _, want := range []string{"command", "initial_input", "env var", "direction"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q in loss reasons:\n%s", want, joined)
		}
	}
	// Same split listed once in LossyCells (despite 4 reasons).
	if len(d.ChangedTabs) != 1 || len(d.ChangedTabs[0].LossyCells) != 1 {
		t.Errorf("expected one tab with one lossy cell, got %+v", d.ChangedTabs)
	}
}
