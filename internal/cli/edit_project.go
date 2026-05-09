package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/erzz/boo/internal/layout"
	"github.com/erzz/boo/internal/project"
)

// executeEdit applies post-edit values to an existing project: rename, change dir, switch template.
// Used by the TUI edit form (EditIntent) and reusable for future `boo edit-project` surfaces.
//
// Per-field semantics: no-op fields (same value) are skipped. Renames migrate the state dir via
// os.Rename. Template changes re-resolve and rewrite layout.yaml (hand-edits are destroyed).
// Dir changes update the registry only — we do NOT validate the new dir exists (project may be
// temporarily offline; boo doctor and the "dir-missing" pill already signal the issue).
//
// All changes happen under the state lock: either rename+save+update together, or none.
// Caveat: if os.Rename succeeds but reg.Save fails, the state dir is moved but the registry
// still points at the old name. Pre-release: no recovery code; user can mv back manually.
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

	// Validate the new name even if unchanged — keeps the invariant that every registered
	// name passes ValidateName regardless of how it got there.
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

		// Collision checks (only when the field is actually changing).
		if nameChanged && reg.Has(newName) {
			return fmt.Errorf("project %q already exists", newName)
		}
		if dirChanged {
			if ex, err := reg.FindByDir(absDir); err == nil && ex.Name != oldName {
				return fmt.Errorf("directory %s is already registered as project %q", absDir, ex.Name)
			}
		}

		// Re-resolve template before rename so a bad name fails before touching the state dir.
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

		// Rename the state dir first. If this fails, abort before touching the registry.
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

		// Apply template change after (potential) rename so we write into the new dir.
		if tplChanged {
			writeName := newName
			if !nameChanged {
				writeName = oldName
			}
			if err := project.SaveLayout(a.Paths, writeName, resolved.Layout); err != nil {
				return err
			}
		}

		// Update the registry: rename is Remove+Add; same-name update goes through Update.
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
