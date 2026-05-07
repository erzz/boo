package cli

import (
	"strings"
	"testing"

	"github.com/erzz/boo/internal/ghostty"
	"github.com/erzz/boo/internal/layout"
	"github.com/erzz/boo/internal/project"
)

func TestCapturedToLayout_BasicShape(t *testing.T) {
	// Tab 0: 1 terminal → leaf root.
	// Tab 1: 2 terminals → row(leaf, leaf) (the "flat tree" produced
	// by buildFlatRoot for N>=2).
	p := project.Project{Name: "demo", Dir: "/tmp/projA", Layout: "1x1x1"}
	desc := &ghostty.DescribedWindow{
		Tabs: []ghostty.DescribedTab{
			{Name: "edit", Terminals: []ghostty.DescribedTerminal{
				{ID: "t1", WorkingDirectory: "/tmp/projA"},
			}},
			{Name: "run", Terminals: []ghostty.DescribedTerminal{
				{ID: "t2", WorkingDirectory: "/tmp/projA"},
				{ID: "t3", WorkingDirectory: "/tmp/projA/logs"},
			}},
		},
	}
	got, warnings := capturedToLayout(p, desc)
	if err := got.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if got.Name != "1x1x1" {
		t.Errorf("name = %q, want 1x1x1", got.Name)
	}
	if len(got.Tabs) != 2 {
		t.Fatalf("tabs = %d, want 2", len(got.Tabs))
	}

	// Tab 0: single-leaf root.
	tab0 := got.Tabs[0]
	if !tab0.Root.IsLeaf() {
		t.Errorf("tab 0 root should be a leaf for 1 terminal, got %+v", tab0.Root)
	}
	if tab0.Root.Cwd != "." {
		t.Errorf("tab 0 cwd = %q, want '.'", tab0.Root.Cwd)
	}

	// Tab 1: row(leaf, leaf).
	tab1 := got.Tabs[1]
	if tab1.Root.IsLeaf() {
		t.Fatalf("tab 1 root should be interior for 2 terminals, got leaf %+v", tab1.Root)
	}
	if tab1.Root.Direction != layout.DirRow {
		t.Errorf("tab 1 root direction = %q, want row", tab1.Root.Direction)
	}
	leaves := collectLeaves(tab1.Root)
	if len(leaves) != 2 {
		t.Fatalf("tab 1 leaves = %d, want 2", len(leaves))
	}
	if leaves[0].Cwd != "." {
		t.Errorf("tab 1 leaf 0 cwd = %q, want '.'", leaves[0].Cwd)
	}
	if leaves[1].Cwd != "logs" {
		t.Errorf("tab 1 leaf 1 cwd = %q, want 'logs'", leaves[1].Cwd)
	}

	// Leaves carry no Direction (direction is interior-only).
	for ti, tb := range got.Tabs {
		for li, lf := range collectLeaves(tb.Root) {
			if lf.Direction != "" {
				t.Errorf("tab %d leaf %d has direction %q (leaves must not)", ti, li, lf.Direction)
			}
		}
	}

	// capturedToLayout should be quiet for healthy input.
	if len(warnings) != 0 {
		t.Errorf("expected no warnings, got: %v", warnings)
	}
}

func TestCapturedToLayout_AbsolutePathsOutsideProjectAreKept(t *testing.T) {
	p := project.Project{Name: "demo", Dir: "/tmp/projA", Layout: "1x1x1"}
	desc := &ghostty.DescribedWindow{
		Tabs: []ghostty.DescribedTab{
			{Name: "edit", Terminals: []ghostty.DescribedTerminal{
				{ID: "t1", WorkingDirectory: "/var/log"},
			}},
		},
	}
	got, _ := capturedToLayout(p, desc)
	if got.Tabs[0].Root.Cwd != "/var/log" {
		t.Fatalf("cwd = %q, want /var/log (outside project root)", got.Tabs[0].Root.Cwd)
	}
}

func TestCapturedToLayout_DropsEmptyTabs(t *testing.T) {
	p := project.Project{Name: "demo", Dir: "/tmp/projA", Layout: "1x1x1"}
	desc := &ghostty.DescribedWindow{
		Tabs: []ghostty.DescribedTab{
			{Name: "good", Terminals: []ghostty.DescribedTerminal{
				{ID: "t1", WorkingDirectory: "/tmp/projA"},
			}},
			{Name: "ghost"}, // no terminals
		},
	}
	got, warnings := capturedToLayout(p, desc)
	if len(got.Tabs) != 1 {
		t.Fatalf("tabs = %d, want 1 (empty dropped)", len(got.Tabs))
	}
	if !strings.Contains(strings.Join(warnings, "\n"), "ghost") {
		t.Errorf("expected warning naming the dropped tab, got %v", warnings)
	}
}

func TestCapturedToLayout_PreservesProjectLayoutName(t *testing.T) {
	p := project.Project{Name: "demo", Dir: "/tmp/projA", Layout: "dev"}
	desc := &ghostty.DescribedWindow{
		Tabs: []ghostty.DescribedTab{
			{Terminals: []ghostty.DescribedTerminal{
				{ID: "t1", WorkingDirectory: "/tmp/projA"},
			}},
		},
	}
	got, _ := capturedToLayout(p, desc)
	if got.Name != "dev" {
		t.Errorf("name = %q, want dev", got.Name)
	}
}

func TestRelativiseCwd(t *testing.T) {
	cases := []struct {
		project, cwd, want string
	}{
		{"/p", "/p", "."},
		{"/p", "/p/sub", "sub"},
		{"/p", "/p/a/b", "a/b"},
		{"/p", "/other", "/other"},
		{"/p", "", "."},
		{"/p/", "/p/sub", "sub"},
	}
	for _, c := range cases {
		if got := relativiseCwd(c.project, c.cwd); got != c.want {
			t.Errorf("relativiseCwd(%q,%q) = %q, want %q", c.project, c.cwd, got, c.want)
		}
	}
}
