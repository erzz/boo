// Package theme owns boo's named visual themes.
//
// A theme is a small palette — six colour slots — plus a name and
// description. Themes are pure data; they don't know about lipgloss or
// the picker. Consumers (today only the picker) read the colour slots
// and build their own styled rendering on top.
//
// Themes come from two places:
//
//  1. Built-in templates embedded in the binary. The `default` theme is
//     guaranteed to exist and is the fallback for every error path.
//  2. User templates in $XDG_CONFIG_HOME/boo/themes/<name>.yaml. User
//     templates with the same name as a built-in shadow the built-in.
//
// Validation is deliberately lenient: a malformed user theme produces a
// warning, not a hard error. Themes are cosmetic — a typo in a colour
// shouldn't block the picker. Use `boo doctor` to surface broken themes.
//
// This is the opposite of `internal/config`'s strict-validation policy
// for `config.yaml` (which fails hard on malformed YAML). The
// difference: config errors mask user mistakes that change behaviour;
// theme errors at worst yield ugly colours.
package theme

// Theme is a named palette. Slots are stable contracts — once a slot
// exists, removing or renaming it is a breaking change for user-authored
// themes. Add slots conservatively; merging slots later is painful.
//
// Colour values are lipgloss-compatible strings:
//
//   - ANSI 16-color: "0".."15"
//   - ANSI 256-color: "16".."255"
//   - Truecolor hex: "#rrggbb"
//
// Lipgloss tolerates garbage colour strings (renders as terminal default
// foreground), so a typo in a user theme produces dim text rather than a
// crash. The picker still calls Resolve to surface lookup errors via
// `boo doctor`.
type Theme struct {
	// Name uniquely identifies the theme. For built-ins this matches
	// the embed filename (without `.yaml`). For user themes it
	// defaults to the filename stem if the YAML omits an explicit
	// `name:` field.
	Name string `yaml:"name"`

	// Description is a one-line human summary shown by `boo themes`.
	// Optional; empty descriptions just collapse to name + path.
	Description string `yaml:"description,omitempty"`

	// Colors is the palette. Missing slots fall back to the built-in
	// default theme's value for that slot — themes can override only
	// the slots they care about.
	Colors Colors `yaml:"colors"`
}

// Colors is the palette. Every field is a lipgloss-compatible colour
// string (ANSI index, 256-color index, or `#rrggbb`).
//
// Slots map 1:1 to roles in `internal/picker/theme.go`. Keep this
// list minimal — every slot is a forever contract.
type Colors struct {
	// Selection, focus, list/form titles, right-pane project name,
	// list pane title, cursor (`ᗣ `).
	Accent string `yaml:"accent,omitempty"`

	// "+ New project" row in the list, prompt-like brand highlights.
	Info string `yaml:"info,omitempty"`

	// Both pane borders (list + right preview).
	Border string `yaml:"border,omitempty"`

	// "● running" project status pill, status-bar success outcome.
	OK string `yaml:"ok,omitempty"`

	// "✖ dir missing", validation errors, status-bar failure outcome.
	Warn string `yaml:"warn,omitempty"`

	// "○ stopped" status pill, neutral foreground for de-emphasized
	// metadata.
	Stopped string `yaml:"stopped,omitempty"`
}

// Source describes where a resolved theme came from.
type Source string

const (
	SourceBuiltin Source = "builtin"
	SourceUser    Source = "user"
)

// Resolved is a Theme plus its provenance.
type Resolved struct {
	Theme  Theme
	Source Source
	// Path is the embed path for built-ins, or the absolute filesystem
	// path for user themes.
	Path string
}
