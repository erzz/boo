package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	booexec "github.com/erzz/boo/internal/exec"
	"github.com/erzz/boo/internal/ghostty"
	"github.com/erzz/boo/internal/layout"
	"github.com/erzz/boo/internal/layoutpreview"
	"github.com/erzz/boo/internal/project"
	"github.com/erzz/boo/internal/state"
)

// makeAppForCmds builds a minimal app suitable for testing commands
// that touch the registry, layout files, and runtime state but do
// NOT need a live Ghostty.
//
// Ghostty is wired to a fake exec.Runner that returns "no terminal"
// for any osascript invocation — the only Ghostty method these tests
// would hit is WindowExists, which short-circuits to "false" when no
// project has ever been launched (no runtime state file).
func makeAppForCmds(t *testing.T) *app {
	t.Helper()
	dir := t.TempDir()
	p := state.ForRoot(dir)
	if err := p.EnsureDirs(); err != nil {
		t.Fatalf("EnsureDirs: %v", err)
	}
	fake := booexec.NewFake(func(_ string, _ []string, _ []byte) ([]byte, []byte, error) {
		// Default: pretend nothing is running. WindowExists in
		// particular only ever sees this when a runtime state
		// file claims a window — which our fixtures never set up.
		return nil, nil, nil
	})
	return &app{
		Paths:   p,
		Ghostty: ghostty.New(fake),
	}
}

