package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	booexec "github.com/erzz/boo/internal/exec"
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

func TestSave_RejectsResponsiveLayouts(t *testing.T) {
	a := makeAppForCmds(t)
	dir := t.TempDir()
	if err := os.MkdirAll(a.Paths.ProjectDir("proj"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	responsive := layout.Layout{
		Name: "responsive",
		Variants: []layout.Variant{
			{Tabs: []layout.Tab{{Root: layout.Split{Cwd: "."}}}},
			{MinCols: 120, Tabs: []layout.Tab{{Root: layout.Split{Direction: layout.DirRow, Children: []layout.Split{{Cwd: "."}, {Cwd: "logs"}}}}}},
		},
	}
	if err := project.SaveLayout(a.Paths, "proj", responsive); err != nil {
		t.Fatalf("SaveLayout: %v", err)
	}
	reg, err := project.Load(a.Paths)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := reg.Add(project.Project{Name: "proj", Dir: dir, Layout: "responsive"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := reg.Save(a.Paths); err != nil {
		t.Fatalf("Save registry: %v", err)
	}
	if err := project.SaveRuntime(a.Paths, "proj", project.Runtime{WindowID: "win-1"}); err != nil {
		t.Fatalf("SaveRuntime: %v", err)
	}

	fake := booexec.NewFake(func(_ string, _ []string, stdin []byte) ([]byte, []byte, error) {
		if strings.Contains(string(stdin), `"windowId":"win-1"`) {
			return []byte(`{"exists":true}`), nil, nil
		}
		return nil, nil, nil
	})
	a.Ghostty = ghostty.New(fake)

	cmd := newSaveCmdWithApp(a)
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"proj"})
	err = cmd.Execute()
	if err == nil {
		t.Fatal("expected responsive save error, got nil")
	}
	if !strings.Contains(err.Error(), "responsive layout") {
		t.Fatalf("error = %v, want responsive-layout message", err)
	}
}

func TestSave_RejectsResponsiveLayoutsWhenSnapshotMissing(t *testing.T) {
	a := makeAppForCmds(t)
	dir := t.TempDir()
	responsive := []byte("name: responsive\nvariants:\n  - tabs:\n      - split:\n          cwd: .\n  - min_cols: 120\n    tabs:\n      - split:\n          direction: row\n          children:\n            - cwd: .\n            - cwd: logs\n")
	if err := os.WriteFile(filepath.Join(a.Paths.LayoutsDir, "responsive.yaml"), responsive, 0o644); err != nil {
		t.Fatalf("write template: %v", err)
	}
	registerProjectForTest(t, a, "proj", dir, "responsive")
	if err := os.Remove(a.Paths.ProjectLayoutFile("proj")); err != nil {
		t.Fatalf("remove snapshot: %v", err)
	}
	if err := project.SaveRuntime(a.Paths, "proj", project.Runtime{WindowID: "win-1"}); err != nil {
		t.Fatalf("SaveRuntime: %v", err)
	}
	a.Ghostty = ghostty.New(booexec.NewFake(func(_ string, _ []string, stdin []byte) ([]byte, []byte, error) {
		if strings.Contains(string(stdin), `"windowId":"win-1"`) {
			return []byte(`{"exists":true}`), nil, nil
		}
		return nil, nil, nil
	}))
	cmd := newSaveCmdWithApp(a)
	cmd.SetArgs([]string{"proj"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "responsive layout") {
		t.Fatalf("err = %v, want responsive-layout rejection", err)
	}
}

func TestSave_RejectsResponsiveLayoutsWhenSnapshotUnreadable(t *testing.T) {
	a := makeAppForCmds(t)
	dir := t.TempDir()
	responsive := []byte("name: responsive\nvariants:\n  - tabs:\n      - split:\n          cwd: .\n  - min_cols: 120\n    tabs:\n      - split:\n          direction: row\n          children:\n            - cwd: .\n            - cwd: logs\n")
	if err := os.WriteFile(filepath.Join(a.Paths.LayoutsDir, "responsive.yaml"), responsive, 0o644); err != nil {
		t.Fatalf("write template: %v", err)
	}
	registerProjectForTest(t, a, "proj", dir, "responsive")
	if err := os.WriteFile(a.Paths.ProjectLayoutFile("proj"), []byte("name: broken\nvariants: ["), 0o644); err != nil {
		t.Fatalf("write broken snapshot: %v", err)
	}
	if err := project.SaveRuntime(a.Paths, "proj", project.Runtime{WindowID: "win-1"}); err != nil {
		t.Fatalf("SaveRuntime: %v", err)
	}
	a.Ghostty = ghostty.New(booexec.NewFake(func(_ string, _ []string, stdin []byte) ([]byte, []byte, error) {
		if strings.Contains(string(stdin), `"windowId":"win-1"`) {
			return []byte(`{"exists":true}`), nil, nil
		}
		return nil, nil, nil
	}))
	cmd := newSaveCmdWithApp(a)
	cmd.SetArgs([]string{"proj"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "responsive layout") {
		t.Fatalf("err = %v, want responsive-layout rejection", err)
	}
}

func TestSave_RejectsResponsiveLayoutsBeforeWindowChecks(t *testing.T) {
	a := makeAppForCmds(t)
	dir := t.TempDir()
	responsive := layout.Layout{
		Name: "responsive",
		Variants: []layout.Variant{
			{Tabs: []layout.Tab{{Root: layout.Split{Cwd: "."}}}},
			{MinCols: 120, Tabs: []layout.Tab{{Root: layout.Split{Direction: layout.DirRow, Children: []layout.Split{{Cwd: "."}, {Cwd: "logs"}}}}}},
		},
	}
	if err := project.SaveLayout(a.Paths, "proj", responsive); err != nil {
		t.Fatalf("SaveLayout: %v", err)
	}
	reg, err := project.Load(a.Paths)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := reg.Add(project.Project{Name: "proj", Dir: dir, Layout: "responsive"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := reg.Save(a.Paths); err != nil {
		t.Fatalf("Save registry: %v", err)
	}

	cmd := newSaveCmdWithApp(a)
	cmd.SetArgs([]string{"proj"})
	err = cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "responsive layout") {
		t.Fatalf("err = %v, want responsive-layout rejection before window checks", err)
	}
}

func TestMatchFrontWindow_DoesNotPersistRecoveredWindowID(t *testing.T) {
	a := makeAppForCmds(t)
	dir := t.TempDir()
	registerProjectForTest(t, a, "proj", dir, "triple")
	if err := project.SaveRuntime(a.Paths, "proj", project.Runtime{}); err != nil {
		t.Fatalf("SaveRuntime: %v", err)
	}
	a.Ghostty = ghostty.New(booexec.NewFake(func(_ string, _ []string, stdin []byte) ([]byte, []byte, error) {
		switch {
		case stdin == nil:
			return []byte(`{"windowId":"win-1"}`), nil, nil
		default:
			return []byte(`{"tabs":[{"terminals":[{"workingDirectory":"` + dir + `"}]}]}`), nil, nil
		}
	}))
	reg, err := project.Load(a.Paths)
	if err != nil {
		t.Fatalf("Load registry: %v", err)
	}

	match, err := matchFrontWindow(nil, a, reg)
	if err != nil {
		t.Fatalf("matchFrontWindow: %v", err)
	}
	if match.recoveredWindowID != "win-1" {
		t.Fatalf("recoveredWindowID = %q, want win-1", match.recoveredWindowID)
	}
	rt, err := project.LoadRuntime(a.Paths, "proj")
	if err != nil {
		t.Fatalf("LoadRuntime: %v", err)
	}
	if rt.WindowID != "" {
		t.Fatalf("runtime WindowID = %q, want unchanged empty string", rt.WindowID)
	}
}
