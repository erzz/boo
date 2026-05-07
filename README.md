# boo

A project launcher for the [Ghostty](https://ghostty.org) terminal emulator.

Switch between project-scoped Ghostty windows by name. Each project remembers its layout — windows, tabs, splits, working directories, startup commands — so jumping between repos takes one command instead of a minute of clicking and `cd`-ing.

```
boo projA          # open or focus the projA window
boo                # fuzzy picker over all known projects
boo new projA      # register a project (existing dir or clone from URL)
boo save           # snapshot the focused window's layout for the current project
boo list           # show all projects (use --json to script)
boo show projA     # everything boo knows about a project
boo edit projA     # open a project's layout file in $EDITOR
boo set-layout projA triple   # switch a project to a different layout template
boo layouts        # preview every available layout template
boo config         # show effective config + where each value came from
boo doctor         # sanity-check your environment
```

## Status

Pre-alpha. Under active development. macOS only.

## How it works

boo drives Ghostty via its native AppleScript / JXA API. There is no tmux, no terminal multiplexer, no leaky abstraction — splits are Ghostty splits, scrollback is Ghostty scrollback, everything Ghostty does just works. Windows you opened with boo stay open across switches, so the processes inside them keep running.

See [`docs/layouts.md`](./docs/layouts.md) for the layout YAML reference, the bundled built-ins, and copy-paste examples (Node, Go, Rust, Python). See [`docs/config.md`](./docs/config.md) for the global config schema. See [`docs/testing.md`](./docs/testing.md) for what CI runs and the manual smoke tests run before a release.

## Requirements

- macOS
- [Ghostty](https://ghostty.org) 1.3+
- Go 1.24+ (to build from source)

## Build

```
make build
./bin/boo doctor
```

## License

TBD.
