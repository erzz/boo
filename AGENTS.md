# Project: boo

Project launcher for the Ghostty terminal emulator on macOS. Go CLI.

## Stack

Go 1.24+, cobra (CLI), Bubble Tea + Lip Gloss + Bubbles (TUI), pelletier/go-toml v2 (config), `osascript -l JavaScript` (JXA) for Ghostty control. macOS only for v1.

## Layout

- `cmd/boo/` — main entrypoint, wires cobra
- `internal/cli/` — cobra command definitions
- `internal/ghostty/` — JXA generation and osascript runner; the only place that talks to Ghostty
- `internal/layout/` — layout model, TOML parsing, validation
- `internal/project/` — project model and registry CRUD
- `internal/state/` — XDG paths, atomic file IO
- `internal/config/` — global config loader
- `internal/picker/` — Bubble Tea TUI
- `internal/doctor/` — environment checks
- `internal/exec/` — `Runner` interface; production wraps `os/exec`, tests use fake
- `assets/jxa/` — JXA script templates (embedded with `go:embed`)
- `assets/layouts/` — bundled default layout templates

## How to run / test / build

```
make build       # builds ./bin/boo
make test        # unit tests
make test-int    # integration tests (-tags=integration; requires Ghostty installed)
make lint        # golangci-lint
make fmt         # gofmt + goimports
./bin/boo doctor # env sanity check
```

## Conventions

- Every shell-out goes through `internal/exec.Runner`. Never call `os/exec` directly outside that package — it breaks tests.
- All Ghostty interaction lives in `internal/ghostty`. No JXA strings or `osascript` calls anywhere else.
- TOML is the only config format. JSON is internal-only (state files, JXA stdin/stdout).
- Errors are wrapped with `fmt.Errorf("...: %w", err)`; user-facing errors come from a small set of helpers in `internal/cli` so messages stay consistent.
- `slog` for logging; `--verbose` global flag flips level to debug. No `fmt.Println` for diagnostics.
- Layout files: TOML in boo's own vocabulary (windows/tabs/splits). Never expose raw JXA or AppleScript to users.

## Entry points

- New command: `internal/cli/<verb>.go`, register in `internal/cli/root.go`.
- New Ghostty capability: extend `internal/ghostty.Client` interface and the JXA template in `assets/jxa/`.
- New layout feature: update `internal/layout/types.go`, the TOML parsing, and the JXA template in lockstep.
- Doctor checks: add to `internal/doctor/checks.go`.

## Gotchas

- `ghostty +new-window` is **not supported on macOS** (`"not supported on this platform"`). Use AppleScript via `osascript -l JavaScript`. Use `open -na Ghostty.app` only for cold-start.
- macOS will prompt for Automation permission the first time boo controls Ghostty. `boo doctor` should detect and surface this — see `internal/doctor`.
- Ghostty window/tab/terminal IDs are stable only within a single Ghostty process lifetime. Don't persist them across reboots — regenerate on cold start.
- JXA escaping is treacherous. Always build the parameter object as JSON in Go, then embed it into the JXA script as a single `JSON.parse(...)` call. Never string-concatenate user values into JS source.
- Ghostty is pre-2.0 and the AppleScript API surface may change. Pin a tested version range in `doctor` and warn on mismatch — don't hard-fail.

See `DESIGN.md` for architecture rationale and `SPIKE.md` for the Ghostty integration research.