// registerProjectForTest writes a project entry + a layout file
// directly, bypassing runCreateProject (which would auto-launch
// Ghostty). Returns the project name.
func registerProjectForTest(t *testing.T, a *app, name, dir, templateName string) {
	t.Helper()
	resolved, err := layout.ResolveTemplate(a.Paths.LayoutsDir, templateName)
	if err != nil {
		t.Fatalf("resolve template %q: %v", templateName, err)
	}
	l := resolved.Layout
	if l.Name == "" {
		l.Name = templateName
	}
	if err := project.SaveLayout(a.Paths, name, l); err != nil {
		t.Fatalf("SaveLayout: %v", err)
	}
	reg, err := project.Load(a.Paths)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := reg.Add(project.Project{
		Name:      name,
		Dir:       dir,
		Layout:    l.Name,
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := reg.Save(a.Paths); err != nil {
		t.Fatalf("Save: %v", err)
	}
}

// G1: set-layout switches a project's layout and reflects it both in
// the registry's display field and in the on-disk layout file.
func TestSetLayout_RewritesLayoutFileAndUpdatesRegistry(t *testing.T) {
	a := makeAppForCmds(t)
	dir := t.TempDir()
	registerProjectForTest(t, a, "proj", dir, "1x1x1")

	// Inline the set-layout flow. Ideally we'd call the cobra command;
	// but newApp() reads BOO_HOME, which conflicts with t.TempDir-based
	// isolation we already have via state.ForRoot. The flow is small
	// enough that re-running it in the test still meaningfully exercises
	// the contract (registry.Update + project.SaveLayout in lockstep).
	resolved, err := layout.ResolveTemplate(a.Paths.LayoutsDir, "triple")
	if err != nil {
		t.Fatalf("resolve triple: %v", err)
	}
	l := resolved.Layout
	if l.Name == "" {
		l.Name = "triple"
	}
	if err := a.Paths.WithLock(func() error {
		reg, err := project.Load(a.Paths)
		if err != nil {
			return err
		}
		p, err := reg.Get("proj")
		if err != nil {
			return err
		}
		if err := project.SaveLayout(a.Paths, "proj", l); err != nil {
			return err
		}
		p.Layout = l.Name
		if err := reg.Update(p); err != nil {
			return err
		}
		return reg.Save(a.Paths)
	}); err != nil {
		t.Fatalf("set-layout flow: %v", err)
	}

	reg, err := project.Load(a.Paths)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	p, err := reg.Get("proj")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if p.Layout != "triple" {
		t.Errorf("registry Layout = %q, want %q", p.Layout, "triple")
	}
	got, err := project.LoadLayout(a.Paths, "proj")
	if err != nil {
		t.Fatalf("LoadLayout: %v", err)
	}
	if got.Name != "triple" {
		t.Errorf("layout.Name = %q, want %q", got.Name, "triple")
	}
}

// Registry.Update returns ErrNotFound for an unknown project. The
// set-layout command relies on this to fail fast rather than silently
// adding a new project entry.
func TestRegistryUpdate_UnknownProjectIsErrNotFound(t *testing.T) {
	a := makeAppForCmds(t)
	reg, err := project.Load(a.Paths)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	err = reg.Update(project.Project{Name: "nope", Dir: "/tmp"})
	if err == nil {
		t.Fatal("Update on unknown project: want error, got nil")
	}
}

// loadOrRegenerateLayout regenerates the snapshot from the registry's
// template name when the on-disk file is missing. This handles old
// pre-YAML installs whose stale .toml snapshots got cleaned up, and
// anyone who blew away ~/.local/share/boo by accident.
func TestLoadOrRegenerateLayout_RecreatesMissingSnapshot(t *testing.T) {
	a := makeAppForCmds(t)
	dir := t.TempDir()
	registerProjectForTest(t, a, "proj", dir, "triple")

	// Remove the snapshot the helper just wrote — simulate the
	// migration scenario where layout.toml got deleted and no
	// layout.yaml ever existed.
	snapPath := a.Paths.ProjectLayoutFile("proj")
	if err := os.Remove(snapPath); err != nil {
		t.Fatalf("remove snapshot: %v", err)
	}

	reg, err := project.Load(a.Paths)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	p, err := reg.Get("proj")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	l, err := loadOrRegenerateLayout(a, p)
	if err != nil {
		t.Fatalf("loadOrRegenerateLayout: %v", err)
	}
	if l.Name != "triple" {
		t.Errorf("regenerated Layout.Name = %q, want %q", l.Name, "triple")
	}
	// Snapshot should be on disk again so subsequent calls go through
	// the fast path.
	if _, err := os.Stat(snapPath); err != nil {
		t.Errorf("snapshot not rewritten: %v", err)
	}
}

// Regeneration only kicks in for "file does not exist" — a present-but-
// corrupt snapshot must surface as a hard error so users notice (vs.
// silently overwriting their hand-edited layout).
func TestLoadOrRegenerateLayout_CorruptSnapshotIsHardError(t *testing.T) {
	a := makeAppForCmds(t)
	dir := t.TempDir()
	registerProjectForTest(t, a, "proj", dir, "triple")

	// Overwrite the snapshot with garbage.
	snapPath := a.Paths.ProjectLayoutFile("proj")
	if err := os.WriteFile(snapPath, []byte("not: valid: yaml: at: all"), 0o644); err != nil {
		t.Fatalf("write garbage: %v", err)
	}

	reg, err := project.Load(a.Paths)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	p, err := reg.Get("proj")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	if _, err := loadOrRegenerateLayout(a, p); err == nil {
		t.Fatal("expected error for corrupt snapshot, got nil")
	}
}

// G7: show — verify the layout file path the command points the user
// at actually exists for a registered project.
func TestShow_LayoutFilePathExistsForRegisteredProject(t *testing.T) {
	a := makeAppForCmds(t)
	dir := t.TempDir()
	registerProjectForTest(t, a, "proj", dir, "triple")

	got := a.Paths.ProjectLayoutFile("proj")
	if _, err := os.Stat(got); err != nil {
		t.Errorf("ProjectLayoutFile %s should exist after registration: %v", got, err)
	}
}

// G5: list --json on an empty registry must emit `[]`, not `null`.
// JSON consumers that use jq (or strict-typed languages) treat the two
// very differently.
func TestRenderListJSON_EmptyRegistryEmitsArray(t *testing.T) {
	a := makeAppForCmds(t)
	var buf bytes.Buffer
	if err := renderListJSON(context.Background(), &buf, a, nil); err != nil {
		t.Fatalf("renderListJSON: %v", err)
	}
	got := strings.TrimSpace(buf.String())
	if got != "[]" {
		t.Errorf("empty registry JSON = %q, want %q", got, "[]")
	}
}

// G5: list --json populated case — round-trip through json.Unmarshal
// to verify the schema matches what we documented.
func TestRenderListJSON_PopulatedRegistryHasExpectedFields(t *testing.T) {
	a := makeAppForCmds(t)
	dir := t.TempDir()
	registerProjectForTest(t, a, "proj", dir, "triple")
	reg, err := project.Load(a.Paths)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	var buf bytes.Buffer
	if err := renderListJSON(context.Background(), &buf, a, reg.Projects); err != nil {
		t.Fatalf("renderListJSON: %v", err)
	}
	var got []listJSONEntry
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, buf.String())
	}
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	e := got[0]
	if e.Name != "proj" || e.Dir != dir || e.Layout != "triple" || e.Status != "stopped" {
		t.Errorf("entry = %+v", e)
	}
}

