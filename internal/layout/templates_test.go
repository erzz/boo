package layout

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// builtinNames is the curated catalogue we ship. Naming schema:
// `tabs × cols × rows` for grids, plus `triple` as the named exception.
//
// Adding to this list = embedding a new YAML file under templates/ and
// (probably) updating docs. Removing = breaking change to anyone using
// `--layout <name>`. Tests below use this slice as the single source of
// truth so adding/removing in only one place doesn't drift.
var builtinNames = []string{
	"1x1x1", "1x2x1", "1x1x2", "1x2x2",
	"2x1x1", "2x2x1", "2x2x2",
	"triple",
}

// Every built-in must resolve, validate, and ship a description block.
// One table-driven test instead of N near-identical tests so the failure
// message points at the exact template that broke.
func TestResolveTemplate_AllBuiltins(t *testing.T) {
	for _, name := range builtinNames {
		t.Run(name, func(t *testing.T) {
			r, err := ResolveTemplate("", name)
			if err != nil {
				t.Fatalf("resolve: %v", err)
			}
			if r.Source != SourceBuiltin {
				t.Errorf("Source = %s, want builtin", r.Source)
			}
			if r.Layout.Name != name {
				t.Errorf("Layout.Name = %q, want %q", r.Layout.Name, name)
			}
			if err := r.Layout.Validate(); err != nil {
				t.Errorf("validate: %v", err)
			}
			if r.Description == "" {
				t.Errorf("missing description (add a leading # comment block to %s.yaml)", name)
			}
		})
	}
}

// Each built-in must have the structural shape its name promises. This
// is the regression net for the tabs × cols × rows schema — a future
// hand-edit that breaks the contract gets caught immediately.
//
// `triple` and grid templates with internal trees (1x2x2, 2x2x2) need
// their tree shape pinned, not just a count.
func TestBuiltinShapes(t *testing.T) {
	type check struct {
		tabs int
		// rootKind is "leaf" for a single-pane tab, or "row"/"column"
		// followed by a pipe-separated child shape: "row|leaf,leaf"
		// means root is a row with two leaf children. We use a tiny
		// language rather than a deep struct comparison because the
		// failure messages are dramatically more readable.
		rootShape string
	}
	want := map[string]check{
		"1x1x1":  {1, "leaf"},
		"1x2x1":  {1, "row|leaf,leaf"},
		"1x1x2":  {1, "column|leaf,leaf"},
		"1x2x2":  {1, "row|column|leaf,leaf,column|leaf,leaf"},
		"2x1x1":  {2, "leaf"},
		"2x2x1":  {2, "row|leaf,leaf"},
		"2x2x2":  {2, "row|column|leaf,leaf,column|leaf,leaf"},
		"triple": {1, "row|leaf,column|leaf,leaf"},
	}
	for name, w := range want {
		t.Run(name, func(t *testing.T) {
			r, err := ResolveTemplate("", name)
			if err != nil {
				t.Fatalf("resolve: %v", err)
			}
			if got := len(r.Layout.Tabs); got != w.tabs {
				t.Fatalf("tabs = %d, want %d", got, w.tabs)
			}
			// Every tab in a built-in must share the same root shape.
			// (None of our built-ins mix shapes per tab.)
			for i, tab := range r.Layout.Tabs {
				got := describeShape(tab.Root)
				if got != w.rootShape {
					t.Errorf("tab %d shape = %q, want %q", i, got, w.rootShape)
				}
			}
		})
	}
}

// describeShape renders a Split tree as the tiny shape language used in
// TestBuiltinShapes. Kept private to the test file — it's a debugging
// aid, not API.
func describeShape(s Split) string {
	if s.IsLeaf() {
		return "leaf"
	}
	parts := make([]string, len(s.Children))
	for i, c := range s.Children {
		parts[i] = describeShape(c)
	}
	return s.Direction + "|" + strings.Join(parts, ",")
}

// Every built-in must be tool-agnostic. None can ship with a baked-in
// `command` (no editors, no runners, no language tooling). If a future
// change adds one to a built-in, this test catches it before it bites
// users without that tool installed.
func TestBuiltins_AreToolAgnostic(t *testing.T) {
	for _, name := range builtinNames {
		t.Run(name, func(t *testing.T) {
			r, err := ResolveTemplate("", name)
			if err != nil {
				t.Fatalf("resolve: %v", err)
			}
			for i, tab := range r.Layout.Tabs {
				assertNoCommands(t, tab.Root, "tab "+itoa(i))
			}
		})
	}
}

func assertNoCommands(t *testing.T, s Split, path string) {
	t.Helper()
	if s.IsLeaf() {
		if s.Command != "" {
			t.Errorf("%s leaf has command %q (built-ins must be tool-agnostic)", path, s.Command)
		}
		return
	}
	for i, c := range s.Children {
		assertNoCommands(t, c, path+"/"+itoa(i))
	}
}

func itoa(i int) string {
	// Tiny inline alternative to importing strconv just for one call.
	if i == 0 {
		return "0"
	}
	var buf [4]byte
	n := 0
	for i > 0 {
		buf[n] = byte('0' + i%10)
		i /= 10
		n++
	}
	// Reverse.
	for l, r := 0, n-1; l < r; l, r = l+1, r-1 {
		buf[l], buf[r] = buf[r], buf[l]
	}
	return string(buf[:n])
}

