# Manual Testing

Work through this checklist before tagging a release, after touching any
command surface, or whenever Ghostty itself updates.

## Before you start

- macOS only. Tests assume Ghostty is installed at `/Applications/Ghostty.app`.
- Use a throwaway state dir to avoid clobbering your real projects:

  ```sh
  export BOO_HOME=/tmp/boo-manual-$$
  mkdir -p "$BOO_HOME"
  ```

  All `boo` commands below will then read/write under that path. Unset when done.
- Do **not** run integration tests (`make test-int`) from inside a Ghostty
  window — the guardrail should stop you, but don't rely on it. Use
  iTerm/Terminal.app for those.
- Some Ghostty checks require Automation permission for your terminal app.
  First run will trigger a macOS prompt; grant it.

Tick boxes as you go. Anything that fails: capture the exact command, output,
and `boo doctor` results.

---

## Setup

- [ ] 1. `make build` succeeds, produces `./bin/boo`.
- [ ] 2. `./bin/boo --help` lists: `new`, `list`, `fzf`, `delete`, `save`, `doctor`.
- [ ] 3. `./bin/boo doctor` — all checks OK (or fzf SKIP if not installed).

## Argument errors

- [ ] 3a. `boo new`, `boo save`, `boo delete` (all no-args) → land in interactive UI (form / front-window detect / picker), not an error.
- [ ] 3b. `boo new a b c` → error mentions got 3, expected 1.

## `boo doctor`

- [ ] 4. With fzf installed: shows `fzf (optional)  OK  /path/to/fzf`.
- [ ] 5. With fzf removed from PATH: shows `fzf (optional)  SKIP` and install hint; overall status still OK.
- [ ] 6. Quit Ghostty, run doctor: `ghostty running` is WARN, `automation permission` is SKIP, `fzf` still reports.

## `boo new` — basic

