// Package layout defines boo's layout vocabulary and the rules for parsing it
// from TOML.
//
// The model is intentionally small for Phase 1:
//
//   - A Layout has one or more Tabs.
//   - Each Tab has one or more Splits.
//   - The first Split in a Tab is its primary surface (no direction). Any
//     subsequent Splits carry a Direction (right|left|up|down) and split
//     off the previous surface in that tab.
//
// Each Split carries cwd / command / env / initial input. cwd values may be
// relative; resolution against the project directory happens at apply time
// (not here).
//
// Splits are deferred for v1 in DESIGN.md but the schema accepts them already
// so layout files don't need a v2 migration when the JXA layer learns splits.
package layout

import (
	"errors"
	"fmt"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// Direction values accepted in a Split.
const (
	DirRight = "right"
	DirLeft  = "left"
	DirDown  = "down"
	DirUp    = "up"
)

// Layout is a complete project layout.
type Layout struct {
	Name string `toml:"name"`
	Tabs []Tab  `toml:"tab"`
}

// Tab is a single Ghostty tab, with one or more Splits.
type Tab struct {
	Name   string  `toml:"name"`
	Splits []Split `toml:"split"`
}

// Split is one terminal surface inside a Tab.
type Split struct {
	Direction    string            `toml:"direction,omitempty"`
	Cwd          string            `toml:"cwd,omitempty"`
	Command      string            `toml:"command,omitempty"`
	InitialInput string            `toml:"initial_input,omitempty"`
	Env          map[string]string `toml:"env,omitempty"`
}

// Default returns boo's built-in default layout: a single tab with a single
// shell at the project root. Used when `boo new` is called without --layout.
func Default() Layout {
	return Layout{
		Name: "default",
		Tabs: []Tab{{
			Name:   "shell",
			Splits: []Split{{Cwd: "."}},
		}},
	}
}

// Parse decodes a TOML document into a Layout, then validates it.
func Parse(data []byte) (Layout, error) {
	var l Layout
	if err := toml.Unmarshal(data, &l); err != nil {
		return Layout{}, fmt.Errorf("layout: parse: %w", err)
	}
	if err := l.Validate(); err != nil {
		return Layout{}, err
	}
	return l, nil
}

// Marshal encodes a Layout as TOML.
func Marshal(l Layout) ([]byte, error) {
	if err := l.Validate(); err != nil {
		return nil, err
	}
	return toml.Marshal(l)
}

// Validate enforces structural rules.
func (l Layout) Validate() error {
	if strings.TrimSpace(l.Name) == "" {
		return errors.New("layout: name is required")
	}
	if len(l.Tabs) == 0 {
		return errors.New("layout: at least one tab is required")
	}
	for i, t := range l.Tabs {
		if len(t.Splits) == 0 {
			return fmt.Errorf("layout: tab %d (%q): at least one split is required", i, t.Name)
		}
		for j, s := range t.Splits {
			if j == 0 && s.Direction != "" {
				return fmt.Errorf("layout: tab %d (%q) split 0: primary split must not have a direction", i, t.Name)
			}
			if j > 0 && !validDirection(s.Direction) {
				return fmt.Errorf("layout: tab %d (%q) split %d: direction %q is not one of right|left|up|down", i, t.Name, j, s.Direction)
			}
		}
	}
	return nil
}

func validDirection(d string) bool {
	switch d {
	case DirRight, DirLeft, DirUp, DirDown:
		return true
	default:
		return false
	}
}
