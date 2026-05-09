// Package theme owns boo's named visual themes: a small palette (six colour
// slots) loaded from built-ins or $XDG_CONFIG_HOME/boo/themes/<name>.yaml.
// Theme errors are warnings, not hard failures — a bad colour yields dim text,
// not a crash. Use `boo doctor` to surface broken themes.
package theme

// Theme is a named palette. Colour slots are stable contracts — adding slots
// is safe, renaming or removing is a breaking change for user themes. Colour
// values are lipgloss-compatible strings: ANSI 16-color ("0".."15"), 256-color
// ("16".."255"), or truecolor hex ("#rrggbb").
type Theme struct {
	// Name uniquely identifies the theme. For built-ins matches the embed
	// filename (without `.yaml`); for user themes defaults to the filename stem.
	Name string `yaml:"name"`

	// Description is a one-line human summary shown by `boo themes`.
	Description string `yaml:"description,omitempty"`

	// Colors is the palette. Missing slots fall back to the built-in default.
	Colors Colors `yaml:"colors"`
}

// Colors is the palette. Each field is a lipgloss-compatible colour string.
// Slots map 1:1 to roles in internal/picker/theme.go; keep this list minimal.
type Colors struct {
	// Accent: selection, focus, list/form titles, cursor.
	Accent string `yaml:"accent,omitempty"`

	// Info: "+ New project" row, brand highlights.
	Info string `yaml:"info,omitempty"`

	// Border: both pane borders (list + right preview).
	Border string `yaml:"border,omitempty"`

	// OK: "● running" status pill, status-bar success outcome.
	OK string `yaml:"ok,omitempty"`

	// Warn: validation errors, status-bar failure outcome.
	Warn string `yaml:"warn,omitempty"`

	// Stopped: "○ stopped" status pill, de-emphasised metadata.
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
	// Path is the embed path for built-ins, or the absolute filesystem path for user themes.
	Path string
}
