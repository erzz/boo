# Layouts

A **layout** describes the shape of a project's Ghostty window: how many
tabs, how each tab is split, what working directory each split starts in,
and (optionally) what command and environment each split launches with.

Layouts are TOML files. boo ships a few built-in ones to get you going,
and you can drop your own into `~/.config/boo/layouts/` to use across
projects.

> **Tip.** Run `boo layouts` to see every layout currently available
> (built-in plus your own) with an ASCII preview of each.

This document covers:

- The TOML vocabulary (one short reference)
- Built-in layouts and what each is for
- Examples you can copy into `~/.config/boo/layouts/` for common stacks

## Vocabulary

```toml
name = "my-layout"      # Required. Used by `boo new --layout my-layout`.

[[tab]]
name = "edit"           # Optional. Shows in the tab title.

  [[tab.split]]
  cwd = "."             # Working directory. "." = project root.
                        # Relative paths resolve under the project dir.
                        # Absolute paths are kept as-is.
  command = "nvim ."    # Optional. Runs after the shell starts.
  initial_input = "..." # Optional. Sent to the shell as keystrokes.

  [env]                 # Optional per-split env vars.
  EDITOR = "nvim"

  [[tab.split]]
  direction = "right"   # "right" | "down". Required for splits 2+.
                        # First split per tab MUST omit direction.
  cwd = "."
```

### Rules the validator enforces

- At least one tab per layout.
- At least one split per tab.
- The first split in a tab must NOT have a `direction`.
- Splits 2+ must have `direction = "right"` or `"down"`.

### What `boo save` can and can't see

`boo save` reads back the live Ghostty window via AppleScript. The API
returns a flat list of terminals per tab and exposes only `id`, `title`,
and `working directory`. That means saving from a live window:

- **Captures correctly**: number of tabs, number of terminals per tab,
  each terminal's cwd.
- **Loses on capture**: split nesting/direction, the launch `command`,
  `env`, and `initial_input`.

`boo save` mitigates this by **merging** invisible fields from the
previously-saved layout into the captured shape (by position) — so a
re-save with the same shape preserves your commands and env. But:

- A tree like *vertical → [horizontal, horizontal]* is captured as a
  flat row of three splits.
- Closing a split that held a `command` permanently drops that command.

If your layout depends on a specific tree shape or specific commands,
**hand-author the layout file** and don't rely on `boo save` to
regenerate it.

## Built-in layouts

These are embedded in the binary. Use them with `boo new --layout <name>`.
None of them assume any particular tooling — they work on a clean
machine.

### `default`

One tab, one shell at the project root.

```toml
name = "default"

[[tab]]
name = "shell"
  [[tab.split]]
  cwd = "."
```

Used when `--layout` is omitted.

### `dev`

Two tabs: a place to edit, a place to run.

```toml
name = "dev"

[[tab]]
name = "edit"
  [[tab.split]]
  cwd = "."

[[tab]]
name = "run"
  [[tab.split]]
  cwd = "."
  [[tab.split]]
  direction = "right"
  cwd = "."
```

The `edit` tab is intentionally a bare shell — open whatever editor you
prefer. The `run` tab is two side-by-side shells (e.g. server in one,
test runner in the other).

### `triple`

Single tab, 1 large pane on the left, 2 stacked on the right.

```toml
name = "triple"

[[tab]]
name = "main"
  [[tab.split]]
  cwd = "."
  [[tab.split]]
  direction = "right"
  cwd = "."
  [[tab.split]]
  direction = "down"
  cwd = "."
```

This is also the canonical example of a layout `boo save` cannot recapture
(see "What `boo save` can and can't see" above).

## Custom layouts

Drop a `<name>.toml` into `~/.config/boo/layouts/` and use it with
`boo new --layout <name>`. User templates always shadow built-ins of the
same name — so you can override `default` with your own.

The examples below assume the named tools are installed; copy and adapt
to taste.

### Node / web (vite, next, etc.)

```toml
name = "web"

[[tab]]
name = "edit"
  [[tab.split]]
  cwd = "."
  command = "code ."

[[tab]]
name = "dev"
  [[tab.split]]
  cwd = "."
  command = "npm run dev"
  [[tab.split]]
  direction = "right"
  cwd = "."

[[tab]]
name = "test"
  [[tab.split]]
  cwd = "."
  command = "npm test -- --watch"
```

### Go service with logs

```toml
name = "go-service"

[[tab]]
name = "main"
  [[tab.split]]
  cwd = "."
  [[tab.split]]
  direction = "right"
  cwd = "."
  command = "make run"
  [[tab.split]]
  direction = "down"
  cwd = "."
  command = "tail -f /tmp/service.log"
```

Demonstrates the 1+2 shape with two of the side panes pre-running long-
lived processes.

### Rust crate

```toml
name = "rust"

[[tab]]
name = "edit"
  [[tab.split]]
  cwd = "."

[[tab]]
name = "watch"
  [[tab.split]]
  cwd = "."
  command = "cargo watch -x check -x test"
```

### Python with venv

```toml
name = "py"

[[tab]]
name = "main"
  [[tab.split]]
  cwd = "."
  initial_input = "source .venv/bin/activate\n"
  [[tab.split]]
  direction = "right"
  cwd = "."
  initial_input = "source .venv/bin/activate\n"
  command = "pytest --watch"
```

`initial_input` is sent to the shell as keystrokes after launch. Useful
for activating venvs or sourcing per-shell setup that you don't want to
put in your global rc.

### Per-split env

```toml
name = "staging"

[[tab]]
name = "main"
  [[tab.split]]
  cwd = "."
  command = "npm run dev"
  [tab.split.env]
  NODE_ENV = "staging"
  API_URL = "https://staging.example.com"
```

Per-split env merges with your shell's env; it does not replace it.

## Tips

- **`boo layouts`** lists every available template (built-in + your
  user templates in `~/.config/boo/layouts/`) with its description and
  an ASCII preview of the resulting window. Use it to discover what's
  available before passing `--layout` to `boo new`. The new-project
  TUI form (`boo new` with no `--yes`, or bare `boo` → "+ New project")
  shows the same preview live as you type a template name.
- **`boo new --layout <name>`** picks the layout for a new project, but
  the project remembers it. Subsequent `boo <project>` calls reuse the
  saved layout file in `~/.local/share/boo/projects/<name>/layout.toml`.
- **To change a project's layout**, edit its `layout.toml` directly. Or
  re-save from a rearranged window with `boo save` — but mind the
  flatten-on-capture caveat above.
- **To share a layout across projects**, put it in `~/.config/boo/layouts/`
  and reference it with `--layout`. Each project still gets its own copy
  on creation.
- **`boo` doesn't prescribe an editor.** None of the built-in layouts
  launch one. Add `command = "code ."` (or your preferred editor) to a
  custom layout if you want one to open automatically.
