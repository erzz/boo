# Configuration

boo reads optional global config from `~/.config/boo/config.yaml`
(or `$XDG_CONFIG_HOME/boo/config.yaml`, or `$BOO_HOME/config/config.yaml`).
The file is **optional** — if absent, boo runs entirely on factory
defaults.

Use `boo config edit` to open the file in `$EDITOR` (it's created if
missing). Use `boo config show` to see the effective values and where
each one came from. Use `boo config path` to print the file path.

## Schema

All keys are optional. Only set what you want to change.

```yaml
# Layout used for new projects when --layout is omitted, and the layout
# preselected in the new-project form. Must be the name of a built-in
# layout or a user template in ~/.config/boo/layouts/.
# Factory default: triple
default_layout: triple

# Parent directory `boo new --from <url>` clones into when --into isn't
# given. Tilde is expanded to $HOME.
# Factory default: unset (clone goes into <cwd>/<repo-name>)
projects_dir: ~/code

git:
  # If set, the new-project form's "Clone from URL" field expands a bare
  # repo name into "<default_remote>/<name>". A full URL or any input
  # containing "/" or ":" is left alone. Trailing slashes on the value
  # are stripped.
  # Factory default: unset (no expansion)
  default_remote: https://github.com/erzz

ui:
  # Named visual theme for the TUI. Built-in: "default" (Ghostty
  # palette). User themes live in ~/.config/boo/themes/<name>.yaml
  # and shadow built-ins of the same name. See docs/themes.md for
  # the schema and `boo themes` to list what's available.
  # Factory default: default
  theme: default
```

## How values combine

For each key, boo uses the first non-empty source in this order:

1. CLI flag (e.g. `--layout`)
2. `~/.config/boo/config.yaml`
3. Factory default

There is no env-var layer and no per-project config. Per-project
customisation belongs in the project's own `layout.yaml`.

## Examples

### "I always use a 4-pane grid"

```yaml
default_layout: 1x2x2
```

`boo new myproj --dir ~/x --yes` (no `--layout`) now creates `myproj`
with the `1x2x2` layout. The new-project form preselects `1x2x2` in
the cycler.

### "I clone everything into ~/code"

```yaml
projects_dir: ~/code
```

`boo new myrepo --from https://github.com/owner/myrepo` clones into
`~/code/myrepo` instead of `<cwd>/myrepo`.

### "I always clone from my own GitHub"

```yaml
git:
  default_remote: https://github.com/erzz
```

In the new-project form, type `boo` in the "Clone from URL" field and
submit — boo expands it to `https://github.com/erzz/boo` and clones
that. A full URL still works as before.

### Combined

```yaml
default_layout: triple
projects_dir: ~/code
git:
  default_remote: https://github.com/erzz
```

Most common setup: pick your layout, pick where clones land, pick
your default remote. Three keys.

## Validation

A missing config file is fine. A malformed config file (broken YAML
syntax, wrong types) is a hard error: `boo doctor` shows it as FAIL,
and any `boo` command that needs the config will refuse to run until
it's fixed. This is intentional — silently falling back to factory
defaults would mask user mistakes (typos in keys especially).

If you suspect your config is wrong:

```sh
boo doctor          # config check at the bottom
boo config show     # what boo actually loaded
boo config edit     # fix it in $EDITOR
```
