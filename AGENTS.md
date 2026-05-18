# Project: boo

Project launcher for the Ghostty terminal emulator on macOS. Go CLI.

## Stack

Go 1.24+, cobra (CLI), Bubble Tea + Lip Gloss + Bubbles (TUI), `sigs.k8s.io/yaml` (layouts), pelletier/go-toml v2 (global config), `osascript -l JavaScript` (JXA) for Ghostty control. macOS only for v1.

## Layout

- `cmd/boo/` — main entrypoint, wires cobra
- `internal/cli/` — cobra command definitions
- `internal/ghostty/` — JXA generation and osascript runner; the only place that talks to Ghostty
- `internal/layout/` — layout tree model, YAML parsing, validation, built-in templates
- `internal/layoutpreview/` — ASCII renderer for layout previews (used by `boo layouts` and the TUI form)
- `internal/project/` — project model and registry CRUD
- `internal/state/` — XDG paths, atomic file IO; all state under `$XDG_CONFIG_HOME/boo/`
- `internal/config/` — global config loader (YAML at `~/.config/boo/config.yaml`)
- `internal/picker/` — Bubble Tea TUI (project list + new-project form with layout cycler)
- `internal/doctor/` — environment checks
- `internal/exec/` — `Runner` interface; production wraps `os/exec`, tests use fake
- `assets/jxa/` — JXA script templates (embedded with `go:embed`)
- `internal/layout/templates/` — bundled layout templates (embedded)

## How to run / test / build

```
make build       # builds ./bin/boo
make test        # unit tests
make test-int    # integration tests (-tags=integration; requires Ghostty installed; refuses to run from inside a Ghostty window without BOO_ALLOW_GHOSTTY_INTEGRATION=1)
make lint        # golangci-lint
make fmt         # gofmt + goimports
./bin/boo doctor # env sanity check
```

## Conventions

- Every shell-out goes through `internal/exec.Runner`. Documented exceptions: `internal/cli/fzf.go` (interactive TTY hand-off), `git remote get-url` in the new-project flow, and `$EDITOR` invocations in `internal/cli/config.go` (`boo config edit`) and `internal/cli/edit.go` (`boo edit`). Anywhere else, calling `os/exec` directly breaks tests.
- All Ghostty interaction lives in `internal/ghostty`. No JXA strings or `osascript` calls anywhere else.
- **Layouts are YAML** (`sigs.k8s.io/yaml`). **Global config is TOML** (pelletier/go-toml v2). **JSON is internal-only** — state files and JXA stdin/stdout payloads.
- Errors are wrapped with `fmt.Errorf("...: %w", err)`; user-facing errors come from a small set of helpers in `internal/cli` so messages stay consistent.
- `slog` for logging; `--verbose` global flag flips level to debug. No `fmt.Println` for diagnostics.
- Layout files: YAML in boo's own vocabulary (windows/tabs/recursive split tree). Never expose raw JXA or AppleScript to users.

## Entry points

- New command: `internal/cli/<verb>.go`, register in `internal/cli/root.go`.
- New Ghostty capability: extend `internal/ghostty.Client` interface and the JXA template in `assets/jxa/`.
- New layout feature: update `internal/layout/layout.go` (tree types + validator), the YAML round-trip, the JXA walker in `assets/jxa/open_layout.js`, and the ASCII renderer in `internal/layoutpreview/` in lockstep.
- Doctor checks: add to `internal/doctor/checks.go`.

## Layout model invariants

- A `Split` is a leaf XOR an interior node. Leaves carry `cwd`/`command`/`env`/`initial_input`. Interior nodes carry `direction` (`row` or `column`) and `children`.
- **Interior nodes have EXACTLY 2 children**, not ≥2. Ghostty's `app.split` halves the focused pane; N>2 children would yield 50/25/25, not equal thirds.
- **`row` → JXA `right`, `column` → JXA `down`.** Direction is interior-only; never appears on a leaf.
- **Leaf order is DFS left-to-right.** This is the same order the JXA walker materialises panes, the same order Ghostty's `DescribeWindow` returns terminals, and the same order `save`'s diff/merge uses to align leaves between the prior layout and the captured window. Don't reshuffle.
- **JXA walker contract for interior nodes:** for `interior(dir, [a, b])`, pre-split before recursing into `a` so the right subtree's space is reserved before `a`'s internal splits subdivide the left half.
- **Tab `name:` is round-tripped but ignored on open.** Ghostty 1.3.x marks `tab.name` as read-only in its sdef and offers no non-interactive "set tab title" action; `inputText` strips the ESC byte so OSC 2 can't be smuggled in. Keep the field in the schema; revisit when Ghostty exposes a writable `tab.name` or a `set_surface_title:<text>` action.

