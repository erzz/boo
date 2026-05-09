package picker

// Brand assets used in the TUI.
// Three babies whose faces spell out "boo": baby 1 = "b", baby 2 = "oo", baby 3 = "^^" (sleepy).

// brandHero is the full papa-Ghostty + 3-babies composition for the empty-state right pane.
// Width = 13 columns, height = 7 rows.
const brandHero = `   ╭───────────╮
   │           │
   │    >_     │
   │           │
   │ ╭──┬──┬──╮│
   ╰─┤b │oo│^^├╯
     ╰v─╯v─╯v─╯`

// brandStrip is the compact 3-row baby strip used as the list-pane header in split mode.
// Width = 10 columns, height = 3 rows.
const brandStrip = `╭──┬──┬──╮
│b │oo│^^│
╰v─╯v─╯v─╯`

// brandCursor is the 1-cell ghost glyph (U+15E3 CANADIAN SYLLABICS NA) used as the
// selection marker. Trailing space keeps column alignment identical across rows.
const brandCursor = "ᗣ "

// brandCursorInactive is the spacer used when a row is NOT selected.
// Same width as brandCursor so all rows align.
const brandCursorInactive = "  "

// brandTagline is the static text shown below the brandHero in the
// empty state. Matches the README hero's tagline.
const brandTagline = "a haunt for every project"