- [ ] 7. `boo new alpha --dir ~/some/existing/dir --yes` → creates project, opens Ghostty window in that dir.
- [ ] 8. `boo new alpha --dir ... --yes` again → fails with "already exists".
- [ ] 9. `boo new beta --dir ~/does/not/exist --yes` → fails clearly (dir doesn't exist).
- [ ] 10. `boo new save --dir ... --yes` → fails (reserved name). Repeat for `doctor`, `list`, `fzf`, `delete`, `new`, `help`, `completion`.
- [ ] 11. `boo new "weird name" --dir ... --yes` → fails name validation.
- [ ] 12. `boo new gamma --dir ~/dir --yes --layout default` → opens with default layout.
- [ ] 13. `boo new delta --dir ~/dir --yes --layout dev` → opens with dev layout (multi-tab/split).
- [ ] 14. `boo new eps --dir ~/dir --yes --layout /nonexistent.toml` → clean error.
- [ ] 15. `boo new eps --dir ~/dir --yes --layout ../../../etc/passwd` → rejected (path traversal).

## `boo new` — interactive form (no args)

- [ ] 15a. `cd ~/some/non-registered/dir && boo new` → form opens with: Name=basename, Directory=that dir, Clone-from=blank, Template=default. If the dir is a git repo, "Detected git remote: origin → <url>" line shown above the form, and Name pre-populates from the repo name (when different from basename).
- [ ] 15b. Tab / Shift-Tab / Down / Up navigates fields.
- [ ] 15c. Enter on the last field submits; Enter on earlier fields advances.
- [ ] 15d. Ctrl-S submits from any field.
- [ ] 15e. Esc cancels — exits 0, no project created.
- [ ] 15f. Submit with empty Name → form shows "✖ name is required" and stays open.
- [ ] 15g. Submit with empty Dir AND empty From → "✖ either Directory or Clone from URL is required".
- [ ] 15h. Submit with all fields → project registered AND Ghostty window opens (auto-launch).
- [ ] 15i. `cd` into a dir that's already registered → form opens on the "This directory is already registered" prompt FIRST, offering [s/enter] switch, [c] continue, [esc] cancel.

## `boo new --from` (clone)

- [ ] 16. `boo new myrepo --from https://github.com/erzz/boo --yes` → clones into derived dir, registers, opens.
- [ ] 17. `boo new myrepo --from <url> --into ~/code/myrepo --yes` → clones into specified dir.
- [ ] 18. `boo new x --from <url> --into ~/existing-nonempty-dir --yes` → fails before clone.
- [ ] 19. `boo new x --from invalid://url --yes` → fails with git error, no project registered.
- [ ] 20. While clone is running, `boo list` in another terminal works (no lock contention).
- [ ] 20a. `boo new --from <url>` (no `--yes`) → form opens with Clone-from pre-populated; submit clones and opens.

## `boo <name>` (switch)

- [ ] 23. `boo alpha` (window already open) → focuses existing window, no new one.
- [ ] 24. Close alpha's window, `boo alpha` → opens fresh window with full layout.
- [ ] 25. `boo nonexistent` → "not found" error.
- [ ] 26. From inside alpha's dir on disk, run `boo` (no args) → opens the TUI picker (no cwd-detect short-circuit; the picker is the one obvious behaviour for no-args).
- [ ] 27. `boo` outside any project dir → opens the built-in TUI picker.

## `boo list`

- [ ] 28. `boo list` → table with name, dir, status (running/stopped/dir-missing), last launched age.
- [ ] 29. Delete a project's dir on disk, `boo list` → status `dir-missing`.
- [ ] 30. Quit Ghostty entirely, `boo list` → all `stopped`.

## `boo delete`

- [ ] 31. `boo delete alpha` → prints what will be removed (registry + state) and what won't (source dir), prompts `Type 'y' to confirm:`. Bare Enter or `n` → "Aborted."; `y` → removes.
- [ ] 31a. `boo delete` (no args) → opens TUI picker in selection-only mode (no `+ New project` row, no `n`/`+` keybind), pick a project → goes to the same confirm prompt. Esc cancels with no output.
- [ ] 31b. `boo delete` (no args, empty registry) → clean error "no projects registered — nothing to delete"; no picker shown.
- [ ] 32. `boo delete alpha --force` → no prompt, removes immediately.
- [ ] 33. `boo delete nonexistent` → clean error.
- [ ] 34. After delete, `boo list` no longer shows it; `~/.local/share/boo/projects/alpha/` is gone.
- [ ] 35. `boo delete alpha` does NOT delete the project's source dir on disk.
- [ ] 35a. `boo delete alpha --purge` (window open) → prompts then closes Ghostty window for alpha too.

## Bare `boo` (picker fallback)

- [ ] 36. `cd` outside any project, run `boo` → built-in Bubble Tea picker opens with all projects.
- [ ] 37. Arrow keys / `j`/`k` navigate; Enter switches; `q` / Esc cancels (exit 0, no action).
- [ ] 38. With zero projects → prints "No projects registered" hint, exits 0 (no UI shown).
- [ ] 39. Pick a `dir-missing` project → switch should fail clearly (not crash).
- [ ] 40. Pick a running project → focuses existing window.
- [ ] 41. Pick a stopped project → opens new window with layout.

## `boo fzf`

- [ ] 42. `boo fzf` (fzf installed) → fzf opens with prompt `boo > `, header about enter/esc.
- [ ] 43. Each row shows `name  status · dir · age` (tab-separated visually).
- [ ] 44. Type to filter; Enter selects → boo switches to that project.
- [ ] 45. Esc → exits 0, no switch, no error.
- [ ] 46. Ctrl-C while fzf open → exit 0, clean.
- [ ] 47. Type a query that matches nothing, Enter → exits 0, no switch.
- [ ] 48. `boo fzf` with fzf NOT on PATH → clear error mentioning install or run `boo` for built-in.
- [ ] 49. `boo fzf` with zero projects → "No projects registered" (fzf never opens).

## `boo save`

- [ ] 50. Open a project window, manually open extra tabs / cd around, then `boo save alpha` → writes layout file; prints "Captured N tab(s)…" summary; on **first** save and on **no-change re-save**, no prompt and no diff.
- [ ] 51. `boo save alpha` again immediately (no changes since last save) → silent overwrite (no prompt, no diff). Idempotent saves don't nag.
- [ ] 52. `boo save alpha --force` → skips any confirmation. Lossy diffs (see 57b) are still printed to stderr under --force.
- [ ] 53. `boo save nonexistent` → clean error (project not registered).
- [ ] 54. `boo save alpha` when alpha has no live window → clean error ("no window for project").
- [ ] 54a. With a project window focused, run `boo save` (no args) → detects the project from the focused Ghostty window (prints "Detected project ... from focused Ghostty window."), then proceeds as a normal save.
- [ ] 54b. `boo save` (no args) when Ghostty has no focused window (or isn't running) → clean error suggesting `boo save <name>`.
- [ ] 54c. `boo save` (no args) when the focused Ghostty window isn't a registered project → falls through to the new-project TUI form, pre-populated with that window's working directory. Submit registers + opens; Esc cancels with no output.
- [ ] 55. Inspect the saved TOML at `~/.local/state/boo/projects/alpha/layout.toml`:
  - tabs match what was open;
  - cwds are relative to project root when inside it, absolute otherwise;
  - non-primary splits saved with `direction = "right"`;
  - any tabs that were empty are dropped (warning printed).
- [ ] 56. After save, close the window and `boo alpha` → reopens with the saved layout (tabs restored, cwds correct).
- [ ] 57a. **Structural diff:** open the window, add a new tab in Ghostty, `boo save alpha` → ASCII diff renders just the changed tab(s) with a `→` arrow; prompt asks "Apply this change? [y/N]". `n` aborts, `y` saves.
- [ ] 57b. **Lossy diff:** hand-edit `~/.local/state/boo/projects/alpha/layout.toml` to set `direction = "down"` on a non-primary split (or add `command = "..."`/`env = {...}`). Re-save → ASCII diff marks affected cells with a trailing `!`; an "Unrecoverable on next save:" section explains what's lost; prompt asks "Save anyway and lose the unrecoverable data above?". `--force` skips the prompt but still prints the diff to stderr.


## Layouts / templates

- [ ] 58. `boo new t1 --dir ~/x --template default` → single tab, project root.
- [ ] 59. `boo new t2 --dir ~/x --template dev` → multi-tab + split layout opens correctly.
- [ ] 60. Hand-edit `~/.local/state/boo/projects/t1/layout.toml`, close window, `boo t1` → new layout takes effect.
- [ ] 61. Corrupt the layout TOML, `boo t1` → clean validation error, no half-built window.
- [ ] 61a. `boo layouts` → lists `default`, `dev`, `triple` (each with `[built-in]`, description, ASCII preview). Output is human-readable; previews show borders, cwd dot, and any annotations.
- [ ] 61b. Drop a `~/.config/boo/layouts/scratch.toml` with a leading `# ...` comment, run `boo layouts` → `scratch` appears with `[user]`, the comment is the description, preview matches the TOML.
- [ ] 61c. Override `default` by writing `~/.config/boo/layouts/default.toml`. `boo layouts` lists `default` ONCE, tagged `[user]`, with the override's description (not the built-in's). Remove the file when done.
- [ ] 61d. Write `~/.config/boo/layouts/broken.toml` containing garbage. `boo layouts` still succeeds, lists `broken` with `[error]` and an inline reason; healthy layouts still appear. Remove the file.
- [ ] 61e. `boo new` (no flags) → form opens with template field showing `default` and a preview block below. Tab to template field, type `dev` then `triple` → preview updates to match each. Type `nope` → preview disappears (no error inside the form).
- [ ] 61f. `boo` (bare) → pick `+ New project`, same preview behaviour as 61e.

## Ghostty edge cases

- [ ] 62. Quit Ghostty entirely, `boo alpha` → cold-starts Ghostty via `open -na`, then opens window.
- [ ] 63. Revoke Automation permission for your terminal → Ghostty → next `boo` command → doctor explains how to fix; commands fail clearly (not silently).
- [ ] 64. Restart Ghostty (kill + relaunch), `boo list` → previously-running projects show `stopped` (stale window IDs cleaned up).
- [ ] 65. `boo alpha`, then quit Ghostty, then `boo alpha` again → no stale-ID errors.

## State / paths

- [ ] 66. `BOO_HOME=/tmp/boo-test ./bin/boo new sandbox --dir ~/foo` → all state under `/tmp/boo-test`, default `~/.local/state/boo` untouched.
- [ ] 67. Two `boo new` invocations racing in parallel → second waits on lock, both succeed (or second fails cleanly on name conflict).
- [ ] 68. Manually delete `~/.local/state/boo/projects.toml` → `boo list` shows empty, no crash.

## Logging

- [ ] 69. `boo --verbose <anything>` → debug logs to stderr; normal output to stdout still clean.
- [ ] 70. No `fmt.Println` diagnostics anywhere in normal output (only structured slog at debug).

## Integration tests (optional, only outside Ghostty)

> Run these from iTerm or Terminal.app — never from inside a Ghostty window.

- [ ] 71. From iTerm/Terminal.app: `make test-int` → integration tests run and pass.
- [ ] 72. From inside a Ghostty window: `make test-int` → guardrail aborts with clear message about `BOO_ALLOW_GHOSTTY_INTEGRATION=1`.
