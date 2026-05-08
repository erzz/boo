package picker

// Brand assets used in the TUI. The papa-and-baby-boos motif (see
// docs/brand/boo.svg for the polished version) appears here as ASCII
// art for places where SVG can't go.
//
// Three babies whose faces spell out "boo":
//   - baby 1: "b"  (composite face — stem + bowl)
//   - baby 2: "oo" (two ring eyes — completes the word)
//   - baby 3: "^^" (closed/sleepy eyes — pure personality, not part of spelling)

// brandHero is the full papa-Ghostty + 3-babies composition. Used as
// the right-pane decoration when the project list is empty (the
// "register your first project" state). Width = 13 columns, height = 7
// rows. Designed to fit comfortably inside the 36-col right pane with
// room for a tagline below.
const brandHero = `   ╭───────────╮
   │           │
   │    >_     │
   │           │
   │ ╭──┬──┬──╮│
   ╰─┤b │oo│^^├╯
     ╰v─╯v─╯v─╯`

// brandStrip is the compact 3-row baby strip — just the three babies
// without papa. Used as the list-pane header in split mode where
// vertical space is limited but a brand accent is still wanted.
// Width = 10 columns, height = 3 rows.
const brandStrip = `╭──┬──┬──╮
│b │oo│^^│
╰v─╯v─╯v─╯`

// brandCursor is the 1-cell ghost glyph used as the selection marker
// in the project list (replacing the old "▌ " block-and-space). The
// trailing space keeps the column alignment identical to before so
// row content doesn't shift on selection.
//
// ᗣ is U+15E3 CANADIAN SYLLABICS NA — chosen because it renders as a
// little ghost-shape in most monospace fonts (rounded top, peaked
// bottom). Not a perfect baby-boo silhouette but the closest single
// codepoint we have, and the glyph is in well-established Unicode
// blocks so it's safe across modern terminals.
const brandCursor = "ᗣ "

// brandCursorInactive is the spacer used when a row is NOT selected.
// Same width as brandCursor so all rows align.
const brandCursorInactive = "  "

// brandTagline is the static text shown below the brandHero in the
// empty state. Matches the README hero's tagline.
const brandTagline = "a haunt for every project"
