# boo — Design

Project launcher for the [Ghostty](https://ghostty.org) terminal emulator. Switch between project-scoped layouts (windows, tabs, splits with working directories and startup commands) by name from the CLI or a TUI picker.

## Goals (v1)

- `boo <name>` opens a Ghostty window for project `<name>` with its saved layout (cwd set, splits arranged, optional startup commands).
- `boo` (no args) inside a known project directory does the same for that project.
- `boo new <name>` registers a project — point at an existing directory or clone from a git URL — and picks a layout template.
- `boo pick` opens a Bubble Tea TUI picker over known projects.
- `boo list`, `boo rm`, `boo doctor`.
- Layouts: windows containing tabs containing splits, each with `cwd`, optional `command`, optional `env`.
- Persistence: a Ghostty window stays open across switches; processes inside survive because the window/tabs/splits are not closed.

## Non-goals (v1)

- **Linux / Windows.** Rich integration relies on macOS AppleScript. Fallback CLI-only mode could come later.
- **tmux or any multiplexer.** Ghostty's native windows/tabs/splits are the unit.
- **Process supervision / restart.** If `npm run dev` crashes, boo doesn't notice.
- **Cross-machine sync.** Local state only.
- **Built-in keybinding daemon.** `boo pick` is exposed; users wire it up via Ghostty keybinds, Raycast, skhd, etc.
- **Replacing Ghostty's own UX.** Splits, scrollback, copy, search are all native Ghostty.
- **direnv/mise/asdf integration.** Boo sets cwd; the user's shell init handles the rest.
- **Layout capture / "save current state."** Deferred; all the AppleScript reads exist to enable this later.
- **Per-project `.boo.toml` overrides.** Central registry only in v1.
- **Editor integration, secrets, remote/SSH.**

## Locked decisions

| Topic | Decision |
|---|---|
| Language | Go (1.24+) |
| Ghostty integration | JXA scripts executed via `osascript -l JavaScript`; cold-start via `open -na Ghostty.app` |
| Layout model | Window → Tabs → Splits, each with `cwd` / `command` / `env` |
| Layout source of truth | Snapshot stored per-project at create time (template changes don't silently mutate existing projects) |
| Config format | TOML (pelletier/go-toml v2) |
| CLI framework | cobra |
| TUI | Bubble Tea + Lip Gloss + Bubbles |
| Logging | stdlib `log/slog` |
| Build / task runner | Makefile |
| Release | goreleaser (added at v0.1) |
| Platform | macOS only for v1 |
| Project identity | Name is primary key. Directory tracked separately; `boo doctor` / `list` flags broken paths. |
| Magic create | `boo <name>` for unknown names fails with hint. Create requires explicit `boo new`. |
| Cwd auto-detect | Bare `boo` (no args) inside a known project dir switches to it. |
| `initialInput` / env vars | Supported in v1 layouts (free with the AppleScript API). |

## Architecture

### Package layout

```
boo/
├── cmd/boo/                  # main, wires cobra
├── internal/
│   ├── cli/                  # cobra command definitions
│   ├── ghostty/              # JXA generation + osascript runner
│   │   └── jxa/              # embedded JXA script templates
│   ├── layout/               # layout model, TOML, validation
│   │   └── templates/        # embedded built-in layout templates (default.toml, dev.toml)
│   ├── project/              # project model, registry
│   ├── state/                # XDG paths, atomic file IO
│   ├── config/               # global config loader
│   ├── picker/               # Bubble Tea TUI
│   ├── doctor/               # environment checks
│   └── exec/                 # Runner interface (real + fake) for testability
├── Makefile
├── go.mod
├── README.md
├── DESIGN.md
├── SPIKE.md
└── AGENTS.md
```

### Key types

- `layout.Layout{Name, Tabs []Tab}`
- `layout.Tab{Name, Splits []Split}`
- `layout.Split{Cwd, Command, Env map[string]string, Direction string, InitialInput string}`
  - First split in a tab is the tab's primary surface (no direction); subsequent splits carry a direction.
- `project.Project{Name, Dir, LayoutPath, WindowID string, CreatedAt}`
- `project.Registry` — list/get/add/remove
- `state.Store` — XDG paths, atomic JSON/TOML read/write
- `ghostty.Client{runner exec.Runner}` — `OpenLayout(layout, dir) (windowID, error)`, `FocusWindow(id) error`, `WindowExists(id) bool`, `CloseWindow(id) error`
- `picker.Model` — Bubble Tea model over `[]Project`
- `doctor.Check{Name, Run() Result}` with a list of checks
- `exec.Runner` — `Run(ctx, cmd, args...) (stdout, stderr, err)` with `realRunner` and `fakeRunner`

### Disk state

- `~/.config/boo/config.toml` — global config (default layout, optional Ghostty path override)
- `~/.config/boo/layouts/*.toml` — user-defined shared layout templates
- `~/.local/share/boo/projects.toml` — registry index
- `~/.local/share/boo/projects/<name>/layout.toml` — resolved layout snapshot for that project
- `~/.local/share/boo/projects/<name>/state.json` — runtime state (last-known WindowID, last-launched-at)

### Core flows

1. **`boo projA` (window still open):** look up `WindowID` → ask Ghostty if it exists → if yes, `activate window` it. Done.
2. **`boo projA` (window gone):** load layout snapshot → generate JXA from layout → execute → store new `WindowID`.
3. **`boo projA` (Ghostty not running):** `open -na Ghostty.app` → wait for app to be up (poll AppleScript) → same as above.
4. **`boo projA` (unknown):** print hint `project "projA" not found. create it with: boo new projA`. Exit non-zero.
5. **`boo` (no args):** detect cwd → if inside a registered project's dir, treat as `boo <that-name>`. Else show short help.
6. **`boo new projA`:** prompt or accept flags for source (`--from <git-url>` to clone, `--into <dir>`, or `--dir <existing>`) and layout (`--layout <name>`, default = `default`). Clone if needed. Resolve layout template → write snapshot to `projects/projA/layout.toml`. Add to registry.
7. **`boo pick`:** Bubble Tea list of projects (name, dir, last-launched, status: window-open / window-closed / dir-missing) → enter dispatches to flow 1/2/3.

### JXA integration

A single Go `text/template` generates the JXA script per launch. Skeleton:

```javascript
const ghostty = Application("Ghostty");
ghostty.includeStandardAdditions = true;

const baseCfg = ghostty.NewSurfaceConfiguration({
  initialWorkingDirectory: "{{.Cwd}}",
  command: "{{.Command}}",
  environmentVariables: [{{range .Env}}"{{.}}",{{end}}],
});

const win = ghostty.newWindow({ withConfiguration: baseCfg });
const windowId = win.id();

// for each additional tab:
ghostty.newTab({ in: win, withConfiguration: tabCfg });

// for each split in a tab:
const term = win.tabs[i].focusedTerminal();
ghostty.split(term, { direction: "right", withConfiguration: splitCfg });

JSON.stringify({ windowId });
```

Output is JSON parsed by Go. Errors surface as non-zero exit + stderr.

## Risks (with mitigations)

1. **Stable IDs may not survive Ghostty restart.** Treat IDs as process-lifetime only; on cold start regenerate. `WindowExists` check before reuse.
2. **macOS Automation permission prompt.** `boo doctor` checks; first write-action will prompt; document clearly in README.
3. **Ghostty pre-stable AppleScript API.** Pin tested Ghostty version range in `boo doctor`; surface mismatch warnings.
4. **JXA escaping bugs.** Pass values via JSON.parse-able strings (build a JS object as a Go-side JSON string, embed once, never string-concat shell args). Test with adversarial cwds (spaces, quotes, unicode).
5. **Scope creep.** Non-goals list above is enforced in PRs.

## Phased delivery

- **Phase 0 — Scaffold.** Module, cobra skeleton with all command stubs returning "not implemented", `boo doctor` working (checks Ghostty installed + version + responds to AppleScript), Makefile, README, CI. Exit: `boo doctor` passes on a clean dev machine.
- **Phase 1 — MVP slice.** `boo new --dir <existing>` registers a project with the default single-tab layout. `boo <name>` opens the window. `boo list`. `boo rm`. Window persists across re-runs; second `boo <name>` focuses existing window.
  - **Deferred to Phase 2:** full transactional rollback across multi-file mutations (mitigated in Phase 1 by a cross-process file lock + best-effort cleanup); CLI-layer tests (the `new` command surface will change substantially when `--from <git-url>` lands, so tests are deferred until then to avoid rework).
- **Phase 2 — Layouts.** Multi-tab layouts. Splits. Layout templates in `~/.config/boo/layouts/`. `--from <git-url>` clone path.
- **Phase 3 — Picker + polish.** Bubble Tea picker. Better error messages. `boo doctor` improvements. Docs with GIFs.
- **Phase 4 — Stretch.** `boo save` (capture live layout). fzf picker mode. Linux CLI-fallback path. Homebrew tap via goreleaser.
