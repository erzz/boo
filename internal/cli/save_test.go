package cli

import (
	"strings"
	"testing"

	"github.com/erzz/boo/internal/ghostty"
	"github.com/erzz/boo/internal/project"
)

func TestCapturedToLayout_BasicShape(t *testing.T) {
	p := project.Project{Name: "demo", Dir: "/tmp/projA", Layout: "default"}
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
	if got.Name != "default" {
		t.Errorf("name = %q, want default", got.Name)
	}
	if len(got.Tabs) != 2 {
		t.Fatalf("tabs = %d, want 2", len(got.Tabs))
	}
	if got.Tabs[0].Splits[0].Cwd != "." {
		t.Errorf("first split cwd = %q, want '.'", got.Tabs[0].Splits[0].Cwd)
	}
	if got.Tabs[1].Splits[1].Direction != "right" {
		t.Errorf("non-primary direction = %q, want right", got.Tabs[1].Splits[1].Direction)
	}
	if got.Tabs[1].Splits[1].Cwd != "logs" {
		t.Errorf("relative cwd = %q, want 'logs'", got.Tabs[1].Splits[1].Cwd)
	}
	// First split of any tab must NOT have a direction.
	for i, tab := range got.Tabs {
		if tab.Splits[0].Direction != "" {
			t.Errorf("tab %d primary split has direction %q", i, tab.Splits[0].Direction)
		}
	}
	// capturedToLayout should be quiet for healthy input. Defensive
	// warnings (e.g. dropped empty tabs) are covered in their own test.
	if len(warnings) != 0 {
		t.Errorf("expected no warnings, got: %v", warnings)
	}
}

func TestCapturedToLayout_AbsolutePathsOutsideProjectAreKept(t *testing.T) {
	p := project.Project{Name: "demo", Dir: "/tmp/projA", Layout: "default"}
	desc := &ghostty.DescribedWindow{
		Tabs: []ghostty.DescribedTab{
			{Name: "edit", Terminals: []ghostty.DescribedTerminal{
				{ID: "t1", WorkingDirectory: "/var/log"},
			}},
		},
	}
	got, _ := capturedToLayout(p, desc)
	if got.Tabs[0].Splits[0].Cwd != "/var/log" {
		t.Fatalf("cwd = %q, want /var/log (outside project root)", got.Tabs[0].Splits[0].Cwd)
	}
}

func TestCapturedToLayout_DropsEmptyTabs(t *testing.T) {
	p := project.Project{Name: "demo", Dir: "/tmp/projA", Layout: "default"}
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