// G5: layouts --json must list every built-in with source set to
// "builtin" or "user" (not the empty string, not the internal
// SourceUser/SourceBuiltin constants).
func TestRunLayoutsJSON_ListsBuiltinsWithSourceTag(t *testing.T) {
	a := makeAppForCmds(t)
	var buf bytes.Buffer
	if err := runLayoutsJSON(a, &buf); err != nil {
		t.Fatalf("runLayoutsJSON: %v", err)
	}
	var got []layoutsJSONEntry
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, buf.String())
	}
	if len(got) < 5 {
		t.Errorf("len = %d, want at least 5 built-ins", len(got))
	}
	seen := map[string]bool{}
	for _, e := range got {
		seen[e.Name] = true
		if e.Source != "builtin" && e.Source != "user" {
			t.Errorf("entry %s has unexpected source %q", e.Name, e.Source)
		}
	}
	for _, want := range []string{"1x1x1", "triple"} {
		if !seen[want] {
			t.Errorf("missing built-in %q in JSON output", want)
		}
	}
}

// G6: shell completion — projectNamesForCompletion must be silent on
// errors, which means: missing BOO_HOME root → nil, not panic.
func TestProjectNamesForCompletion_SilentOnNoState(t *testing.T) {
	t.Setenv("BOO_HOME", t.TempDir())
	got := projectNamesForCompletion()
	if len(got) != 0 {
		t.Errorf("want empty for fresh state, got %v", got)
	}
}

