# Layouts

A **layout** describes the shape of a project's Ghostty window: how many
tabs, how each tab is recursively split, what working directory each pane
starts in, and (optionally) what command and environment each pane
launches with.

Layouts are YAML files. boo ships eight built-in layouts and you can drop
your own into `~/.config/boo/layouts/` to use across projects.

> **Tip.** Run `boo layouts` to see every layout currently available
> (built-in plus your own) with an ASCII preview of each.

## Vocabulary

A layout is a list of tabs. Each tab has a root split, written as the
`split:` field in YAML. A split is
either a **leaf** (a single shell pane) or an **interior** node (a
horizontal or vertical division of two child splits).

```yaml
name: my-layout
tabs:
  - name: edit                # Optional. Round-tripped but ignored on open
                              # — see "Tab names" below.
    split:
      cwd: .                  # Leaf: a single shell pane.
                              # "." resolves to the project root.
                              # Relative paths resolve under the project dir;
                              # absolute paths are kept as-is.
      command: nvim .         # Optional. Typed into the default shell and
                              # executed (followed by Enter). Your shell's
                              # rc files run normally and PATH is intact.
                              # When the command exits you're left at a
                              # normal shell prompt — the pane does not close.
      initial_input: ""       # Optional. Sent to the shell as keystrokes
                              # WITHOUT a trailing Enter, so you can review
                              # or edit before pressing return yourself.
      env:                    # Optional per-pane env vars.
        EDITOR: nvim

  - name: run
    split:
      direction: row          # Interior: split horizontally (left | right).
                              # "column" splits vertically (top | bottom).
      size: 0.4               # Optional. First child's fractional share of
                              # this split's extent (0,1) — here the left
                              # pane gets 40%, the right 60%. Omit (or set
                              # to 0) for an even split. Honored by both
                              # the preview renderer and the live window.
      children:
        - cwd: .              # Left child (leaf).
        - direction: column   # Right child (interior).
          children:
            - cwd: .          # Top.
              command: make run
            - cwd: .          # Bottom.
              command: tail -f /tmp/service.log
```

### Rules the validator enforces

- At least one tab per layout.
- Every tab has a `root` split.
- A split is a leaf XOR an interior node — never both.
- Interior nodes have **exactly 2 children**, no more, no fewer. Ghostty
  splits halve the focused pane; three children would yield 50/25/25,
  not equal thirds. To get N panes, nest interiors.
- Interior `direction` is `row` (left/right) or `column` (top/bottom).
- Direction never appears on a leaf.
- Optional `size` on an interior node is a float strictly between 0 and
  1 — the first child's fractional share. Omit (or set to 0) for an
  even split. Never appears on a leaf.

### Responsive variants

Layouts can also switch shape based on terminal width using `variants:`.
Selection is based on terminal columns only.

```yaml
name: responsive-dev
variants:
  - tabs:                  # Default variant. Required fallback.
      - name: compact
        split:
          direction: column
          children:
            - cwd: .
            - cwd: .
              command: npm run dev

  - min_cols: 140
    tabs:
      - name: wide
        split:
          direction: row
          children:
            - cwd: .
            - direction: column
              children:
                - cwd: .
                  command: npm run dev
                - cwd: .
                  command: npm test -- --watch
```

Rules:

- A layout uses either `tabs:` or `variants:`. Never both.
- A responsive layout must declare exactly one default variant: the one
  with no `min_cols` or `max_cols`.
- `min_cols` and `max_cols` are inclusive when set.
- Variants are checked in file order. First matching non-default variant
  wins. If boo cannot determine terminal width, it falls back to the
  default variant.

Current limits:

- `boo <project>` uses responsive selection when opening project windows.
- `boo layouts` and picker previews render the default variant.
- `boo save` rejects responsive layouts instead of flattening one variant
  back into the file.
- Opening a project's `layout.yaml` in the picker/editor and the new-project
  layout editor do not support responsive layouts yet.

### Tab names

The `name:` field on a tab is parsed and round-tripped by `boo save`,
but **ignored when boo opens the window**. Ghostty 1.3.x marks tab
titles as read-only via AppleScript and offers no non-interactive way
to set them. The field is kept in the schema so it survives re-saves;
it'll start working once Ghostty exposes a writable tab title.

### What `boo save` can and can't see

`boo save` reads the live Ghostty window via AppleScript. The API
returns a flat per-tab terminal list with only `id`, `title`, and
`working directory`. That means saving from a live window:

- **Captures correctly**: number of tabs, number of panes per tab,
  each pane's cwd.
- **Loses on capture**: split tree shape, the launch `command`, `env`,
  and `initial_input`.

`boo save` mitigates this by **merging** invisible fields from the
previously-saved layout into the captured shape:

- If the captured pane count for a tab matches the previously-saved
  tab's pane count, boo walks the prior tree preserving its shape and
  zips in the captured cwds. Re-saves of hand-authored layouts are
  lossless.
