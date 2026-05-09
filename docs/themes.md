# Themes

A **theme** is a small palette — six colour slots — that the boo TUI
uses to render itself. Themes are pure cosmetics: changing one swaps
the visual style without affecting any behaviour. A typo in a theme
file produces dim text, never a crash or a refusal to launch.

> **What a theme does and doesn't do.** A boo theme only colours
> boo's own UI elements: borders, accent text, status pills, the
> brand strip. It does **not** change your terminal background, the
> ANSI palette, or anything else outside boo's window. That's
> Ghostty's job — set a Ghostty colour scheme separately. Pick a boo
> theme that coordinates with your Ghostty scheme: `light` and
> `solarized-light` for light backgrounds; `default`, `tokyonight`,
> and `solarized-dark` for dark ones.

boo ships several built-in themes (`default`, `tokyonight`,
`solarized-dark`, `solarized-light`, `light`) and you can drop your
own into `~/.config/boo/themes/` to use across every boo session.

> **Tip.** Run `boo themes` to see every theme currently available
> (built-in plus your own) with a colour swatch and the active theme
> marked.

## How to switch themes

Set `ui.theme` in `~/.config/boo/config.yaml`:

```yaml
ui:
  theme: <name>
```

`<name>` is whatever you'd see in the first column of `boo themes`.
Restart any open boo TUI to see the change. (boo doesn't watch the
config file — themes load once at startup.)

If `ui.theme` is unset, empty, or names a theme that doesn't exist or
won't parse, boo falls back to the built-in `default` theme silently.
Run `boo doctor` to surface broken themes.

## Built-in themes

| Name | When to use |
|---|---|
| `default` | Ghostty purple. The boo brand palette. **Default.** |
| `tokyonight` | Tokyo Night Storm — cool blue-purple on dark. Pairs with the Tokyo Night Ghostty colour scheme. |
| `solarized-dark` | Ethan Schoonover's canonical Solarized Dark palette. |
| `solarized-light` | Solarized Light — for light terminal backgrounds. |
| `light` | Ghostty purple, tuned for light backgrounds. Use this if you like the boo identity but run a light terminal. |

Activate any of them with `ui.theme: <name>` in your config. Run
`boo themes` to see them all with colour swatches and the active one
marked.

## Schema

A theme is YAML with three fields. Only `colors` is required.

```yaml
name: my-theme
description: A short, one-line summary shown by `boo themes`.
colors:
  accent: "#A594FF"   # selection, focus, list/form titles, the cursor
  info: "#5B5BD6"     # "+ New project" row, brand highlights
  border: "#5B5BD6"   # both pane borders (list + right preview)
  ok: "10"            # "● running" status pill, success outcomes
  warn: "9"           # "✖ dir missing", error states
  stopped: "8"        # "○ stopped" status pill, neutral foreground
```

### Colour values

Each slot accepts any lipgloss-compatible colour string:

| Format | Example | Notes |
|---|---|---|
| ANSI 16-color index | `"3"`, `"12"` | `0`–`15`. Respects your terminal's palette — best for portable themes. |
| ANSI 256-color index | `"196"`, `"242"` | `16`–`255`. Standard xterm-256 palette. |
| Truecolor hex | `"#5B5BD6"` | 24-bit. Best fidelity in modern terminals (Ghostty included). |

Lipgloss tolerates garbage colour strings — a typo renders as your
terminal's default foreground rather than crashing — so experimentation
is cheap.

### Slot reference

Six slots, mapped to roles in the picker:

| Slot | Where it shows |
|---|---|
| `accent` | Selected list item, the `ᗣ ` cursor, list/form titles, right-pane project name, list pane title. |
| `info` | The `+ New project` row, prompt-like brand highlights. |
| `border` | Both pane borders — left list pane and right preview pane. |
| `ok` | `● running` status pill, status-bar success outcome (`✓ deleted alpha`). |
| `warn` | `✖ dir missing`, validation errors, status-bar failure outcome. |
| `stopped` | `○ stopped` status pill, neutral / faint metadata. |

### Partial themes