## Save pipeline

- `boo save` reads the live window via JXA. The capture is a **flat per-tab terminal list** (id/title/cwd) — split tree shape, command, env, and initial-input are not visible.
- Merge strategy: if the captured leaf count for a tab matches the previously-saved tab's leaf count, walk the prior tree preserving its shape and zip in the captured cwds (lossless re-save of hand-authored trees). If the count differs, fall back to a right-leaning row chain — matches the Cmd-D-N-1-times default and avoids silently changing pane proportions.
- The diff uses DFS leaf indexing (`LossyLeaves`) to mark cells where unrecoverable fields would be dropped. `--force` skips the prompt but still prints the diff to stderr.

## Defaults & UX

- Default layout template is `triple` (1 large left pane, 2 stacked on the right). Single source of truth: `picker/form.go`'s `mk("triple", ...)` calls. Upstream callers leave `Template: ""` and let `ResolveTemplate("","")` and the form fall through to `triple`.
- The new-project form's Layout field is a **cycler** (←/→ or h/l), not free text — closed enum, closed-enum widget.
- Bare `boo` always lands on the project list. The "this directory is already registered" interstitial only fires for `formOnly` flows (`boo new` / `boo save` fallback).
- The new-project form opens the **layout editor** on submit (`internal/picker/layout_editor.go`). It opens in **LAYOUT mode** (move dividers, select panes); `c` enters COMMAND mode for the focused leaf, `enter`/`esc` commits and returns. Apply is `ctrl+s` (not `enter` — would collide with the textinput). Split proportions live on interior nodes as `Split.Size` (first child's fractional share, `(0,1)` exclusive; `0`/omitempty = even).

## Gotchas

- `ghostty +new-window` is **not supported on macOS** (`"not supported on this platform"`). Use AppleScript via `osascript -l JavaScript`. Use `open -na Ghostty.app` only for cold-start.
- macOS will prompt for Automation permission the first time boo controls Ghostty. `boo doctor` detects and surfaces this.
- Ghostty window/tab/terminal IDs are stable only within a single Ghostty process lifetime. Don't persist them across reboots — regenerate on cold start.
- JXA escaping is treacherous. Always build the parameter object as JSON in Go, then embed it into the JXA script as a single `JSON.parse(...)` call. Never string-concatenate user values into JS source.
- **Surface configuration properties are silently dropped if passed to `newSurfaceConfiguration({...})`.** Construct the empty config, then assign properties to the returned record (`cfg.initialWorkingDirectory = "..."`). Re-test if Ghostty's sdef changes.
- Ghostty is pre-2.0 and the AppleScript API surface may change. Pin a tested version range in `doctor` and warn on mismatch — don't hard-fail.
- **JXA method names ≠ sdef command names.** The `"perform action"` command in `Ghostty.sdef` is exposed in JavaScript for Automation as `app.performAction(...)`, not `app.perform(...)`. Calling the latter silently dispatches to a generic Cocoa selector that returns "Message not understood." at runtime — and our `flushResizes` swallows that error on purpose, so the only symptom is dividers staying at 50/50. Always camel-case the sdef command name (whitespace dropped). Regression pin: `TestOpenLayoutScript_UsesPerformActionNotPerform`.
- **`resize_split:<dir>,N` grows the FOCUSED pane in that direction**, not "moves the divider that way". Our JXA focuses the second child's leftmost leaf when resizing, so to grow the FIRST child we send the action toward the SECOND pane's interior (row → `right`, column → `down`). The naming feels inverted; the comment block in `open_layout.js`'s `recordResize` is the source of truth — re-derive from a sized test (`size=0.7` should grow the first child to 70%) before changing the action names.
- Pre-release: no migration code. Old `~/.config/boo/layouts/*.toml` and `~/.config/boo/projects/*/layout.toml` from before the YAML migration will fail to parse — delete them. If you have old state under `~/.local/share/boo/` from before the config consolidation, `boo doctor` will warn you — delete it.
