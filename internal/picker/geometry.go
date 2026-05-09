package picker

// Geometry constants for the split-pane layout. Centralised so pane-sizing formulas reference
// named values instead of bare integers.
//
// Layout:
//
//	┌──────────────────────────┐  ┌──────────────────────┐
//	│ list pane (bordered)     │  │ right pane (bordered) │
//	└──────────────────────────┘  └──────────────────────┘
//	status bar (1 line)

// rightPaneBorderCols is horizontal columns consumed by the right pane's rounded border (1 per side).
// lipgloss Width() sets content+padding width; border is added on top, so subtract this when computing Width().
const rightPaneBorderCols = 2

// rightPanePaddingCols is horizontal columns consumed by the right pane's Padding(0,1) (1 per side).
const rightPanePaddingCols = 2

// rightPaneInnerWidth is the usable content width inside the right pane after border and padding overhead.
//
// NOTE: picker_helpers.go (package cli) keeps its own local copy (const rightPaneInnerWidth = 36).
// Keep both in sync if rightPaneWidth, rightPaneBorderCols, or rightPanePaddingCols change.
const rightPaneInnerWidth = rightPaneWidth - rightPaneBorderCols - rightPanePaddingCols

// listPanePaddingCols is horizontal columns consumed by the list pane's Padding(0,1) (1 per side).
const listPanePaddingCols = 2

// borderCorrectionPx is the +1 height bonus on the right pane so its bottom border aligns with the list pane.
// The list pane renders 1 visual row taller than its declared Height (a lipgloss padding/wrap interaction,
// not reproducible in unit tests). Without this the right pane's bottom border sits 1 row too high.
const borderCorrectionPx = 1