You can override only the slots you care about — missing slots fall
back to the built-in `default` theme's value for that slot. So a theme
that only changes the accent colour is just:

```yaml
name: minimal-purple
description: Default theme with a brighter accent.
colors:
  accent: "#FF66FF"
```

Everything else (info, border, ok, warn, stopped) stays the default.
This makes incremental customisation easy.

### `name:` and `description:`

- `name:` is what users type in `ui.theme: <name>`. If omitted, boo
  uses the filename stem (`my-theme.yaml` → `my-theme`).
- `description:` is a one-line summary shown in `boo themes`. Optional;
  themes without it just show name + path.

## Creating a theme

The fastest way: seed from the built-in default and edit.

```sh
boo themes init my-theme
```

This writes `~/.config/boo/themes/my-theme.yaml` containing the
default theme's full palette, with `name:` set to `my-theme`. Edit the
colour values, save, then activate it:

```yaml
# ~/.config/boo/config.yaml
ui:
  theme: my-theme
```

`boo themes init` refuses to overwrite an existing file. Use
`--force` to replace one. Use `--from <theme>` to seed from a
different built-in (when more built-ins ship later).

Alternatively, write the file yourself — it's just YAML — and drop it
into `~/.config/boo/themes/`. boo picks it up on next launch.

## User themes shadow built-ins

If you create `~/.config/boo/themes/default.yaml`, it shadows the
built-in `default` for the rest of boo. The original is still embedded
in the binary; remove your file to revert. `boo themes` marks shadowed
entries as `[user]` so it's obvious which version is active.

## Sharing themes

Themes are single self-contained YAML files — copy them between
machines, paste them into gists, commit them to a dotfiles repo. There
are no external dependencies.

## Examples

### Catppuccin Mocha–ish

```yaml
# ~/.config/boo/themes/mocha.yaml
name: mocha
description: Inspired by Catppuccin Mocha.
colors:
  accent: "#cba6f7"   # mauve
  info: "#89b4fa"     # blue
  border: "#585b70"   # surface2
  ok: "#a6e3a1"       # green
  warn: "#f38ba8"     # red
  stopped: "#6c7086"  # overlay0
```

### High-contrast monochrome

```yaml
# ~/.config/boo/themes/mono.yaml
name: mono
description: Black and white only — for screenshots and accessibility.
colors:
  accent: "15"        # bright white
  info: "15"
  border: "7"         # white
  ok: "15"
  warn: "15"
  stopped: "8"        # bright black (grey)
```

### Gruvbox Dark

```yaml
# ~/.config/boo/themes/gruvbox.yaml
name: gruvbox
description: Gruvbox Dark accent colours.
colors:
  accent: "#fabd2f"   # bright yellow
  info: "#83a598"     # bright blue
  border: "#665c54"   # bg3
  ok: "#b8bb26"       # bright green
  warn: "#fb4934"     # bright red
  stopped: "#928374"  # gray
```

For more, peek at the built-ins under
`internal/theme/themes/*.yaml` in the boo source — they're the
same six-slot YAML files you'd write yourself.

## Inspecting and managing themes

| Command | What it does |
|---|---|
| `boo themes` | List every available theme with a colour swatch and active marker |
| `boo themes list --json` | Same listing, machine-readable, no ANSI |
| `boo themes show <name>` | Print the raw YAML for a theme (user version preferred) |
| `boo themes path` | Print the user themes directory |
| `boo themes init <name>` | Write a starter theme file you can edit |
| `boo doctor` | Includes a check that flags broken user themes (WARN, never FAIL) |

## Tips

- **Themes are loaded at boo's startup.** Edit a theme, then re-launch
  the TUI to see the change.
- **Fall back deliberately.** If you want to disable a custom theme
  temporarily, comment out `ui.theme` in `config.yaml` rather than
  deleting your theme file. Empty / unset = built-in default.
- **boo never auto-creates `~/.config/boo/themes/`.** It only appears
  when you explicitly run `boo themes init <name>` or create the
  directory yourself. Until then boo runs entirely on built-ins.
- **Share favourite themes** as standalone gists. Since the schema is
  six values, the entire theme fits in a tweet.
