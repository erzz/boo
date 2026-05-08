<p align="center">
  <img src="docs/brand/boo.svg" alt="boo" width="240">
</p>

<p align="center"><i>a haunt for every project</i></p>

# boo

A project launcher for [Ghostty](https://ghostty.org).

Switching between projects in a terminal usually means a minute of yak-shaving: open a new window, `cd` to the right place, re-open your editor, re-launch the dev server, re-open lazygit in a split. boo collapses all of that into one command — `boo projA` — and remembers each project's layout (windows, tabs, splits, working directories, startup commands) so it's identical every time.

```
boo                  # interactive picker over all known projects
boo projA            # open or focus the projA window directly
boo new projA        # register a project (existing dir or clone from URL)
boo save             # snapshot the focused window's layout for the current project
boo list             # show all projects (--json to script)
boo show projA       # everything boo knows about a project
boo edit projA       # open a project's layout file in $EDITOR
boo set-layout projA triple   # switch a project to a different layout template
boo layouts          # preview every available layout template
boo delete projA     # remove a project (--purge to also close its window)
boo config           # inspect / edit global config
boo doctor           # sanity-check your environment
```

## Status

Pre-alpha. Under active development. macOS only.

## How it works

boo drives Ghostty via its native AppleScript / JXA API. There is no tmux, no terminal multiplexer, no leaky abstraction — splits are Ghostty splits, scrollback is Ghostty scrollback, everything Ghostty does just works. Windows you opened with boo stay open across switches, so the processes inside them keep running.

## Interactive picker

Running `boo` with no arguments opens a Bubble Tea TUI: bordered project list on the left, context preview on the right (when the terminal is wide enough), status bar at the bottom.

Keybindings on the project list:

| key | action |
|-----|--------|
| `↑` / `↓`, `k` / `j` | move selection |
| `/` | filter by name or path |
| `enter` | switch to the selected project |
| `n`, `+` | register a new project |
| `e` | edit the selected project (rename, change directory, change layout template) |
| `l` | cycle the layout template for the selected project (`←` / `→` to choose, `enter` to apply) |
| `o` | open the selected project's layout YAML in `$EDITOR` (TUI suspends, resumes on exit) |
| `d` | delete the selected project (asks to confirm) |
| `D` | delete **and** close the project's Ghostty window |
| `?` | toggle full help |
| `q`, `esc`, `ctrl-c` | quit |

The right pane shows the selected project's directory, status (running / stopped / dir-missing), last-launched time, and an ASCII preview of its current layout. Below ~90 cols or ~24 rows the picker collapses to single-pane mode and uses the full width for the list — visible status pills and the status bar are still rendered.

The status bar reflects the most recent action's outcome: `✓ deleted alpha`, `✖ rename failed: …`, etc. Idle state shows a faint `press ? for help`.

## New-project form

`n` (or selecting `+ New project`) opens a form:

- **Name** — display name and the key used by `boo <name>`
- **Directory** — existing path, or a `git@…` / `https://…` URL to clone
- **Layout** — cycler over the available templates (`←` / `→` to choose). Defaults to `triple`.

Submitting the form registers the project, captures or generates its layout, and immediately opens the Ghostty window. Cancel with `esc`.

## Layouts

Layouts are YAML — one per project, plus shared templates. See [`docs/layouts.md`](./docs/layouts.md) for the full reference, the bundled built-ins (`single`, `dual`, `triple`, …), and copy-paste examples (Node, Go, Rust, Python).

A layout is a tree of windows → tabs → splits. Each leaf can specify:

- `cwd` — initial working directory (defaults to the project's root)
- `command` — long-running command to launch in that pane (e.g. `nvim .`)
- `initial_input` — text to type into the shell after the pane opens (e.g. `lazygit\n`)
- `env` — extra environment variables for that pane

`triple` is the default: one large left pane, two stacked panes on the right.

## Configuration

Global config lives at `~/.config/boo/config.yaml`. See [`docs/config.md`](./docs/config.md) for the schema. `boo config` prints the effective config plus where each value came from; `boo config edit` opens it in `$EDITOR`.

## Brand

The papa-and-baby-boos mark, colour palette, and ASCII variants live in [`docs/brand/`](./docs/brand). See [`docs/brand/BRAND.md`](./docs/brand/BRAND.md) for the design intent.

## Requirements

- macOS
- [Ghostty](https://ghostty.org) 1.3+
- Go 1.24+ (only to build from source)

The first time boo controls Ghostty, macOS will prompt for **Automation** permission. `boo doctor` detects and surfaces the common setup issues (missing Ghostty, version mismatch, missing automation permission, missing `$EDITOR`).

## Build

```
make build
./bin/boo doctor
```

See [`docs/testing.md`](./docs/testing.md) for what CI runs and the manual smoke tests run before a release.

## License

TBD.
