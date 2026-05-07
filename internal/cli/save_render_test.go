package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/erzz/boo/internal/layout"
)

func TestRenderTabDiff_StructuralAdd(t *testing.T) {
	td := TabDiff{
		Index: 1,
		Name:  "logs",
		Prev:  nil,
		Next:  &layout.Tab{Name: "logs", Root: layout.Split{Cwd: "logs"}},
	}
	got := renderTabDiff(td)
	// Sanity: contains the title, the "(removed)" placeholder for prev,
	// and a cell with "logs". Don't golden-match the whole thing — pad/box
	// alignment is exercised by the side-by-side test below.
	if !strings.Contains(got, "Tab 1 \"logs\"") {
		t.Errorf("missing title row: %s", got)
	}
	if !strings.Contains(got, "(removed)") {
		t.Errorf("expected '(removed)' for nil Prev, got: %s", got)
	}
	if !strings.Contains(got, "logs") {
		t.Errorf("expected cwd 'logs' on right side, got: %s", got)
	}
}

func TestRenderTabDiff_LossyMarksAffectedLeaves(t *testing.T) {
	// Prev: row(., logs-with-cmd). Next: row(., logs) — command dropped.
	// Leaf 1 is lossy. The renderer should mark it with "L1 !" on the
	// label row of the LEFT (prev) side; the right side carries no '!'
	// because the post-save shape isn't lossy (the loss already happened).
	td := TabDiff{
		Index: 0,
		Name:  "run",
		Prev: &layout.Tab{Name: "run", Root: layout.Split{
			Direction: layout.DirRow,
			Children: []layout.Split{
				{Cwd: "."},
				{Cwd: "logs", Command: "tail -f"},
			},
		}},
		Next: &layout.Tab{Name: "run", Root: layout.Split{
			Direction: layout.DirRow,
			Children:  []layout.Split{{Cwd: "."}, {Cwd: "logs"}},
		}},
		LossyLeaves: []int{1},
	}
	got := renderTabDiff(td)
	// The lossy leaf carries a trailing '!' on its label cell.
	if !strings.Contains(got, "L1 !") {
		t.Errorf("expected 'L1 !' marker on lossy leaf, got:\n%s", got)
	}
	// Left side should also still show the (cmd) annotation on leaf 1.
	if !strings.Contains(got, "(cmd)") {
		t.Errorf("expected '(cmd)' annotation on prev leaf 1, got:\n%s", got)
	}
	// On every row that contains the arrow, the right side (after the
	// arrow) should not contain '!' — the post-save shape isn't lossy.
	for _, line := range strings.Split(got, "\n") {
		if !strings.Contains(line, "→") {
			continue
		}
		_, right, _ := strings.Cut(line, "→")
		if strings.Contains(right, "!") {
			t.Errorf("right side of arrow row should not contain '!':\n%s", got)
		}
	}
}

func TestRenderDiff_SilentOutcomeWritesNothing(t *testing.T) {
	var buf bytes.Buffer
	renderDiff(SaveDiff{Outcome: OutcomeSilent}, &buf)
	if buf.Len() != 0 {
		t.Errorf("OutcomeSilent should write nothing, got: %q", buf.String())
	}
}

func TestRenderDiff_LossyHeaderAndReasons(t *testing.T) {
	var buf bytes.Buffer
	renderDiff(SaveDiff{
		Outcome: OutcomeLossy,
		ChangedTabs: []TabDiff{{
			Index: 0,
			Name:  "run",
			Prev: &layout.Tab{Name: "run", Root: layout.Split{
				Direction: layout.DirRow,
				Children: []layout.Split{
					{Cwd: "."},
					{Cwd: "logs", Command: "tail -f"},
				},
			}},
			Next: &layout.Tab{Name: "run", Root: layout.Split{
				Direction: layout.DirRow,
				Children:  []layout.Split{{Cwd: "."}, {Cwd: "logs"}},
			}},
			LossyLeaves: []int{1},
		}},
		LossReasons: []string{`tab 0 ("run") leaf 1: command "tail -f" will be lost`},
	}, &buf)
	out := buf.String()
	if !strings.Contains(out, "CANNOT be recovered") {
		t.Errorf("expected lossy header, got:\n%s", out)
	}
	if !strings.Contains(out, "Unrecoverable on next save:") {
		t.Errorf("expected 'Unrecoverable on next save:' section, got:\n%s", out)
	}
	if !strings.Contains(out, `command "tail -f"`) {
		t.Errorf("expected loss reason in body, got:\n%s", out)
	}
}

func TestPadCell_TruncatesWithEllipsis(t *testing.T) {
	// Cell is 12 wide, capacity = 10 chars before truncation.
	got := padCell("abcdefghijklmnop")
	if len(got) != renderCellWidth {
		t.Fatalf("padded len = %d, want %d (got %q)", len(got), renderCellWidth, got)
	}
	if !strings.Contains(got, "~") {
		t.Errorf("expected truncation marker '~', got %q", got)
	}
}

func TestPadCell_PadsShortContent(t *testing.T) {
	got := padCell("hi")
	if len(got) != renderCellWidth {
		t.Fatalf("padded len = %d, want %d (got %q)", len(got), renderCellWidth, got)
	}
	if !strings.Contains(got, "hi") {
		t.Errorf("expected content 'hi' in cell, got %q", got)
	}
}