- If the pane count differs (you closed or opened panes), boo falls
  back to a right-leaning row chain — matches what Ghostty's default
  Cmd-D N times produces and avoids silently changing pane proportions.

If your layout depends on a specific tree shape or specific commands,
**hand-author the layout file** and don't rely on `boo save` to
regenerate it from a window where panes have come and gone.

## Built-in layouts

These are embedded in the binary. Use them with `boo new --layout <name>`,
or pick them in the new-project form's layout cycler (←/→ or h/l). All
are tool-agnostic — they just open shells.

| Name | Shape |
|---|---|
| `1x1x1` | One tab, one pane. |
| `1x2x1` | One tab, two side-by-side panes. |
| `1x1x2` | One tab, two stacked panes. |
| `1x2x2` | One tab, 2×2 grid (4 panes). |
| `2x1x1` | Two tabs, each with one pane. |
| `2x2x1` | Two tabs, each two side-by-side. |
| `2x2x2` | Two tabs, each 2×2 (8 panes total). |
| `triple` | One tab, 1 large left pane + 2 stacked on the right. **Default.** |

`triple` is the layout you get when `--layout` is omitted and the layout
the new-project form preselects.

Run `boo layouts` to see each one rendered as ASCII.

## Custom layouts

Drop a `<name>.yaml` (or `.yml`) into `~/.config/boo/layouts/` and use
it with `boo new --layout <name>`. User templates always shadow built-ins
of the same name — so you can override `triple` with your own.

The first leading `# ...` comment block in the file is used as the
description shown by `boo layouts`.

The examples below assume the named tools are installed; copy and adapt
to taste.

### Node / web (vite, next, etc.)

```yaml
# Three-tab Node web project: editor, dev server + scratch shell, test watcher.
name: web
tabs:
  - name: edit
    root:
      cwd: .
      command: code .

  - name: dev
    root:
      direction: row
      children:
        - cwd: .
          command: npm run dev
        - cwd: .

  - name: test
    root:
      cwd: .
      command: npm test -- --watch
```

### Go service with logs

```yaml
# Single tab: shell on the left, server top-right, log tail bottom-right.
name: go-service
tabs:
  - name: main
    root:
      direction: row
      children:
        - cwd: .
        - direction: column
          children:
            - cwd: .
              command: make run
            - cwd: .
              command: tail -f /tmp/service.log
```

### Rust crate

```yaml
# Two tabs: editor + cargo-watch.
name: rust
tabs:
  - name: edit
    root:
      cwd: .

  - name: watch
    root:
      cwd: .
      command: cargo watch -x check -x test
```

### Python with venv

```yaml
# One tab, two panes; both activate .venv on launch, right pane runs pytest.
name: py
tabs:
  - name: main
    root:
      direction: row
      children:
        - cwd: .
          initial_input: "source .venv/bin/activate\n"
        - cwd: .
          initial_input: "source .venv/bin/activate\n"
          command: pytest --watch
```

`initial_input` is sent to the shell as keystrokes after launch, with no
trailing Enter — useful for activating venvs or sourcing per-shell setup
that you don't want to put in your global rc, where you might want to
review the command before running it.

### `command` vs `initial_input`

Both fields end up as keystrokes typed into your default shell after it
starts (login shell, full PATH, rc files run normally). The only
difference is the trailing newline:

- `command: foo` → types `foo\n` (executes immediately).
- `initial_input: "foo"` → types `foo` (waits for you to press Enter).
- Both set → types `command\n` then `initial_input` (handy when the
  command launches a REPL or interactive program you want to seed).

When a `command` exits, the pane returns to a normal shell prompt — it
does not close. If you want the surface to die with the command, exit
the shell yourself or run `exec foo` instead.

### Per-pane env

```yaml
# Single pane with staging env baked in.
name: staging
tabs:
  - name: main
    root:
      cwd: .
      command: npm run dev
      env:
        NODE_ENV: staging
        API_URL: https://staging.example.com
```

Per-pane env merges with your shell's env; it does not replace it.

## Tips

- **`boo layouts`** lists every available template (built-in + your
  user templates in `~/.config/boo/layouts/`) with its description and
  an ASCII preview of the resulting window. The new-project form
  (`boo new` with no `--yes`, or bare `boo` → "+ New project") shows
  the same preview live as you cycle through templates with ←/→ or h/l.
- **`boo new --layout <name>`** picks the layout for a new project, but
  the project remembers it. Subsequent `boo <project>` calls reuse the
  saved layout file in `~/.config/boo/projects/<name>/layout.yaml`.
- **To change a project's layout**, edit its `layout.yaml` directly. Or
  re-save from a rearranged window with `boo save` — but mind the
  capture caveats above.
- **To share a layout across projects**, put it in `~/.config/boo/layouts/`
  and reference it with `--layout`. Each project still gets its own copy
  on creation.
- **boo doesn't prescribe an editor.** None of the built-in layouts
  launch one. Add `command: code .` (or your preferred editor) to a
  custom layout if you want one to open automatically.
