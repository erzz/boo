# boo

A project launcher for the [Ghostty](https://ghostty.org) terminal emulator.

Switch between project-scoped Ghostty windows by name. Each project remembers its layout — windows, tabs, splits, working directories, startup commands — so jumping between repos takes one command instead of a minute of clicking and `cd`-ing.

```
boo projA          # open or focus the projA window
boo                # if you're cd'd inside a known project, switch to it
boo pick           # fuzzy picker over all known projects
boo new projA      # register a project (existing dir or clone from URL)
boo list           # show all projects
boo doctor         # sanity-check your environment
```

## Status

Pre-alpha. Under active development. macOS only.

## How it works

boo drives Ghostty via its native AppleScript / JXA API. There is no tmux, no terminal multiplexer, no leaky abstraction — splits are Ghostty splits, scrollback is Ghostty scrollback, everything Ghostty does just works. Windows you opened with boo stay open across switches, so the processes inside them keep running.

See [`DESIGN.md`](./DESIGN.md) for architecture and [`SPIKE.md`](./SPIKE.md) for why this approach was chosen.

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
