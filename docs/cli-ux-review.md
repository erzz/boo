# CLI UX review

Snapshot of the surface as of this review. Captured by walking every
command's `--help`, exercising error paths, and reading the user-facing
strings in `internal/cli` and `internal/picker`.

Two passes: **gaps** (real holes), then **polish** (annoyances and
inconsistencies). Each item has a recommendation and a rough size.

## Gaps

### G1. No way to switch a project's layout after registration

Users can author a layout, can drop it in `~/.config/boo/layouts/`, can
preview it with `boo layouts`, but there's no command to **point an
existing project at a different layout**. They have to either
`boo delete X && boo new X --layout Y` (loses the layout *file's*
hand-edits if any) or hand-edit `~/.local/share/boo/projects/X/layout.yaml`
(needs to know the path).

**Recommend:** `boo set-layout <name> <template>` (or `boo layout set
<name> <template>` if we want a `layout` subcommand verb). Also unblocks
"I made a new template, switch all my projects to it" via shell loop.

Size: **small** (~80 lines, one file).

### G2. No way to edit a project's layout

Today: `vim ~/.local/share/boo/projects/<name>/layout.yaml`. This is a
power-user move that requires knowing the path. We have `boo config
edit` that does the equivalent for global config — apply the same
pattern.

**Recommend:** `boo edit <name>` opens the project's `layout.yaml` in
`$EDITOR`. Validates on save (refuses to write back broken YAML? — too
clever; just let the next `boo <name>` surface the validation error).

Size: **trivial** (~30 lines).

### G3. `boo new --yes` with no flags silently succeeds

Reproducer: from any git repo dir, `boo new --yes` (no `--dir`, no
`--from`) registers the cwd as a project. The help text claims
`--yes ... requires --dir or --from`. It lies — the cwd inspection in
`buildNewProjectDefaults` populates `dir` from `os.Getwd()` and the
auto-detected repo name from the git remote, so `validateIntent`
passes.

This isn't necessarily *wrong* behaviour (it's actually quite useful),
but the help text is misleading and the silent success in scripts is
dangerous. Either: (a) fix the help text, or (b) make `--yes` strict
and require an explicit flag.

**Recommend:** **(a)** — keep the convenience, fix the lie. Update
the flag doc to read `skip the form and register immediately (uses
cwd if --dir/--from omitted)`.

Size: **trivial** (one string).

### G4. `boo fzf` outside a TTY hangs forever

Tested by running `./bin/boo fzf` in a non-interactive shell — process
hangs indefinitely waiting on stdin. Should error fast with the same
message `boo` (bare) gives in that situation.

**Recommend:** detect non-TTY at command entry; print the same "needs a
TTY" error pattern the picker uses.

Size: **small** (~10 lines).

### G5. No `--json` output anywhere

`boo list`, `boo layouts`, `boo config show` are all human-formatted
only. Anyone scripting boo (auto-completion, status bars, integration
with launchers) has to scrape ANSI-coloured tables. The most painful
gap: `boo list --json` for shell completions and `boo config show
--json` for "what's my effective config" tooling.

**Recommend:** add `--json` to `list`, `layouts`, `config show`. Same
flag name on every command. Skip the others (TUI commands have no
useful machine output).

Size: **medium** (~150 lines across 3 files).

### G6. Shell completion is registered but project names aren't completed

`boo completion` exists (cobra default) but `boo <TAB>` won't suggest
project names. Cobra has a `ValidArgsFunction` mechanism. Once present,
`boo de<TAB>` completes to `boo delete`, `boo de<TAB><TAB>` lists
projects.

**Recommend:** add `ValidArgsFunction` on the root, on `delete`, on
`save`. Sources project names from `project.Load(a.Paths)`.

Size: **small** (~50 lines).

### G7. No way to see one project's details

`boo list` is the only inspection command. To see `where is project X
on disk, what layout, when did I last open it, is the window currently
alive`, the user has to `boo list | grep X` and squint.