// G6: completion picks up registered project names. We use BOO_HOME to
// point the helper at our temp state.
func TestProjectNamesForCompletion_ReturnsRegisteredNames(t *testing.T) {
	root := t.TempDir()
	t.Setenv("BOO_HOME", root)
	p := state.ForRoot(root)
	if err := p.EnsureDirs(); err != nil {
		t.Fatalf("EnsureDirs: %v", err)
	}
	reg := &project.Registry{}
	if err := reg.Add(project.Project{Name: "alpha", Dir: "/tmp/alpha"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := reg.Add(project.Project{Name: "beta", Dir: "/tmp/beta"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := reg.Save(p); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got := projectNamesForCompletion()
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2: %v", len(got), got)
	}
	have := map[string]bool{got[0]: true, got[1]: true}
	if !have["alpha"] || !have["beta"] {
		t.Errorf("missing expected names in %v", got)
	}
}

// G6: completion templates resolves built-ins via state.ForRoot's
// LayoutsDir even when no user templates are present.
func TestTemplateNamesForCompletion_ReturnsBuiltins(t *testing.T) {
	t.Setenv("BOO_HOME", t.TempDir())
	got := templateNamesForCompletion()
	if len(got) < 5 {
		t.Errorf("want at least 5 built-ins, got %d: %v", len(got), got)
	}
}

// G6: a malformed config file MUST NOT break shell completion. The
// whole point of the helper bypassing newApp() is so a user with a
// busted YAML file can still TAB-complete project names while they
// fix it.
func TestProjectNamesForCompletion_StillWorksWithMalformedConfig(t *testing.T) {
	root := t.TempDir()
	t.Setenv("BOO_HOME", root)
	p := state.ForRoot(root)
	if err := p.EnsureDirs(); err != nil {
		t.Fatalf("EnsureDirs: %v", err)
	}
	// Write a deliberately broken config file.
	if err := os.WriteFile(p.ConfigFile, []byte("default_layout: [unclosed\n"), 0o644); err != nil {
		t.Fatalf("write broken config: %v", err)
	}
	// Register a project so we have something to complete.
	reg := &project.Registry{}
	if err := reg.Add(project.Project{Name: "alpha", Dir: "/tmp/alpha"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := reg.Save(p); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got := projectNamesForCompletion()
	if len(got) != 1 || got[0] != "alpha" {
		t.Errorf("want [alpha] despite broken config, got %v", got)
	}
}

// G5 black-box: list --json schema verified WITHOUT round-tripping
// through the same internal struct (which would mask tag/shape
// regressions). Decode into map[string]any and assert the wire-level
// keys/types directly.
func TestListJSON_BlackBoxSchema(t *testing.T) {
	a := makeAppForCmds(t)
	dir := t.TempDir()
	registerProjectForTest(t, a, "proj", dir, "triple")
	reg, err := project.Load(a.Paths)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	var buf bytes.Buffer
	if err := renderListJSON(context.Background(), &buf, a, reg.Projects); err != nil {
		t.Fatalf("renderListJSON: %v", err)
	}
	var raw []map[string]any
	if err := json.Unmarshal(buf.Bytes(), &raw); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, buf.String())
	}
	if len(raw) != 1 {
		t.Fatalf("len = %d, want 1", len(raw))
	}
	want := []string{"name", "dir", "layout", "status"}
	for _, k := range want {
		if _, ok := raw[0][k]; !ok {
			t.Errorf("list --json entry missing required key %q (have keys: %v)", k, mapKeys(raw[0]))
		}
	}
	if raw[0]["name"] != "proj" {
		t.Errorf("name = %v, want \"proj\"", raw[0]["name"])
	}
}

// G5 black-box: layouts --json schema.
func TestLayoutsJSON_BlackBoxSchema(t *testing.T) {
	a := makeAppForCmds(t)
	var buf bytes.Buffer
	if err := runLayoutsJSON(a, &buf); err != nil {
		t.Fatalf("runLayoutsJSON: %v", err)
	}
	var raw []map[string]any
	if err := json.Unmarshal(buf.Bytes(), &raw); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, buf.String())
	}
	if len(raw) < 5 {
		t.Fatalf("len = %d, want at least 5", len(raw))
	}
	for _, e := range raw {
		if _, ok := e["name"]; !ok {
			t.Errorf("entry missing 'name': %v", e)
		}
		if _, ok := e["source"]; !ok {
			t.Errorf("entry missing 'source': %v", e)
		}
		if s, _ := e["source"].(string); s != "builtin" && s != "user" {
			t.Errorf("entry source = %v, want builtin|user", e["source"])
		}
	}
}

// G5: a broken user template MUST appear in --json output with a
// non-empty `error` field, NOT silently disappear.
func TestLayoutsJSON_BrokenUserTemplateHasErrorField(t *testing.T) {
	a := makeAppForCmds(t)
	if err := os.WriteFile(filepath.Join(a.Paths.LayoutsDir, "broken.yaml"),
		[]byte("name: broken\ntabs: [ {root: }\n"), 0o644); err != nil {
		t.Fatalf("write broken: %v", err)
	}
	var buf bytes.Buffer
	if err := runLayoutsJSON(a, &buf); err != nil {
		t.Fatalf("runLayoutsJSON: %v", err)
	}
	var raw []map[string]any
	if err := json.Unmarshal(buf.Bytes(), &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	var brokenEntry map[string]any
	for _, e := range raw {
		if e["name"] == "broken" {
			brokenEntry = e
			break
		}
	}
	if brokenEntry == nil {
		t.Fatal("broken template not present in JSON output (was silently dropped?)")
	}
	if errStr, _ := brokenEntry["error"].(string); errStr == "" {
		t.Errorf("broken template entry missing error field: %v", brokenEntry)
	}
}

// G5: config show --json schema. Verifies the dotted-key flattening
// and the {value, source} pair shape.
func TestConfigShowJSON_BlackBoxSchema(t *testing.T) {
	t.Setenv("BOO_HOME", t.TempDir())
	var buf bytes.Buffer
	if err := runConfigShowJSON(&buf); err != nil {
		t.Fatalf("runConfigShowJSON: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(buf.Bytes(), &raw); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, buf.String())
	}
	if _, ok := raw["config_file"]; !ok {
		t.Error("missing top-level 'config_file' key")
	}
	values, ok := raw["values"].(map[string]any)
	if !ok {
		t.Fatalf("values is not an object: %T", raw["values"])
	}
	for _, k := range []string{"default_layout", "projects_dir", "git.default_remote", "ui.theme"} {
		field, ok := values[k].(map[string]any)
		if !ok {
			t.Errorf("values[%q] missing or wrong type: %v", k, values[k])
			continue
		}
		if _, ok := field["value"]; !ok {
			t.Errorf("values[%q] missing 'value'", k)
		}
		if _, ok := field["source"]; !ok {
			t.Errorf("values[%q] missing 'source'", k)
		}
	}
	// default_layout's factory default is "triple"; this is the
	// canonical user-visible default and changing it is a UX
	// commitment.
	dl := values["default_layout"].(map[string]any)
	if dl["value"] != "triple" {
		t.Errorf("factory default_layout = %v, want \"triple\"", dl["value"])
	}
	if dl["source"] != "factory" {
		t.Errorf("factory default_layout source = %v, want \"factory\"", dl["source"])
	}
}

func mapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// TestProjectPreviewer_UsesSavedLayoutOverTemplate verifies that the right-pane
// project preview renders the on-disk saved layout snapshot rather than
// resolving the template name from the registry.  This ensures that manual
// edits via `boo edit` / `boo save` are reflected in the picker immediately.
func TestProjectPreviewer_UsesSavedLayoutOverTemplate(t *testing.T) {
	a := makeAppForCmds(t)
	dir := t.TempDir()

	// Register with template "triple" (3-pane layout).
	registerProjectForTest(t, a, "proj", dir, "triple")

	// Overwrite the saved snapshot with a single-pane layout — deliberately
	// different from the "triple" template so we can tell which was rendered.
	singlePane := layout.Layout{
		Name: "single",
		Tabs: []layout.Tab{{
			Name: "main",
			Root: layout.Split{Cwd: "."},
		}},
	}
	if err := project.SaveLayout(a.Paths, "proj", singlePane); err != nil {
		t.Fatalf("SaveLayout: %v", err)
	}

	previewer := projectPreviewer(context.Background(), a, pickerTheme(a))
	got := previewer("proj")

	if got == "" {
		t.Fatal("projectPreviewer returned empty string; expected project info")
	}

	// Render both layouts at the width the previewer uses.
	const previewWidth = 36
	savedRendered := layoutpreview.RenderLayout(singlePane, previewWidth)

	resolved, err := layout.ResolveTemplate(a.Paths.LayoutsDir, "triple")
	if err != nil {
		t.Fatalf("resolve triple: %v", err)
	}
	templateRendered := layoutpreview.RenderLayout(resolved.Layout, previewWidth)

	if !strings.Contains(got, savedRendered) {
		t.Errorf("preview does not contain saved layout rendering.\nPreview:\n%s\nExpected substring:\n%s", got, savedRendered)
	}
	// The template rendering must NOT appear (assuming they differ, which they
	// do: single pane vs. 3-pane triple layout).
	if savedRendered != templateRendered && strings.Contains(got, templateRendered) {
		t.Errorf("preview should show saved layout (single pane), not the template (triple).\nGot:\n%s", got)
	}
}
