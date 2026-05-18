package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/erzz/boo/internal/layout"
	"github.com/erzz/boo/internal/picker"
	"github.com/erzz/boo/internal/project"
)

func newNewCmd() *cobra.Command { return newNewCmdWithApp(nil) }

// newNewCmdWithApp is like newNewCmd but accepts a pre-built *app for testing.
// Pass nil for production behaviour.
func newNewCmdWithApp(appIn *app) *cobra.Command {
	var (
		fromURL    string
		intoDir    string
		existing   string
		layoutName string
		yes        bool
	)
	cmd := &cobra.Command{
		Use:   "new [name]",
		Short: "Register a new project",
		Long: `Register a project so 'boo <name>' can launch it.

With no arguments, opens an interactive form pre-populated from the current
directory (and any detected git remote). The same form is reachable from
the main 'boo' picker via the '+ New project' entry.

Non-interactive use (scripting):
  boo new projA --dir ~/code/projA --yes              # register existing dir
  boo new projA --from https://example/projA.git --yes # clone, dest derived from URL
  boo new projA --from <url> --dir ~/code/projA --yes  # clone into specific dir

When --from is given, --dir (or its alias --into) controls the clone destination;
otherwise --dir points at an existing directory to register.

--yes skips the form and registers immediately. With no flags it falls
back to the current directory (and any detected git remote), so
'boo new --yes' inside a git repo registers it as-is. Without --yes,
flags act as form pre-population and the user can edit before submitting.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			a := appIn
			if a == nil {
				var err error
				a, err = newApp()
				if err != nil {
					return err
				}
			}

			// Build form defaults from flags + cwd inspection. Flags always win.
			defs, err := buildNewProjectDefaults(a, defaultsFromFlags{
				name:     firstArg(args),
				dir:      existing,
				from:     fromURL,
				into:     intoDir,
				template: layoutName,
			})
			if err != nil {
				return err
			}

			// --yes is the non-interactive escape hatch. Error if anything required is missing.
			if yes {
				intent := defaultsToIntent(defs)
				if err := validateIntent(intent); err != nil {
					return fmt.Errorf("--yes was given but %w", err)
				}
				return runCreateProject(c.Context(), a, *intent, c.OutOrStdout())
			}

			// Interactive: open the form (skipping the project list).
			res, err := picker.Run(nil, picker.Options{
				Defaults:                 defs,
				SkipListGoStraightToForm: true,
				PreviewTemplate:          templatePreviewer(a),
				LayoutNames:              templateNames(a),
				ResolveLayout:            layoutResolver(a),
				Theme:                    a.Config.ThemeOr("default"),
				ThemesDir:                a.Paths.ThemesDir,
				ConfigPath:               a.Paths.ConfigFile,
			})
			if err != nil {
				return err
			}
			if res.Cancelled() {
				return nil
			}
			// User may have switched to an existing project from the AlreadyRegistered prompt.
			// In form-only mode that prompt is never shown, but handle it safely regardless.
			switch v := res.Intent.(type) {
			case picker.SwitchIntent:
				p, err := project.Load(a.Paths)
				if err != nil {
					return err
				}
				pr, err := p.Get(v.Name)
				if err != nil {
					return err
				}
				return switchToProject(c.Context(), a, pr)
			case picker.NewProjectIntent:
				return runCreateProject(c.Context(), a, v, c.OutOrStdout())
			default:
				return fmt.Errorf("picker: unexpected intent %T", v)
			}
		},
	}
	cmd.Flags().StringVar(&fromURL, "from", "", "git URL to clone from")
	cmd.Flags().StringVar(&intoDir, "into", "", "directory to clone into (with --from); defaults to repo name in cwd")
	cmd.Flags().StringVar(&existing, "dir", "", "existing directory to register")
	cmd.Flags().StringVar(&layoutName, "layout", "", "layout template name (uses configured default if omitted)")
	cmd.Flags().BoolVar(&yes, "yes", false, "skip the form and register immediately (uses cwd if --dir/--from omitted)")
	return cmd
}

// firstArg returns args[0] or "" if there are none. Saves a length check at
// every call site.
func firstArg(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return args[0]
}

// defaultsFromFlags is what the CLI flags contribute to the form defaults.
// Kept as a struct so the call to buildNewProjectDefaults stays readable.
type defaultsFromFlags struct {
	name, dir, from, into, template string
}

// buildNewProjectDefaults assembles form defaults for TUI pre-population and --yes mode.
// Resolution order per field: flags → cwd inspection (basename, git remote) → hardcoded defaults.
// AlreadyRegisteredAs is filled when the resolved Dir is already registered.
func buildNewProjectDefaults(a *app, fl defaultsFromFlags) (picker.FormDefaults, error) {
	cwd, _ := os.Getwd()

	// Directory resolution: explicit --dir wins; then --into (for clones); then cwd or URL-derived.
	dir := strings.TrimSpace(fl.dir)
	if dir == "" {
		dir = strings.TrimSpace(fl.into)
	}
	if dir == "" && fl.from == "" {
		// Non-clone flow: default to cwd so "boo new" inside a git repo pre-fills the form.
		dir = cwd
	}
	if dir == "" && fl.from != "" {
		// Clone flow: derive destination from URL so the form shows the real target.
		// Best-effort: if resolution fails, leave dir empty; the form accepts manual input.
		expanded := expandRepoShorthand(fl.from, a.Config.GitDefaultRemoteOr(""))
		if derived, err := resolveCloneDestination("", expanded, a.Config.ProjectsDirOr("")); err == nil {
			dir = derived
		}
	}
	if dir != "" {
		if abs, err := filepath.Abs(dir); err == nil {
			dir = abs
		}
	}

	// Git remote: best-effort, absent is normal.
	var gitRemote, repoName string
	if dirExists(dir) {
		if remote, err := readGitRemote(dir); err == nil && remote != "" {
			gitRemote = "origin → " + remote
			repoName = repoNameFromRemoteURL(remote)
		}
	}

	// Name: explicit positional arg wins; then git-derived repo name; then dir basename; then URL.
	name := strings.TrimSpace(fl.name)
	if name == "" {
		if repoName != "" {
			name = repoName
		} else if dir != "" && filepath.Base(dir) != "." {
			name = filepath.Base(dir)
		} else if fl.from != "" {
			// Clone flow where dir derivation failed — try URL directly.
			name = repoNameFromRemoteURL(fl.from)
		}
	}

	// Already-registered detection: only when dir exists and not a clone flow.
	var alreadyAs string
	if dirExists(dir) && fl.from == "" {
		reg, err := project.Load(a.Paths)
		if err == nil {
			if p, err := reg.FindByDir(dir); err == nil {
				alreadyAs = p.Name
			}
		}
	}

	// Leave Template empty; the form supplies its own default ("triple").
	// Single source of truth lives in the form, not here.
	template := strings.TrimSpace(fl.template)

	return picker.FormDefaults{
		Name:                name,
		Dir:                 dir,
		From:                strings.TrimSpace(fl.from),
		Template:            template,
		GitRemote:           gitRemote,
		AlreadyRegisteredAs: alreadyAs,
		DefaultLayout:       a.Config.DefaultLayoutOr("triple"),
	}, nil
}

// defaultsToIntent converts fully-specified defaults into an intent for --yes mode.
// If Template is blank, applies DefaultLayout so --yes honours the user's configured default.
func defaultsToIntent(d picker.FormDefaults) *picker.NewProjectIntent {
	tpl := d.Template
	if tpl == "" {
		tpl = d.DefaultLayout
	}
	return &picker.NewProjectIntent{
		Name:     d.Name,
		Dir:      d.Dir,
		From:     d.From,
		Template: tpl,
	}
}

func validateIntent(i *picker.NewProjectIntent) error {
	if i == nil {
		return errors.New("no intent")
	}
	if strings.TrimSpace(i.Name) == "" {
		return errors.New("name is required")
	}
	if strings.TrimSpace(i.Dir) == "" && strings.TrimSpace(i.From) == "" {
		return errors.New("either --dir or --from is required")
	}
	return nil
}

// runCreateProject performs the actual registration (and clone if From is set).
// Clones run outside the registry lock; the lock is taken only for the registry write.
func runCreateProject(ctx context.Context, a *app, intent picker.NewProjectIntent, out io.Writer) error {
	if err := project.ValidateName(intent.Name); err != nil {
		return err
	}
	// When both From and Dir are set, treat Dir as the clone destination.
	if intent.From == "" && intent.Dir == "" {
		return errors.New("either Directory or Clone from URL is required")
	}

	// Resolve layout up front. The picker's layout editor may have already
	// materialised the tree (form → editor → apply); when present, that
	// edited tree wins over re-resolving the bare template.
	//
	// The registry's `Project.Layout` field is a template lookup key that
	// `loadOrRegenerateLayout` will resolve later. It must remain a valid
	// template key (or empty → "default"); it is NOT the same as the
	// layout struct's display `Name`, which can come from the YAML file's
	// `name:` and may differ from its lookup key. We therefore derive
	// `templateKey` from the user's submitted intent, independently of
	// `l.Name`.
	var l layout.Layout
	if intent.MaterialisedLayout != nil {
		l = *intent.MaterialisedLayout
	} else {
		resolved, err := layout.ResolveTemplate(a.Paths.LayoutsDir, intent.Template)
		if err != nil {
			return err
		}
		l = resolved.Layout
	}
	if l.Name == "" {
		l.Name = intent.Template
	}
	if l.Name == "" {
		l.Name = "default"
	}
	templateKey := strings.TrimSpace(intent.Template)
	if templateKey == "" {
		templateKey = "default"
	}

	// Resolve directory.
	var (
		dir string
		err error
	)
	if intent.From != "" {
		// Apply git default-remote shorthand expansion (bare "boo" → "<remote>/boo").
		intent.From = expandRepoShorthand(intent.From, a.Config.GitDefaultRemoteOr(""))

		// If Dir was provided treat it as --into; otherwise derive from the URL.
		if intent.Dir != "" {
			abs, err := filepath.Abs(intent.Dir)
			if err != nil {
				return err
			}
			dir = abs
		} else {
			dir, err = resolveCloneDestination("", intent.From, a.Config.ProjectsDirOr(""))
			if err != nil {
				return err
			}
		}
		if err := preCheckCollisions(a, intent.Name, dir); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(out, "Cloning %s into %s ...\n", intent.From, dir)
		cloned, err := a.Git.Clone(ctx, intent.From, dir)
		if err != nil {
			return err
		}
		dir = cloned
	} else {
		dir, err = resolveDir(intent.Dir)
		if err != nil {
			return err
		}
	}

	return a.Paths.WithLock(func() error {
		reg, err := project.Load(a.Paths)
		if err != nil {
			return err
		}
		if reg.Has(intent.Name) {
			if intent.From != "" {
				return fmt.Errorf("project %q already registered (the clone at %s was kept; remove it manually if unwanted)", intent.Name, dir)
			}
			return fmt.Errorf("project %q already registered (use 'boo delete %s' first)", intent.Name, intent.Name)
		}
		if reg.HasDir(dir) {
			ex, _ := reg.FindByDir(dir)
			return fmt.Errorf("directory %s is already registered as project %q", dir, ex.Name)
		}

		if err := project.SaveLayout(a.Paths, intent.Name, l); err != nil {
			return err
		}
		if err := reg.Add(project.Project{
			Name:      intent.Name,
			Dir:       dir,
			Layout:    templateKey,
			CreatedAt: time.Now().UTC(),
		}); err != nil {
			_ = project.PurgeProjectDir(a.Paths, intent.Name)
			return err
		}
		if err := reg.Save(a.Paths); err != nil {
			_ = project.PurgeProjectDir(a.Paths, intent.Name)
			return err
		}
		_, _ = fmt.Fprintf(out, "Registered %q at %s (layout: %s)\n", intent.Name, dir, templateKey)

		// Auto-launch after creation (matches user expectation that "registering" also opens).
		p, err := reg.Get(intent.Name)
		if err != nil {
			return err
		}
		return switchToProject(ctx, a, p)
	})
}

// preCheckCollisions surfaces name/dir collisions before a potentially slow clone.
// Same checks are repeated under the lock later — this is a UX improvement only.
func preCheckCollisions(a *app, name, dir string) error {
	reg, err := project.Load(a.Paths)
	if err != nil {
		return nil //nolint:nilerr // best-effort pre-check
	}
	if reg.Has(name) {
		return fmt.Errorf("project %q already registered (use 'boo delete %s' first)", name, name)
	}
	if reg.HasDir(dir) {
		ex, _ := reg.FindByDir(dir)
		return fmt.Errorf("directory %s is already registered as project %q", dir, ex.Name)
	}
	return nil
}

// readGitRemote returns the URL of the `origin` remote for the git repo at dir,
// or "" if unavailable. Shells out directly to os/exec (not Runner) because
// Runner doesn't model a working directory — a chdir on the boo process would be
// unsafe with concurrent invocations. Call is informational and never fatal.
func readGitRemote(dir string) (string, error) {
	cmd := exec.Command("git", "remote", "get-url", "origin")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// repoNameFromRemoteURL extracts the repo name from a clone URL.
// Local copy avoids importing internal/git (which only exposes a Cloner).
// Returns "" if the URL isn't a recognisable clone URL.
func repoNameFromRemoteURL(url string) string {
	url = strings.TrimSpace(url)
	if url == "" {
		return ""
	}
	trimmed := strings.TrimRight(url, "/")
	idx := strings.LastIndexAny(trimmed, "/:")
	if idx == -1 {
		return ""
	}
	name := trimmed[idx+1:]
	return strings.TrimSuffix(name, ".git")
}