**Recommend:** `boo show <name>` — print a small block: name, dir,
layout, status, last-launched, path to layout file, path to state file.
Also useful as a debugging primitive ("is boo seeing what I think it
is?").

Size: **small** (~60 lines, follows `boo config show`'s shape).

### G8. `boo save --help` mentions `.toml`, but layouts are now `.yaml`

In `internal/cli/layouts.go` the long help still says `~/.config/boo/layouts/<name>.toml`
and `docs/layouts.md` for "the TOML reference". Stale from the YAML
migration.

**Recommend:** sweep all command help text for "TOML"/`.toml` and fix.

Size: **trivial**.

## Polish

### P1. `boo` (bare) and `boo fzf` and `boo` -inside-project all do similar things

Three picker entry points exist:

- `boo` — built-in Bubble Tea picker
- `boo fzf` — fzf picker
- `boo` inside a project's dir — same as bare `boo` (no shortcut to that
  project's switch)

I think this is **fine** — the picker-first behaviour is intentional
per the docs. But it's worth making the bare-`boo` help mention `boo
fzf` as the alternative, so users discover it.

**Recommend:** one line in `root.go`'s `Long` mentioning `boo fzf`.

Size: **trivial**.

### P2. Inconsistent error formatting

Compare:

- `Error: project "nonesuch" not found. Create it with: boo new nonesuch --dir <path>` — period, suggested fix inline.
- `Error: no projects registered — nothing to delete` — em-dash separator, no period.
- `Error: layout template "doesnotexist" not found (no user template at ..., no built-in)` — parenthetical detail.

All three are *fine*, but they're three different error styles in three
adjacent commands. Not painful, but a future user-facing string review
should converge on one shape.

**Recommend:** defer until there's enough surface to feel the
inconsistency. Note it in AGENTS for now.

Size: **defer**.

### P3. `boo doctor` output is dense and uncoloured

Each result is one line `[STATUS] name — detail`. No colour, no
spacing between groups. Compare `brew doctor` or `gh auth status` —
clear visual blocks.

**Recommend:** group results visually (blank line between platform /
ghostty / fzf / config), colour the status badge (green OK, yellow
WARN/SKIP, red FAIL). Lipgloss is already a dep.

Size: **small** (~30 lines).

### P4. `boo list` is just a table — no hint about what to do next

After `boo list` shows the projects, there's no footer telling the
user `run 'boo <name>' to switch`, `boo save <name>` to capture, etc.
First-run users don't know `boo` (bare) opens the picker either.

**Recommend:** when stdout is a TTY, append a faint one-line footer:
`Run 'boo <name>' to switch · 'boo' (no args) for picker`.

Size: **trivial**.

### P5. The picker's `+ New project` row says "press enter to register a project"

Fine, but the picker also has `n`/`+` keybinds that go to the same
place — not surfaced. The help bar at the bottom of the list shows
them, but it competes with bubbles' default keybind hints.

**Recommend:** the existing `AdditionalShortHelpKeys = shortKeys`
already exposes `n`. This is fine — just noting that the secondary
text on the row is a missed opportunity to say `or press 'n'`.

Size: **trivial / defer**.

### P6. The "this directory is already registered" interstitial is good — but only used by `boo new` and `boo save`'s form-fallback

`boo new` inside a dir already registered → interstitial offers
`[s/enter] switch  [c] continue  [esc] cancel`. That's nice. But this
is the only place in the TUI with that affordance — every other
prompt is a free-text input or list.

Not really a gap — just noting that affordance density across the TUI
is uneven. Real fix is the upcoming TUI polish pass.

Size: **defer to TUI pass**.

### P7. `boo new`'s `--into` / `--dir` overlap is confusing

`--dir` means "register this existing dir" UNLESS `--from` is also set,
in which case it means "clone target". `--into` means "clone target",
period. The help text explains it but it's still two flags with
overlapping semantics.

**Recommend:** keep both for backward compat, but make `--into` take
precedence over `--dir` (currently `--dir` wins per `buildNewProjectDefaults`).
And drop `--into` from the next breaking-change release.

Actually re-reading: the current order is `--dir` first, then fall
back to `--into`. So `--dir` wins. That matches the help. **No
change.** Just noting it as something to revisit at v1.

Size: **defer**.

### P8. No `--quiet` / `-q`

Scripts that wrap boo can't suppress the `Registered "X" at ... (layout: triple)`
chatty output. Not a v1 problem, but a flag worth adding once the
surface stabilises.

**Recommend:** defer.

Size: **defer**.

### P9. `boo doctor` doesn't mention config validity in its error suggestion

When config is malformed, doctor reports it but the FAIL line is just
the error. The hint mentions `boo config edit`. Good. But a fresh
user might not connect "config is broken" to "every other boo command
will now fail until I fix it". The current FAIL → exit 1 already
forces attention, so this is minor.

Size: **defer**.

### P10. `boo save`'s long help is excellent but very long

Five paragraphs explaining shape diff, lossy diff, the no-args inference,
etc. Probably the right amount of info but it's a wall of text on
`--help`. Consider moving the deep explanation to `docs/save.md` and
keeping `--help` to a short "what happens" + "see docs/save.md".

**Recommend:** defer until docs/save.md exists.

Size: **defer**.

## Suggested order

If you want to fix things now, in this order:

1. **G3** (one-line lie in help) — 30 seconds.
2. **G8** (TOML→YAML in help text) — 2 minutes.
3. **G4** (fzf hangs without TTY) — 10 minutes.
4. **G2** (`boo edit <name>`) — 30 minutes.
5. **G1** (`boo set-layout`) — 1 hour.
6. **G7** (`boo show <name>`) — 1 hour.
7. **G6** (shell completion of project names) — 1 hour.
8. **G5** (`--json` outputs) — 2 hours.
9. **P3** (doctor visual polish) — fold into the upcoming TUI/theming
   pass.
10. **P4** (list footer) — fold into the upcoming TUI/theming pass.

Items 1–8 are pure CLI; 9–10 want the theming work first so we don't
bake colours that we then re-do.

## Things I deliberately did NOT recommend

- A `boo rename` command — possible but the registry uses name as primary
  key with state files keyed by name. Implementable but not free.
- A `boo open <name>` alias for `boo <name>` — the bare-name verb is
  already the primary use case; an alias just adds a second name for
  the same thing.
- `boo init` for "create a config + first project at once" — too cute;
  separate operations are clearer.
- A `boo update` self-update — outside scope; let homebrew/goreleaser
  handle.
