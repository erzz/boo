# Testing

## What CI runs

[`.github/workflows/ci.yml`](../.github/workflows/ci.yml) runs on every push to
`main` and every pull request, on `macos-latest` with Go 1.24:

- `go build ./...`
- `go test ./...` (unit tests only)
- `go vet ./...`

Integration tests (`-tags=integration`) are **not** run in CI — they require a
running Ghostty plus macOS Automation permission, neither of which a CI runner
has.

## What `make test-int` does

Runs the unit tests plus the integration tests in `internal/ghostty` and
`internal/cli` that drive a real Ghostty via JXA. Required:

- Ghostty installed at `/Applications/Ghostty.app`.
- Automation permission granted to your terminal (first run will prompt).
- **Not run from inside a Ghostty window** — the suite refuses to launch from
  one without `BOO_ALLOW_GHOSTTY_INTEGRATION=1`. Use iTerm or Terminal.app.

## Manual smoke tests

Run before tagging a release, after touching any command surface, or whenever
Ghostty itself updates. Use a throwaway state dir so you don't clobber your
real projects:

```sh
export BOO_HOME=/tmp/boo-smoke-$$
mkdir -p "$BOO_HOME"
trap 'rm -rf "$BOO_HOME"; unset BOO_HOME' EXIT
make build
```

The minimum useful set:

1. **Doctor.** `./bin/boo doctor` — all checks OK (or `fzf` SKIP if not
   installed). Quit Ghostty and re-run; `ghostty running` should be WARN and
   `automation permission` SKIP, no crash.

2. **New project, default layout.** `./bin/boo new alpha --dir ~/some/existing
   --yes` opens a Ghostty window with the `triple` layout (1 large left pane +
   2 stacked right). `./bin/boo alpha` again focuses the same window.

3. **Layout cycler.** `./bin/boo new` (no args) inside a non-registered dir
   opens the form with `triple` preselected; ←/→ or h/l cycles the layout
   field, and the ASCII preview updates as you cycle.

4. **Layouts command.** `./bin/boo layouts` lists all 8 built-ins (`1x1x1`
   through `triple`) with previews. Drop a `~/.config/boo/layouts/scratch.yaml`
   with a leading `# description` comment; it appears tagged `[user]`. Drop a
   syntactically broken `broken.yaml`; it appears tagged `[error]` without
   crashing the listing. Clean up both when done.

5. **Save round-trip (lossless).** Open a project's window, then
   `./bin/boo save alpha` — first save is silent, prints "Captured N tab(s),
   M pane(s)". Run it again immediately with no changes — silent re-save, no
   prompt. Inspect `$BOO_HOME/.../projects/alpha/layout.yaml`: tree shape and
   pane count match what's open.

6. **Save with structural change.** Add a tab in Ghostty, `./bin/boo save
   alpha` — ASCII diff renders, prompt asks to apply. `n` aborts cleanly.

7. **Save with lossy change.** Hand-edit the saved layout to add a `command:`
   on a leaf, close that pane in Ghostty, then `./bin/boo save alpha`. Diff
   marks the affected leaf with `!`, prompt warns about losing the command,
   `--force` skips the prompt but still prints to stderr.

8. **Bare `boo` lands on the list.** `cd` into alpha's dir on disk, run
   `./bin/boo` (no args). The picker opens on the project list — not on the
   "this directory is already registered" interstitial.

9. **Cold start.** Quit Ghostty entirely, `./bin/boo alpha`. Cold-starts
   Ghostty via `open -na`, then opens the window with the layout. No stale-ID
   errors.

If any of these regress: capture the exact command, output, and `boo doctor`
results, then file an issue.
