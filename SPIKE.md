# Ghostty Integration Spike

Date: 2026-05-06
Ghostty version tested: 1.3.1 (stable)
Outcome: **GO** — AppleScript via JXA gives us everything boo needs.

## Question

Can we control Ghostty from outside well enough to build boo (open windows/tabs/splits with cwd, command, optionally targeting a specific window)?

## Findings

### CLI surface — limited

`ghostty` exposes `+actions` (`+new-window`, `+show-config`, `+list-actions`, etc.) but most window/tab/split management actions are keybinding-only. Notably `+new-window` returns *"not supported on this platform"* on macOS — on macOS the CLI is essentially "actions only," and the recommended launch is `open -na Ghostty.app`.

CLI is fine for: launching the app, single-window launch with `--working-directory`, single startup command via `-e <cmd>`. Not enough for boo's layouts.

### AppleScript via `Ghostty.sdef` — comprehensive

Located at `/Applications/Ghostty.app/Contents/Resources/Ghostty.sdef`. This is the real API. Capabilities:

| Capability | Mechanism |
|---|---|
| Open new window | `new window with configuration {…}` |
| Open new tab in *specific* window | `new tab in window id <id> with configuration {…}` |
| Split a terminal in any direction | `split <terminal> direction (right\|left\|up\|down) with configuration {…}` |
| Set cwd, command, env vars per surface | `surface configuration` record (workingDirectory, command, environmentVariables, initialInput, fontSize, waitAfterCommand) |
| Send "typed" input after launch | `initialInput` (sleeper feature for shell init) |
| Enumerate windows / tabs / terminals | `windows`, `tabs`, `terminals` element collections with stable IDs |
| Activate / focus / close window | `activate window`, `close window`, etc. |
| Read working dir of any terminal | `workingDirectory` property on terminal |
| Send arbitrary keys/text/mouse | `input text`, `send key`, `send mouse button` (escape hatch) |

Smoke test confirmed working without any permission prompt for read-only:
```
$ osascript -l JavaScript -e 'Application("Ghostty").version()'
1.3.1
```

### Implications

1. **No tmux.** AppleScript handles persistence-by-not-closing and rich layout creation natively.
2. **Window targeting works** via stable IDs, so boo can reliably "focus the projA window" or "close all windows belonging to projA."
3. **Splits are first-class** — direction + per-pane surface config (cwd, command, env vars).
4. **`initialInput`** lets us drive shell-aware setup (direnv, mise, etc.) without hacks.
5. **macOS-only is a hard reality** for the rich integration. Linux/Windows would need a parallel CLI-only path. Explicit non-goal for v1.

### Real-world quirks discovered during scaffold

- The `.sdef` declares `new surface configuration` as a **command**, not a class. In JXA you call it as `app.newSurfaceConfiguration({...})` (lowercase, command-style), not `app.NewSurfaceConfiguration(...)`. Calling the latter yields `"NewSurfaceConfiguration is not a valid class for application Ghostty"`.
- **Surface configuration properties are silently dropped if passed to the constructor.** Verified on Ghostty 1.3.x: `app.newSurfaceConfiguration({initialWorkingDirectory: "/tmp"})` returns a config object, but the field is ignored — windows open in the cwd of the `osascript` process. The properties **must** be assigned to the returned record after construction:
  ```js
  const cfg = app.newSurfaceConfiguration();
  cfg.initialWorkingDirectory = "/tmp";  // this works
  ```
  Re-test if Ghostty's AppleScript dictionary changes.
- Window IDs returned look like `"tab-group-9a98cc000"` — opaque strings, fine for our purposes.
- No Automation permission prompt was triggered for either read-only (`version`) or write (`new window`) operations from the OpenCode-spawned shell. May differ from a fresh user's first run; `boo doctor` should still surface the possibility.

### Open risks

- **Stable ID lifetime.** The sdef says "Stable ID for this window" but doesn't promise survival across Ghostty restarts. Likely process-lifetime only. If so, fine — when Ghostty dies all windows die too, so we just regenerate. Test once we have code.
- **Automation permission prompt.** First write-action will trigger macOS "X wants to control Ghostty.app" prompt. `boo doctor` should detect and surface this clearly.
- **`osascript` startup latency** ~50–100ms. Invisible per launch; acceptable for picker.

## Decision

Boo's primary integration with Ghostty is **JXA (JavaScript for Automation) scripts executed via `osascript -l JavaScript`**. Generated from Go templates. Stable IDs returned as JSON and stored in project state.

CLI invocation (`open -na Ghostty.app …`) reserved for cold-start (Ghostty not running) and as a documented fallback.
