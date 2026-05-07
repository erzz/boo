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

func newNewCmd() *cobra.Command {
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
			a, err := newApp()
			if err != nil {
				return err
			}

			// Build form defaults from flags + cwd inspection. Flag values
			// always win; only unset fields fall back to detection.
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

			// --yes is the non-interactive escape hatch. It requires a fully
			// resolved intent (everything that a successful form submission
			// would produce). If anything is missing, we error rather than
			// silently dropping into the TUI on a script.
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
			})
			if err != nil {
				return err
			}
			if res.Cancelled() {
				return nil
			}
			// User may have switched to an existing project from the
			// AlreadyRegistered prompt — but in form-only mode we never show
			// that prompt, so this branch shouldn't fire. Handle it safely
			// regardless.
			if res.Selected != "" {
				p, err := project.Load(a.Paths)
				if err != nil {
					return err
				}
				pr, err := p.Get(res.Selected)
				if err != nil {
					return err
				}
				return switchToProject(c.Context(), a, pr)
			}
			return runCreateProject(c.Context(), a, *res.NewProject, c.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&fromURL, "from", "", "git URL to clone from")
	cmd.Flags().StringVar(&intoDir, "into", "", "directory to clone into (with --from); defaults to repo name in cwd")
	cmd.Flags().StringVar(&existing, "dir", "", "existing directory to register")
	cmd.Flags().StringVar(&layoutName, "layout", "", "layout template to use (default: 'triple')")
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

// buildNewProjectDefaults assembles the form defaults shown in the TUI (or
// pre-resolves the intent for --yes mode).
//
// Resolution order, per field:
//
//   - flags win
//   - then: cwd inspection (basename, git remote)
//   - then: hard-coded defaults (form supplies "triple" when blank)
//
// AlreadyRegisteredAs is filled when the resolved Dir matches an existing
// registered project — the TUI then prompts "switch or continue?".
func buildNewProjectDefaults(a *app, fl defaultsFromFlags) (picker.FormDefaults, error) {
	cwd, _ := os.Getwd()

	// Directory: explicit --dir wins; otherwise --into (for clones); otherwise
	// cwd as a sensible default for "register what I'm in".
	dir := strings.TrimSpace(fl.dir)
	if dir == "" {
		dir = strings.TrimSpace(fl.into)
	}
	if dir == "" {
		dir = cwd
	}
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}

	// Git remote: only inspected when dir exists. Best-effort; absent remote
	// is normal.
	var gitRemote, repoName string
	if dirExists(dir) {
		if remote, err := readGitRemote(dir); err == nil && remote != "" {
			gitRemote = "origin → " + remote
			repoName = repoNameFromRemoteURL(remote)
		}
	}

	// Name: explicit positional arg wins; otherwise the git-derived repo name
	// (preferred when distinct from the path basename); otherwise basename.
	name := strings.TrimSpace(fl.name)
	if name == "" {
		if repoName != "" {
			name = repoName
		} else {
			name = filepath.Base(dir)
		}
	}

	// Already-registered detection: only meaningful when we're suggesting an
	// existing dir. For clone flows the dir doesn't exist yet, so skip.
	var alreadyAs string
	if dirExists(dir) && fl.from == "" {
		reg, err := project.Load(a.Paths)
		if err == nil {
			if p, err := reg.FindByDir(dir); err == nil {
				alreadyAs = p.Name
			}
		}
	}

	// When no --layout was given, leave Template empty here. The form
	// (newFormModel) supplies its own default ("triple"); duplicating it
	// here would mean the migration has to update two places every time
	// the default changes. Single source of truth lives in the form.
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

// defaultsToIntent converts a fully-specified set of defaults into an intent.
// Used by --yes mode to skip the form when the caller has supplied enough.
//
// If Template was left blank (no --layout flag), we apply DefaultLayout
// here so --yes honours the user's configured default. The form path
// applies the same fallback inside collect().
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

// runCreateProject performs the actual registration (and clone, if From is
// set). Lifted out of the cobra RunE so it can be called from both the
// CLI flag-driven path and the interactive form's submission path.
//
// Concurrency: clones run outside the registry lock so slow network IO
// doesn't block other boo invocations. The lock is taken only for the
// read-modify-write window on the registry itself.
func runCreateProject(ctx context.Context, a *app, intent picker.NewProjectIntent, out io.Writer) error {
	if err := project.ValidateName(intent.Name); err != nil {
		return err
	}
	// When both From and Dir are set we treat Dir as the clone destination
	// ("into") rather than rejecting, so the TUI can pre-populate Dir from
	// a derived clone destination. No special branch needed — the clone
	// path below already does the right thing.
	if intent.From == "" && intent.Dir == "" {
		return errors.New("either Directory or Clone from URL is required")
	}

	// Resolve layout up front.
	resolved, err := layout.ResolveTemplate(a.Paths.LayoutsDir, intent.Template)
	if err != nil {
		return err
	}
	l := resolved.Layout
	if l.Name == "" {
		l.Name = intent.Template
	}
	if l.Name == "" {
		l.Name = "default"
	}

	// Resolve directory.
	var dir string
	if intent.From != "" {
		// Apply git default-remote shorthand expansion: a bare repo
		// name like "boo" becomes "<git.default_remote>/boo". A full
		// URL is left alone. See expandRepoShorthand for the rules.
		intent.From = expandRepoShorthand(intent.From, a.Config.GitDefaultRemoteOr(""))

		// Clone flow. If Dir was provided treat it as --into; otherwise
		// derive from the URL relative to projects_dir (or cwd).
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
			Layout:    l.Name,
			CreatedAt: time.Now().UTC(),
		}); err != nil {
			_ = project.PurgeProjectDir(a.Paths, intent.Name)
			return err
		}
		if err := reg.Save(a.Paths); err != nil {
			_ = project.PurgeProjectDir(a.Paths, intent.Name)
			return err
		}
		_, _ = fmt.Fprintf(out, "Registered %q at %s (layout: %s)\n", intent.Name, dir, l.Name)

		// Auto-launch after creation. Matches the user expectation that
		// "registering" a new project also opens it (mirrors the pre-form
		// behaviour of `boo new`).
		p, err := reg.Get(intent.Name)
		if err != nil {
			return err
		}
		return switchToProject(ctx, a, p)
	})
}

// preCheckCollisions surfaces obvious name/dir collisions before kicking off
// a (potentially slow) clone. Same checks are repeated under the lock later
// — this is purely a UX improvement.
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

// readGitRemote returns the URL of the `origin` remote for the git repo at
// dir, or "" if dir is not a git repo / has no origin / git is unavailable.
//
// We shell out directly to os/exec rather than going through internal/exec's
// Runner because Runner doesn't model a working directory (it would force a
// chdir on the boo process — unsafe with concurrent invocations). The call
// is informational, best-effort, and never fatal: any error is swallowed by
// the caller and the form just won't pre-populate.
func readGitRemote(dir string) (string, error) {
	cmd := exec.Command("git", "remote", "get-url", "origin")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// repoNameFromRemoteURL is a defensive copy of the git package's URL parser.
// We don't import internal/git here because that package only exposes a
// Cloner; pulling in DeriveDestination would create a small dependency
// cycle risk if git ever needs to call back into anything in cli/.
//
// Returns "" if the URL doesn't look like a clone URL we recognise.
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
