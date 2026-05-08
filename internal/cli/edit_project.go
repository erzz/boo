package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/erzz/boo/internal/layout"
	"github.com/erzz/boo/internal/project"
)

// executeEdit applies the user's desired post-edit values to an
// existing project: rename, change directory, and/or switch template.
// Used by the TUI's edit form (EditIntent dispatch); reusable for any
// future `boo edit-project` CLI surface.
//
// Semantics:
//
//   - Each field is compared against the current value. No-op fields
//     are skipped — passing the same name/dir/template back is fine
//     and writes nothing.
//   - Renames migrate the per-project state directory by os.Rename
//     (cheap, atomic on the same filesystem) and replace the registry
//     entry. The state dir holds layout.yaml + state.json; everything
//     else (project source code) lives outside boo's data dir and is
//     never touched.
//   - Template changes re-resolve and rewrite layout.yaml, same as
//     executeSetLayout. Hand-edits to the previous layout file are
//     destroyed — matches the "I want this template now" intent.
//   - Directory changes update the registry's Dir field only. We do
//     NOT validate that the new dir exists — the project may be
//     temporarily offline (external drive, sshfs, etc.) and we don't
//     want to refuse a config change for that. `boo doctor` and the
//     picker's "dir-missing" status pill already signal the issue.
//
// All checks and writes happen under the state lock so concurrent
// shells can't race on the registry. On any error the function
// returns without partial application — we either rename + save +
// update together, or none of them.
//
// Caveat: if os.Rename succeeds but reg.Save fails (disk full, etc.),
// the state dir is moved but the registry still points at the old
// name. The next boo invocation will fail to find the layout under
// the old name. Pre-release: no recovery code; user can `mv` the
// state dir back manually. Post-release we'd add a journal step.
func executeEdit(a *app, oldName, newName, newDir, newTemplate string) error {
	newName = strings.TrimSpace(newName)
	newDir = strings.TrimSpace(newDir)
	newTemplate = strings.TrimSpace(newTemplate)

	if newName == "" {
		return fmt.Errorf("new name is required")
	}
	if newDir == "" {
		return fmt.Errorf("new directory is required")
	}

	// Resolve the new dir to an absolute path so the registry stores
	// canonical values. Mirrors `boo new`'s resolveDir behaviour.
	absDir, err := filepath.Abs(newDir)
	if err != nil {
		return fmt.Errorf("resolve new directory: %w", err)
	}

	// Validate the new name even if it matches the old one — keeps
	// the rule "every name in the registry passes ValidateName" true
	// regardless of how the entry got there. Cheap.
	if err := project.ValidateName(newName); err != nil {
		return err
	}

	return a.Paths.WithLock(func() error {
		reg, err := project.Load(a.Paths)
		if err != nil {
			return err
		}
		current, err := reg.Get(oldName)
		if err != nil {
			return err
		}

		nameChanged := newName != oldName
		dirChanged := absDir != current.Dir
		tplChanged := newTemplate != "" && newTemplate != current.Layout

		if !nameChanged && !dirChanged && !tplChanged {
			return nil // nothing to do; treat as success
		}

		// Collision checks. Only meaningful when the corresponding
		// field is changing — a project always "collides" with itself
		// on the unchanged axes.
		if nameChanged && reg.Has(newName) {
			return fmt.Errorf("project %q already exists", newName)
		}
		if dirChanged {
			if ex, err := reg.FindByDir(absDir); err == nil && ex.Name != oldName {
				return fmt.Errorf("directory %s is already registered as project %q", absDir, ex.Name)
			}
		}

		// Re-resolve template up-front: a template typo should fail
		// the edit *before* we rename the state dir, so a bad
		// template name doesn't leave the user with a renamed
		// project sitting on the old layout.
		var resolved layout.ResolvedTemplate
		if tplChanged {
			r, err := layout.ResolveTemplate(a.Paths.LayoutsDir, newTemplate)
			if err != nil {
				return err
			}
			resolved = r
			if resolved.Layout.Name == "" {
				resolved.Layout.Name = newTemplate
			}
		}

		// Rename the state directory first. If this fails (permission,
		// cross-device link, etc.) we abort before touching the
		// registry, so on-disk state stays self-consistent.
		if nameChanged {
			oldDir := a.Paths.ProjectDir(oldName)
			newDirPath := a.Paths.ProjectDir(newName)
			if _, err := os.Stat(newDirPath); err == nil {
				return fmt.Errorf("state dir for %q already exists at %s; remove it manually first", newName, newDirPath)
			}
			if err := os.Rename(oldDir, newDirPath); err != nil {
				return fmt.Errorf("rename project state dir: %w", err)
			}
		}

		// Apply template change after the (potential) rename so we
		// write into the new directory.
		if tplChanged {
			writeName := newName
			if !nameChanged {
				writeName = oldName
			}
			if err := project.SaveLayout(a.Paths, writeName, resolved.Layout); err != nil {
				return err
			}
		}

		// Update the registry entry. Since Project.Name is part of
		// the key, a rename is Remove+Add; a same-name update goes
		// through Registry.Update.
		updated := current
		updated.Name = newName
		updated.Dir = absDir
		if tplChanged {
			updated.Layout = resolved.Layout.Name
		}

		if nameChanged {
			if err := reg.Remove(oldName); err != nil {
				return err
			}
			if err := reg.Add(updated); err != nil {
				return err
			}
		} else {
			if err := reg.Update(updated); err != nil {
				return err
			}
		}
		return reg.Save(a.Paths)
	})
}