// `--layout ""` should fall back to "triple" (the new default —
// triple suits the most common shell-driven workflow without being
// so opinionated that single-shell users feel punished).
func TestResolveTemplate_EmptyNameDefaultsToTriple(t *testing.T) {
	r, err := ResolveTemplate("", "")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if r.Layout.Name != "triple" {
		t.Fatalf("got %q, want triple", r.Layout.Name)
	}
}

func TestResolveTemplate_UserShadowsBuiltin(t *testing.T) {
	dir := t.TempDir()
	custom := []byte(`name: 1x1x1
tabs:
  - name: user-override
    split:
      cwd: .
`)
	if err := os.WriteFile(filepath.Join(dir, "1x1x1.yaml"), custom, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	r, err := ResolveTemplate(dir, "1x1x1")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if r.Source != SourceUser {
		t.Fatalf("Source = %s, want user", r.Source)
	}
	if r.Layout.Tabs[0].Name != "user-override" {
		t.Fatalf("user template not used: %+v", r.Layout)
	}
}

func TestResolveTemplate_UserOnly(t *testing.T) {
	dir := t.TempDir()
	custom := []byte(`name: mine
tabs:
  - name: x
    split:
      cwd: .
`)
	if err := os.WriteFile(filepath.Join(dir, "mine.yaml"), custom, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	r, err := ResolveTemplate(dir, "mine")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if r.Source != SourceUser || r.Layout.Name != "mine" {
		t.Fatalf("unexpected: %+v", r)
	}
}

func TestResolveTemplate_NotFound(t *testing.T) {
	_, err := ResolveTemplate(t.TempDir(), "no-such-template")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestResolveTemplate_RejectsTraversal(t *testing.T) {
	for _, name := range []string{"../etc/passwd", "/abs", "..", ".", "a/b"} {
		if _, err := ResolveTemplate(t.TempDir(), name); err == nil {
			t.Fatalf("expected error for %q", name)
		}
	}
}

func TestResolveTemplate_InvalidUserTemplate(t *testing.T) {
	dir := t.TempDir()
	// No tabs → fails Validate.
	bad := []byte("name: broken\n")
	if err := os.WriteFile(filepath.Join(dir, "broken.yaml"), bad, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, err := ResolveTemplate(dir, "broken")
	if err == nil {
		t.Fatal("expected error for invalid user template")
	}
}

func TestListTemplates_BuiltinsOnly(t *testing.T) {
	names, err := ListTemplates("")
	if err != nil {
		t.Fatalf("ListTemplates: %v", err)
	}
	if got := len(names); got != len(builtinNames) {
		t.Errorf("got %d built-ins, want %d (%v vs %v)", got, len(builtinNames), names, builtinNames)
	}
	want := map[string]bool{}
	for _, n := range builtinNames {
		want[n] = false
	}
	for _, n := range names {
		want[n] = true
	}
	for n, found := range want {
		if !found {
			t.Errorf("expected built-in %q to be listed, got %v", n, names)
		}
	}
}

func TestListTemplates_UnionAndDedup(t *testing.T) {
	dir := t.TempDir()
	// Shadow a built-in (1x1x1) and add a new one (mine). The
	// listing must contain each name exactly once.
	for _, n := range []string{"1x1x1.yaml", "mine.yaml"} {
		if err := os.WriteFile(filepath.Join(dir, n),
			[]byte("name: x\ntabs:\n  - split:\n      cwd: .\n"),
			0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	names, err := ListTemplates(dir)
	if err != nil {
		t.Fatalf("ListTemplates: %v", err)
	}
	count := map[string]int{}
	for _, n := range names {
		count[n]++
	}
	if count["1x1x1"] != 1 {
		t.Errorf("1x1x1 appeared %d times, want 1", count["1x1x1"])
	}
	if count["mine"] != 1 {
		t.Errorf("mine missing or duplicated: %v", names)
	}
}

// Description-extraction tests pin down the leading-comment-block
// contract used by `boo layouts` and the new-project preview. YAML uses
// `#` for comments same as TOML — the parser is format-agnostic.

func TestExtractDescription_LeadingBlock(t *testing.T) {
	in := []byte("# First line.\n# Second line.\n\nname: x\n")
	got := extractDescription(in)
	want := "First line.\nSecond line."
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestExtractDescription_NoLeadingComment(t *testing.T) {
	in := []byte("name: x\n# This comment doesn't count, it's after the name.\n")
	if got := extractDescription(in); got != "" {
		t.Errorf("got %q, want empty (comment is not at top)", got)
	}
}

func TestExtractDescription_BlankCommentLineEndsBlock(t *testing.T) {
	in := []byte("# Para 1.\n#\n# Para 2.\n\nname: x\n")
	want := "Para 1.\n\nPara 2."
	if got := extractDescription(in); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestExtractDescription_StripsSingleLeadingSpace(t *testing.T) {
	in := []byte("# spaced\n#tight\n#   indented\n\nname: x\n")
	want := "spaced\ntight\n  indented"
	if got := extractDescription(in); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
