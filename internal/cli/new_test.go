package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/erzz/boo/internal/config"
	"github.com/erzz/boo/internal/layout"
	"github.com/erzz/boo/internal/picker"
	"github.com/erzz/boo/internal/project"
)

func TestRepoNameFromRemoteURL(t *testing.T) {
	cases := map[string]string{
		"https://github.com/owner/repo":         "repo",
		"https://github.com/owner/repo.git":     "repo",
		"https://github.com/owner/repo.git/":    "repo",
		"git@github.com:owner/repo.git":         "repo",
		"git@github.com:owner/repo":             "repo",
		"":                                      "",
		"   ":                                   "",
		"not-a-url":                             "",
		"https://example.com/some-org/My.Repo/": "My.Repo",
	}
	for in, want := range cases {
		got := repoNameFromRemoteURL(in)
		if got != want {
			t.Errorf("repoNameFromRemoteURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestValidateIntent(t *testing.T) {
	cases := []struct {
		name    string
		intent  *picker.NewProjectIntent
		wantErr bool
	}{
		{
			name:    "nil",
			intent:  nil,
			wantErr: true,
		},
		{
			name:    "empty name",
			intent:  &picker.NewProjectIntent{Dir: "/x"},
			wantErr: true,
		},
		{
			name:    "neither dir nor from",
			intent:  &picker.NewProjectIntent{Name: "p"},
			wantErr: true,
		},
		{
			name:    "dir only ok",
			intent:  &picker.NewProjectIntent{Name: "p", Dir: "/x"},
			wantErr: false,
		},
		{
			name:    "from only ok",
			intent:  &picker.NewProjectIntent{Name: "p", From: "https://x/y"},
			wantErr: false,
		},
		{
			name:    "whitespace name treated as empty",
			intent:  &picker.NewProjectIntent{Name: "  ", Dir: "/x"},
			wantErr: true,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateIntent(c.intent)
			if (err != nil) != c.wantErr {
				t.Fatalf("got err=%v, wantErr=%v", err, c.wantErr)
			}
		})
	}
}

func TestDefaultsToIntent_PreservesFields(t *testing.T) {
	d := picker.FormDefaults{
		Name:                "alpha",
		Dir:                 "/some/path",
		From:                "https://x",
		Template:            "dev",
		GitRemote:           "origin → ...",    // info-only; not propagated to intent
		AlreadyRegisteredAs: "should-not-leak", // ditto
	}
	got := defaultsToIntent(d)
	if got.Name != "alpha" || got.Dir != "/some/path" || got.From != "https://x" || got.Template != "dev" {
		t.Fatalf("intent fields wrong: %+v", got)
	}
}

func TestFirstArg(t *testing.T) {
	if got := firstArg(nil); got != "" {
		t.Errorf("nil → %q, want empty", got)
	}
	if got := firstArg([]string{}); got != "" {
		t.Errorf("[] → %q, want empty", got)
	}
	if got := firstArg([]string{"only"}); got != "only" {
		t.Errorf("[only] → %q, want 'only'", got)
	}
	if got := firstArg([]string{"first", "second"}); got != "first" {
		t.Errorf("[first second] → %q, want 'first'", got)
	}
}

func TestDefaultsToIntent_AppliesDefaultLayoutWhenTemplateBlank(t *testing.T) {
	// --yes with no --layout: config's default_layout must be carried into the intent.
	d := picker.FormDefaults{
		Name:          "alpha",
		Dir:           "/x",
		Template:      "",
		DefaultLayout: "1x2x1",
	}
	got := defaultsToIntent(d)
	if got.Template != "1x2x1" {
		t.Errorf("Template = %q, want '1x2x1' (config default should win when --layout omitted)", got.Template)
	}
}

func TestDefaultsToIntent_ExplicitTemplateBeatsDefault(t *testing.T) {
	// Explicit --layout must always beat the configured default.
	d := picker.FormDefaults{
		Name:          "alpha",
		Dir:           "/x",
		Template:      "2x2x2",
		DefaultLayout: "1x2x1",
	}
	got := defaultsToIntent(d)
	if got.Template != "2x2x2" {
		t.Errorf("Template = %q, want '2x2x2' (explicit --layout must beat config default)", got.Template)
	}
}

func TestExpandRepoShorthand(t *testing.T) {
	cases := []struct {
		name          string
		from          string
		defaultRemote string
		want          string
	}{
		{"empty from is unchanged", "", "https://github.com/erzz", ""},
		{"empty remote leaves bare name unchanged", "myrepo", "", "myrepo"},
		{"bare name expands", "myrepo", "https://github.com/erzz", "https://github.com/erzz/myrepo"},
		{"trailing slash on remote is stripped", "myrepo", "https://github.com/erzz/", "https://github.com/erzz/myrepo"},
		{"full https URL is unchanged", "https://github.com/owner/repo", "https://github.com/erzz", "https://github.com/owner/repo"},
		{"SSH-style URL is unchanged", "git@github.com:owner/repo.git", "https://github.com/erzz", "git@github.com:owner/repo.git"},
		{"path-containing input is unchanged (already qualified)", "owner/repo", "https://github.com/erzz", "owner/repo"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := expandRepoShorthand(c.from, c.defaultRemote); got != c.want {
				t.Errorf("expandRepoShorthand(%q, %q) = %q, want %q", c.from, c.defaultRemote, got, c.want)
			}
		})
	}
}

// TestBuildNewProjectDefaults_CloneFlow_DerivesDirFromURL: --from with no --dir/--into
// must derive Dir from the URL (not use cwd).
func TestBuildNewProjectDefaults_CloneFlow_DerivesDirFromURL(t *testing.T) {
	a := makeAppForCmds(t)
	a.Config = config.DefaultConfig()

	defs, err := buildNewProjectDefaults(a, defaultsFromFlags{
		from: "https://github.com/owner/myrepo.git",
	})
	if err != nil {
		t.Fatalf("buildNewProjectDefaults: %v", err)
	}

	// Dir should be derived from the URL — basename must be the repo name.
	if filepath.Base(defs.Dir) != "myrepo" {
		t.Errorf("Dir = %q, want basename 'myrepo' (derived from URL)", defs.Dir)
	}

	// Dir must NOT equal cwd (that was the original bug).
	cwd, _ := os.Getwd()
	if defs.Dir == cwd {
		t.Error("Dir must not be cwd for clone flows — cloning into cwd itself is wrong")
	}

	// From should be passed through unchanged.
	if defs.From != "https://github.com/owner/myrepo.git" {
		t.Errorf("From = %q, want the original URL", defs.From)
	}
}

// TestBuildNewProjectDefaults_CloneFlow_ExplicitIntoWins: explicit --into overrides URL-derived Dir.
func TestBuildNewProjectDefaults_CloneFlow_ExplicitIntoWins(t *testing.T) {
	a := makeAppForCmds(t)
	a.Config = config.DefaultConfig()

	target := t.TempDir()
	defs, err := buildNewProjectDefaults(a, defaultsFromFlags{
		from: "https://github.com/owner/myrepo.git",
		into: target,
	})
	if err != nil {
		t.Fatalf("buildNewProjectDefaults: %v", err)
	}

	// Explicit --into must win.
	if defs.Dir != target {
		t.Errorf("Dir = %q, want %q (explicit --into must override URL derivation)", defs.Dir, target)
	}
}

// TestBuildNewProjectDefaults_CloneFlow_ExplicitDirWins: explicit --dir overrides URL-derived Dir.
func TestBuildNewProjectDefaults_CloneFlow_ExplicitDirWins(t *testing.T) {
	a := makeAppForCmds(t)
	a.Config = config.DefaultConfig()

	target := t.TempDir()
	defs, err := buildNewProjectDefaults(a, defaultsFromFlags{
		from: "https://github.com/owner/myrepo.git",
		dir:  target,
	})
	if err != nil {
		t.Fatalf("buildNewProjectDefaults: %v", err)
	}

	if defs.Dir != target {
		t.Errorf("Dir = %q, want %q (explicit --dir must override URL derivation)", defs.Dir, target)
	}
}

// TestBuildNewProjectDefaults_NonCloneFlow_UsesCwd: no --from → Dir defaults to cwd.
func TestBuildNewProjectDefaults_NonCloneFlow_UsesCwd(t *testing.T) {
	a := makeAppForCmds(t)
	a.Config = config.DefaultConfig()

	defs, err := buildNewProjectDefaults(a, defaultsFromFlags{})
	if err != nil {
		t.Fatalf("buildNewProjectDefaults: %v", err)
	}

	cwd, _ := os.Getwd()
	if defs.Dir != cwd {
		t.Errorf("Dir = %q, want cwd %q for non-clone flow", defs.Dir, cwd)
	}
}

// TestRunCreateProject_RegistryLayoutKeyIsTemplateNotLayoutName: regression
// for the M3 invariant. The registry's `Project.Layout` field must store the
// template lookup key submitted by the user — NOT the resolved layout's
// internal `name:` (which can differ). If we wrote `l.Name`, a template file
// whose internal name diverges from its filename key would yield a
// non-resolvable `Project.Layout`, breaking `loadOrRegenerateLayout`.
func TestRunCreateProject_RegistryLayoutKeyIsTemplateNotLayoutName(t *testing.T) {
	a := makeAppForCmds(t)
	a.Config = config.DefaultConfig()
	// User template keyed as "mytemplate.yaml" but with a different internal name.
	tmplPath := filepath.Join(a.Paths.LayoutsDir, "mytemplate.yaml")
	body := []byte("name: divergent-internal-name\ntabs:\n  - split:\n      cwd: \"\"\n")
	if err := os.WriteFile(tmplPath, body, 0o600); err != nil {
		t.Fatalf("write template: %v", err)
	}

	intent := picker.NewProjectIntent{
		Name:     "regproj",
		Dir:      t.TempDir(),
		Template: "mytemplate",
	}
	var out bytes.Buffer
	// switchToProject is best-effort with the all-nil fake; ignore its error.
	// The registry write happens before the switch attempt, so the row exists either way.
	_ = runCreateProject(context.Background(), a, intent, &out)

	reg, err := project.Load(a.Paths)
	if err != nil {
		t.Fatalf("Load registry: %v", err)
	}
	p, err := reg.Get("regproj")
	if err != nil {
		t.Fatalf("Get regproj: %v", err)
	}
	if p.Layout != "mytemplate" {
		t.Errorf("Project.Layout = %q, want %q (must be the lookup key, not the layout struct's internal name)", p.Layout, "mytemplate")
	}
}

// TestRunCreateProject_MaterialisedLayoutPersistsAndKeepsTemplateKey: when the
// editor produced a customised tree, it must be written to the project's own
// layout.yaml AND `Project.Layout` must remain the original template key.
func TestRunCreateProject_MaterialisedLayoutPersistsAndKeepsTemplateKey(t *testing.T) {
	a := makeAppForCmds(t)
	a.Config = config.DefaultConfig()

	// Resolve "triple" then mutate one leaf's Command — simulating the editor's apply path.
	resolved, err := layout.ResolveTemplate(a.Paths.LayoutsDir, "triple")
	if err != nil {
		t.Fatalf("resolve triple: %v", err)
	}
	mat := resolved.Layout
	leaves := layout.LeafPointers(&mat.Tabs[0].Root)
	if len(leaves) == 0 {
		t.Fatalf("triple has no leaves")
	}
	leaves[0].Command = "echo customised"

	intent := picker.NewProjectIntent{
		Name:               "matproj",
		Dir:                t.TempDir(),
		Template:           "triple",
		MaterialisedLayout: &mat,
	}
	var out bytes.Buffer
	_ = runCreateProject(context.Background(), a, intent, &out)

	// 1. Registry: Layout stays the template key.
	reg, err := project.Load(a.Paths)
	if err != nil {
		t.Fatalf("Load registry: %v", err)
	}
	p, err := reg.Get("matproj")
	if err != nil {
		t.Fatalf("Get matproj: %v", err)
	}
	if p.Layout != "triple" {
		t.Errorf("Project.Layout = %q, want %q (template key must survive materialisation)", p.Layout, "triple")
	}

	// 2. On-disk layout.yaml carries the customisation.
	got, err := project.LoadLayout(a.Paths, "matproj")
	if err != nil {
		t.Fatalf("LoadLayout: %v", err)
	}
	gotLeaves := layout.LeafPointers(&got.Tabs[0].Root)
	if len(gotLeaves) == 0 || gotLeaves[0].Command != "echo customised" {
		t.Errorf("persisted layout did not retain custom command; first leaf = %+v", gotLeaves[0])
	}
}
